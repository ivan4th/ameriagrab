package cmd

import (
	"testing"

	"github.com/ivan4th/ameriagrab/client"
	"github.com/ivan4th/ameriagrab/db"
)

// mockCardClient implements the interface used by syncCard
type mockCardClient struct {
	transactions    *client.TransactionsResponse
	transactionsErr error
	eventsPast      map[int]*client.TransactionsResponse // page -> response
	eventsPastErr   error
	details         map[string]*client.TransactionDetailsResponse // txnID -> response
	detailsErr      error
}

func (m *mockCardClient) GetTransactions(accessToken, cardID string) (*client.TransactionsResponse, error) {
	return m.transactions, m.transactionsErr
}

func (m *mockCardClient) GetEventsPast(accessToken, accountID string, size, page int) (*client.TransactionsResponse, error) {
	if m.eventsPastErr != nil {
		return nil, m.eventsPastErr
	}
	if resp, ok := m.eventsPast[page]; ok {
		return resp, nil
	}
	return &client.TransactionsResponse{Status: "success"}, nil
}

func (m *mockCardClient) GetTransactionDetails(accessToken, transactionID string) (*client.TransactionDetailsResponse, error) {
	if m.detailsErr != nil {
		return nil, m.detailsErr
	}
	if resp, ok := m.details[transactionID]; ok {
		return resp, nil
	}
	return &client.TransactionDetailsResponse{Status: "success"}, nil
}

// mockAccountClient implements the interface used by syncAccount
type mockAccountClient struct {
	history    map[int]*client.HistoryResponse // page -> response
	historyErr error
}

func (m *mockAccountClient) GetAccountHistory(accessToken, accountID string, size, page int) (*client.HistoryResponse, error) {
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	if resp, ok := m.history[page]; ok {
		return resp, nil
	}
	return &client.HistoryResponse{Status: "success"}, nil
}

func makeTransactionsResponse(entries ...client.Transaction) *client.TransactionsResponse {
	resp := &client.TransactionsResponse{Status: "success"}
	resp.Data.TotalCount = len(entries)
	resp.Data.Entries = entries
	return resp
}

func makeHistoryResponse(hasNext bool, txns ...client.AccountTransaction) *client.HistoryResponse {
	resp := &client.HistoryResponse{Status: "success"}
	resp.Data.HasNext = hasNext
	resp.Data.IsUpToDate = true
	resp.Data.Transactions = txns
	return resp
}

func TestSyncCard_NewTransactions(t *testing.T) {
	database, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Insert a product first
	products := []client.ProductInfo{{
		ID:          "card1",
		ProductType: "CARD",
		Name:        "Test Card",
		AccountID:   "acc1",
	}}
	if err := database.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	mockClient := &mockCardClient{
		transactions: makeTransactionsResponse(
			client.Transaction{ID: "txn1", OperationDate: "2024-01-01T10:00:00Z", Details: "Purchase 1"},
			client.Transaction{ID: "txn2", OperationDate: "2024-01-02T10:00:00Z", Details: "Purchase 2"},
		),
		eventsPast: map[int]*client.TransactionsResponse{
			0: makeTransactionsResponse(), // Empty page
		},
	}

	// Run sync
	syncVerbose = false
	if err := syncCard(database, mockClient, "token", "card1", "acc1", "Test Card"); err != nil {
		t.Fatalf("syncCard failed: %v", err)
	}

	// Verify transactions were inserted
	count, err := database.CountCardTransactions("card1")
	if err != nil {
		t.Fatalf("CountCardTransactions failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 card transactions, got %d", count)
	}
}

func TestSyncCard_Deduplication(t *testing.T) {
	database, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Insert product
	products := []client.ProductInfo{{
		ID:          "card1",
		ProductType: "CARD",
		Name:        "Test Card",
		AccountID:   "acc1",
	}}
	if err := database.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	mockClient := &mockCardClient{
		transactions: makeTransactionsResponse(
			client.Transaction{ID: "txn1", OperationDate: "2024-01-01T10:00:00Z", Details: "Purchase 1"},
			client.Transaction{ID: "txn2", OperationDate: "2024-01-02T10:00:00Z", Details: "Purchase 2"},
		),
		eventsPast: map[int]*client.TransactionsResponse{
			0: makeTransactionsResponse(),
		},
	}

	// First sync
	syncVerbose = false
	if err := syncCard(database, mockClient, "token", "card1", "acc1", "Test Card"); err != nil {
		t.Fatalf("first syncCard failed: %v", err)
	}

	// Second sync with same transactions (should not duplicate)
	if err := syncCard(database, mockClient, "token", "card1", "acc1", "Test Card"); err != nil {
		t.Fatalf("second syncCard failed: %v", err)
	}

	// Should still have only 2 transactions
	count, err := database.CountCardTransactions("card1")
	if err != nil {
		t.Fatalf("CountCardTransactions failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 card transactions after dedup, got %d", count)
	}
}

func TestSyncCard_IncrementalSync(t *testing.T) {
	database, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Insert product
	products := []client.ProductInfo{{
		ID:          "card1",
		ProductType: "CARD",
		Name:        "Test Card",
		AccountID:   "acc1",
	}}
	if err := database.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// First sync with 2 transactions
	mockClient := &mockCardClient{
		transactions: makeTransactionsResponse(
			client.Transaction{ID: "txn1", OperationDate: "2024-01-01T10:00:00Z", Details: "Purchase 1"},
			client.Transaction{ID: "txn2", OperationDate: "2024-01-02T10:00:00Z", Details: "Purchase 2"},
		),
		eventsPast: map[int]*client.TransactionsResponse{
			0: makeTransactionsResponse(),
		},
	}

	syncVerbose = false
	if err := syncCard(database, mockClient, "token", "card1", "acc1", "Test Card"); err != nil {
		t.Fatalf("first syncCard failed: %v", err)
	}

	// Second sync with 3 transactions (1 new)
	mockClient.transactions = makeTransactionsResponse(
		client.Transaction{ID: "txn1", OperationDate: "2024-01-01T10:00:00Z", Details: "Purchase 1"},
		client.Transaction{ID: "txn2", OperationDate: "2024-01-02T10:00:00Z", Details: "Purchase 2"},
		client.Transaction{ID: "txn3", OperationDate: "2024-01-03T10:00:00Z", Details: "Purchase 3"},
	)

	if err := syncCard(database, mockClient, "token", "card1", "acc1", "Test Card"); err != nil {
		t.Fatalf("second syncCard failed: %v", err)
	}

	// Should now have 3 transactions
	count, err := database.CountCardTransactions("card1")
	if err != nil {
		t.Fatalf("CountCardTransactions failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 card transactions after incremental sync, got %d", count)
	}
}

func TestSyncAccount_NewTransactions(t *testing.T) {
	database, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Insert product
	products := []client.ProductInfo{{
		ID:          "acc1",
		ProductType: "ACCOUNT",
		Name:        "Test Account",
	}}
	if err := database.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	mockClient := &mockAccountClient{
		history: map[int]*client.HistoryResponse{
			0: makeHistoryResponse(false,
				client.AccountTransaction{ID: "txn1", Details: "Transfer 1", TransactionDate: 1704067200000},
				client.AccountTransaction{ID: "txn2", Details: "Transfer 2", TransactionDate: 1704153600000},
			),
		},
	}

	syncVerbose = false
	if err := syncAccount(database, mockClient, "token", "acc1", "Test Account"); err != nil {
		t.Fatalf("syncAccount failed: %v", err)
	}

	// Verify transactions were inserted
	txns, err := database.GetAccountTransactions("acc1", false)
	if err != nil {
		t.Fatalf("GetAccountTransactions failed: %v", err)
	}
	if len(txns) != 2 {
		t.Errorf("expected 2 account transactions, got %d", len(txns))
	}
}

func TestSyncAccount_Pagination(t *testing.T) {
	database, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Insert product
	products := []client.ProductInfo{{
		ID:          "acc1",
		ProductType: "ACCOUNT",
		Name:        "Test Account",
	}}
	if err := database.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// Mock multiple pages
	mockClient := &mockAccountClient{
		history: map[int]*client.HistoryResponse{
			0: makeHistoryResponse(true,
				client.AccountTransaction{ID: "txn1", Details: "Transfer 1"},
			),
			1: makeHistoryResponse(false,
				client.AccountTransaction{ID: "txn2", Details: "Transfer 2"},
			),
		},
	}

	syncVerbose = false
	if err := syncAccount(database, mockClient, "token", "acc1", "Test Account"); err != nil {
		t.Fatalf("syncAccount failed: %v", err)
	}

	// Should have transactions from both pages
	txns, err := database.GetAccountTransactions("acc1", false)
	if err != nil {
		t.Fatalf("GetAccountTransactions failed: %v", err)
	}
	if len(txns) != 2 {
		t.Errorf("expected 2 account transactions from pagination, got %d", len(txns))
	}
}

func TestSyncAccount_Deduplication(t *testing.T) {
	database, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Insert product
	products := []client.ProductInfo{{
		ID:          "acc1",
		ProductType: "ACCOUNT",
		Name:        "Test Account",
	}}
	if err := database.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	mockClient := &mockAccountClient{
		history: map[int]*client.HistoryResponse{
			0: makeHistoryResponse(false,
				client.AccountTransaction{ID: "txn1", Details: "Transfer 1"},
			),
		},
	}

	syncVerbose = false

	// First sync
	if err := syncAccount(database, mockClient, "token", "acc1", "Test Account"); err != nil {
		t.Fatalf("first syncAccount failed: %v", err)
	}

	// Second sync (same data)
	if err := syncAccount(database, mockClient, "token", "acc1", "Test Account"); err != nil {
		t.Fatalf("second syncAccount failed: %v", err)
	}

	// Should still have 1 transaction
	txns, err := database.GetAccountTransactions("acc1", false)
	if err != nil {
		t.Fatalf("GetAccountTransactions failed: %v", err)
	}
	if len(txns) != 1 {
		t.Errorf("expected 1 account transaction after dedup, got %d", len(txns))
	}
}

func TestSyncLinkedAccountTransactions_WithExtendedInfo(t *testing.T) {
	database, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Insert product
	products := []client.ProductInfo{{
		ID:          "card1",
		ProductType: "CARD",
		Name:        "Test Card",
		AccountID:   "acc1",
	}}
	if err := database.UpsertProducts(products); err != nil {
		t.Fatalf("failed to upsert products: %v", err)
	}

	// Build transaction details response
	detailsResp := &client.TransactionDetailsResponse{Status: "success"}
	detailsResp.Data.Transaction.BeneficiaryName = "John Doe"
	detailsResp.Data.Transaction.CreditAccountNumber = "1234567890"

	mockClient := &mockCardClient{
		eventsPast: map[int]*client.TransactionsResponse{
			0: makeTransactionsResponse(
				client.Transaction{ID: "txn1", OperationDate: "2024-01-01T10:00:00Z", Details: "Transfer to card"},
			),
		},
		details: map[string]*client.TransactionDetailsResponse{
			"txn1": detailsResp,
		},
	}

	syncVerbose = false
	if err := syncCardAccountTransactions(database, mockClient, "token", "card1", "acc1", "Test Card"); err != nil {
		t.Fatalf("syncCardAccountTransactions failed: %v", err)
	}

	// Verify transaction was inserted
	count, err := database.CountLinkedAccountTransactions("card1")
	if err != nil {
		t.Fatalf("CountLinkedAccountTransactions failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 linked account transaction, got %d", count)
	}

	// Verify extended info was stored
	txns, err := database.GetLinkedAccountTransactions("card1", 0, 0, true, false)
	if err != nil {
		t.Fatalf("GetLinkedAccountTransactions failed: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	if txns[0].Extended == nil {
		t.Error("expected extended info to be populated")
	} else if txns[0].Extended.BeneficiaryName != "John Doe" {
		t.Errorf("expected beneficiary 'John Doe', got %q", txns[0].Extended.BeneficiaryName)
	}
}
