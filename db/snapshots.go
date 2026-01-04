package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ivan4th/ameriagrab/client"
)

// Snapshot represents a point-in-time balance snapshot
type Snapshot struct {
	ID        int64
	CreatedAt time.Time
	Products  []client.ProductInfo
}

// CreateSnapshot creates a new snapshot by copying current product data
func (db *DB) CreateSnapshot() (int64, error) {
	createdAt := time.Now().Unix()

	var snapshotID int64
	err := db.WithTransaction(func(tx *sql.Tx) error {
		// Create snapshot record
		result, err := tx.Exec(`INSERT INTO snapshots (created_at) VALUES (?)`, createdAt)
		if err != nil {
			return fmt.Errorf("failed to create snapshot: %w", err)
		}

		snapshotID, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get snapshot ID: %w", err)
		}

		// Copy current product data to snapshot_products
		_, err = tx.Exec(`
			INSERT INTO snapshot_products (
				snapshot_id, product_id, product_type, name,
				card_number, account_number, currency, balance, status, order_index
			)
			SELECT ?, id, product_type, name,
				   card_number, account_number, currency, balance, status, order_index
			FROM products
			ORDER BY order_index
		`, snapshotID)
		if err != nil {
			return fmt.Errorf("failed to copy products to snapshot: %w", err)
		}

		return nil
	})

	return snapshotID, err
}

// GetSnapshots returns all snapshots with their products, in ascending chronological order
func (db *DB) GetSnapshots() ([]Snapshot, error) {
	// First get all snapshots
	rows, err := db.Query(`SELECT id, created_at FROM snapshots ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var s Snapshot
		var createdAt int64
		if err := rows.Scan(&s.ID, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan snapshot: %w", err)
		}
		s.CreatedAt = time.Unix(createdAt, 0)
		snapshots = append(snapshots, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating snapshots: %w", err)
	}

	// Then load products for each snapshot
	for i := range snapshots {
		products, err := db.getSnapshotProducts(snapshots[i].ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get products for snapshot %d: %w", snapshots[i].ID, err)
		}
		snapshots[i].Products = products
	}

	return snapshots, nil
}

// getSnapshotProducts retrieves products for a specific snapshot
func (db *DB) getSnapshotProducts(snapshotID int64) ([]client.ProductInfo, error) {
	rows, err := db.Query(`
		SELECT product_id, product_type, name, card_number, account_number,
			   currency, balance, status
		FROM snapshot_products
		WHERE snapshot_id = ?
		ORDER BY order_index
	`, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("failed to query snapshot products: %w", err)
	}
	defer rows.Close()

	var products []client.ProductInfo
	for rows.Next() {
		var p client.ProductInfo
		var cardNumber, accountNumber sql.NullString
		var balance sql.NullFloat64

		err := rows.Scan(
			&p.ID, &p.ProductType, &p.Name,
			&cardNumber, &accountNumber,
			&p.Currency, &balance, &p.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}

		p.CardNumber = cardNumber.String
		p.AccountNumber = accountNumber.String
		p.Balance = balance.Float64

		products = append(products, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating products: %w", err)
	}

	return products, nil
}

// CountSnapshots returns the number of snapshots in the database
func (db *DB) CountSnapshots() (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM snapshots`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count snapshots: %w", err)
	}
	return count, nil
}
