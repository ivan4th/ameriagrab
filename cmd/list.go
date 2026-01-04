package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/ivan4th/ameriagrab/output"
	"github.com/spf13/cobra"
)

var listJSONOutput bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all accounts and cards",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, accessToken, err := SetupClient()
		if err != nil {
			return err
		}

		resp, err := client.GetAccountsAndCards(accessToken)
		if err != nil {
			return fmt.Errorf("fetching accounts and cards: %w", err)
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
}
