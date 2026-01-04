package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GetTransactions fetches settled transactions for a card
func (c *Client) GetTransactions(accessToken, cardID string) (*TransactionsResponse, error) {
	txnURL := fmt.Sprintf("%s/api/events/settled/%s", c.APIBaseURL, cardID)

	req, err := http.NewRequest("GET", txnURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactions request: %w", err)
	}
	c.AddAPIHeaders(req, accessToken)

	resp, err := c.HTTPClient.Do(req)
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
func (c *Client) GetAccountsAndCards(accessToken string) (*AccountsAndCardsResponse, error) {
	url := fmt.Sprintf("%s/api/accounts-and-cards?page=0&size=100&skipApplications=false&specifications=SIMPLE&isFullList=true", c.APIBaseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create accounts-and-cards request: %w", err)
	}
	c.AddAPIHeaders(req, accessToken)

	resp, err := c.HTTPClient.Do(req)
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
func (c *Client) GetAccountHistory(accessToken, accountID string, size, page int) (*HistoryResponse, error) {
	url := fmt.Sprintf("%s/api/history?accountIds=%s&size=%d&page=%d", c.APIBaseURL, accountID, size, page)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create history request: %w", err)
	}
	c.AddAPIHeaders(req, accessToken)

	resp, err := c.HTTPClient.Do(req)
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
func (c *Client) GetEventsPast(accessToken, accountID string, size, page int) (*TransactionsResponse, error) {
	url := fmt.Sprintf("%s/api/events/past?locale=ru&fromAmount=0.1&accountIds=%s&sort=date&size=%d&page=%d",
		c.APIBaseURL, accountID, size, page)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create events/past request: %w", err)
	}
	c.AddAPIHeaders(req, accessToken)

	resp, err := c.HTTPClient.Do(req)
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

// GetTransactionDetails fetches extended info for a transaction by its UUID
func (c *Client) GetTransactionDetails(accessToken, transactionID string) (*TransactionDetailsResponse, error) {
	url := fmt.Sprintf("%s/api/transactions/%s", c.APIBaseURL, transactionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction details request: %w", err)
	}
	c.AddAPIHeaders(req, accessToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transaction details: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read transaction details response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transaction details request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result TransactionDetailsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse transaction details response: %w", err)
	}

	return &result, nil
}

// GetAvailableBalance fetches the available balance for a product (card or account)
func (c *Client) GetAvailableBalance(accessToken, productType, productID string) (*AvailableBalanceResponse, error) {
	url := fmt.Sprintf("%s/api/accounts-and-cards/available-balance?productType=%s&productId=%s",
		c.APIBaseURL, productType, productID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create available-balance request: %w", err)
	}
	c.AddAPIHeaders(req, accessToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch available-balance: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read available-balance response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("available-balance request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result AvailableBalanceResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse available-balance response: %w", err)
	}

	return &result, nil
}

// GetTemplates fetches transfer templates
func (c *Client) GetTemplates(accessToken string) (*TemplatesResponse, error) {
	url := fmt.Sprintf("%s/api/templates?page=1&size=1000&hasGroup=false", c.APIBaseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create templates request: %w", err)
	}
	c.AddAPIHeaders(req, accessToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch templates: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read templates response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("templates request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var templatesResult TemplatesResponse
	if err := json.Unmarshal(body, &templatesResult); err != nil {
		return nil, fmt.Errorf("failed to parse templates response: %w", err)
	}

	return &templatesResult, nil
}
