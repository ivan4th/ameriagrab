package client

import (
	"net/http"
	"time"
)

// PushStatusResponse holds the push notification status
type PushStatusResponse struct {
	Status string `json:"status"`
	Data   struct {
		SessionStatus string `json:"sessionStatus"`
	} `json:"data"`
	ErrorMessages interface{} `json:"errorMessages"`
}

// TokenResponse holds the OAuth token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	IDToken      string `json:"id_token"`
}

// TransactionsResponse holds the response from /api/events/settled/{cardId} or /api/events/past
type TransactionsResponse struct {
	Status string `json:"status"`
	Data   struct {
		TotalCount int           `json:"totalCount"`
		Entries    []Transaction `json:"entries"`
	} `json:"data"`
	ErrorMessages interface{} `json:"errorMessages"`
}

// Transaction represents a card transaction
type Transaction struct {
	ID                         string `json:"id"`
	TransactionType            string `json:"transactionType"`
	AccountingType             string `json:"accountingType"`
	State                      string `json:"state"`
	Amount                     Amount `json:"amount"`
	CorrespondentAccountNumber string `json:"correspondentAccountNumber"`
	CorrespondentAccountName   string `json:"correspondentAccountName"`
	Details                    string `json:"details"`
	OperationDate              string `json:"operationDate"`
	WorkflowCode               string `json:"workflowCode"`
	Date                       string `json:"date"`
	Year                       string `json:"year"`
	Month                      string `json:"month"`
}

// Amount represents a monetary amount with currency
type Amount struct {
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// AccountsAndCardsResponse holds the response from /api/accounts-and-cards
type AccountsAndCardsResponse struct {
	Status string `json:"status"`
	Data   struct {
		AccountsAndCards []ProductInfo `json:"accountsAndCards"`
	} `json:"data"`
	ErrorMessages interface{} `json:"errorMessages"`
}

// ProductInfo represents a card or account
type ProductInfo struct {
	ProductType   string  `json:"productType"` // "CARD" or "ACCOUNT"
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	CardNumber    string  `json:"cardNumber,omitempty"`    // Cards only
	AccountNumber string  `json:"accountNumber,omitempty"` // Accounts only
	AccountID     string  `json:"accountId,omitempty"`     // Cards only: linked account ID
	Currency      string  `json:"currency"`
	Balance       float64 `json:"balance"`
	Status        string  `json:"status"`
}

// HistoryResponse holds the response from /api/history
type HistoryResponse struct {
	Status string `json:"status"`
	Data   struct {
		HasNext      bool                 `json:"hasNext"`
		IsUpToDate   bool                 `json:"isUpToDate"`
		Transactions []AccountTransaction `json:"transactions"`
	} `json:"data"`
	ErrorMessages interface{} `json:"errorMessages"`
}

// AccountTransaction represents a transaction in account history
type AccountTransaction struct {
	ID                  string         `json:"id"`
	TransactionID       string         `json:"transactionId"`
	OperationID         string         `json:"operationId"`
	Status              string         `json:"status"`
	TransactionType     string         `json:"transactionType"`
	WorkflowCode        string         `json:"workflowCode"`
	FlowDirection       string         `json:"flowDirection"`
	TransactionDate     int64          `json:"transactionDate"`
	SettledDate         int64          `json:"settledDate"`
	Date                string         `json:"date"`
	Month               string         `json:"month"`
	Year                string         `json:"year"`
	DebitAccountNumber  string         `json:"debitAccountNumber"`
	CreditAccountNumber string         `json:"creditAccountNumber"`
	BeneficiaryName     string         `json:"beneficiaryName"`
	Details             string         `json:"details"`
	SourceSystem        string         `json:"sourceSystem"`
	TransactionAmount   TransactionAmt `json:"transactionAmount"`
	SettledAmount       TransactionAmt `json:"settledAmount"`
	DomesticAmount      TransactionAmt `json:"domesticAmount"`
}

// TransactionAmt represents an amount with currency in history
type TransactionAmt struct {
	Currency string  `json:"currency"`
	Value    float64 `json:"value"`
}

// SessionData holds persistent session information
type SessionData struct {
	AccessToken  string             `json:"access_token"`
	RefreshToken string             `json:"refresh_token"`
	ExpiresAt    time.Time          `json:"expires_at"`
	ClientID     string             `json:"client_id"`
	Cookies      []SerializedCookie `json:"cookies"`
}

// SerializedCookie represents a cookie that can be serialized to JSON
type SerializedCookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires"`
	Secure   bool      `json:"secure"`
	HttpOnly bool      `json:"http_only"`
}

// UserInfoResponse holds the user info API response
type UserInfoResponse struct {
	Status string `json:"status"`
	Data   struct {
		UserInfo struct {
			Sub string `json:"sub"`
			ID  string `json:"id"`
		} `json:"userInfo"`
	} `json:"data"`
}

// ClientsResponse holds the clients API response
type ClientsResponse struct {
	Status string `json:"status"`
	Data   struct {
		Clients []struct {
			ID      string `json:"id"`
			Default bool   `json:"default"`
		} `json:"clients"`
	} `json:"data"`
}

// Client represents the Ameriabank API client
type Client struct {
	HTTPClient  *http.Client
	Username    string
	Password    string
	SessionDir  string
	SessionFile string
	DebugDir    string
	ClientID    string // Consistent client ID for this session
	APIBaseURL  string // Base URL for API calls (defaults to APIBaseURL constant)
	AuthBaseURL string // Base URL for auth calls (defaults to AuthBaseURL constant)
}
