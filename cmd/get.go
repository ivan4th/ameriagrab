package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ivan4th/ameriagrab/output"
	"github.com/spf13/cobra"
)

var (
	getSize            int
	getPage            int
	getJSONOutput      bool
	getForceAccountAPI bool
)

var getCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get transactions for a card or account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

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
			txns, err := c.GetEventsPast(accessToken, accountID, getSize, getPage)
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
			history, err := c.GetAccountHistory(accessToken, id, getSize, getPage)
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
	},
}

func init() {
	getCmd.Flags().IntVarP(&getSize, "size", "s", 50, "Number of transactions to fetch")
	getCmd.Flags().IntVarP(&getPage, "page", "p", 0, "Page number (0-indexed)")
	getCmd.Flags().BoolVarP(&getJSONOutput, "json", "j", false, "Output as JSON")
	getCmd.Flags().BoolVarP(&getForceAccountAPI, "account", "a", false, "Use account history API (even for cards)")
}
