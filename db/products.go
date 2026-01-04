package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ivan4th/ameriagrab/client"
)

// UpsertProducts inserts or updates products in the database, preserving order
func (db *DB) UpsertProducts(products []client.ProductInfo) error {
	syncedAt := time.Now().Unix()

	return db.WithTransaction(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`
			INSERT INTO products (
				id, product_type, name, card_number, account_number,
				account_id, currency, balance, status, order_index, synced_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				product_type = excluded.product_type,
				name = excluded.name,
				card_number = excluded.card_number,
				account_number = excluded.account_number,
				account_id = excluded.account_id,
				currency = excluded.currency,
				balance = excluded.balance,
				status = excluded.status,
				order_index = excluded.order_index,
				synced_at = excluded.synced_at
		`)
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for i, p := range products {
			_, err := stmt.Exec(
				p.ID,
				p.ProductType,
				p.Name,
				nullString(p.CardNumber),
				nullString(p.AccountNumber),
				nullString(p.AccountID),
				p.Currency,
				p.Balance,
				p.Status,
				i, // order_index preserves API order
				syncedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to upsert product %s: %w", p.ID, err)
			}
		}
		return nil
	})
}

// GetProducts retrieves all products from the database in API order
func (db *DB) GetProducts() ([]client.ProductInfo, error) {
	rows, err := db.Query(`
		SELECT id, product_type, name, card_number, account_number,
			   account_id, currency, balance, status
		FROM products
		ORDER BY order_index
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []client.ProductInfo
	for rows.Next() {
		var p client.ProductInfo
		var cardNumber, accountNumber, accountID sql.NullString

		err := rows.Scan(
			&p.ID,
			&p.ProductType,
			&p.Name,
			&cardNumber,
			&accountNumber,
			&accountID,
			&p.Currency,
			&p.Balance,
			&p.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}

		p.CardNumber = cardNumber.String
		p.AccountNumber = accountNumber.String
		p.AccountID = accountID.String

		products = append(products, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating products: %w", err)
	}

	return products, nil
}

// GetProductByID retrieves a single product by ID
func (db *DB) GetProductByID(id string) (*client.ProductInfo, error) {
	var p client.ProductInfo
	var cardNumber, accountNumber, accountID sql.NullString

	err := db.QueryRow(`
		SELECT id, product_type, name, card_number, account_number,
			   account_id, currency, balance, status
		FROM products WHERE id = ?
	`, id).Scan(
		&p.ID,
		&p.ProductType,
		&p.Name,
		&cardNumber,
		&accountNumber,
		&accountID,
		&p.Currency,
		&p.Balance,
		&p.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query product: %w", err)
	}

	p.CardNumber = cardNumber.String
	p.AccountNumber = accountNumber.String
	p.AccountID = accountID.String

	return &p, nil
}

// nullString returns a sql.NullString for empty strings
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
