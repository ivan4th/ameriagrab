package db

import (
	"encoding/json"
	"time"

	"github.com/ivan4th/ameriagrab/client"
)

// SaveSession saves session data to the database
func (db *DB) SaveSession(data *client.SessionData) error {
	cookiesJSON, err := json.Marshal(data.Cookies)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT OR REPLACE INTO session (id, access_token, refresh_token, expires_at, client_id, cookies_json, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
	`, data.AccessToken, data.RefreshToken, data.ExpiresAt.Unix(), data.ClientID, string(cookiesJSON), time.Now().Unix())
	return err
}

// LoadSession loads session data from the database
func (db *DB) LoadSession() (*client.SessionData, error) {
	var accessToken, refreshToken, clientID string
	var expiresAt int64
	var cookiesJSON string

	err := db.QueryRow(`
		SELECT access_token, refresh_token, expires_at, client_id, cookies_json
		FROM session WHERE id = 1
	`).Scan(&accessToken, &refreshToken, &expiresAt, &clientID, &cookiesJSON)

	if err != nil {
		// No session found
		return nil, nil
	}

	var cookies []client.SerializedCookie
	if cookiesJSON != "" {
		if err := json.Unmarshal([]byte(cookiesJSON), &cookies); err != nil {
			return nil, err
		}
	}

	return &client.SessionData{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Unix(expiresAt, 0),
		ClientID:     clientID,
		Cookies:      cookies,
	}, nil
}

// UpdateClientID updates just the client ID in the saved session
func (db *DB) UpdateClientID(clientID string) error {
	_, err := db.Exec(`
		UPDATE session SET client_id = ?, updated_at = ? WHERE id = 1
	`, clientID, time.Now().Unix())
	return err
}
