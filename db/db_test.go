package db

import (
	"database/sql"
	"testing"

	"github.com/ivan4th/ameriagrab/client"
)

func TestOpenInMemory(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer db.Close()

	// Verify database is accessible
	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
		t.Fatalf("failed to query database: %v", err)
	}
	if result != 1 {
		t.Errorf("expected 1, got %d", result)
	}
}

func TestMigration(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Check schema version
	version, err := db.GetSchemaVersion()
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("expected schema version %d, got %d", schemaVersion, version)
	}

	// Verify tables exist
	tables := []string{"products", "card_transactions", "card_linked_account_transactions", "account_transactions", "schema_version"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s does not exist: %v", table, err)
		}
	}

	// Verify indexes exist
	indexes := []string{"idx_card_txn_product_date", "idx_card_linked_txn_product_date", "idx_acct_txn_product_date"}
	for _, index := range indexes {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", index).Scan(&name)
		if err != nil {
			t.Errorf("index %s does not exist: %v", index, err)
		}
	}
}

func TestMigrationIdempotent(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Run migration again - should not fail
	if err := db.Migrate(); err != nil {
		t.Fatalf("idempotent migration failed: %v", err)
	}

	version, err := db.GetSchemaVersion()
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("expected schema version %d after re-migration, got %d", schemaVersion, version)
	}
}

func TestWithTransaction_Success(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert a product within a transaction
	err = db.WithTransaction(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO products (id, product_type, name, synced_at)
			VALUES ('test-id', 'CARD', 'Test Card', 1234567890)
		`)
		return err
	})
	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	// Verify the insert succeeded
	var name string
	err = db.QueryRow("SELECT name FROM products WHERE id = 'test-id'").Scan(&name)
	if err != nil {
		t.Fatalf("failed to query inserted row: %v", err)
	}
	if name != "Test Card" {
		t.Errorf("expected 'Test Card', got %q", name)
	}
}

func TestWithTransaction_Rollback(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert then intentionally fail
	err = db.WithTransaction(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO products (id, product_type, name, synced_at)
			VALUES ('rollback-test', 'ACCOUNT', 'Rollback Test', 1234567890)
		`)
		if err != nil {
			return err
		}
		// Force an error to trigger rollback
		return sql.ErrNoRows
	})
	if err != sql.ErrNoRows {
		t.Fatalf("expected ErrNoRows, got: %v", err)
	}

	// Verify the row was NOT inserted (rolled back)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM products WHERE id = 'rollback-test'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows (rolled back), got %d", count)
	}
}

func TestProductsTableSchema(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert a full product record to verify schema
	_, err = db.Exec(`
		INSERT INTO products (
			id, product_type, name, card_number, account_number,
			account_id, currency, balance, status, synced_at
		) VALUES (
			'card-123', 'CARD', 'My Card', '1234****5678', NULL,
			'acct-456', 'AMD', 100000.50, 'ACTIVE', 1234567890
		)
	`)
	if err != nil {
		t.Fatalf("failed to insert product: %v", err)
	}

	// Query and verify
	var id, productType, name, cardNumber, accountID, currency, status string
	var balance float64
	var accountNumber sql.NullString
	var syncedAt int64

	err = db.QueryRow(`
		SELECT id, product_type, name, card_number, account_number,
			   account_id, currency, balance, status, synced_at
		FROM products WHERE id = 'card-123'
	`).Scan(&id, &productType, &name, &cardNumber, &accountNumber, &accountID, &currency, &balance, &status, &syncedAt)
	if err != nil {
		t.Fatalf("failed to query product: %v", err)
	}

	if id != "card-123" || productType != "CARD" || name != "My Card" {
		t.Errorf("unexpected product values: id=%s, type=%s, name=%s", id, productType, name)
	}
	if balance != 100000.50 {
		t.Errorf("expected balance 100000.50, got %f", balance)
	}
}

func TestCardTransactionsTableSchema(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert a card transaction to verify schema
	_, err = db.Exec(`
		INSERT INTO card_transactions (
			id, product_id, transaction_type, accounting_type, state,
			amount_currency, amount_value, correspondent_account_number,
			correspondent_account_name, details, operation_date,
			workflow_code, date, year, month, synced_at
		) VALUES (
			'txn-001', 'card-123', 'PURCHASE', 'DEBIT', 'SETTLED',
			'AMD', 5000.00, '1234567890',
			'ACME Store', 'Purchase at ACME', '2024-01-15T10:30:00',
			'WF001', '15.01.2024', '2024', '01', 1234567890
		)
	`)
	if err != nil {
		t.Fatalf("failed to insert card transaction: %v", err)
	}

	// Verify it's queryable by product and date
	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM card_transactions
		WHERE product_id = 'card-123' AND operation_date LIKE '2024%'
	`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 transaction, got %d", count)
	}
}

func TestAccountTransactionsTableSchema(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert an account transaction to verify schema
	_, err = db.Exec(`
		INSERT INTO account_transactions (
			id, product_id, transaction_id, operation_id, status,
			transaction_type, workflow_code, flow_direction,
			transaction_date, settled_date, date, month, year,
			debit_account_number, credit_account_number,
			beneficiary_name, details, source_system,
			transaction_amount_currency, transaction_amount_value,
			settled_amount_currency, settled_amount_value,
			domestic_amount_currency, domestic_amount_value,
			synced_at
		) VALUES (
			'atxn-001', 'acct-456', 'TXN123', 'OP456', 'COMPLETED',
			'TRANSFER', 'WF002', 'OUT',
			1705315200000, 1705315200000, '15.01.2024', '01', '2024',
			'DE123456', 'AT789012',
			'John Doe', 'Payment for services', 'CORE',
			'EUR', 100.00,
			'EUR', 100.00,
			'AMD', 42000.00,
			1234567890
		)
	`)
	if err != nil {
		t.Fatalf("failed to insert account transaction: %v", err)
	}

	// Verify it's queryable
	var details string
	var value float64
	err = db.QueryRow(`
		SELECT details, transaction_amount_value FROM account_transactions
		WHERE id = 'atxn-001'
	`).Scan(&details, &value)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if details != "Payment for services" || value != 100.00 {
		t.Errorf("unexpected values: details=%s, value=%f", details, value)
	}
}

func TestUpsertProducts(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	products := []client.ProductInfo{
		{
			ID:          "card-001",
			ProductType: "CARD",
			Name:        "My Card",
			CardNumber:  "1234****5678",
			AccountID:   "acct-001",
			Currency:    "AMD",
			Balance:     50000.0,
			Status:      "ACTIVE",
		},
		{
			ID:            "acct-001",
			ProductType:   "ACCOUNT",
			Name:          "Current Account",
			AccountNumber: "1500123456789",
			Currency:      "AMD",
			Balance:       100000.0,
			Status:        "ACTIVE",
		},
	}

	if err := db.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// Verify products were inserted
	result, err := db.GetProducts()
	if err != nil {
		t.Fatalf("failed to get products: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 products, got %d", len(result))
	}

	// Update balance and upsert again
	products[0].Balance = 75000.0
	if err := db.UpsertProducts(products[:1]); err != nil {
		t.Fatalf("failed to upsert updated product: %v", err)
	}

	// Verify update
	p, err := db.GetProductByID("card-001")
	if err != nil {
		t.Fatalf("failed to get product by ID: %v", err)
	}
	if p.Balance != 75000.0 {
		t.Errorf("expected balance 75000, got %f", p.Balance)
	}
}

func TestInsertCardTransactions_Deduplication(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	txns := []client.Transaction{
		{
			ID:              "txn-001",
			TransactionType: "PURCHASE",
			AccountingType:  "DEBIT",
			State:           "SETTLED",
			Amount:          client.Amount{Currency: "AMD", Amount: 5000},
			Details:         "Coffee Shop",
			OperationDate:   "2024-01-15T10:00:00",
		},
		{
			ID:              "txn-002",
			TransactionType: "ATM",
			AccountingType:  "DEBIT",
			State:           "SETTLED",
			Amount:          client.Amount{Currency: "AMD", Amount: 10000},
			Details:         "Cash withdrawal",
			OperationDate:   "2024-01-16T11:00:00",
		},
	}

	// First insert
	inserted, err := db.InsertCardTransactions("card-001", txns)
	if err != nil {
		t.Fatalf("failed to insert transactions: %v", err)
	}
	if inserted != 2 {
		t.Errorf("expected 2 inserted, got %d", inserted)
	}

	// Insert same transactions again - should be ignored
	inserted, err = db.InsertCardTransactions("card-001", txns)
	if err != nil {
		t.Fatalf("failed to insert duplicate transactions: %v", err)
	}
	if inserted != 0 {
		t.Errorf("expected 0 inserted (duplicates), got %d", inserted)
	}

	// Insert mix of new and existing
	txns = append(txns, client.Transaction{
		ID:              "txn-003",
		TransactionType: "PURCHASE",
		AccountingType:  "DEBIT",
		State:           "SETTLED",
		Amount:          client.Amount{Currency: "AMD", Amount: 2000},
		Details:         "Grocery Store",
		OperationDate:   "2024-01-17T12:00:00",
	})
	inserted, err = db.InsertCardTransactions("card-001", txns)
	if err != nil {
		t.Fatalf("failed to insert mixed transactions: %v", err)
	}
	if inserted != 1 {
		t.Errorf("expected 1 inserted (new only), got %d", inserted)
	}

	// Verify total count
	count, err := db.CountCardTransactions("card-001")
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 total transactions, got %d", count)
	}
}

func TestCardTransactionsCompositeKey(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert same ID with different operation dates (should both be stored)
	txns := []client.Transaction{
		{
			ID:              "txn-001",
			TransactionType: "PURCHASE",
			State:           "PARTIALLY",
			Amount:          client.Amount{Currency: "AMD", Amount: 5000},
			Details:         "Coffee Shop",
			OperationDate:   "2024-01-15T10:00:00",
		},
		{
			ID:              "txn-001",
			TransactionType: "PURCHASE",
			State:           "SETTLED",
			Amount:          client.Amount{Currency: "AMD", Amount: 5000},
			Details:         "Coffee Shop",
			OperationDate:   "2024-01-15T10:30:00",
		},
	}

	inserted, err := db.InsertCardTransactions("card-001", txns)
	if err != nil {
		t.Fatalf("failed to insert transactions: %v", err)
	}
	if inserted != 2 {
		t.Errorf("expected 2 inserted (same ID, different dates), got %d", inserted)
	}

	// Verify both are stored
	count, err := db.CountCardTransactions("card-001")
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 transactions, got %d", count)
	}

	// Verify keys contain both entries
	keys, err := db.GetExistingCardTxnKeys("card-001")
	if err != nil {
		t.Fatalf("failed to get keys: %v", err)
	}
	key1 := TxnKey("txn-001", "2024-01-15T10:00:00")
	key2 := TxnKey("txn-001", "2024-01-15T10:30:00")
	if !keys[key1] || !keys[key2] {
		t.Errorf("expected both composite keys to exist")
	}
}

func TestGetExistingCardTxnKeys(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	txns := []client.Transaction{
		{ID: "txn-001", OperationDate: "2024-01-15T10:00:00", Details: "Test 1"},
		{ID: "txn-002", OperationDate: "2024-01-16T11:00:00", Details: "Test 2"},
	}
	if _, err := db.InsertCardTransactions("card-001", txns); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	keys, err := db.GetExistingCardTxnKeys("card-001")
	if err != nil {
		t.Fatalf("failed to get existing keys: %v", err)
	}

	key1 := TxnKey("txn-001", "2024-01-15T10:00:00")
	key2 := TxnKey("txn-002", "2024-01-16T11:00:00")
	key3 := TxnKey("txn-003", "2024-01-17T12:00:00")

	if !keys[key1] || !keys[key2] {
		t.Errorf("expected both keys to exist in map")
	}
	if keys[key3] {
		t.Errorf("txn-003 key should not exist")
	}
}

func TestInsertAccountTransactions_Deduplication(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	txns := []client.AccountTransaction{
		{
			ID:              "atxn-001",
			TransactionID:   "TXN001",
			TransactionType: "TRANSFER",
			FlowDirection:   "OUT",
			TransactionDate: 1705315200000,
			TransactionAmount: client.TransactionAmt{
				Currency: "AMD",
				Value:    50000,
			},
		},
		{
			ID:              "atxn-002",
			TransactionID:   "TXN002",
			TransactionType: "SALARY",
			FlowDirection:   "IN",
			TransactionDate: 1705401600000,
			TransactionAmount: client.TransactionAmt{
				Currency: "AMD",
				Value:    500000,
			},
		},
	}

	// First insert
	inserted, err := db.InsertAccountTransactions("acct-001", txns)
	if err != nil {
		t.Fatalf("failed to insert transactions: %v", err)
	}
	if inserted != 2 {
		t.Errorf("expected 2 inserted, got %d", inserted)
	}

	// Insert duplicates
	inserted, err = db.InsertAccountTransactions("acct-001", txns)
	if err != nil {
		t.Fatalf("failed to insert duplicate transactions: %v", err)
	}
	if inserted != 0 {
		t.Errorf("expected 0 inserted (duplicates), got %d", inserted)
	}

	// Verify total count
	count, err := db.CountAccountTransactions("acct-001")
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 total transactions, got %d", count)
	}
}

func TestGetCardTransactions(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	txns := []client.Transaction{
		{
			ID:              "txn-001",
			TransactionType: "PURCHASE",
			Amount:          client.Amount{Currency: "AMD", Amount: 5000},
			Details:         "Coffee Shop",
			OperationDate:   "2024-01-15T10:00:00",
		},
		{
			ID:              "txn-002",
			TransactionType: "ATM",
			Amount:          client.Amount{Currency: "AMD", Amount: 10000},
			Details:         "Cash withdrawal",
			OperationDate:   "2024-01-16T11:00:00",
		},
		{
			ID:              "txn-003",
			TransactionType: "PURCHASE",
			Amount:          client.Amount{Currency: "AMD", Amount: 2000},
			Details:         "Grocery",
			OperationDate:   "2024-01-17T12:00:00",
		},
	}
	if _, err := db.InsertCardTransactions("card-001", txns); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	// Test size=0 (no limit), descending (default)
	result, err := db.GetCardTransactions("card-001", 0, 0, false)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 transactions with size=0, got %d", len(result))
	}
	if result[0].ID != "txn-003" {
		t.Errorf("expected newest first (desc), got %s", result[0].ID)
	}

	// Test ascending order
	result, err = db.GetCardTransactions("card-001", 0, 0, true)
	if err != nil {
		t.Fatalf("failed to get transactions ascending: %v", err)
	}
	if result[0].ID != "txn-001" {
		t.Errorf("expected oldest first (asc), got %s", result[0].ID)
	}

	// Test pagination: size=2, page=0
	result, err = db.GetCardTransactions("card-001", 2, 0, false)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 transactions with size=2, got %d", len(result))
	}

	// Test pagination: size=2, page=1
	result, err = db.GetCardTransactions("card-001", 2, 1, false)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 transaction on page 1, got %d", len(result))
	}
	if result[0].ID != "txn-001" {
		t.Errorf("expected oldest txn on page 1, got %s", result[0].ID)
	}
}

func TestGetAccountTransactions(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	txns := []client.AccountTransaction{
		{
			ID:              "atxn-001",
			TransactionType: "TRANSFER",
			TransactionDate: 1705315200000,
			TransactionAmount: client.TransactionAmt{
				Currency: "EUR",
				Value:    100,
			},
			SettledAmount: client.TransactionAmt{
				Currency: "AMD",
				Value:    42000,
			},
		},
	}
	if _, err := db.InsertAccountTransactions("acct-001", txns); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	result, err := db.GetAccountTransactions("acct-001", false)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(result))
	}
	if result[0].ID != "atxn-001" {
		t.Errorf("expected ID atxn-001, got %s", result[0].ID)
	}
	if result[0].TransactionAmount.Value != 100 {
		t.Errorf("expected transaction amount 100, got %f", result[0].TransactionAmount.Value)
	}
	if result[0].SettledAmount.Value != 42000 {
		t.Errorf("expected settled amount 42000, got %f", result[0].SettledAmount.Value)
	}
}

func TestGetAccountTransactions_Ascending(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	txns := []client.AccountTransaction{
		{
			ID:              "atxn-001",
			TransactionType: "TRANSFER",
			TransactionDate: 1705315200000, // Earlier
		},
		{
			ID:              "atxn-002",
			TransactionType: "TRANSFER",
			TransactionDate: 1705401600000, // Later
		},
	}
	if _, err := db.InsertAccountTransactions("acct-001", txns); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	// Test descending (default)
	result, err := db.GetAccountTransactions("acct-001", false)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if result[0].ID != "atxn-002" {
		t.Errorf("expected newest first (desc), got %s", result[0].ID)
	}

	// Test ascending
	result, err = db.GetAccountTransactions("acct-001", true)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if result[0].ID != "atxn-001" {
		t.Errorf("expected oldest first (asc), got %s", result[0].ID)
	}
}

func TestInsertLinkedAccountTransactions_Deduplication(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	txns := []client.Transaction{
		{
			ID:              "lat-001",
			TransactionType: "PURCHASE",
			AccountingType:  "DEBIT",
			State:           "SETTLED",
			Amount:          client.Amount{Currency: "AMD", Amount: 5000},
			Details:         "Coffee Shop",
			OperationDate:   "2024-01-15T10:00:00",
		},
		{
			ID:              "lat-002",
			TransactionType: "ATM",
			AccountingType:  "DEBIT",
			State:           "SETTLED",
			Amount:          client.Amount{Currency: "AMD", Amount: 10000},
			Details:         "Cash withdrawal",
			OperationDate:   "2024-01-16T11:00:00",
		},
	}

	// First insert
	inserted, err := db.InsertLinkedAccountTransactions("card-001", txns)
	if err != nil {
		t.Fatalf("failed to insert transactions: %v", err)
	}
	if inserted != 2 {
		t.Errorf("expected 2 inserted, got %d", inserted)
	}

	// Insert same transactions again - should be ignored
	inserted, err = db.InsertLinkedAccountTransactions("card-001", txns)
	if err != nil {
		t.Fatalf("failed to insert duplicate transactions: %v", err)
	}
	if inserted != 0 {
		t.Errorf("expected 0 inserted (duplicates), got %d", inserted)
	}

	// Verify total count
	count, err := db.CountLinkedAccountTransactions("card-001")
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 total transactions, got %d", count)
	}
}

func TestGetLinkedAccountTransactions(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	txns := []client.Transaction{
		{
			ID:              "lat-001",
			TransactionType: "PURCHASE",
			Amount:          client.Amount{Currency: "AMD", Amount: 5000},
			Details:         "Coffee Shop",
			OperationDate:   "2024-01-15T10:00:00",
		},
		{
			ID:              "lat-002",
			TransactionType: "ATM",
			Amount:          client.Amount{Currency: "AMD", Amount: 10000},
			Details:         "Cash withdrawal",
			OperationDate:   "2024-01-16T11:00:00",
		},
		{
			ID:              "lat-003",
			TransactionType: "PURCHASE",
			Amount:          client.Amount{Currency: "AMD", Amount: 2000},
			Details:         "Grocery",
			OperationDate:   "2024-01-17T12:00:00",
		},
	}
	if _, err := db.InsertLinkedAccountTransactions("card-001", txns); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	// Test size=0 (no limit), descending (default)
	result, err := db.GetLinkedAccountTransactions("card-001", 0, 0, false, false)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 transactions with size=0, got %d", len(result))
	}
	// Descending: newest first
	if result[0].ID != "lat-003" {
		t.Errorf("expected newest first (desc), got %s", result[0].ID)
	}

	// Test pagination: size=2, page=0
	result, err = db.GetLinkedAccountTransactions("card-001", 2, 0, false, false)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 transactions with size=2, got %d", len(result))
	}

	// Test pagination: size=2, page=1
	result, err = db.GetLinkedAccountTransactions("card-001", 2, 1, false, false)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 transaction on page 1, got %d", len(result))
	}
	if result[0].ID != "lat-001" {
		t.Errorf("expected oldest txn on page 1, got %s", result[0].ID)
	}

	// Test ascending order
	result, err = db.GetLinkedAccountTransactions("card-001", 0, 0, false, true)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if result[0].ID != "lat-001" {
		t.Errorf("expected oldest first (asc), got %s", result[0].ID)
	}
	if result[2].ID != "lat-003" {
		t.Errorf("expected newest last (asc), got %s", result[2].ID)
	}
}

func TestGetExistingLinkedAccountTxnKeys(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	txns := []client.Transaction{
		{ID: "lat-001", OperationDate: "2024-01-15T10:00:00", Details: "Test 1"},
		{ID: "lat-002", OperationDate: "2024-01-16T11:00:00", Details: "Test 2"},
	}
	if _, err := db.InsertLinkedAccountTransactions("card-001", txns); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	keys, err := db.GetExistingLinkedAccountTxnKeys("card-001")
	if err != nil {
		t.Fatalf("failed to get existing keys: %v", err)
	}

	key1 := TxnKey("lat-001", "2024-01-15T10:00:00")
	key2 := TxnKey("lat-002", "2024-01-16T11:00:00")
	key3 := TxnKey("lat-003", "2024-01-17T12:00:00")

	if !keys[key1] || !keys[key2] {
		t.Errorf("expected both keys to exist in map")
	}
	if keys[key3] {
		t.Errorf("lat-003 key should not exist")
	}
}

func TestGetProductByNameOrID_ByID(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	products := []client.ProductInfo{
		{
			ID:          "card-001",
			ProductType: "CARD",
			Name:        "My Card",
			Currency:    "AMD",
			Balance:     50000.0,
			Status:      "ACTIVE",
		},
	}
	if err := db.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// Lookup by ID should work
	p, err := db.GetProductByNameOrID("card-001")
	if err != nil {
		t.Fatalf("failed to get product by ID: %v", err)
	}
	if p == nil {
		t.Fatal("expected product, got nil")
	}
	if p.ID != "card-001" {
		t.Errorf("expected ID card-001, got %s", p.ID)
	}
	if p.Name != "My Card" {
		t.Errorf("expected name 'My Card', got %s", p.Name)
	}
}

func TestGetProductByNameOrID_ByName(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	products := []client.ProductInfo{
		{
			ID:          "card-001",
			ProductType: "CARD",
			Name:        "VISA_CLASSIC",
			Currency:    "AMD",
			Balance:     50000.0,
			Status:      "ACTIVE",
		},
	}
	if err := db.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// Lookup by exact name should work
	p, err := db.GetProductByNameOrID("VISA_CLASSIC")
	if err != nil {
		t.Fatalf("failed to get product by name: %v", err)
	}
	if p == nil {
		t.Fatal("expected product, got nil")
	}
	if p.ID != "card-001" {
		t.Errorf("expected ID card-001, got %s", p.ID)
	}
}

func TestGetProductByNameOrID_ByNameCaseInsensitive(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	products := []client.ProductInfo{
		{
			ID:          "card-001",
			ProductType: "CARD",
			Name:        "VISA_CLASSIC",
			Currency:    "AMD",
			Balance:     50000.0,
			Status:      "ACTIVE",
		},
	}
	if err := db.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// Lookup by name with different case should work
	p, err := db.GetProductByNameOrID("visa_classic")
	if err != nil {
		t.Fatalf("failed to get product by name (lowercase): %v", err)
	}
	if p == nil {
		t.Fatal("expected product, got nil")
	}
	if p.ID != "card-001" {
		t.Errorf("expected ID card-001, got %s", p.ID)
	}

	// Mixed case should also work
	p, err = db.GetProductByNameOrID("Visa_Classic")
	if err != nil {
		t.Fatalf("failed to get product by name (mixed case): %v", err)
	}
	if p == nil {
		t.Fatal("expected product, got nil")
	}
}

func TestGetProductByNameOrID_NotFound(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	products := []client.ProductInfo{
		{
			ID:          "card-001",
			ProductType: "CARD",
			Name:        "My Card",
			Currency:    "AMD",
			Balance:     50000.0,
			Status:      "ACTIVE",
		},
	}
	if err := db.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// Non-existent ID/name should return nil, not error
	p, err := db.GetProductByNameOrID("nonexistent")
	if err != nil {
		t.Fatalf("expected nil error for not found, got: %v", err)
	}
	if p != nil {
		t.Errorf("expected nil product for not found, got: %+v", p)
	}
}

func TestGetProductByNameOrID_AmbiguousName(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Two products with the same name
	products := []client.ProductInfo{
		{
			ID:          "card-001",
			ProductType: "CARD",
			Name:        "CURRENT",
			Currency:    "AMD",
			Balance:     50000.0,
			Status:      "ACTIVE",
		},
		{
			ID:          "card-002",
			ProductType: "CARD",
			Name:        "CURRENT",
			Currency:    "USD",
			Balance:     1000.0,
			Status:      "ACTIVE",
		},
	}
	if err := db.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// Lookup by ambiguous name should return error
	p, err := db.GetProductByNameOrID("CURRENT")
	if err == nil {
		t.Fatal("expected error for ambiguous name, got nil")
	}
	if p != nil {
		t.Errorf("expected nil product for ambiguous name, got: %+v", p)
	}
	// Error message should mention "ambiguous"
	if !contains(err.Error(), "ambiguous") {
		t.Errorf("expected error to contain 'ambiguous', got: %v", err)
	}
}

func TestGetProductByNameOrID_IDTakesPriority(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Product with ID that matches another product's name
	products := []client.ProductInfo{
		{
			ID:          "SAVINGS",
			ProductType: "ACCOUNT",
			Name:        "Savings Account",
			Currency:    "AMD",
			Balance:     100000.0,
			Status:      "ACTIVE",
		},
		{
			ID:          "card-001",
			ProductType: "CARD",
			Name:        "SAVINGS", // Name matches ID of first product
			Currency:    "AMD",
			Balance:     50000.0,
			Status:      "ACTIVE",
		},
	}
	if err := db.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// Lookup "SAVINGS" should find by ID first (the account), not by name (the card)
	p, err := db.GetProductByNameOrID("SAVINGS")
	if err != nil {
		t.Fatalf("failed to get product: %v", err)
	}
	if p == nil {
		t.Fatal("expected product, got nil")
	}
	if p.ID != "SAVINGS" {
		t.Errorf("expected ID 'SAVINGS' (by ID match), got %s", p.ID)
	}
	if p.ProductType != "ACCOUNT" {
		t.Errorf("expected ACCOUNT (found by ID), got %s", p.ProductType)
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
