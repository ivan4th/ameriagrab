package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/ivan4th/ameriagrab/client"
	"github.com/ivan4th/ameriagrab/output"
	"github.com/spf13/cobra"
)

var (
	listJSONOutput bool
	listLocal      bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all accounts and cards",
	RunE: func(cmd *cobra.Command, args []string) error {
		var resp *client.AccountsAndCardsResponse

		if listLocal {
			// Load from local database
			database, err := OpenDatabase()
			if err != nil {
				return err
			}
			defer database.Close()

			products, err := database.GetProducts()
			if err != nil {
				return fmt.Errorf("fetching products from database: %w", err)
			}

			resp = &client.AccountsAndCardsResponse{
				Status: "success",
			}
			resp.Data.AccountsAndCards = products
		} else {
			// Fetch from API
			c, accessToken, err := SetupClient()
			if err != nil {
				return err
			}

			resp, err = c.GetAccountsAndCards(accessToken)
			if err != nil {
				return fmt.Errorf("fetching accounts and cards: %w", err)
			}

			// Fetch available balance for each product
			for i := range resp.Data.AccountsAndCards {
				p := &resp.Data.AccountsAndCards[i]
				balResp, err := c.GetAvailableBalance(accessToken, p.ProductType, p.ID)
				if err != nil {
					return fmt.Errorf("fetching available balance for %s: %w", p.ID, err)
				}
				p.AvailableBalance = balResp.Data.AvailableBalance
			}
		}

		if listJSONOutput {
			out, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling response: %w", err)
			}
			fmt.Println(string(out))
		} else {
			output.PrintAccountsAndCards(resp)
		}
		return nil
	},
}

func init() {
	listCmd.Flags().BoolVarP(&listJSONOutput, "json", "j", false, "Output as JSON")
	listCmd.Flags().BoolVarP(&listLocal, "local", "l", false, "Read from local database")
}
