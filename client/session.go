package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// GetSessionURLs returns the URLs for which we want to save/restore cookies
func GetSessionURLs() []*url.URL {
	return GetSessionURLsWithBases(AuthBaseURL, APIBaseURL)
}

// GetSessionURLsWithBases returns session URLs using custom base URLs (for testing)
func GetSessionURLsWithBases(authBase, apiBase string) []*url.URL {
	authURL, _ := url.Parse(authBase)
	apiURL, _ := url.Parse(apiBase)
	mainURL, _ := url.Parse(RedirectURI)
	return []*url.URL{authURL, apiURL, mainURL}
}

// SaveSession saves the current session (cookies + tokens) to disk
func (c *Client) SaveSession(accessToken, refreshToken string, expiresIn int) error {
	if c.SessionFile == "" {
		return nil // Session persistence disabled
	}

	session := SessionData{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		ClientID:     c.ClientID,
		Cookies:      []SerializedCookie{},
	}

	// Collect cookies from all relevant URLs
	for _, u := range GetSessionURLs() {
		for _, cookie := range c.HTTPClient.Jar.Cookies(u) {
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
	if err := os.WriteFile(c.SessionFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Debug: Session saved to %s\n", c.SessionFile)
	return nil
}

// UpdateSessionClientID updates the saved session with the current clientID
func (c *Client) UpdateSessionClientID() error {
	if c.SessionFile == "" {
		return nil // Session persistence disabled
	}

	data, err := os.ReadFile(c.SessionFile)
	if err != nil {
		return fmt.Errorf("failed to read session file: %w", err)
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("failed to parse session: %w", err)
	}

	session.ClientID = c.ClientID

	data, err = json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(c.SessionFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Debug: Session updated with Client ID: %s\n", c.ClientID)
	return nil
}

// LoadSession attempts to load a saved session from disk
func (c *Client) LoadSession() (*SessionData, error) {
	if c.SessionFile == "" {
		return nil, nil // Session persistence disabled
	}

	data, err := os.ReadFile(c.SessionFile)
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
	for _, u := range GetSessionURLs() {
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
			c.HTTPClient.Jar.SetCookies(u, cookies)
		}
	}

	// Restore client ID
	if session.ClientID != "" {
		c.ClientID = session.ClientID
		fmt.Fprintf(os.Stderr, "Debug: Restored Client ID: %s\n", c.ClientID)
	}

	fmt.Fprintf(os.Stderr, "Debug: Session loaded from %s (expires at %s)\n", c.SessionFile, session.ExpiresAt.Format(time.RFC3339))
	return &session, nil
}

// ValidateSession checks if the saved session is still valid by making a test API call
func (c *Client) ValidateSession(accessToken string) bool {
	// Use a short timeout to detect killed server-side sessions quickly
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use /api/users/info - same endpoint that InitializeSession uses successfully
	req, err := http.NewRequestWithContext(ctx, "GET", c.APIBaseURL+"/api/users/info", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Debug: validateSession request creation failed: %v\n", err)
		return false
	}

	// Use full API headers (requires clientID to be set)
	c.AddAPIHeaders(req, accessToken)

	resp, err := c.HTTPClient.Do(req)
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
func (c *Client) GetOrRefreshToken() (string, error) {
	// Try to load saved session
	session, err := c.LoadSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Debug: Error loading session: %v\n", err)
	}

	if session != nil {
		fmt.Fprintf(os.Stderr, "Debug: Found saved session, validating...\n")
		if c.ValidateSession(session.AccessToken) {
			fmt.Fprintf(os.Stderr, "Debug: Saved session is valid, reusing\n")
			return session.AccessToken, nil
		}
		fmt.Fprintf(os.Stderr, "Debug: Saved session is invalid, need fresh login\n")
		// Clear clientID so InitializeSession runs after fresh login
		c.ClientID = ""
	}

	// Perform fresh login
	return c.Login()
}
