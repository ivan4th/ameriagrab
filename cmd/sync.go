package cmd

import (
	"fmt"
	"os"

	"github.com/ivan4th/ameriagrab/client"
	"github.com/ivan4th/ameriagrab/db"
	"github.com/spf13/cobra"
)

var (
	syncVerbose bool
	syncForce   bool
)

const syncPageSize = 1000

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync all transactions to local database",
	Long: `Downloads all accounts, cards, and their transactions to a local SQLite database.

For cards, fetches both card transactions and linked account transactions.
Uses page size of 1000 to efficiently download all history.

Environment variables:
  AMERIA_DB_PATH - Path to SQLite database file (required)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Open database
		database, err := OpenDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		// Setup client and authenticate
		c, accessToken, err := SetupClient()
		if err != nil {
			return err
		}

		// Fetch and store products
		fmt.Fprintln(os.Stderr, "Fetching accounts and cards...")
		resp, err := c.GetAccountsAndCards(accessToken)
		if err != nil {
			return fmt.Errorf("fetching accounts and cards: %w", err)
		}

		if err := database.UpsertProducts(resp.Data.AccountsAndCards); err != nil {
			return fmt.Errorf("storing products: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Stored %d products\n", len(resp.Data.AccountsAndCards))

		// Sync transactions for each product
		for _, p := range resp.Data.AccountsAndCards {
			if p.ProductType == "CARD" {
				if err := syncCard(database, c, accessToken, p.ID, p.AccountID, p.Name); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: error syncing card %s: %v\n", p.ID, err)
				}
			} else {
				if err := syncAccount(database, c, accessToken, p.ID, p.Name); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: error syncing account %s: %v\n", p.ID, err)
				}
			}
		}

		fmt.Fprintln(os.Stderr, "Sync complete!")
		return nil
	},
}

func syncCard(database *db.DB, c interface {
	GetTransactions(accessToken, cardID string) (*client.TransactionsResponse, error)
	GetEventsPast(accessToken, accountID string, size, page int) (*client.TransactionsResponse, error)
}, accessToken, cardID, linkedAccountID, name string) error {
	if syncVerbose {
		fmt.Fprintf(os.Stderr, "Syncing card: %s (%s)\n", name, cardID)
	}

	// Get existing card transaction keys for deduplication
	existingCardKeys, err := database.GetExistingCardTxnKeys(cardID)
	if err != nil {
		return fmt.Errorf("getting existing card keys: %w", err)
	}

	// Fetch card transactions (GetTransactions)
	if syncVerbose {
		fmt.Fprintf(os.Stderr, "  Fetching card transactions...\n")
	}
	txnResp, err := c.GetTransactions(accessToken, cardID)
	if err != nil {
		return fmt.Errorf("fetching card transactions: %w", err)
	}

	// Filter new transactions (by composite key: id + operation_date)
	var newTxns []client.Transaction
	for _, t := range txnResp.Data.Entries {
		key := db.TxnKey(t.ID, t.OperationDate)
		if !existingCardKeys[key] {
			newTxns = append(newTxns, t)
		}
	}

	if len(newTxns) > 0 {
		inserted, err := database.InsertCardTransactions(cardID, newTxns)
		if err != nil {
			return fmt.Errorf("inserting card transactions: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  Card %s: +%d card transactions\n", name, inserted)
	} else if syncVerbose {
		fmt.Fprintf(os.Stderr, "  Card %s: no new card transactions\n", name)
	}

	// Fetch linked account transactions if available (GetEventsPast)
	if linkedAccountID != "" {
		if err := syncCardAccountTransactions(database, c, accessToken, cardID, linkedAccountID, name); err != nil {
			return fmt.Errorf("syncing linked account: %w", err)
		}
	}

	return nil
}

func syncCardAccountTransactions(database *db.DB, c interface {
	GetEventsPast(accessToken, accountID string, size, page int) (*client.TransactionsResponse, error)
}, accessToken, cardID, accountID, name string) error {
	if syncVerbose {
		fmt.Fprintf(os.Stderr, "  Fetching linked account transactions (account %s)...\n", accountID)
	}

	// Get existing linked account transaction keys for this card
	existingKeys, err := database.GetExistingLinkedAccountTxnKeys(cardID)
	if err != nil {
		return fmt.Errorf("getting existing linked account keys: %w", err)
	}

	totalInserted := 0
	page := 0

	for {
		resp, err := c.GetEventsPast(accessToken, accountID, syncPageSize, page)
		if err != nil {
			return fmt.Errorf("fetching events/past page %d: %w", page, err)
		}

		if len(resp.Data.Entries) == 0 {
			break
		}

		// Check for new transactions (by composite key: id + operation_date)
		var newTxns []client.Transaction
		allExist := true
		for _, t := range resp.Data.Entries {
			key := db.TxnKey(t.ID, t.OperationDate)
			if !existingKeys[key] {
				newTxns = append(newTxns, t)
				existingKeys[key] = true // Mark as seen
				allExist = false
			}
		}

		if len(newTxns) > 0 {
			inserted, err := database.InsertLinkedAccountTransactions(cardID, newTxns)
			if err != nil {
				return fmt.Errorf("inserting linked account transactions: %w", err)
			}
			totalInserted += inserted
		}

		// Stop if: less than page size returned, OR all transactions already existed
		if len(resp.Data.Entries) < syncPageSize || allExist {
			break
		}

		page++
		if syncVerbose {
			fmt.Fprintf(os.Stderr, "  Fetching page %d...\n", page)
		}
	}

	if totalInserted > 0 {
		fmt.Fprintf(os.Stderr, "  Card %s: +%d linked account transactions\n", name, totalInserted)
	} else if syncVerbose {
		fmt.Fprintf(os.Stderr, "  Card %s: no new linked account transactions\n", name)
	}

	return nil
}

func syncAccount(database *db.DB, c interface {
	GetAccountHistory(accessToken, accountID string, size, page int) (*client.HistoryResponse, error)
}, accessToken, accountID, name string) error {
	if syncVerbose {
		fmt.Fprintf(os.Stderr, "Syncing account: %s (%s)\n", name, accountID)
	}

	// Get existing transaction IDs for deduplication
	existingIDs, err := database.GetExistingAccountTxnIDs(accountID)
	if err != nil {
		return fmt.Errorf("getting existing IDs: %w", err)
	}

	totalInserted := 0
	page := 0

	for {
		resp, err := c.GetAccountHistory(accessToken, accountID, syncPageSize, page)
		if err != nil {
			return fmt.Errorf("fetching history page %d: %w", page, err)
		}

		if len(resp.Data.Transactions) == 0 {
			break
		}

		// Check for new transactions
		var newTxns []client.AccountTransaction
		allExist := true
		for _, t := range resp.Data.Transactions {
			if !existingIDs[t.ID] {
				newTxns = append(newTxns, t)
				existingIDs[t.ID] = true // Mark as seen
				allExist = false
			}
		}

		if len(newTxns) > 0 {
			inserted, err := database.InsertAccountTransactions(accountID, newTxns)
			if err != nil {
				return fmt.Errorf("inserting transactions: %w", err)
			}
			totalInserted += inserted
		}

		// Stop if: no more pages, or all transactions already existed
		if !resp.Data.HasNext || allExist {
			break
		}

		page++
		if syncVerbose {
			fmt.Fprintf(os.Stderr, "  Fetching page %d...\n", page)
		}
	}

	if totalInserted > 0 {
		fmt.Fprintf(os.Stderr, "  Account %s: +%d transactions\n", name, totalInserted)
	} else if syncVerbose {
		fmt.Fprintf(os.Stderr, "  Account %s: no new transactions\n", name)
	}

	return nil
}

func init() {
	syncCmd.Flags().BoolVarP(&syncVerbose, "verbose", "v", false, "Verbose output")
	syncCmd.Flags().BoolVarP(&syncForce, "force", "f", false, "Force re-sync all transactions")
}
