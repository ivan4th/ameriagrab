package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ivan4th/ameriagrab/client"
)

// InsertCardTransactions inserts card transactions (from GetTransactions), ignoring duplicates
func (db *DB) InsertCardTransactions(productID string, txns []client.Transaction) (int, error) {
	syncedAt := time.Now().Unix()
	var inserted int

	err := db.WithTransaction(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO card_transactions (
				id, product_id, transaction_type, accounting_type, state,
				amount_currency, amount_value, correspondent_account_number,
				correspondent_account_name, details, operation_date,
				workflow_code, date, year, month, synced_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, t := range txns {
			result, err := stmt.Exec(
				t.ID,
				productID,
				t.TransactionType,
				t.AccountingType,
				t.State,
				t.Amount.Currency,
				t.Amount.Amount,
				t.CorrespondentAccountNumber,
				t.CorrespondentAccountName,
				t.Details,
				t.OperationDate,
				t.WorkflowCode,
				t.Date,
				t.Year,
				t.Month,
				syncedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to insert transaction %s: %w", t.ID, err)
			}
			rowsAffected, _ := result.RowsAffected()
			if rowsAffected > 0 {
				inserted++
			}
		}
		return nil
	})

	return inserted, err
}

// GetCardTransactions retrieves card transactions for a product with optional pagination.
// If size is 0, returns all transactions. Otherwise returns up to size transactions starting at page.
// If ascending is true, returns oldest first; otherwise newest first.
func (db *DB) GetCardTransactions(productID string, size, page int, ascending bool) ([]client.Transaction, error) {
	var rows *sql.Rows
	var err error

	order := "DESC"
	if ascending {
		order = "ASC"
	}

	if size == 0 {
		// No limit - return all
		rows, err = db.Query(fmt.Sprintf(`
			SELECT id, transaction_type, accounting_type, state,
				   amount_currency, amount_value, correspondent_account_number,
				   correspondent_account_name, details, operation_date,
				   workflow_code, date, year, month
			FROM card_transactions
			WHERE product_id = ?
			ORDER BY operation_date %s
		`, order), productID)
	} else {
		// Paginated query
		offset := page * size
		rows, err = db.Query(fmt.Sprintf(`
			SELECT id, transaction_type, accounting_type, state,
				   amount_currency, amount_value, correspondent_account_number,
				   correspondent_account_name, details, operation_date,
				   workflow_code, date, year, month
			FROM card_transactions
			WHERE product_id = ?
			ORDER BY operation_date %s
			LIMIT ? OFFSET ?
		`, order), productID, size, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query card transactions: %w", err)
	}
	defer rows.Close()

	var txns []client.Transaction
	for rows.Next() {
		var t client.Transaction
		var currency sql.NullString
		var amount sql.NullFloat64

		err := rows.Scan(
			&t.ID,
			&t.TransactionType,
			&t.AccountingType,
			&t.State,
			&currency,
			&amount,
			&t.CorrespondentAccountNumber,
			&t.CorrespondentAccountName,
			&t.Details,
			&t.OperationDate,
			&t.WorkflowCode,
			&t.Date,
			&t.Year,
			&t.Month,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}

		t.Amount = client.Amount{
			Currency: currency.String,
			Amount:   amount.Float64,
		}

		txns = append(txns, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transactions: %w", err)
	}

	return txns, nil
}

// TxnKey creates a composite key from transaction ID and operation date
func TxnKey(id, operationDate string) string {
	return id + "|" + operationDate
}

// GetExistingCardTxnKeys returns a set of existing transaction keys (id|operation_date) for a product
func (db *DB) GetExistingCardTxnKeys(productID string) (map[string]bool, error) {
	rows, err := db.Query(`
		SELECT id, operation_date FROM card_transactions WHERE product_id = ?
	`, productID)
	if err != nil {
		return nil, fmt.Errorf("failed to query transaction keys: %w", err)
	}
	defer rows.Close()

	keys := make(map[string]bool)
	for rows.Next() {
		var id, operationDate string
		if err := rows.Scan(&id, &operationDate); err != nil {
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}
		keys[TxnKey(id, operationDate)] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating keys: %w", err)
	}

	return keys, nil
}

// CountCardTransactions returns the total count of card transactions for a product
func (db *DB) CountCardTransactions(productID string) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM card_transactions WHERE product_id = ?
	`, productID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}
	return count, nil
}
