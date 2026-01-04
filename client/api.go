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
