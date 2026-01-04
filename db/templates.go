package db

import (
	"database/sql"
	"time"

	"github.com/ivan4th/ameriagrab/client"
)

// extractCardKey extracts first 4 + last 3 digits from a masked card number
// Using last 3 because transaction masks show different digits than template masks:
// - Template: "4454********6615" shows first 4 + last 4
// - Transaction: "44543********615" shows first 5 + last 3
// Both represent the same card, so we use first 4 + last 3 for matching
func extractCardKey(masked string) string {
	if len(masked) < 7 {
		return ""
	}
	// Extract only digits from the string
	var digits []byte
	for i := 0; i < len(masked); i++ {
		if masked[i] >= '0' && masked[i] <= '9' {
			digits = append(digits, masked[i])
		}
	}
	if len(digits) < 7 {
		return ""
	}
	// Return first 4 + last 3 digits
	return string(digits[:4]) + string(digits[len(digits)-3:])
}

// UpsertTemplates replaces all templates with the new set
func (db *DB) UpsertTemplates(templates []client.TransferTemplate) error {
	return db.WithTransaction(func(tx *sql.Tx) error {
		// Delete all existing templates
		if _, err := tx.Exec("DELETE FROM transfer_templates"); err != nil {
			return err
		}

		// Insert new templates
		stmt, err := tx.Prepare(`
			INSERT INTO transfer_templates (
				id, name, workflow_code, masked_card_number, account_number, beneficiary, card_key, synced_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		now := time.Now().Unix()
		for _, t := range templates {
			maskedCard := t.Data.CreditTarget.Number
			var accountNumber string
			var cardKey string
			if t.Data.CreditTarget.Type == "ACCOUNT" {
				accountNumber = t.Data.CreditTarget.Number
				maskedCard = ""
			} else {
				cardKey = extractCardKey(maskedCard)
			}

			_, err := stmt.Exec(
				t.ID,
				t.Name,
				t.WorkflowCode,
				maskedCard,
				accountNumber,
				t.Data.Beneficiary,
				cardKey,
				now,
			)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// GetTemplateByMaskedCard looks up a template by masked card number using card_key matching
func (db *DB) GetTemplateByMaskedCard(maskedCard string) (string, error) {
	if maskedCard == "" {
		return "", nil
	}

	// Normalize the input card number to card_key format (first 4 + last 4 digits)
	cardKey := extractCardKey(maskedCard)
	if cardKey == "" {
		return "", nil
	}

	var name string
	err := db.QueryRow(
		"SELECT name FROM transfer_templates WHERE card_key = ?",
		cardKey,
	).Scan(&name)

	if err != nil {
		// No template found is not an error
		return "", nil
	}

	return name, nil
}

// GetTemplateByAccount looks up a template by account number
func (db *DB) GetTemplateByAccount(accountNumber string) (string, error) {
	if accountNumber == "" {
		return "", nil
	}

	var name string
	err := db.QueryRow(
		"SELECT name FROM transfer_templates WHERE account_number = ?",
		accountNumber,
	).Scan(&name)

	if err != nil {
		// No template found is not an error
		return "", nil
	}

	return name, nil
}

// CountTemplates returns the number of stored templates
func (db *DB) CountTemplates() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM transfer_templates").Scan(&count)
	return count, err
}
