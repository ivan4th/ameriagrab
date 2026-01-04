package db

import (
	"fmt"
	"sort"
	"time"

	"github.com/ivan4th/ameriagrab/client"
)

// RealTimeTransactionTypes are linked account transaction types with accurate timestamps
var RealTimeTransactionTypes = map[string]bool{
	"transfer:to-card":              true,
	"transfer:local":                true,
	"exchange":                      true,
	"cash-out":                      true,
	"transfer:between-own-accounts": true,
}

// CombinedTransactionsOptions configures GetCombinedTransactions behavior
type CombinedTransactionsOptions struct {
	Size            int
	Page            int
	IncludeExtended bool
	Ascending       bool
}

// GetCombinedTransactions merges card and linked account transactions.
// Card transactions are matched to linked account transactions by amount and time proximity.
// When matched, the linked account transaction (with extended info) is used.
// Unmatched linked transactions with real-time types are included; "card" type is excluded.
func (db *DB) GetCombinedTransactions(productID string, opts CombinedTransactionsOptions) ([]client.Transaction, int, error) {
	// Fetch ALL transactions from both sources (no pagination at DB level)
	cardTxns, err := db.GetCardTransactions(productID, 0, 0, false)
	if err != nil {
		return nil, 0, fmt.Errorf("fetching card transactions: %w", err)
	}

	linkedTxns, err := db.GetLinkedAccountTransactions(productID, 0, 0, opts.IncludeExtended, false)
	if err != nil {
		return nil, 0, fmt.Errorf("fetching linked account transactions: %w", err)
	}

	// Merge transactions
	combined := mergeTransactions(cardTxns, linkedTxns)

	// Sort by operation_date
	sortTransactions(combined, opts.Ascending)

	// Calculate total before pagination
	totalCount := len(combined)

	// Apply pagination
	if opts.Size > 0 {
		offset := opts.Page * opts.Size
		if offset >= len(combined) {
			combined = []client.Transaction{}
		} else {
			end := offset + opts.Size
			if end > len(combined) {
				end = len(combined)
			}
			combined = combined[offset:end]
		}
	}

	return combined, totalCount, nil
}

// mergeTransactions implements the matching algorithm
func mergeTransactions(cardTxns, linkedTxns []client.Transaction) []client.Transaction {
	const timeTolerance = time.Minute

	// Build lookup map for linked transactions by amount
	linkedByAmount := make(map[float64][]*client.Transaction)
	for i := range linkedTxns {
		amt := linkedTxns[i].Amount.Amount
		linkedByAmount[amt] = append(linkedByAmount[amt], &linkedTxns[i])
	}

	matchedLinked := make(map[string]bool) // key: id|operationDate
	var result []client.Transaction

	// Match card transactions to linked account transactions
	for _, cardTxn := range cardTxns {
		cardTime, err := time.Parse(time.RFC3339, cardTxn.OperationDate)
		if err != nil {
			// Can't parse time, keep card transaction as-is
			result = append(result, cardTxn)
			continue
		}

		candidates := linkedByAmount[cardTxn.Amount.Amount]
		var matched *client.Transaction
		var minDiff time.Duration = timeTolerance + 1

		for _, linked := range candidates {
			linkedTime, err := time.Parse(time.RFC3339, linked.OperationDate)
			if err != nil {
				continue
			}

			diff := cardTime.Sub(linkedTime)
			if diff < 0 {
				diff = -diff
			}

			if diff <= timeTolerance && diff < minDiff {
				matched = linked
				minDiff = diff
			}
		}

		if matched != nil {
			matchedLinked[TxnKey(matched.ID, matched.OperationDate)] = true
			result = append(result, *matched)
		} else {
			result = append(result, cardTxn)
		}
	}

	// Add unmatched linked transactions with real-time timestamps
	for _, linkedTxn := range linkedTxns {
		key := TxnKey(linkedTxn.ID, linkedTxn.OperationDate)
		if matchedLinked[key] {
			continue
		}

		// Skip "card" type transactions (delayed timestamps)
		if linkedTxn.TransactionType == "card" {
			continue
		}

		// Include if it's a known real-time type
		if RealTimeTransactionTypes[linkedTxn.TransactionType] {
			result = append(result, linkedTxn)
		}
		// Skip unknown types (conservative: only include known real-time types)
	}

	return result
}

// sortTransactions sorts by operation_date
func sortTransactions(txns []client.Transaction, ascending bool) {
	sort.Slice(txns, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, txns[i].OperationDate)
		tj, _ := time.Parse(time.RFC3339, txns[j].OperationDate)
		if ascending {
			return ti.Before(tj)
		}
		return ti.After(tj)
	})
}
