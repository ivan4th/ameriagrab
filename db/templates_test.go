package db

import (
	"testing"

	"github.com/ivan4th/ameriagrab/client"
)

func TestExtractCardKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "template format (first 4, last 4 visible)",
			input:    "4454********6615",
			expected: "4454615",
		},
		{
			name:     "transaction format (first 5, last 3 visible)",
			input:    "44543********615",
			expected: "4454615",
		},
		{
			name:     "full card number",
			input:    "4454300012346615",
			expected: "4454615",
		},
		{
			name:     "short string",
			input:    "123456",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only asterisks",
			input:    "****************",
			expected: "",
		},
		{
			name:     "spaces and dashes",
			input:    "4454-****-****-6615",
			expected: "4454615",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCardKey(tt.input)
			if result != tt.expected {
				t.Errorf("extractCardKey(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func makeTemplate(id, name, number, targetType, beneficiary string) client.TransferTemplate {
	t := client.TransferTemplate{
		ID:           id,
		Name:         name,
		WorkflowCode: "LIME_TRANSFER_TO_CARD",
	}
	t.Data.CreditTarget.Number = number
	t.Data.CreditTarget.Type = targetType
	t.Data.Beneficiary = beneficiary
	return t
}

func TestUpsertTemplates_Insert(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	templates := []client.TransferTemplate{
		makeTemplate("id1", "Alice", "4454********6615", "CARD", ""),
		makeTemplate("id2", "Bob", "5555********1234", "CARD", ""),
	}

	if err := db.UpsertTemplates(templates); err != nil {
		t.Fatalf("UpsertTemplates failed: %v", err)
	}

	count, err := db.CountTemplates()
	if err != nil {
		t.Fatalf("CountTemplates failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 templates, got %d", count)
	}

	// Verify lookup works
	name, err := db.GetTemplateByMaskedCard("4454********6615")
	if err != nil {
		t.Fatalf("GetTemplateByMaskedCard failed: %v", err)
	}
	if name != "Alice" {
		t.Errorf("expected name 'Alice', got %q", name)
	}
}

func TestUpsertTemplates_Update(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Initial insert
	templates1 := []client.TransferTemplate{
		makeTemplate("id1", "OldName", "4454********6615", "CARD", ""),
	}
	if err := db.UpsertTemplates(templates1); err != nil {
		t.Fatalf("initial UpsertTemplates failed: %v", err)
	}

	// Verify initial name
	name, _ := db.GetTemplateByMaskedCard("4454********6615")
	if name != "OldName" {
		t.Errorf("expected initial name 'OldName', got %q", name)
	}

	// Update with modified name (same ID, different name)
	templates2 := []client.TransferTemplate{
		makeTemplate("id1", "NewName", "4454********6615", "CARD", ""),
	}
	if err := db.UpsertTemplates(templates2); err != nil {
		t.Fatalf("update UpsertTemplates failed: %v", err)
	}

	// Verify update applied
	name, _ = db.GetTemplateByMaskedCard("4454********6615")
	if name != "NewName" {
		t.Errorf("expected updated name 'NewName', got %q", name)
	}

	// Count should still be 1
	count, _ := db.CountTemplates()
	if count != 1 {
		t.Errorf("expected 1 template after update, got %d", count)
	}
}

func TestUpsertTemplates_Delete(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Initial insert with two templates
	templates1 := []client.TransferTemplate{
		makeTemplate("id1", "Alice", "4454********6615", "CARD", ""),
		makeTemplate("id2", "Bob", "5555********1234", "CARD", ""),
	}
	if err := db.UpsertTemplates(templates1); err != nil {
		t.Fatalf("initial UpsertTemplates failed: %v", err)
	}

	count, _ := db.CountTemplates()
	if count != 2 {
		t.Errorf("expected 2 templates initially, got %d", count)
	}

	// Sync with only one template (Bob is deleted on bank side)
	templates2 := []client.TransferTemplate{
		makeTemplate("id1", "Alice", "4454********6615", "CARD", ""),
	}
	if err := db.UpsertTemplates(templates2); err != nil {
		t.Fatalf("second UpsertTemplates failed: %v", err)
	}

	// Bob should be gone
	count, _ = db.CountTemplates()
	if count != 1 {
		t.Errorf("expected 1 template after deletion, got %d", count)
	}

	name, _ := db.GetTemplateByMaskedCard("5555********1234")
	if name != "" {
		t.Errorf("expected Bob to be deleted, but got name %q", name)
	}

	// Alice should still exist
	name, _ = db.GetTemplateByMaskedCard("4454********6615")
	if name != "Alice" {
		t.Errorf("expected Alice to remain, got %q", name)
	}
}

func TestUpsertTemplates_Mixed(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Initial state: Alice, Bob, Charlie
	templates1 := []client.TransferTemplate{
		makeTemplate("id1", "Alice", "4454********6615", "CARD", ""),
		makeTemplate("id2", "Bob", "5555********1234", "CARD", ""),
		makeTemplate("id3", "Charlie", "6666********5678", "CARD", ""),
	}
	if err := db.UpsertTemplates(templates1); err != nil {
		t.Fatalf("initial UpsertTemplates failed: %v", err)
	}

	// Mixed operation:
	// - Alice: name changed to "Alice Updated"
	// - Bob: deleted
	// - Charlie: unchanged
	// - Diana: added
	templates2 := []client.TransferTemplate{
		makeTemplate("id1", "Alice Updated", "4454********6615", "CARD", ""),
		makeTemplate("id3", "Charlie", "6666********5678", "CARD", ""),
		makeTemplate("id4", "Diana", "7777********9999", "CARD", ""),
	}
	if err := db.UpsertTemplates(templates2); err != nil {
		t.Fatalf("mixed UpsertTemplates failed: %v", err)
	}

	// Verify count
	count, _ := db.CountTemplates()
	if count != 3 {
		t.Errorf("expected 3 templates after mixed operation, got %d", count)
	}

	// Verify Alice updated
	name, _ := db.GetTemplateByMaskedCard("4454********6615")
	if name != "Alice Updated" {
		t.Errorf("expected 'Alice Updated', got %q", name)
	}

	// Verify Bob deleted
	name, _ = db.GetTemplateByMaskedCard("5555********1234")
	if name != "" {
		t.Errorf("expected Bob deleted, got %q", name)
	}

	// Verify Charlie unchanged
	name, _ = db.GetTemplateByMaskedCard("6666********5678")
	if name != "Charlie" {
		t.Errorf("expected 'Charlie', got %q", name)
	}

	// Verify Diana added
	name, _ = db.GetTemplateByMaskedCard("7777********9999")
	if name != "Diana" {
		t.Errorf("expected 'Diana', got %q", name)
	}
}

func TestGetTemplateByMaskedCard_CrossFormatMatching(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert template with template format (first 4, last 4 visible)
	templates := []client.TransferTemplate{
		makeTemplate("id1", "Alice", "4454********6615", "CARD", ""),
	}
	if err := db.UpsertTemplates(templates); err != nil {
		t.Fatalf("UpsertTemplates failed: %v", err)
	}

	// Lookup using transaction format (first 5, last 3 visible)
	// This simulates how transactions store card numbers differently
	name, err := db.GetTemplateByMaskedCard("44543********615")
	if err != nil {
		t.Fatalf("GetTemplateByMaskedCard failed: %v", err)
	}
	if name != "Alice" {
		t.Errorf("cross-format matching failed: expected 'Alice', got %q", name)
	}
}

func TestGetTemplateByMaskedCard_NotFound(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Empty database - should return empty string, not error
	name, err := db.GetTemplateByMaskedCard("4454********6615")
	if err != nil {
		t.Fatalf("GetTemplateByMaskedCard failed: %v", err)
	}
	if name != "" {
		t.Errorf("expected empty string for not found, got %q", name)
	}
}

func TestGetTemplateByMaskedCard_EmptyInput(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	name, err := db.GetTemplateByMaskedCard("")
	if err != nil {
		t.Fatalf("GetTemplateByMaskedCard failed: %v", err)
	}
	if name != "" {
		t.Errorf("expected empty string for empty input, got %q", name)
	}
}

func TestGetTemplateByAccount(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert account template
	templates := []client.TransferTemplate{
		makeTemplate("id1", "Company Account", "1570012345678900", "ACCOUNT", "Company Inc."),
	}
	if err := db.UpsertTemplates(templates); err != nil {
		t.Fatalf("UpsertTemplates failed: %v", err)
	}

	// Lookup by account number
	name, err := db.GetTemplateByAccount("1570012345678900")
	if err != nil {
		t.Fatalf("GetTemplateByAccount failed: %v", err)
	}
	if name != "Company Account" {
		t.Errorf("expected 'Company Account', got %q", name)
	}
}

func TestGetTemplateByAccount_NotFound(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	name, err := db.GetTemplateByAccount("nonexistent")
	if err != nil {
		t.Fatalf("GetTemplateByAccount failed: %v", err)
	}
	if name != "" {
		t.Errorf("expected empty string for not found, got %q", name)
	}
}

func TestGetTemplateByAccount_EmptyInput(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	name, err := db.GetTemplateByAccount("")
	if err != nil {
		t.Fatalf("GetTemplateByAccount failed: %v", err)
	}
	if name != "" {
		t.Errorf("expected empty string for empty input, got %q", name)
	}
}

func TestUpsertTemplates_Empty(t *testing.T) {
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert some templates first
	templates1 := []client.TransferTemplate{
		makeTemplate("id1", "Alice", "4454********6615", "CARD", ""),
	}
	if err := db.UpsertTemplates(templates1); err != nil {
		t.Fatalf("initial UpsertTemplates failed: %v", err)
	}

	// Sync with empty list (all templates deleted on bank side)
	if err := db.UpsertTemplates([]client.TransferTemplate{}); err != nil {
		t.Fatalf("empty UpsertTemplates failed: %v", err)
	}

	count, _ := db.CountTemplates()
	if count != 0 {
		t.Errorf("expected 0 templates after empty sync, got %d", count)
	}
}
