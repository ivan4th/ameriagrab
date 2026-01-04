package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ivan4th/ameriagrab/client"
)

// InsertAccountTransactions inserts account transactions, ignoring duplicates
func (db *DB) InsertAccountTransactions(productID string, txns []client.AccountTransaction) (int, error) {
	syncedAt := time.Now().Unix()
	var inserted int

	err := db.WithTransaction(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO account_transactions (
				id, product_id, transaction_id, operation_id, status,
				transaction_type, workflow_code, flow_direction,
				transaction_date, settled_date, date, month, year,
				debit_account_number, credit_account_number,
				beneficiary_name, details, source_system,
				transaction_amount_currency, transaction_amount_value,
				settled_amount_currency, settled_amount_value,
				domestic_amount_currency, domestic_amount_value,
				synced_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, t := range txns {
			result, err := stmt.Exec(
				t.ID,
				productID,
				t.TransactionID,
				t.OperationID,
				t.Status,
				t.TransactionType,
				t.WorkflowCode,
				t.FlowDirection,
				t.TransactionDate,
				t.SettledDate,
				t.Date,
				t.Month,
				t.Year,
				t.DebitAccountNumber,
				t.CreditAccountNumber,
				t.BeneficiaryName,
				t.Details,
				t.SourceSystem,
				t.TransactionAmount.Currency,
				t.TransactionAmount.Value,
				t.SettledAmount.Currency,
				t.SettledAmount.Value,
				t.DomesticAmount.Currency,
				t.DomesticAmount.Value,
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

// GetAccountTransactions retrieves all account transactions for a product.
// If ascending is true, returns oldest first; otherwise newest first.
func (db *DB) GetAccountTransactions(productID string, ascending bool) ([]client.AccountTransaction, error) {
	order := "DESC"
	if ascending {
		order = "ASC"
	}

	rows, err := db.Query(fmt.Sprintf(`
		SELECT id, transaction_id, operation_id, status,
			   transaction_type, workflow_code, flow_direction,
			   transaction_date, settled_date, date, month, year,
			   debit_account_number, credit_account_number,
			   beneficiary_name, details, source_system,
			   transaction_amount_currency, transaction_amount_value,
			   settled_amount_currency, settled_amount_value,
			   domestic_amount_currency, domestic_amount_value
		FROM account_transactions
		WHERE product_id = ?
		ORDER BY transaction_date %s
	`, order), productID)
	if err != nil {
		return nil, fmt.Errorf("failed to query account transactions: %w", err)
	}
	defer rows.Close()

	var txns []client.AccountTransaction
	for rows.Next() {
		var t client.AccountTransaction
		var txnAmtCurrency, settledAmtCurrency, domesticAmtCurrency sql.NullString
		var txnAmtValue, settledAmtValue, domesticAmtValue sql.NullFloat64

		err := rows.Scan(
			&t.ID,
			&t.TransactionID,
			&t.OperationID,
			&t.Status,
			&t.TransactionType,
			&t.WorkflowCode,
			&t.FlowDirection,
			&t.TransactionDate,
			&t.SettledDate,
			&t.Date,
			&t.Month,
			&t.Year,
			&t.DebitAccountNumber,
			&t.CreditAccountNumber,
			&t.BeneficiaryName,
			&t.Details,
			&t.SourceSystem,
			&txnAmtCurrency,
			&txnAmtValue,
			&settledAmtCurrency,
			&settledAmtValue,
			&domesticAmtCurrency,
			&domesticAmtValue,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}

		t.TransactionAmount = client.TransactionAmt{
			Currency: txnAmtCurrency.String,
			Value:    txnAmtValue.Float64,
		}
		t.SettledAmount = client.TransactionAmt{
			Currency: settledAmtCurrency.String,
			Value:    settledAmtValue.Float64,
		}
		t.DomesticAmount = client.TransactionAmt{
			Currency: domesticAmtCurrency.String,
			Value:    domesticAmtValue.Float64,
		}

		txns = append(txns, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transactions: %w", err)
	}

	return txns, nil
}

// GetExistingAccountTxnIDs returns a set of existing transaction IDs for a product
func (db *DB) GetExistingAccountTxnIDs(productID string) (map[string]bool, error) {
	rows, err := db.Query(`
		SELECT id FROM account_transactions WHERE product_id = ?
	`, productID)
	if err != nil {
		return nil, fmt.Errorf("failed to query transaction IDs: %w", err)
	}
	defer rows.Close()

	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan ID: %w", err)
		}
		ids[id] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating IDs: %w", err)
	}

	return ids, nil
}

// CountAccountTransactions returns the total count of account transactions for a product
func (db *DB) CountAccountTransactions(productID string) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM account_transactions WHERE product_id = ?
	`, productID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}
	return count, nil
}
