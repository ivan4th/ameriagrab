package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockSessionStorage is a simple in-memory session storage for testing
type mockSessionStorage struct {
	session *SessionData
}

func (m *mockSessionStorage) SaveSession(data *SessionData) error {
	m.session = data
	return nil
}

func (m *mockSessionStorage) LoadSession() (*SessionData, error) {
	return m.session, nil
}

func (m *mockSessionStorage) UpdateClientID(clientID string) error {
	if m.session != nil {
		m.session.ClientID = clientID
	}
	return nil
}

func TestNewClient(t *testing.T) {
	storage := &mockSessionStorage{}

	c, err := NewClient("testuser", "testpass", storage, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if c.Username != "testuser" {
		t.Errorf("expected username 'testuser', got '%s'", c.Username)
	}
	if c.Password != "testpass" {
		t.Errorf("expected password 'testpass', got '%s'", c.Password)
	}
	if c.SessionStorage != storage {
		t.Error("expected SessionStorage to be set")
	}
}

func TestNewClient_NoSessionStorage(t *testing.T) {
	c, err := NewClient("testuser", "testpass", nil, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if c.SessionStorage != nil {
		t.Error("expected nil SessionStorage")
	}
}

func TestSaveLoadSession(t *testing.T) {
	storage := &mockSessionStorage{}

	c, err := NewClient("testuser", "testpass", storage, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	c.ClientID = "test-client-123"

	// Save session
	err = c.SaveSession("test-access-token", "test-refresh-token", 3600)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Verify session was stored
	if storage.session == nil {
		t.Fatal("Session was not stored")
	}
	if storage.session.AccessToken != "test-access-token" {
		t.Errorf("expected access token 'test-access-token', got '%s'", storage.session.AccessToken)
	}

	// Create new client and load session
	c2, err := NewClient("testuser", "testpass", storage, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	session, err := c2.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if session == nil {
		t.Fatal("LoadSession returned nil session")
	}

	if session.AccessToken != "test-access-token" {
		t.Errorf("expected access token 'test-access-token', got '%s'", session.AccessToken)
	}
	if session.RefreshToken != "test-refresh-token" {
		t.Errorf("expected refresh token 'test-refresh-token', got '%s'", session.RefreshToken)
	}
	if c2.ClientID != "test-client-123" {
		t.Errorf("expected clientID 'test-client-123', got '%s'", c2.ClientID)
	}
}

func TestSaveSession_Disabled(t *testing.T) {
	c, err := NewClient("testuser", "testpass", nil, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Should not error when session persistence is disabled
	err = c.SaveSession("token", "refresh", 3600)
	if err != nil {
		t.Errorf("SaveSession should not error when disabled: %v", err)
	}
}

func TestLoadSession_Expired(t *testing.T) {
	storage := &mockSessionStorage{
		session: &SessionData{
			AccessToken:  "expired-token",
			RefreshToken: "expired-refresh",
			ExpiresAt:    time.Now().Add(-time.Hour), // Expired
			ClientID:     "test-client",
			Cookies:      []SerializedCookie{},
		},
	}

	c, err := NewClient("testuser", "testpass", storage, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Load should return nil for expired session
	loaded, err := c.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for expired session, got non-nil")
	}
}

func TestLoadSession_AlmostExpired(t *testing.T) {
	storage := &mockSessionStorage{
		session: &SessionData{
			AccessToken:  "almost-expired-token",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(30 * time.Second), // Within 1-minute buffer
			ClientID:     "test-client",
			Cookies:      []SerializedCookie{},
		},
	}

	c, err := NewClient("testuser", "testpass", storage, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Load should return nil (within 1-minute expiration buffer)
	loaded, err := c.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for almost-expired session, got non-nil")
	}
}

func TestLoadSession_NoSession(t *testing.T) {
	storage := &mockSessionStorage{session: nil}

	c, err := NewClient("testuser", "testpass", storage, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// No session exists
	session, err := c.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if session != nil {
		t.Error("expected nil for non-existent session, got non-nil")
	}
}

func TestLoadSession_Disabled(t *testing.T) {
	c, err := NewClient("testuser", "testpass", nil, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	session, err := c.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if session != nil {
		t.Error("expected nil when session persistence disabled")
	}
}

func TestUpdateSessionClientID(t *testing.T) {
	storage := &mockSessionStorage{}

	c, err := NewClient("testuser", "testpass", storage, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Save initial session
	c.ClientID = "initial-client-id"
	err = c.SaveSession("token", "refresh", 3600)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Update client ID
	c.ClientID = "updated-client-id"
	err = c.UpdateSessionClientID()
	if err != nil {
		t.Fatalf("UpdateSessionClientID failed: %v", err)
	}

	// Verify update
	if storage.session.ClientID != "updated-client-id" {
		t.Errorf("expected clientID 'updated-client-id', got '%s'", storage.session.ClientID)
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{0, 0, 0},
		{-1, 1, -1},
		{100, 50, 50},
	}

	for _, tt := range tests {
		result := Min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("Min(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestSaveDebugFile(t *testing.T) {
	tmpDir := t.TempDir()

	c, _ := NewClient("testuser", "testpass", nil, tmpDir)

	c.SaveDebugFile("test.txt", []byte("test content"))

	// Verify file was created
	content, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
	if err != nil {
		t.Fatalf("failed to read debug file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("expected 'test content', got '%s'", string(content))
	}
}

func TestSaveDebugFile_NoDebugDir(t *testing.T) {
	c, _ := NewClient("testuser", "testpass", nil, "")

	// Should not panic or error when debug dir is not set
	c.SaveDebugFile("test.txt", []byte("test content"))
}

func TestBuildCDDCHeader(t *testing.T) {
	header := BuildCDDCHeader()

	if header == "" {
		t.Error("BuildCDDCHeader returned empty string")
	}

	// Verify it's valid base64 by checking it doesn't contain invalid characters
	for _, c := range header {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=') {
			t.Errorf("BuildCDDCHeader contains invalid base64 character: %c", c)
		}
	}
}

func TestBuildTwoFAHeader(t *testing.T) {
	header := BuildTwoFAHeader()

	if header == "" {
		t.Error("BuildTwoFAHeader returned empty string")
	}

	// Verify it's valid base64
	for _, c := range header {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=') {
			t.Errorf("BuildTwoFAHeader contains invalid base64 character: %c", c)
		}
	}
}

func TestGetSessionURLs(t *testing.T) {
	urls := GetSessionURLs()

	if len(urls) != 3 {
		t.Errorf("expected 3 session URLs, got %d", len(urls))
	}

	// Verify URLs are valid
	for _, u := range urls {
		if u == nil {
			t.Error("GetSessionURLs returned nil URL")
		}
		if u.Host == "" {
			t.Error("GetSessionURLs returned URL with empty host")
		}
	}
}

// mockAPIServer creates a test server that simulates Ameriabank API responses
func mockAPIServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/accounts-and-cards":
			json.NewEncoder(w).Encode(AccountsAndCardsResponse{
				Status: "SUCCESS",
				Data: struct {
					AccountsAndCards []ProductInfo `json:"accountsAndCards"`
				}{
					AccountsAndCards: []ProductInfo{
						{
							ProductType: "CARD",
							ID:          "1000000001",
							Name:        "Test Card",
							CardNumber:  "4000********1234",
							AccountID:   "2000000001",
							Currency:    "AMD",
							Balance:     150000.00,
							Status:      "ACTIVE",
						},
						{
							ProductType:   "ACCOUNT",
							ID:            "2000000001",
							Name:          "Test Account",
							AccountNumber: "1570000000000000",
							Currency:      "AMD",
							Balance:       500000.00,
							Status:        "ACTIVE",
						},
					},
				},
			})
		case "/api/events/settled/1000000001":
			json.NewEncoder(w).Encode(TransactionsResponse{
				Status: "SUCCESS",
				Data: struct {
					TotalCount int           `json:"totalCount"`
					Entries    []Transaction `json:"entries"`
				}{
					TotalCount: 2,
					Entries: []Transaction{
						{
							ID:              "txn001",
							TransactionType: "purchase:pos",
							AccountingType:  "DEBIT",
							Amount:          Amount{Currency: "AMD", Amount: 5000.00},
							Details:         "Test Store",
							Date:            "2024-01-15",
						},
						{
							ID:              "txn002",
							TransactionType: "pre-purchase:atm",
							AccountingType:  "CREDIT",
							Amount:          Amount{Currency: "USD", Amount: 100.00},
							Details:         "ATM Deposit",
							Date:            "2024-01-14",
						},
					},
				},
			})
		case "/api/history":
			json.NewEncoder(w).Encode(HistoryResponse{
				Status: "SUCCESS",
				Data: struct {
					HasNext      bool                 `json:"hasNext"`
					IsUpToDate   bool                 `json:"isUpToDate"`
					Transactions []AccountTransaction `json:"transactions"`
				}{
					HasNext:    false,
					IsUpToDate: true,
					Transactions: []AccountTransaction{
						{
							ID:              "hist001",
							TransactionType: "transfer:internal",
							FlowDirection:   "OUTCOME",
							TransactionDate: 1705312200000,
							BeneficiaryName: "Test User",
							Details:         "Internal transfer",
							TransactionAmount: TransactionAmt{
								Currency: "AMD",
								Value:    25000.00,
							},
						},
					},
				},
			})
		case "/api/events/past":
			json.NewEncoder(w).Encode(TransactionsResponse{
				Status: "SUCCESS",
				Data: struct {
					TotalCount int           `json:"totalCount"`
					Entries    []Transaction `json:"entries"`
				}{
					TotalCount: 1,
					Entries: []Transaction{
						{
							ID:              "past001",
							TransactionType: "purchase:online",
							AccountingType:  "DEBIT",
							Amount:          Amount{Currency: "AMD", Amount: 15000.00},
							Details:         "Online Purchase",
							Date:            "2024-01-13",
						},
					},
				},
			})
		case "/api/users/info":
			json.NewEncoder(w).Encode(UserInfoResponse{
				Status: "SUCCESS",
				Data: struct {
					UserInfo struct {
						Sub string `json:"sub"`
						ID  string `json:"id"`
					} `json:"userInfo"`
				}{
					UserInfo: struct {
						Sub string `json:"sub"`
						ID  string `json:"id"`
					}{
						Sub: "test-sub",
						ID:  "test-user-id",
					},
				},
			})
		case "/api/users/test-user-id/clients":
			json.NewEncoder(w).Encode(ClientsResponse{
				Status: "SUCCESS",
				Data: struct {
					Clients []struct {
						ID      string `json:"id"`
						Default bool   `json:"default"`
					} `json:"clients"`
				}{
					Clients: []struct {
						ID      string `json:"id"`
						Default bool   `json:"default"`
					}{
						{ID: "test-client-id", Default: true},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"status":"ERROR","errorMessages":["not found"]}`))
		}
	}))
}

func TestGetAccountsAndCards_WithMockServer(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	c, _ := NewClient("testuser", "testpass", nil, "")
	c.APIBaseURL = server.URL
	c.ClientID = "test-client-id"

	resp, err := c.GetAccountsAndCards("test-token")
	if err != nil {
		t.Fatalf("GetAccountsAndCards failed: %v", err)
	}

	if resp.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", resp.Status)
	}
	if len(resp.Data.AccountsAndCards) != 2 {
		t.Errorf("expected 2 products, got %d", len(resp.Data.AccountsAndCards))
	}
	if resp.Data.AccountsAndCards[0].ProductType != "CARD" {
		t.Errorf("expected first product to be CARD, got %s", resp.Data.AccountsAndCards[0].ProductType)
	}
	if resp.Data.AccountsAndCards[0].ID != "1000000001" {
		t.Errorf("expected card ID 1000000001, got %s", resp.Data.AccountsAndCards[0].ID)
	}
}

func TestGetTransactions_WithMockServer(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	c, _ := NewClient("testuser", "testpass", nil, "")
	c.APIBaseURL = server.URL
	c.ClientID = "test-client-id"

	resp, err := c.GetTransactions("test-token", "1000000001")
	if err != nil {
		t.Fatalf("GetTransactions failed: %v", err)
	}

	if resp.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", resp.Status)
	}
	if resp.Data.TotalCount != 2 {
		t.Errorf("expected 2 transactions, got %d", resp.Data.TotalCount)
	}
	if len(resp.Data.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp.Data.Entries))
	}
	if resp.Data.Entries[0].TransactionType != "purchase:pos" {
		t.Errorf("expected transaction type 'purchase:pos', got %s", resp.Data.Entries[0].TransactionType)
	}
	if resp.Data.Entries[0].Amount.Amount != 5000.00 {
		t.Errorf("expected amount 5000.00, got %f", resp.Data.Entries[0].Amount.Amount)
	}
}

func TestGetAccountHistory_WithMockServer(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	c, _ := NewClient("testuser", "testpass", nil, "")
	c.APIBaseURL = server.URL
	c.ClientID = "test-client-id"

	resp, err := c.GetAccountHistory("test-token", "2000000001", 50, 0)
	if err != nil {
		t.Fatalf("GetAccountHistory failed: %v", err)
	}

	if resp.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", resp.Status)
	}
	if len(resp.Data.Transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(resp.Data.Transactions))
	}
	if resp.Data.Transactions[0].FlowDirection != "OUTCOME" {
		t.Errorf("expected flow direction OUTCOME, got %s", resp.Data.Transactions[0].FlowDirection)
	}
	if resp.Data.Transactions[0].TransactionAmount.Value != 25000.00 {
		t.Errorf("expected amount 25000.00, got %f", resp.Data.Transactions[0].TransactionAmount.Value)
	}
}

func TestGetEventsPast_WithMockServer(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	c, _ := NewClient("testuser", "testpass", nil, "")
	c.APIBaseURL = server.URL
	c.ClientID = "test-client-id"

	resp, err := c.GetEventsPast("test-token", "2000000001", 50, 0)
	if err != nil {
		t.Fatalf("GetEventsPast failed: %v", err)
	}

	if resp.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", resp.Status)
	}
	if resp.Data.TotalCount != 1 {
		t.Errorf("expected 1 transaction, got %d", resp.Data.TotalCount)
	}
	if resp.Data.Entries[0].TransactionType != "purchase:online" {
		t.Errorf("expected transaction type 'purchase:online', got %s", resp.Data.Entries[0].TransactionType)
	}
}

func TestValidateSession_Valid(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	c, _ := NewClient("testuser", "testpass", nil, "")
	c.APIBaseURL = server.URL
	c.ClientID = "test-client-id"

	valid := c.ValidateSession("test-token")
	if !valid {
		t.Error("expected session to be valid")
	}
}

func TestValidateSession_Invalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	c, _ := NewClient("testuser", "testpass", nil, "")
	c.APIBaseURL = server.URL
	c.ClientID = "test-client-id"

	valid := c.ValidateSession("invalid-token")
	if valid {
		t.Error("expected session to be invalid")
	}
}

func TestInitializeSession_WithMockServer(t *testing.T) {
	server := mockAPIServer(t)
	defer server.Close()

	c, _ := NewClient("testuser", "testpass", nil, "")
	c.APIBaseURL = server.URL

	err := c.InitializeSession("test-token")
	if err != nil {
		t.Fatalf("InitializeSession failed: %v", err)
	}

	if c.ClientID != "test-client-id" {
		t.Errorf("expected clientID 'test-client-id', got '%s'", c.ClientID)
	}
}

func TestGetTransactions_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer server.Close()

	c, _ := NewClient("testuser", "testpass", nil, "")
	c.APIBaseURL = server.URL
	c.ClientID = "test-client-id"

	_, err := c.GetTransactions("test-token", "123")
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestGetAccountsAndCards_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	c, _ := NewClient("testuser", "testpass", nil, "")
	c.APIBaseURL = server.URL
	c.ClientID = "test-client-id"

	_, err := c.GetAccountsAndCards("test-token")
	if err == nil {
		t.Error("expected error for not found response")
	}
}
