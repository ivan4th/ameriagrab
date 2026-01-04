package db

import (
	"fmt"
)

// Current schema version
const schemaVersion = 5

// migrations is a list of SQL statements to run for each version
var migrations = []string{
	// Version 1: Initial schema
	`
	-- Products table (cards and accounts)
	CREATE TABLE IF NOT EXISTS products (
		id TEXT PRIMARY KEY,
		product_type TEXT NOT NULL,
		name TEXT,
		card_number TEXT,
		account_number TEXT,
		account_id TEXT,
		currency TEXT,
		balance REAL,
		available_balance REAL,
		status TEXT,
		order_index INTEGER NOT NULL DEFAULT 0,
		synced_at INTEGER NOT NULL
	);

	-- Card transactions (from GetTransactions - card-specific transactions)
	-- Uses composite key (id, operation_date) to allow multiple entries with same backend ID
	CREATE TABLE IF NOT EXISTS card_transactions (
		id TEXT NOT NULL,
		product_id TEXT NOT NULL,
		transaction_type TEXT,
		accounting_type TEXT,
		state TEXT,
		amount_currency TEXT,
		amount_value REAL,
		correspondent_account_number TEXT,
		correspondent_account_name TEXT,
		details TEXT,
		operation_date TEXT NOT NULL,
		workflow_code TEXT,
		date TEXT,
		year TEXT,
		month TEXT,
		synced_at INTEGER NOT NULL,
		PRIMARY KEY (id, operation_date)
	);
	CREATE INDEX IF NOT EXISTS idx_card_txn_product_date ON card_transactions(product_id, operation_date);

	-- Card linked account transactions (from GetEventsPast - card's linked account history)
	-- Uses composite key (id, operation_date) to allow multiple entries with same backend ID
	CREATE TABLE IF NOT EXISTS card_linked_account_transactions (
		id TEXT NOT NULL,
		product_id TEXT NOT NULL,
		transaction_type TEXT,
		accounting_type TEXT,
		state TEXT,
		amount_currency TEXT,
		amount_value REAL,
		correspondent_account_number TEXT,
		correspondent_account_name TEXT,
		details TEXT,
		operation_date TEXT NOT NULL,
		workflow_code TEXT,
		date TEXT,
		year TEXT,
		month TEXT,
		synced_at INTEGER NOT NULL,
		-- Extended info columns (populated during sync)
		beneficiary_name TEXT,
		beneficiary_address TEXT,
		credit_account_number TEXT,
		card_masked_number TEXT,
		ext_operation_id TEXT,
		swift_details TEXT,
		extended_fetched INTEGER DEFAULT 0,
		PRIMARY KEY (id, operation_date)
	);
	CREATE INDEX IF NOT EXISTS idx_card_linked_txn_product_date ON card_linked_account_transactions(product_id, operation_date);

	-- Account transactions (from GetAccountHistory)
	CREATE TABLE IF NOT EXISTS account_transactions (
		id TEXT PRIMARY KEY,
		product_id TEXT NOT NULL,
		transaction_id TEXT,
		operation_id TEXT,
		status TEXT,
		transaction_type TEXT,
		workflow_code TEXT,
		flow_direction TEXT,
		transaction_date INTEGER,
		settled_date INTEGER,
		date TEXT,
		month TEXT,
		year TEXT,
		debit_account_number TEXT,
		credit_account_number TEXT,
		beneficiary_name TEXT,
		details TEXT,
		source_system TEXT,
		transaction_amount_currency TEXT,
		transaction_amount_value REAL,
		settled_amount_currency TEXT,
		settled_amount_value REAL,
		domestic_amount_currency TEXT,
		domestic_amount_value REAL,
		synced_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_acct_txn_product_date ON account_transactions(product_id, transaction_date);
	`,
	// Version 2: Snapshots
	`
	-- Snapshots table (point-in-time balance captures)
	CREATE TABLE IF NOT EXISTS snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at INTEGER NOT NULL
	);

	-- Snapshot products (balance data at snapshot time)
	CREATE TABLE IF NOT EXISTS snapshot_products (
		snapshot_id INTEGER NOT NULL,
		product_id TEXT NOT NULL,
		product_type TEXT NOT NULL,
		name TEXT,
		card_number TEXT,
		account_number TEXT,
		currency TEXT,
		balance REAL,
		available_balance REAL,
		status TEXT,
		order_index INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (snapshot_id, product_id),
		FOREIGN KEY (snapshot_id) REFERENCES snapshots(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_snapshot_products_snapshot ON snapshot_products(snapshot_id);
	`,
	// Version 3: Transfer templates
	`
	-- Transfer templates (for enriching transaction counterparty display)
	CREATE TABLE IF NOT EXISTS transfer_templates (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		workflow_code TEXT,
		masked_card_number TEXT,
		account_number TEXT,
		beneficiary TEXT,
		synced_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_transfer_templates_card ON transfer_templates(masked_card_number);
	CREATE INDEX IF NOT EXISTS idx_transfer_templates_account ON transfer_templates(account_number);
	`,
	// Version 4: Add card_key column for normalized card number matching
	`
	ALTER TABLE transfer_templates ADD COLUMN card_key TEXT;
	CREATE INDEX IF NOT EXISTS idx_transfer_templates_card_key ON transfer_templates(card_key);
	`,
	// Version 5: Session storage (replaces file-based session)
	`
	CREATE TABLE IF NOT EXISTS session (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		access_token TEXT NOT NULL,
		refresh_token TEXT NOT NULL,
		expires_at INTEGER NOT NULL,
		client_id TEXT,
		cookies_json TEXT,
		updated_at INTEGER NOT NULL
	);
	`,
}

// Migrate runs all pending migrations
func (db *DB) Migrate() error {
	// Create schema_version table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	// Get current version
	var currentVersion int
	row := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	// Run pending migrations
	for i := currentVersion; i < len(migrations); i++ {
		version := i + 1
		if _, err := db.Exec(migrations[i]); err != nil {
			return fmt.Errorf("failed to run migration %d: %w", version, err)
		}
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", version); err != nil {
			return fmt.Errorf("failed to record migration %d: %w", version, err)
		}
	}

	return nil
}

// GetSchemaVersion returns the current schema version
func (db *DB) GetSchemaVersion() (int, error) {
	var version int
	row := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version")
	if err := row.Scan(&version); err != nil {
		return 0, fmt.Errorf("failed to get schema version: %w", err)
	}
	return version, nil
}
