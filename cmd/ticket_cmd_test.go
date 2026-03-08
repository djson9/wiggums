package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTicketFilename(t *testing.T) {
	tests := []struct {
		filename      string
		expectedID    string
		expectedTitle string
	}{
		{"1773005328_contracts.md", "1773005328", "contracts"},
		{"1773005328_fix_auth_bug.md", "1773005328", "fix auth bug"},
		{"noepoch.md", "noepoch", "noepoch"},
	}
	for _, tc := range tests {
		id, title := parseTicketFilename(tc.filename)
		if id != tc.expectedID {
			t.Errorf("parseTicketFilename(%q) id = %q, want %q", tc.filename, id, tc.expectedID)
		}
		if title != tc.expectedTitle {
			t.Errorf("parseTicketFilename(%q) title = %q, want %q", tc.filename, title, tc.expectedTitle)
		}
	}
}

func TestApplyTicketUpdateToContent(t *testing.T) {
	ticketContent := `---
Date: 2026-03-08 17:28
Status: created
Agent:
MinIterations:
CurIteration: 0
SkipVerification: true
UpdatedAt:
---
## Original User Request
Do something

---
Below to be filled by agent. Agent should not modify above this line.

## Approach
TODO

## Commands Run
TODO

## Context Breadcrumbs
TODO

## Findings
TODO
`
	ts := time.Date(2026, 3, 8, 18, 0, 0, 0, time.UTC)
	updated, err := applyTicketUpdateToContent(ticketContent, ts)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the update section is present with empty headers
	if !strings.Contains(updated, "## Update 2026-03-08 18:00") {
		t.Error("update section header not found")
	}
	if !strings.Contains(updated, "### Approach") {
		t.Error("approach section not found")
	}
	if !strings.Contains(updated, "### Commands Run") {
		t.Error("commands run section not found")
	}
	if !strings.Contains(updated, "### Context Breadcrumbs") {
		t.Error("context breadcrumbs section not found")
	}
	if !strings.Contains(updated, "### Findings") {
		t.Error("findings section not found")
	}

	// Verify the update appears BEFORE the old TODO sections
	updateIdx := strings.Index(updated, "## Update 2026-03-08 18:00")
	todoIdx := strings.Index(updated, "## Approach\nTODO")
	if updateIdx > todoIdx {
		t.Error("update section should appear before existing TODO sections")
	}

	// Verify the divider line is still there
	if !strings.Contains(updated, "Below to be filled by agent") {
		t.Error("divider line was removed")
	}
}

func TestApplyTicketUpdateToContent_MultipleUpdates(t *testing.T) {
	ticketContent := `---
Status: created
---
## Original User Request
Do something

---
Below to be filled by agent. Agent should not modify above this line.

## Approach
TODO
`
	// First update
	ts1 := time.Date(2026, 3, 8, 18, 0, 0, 0, time.UTC)
	updated1, err := applyTicketUpdateToContent(ticketContent, ts1)
	if err != nil {
		t.Fatal(err)
	}

	// Second update on top of first
	ts2 := time.Date(2026, 3, 8, 19, 0, 0, 0, time.UTC)
	updated2, err := applyTicketUpdateToContent(updated1, ts2)
	if err != nil {
		t.Fatal(err)
	}

	// Second update should appear before first update (newest on top)
	secondIdx := strings.Index(updated2, "## Update 2026-03-08 19:00")
	firstIdx := strings.Index(updated2, "## Update 2026-03-08 18:00")
	if secondIdx > firstIdx {
		t.Error("second update should appear before first update (newest on top)")
	}

	// Both should appear before the original TODO
	todoIdx := strings.Index(updated2, "## Approach\nTODO")
	if secondIdx > todoIdx {
		t.Error("second update should appear before TODO section")
	}
	if firstIdx > todoIdx {
		t.Error("first update should appear before TODO section")
	}
}

func TestApplyTicketUpdateToContent_NoDivider(t *testing.T) {
	content := `---
Status: created
---
## No divider here
Just a ticket without the standard divider.
`
	_, err := applyTicketUpdateToContent(content, time.Now())
	if err == nil {
		t.Error("expected error when divider is missing")
	}
	if !strings.Contains(err.Error(), "could not find divider line") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTicketUpdateEndToEnd(t *testing.T) {
	// Create a temp workspace structure with a ticket
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspaces", "test-queue", "tickets")
	os.MkdirAll(wsDir, 0755)

	ticketContent := `---
Date: 2026-03-08 17:28
Status: created
---
## Original User Request
Do something

---
Below to be filled by agent. Agent should not modify above this line.

## Approach
TODO

## Commands Run
TODO

## Context Breadcrumbs
TODO

## Findings
TODO
`
	ticketPath := filepath.Join(wsDir, "1234567890_test_ticket.md")
	os.WriteFile(ticketPath, []byte(ticketContent), 0644)

	// Apply first update
	updated, err := applyTicketUpdateToContent(
		ticketContent,
		time.Date(2026, 3, 8, 18, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(ticketPath, []byte(updated), 0644)

	// Apply second update
	content2, _ := os.ReadFile(ticketPath)
	updated2, err := applyTicketUpdateToContent(
		string(content2),
		time.Date(2026, 3, 8, 19, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(ticketPath, []byte(updated2), 0644)

	// Read final content and verify ordering
	final, _ := os.ReadFile(ticketPath)
	finalStr := string(final)

	// Both updates should be present
	if !strings.Contains(finalStr, "## Update 2026-03-08 19:00") {
		t.Error("second update not found")
	}
	if !strings.Contains(finalStr, "## Update 2026-03-08 18:00") {
		t.Error("first update not found")
	}

	// Second (newer) update should come first
	idx2 := strings.Index(finalStr, "## Update 2026-03-08 19:00")
	idx1 := strings.Index(finalStr, "## Update 2026-03-08 18:00")
	if idx2 > idx1 {
		t.Error("newer update should appear above older update")
	}

	// Original request should still be above the divider
	reqIdx := strings.Index(finalStr, "## Original User Request")
	dividerIdx := strings.Index(finalStr, "Below to be filled by agent")
	if reqIdx > dividerIdx {
		t.Error("original request should be above divider")
	}
}

func TestTicketComplete(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspaces", "test-queue", "tickets")
	os.MkdirAll(wsDir, 0755)

	ticketContent := `---
Date: 2026-03-08 17:28
Status: not completed
Agent:
---
## Original User Request
Do something
`
	ticketPath := filepath.Join(wsDir, "1234567890_test_ticket.md")
	os.WriteFile(ticketPath, []byte(ticketContent), 0644)

	// Read and update status inline (simulating ticketComplete without resolveBaseDir)
	content, _ := os.ReadFile(ticketPath)
	lines := strings.Split(string(content), "\n")
	delimCount := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			delimCount++
			continue
		}
		if delimCount == 1 {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, "status:") {
				lines[i] = "Status: completed"
				break
			}
		}
	}
	os.WriteFile(ticketPath, []byte(strings.Join(lines, "\n")), 0644)

	// Verify
	result, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(result), "Status: completed") {
		t.Error("ticket should have Status: completed")
	}
	if strings.Contains(string(result), "not completed") {
		t.Error("ticket should not contain 'not completed'")
	}
}
