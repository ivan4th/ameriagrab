package output

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ivan4th/ameriagrab/client"
	"github.com/ivan4th/ameriagrab/db"
)

// TruncateString truncates a string to maxLen characters, adding "..." if truncated
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// TemplateLookupFunc is a function that looks up a template name by masked card number
type TemplateLookupFunc func(maskedCard string) string

// PrintCardTransactions prints card transactions in human-readable table format
func PrintCardTransactions(txns *client.TransactionsResponse, showExtended, wide bool) {
	PrintCardTransactionsWithLookup(txns, showExtended, wide, nil)
}

// PrintCardTransactionsWithLookup prints card transactions with optional template name lookup
func PrintCardTransactionsWithLookup(txns *client.TransactionsResponse, showExtended, wide bool, lookupFn TemplateLookupFunc) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if showExtended {
		fmt.Fprintln(w, "DATE\tTYPE\tAMOUNT\tDETAILS\tCOUNTERPARTY")
	} else {
		fmt.Fprintln(w, "DATE\tTYPE\tAMOUNT\tDETAILS")
	}
	for _, t := range txns.Data.Entries {
		// Format amount with +/- sign based on accounting type
		sign := "-"
		if t.AccountingType == "CREDIT" {
			sign = "+"
		}
		amount := fmt.Sprintf("%s%.2f %s", sign, t.Amount.Amount, t.Amount.Currency)

		// Parse and format date
		date := t.Date
		if t.OperationDate != "" {
			if parsed, err := time.Parse(time.RFC3339, t.OperationDate); err == nil {
				date = parsed.Format("2006-01-02 15:04")
			}
		}

		// Shorten transaction type for display
		txType := t.TransactionType
		txType = strings.ReplaceAll(txType, "pre-purchase:", "prep:")
		txType = strings.ReplaceAll(txType, "purchasecompletion:", "pcomp:")
		txType = strings.ReplaceAll(txType, "purchase:", "p:")

		details := t.Details
		if !wide {
			if showExtended {
				details = TruncateString(details, 40)
			} else {
				details = TruncateString(details, 50)
			}
		}

		if showExtended {
			receiver := formatReceiverWithLookup(t.Extended, lookupFn)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				date, txType, amount, details, receiver)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				date, txType, amount, details)
		}
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "\nTotal: %d transactions\n", txns.Data.TotalCount)
}

// formatReceiverWithLookup formats the receiver info, using template lookup if available
func formatReceiverWithLookup(ext *client.TransactionExtendedInfo, lookupFn TemplateLookupFunc) string {
	if ext == nil {
		return ""
	}

	// Get card or account number
	cardNum := ext.CardMaskedNumber
	acctNum := ext.CreditAccountNumber

	// Check if name is valid (not empty or placeholder)
	name := ext.BeneficiaryName
	hasValidName := name != "" && name != "Firstname Lastname"

	// Try template lookup for card number if no valid beneficiary name
	if !hasValidName && lookupFn != nil && cardNum != "" {
		if templateName := lookupFn(cardNum); templateName != "" {
			// Found template: show "TemplateName (****1234)"
			shortCard := shortenMaskedCard(cardNum)
			return fmt.Sprintf("%s (%s)", templateName, shortCard)
		}
	}

	// Fall back to original logic
	acctOrCard := cardNum
	if acctOrCard == "" {
		acctOrCard = acctNum
	}

	if hasValidName && acctOrCard != "" {
		return fmt.Sprintf("%s (%s)", name, acctOrCard)
	}
	if hasValidName {
		return name
	}
	if acctOrCard != "" {
		return acctOrCard
	}
	return ""
}

// shortenMaskedCard shortens "4454********6615" to "****6615"
func shortenMaskedCard(masked string) string {
	if len(masked) < 4 {
		return masked
	}
	return "****" + masked[len(masked)-4:]
}

// PrintAccountHistory prints account history in human-readable table format
func PrintAccountHistory(history *client.HistoryResponse, wide bool) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tTYPE\tAMOUNT\tBENEFICIARY\tDETAILS")
	for _, t := range history.Data.Transactions {
		// Format amount with +/- sign based on flow direction
		sign := "-"
		if t.FlowDirection == "INCOME" {
			sign = "+"
		}
		amount := fmt.Sprintf("%s%.2f %s", sign, t.TransactionAmount.Value, t.TransactionAmount.Currency)

		// Format date from timestamp
		date := t.Date
		if t.TransactionDate > 0 {
			date = time.UnixMilli(t.TransactionDate).Format("2006-01-02 15:04")
		}

		// Shorten transaction type for display
		txType := t.TransactionType
		txType = strings.ReplaceAll(txType, "transfer:", "xfer:")

		beneficiary := t.BeneficiaryName
		details := t.Details
		if !wide {
			beneficiary = TruncateString(beneficiary, 30)
			details = TruncateString(details, 40)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			date, txType, amount, beneficiary, details)
	}
	w.Flush()
	if history.Data.HasNext {
		fmt.Fprintln(os.Stderr, "\n(more transactions available, use --page to paginate)")
	}
}

// PrintAccountsAndCards prints accounts and cards in human-readable table format
func PrintAccountsAndCards(resp *client.AccountsAndCardsResponse) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tID\tNUMBER\tNAME\tCURRENCY\tBALANCE\tSTATUS")
	for _, p := range resp.Data.AccountsAndCards {
		number := p.CardNumber
		if p.ProductType == "ACCOUNT" {
			number = p.AccountNumber
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%.2f\t%s\n",
			p.ProductType, p.ID, number, p.Name, p.Currency, p.AvailableBalance, p.Status)
	}
	w.Flush()
}

// PrintSnapshots prints snapshots grouped by date in human-readable format
func PrintSnapshots(snapshots []db.Snapshot) {
	for i, s := range snapshots {
		// Print date header
		fmt.Printf("=== %s ===\n", s.CreatedAt.Format("2006-01-02 15:04:05"))

		// Print products table
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TYPE\tID\tNUMBER\tNAME\tCURRENCY\tBALANCE\tSTATUS")
		for _, p := range s.Products {
			number := p.CardNumber
			if p.ProductType == "ACCOUNT" {
				number = p.AccountNumber
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%.2f\t%s\n",
				p.ProductType, p.ID, number, p.Name, p.Currency, p.AvailableBalance, p.Status)
		}
		w.Flush()

		// Add blank line between snapshots (except after the last one)
		if i < len(snapshots)-1 {
			fmt.Println()
		}
	}
}
