package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Login performs the full OAuth login flow with push notification 2FA
func (c *Client) Login() (string, error) {
	// Step 1: Get the login page to extract the action URL
	state := uuid.New().String()
	nonce := uuid.New().String()

	authURL := fmt.Sprintf(
		"%s/auth/realms/ameria/protocol/openid-connect/auth?client_id=%s&redirect_uri=%s&state=%s&response_mode=fragment&response_type=code&scope=openid&nonce=%s&kc_locale=ru",
		c.AuthBaseURL,
		OAuthClient,
		url.QueryEscape(RedirectURI),
		state,
		nonce,
	)

	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}
	c.AddFirefoxHeaders(req)
	req.Header.Set("Referer", RedirectURI)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Sec-Fetch-User", "?1")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get login page: %w", err)
	}
	defer resp.Body.Close()

	fmt.Fprintf(os.Stderr, "Debug: Login page status: %d\n", resp.StatusCode)
	for _, cookie := range resp.Cookies() {
		fmt.Fprintf(os.Stderr, "Debug: Set-Cookie: %s=%s...\n", cookie.Name, cookie.Value[:Min(20, len(cookie.Value))])
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read login page: %w", err)
	}

	// Extract actionUrl from the login page
	actionURLRegex := regexp.MustCompile(`actionUrl:\s*"([^"]+)"`)
	matches := actionURLRegex.FindSubmatch(body)
	if len(matches) < 2 {
		c.SaveDebugFile("login_page.html", body)
		return "", fmt.Errorf("failed to find actionUrl in login page")
	}
	actionURL := string(matches[1])
	fmt.Fprintf(os.Stderr, "Debug: Action URL: %s\n", actionURL)

	// Step 2: Submit login credentials
	formData := url.Values{}
	formData.Set("username", c.Username)
	formData.Set("password", c.Password)
	formData.Set("X-Banqr-CDDC", BuildCDDCHeader())
	formData.Set("remember", "on")

	encodedForm := formData.Encode()
	fmt.Fprintf(os.Stderr, "Debug: Form data (length %d)\n", len(encodedForm))

	req, err = http.NewRequest("POST", actionURL, strings.NewReader(encodedForm))
	if err != nil {
		return "", fmt.Errorf("failed to create login request: %w", err)
	}
	c.AddFirefoxHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.AuthBaseURL)
	req.Header.Set("Referer", authURL)
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Priority", "u=0, i")

	resp, err = c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to submit login: %w", err)
	}
	defer resp.Body.Close()

	fmt.Fprintf(os.Stderr, "Debug: Login POST status: %d\n", resp.StatusCode)
	for _, cookie := range resp.Cookies() {
		fmt.Fprintf(os.Stderr, "Debug: Post Set-Cookie: %s=%s...\n", cookie.Name, cookie.Value[:Min(20, len(cookie.Value))])
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
		c.SaveDebugFile("debug_response.html", body)
		return "", fmt.Errorf("failed to find push session ID in response. Login may have failed. Response preview: %s", string(body[:Min(1000, len(body))]))
	}
	pushSessionID := string(matches[1])

	// Extract evaluatedRequestId for the second POST
	evalReqIDRegex := regexp.MustCompile(`evaluatedRequestId:\s*\\?"([^"\\]+)`)
	evalMatches := evalReqIDRegex.FindSubmatch(body)
	if len(evalMatches) < 2 {
		c.SaveDebugFile("debug_response.html", body)
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
	pushFormData.Set("X-Banqr-CDDC", BuildCDDCHeader())
	pushFormData.Set("evaluate_action", "true")
	pushFormData.Set("form_action", "submit_button")
	pushFormData.Set("totp", pushSessionID)

	fmt.Fprintf(os.Stderr, "Debug: Push form - evaluated_request_id=%s, totp=%s\n", evaluatedRequestID, pushSessionID)

	req, err = http.NewRequest("POST", pushActionURL, strings.NewReader(pushFormData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create push confirmation request: %w", err)
	}
	c.AddFirefoxHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.AuthBaseURL)
	req.Header.Set("Referer", actionURL)
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Priority", "u=0, i")

	resp, err = c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to submit push confirmation: %w", err)
	}
	defer resp.Body.Close()

	fmt.Fprintf(os.Stderr, "Debug: Push confirmation POST status: %d\n", resp.StatusCode)

	// Check for redirect with authorization code
	location := resp.Header.Get("Location")
	if location == "" {
		body, _ := io.ReadAll(resp.Body)
		c.SaveDebugFile("push_response.html", body)
		return "", fmt.Errorf("expected redirect after push confirmation, got status %d. Preview: %s", resp.StatusCode, string(body[:Min(500, len(body))]))
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

func (c *Client) waitForPushConfirmation(sessionID string) error {
	pushStatusURL := fmt.Sprintf("%s/push-status?sessionId=%s", c.AuthBaseURL, sessionID)

	startTime := time.Now()
	for {
		if time.Since(startTime) > PollTimeout {
			return fmt.Errorf("push confirmation timed out after %v", PollTimeout)
		}

		req, err := http.NewRequest("GET", pushStatusURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create push status request: %w", err)
		}
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")

		resp, err := c.HTTPClient.Do(req)
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
			time.Sleep(PollInterval)
		case "rejected":
			return fmt.Errorf("push notification was rejected")
		case "expired":
			return fmt.Errorf("push notification expired")
		default:
			return fmt.Errorf("unexpected push status: %s", status.Data.SessionStatus)
		}
	}
}

func (c *Client) exchangeCodeForToken(code string) (string, error) {
	tokenURL := fmt.Sprintf("%s/auth/realms/ameria/protocol/openid-connect/token", c.AuthBaseURL)

	formData := url.Values{}
	formData.Set("code", code)
	formData.Set("grant_type", "authorization_code")
	formData.Set("redirect_uri", RedirectURI)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	// Add Basic auth header
	auth := base64.StdEncoding.EncodeToString([]byte(OAuthClient + ":" + ClientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", RedirectURI[:len(RedirectURI)-1])
	req.Header.Set("Referer", RedirectURI)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")

	resp, err := c.HTTPClient.Do(req)
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
	if err := c.SaveSession(tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save session: %v\n", err)
	}

	return tokenResp.AccessToken, nil
}

// InitializeSession fetches the real client ID and sets up the session
func (c *Client) InitializeSession(accessToken string) error {
	fmt.Fprintf(os.Stderr, "Debug: Initializing session...\n")

	// Step 1: Get user info to get user ID
	req, err := http.NewRequest("GET", c.APIBaseURL+"/api/users/info", nil)
	if err != nil {
		return fmt.Errorf("failed to create user info request: %w", err)
	}
	c.AddAPIHeaders(req, accessToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("user info request failed with status %d: %s", resp.StatusCode, string(body[:Min(200, len(body))]))
	}

	var userInfo UserInfoResponse
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return fmt.Errorf("failed to parse user info: %w", err)
	}
	userID := userInfo.Data.UserInfo.ID
	fmt.Fprintf(os.Stderr, "Debug: User ID: %s\n", userID)

	// Step 2: Get clients to find the real Client-Id
	req, err = http.NewRequest("GET", fmt.Sprintf("%s/api/users/%s/clients", c.APIBaseURL, userID), nil)
	if err != nil {
		return fmt.Errorf("failed to create clients request: %w", err)
	}
	c.AddAPIHeaders(req, accessToken)

	resp, err = c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch clients: %w", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("clients request failed with status %d: %s", resp.StatusCode, string(body[:Min(200, len(body))]))
	}

	var clientsResp ClientsResponse
	if err := json.Unmarshal(body, &clientsResp); err != nil {
		return fmt.Errorf("failed to parse clients: %w", err)
	}

	// Find the default client or use the first one
	for _, client := range clientsResp.Data.Clients {
		if client.Default {
			c.ClientID = client.ID
			break
		}
	}
	if c.ClientID == "" && len(clientsResp.Data.Clients) > 0 {
		c.ClientID = clientsResp.Data.Clients[0].ID
	}
	if c.ClientID == "" {
		return fmt.Errorf("no client found in clients response")
	}

	fmt.Fprintf(os.Stderr, "Debug: Client ID set to: %s\n", c.ClientID)
	fmt.Fprintf(os.Stderr, "Debug: Session initialized successfully\n")
	return nil
}
