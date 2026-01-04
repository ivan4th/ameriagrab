package client

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
)

// NewClient creates a new Ameriabank API client
func NewClient(username, password, sessionDir, debugDir string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	httpClient := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects automatically for the final authenticate step
			if strings.Contains(req.URL.String(), "myameria.am/#") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	c := &Client{
		HTTPClient:  httpClient,
		Username:    username,
		Password:    password,
		ClientID:    "", // Will be set by InitializeSession or restored from saved session
		APIBaseURL:  APIBaseURL,
		AuthBaseURL: AuthBaseURL,
	}

	// Set up session directory with restricted permissions (if provided)
	if sessionDir != "" {
		if err := os.MkdirAll(sessionDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create session directory: %w", err)
		}
		c.SessionDir = sessionDir
		c.SessionFile = filepath.Join(sessionDir, "session.json")
		fmt.Fprintf(os.Stderr, "Debug: Session persistence enabled at %s\n", c.SessionFile)
	} else {
		fmt.Fprintf(os.Stderr, "Debug: Session persistence disabled (AMERIA_SESSION_DIR not set)\n")
	}

	// Set up debug directory (if provided)
	if debugDir != "" {
		if err := os.MkdirAll(debugDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create debug directory: %w", err)
		}
		c.DebugDir = debugDir
		fmt.Fprintf(os.Stderr, "Debug: Debug file output enabled at %s\n", c.DebugDir)
	}

	return c, nil
}

// SaveDebugFile saves content to a debug file if debug dir is configured
func (c *Client) SaveDebugFile(filename string, content []byte) {
	if c.DebugDir == "" {
		return
	}
	path := filepath.Join(c.DebugDir, filename)
	if err := os.WriteFile(path, content, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save debug file %s: %v\n", path, err)
	} else {
		fmt.Fprintf(os.Stderr, "Debug: Saved debug file to %s\n", path)
	}
}

// Min returns the minimum of two integers
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
