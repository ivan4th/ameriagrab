package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:146.0) Gecko/20100101 Firefox/146.0"

	authBaseURL   = "https://account.myameria.am"
	apiBaseURL    = "https://ob.myameria.am"
	redirectURI   = "https://myameria.am/"
	clientID      = "banqr-online"
	clientSecret  = "b54f3f83-a696-48da-95de-b9b4154a3944"
	pollInterval  = 3 * time.Second
	pollTimeout   = 120 * time.Second
)

type PushStatusResponse struct {
	Status string `json:"status"`
	Data   struct {
		SessionStatus string `json:"sessionStatus"`
	} `json:"data"`
	ErrorMessages interface{} `json:"errorMessages"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	IDToken      string `json:"id_token"`
}

type TransactionsResponse struct {
	Status string `json:"status"`
	Data   struct {
		TotalCount int           `json:"totalCount"`
		Entries    []Transaction `json:"entries"`
	} `json:"data"`
	ErrorMessages interface{} `json:"errorMessages"`
}

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
	ID                  string          `json:"id"`
	TransactionID       string          `json:"transactionId"`
	OperationID         string          `json:"operationId"`
	Status              string          `json:"status"`
	TransactionType     string          `json:"transactionType"`
	WorkflowCode        string          `json:"workflowCode"`
	FlowDirection       string          `json:"flowDirection"`
	TransactionDate     int64           `json:"transactionDate"`
	SettledDate         int64           `json:"settledDate"`
	Date                string          `json:"date"`
	Month               string          `json:"month"`
	Year                string          `json:"year"`
	DebitAccountNumber  string          `json:"debitAccountNumber"`
	CreditAccountNumber string          `json:"creditAccountNumber"`
	BeneficiaryName     string          `json:"beneficiaryName"`
	Details             string          `json:"details"`
	SourceSystem        string          `json:"sourceSystem"`
	TransactionAmount   TransactionAmt  `json:"transactionAmount"`
	SettledAmount       TransactionAmt  `json:"settledAmount"`
	DomesticAmount      TransactionAmt  `json:"domesticAmount"`
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

type AmeriabankClient struct {
	httpClient  *http.Client
	username    string
	password    string
	sessionDir  string
	sessionFile string
	debugDir    string
	clientID    string // Consistent client ID for this session
}

func NewAmeriabankClient(username, password, sessionDir, debugDir string) (*AmeriabankClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects automatically for the final authenticate step
			if strings.Contains(req.URL.String(), "myameria.am/#") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	ac := &AmeriabankClient{
		httpClient: client,
		username:   username,
		password:   password,
		clientID:   "", // Will be set by InitializeSession or restored from saved session
	}

	// Set up session directory with restricted permissions (if provided)
	if sessionDir != "" {
		if err := os.MkdirAll(sessionDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create session directory: %w", err)
		}
		ac.sessionDir = sessionDir
		ac.sessionFile = filepath.Join(sessionDir, "session.json")
		fmt.Fprintf(os.Stderr, "Debug: Session persistence enabled at %s\n", ac.sessionFile)
	} else {
		fmt.Fprintf(os.Stderr, "Debug: Session persistence disabled (AMERIA_SESSION_DIR not set)\n")
	}

	// Set up debug directory (if provided)
	if debugDir != "" {
		if err := os.MkdirAll(debugDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create debug directory: %w", err)
		}
		ac.debugDir = debugDir
		fmt.Fprintf(os.Stderr, "Debug: Debug file output enabled at %s\n", ac.debugDir)
	}

	return ac, nil
}

// saveDebugFile saves content to a debug file if debug dir is configured
func (c *AmeriabankClient) saveDebugFile(filename string, content []byte) {
	if c.debugDir == "" {
		return
	}
	path := filepath.Join(c.debugDir, filename)
	if err := os.WriteFile(path, content, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save debug file %s: %v\n", path, err)
	} else {
		fmt.Fprintf(os.Stderr, "Debug: Saved debug file to %s\n", path)
	}
}

// getSessionURLs returns the URLs for which we want to save/restore cookies
func getSessionURLs() []*url.URL {
	authURL, _ := url.Parse(authBaseURL)
	apiURL, _ := url.Parse(apiBaseURL)
	mainURL, _ := url.Parse(redirectURI)
	return []*url.URL{authURL, apiURL, mainURL}
}

// saveSession saves the current session (cookies + tokens) to disk
func (c *AmeriabankClient) saveSession(accessToken, refreshToken string, expiresIn int) error {
	if c.sessionFile == "" {
		return nil // Session persistence disabled
	}

	session := SessionData{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		ClientID:     c.clientID,
		Cookies:      []SerializedCookie{},
	}

	// Collect cookies from all relevant URLs
	for _, u := range getSessionURLs() {
		for _, cookie := range c.httpClient.Jar.Cookies(u) {
			session.Cookies = append(session.Cookies, SerializedCookie{
				Name:     cookie.Name,
				Value:    cookie.Value,
				Domain:   u.Host,
				Path:     cookie.Path,
				Expires:  cookie.Expires,
				Secure:   cookie.Secure,
				HttpOnly: cookie.HttpOnly,
			})
		}
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Write with restricted permissions (owner read/write only)
	if err := os.WriteFile(c.sessionFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Debug: Session saved to %s\n", c.sessionFile)
	return nil
}

// updateSessionClientID updates the saved session with the current clientID
func (c *AmeriabankClient) updateSessionClientID() error {
	if c.sessionFile == "" {
		return nil // Session persistence disabled
	}

	data, err := os.ReadFile(c.sessionFile)
	if err != nil {
		return fmt.Errorf("failed to read session file: %w", err)
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("failed to parse session: %w", err)
	}

	session.ClientID = c.clientID

	data, err = json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(c.sessionFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Debug: Session updated with Client ID: %s\n", c.clientID)
	return nil
}

// loadSession attempts to load a saved session from disk
func (c *AmeriabankClient) loadSession() (*SessionData, error) {
	if c.sessionFile == "" {
		return nil, nil // Session persistence disabled
	}

	data, err := os.ReadFile(c.sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No saved session
		}
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	// Check if token is expired (with 1 minute buffer)
	if time.Now().Add(time.Minute).After(session.ExpiresAt) {
		fmt.Fprintf(os.Stderr, "Debug: Saved session has expired\n")
		return nil, nil
	}

	// Restore cookies to the jar
	for _, u := range getSessionURLs() {
		var cookies []*http.Cookie
		for _, sc := range session.Cookies {
			if sc.Domain == u.Host || strings.HasSuffix(u.Host, sc.Domain) {
				cookies = append(cookies, &http.Cookie{
					Name:     sc.Name,
					Value:    sc.Value,
					Path:     sc.Path,
					Expires:  sc.Expires,
					Secure:   sc.Secure,
					HttpOnly: sc.HttpOnly,
				})
			}
		}
		if len(cookies) > 0 {
			c.httpClient.Jar.SetCookies(u, cookies)
		}
	}

	// Restore client ID
	if session.ClientID != "" {
		c.clientID = session.ClientID
		fmt.Fprintf(os.Stderr, "Debug: Restored Client ID: %s\n", c.clientID)
	}

	fmt.Fprintf(os.Stderr, "Debug: Session loaded from %s (expires at %s)\n", c.sessionFile, session.ExpiresAt.Format(time.RFC3339))
	return &session, nil
}

// validateSession checks if the saved session is still valid by making a test API call
func (c *AmeriabankClient) validateSession(accessToken string) bool {
	// Use a short timeout to detect killed server-side sessions quickly
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use /api/users/info - same endpoint that InitializeSession uses successfully
	req, err := http.NewRequestWithContext(ctx, "GET", apiBaseURL+"/api/users/info", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Debug: validateSession request creation failed: %v\n", err)
		return false
	}

	// Use full API headers (requires clientID to be set)
	c.addAPIHeaders(req, accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Timeout or network error - session likely invalid or server unreachable
		fmt.Fprintf(os.Stderr, "Debug: validateSession request failed: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	fmt.Fprintf(os.Stderr, "Debug: validateSession response status: %d\n", resp.StatusCode)

	// 200 means session is valid
	// 401 or 403 means token/session is invalid
	return resp.StatusCode == http.StatusOK
}

// GetOrRefreshToken tries to use a saved session or performs a fresh login
func (c *AmeriabankClient) GetOrRefreshToken() (string, error) {
	// Try to load saved session
	session, err := c.loadSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Debug: Error loading session: %v\n", err)
	}

	if session != nil {
		fmt.Fprintf(os.Stderr, "Debug: Found saved session, validating...\n")
		if c.validateSession(session.AccessToken) {
			fmt.Fprintf(os.Stderr, "Debug: Saved session is valid, reusing\n")
			return session.AccessToken, nil
		}
		fmt.Fprintf(os.Stderr, "Debug: Saved session is invalid, need fresh login\n")
		// Clear clientID so InitializeSession runs after fresh login
		c.clientID = ""
	}

	// Perform fresh login
	return c.Login()
}

// addAPIHeaders adds common headers for API requests
func (c *AmeriabankClient) addAPIHeaders(req *http.Request, accessToken string) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Referer", redirectURI)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Banqr-2FA", buildTwoFAHeader())
	req.Header.Set("X-Banqr-CDDC", buildCDDCHeader())
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Timezone-Offset", "-240")
	req.Header.Set("Client-Time", time.Now().Format("15:04:05"))
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Locale", "ru")
	req.Header.Set("Origin", redirectURI[:len(redirectURI)-1])
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Priority", "u=0")
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

// InitializeSession fetches the real client ID and sets up the session
func (c *AmeriabankClient) InitializeSession(accessToken string) error {
	fmt.Fprintf(os.Stderr, "Debug: Initializing session...\n")

	// Step 1: Get user info to get user ID
	req, err := http.NewRequest("GET", apiBaseURL+"/api/users/info", nil)
	if err != nil {
		return fmt.Errorf("failed to create user info request: %w", err)
	}
	c.addAPIHeaders(req, accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("user info request failed with status %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
	}

	var userInfo UserInfoResponse
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return fmt.Errorf("failed to parse user info: %w", err)
	}
	userID := userInfo.Data.UserInfo.ID
	fmt.Fprintf(os.Stderr, "Debug: User ID: %s\n", userID)

	// Step 2: Get clients to find the real Client-Id
	req, err = http.NewRequest("GET", fmt.Sprintf("%s/api/users/%s/clients", apiBaseURL, userID), nil)
	if err != nil {
		return fmt.Errorf("failed to create clients request: %w", err)
	}
	c.addAPIHeaders(req, accessToken)

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch clients: %w", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("clients request failed with status %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
	}

	var clientsResp ClientsResponse
	if err := json.Unmarshal(body, &clientsResp); err != nil {
		return fmt.Errorf("failed to parse clients: %w", err)
	}

	// Find the default client or use the first one
	for _, client := range clientsResp.Data.Clients {
		if client.Default {
			c.clientID = client.ID
			break
		}
	}
	if c.clientID == "" && len(clientsResp.Data.Clients) > 0 {
		c.clientID = clientsResp.Data.Clients[0].ID
	}
	if c.clientID == "" {
		return fmt.Errorf("no client found in clients response")
	}

	fmt.Fprintf(os.Stderr, "Debug: Client ID set to: %s\n", c.clientID)
	fmt.Fprintf(os.Stderr, "Debug: Session initialized successfully\n")
	return nil
}

func (c *AmeriabankClient) addFirefoxHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Connection", "keep-alive")
}

func (c *AmeriabankClient) Login() (string, error) {
	// Step 1: Get the login page to extract the action URL
	state := uuid.New().String()
	nonce := uuid.New().String()

	authURL := fmt.Sprintf(
		"%s/auth/realms/ameria/protocol/openid-connect/auth?client_id=%s&redirect_uri=%s&state=%s&response_mode=fragment&response_type=code&scope=openid&nonce=%s&kc_locale=ru",
		authBaseURL,
		clientID,
		url.QueryEscape(redirectURI),
		state,
		nonce,
	)

	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}
	c.addFirefoxHeaders(req)
	req.Header.Set("Referer", redirectURI)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Sec-Fetch-User", "?1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get login page: %w", err)
	}
	defer resp.Body.Close()

	fmt.Fprintf(os.Stderr, "Debug: Login page status: %d\n", resp.StatusCode)
	for _, cookie := range resp.Cookies() {
		fmt.Fprintf(os.Stderr, "Debug: Set-Cookie: %s=%s...\n", cookie.Name, cookie.Value[:min(20, len(cookie.Value))])
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read login page: %w", err)
	}

	// Extract actionUrl from the login page
	actionURLRegex := regexp.MustCompile(`actionUrl:\s*"([^"]+)"`)
	matches := actionURLRegex.FindSubmatch(body)
	if len(matches) < 2 {
		c.saveDebugFile("login_page.html", body)
		return "", fmt.Errorf("failed to find actionUrl in login page")
	}
	actionURL := string(matches[1])
	fmt.Fprintf(os.Stderr, "Debug: Action URL: %s\n", actionURL)

	// Step 2: Submit login credentials
	formData := url.Values{}
	formData.Set("username", c.username)
	formData.Set("password", c.password)
	formData.Set("X-Banqr-CDDC", buildCDDCHeader())
	formData.Set("remember", "on")

	encodedForm := formData.Encode()
	fmt.Fprintf(os.Stderr, "Debug: Form data (length %d)\n", len(encodedForm))

	req, err = http.NewRequest("POST", actionURL, strings.NewReader(encodedForm))
	if err != nil {
		return "", fmt.Errorf("failed to create login request: %w", err)
	}
	c.addFirefoxHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", authBaseURL)
	req.Header.Set("Referer", authURL)
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Priority", "u=0, i")

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to submit login: %w", err)
	}
	defer resp.Body.Close()

	fmt.Fprintf(os.Stderr, "Debug: Login POST status: %d\n", resp.StatusCode)
	for _, cookie := range resp.Cookies() {
		fmt.Fprintf(os.Stderr, "Debug: Post Set-Cookie: %s=%s...\n", cookie.Name, cookie.Value[:min(20, len(cookie.Value))])
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read login response: %w", err)
	}

	// Check for error messages in the response
	errorRegex := regexp.MustCompile(`(?i)error|invalid|incorrect|failed`)
	if errorRegex.Match(body) {
		// Look for specific error message
		msgRegex := regexp.MustCompile(`"message"\s*:\s*"([^"]+)"`)
		if msgMatch := msgRegex.FindSubmatch(body); len(msgMatch) >= 2 {
			fmt.Fprintf(os.Stderr, "Debug: Error message found: %s\n", string(msgMatch[1]))
		}
	}

	// Debug: print response status and check template type
	templateRegex := regexp.MustCompile(`template:\s*"([^"]+)"`)
	if tmplMatch := templateRegex.FindSubmatch(body); len(tmplMatch) >= 2 {
		fmt.Fprintf(os.Stderr, "Debug: Template type: %s\n", string(tmplMatch[1]))
	}

	// Extract push session ID and new action URL
	sessionIDRegex := regexp.MustCompile(`external_system_request_id.*?=\s*"([^"]+)"`)
	matches = sessionIDRegex.FindSubmatch(body)
	if len(matches) < 2 {
		c.saveDebugFile("debug_response.html", body)
		return "", fmt.Errorf("failed to find push session ID in response. Login may have failed. Response preview: %s", string(body[:min(1000, len(body))]))
	}
	pushSessionID := string(matches[1])

	// Extract evaluatedRequestId for the second POST
	evalReqIDRegex := regexp.MustCompile(`evaluatedRequestId:\s*\\?"([^"\\]+)`)
	evalMatches := evalReqIDRegex.FindSubmatch(body)
	if len(evalMatches) < 2 {
		c.saveDebugFile("debug_response.html", body)
		return "", fmt.Errorf("failed to find evaluatedRequestId in response")
	}
	evaluatedRequestID := string(evalMatches[1])
	fmt.Fprintf(os.Stderr, "Debug: evaluatedRequestId: %s\n", evaluatedRequestID)

	// Extract new actionUrl for the second POST
	matches = actionURLRegex.FindSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to find actionUrl for push confirmation")
	}
	pushActionURL := string(matches[1])

	// Step 3: Wait for push notification confirmation
	fmt.Println("Waiting for push notification confirmation on your phone...")

	err = c.waitForPushConfirmation(pushSessionID)
	if err != nil {
		return "", fmt.Errorf("push confirmation failed: %w", err)
	}

	fmt.Println("\nPush confirmed! Completing authentication...")

	// Step 4: Submit push confirmation with required form data
	pushFormData := url.Values{}
	pushFormData.Set("evaluated_request_id", evaluatedRequestID)
	pushFormData.Set("X-Banqr-CDDC", buildCDDCHeader())
	pushFormData.Set("evaluate_action", "true")
	pushFormData.Set("form_action", "submit_button")
	pushFormData.Set("totp", pushSessionID)

	fmt.Fprintf(os.Stderr, "Debug: Push form - evaluated_request_id=%s, totp=%s\n", evaluatedRequestID, pushSessionID)

	req, err = http.NewRequest("POST", pushActionURL, strings.NewReader(pushFormData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create push confirmation request: %w", err)
	}
	c.addFirefoxHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", authBaseURL)
	req.Header.Set("Referer", actionURL)
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Priority", "u=0, i")

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to submit push confirmation: %w", err)
	}
	defer resp.Body.Close()

	fmt.Fprintf(os.Stderr, "Debug: Push confirmation POST status: %d\n", resp.StatusCode)

	// Check for redirect with authorization code
	location := resp.Header.Get("Location")
	if location == "" {
		body, _ := io.ReadAll(resp.Body)
		c.saveDebugFile("push_response.html", body)
		return "", fmt.Errorf("expected redirect after push confirmation, got status %d. Preview: %s", resp.StatusCode, string(body[:min(500, len(body))]))
	}
	fmt.Fprintf(os.Stderr, "Debug: Redirect location: %s\n", location)

	// Extract authorization code from redirect URL fragment
	codeRegex := regexp.MustCompile(`code=([^&]+)`)
	codeMatches := codeRegex.FindStringSubmatch(location)
	if len(codeMatches) < 2 {
		return "", fmt.Errorf("failed to extract authorization code from redirect: %s", location)
	}
	authCode := codeMatches[1]

	// Step 5: Exchange authorization code for access token
	return c.exchangeCodeForToken(authCode)
}

func (c *AmeriabankClient) waitForPushConfirmation(sessionID string) error {
	pushStatusURL := fmt.Sprintf("%s/push-status?sessionId=%s", authBaseURL, sessionID)

	startTime := time.Now()
	for {
		if time.Since(startTime) > pollTimeout {
			return fmt.Errorf("push confirmation timed out after %v", pollTimeout)
		}

		req, err := http.NewRequest("GET", pushStatusURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create push status request: %w", err)
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to check push status: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read push status response: %w", err)
		}

		var status PushStatusResponse
		if err := json.Unmarshal(body, &status); err != nil {
			return fmt.Errorf("failed to parse push status response: %w", err)
		}

		switch status.Data.SessionStatus {
		case "accepted":
			return nil
		case "pending":
			fmt.Print(".")
			time.Sleep(pollInterval)
		case "rejected":
			return fmt.Errorf("push notification was rejected")
		case "expired":
			return fmt.Errorf("push notification expired")
		default:
			return fmt.Errorf("unexpected push status: %s", status.Data.SessionStatus)
		}
	}
}

func (c *AmeriabankClient) exchangeCodeForToken(code string) (string, error) {
	tokenURL := fmt.Sprintf("%s/auth/realms/ameria/protocol/openid-connect/token", authBaseURL)

	formData := url.Values{}
	formData.Set("code", code)
	formData.Set("grant_type", "authorization_code")
	formData.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	// Add Basic auth header
	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", redirectURI[:len(redirectURI)-1])
	req.Header.Set("Referer", redirectURI)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	// Debug: decode and print token claims
	parts := strings.Split(tokenResp.AccessToken, ".")
	if len(parts) >= 2 {
		// Add padding for base64 decoding
		payload := parts[1]
		if pad := len(payload) % 4; pad != 0 {
			payload += strings.Repeat("=", 4-pad)
		}
		if decoded, err := base64.URLEncoding.DecodeString(payload); err == nil {
			// Parse JSON to find scope
			var claims map[string]interface{}
			if json.Unmarshal(decoded, &claims) == nil {
				if scope, ok := claims["scope"]; ok {
					fmt.Fprintf(os.Stderr, "Debug: Token scope: %v\n", scope)
				}
				if channel, ok := claims["user-channel"]; ok {
					fmt.Fprintf(os.Stderr, "Debug: Token user-channel: %v\n", channel)
				}
			}
		}
	}

	// Save session for future use
	if err := c.saveSession(tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save session: %v\n", err)
	}

	return tokenResp.AccessToken, nil
}

func (c *AmeriabankClient) GetTransactions(accessToken, cardID string) (*TransactionsResponse, error) {
	txnURL := fmt.Sprintf("%s/api/events/settled/%s", apiBaseURL, cardID)

	req, err := http.NewRequest("GET", txnURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactions request: %w", err)
	}
	c.addAPIHeaders(req, accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transactions: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read transactions response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transactions request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var txnResp TransactionsResponse
	if err := json.Unmarshal(body, &txnResp); err != nil {
		return nil, fmt.Errorf("failed to parse transactions response: %w", err)
	}

	return &txnResp, nil
}

// GetAccountsAndCards fetches all accounts and cards
func (c *AmeriabankClient) GetAccountsAndCards(accessToken string) (*AccountsAndCardsResponse, error) {
	url := fmt.Sprintf("%s/api/accounts-and-cards?page=0&size=100&skipApplications=false&specifications=SIMPLE&isFullList=true", apiBaseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create accounts-and-cards request: %w", err)
	}
	c.addAPIHeaders(req, accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch accounts-and-cards: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read accounts-and-cards response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("accounts-and-cards request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result AccountsAndCardsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse accounts-and-cards response: %w", err)
	}

	return &result, nil
}

// GetAccountHistory fetches transaction history for an account
func (c *AmeriabankClient) GetAccountHistory(accessToken, accountID string, size, page int) (*HistoryResponse, error) {
	url := fmt.Sprintf("%s/api/history?accountIds=%s&size=%d&page=%d", apiBaseURL, accountID, size, page)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create history request: %w", err)
	}
	c.addAPIHeaders(req, accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch history: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read history response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("history request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result HistoryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse history response: %w", err)
	}

	return &result, nil
}

// GetEventsPast fetches past events/transactions for an account (works with card-linked accounts)
func (c *AmeriabankClient) GetEventsPast(accessToken, accountID string, size, page int) (*TransactionsResponse, error) {
	url := fmt.Sprintf("%s/api/events/past?locale=ru&fromAmount=0.1&accountIds=%s&sort=date&size=%d&page=%d",
		apiBaseURL, accountID, size, page)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create events/past request: %w", err)
	}
	c.addAPIHeaders(req, accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events/past: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read events/past response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("events/past request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result TransactionsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse events/past response: %w", err)
	}

	return &result, nil
}

func buildCDDCHeader() string {
	cddc := map[string]interface{}{
		"browserCDDC": map[string]interface{}{
			"fingerprintRaw": `{"browser":{"userAgent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:146.0) Gecko/20100101 Firefox/146.0","applicationVersion":"5.0 (Macintosh)","applicationCode":"Mozilla","applicationName":"Netscape","cookieEnabled":true,"javaEnabled":false},"support":{"ajax":true,"boxModel":true,"changeBubbles":true,"checkClone":true,"checkOn":true,"cors":true,"cssFloat":true,"hrefNormalized":true,"htmlSerialize":true,"leadingWhitespace":true,"noCloneChecked":true,"noCloneEvent":true,"opacity":true,"optDisabled":true,"style":true,"submitBubbles":true,"tbody":true},"device":{"screenWidth":1920,"screenHeight":1080,"os":"Apple MacOS","language":"en-US","platform":"MacIntel","timeZone":-240},"plugin":[{"name":"PDF Viewer","file":"internal-pdf-viewer"},{"name":"Chrome PDF Viewer","file":"internal-pdf-viewer"},{"name":"Chromium PDF Viewer","file":"internal-pdf-viewer"},{"name":"Microsoft Edge PDF Viewer","file":"internal-pdf-viewer"},{"name":"WebKit built-in PDF","file":"internal-pdf-viewer"}]}`,
			"fingerprintHash": "2ce4831e26386fd68e0554378fc85ef41902ad5f6ab0ba61a74265ebb05923b0",
		},
	}
	jsonBytes, _ := json.Marshal(cddc)
	return base64.StdEncoding.EncodeToString(jsonBytes)
}

func buildTwoFAHeader() string {
	twoFA := map[string]interface{}{
		"otpEvaluation": map[string]interface{}{
			"securityProviderName": "ONESPAN-PHONE",
		},
	}
	jsonBytes, _ := json.Marshal(twoFA)
	return base64.StdEncoding.EncodeToString(jsonBytes)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// printCardTransactions prints card transactions in human-readable table format
func printCardTransactions(txns *TransactionsResponse) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tTYPE\tAMOUNT\tDETAILS")
	for _, t := range txns.Data.Entries {
		// Format amount with +/- sign based on accounting type
		sign := "-"
		if t.AccountingType == "CREDIT" {
			sign = "+"
		}
		amount := fmt.Sprintf("%s%.2f %s", sign, t.Amount.Amount, t.Amount.Currency)

		// Parse and format date
		date := t.Date
		if t.OperationDate != "" {
			if parsed, err := time.Parse(time.RFC3339, t.OperationDate); err == nil {
				date = parsed.Format("2006-01-02 15:04")
			}
		}

		// Shorten transaction type for display
		txType := t.TransactionType
		txType = strings.ReplaceAll(txType, "pre-purchase:", "prep:")
		txType = strings.ReplaceAll(txType, "purchasecompletion:", "pcomp:")
		txType = strings.ReplaceAll(txType, "purchase:", "p:")

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			date, txType, amount, truncateString(t.Details, 50))
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "\nTotal: %d transactions\n", txns.Data.TotalCount)
}

// printAccountHistory prints account history in human-readable table format
func printAccountHistory(history *HistoryResponse) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tTYPE\tAMOUNT\tBENEFICIARY\tDETAILS")
	for _, t := range history.Data.Transactions {
		// Format amount with +/- sign based on flow direction
		sign := "-"
		if t.FlowDirection == "INCOME" {
			sign = "+"
		}
		amount := fmt.Sprintf("%s%.2f %s", sign, t.TransactionAmount.Value, t.TransactionAmount.Currency)

		// Format date from timestamp
		date := t.Date
		if t.TransactionDate > 0 {
			date = time.UnixMilli(t.TransactionDate).Format("2006-01-02 15:04")
		}

		// Shorten transaction type for display
		txType := t.TransactionType
		txType = strings.ReplaceAll(txType, "transfer:", "xfer:")

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			date, txType, amount, truncateString(t.BeneficiaryName, 30), truncateString(t.Details, 40))
	}
	w.Flush()
	if history.Data.HasNext {
		fmt.Fprintln(os.Stderr, "\n(more transactions available, use --page to paginate)")
	}
}

// setupClient creates and authenticates the Ameriabank client
func setupClient() (*AmeriabankClient, string, error) {
	username := os.Getenv("AMERIA_USERNAME")
	password := os.Getenv("AMERIA_PASSWORD")
	sessionDir := os.Getenv("AMERIA_SESSION_DIR")
	debugDir := os.Getenv("AMERIA_DEBUG_DIR")

	if username == "" || password == "" {
		return nil, "", fmt.Errorf("AMERIA_USERNAME and AMERIA_PASSWORD environment variables must be set")
	}

	client, err := NewAmeriabankClient(username, password, sessionDir, debugDir)
	if err != nil {
		return nil, "", fmt.Errorf("creating client: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Checking for saved session or logging in...")
	accessToken, err := client.GetOrRefreshToken()
	if err != nil {
		return nil, "", fmt.Errorf("getting access token: %w", err)
	}

	// Initialize session with prerequisite API calls (only if clientID not restored)
	if client.clientID == "" {
		fmt.Fprintln(os.Stderr, "Client ID not found in session, initializing...")
		if err := client.InitializeSession(accessToken); err != nil {
			return nil, "", fmt.Errorf("initializing session: %w", err)
		}
		if err := client.updateSessionClientID(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update session with client ID: %v\n", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Using restored Client ID: %s\n", client.clientID)
	}

	return client, accessToken, nil
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "ameriagrab",
		Short: "Ameriabank transaction grabber",
		Long: `ameriagrab retrieves accounts, cards, and transaction data from Ameriabank.

Environment variables:
  AMERIA_USERNAME    - Ameriabank username (required)
  AMERIA_PASSWORD    - Ameriabank password (required)
  AMERIA_SESSION_DIR - Directory to persist session (optional)
  AMERIA_DEBUG_DIR   - Directory to save debug files (optional)`,
	}

	// list command
	var jsonOutput bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all accounts and cards",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, accessToken, err := setupClient()
			if err != nil {
				return err
			}

			resp, err := client.GetAccountsAndCards(accessToken)
			if err != nil {
				return fmt.Errorf("fetching accounts and cards: %w", err)
			}

			if jsonOutput {
				output, err := json.MarshalIndent(resp, "", "  ")
				if err != nil {
					return fmt.Errorf("marshaling response: %w", err)
				}
				fmt.Println(string(output))
			} else {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "TYPE\tID\tNUMBER\tNAME\tCURRENCY\tBALANCE\tSTATUS")
				for _, p := range resp.Data.AccountsAndCards {
					number := p.CardNumber
					if p.ProductType == "ACCOUNT" {
						number = p.AccountNumber
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%.2f\t%s\n",
						p.ProductType, p.ID, number, p.Name, p.Currency, p.Balance, p.Status)
				}
				w.Flush()
			}
			return nil
		},
	}
	listCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output as JSON")
	rootCmd.AddCommand(listCmd)

	// get command
	var size, page int
	var getJsonOutput bool
	var forceAccountAPI bool
	getCmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get transactions for a card or account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			client, accessToken, err := setupClient()
			if err != nil {
				return err
			}

			// First, determine if this is a card or account
			resp, err := client.GetAccountsAndCards(accessToken)
			if err != nil {
				return fmt.Errorf("fetching accounts and cards: %w", err)
			}

			var product *ProductInfo
			for i := range resp.Data.AccountsAndCards {
				if resp.Data.AccountsAndCards[i].ID == id {
					product = &resp.Data.AccountsAndCards[i]
					break
				}
			}

			if product == nil {
				return fmt.Errorf("ID %s not found in accounts or cards", id)
			}

			if product.ProductType == "CARD" && !forceAccountAPI {
				// Card: use settled events API
				fmt.Fprintln(os.Stderr, "Fetching card transactions...")
				txns, err := client.GetTransactions(accessToken, id)
				if err != nil {
					return fmt.Errorf("fetching card transactions: %w", err)
				}
				if getJsonOutput {
					output, err := json.MarshalIndent(txns, "", "  ")
					if err != nil {
						return fmt.Errorf("marshaling response: %w", err)
					}
					fmt.Println(string(output))
				} else {
					printCardTransactions(txns)
				}
			} else if product.ProductType == "CARD" && forceAccountAPI {
				// Card with --account flag: use events/past API with linked account ID
				if product.AccountID == "" {
					return fmt.Errorf("card %s has no linked account ID", id)
				}
				fmt.Fprintf(os.Stderr, "Fetching card account history (events/past) for account %s...\n", product.AccountID)
				txns, err := client.GetEventsPast(accessToken, product.AccountID, size, page)
				if err != nil {
					return fmt.Errorf("fetching card account history: %w", err)
				}
				if getJsonOutput {
					output, err := json.MarshalIndent(txns, "", "  ")
					if err != nil {
						return fmt.Errorf("marshaling response: %w", err)
					}
					fmt.Println(string(output))
				} else {
					printCardTransactions(txns)
				}
			} else {
				// Account: use history API
				fmt.Fprintln(os.Stderr, "Fetching account history...")
				history, err := client.GetAccountHistory(accessToken, id, size, page)
				if err != nil {
					return fmt.Errorf("fetching account history: %w", err)
				}
				if getJsonOutput {
					output, err := json.MarshalIndent(history, "", "  ")
					if err != nil {
						return fmt.Errorf("marshaling response: %w", err)
					}
					fmt.Println(string(output))
				} else {
					printAccountHistory(history)
				}
			}

			return nil
		},
	}
	getCmd.Flags().IntVarP(&size, "size", "s", 50, "Number of transactions to fetch")
	getCmd.Flags().IntVarP(&page, "page", "p", 0, "Page number (0-indexed)")
	getCmd.Flags().BoolVarP(&getJsonOutput, "json", "j", false, "Output as JSON")
	getCmd.Flags().BoolVarP(&forceAccountAPI, "account", "a", false, "Use account history API (even for cards)")
	rootCmd.AddCommand(getCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
