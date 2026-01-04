package output

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/ivan4th/ameriagrab/client"
)

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "he..."},
		{"hello world", 8, "hello..."},
		{"ab", 3, "ab"},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
		{"", 5, ""},
		{"test", 0, ""},
		{"test", 1, "t"},
		{"test", 2, "te"},
		{"test", 3, "tes"},
	}

	for _, tt := range tests {
		result := TruncateString(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("TruncateString(%q, %d) = %q, expected %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestPrintCardTransactions_Table(t *testing.T) {
	txns := &client.TransactionsResponse{
		Status: "SUCCESS",
		Data: struct {
			TotalCount int                  `json:"totalCount"`
			Entries    []client.Transaction `json:"entries"`
		}{
			TotalCount: 2,
			Entries: []client.Transaction{
				{
					ID:              "txn001",
					TransactionType: "purchase:pos",
					AccountingType:  "DEBIT",
					Amount: client.Amount{
						Currency: "AMD",
						Amount:   5000.00,
					},
					Details:       "Test Store Purchase",
					OperationDate: "2024-01-15T10:30:00Z",
					Date:          "2024-01-15",
				},
				{
					ID:              "txn002",
					TransactionType: "pre-purchase:atm",
					AccountingType:  "CREDIT",
					Amount: client.Amount{
						Currency: "USD",
						Amount:   100.00,
					},
					Details:       "ATM Cash Deposit",
					OperationDate: "2024-01-14T14:00:00Z",
					Date:          "2024-01-14",
				},
			},
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintCardTransactions(txns)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected elements
	if !strings.Contains(output, "DATE") {
		t.Error("output should contain DATE header")
	}
	if !strings.Contains(output, "TYPE") {
		t.Error("output should contain TYPE header")
	}
	if !strings.Contains(output, "AMOUNT") {
		t.Error("output should contain AMOUNT header")
	}
	if !strings.Contains(output, "DETAILS") {
		t.Error("output should contain DETAILS header")
	}

	// Check transaction type shortening
	if !strings.Contains(output, "p:pos") {
		t.Error("output should contain shortened 'p:pos' for 'purchase:pos'")
	}
	if !strings.Contains(output, "prep:atm") {
		t.Error("output should contain shortened 'prep:atm' for 'pre-purchase:atm'")
	}

	// Check amounts with signs
	if !strings.Contains(output, "-5000.00 AMD") {
		t.Error("output should contain '-5000.00 AMD' for DEBIT transaction")
	}
	if !strings.Contains(output, "+100.00 USD") {
		t.Error("output should contain '+100.00 USD' for CREDIT transaction")
	}
}

func TestPrintAccountHistory_Table(t *testing.T) {
	history := &client.HistoryResponse{
		Status: "SUCCESS",
		Data: struct {
			HasNext      bool                       `json:"hasNext"`
			IsUpToDate   bool                       `json:"isUpToDate"`
			Transactions []client.AccountTransaction `json:"transactions"`
		}{
			HasNext:    true,
			IsUpToDate: true,
			Transactions: []client.AccountTransaction{
				{
					ID:              "hist001",
					TransactionType: "transfer:internal",
					FlowDirection:   "OUTCOME",
					TransactionDate: 1705312200000,
					BeneficiaryName: "Test Beneficiary",
					Details:         "Internal transfer for testing",
					TransactionAmount: client.TransactionAmt{
						Currency: "AMD",
						Value:    25000.00,
					},
				},
				{
					ID:              "hist002",
					TransactionType: "transfer:external",
					FlowDirection:   "INCOME",
					TransactionDate: 1705225800000,
					BeneficiaryName: "External Sender",
					Details:         "Incoming transfer",
					TransactionAmount: client.TransactionAmt{
						Currency: "USD",
						Value:    500.00,
					},
				},
			},
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintAccountHistory(history)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected elements
	if !strings.Contains(output, "DATE") {
		t.Error("output should contain DATE header")
	}
	if !strings.Contains(output, "BENEFICIARY") {
		t.Error("output should contain BENEFICIARY header")
	}

	// Check transaction type shortening
	if !strings.Contains(output, "xfer:internal") {
		t.Error("output should contain shortened 'xfer:internal' for 'transfer:internal'")
	}

	// Check amounts with signs
	if !strings.Contains(output, "-25000.00 AMD") {
		t.Error("output should contain '-25000.00 AMD' for OUTCOME transaction")
	}
	if !strings.Contains(output, "+500.00 USD") {
		t.Error("output should contain '+500.00 USD' for INCOME transaction")
	}
}

func TestPrintAccountsAndCards(t *testing.T) {
	resp := &client.AccountsAndCardsResponse{
		Status: "SUCCESS",
		Data: struct {
			AccountsAndCards []client.ProductInfo `json:"accountsAndCards"`
		}{
			AccountsAndCards: []client.ProductInfo{
				{
					ProductType: "CARD",
					ID:          "1000000001",
					Name:        "Test Card",
					CardNumber:  "4000********1234",
					Currency:    "AMD",
					Balance:     150000.00,
					Status:      "ACTIVE",
				},
				{
					ProductType:   "ACCOUNT",
					ID:            "2000000001",
					Name:          "Test Account",
					AccountNumber: "1570000000000000",
					Currency:      "USD",
					Balance:       5000.00,
					Status:        "ACTIVE",
				},
			},
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintAccountsAndCards(resp)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected elements
	if !strings.Contains(output, "TYPE") {
		t.Error("output should contain TYPE header")
	}
	if !strings.Contains(output, "ID") {
		t.Error("output should contain ID header")
	}
	if !strings.Contains(output, "NUMBER") {
		t.Error("output should contain NUMBER header")
	}
	if !strings.Contains(output, "BALANCE") {
		t.Error("output should contain BALANCE header")
	}

	// Check for card
	if !strings.Contains(output, "CARD") {
		t.Error("output should contain CARD product type")
	}
	if !strings.Contains(output, "4000********1234") {
		t.Error("output should contain card number")
	}

	// Check for account
	if !strings.Contains(output, "ACCOUNT") {
		t.Error("output should contain ACCOUNT product type")
	}
	if !strings.Contains(output, "1570000000000000") {
		t.Error("output should contain account number")
	}
}

func TestTruncateString_EdgeCases(t *testing.T) {
	// Unicode characters
	result := TruncateString("Hello, 世界!", 10)
	if len(result) > 10 {
		t.Errorf("TruncateString should limit to 10 bytes, got %d", len(result))
	}

	// Long string
	longStr := strings.Repeat("a", 1000)
	result = TruncateString(longStr, 50)
	if result != strings.Repeat("a", 47)+"..." {
		t.Error("TruncateString should truncate long strings properly")
	}
}
