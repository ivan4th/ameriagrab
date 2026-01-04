package cmd

import (
	"fmt"
	"os"

	"github.com/ivan4th/ameriagrab/client"
	"github.com/ivan4th/ameriagrab/db"
	"github.com/spf13/cobra"
)

// RootCmd represents the base command
var RootCmd = &cobra.Command{
	Use:   "ameriagrab",
	Short: "Ameriabank transaction grabber",
	Long: `ameriagrab retrieves accounts, cards, and transaction data from Ameriabank.

Environment variables:
  AMERIA_USERNAME  - Ameriabank username (required)
  AMERIA_PASSWORD  - Ameriabank password (required)
  AMERIA_DEBUG_DIR - Directory to save debug files (optional)
  AMERIA_DB_PATH   - Path to SQLite database for sync/local mode and session persistence (optional)`,
}

// SetupClient creates and authenticates the Ameriabank client
func SetupClient() (*client.Client, string, error) {
	username := os.Getenv("AMERIA_USERNAME")
	password := os.Getenv("AMERIA_PASSWORD")
	debugDir := os.Getenv("AMERIA_DEBUG_DIR")

	if username == "" || password == "" {
		return nil, "", fmt.Errorf("AMERIA_USERNAME and AMERIA_PASSWORD environment variables must be set")
	}

	// Try to use database for session storage if AMERIA_DB_PATH is set
	var sessionStorage client.SessionStorage
	dbPath := os.Getenv("AMERIA_DB_PATH")
	if dbPath != "" {
		database, err := db.Open(dbPath)
		if err != nil {
			return nil, "", fmt.Errorf("opening database for session: %w", err)
		}
		sessionStorage = database
		// Note: we don't close the database here - it will be used for the session
		// The caller should manage the database lifecycle if needed
	}

	c, err := client.NewClient(username, password, sessionStorage, debugDir)
	if err != nil {
		return nil, "", fmt.Errorf("creating client: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Checking for saved session or logging in...")
	accessToken, err := c.GetOrRefreshToken()
	if err != nil {
		return nil, "", fmt.Errorf("getting access token: %w", err)
	}

	// Initialize session with prerequisite API calls (only if clientID not restored)
	if c.ClientID == "" {
		fmt.Fprintln(os.Stderr, "Client ID not found in session, initializing...")
		if err := c.InitializeSession(accessToken); err != nil {
			return nil, "", fmt.Errorf("initializing session: %w", err)
		}
		if err := c.UpdateSessionClientID(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update session with client ID: %v\n", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Using restored Client ID: %s\n", c.ClientID)
	}

	return c, accessToken, nil
}

// OpenDatabase opens the SQLite database from AMERIA_DB_PATH
func OpenDatabase() (*db.DB, error) {
	dbPath := os.Getenv("AMERIA_DB_PATH")
	if dbPath == "" {
		return nil, fmt.Errorf("AMERIA_DB_PATH environment variable must be set")
	}

	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	return database, nil
}

func init() {
	RootCmd.AddCommand(listCmd)
	RootCmd.AddCommand(getCmd)
	RootCmd.AddCommand(syncCmd)
	RootCmd.AddCommand(listSnapshotsCmd)
}
