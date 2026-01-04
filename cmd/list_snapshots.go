package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ivan4th/ameriagrab/client"
	"github.com/ivan4th/ameriagrab/output"
	"github.com/spf13/cobra"
)

var listSnapshotsJSONOutput bool

// SnapshotJSON is the JSON representation of a snapshot
type SnapshotJSON struct {
	ID        int64                `json:"id"`
	CreatedAt time.Time            `json:"created_at"`
	Products  []client.ProductInfo `json:"products"`
}

var listSnapshotsCmd = &cobra.Command{
	Use:   "list-snapshots",
	Short: "List balance snapshots",
	Long: `Lists all balance snapshots stored in the local database.

Each snapshot shows account/card balances at a specific point in time.
Snapshots are created using 'sync --snapshot'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := OpenDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		snapshots, err := database.GetSnapshots()
		if err != nil {
			return fmt.Errorf("fetching snapshots: %w", err)
		}

		if len(snapshots) == 0 {
			fmt.Println("No snapshots found. Use 'sync --snapshot' to create one.")
			return nil
		}

		if listSnapshotsJSONOutput {
			// Convert to JSON format
			jsonSnapshots := make([]SnapshotJSON, len(snapshots))
			for i, s := range snapshots {
				jsonSnapshots[i] = SnapshotJSON{
					ID:        s.ID,
					CreatedAt: s.CreatedAt,
					Products:  s.Products,
				}
			}

			out, err := json.MarshalIndent(jsonSnapshots, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling snapshots: %w", err)
			}
			fmt.Println(string(out))
		} else {
			output.PrintSnapshots(snapshots)
		}

		return nil
	},
}

func init() {
	listSnapshotsCmd.Flags().BoolVarP(&listSnapshotsJSONOutput, "json", "j", false, "Output as JSON")
}
