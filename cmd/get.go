package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ivan4th/ameriagrab/client"
	"github.com/ivan4th/ameriagrab/output"
	"github.com/spf13/cobra"
)

var (
	getSize            int
	getPage            int
	getJSONOutput      bool
	getForceAccountAPI bool
	getLocal           bool
)

var getCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get transactions for a card or account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		if getLocal {
			return getFromLocal(id)
		}
		return getFromAPI(id)
	},
}

func getFromLocal(id string) error {
	database, err := OpenDatabase()
	if err != nil {
		return err
	}
	defer database.Close()

	// Get product info
	product, err := database.GetProductByID(id)
	if err != nil {
		return fmt.Errorf("fetching product: %w", err)
	}
	if product == nil {
		return fmt.Errorf("ID %s not found in database", id)
	}

	if product.ProductType == "CARD" {
		var txns []client.Transaction
		var err error
		var totalCount int

		if getForceAccountAPI {
			// Get linked account transactions with pagination
			// size=0 means no limit for DB
			txns, err = database.GetLinkedAccountTransactions(id, getSize, getPage)
			if err != nil {
				return fmt.Errorf("fetching linked account transactions: %w", err)
			}
			totalCount, err = database.CountLinkedAccountTransactions(id)
			if err != nil {
				return fmt.Errorf("counting linked account transactions: %w", err)
			}
		} else {
			// Get card transactions with pagination
			// size=0 means no limit for DB
			txns, err = database.GetCardTransactions(id, getSize, getPage)
			if err != nil {
				return fmt.Errorf("fetching card transactions: %w", err)
			}
			totalCount, err = database.CountCardTransactions(id)
			if err != nil {
				return fmt.Errorf("counting card transactions: %w", err)
			}
		}

		resp := &client.TransactionsResponse{
			Status: "success",
		}
		resp.Data.TotalCount = totalCount
		resp.Data.Entries = txns

		if getJSONOutput {
			out, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling response: %w", err)
			}
			fmt.Println(string(out))
		} else {
			output.PrintCardTransactions(resp)
		}
	} else {
		// For accounts, return account transactions from DB
		txns, err := database.GetAccountTransactions(id)
		if err != nil {
			return fmt.Errorf("fetching account transactions: %w", err)
		}

		resp := &client.HistoryResponse{
			Status: "success",
		}
		resp.Data.Transactions = txns
		resp.Data.HasNext = false
		resp.Data.IsUpToDate = true

		if getJSONOutput {
			out, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling response: %w", err)
			}
			fmt.Println(string(out))
		} else {
			output.PrintAccountHistory(resp)
		}
	}

	return nil
}

func getFromAPI(id string) error {
	c, accessToken, err := SetupClient()
	if err != nil {
		return err
	}

	// First, determine if this is a card or account
	resp, err := c.GetAccountsAndCards(accessToken)
	if err != nil {
		return fmt.Errorf("fetching accounts and cards: %w", err)
	}

	var productType, accountID string
	for _, p := range resp.Data.AccountsAndCards {
		if p.ID == id {
			productType = p.ProductType
			accountID = p.AccountID
			break
		}
	}

	if productType == "" {
		return fmt.Errorf("ID %s not found in accounts or cards", id)
	}

	if productType == "CARD" && !getForceAccountAPI {
		// Card: use settled events API
		fmt.Fprintln(os.Stderr, "Fetching card transactions...")
		txns, err := c.GetTransactions(accessToken, id)
		if err != nil {
			return fmt.Errorf("fetching card transactions: %w", err)
		}
		if getJSONOutput {
			out, err := json.MarshalIndent(txns, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling response: %w", err)
			}
			fmt.Println(string(out))
		} else {
			output.PrintCardTransactions(txns)
		}
	} else if productType == "CARD" && getForceAccountAPI {
		// Card with --account flag: use events/past API with linked account ID
		if accountID == "" {
			return fmt.Errorf("card %s has no linked account ID", id)
		}
		fmt.Fprintf(os.Stderr, "Fetching card account history (events/past) for account %s...\n", accountID)
		// size=0 means use 1000 for API
		apiSize := getSize
		if apiSize == 0 {
			apiSize = 1000
		}
		txns, err := c.GetEventsPast(accessToken, accountID, apiSize, getPage)
		if err != nil {
			return fmt.Errorf("fetching card account history: %w", err)
		}
		if getJSONOutput {
			out, err := json.MarshalIndent(txns, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling response: %w", err)
			}
			fmt.Println(string(out))
		} else {
			output.PrintCardTransactions(txns)
		}
	} else {
		// Account: use history API
		fmt.Fprintln(os.Stderr, "Fetching account history...")
		// size=0 means use 1000 for API
		apiSize := getSize
		if apiSize == 0 {
			apiSize = 1000
		}
		history, err := c.GetAccountHistory(accessToken, id, apiSize, getPage)
		if err != nil {
			return fmt.Errorf("fetching account history: %w", err)
		}
		if getJSONOutput {
			out, err := json.MarshalIndent(history, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling response: %w", err)
			}
			fmt.Println(string(out))
		} else {
			output.PrintAccountHistory(history)
		}
	}

	return nil
}

func init() {
	getCmd.Flags().IntVarP(&getSize, "size", "s", 50, "Number of transactions to fetch")
	getCmd.Flags().IntVarP(&getPage, "page", "p", 0, "Page number (0-indexed)")
	getCmd.Flags().BoolVarP(&getJSONOutput, "json", "j", false, "Output as JSON")
	getCmd.Flags().BoolVarP(&getForceAccountAPI, "account", "a", false, "Use account history API (even for cards)")
	getCmd.Flags().BoolVarP(&getLocal, "local", "l", false, "Read from local database")
}
