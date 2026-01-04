package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ivan4th/ameriagrab/client"
)

// InsertLinkedAccountTransactions inserts card linked account transactions (from GetEventsPast), ignoring duplicates
func (db *DB) InsertLinkedAccountTransactions(productID string, txns []client.Transaction) (int, error) {
	syncedAt := time.Now().Unix()
	var inserted int

	err := db.WithTransaction(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO card_linked_account_transactions (
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

// GetLinkedAccountTransactions retrieves card linked account transactions for a product with optional pagination.
// If size is 0, returns all transactions. Otherwise returns up to size transactions starting at page.
// If includeExtended is true, also loads extended info for transactions that have it.
func (db *DB) GetLinkedAccountTransactions(productID string, size, page int, includeExtended bool) ([]client.Transaction, error) {
	var rows *sql.Rows
	var err error

	cols := `id, transaction_type, accounting_type, state,
			 amount_currency, amount_value, correspondent_account_number,
			 correspondent_account_name, details, operation_date,
			 workflow_code, date, year, month`
	if includeExtended {
		cols += `, beneficiary_name, beneficiary_address, credit_account_number,
				  card_masked_number, ext_operation_id, swift_details, extended_fetched`
	}

	if size == 0 {
		// No limit - return all
		rows, err = db.Query(fmt.Sprintf(`
			SELECT %s
			FROM card_linked_account_transactions
			WHERE product_id = ?
			ORDER BY operation_date DESC
		`, cols), productID)
	} else {
		// Paginated query
		offset := page * size
		rows, err = db.Query(fmt.Sprintf(`
			SELECT %s
			FROM card_linked_account_transactions
			WHERE product_id = ?
			ORDER BY operation_date DESC
			LIMIT ? OFFSET ?
		`, cols), productID, size, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query linked account transactions: %w", err)
	}
	defer rows.Close()

	var txns []client.Transaction
	for rows.Next() {
		var t client.Transaction
		var currency sql.NullString
		var amount sql.NullFloat64

		if includeExtended {
			var beneficiaryName, beneficiaryAddress, creditAccountNumber sql.NullString
			var cardMaskedNumber, extOperationID, swiftDetails sql.NullString
			var extendedFetched sql.NullInt64

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
				&beneficiaryName,
				&beneficiaryAddress,
				&creditAccountNumber,
				&cardMaskedNumber,
				&extOperationID,
				&swiftDetails,
				&extendedFetched,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to scan transaction: %w", err)
			}

			// Only set Extended if we actually fetched it
			if extendedFetched.Valid && extendedFetched.Int64 == 1 {
				t.Extended = &client.TransactionExtendedInfo{
					BeneficiaryName:     beneficiaryName.String,
					BeneficiaryAddress:  beneficiaryAddress.String,
					CreditAccountNumber: creditAccountNumber.String,
					CardMaskedNumber:    cardMaskedNumber.String,
					OperationID:         extOperationID.String,
					SwiftDetails:        swiftDetails.String,
				}
			}
		} else {
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

// GetExistingLinkedAccountTxnKeys returns a set of existing transaction keys (id|operation_date) for a product
func (db *DB) GetExistingLinkedAccountTxnKeys(productID string) (map[string]bool, error) {
	rows, err := db.Query(`
		SELECT id, operation_date FROM card_linked_account_transactions WHERE product_id = ?
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

// CountLinkedAccountTransactions returns the total count of linked account transactions for a product
func (db *DB) CountLinkedAccountTransactions(productID string) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM card_linked_account_transactions WHERE product_id = ?
	`, productID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}
	return count, nil
}

// UpdateTransactionExtendedInfo updates extended info for a linked account transaction
func (db *DB) UpdateTransactionExtendedInfo(productID, txnID, operationDate string, ext *client.TransactionExtendedInfo) error {
	_, err := db.Exec(`
		UPDATE card_linked_account_transactions
		SET beneficiary_name = ?,
			beneficiary_address = ?,
			credit_account_number = ?,
			card_masked_number = ?,
			ext_operation_id = ?,
			swift_details = ?,
			extended_fetched = 1
		WHERE product_id = ? AND id = ? AND operation_date = ?
	`, ext.BeneficiaryName, ext.BeneficiaryAddress, ext.CreditAccountNumber,
		ext.CardMaskedNumber, ext.OperationID, ext.SwiftDetails,
		productID, txnID, operationDate)
	if err != nil {
		return fmt.Errorf("failed to update extended info: %w", err)
	}
	return nil
}

// GetTransactionsNeedingExtendedInfo returns transactions that haven't had extended info fetched yet
func (db *DB) GetTransactionsNeedingExtendedInfo(productID string) ([]client.Transaction, error) {
	rows, err := db.Query(`
		SELECT id, operation_date
		FROM card_linked_account_transactions
		WHERE product_id = ? AND (extended_fetched IS NULL OR extended_fetched = 0)
	`, productID)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions needing extended info: %w", err)
	}
	defer rows.Close()

	var txns []client.Transaction
	for rows.Next() {
		var t client.Transaction
		if err := rows.Scan(&t.ID, &t.OperationDate); err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		txns = append(txns, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transactions: %w", err)
	}

	return txns, nil
}
