package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCheckPreprocessingComplete(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "no preprocessing section",
			content: "---\nStatus: created\n---\n# My Ticket\nSome content",
			want:    false,
		},
		{
			name:    "has preprocessing section with content",
			content: "---\nStatus: created\n---\n# My Ticket\nSome content\n\n## Preprocessing\nResearch results here",
			want:    true,
		},
		{
			name:    "has preprocessing heading but no content",
			content: "---\nStatus: created\n---\n# My Ticket\n\n## Preprocessing\n",
			want:    false,
		},
		{
			name:    "has preprocessing heading with only whitespace after",
			content: "---\nStatus: created\n---\n# My Ticket\n\n## Preprocessing\n   \n  \n",
			want:    false,
		},
		{
			name:    "has preprocessing section with actual content after whitespace",
			content: "---\nStatus: created\n---\n# My Ticket\n\n## Preprocessing\n\nFound relevant data:\n- Item 1",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "ticket.md")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			got := checkPreprocessingComplete(tmpFile)
			if got != tt.want {
				t.Errorf("checkPreprocessingComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckPreprocessingComplete_MissingFile(t *testing.T) {
	got := checkPreprocessingComplete("/nonexistent/path.md")
	if got {
		t.Error("checkPreprocessingComplete should return false for missing file")
	}
}

func TestBuildPreprocessPrompt(t *testing.T) {
	baseDir := t.TempDir()

	// Create a prompt template
	promptsDir := filepath.Join(baseDir, "prompts")
	os.MkdirAll(promptsDir, 0755)
	template := "# Preprocess\nDir: {{WIGGUMS_DIR}}\nQueue: {{QUEUE_PATH}}\n"
	os.WriteFile(filepath.Join(promptsDir, "preprocess.md"), []byte(template), 0644)

	tickets := []preprocessTicket{
		{
			index: 0,
			ticket: QueueTicket{
				Path:             "/tmp/tickets/001_test.md",
				Workspace:        "my-workspace",
				PreprocessPrompt: "Research this topic",
			},
		},
		{
			index: 1,
			ticket: QueueTicket{
				Path:             "/tmp/tickets/002_other.md",
				Workspace:        "other-ws",
				PreprocessPrompt: "Gather data",
			},
		},
	}

	queuePath := "/tmp/queue.json"
	prompt := buildPreprocessPrompt(baseDir, tickets, queuePath)

	// Check template substitution
	if !contains(prompt, baseDir) {
		t.Errorf("prompt should contain baseDir %q", baseDir)
	}
	if !contains(prompt, queuePath) {
		t.Errorf("prompt should contain queuePath %q", queuePath)
	}
	// Check ticket list
	if !contains(prompt, "001_test.md") {
		t.Error("prompt should contain first ticket path")
	}
	if !contains(prompt, "Research this topic") {
		t.Error("prompt should contain first ticket instruction")
	}
	if !contains(prompt, "002_other.md") {
		t.Error("prompt should contain second ticket path")
	}
	if !contains(prompt, "Gather data") {
		t.Error("prompt should contain second ticket instruction")
	}
}

func TestBuildPreprocessPrompt_FallbackTemplate(t *testing.T) {
	// Empty dir with no prompts/preprocess.md
	baseDir := t.TempDir()
	tickets := []preprocessTicket{
		{
			index: 0,
			ticket: QueueTicket{
				Path:             "/tmp/test.md",
				Workspace:        "ws",
				PreprocessPrompt: "Do something",
			},
		},
	}
	prompt := buildPreprocessPrompt(baseDir, tickets, "/tmp/q.json")
	if !contains(prompt, "preprocessing agent") {
		t.Error("fallback template should contain 'preprocessing agent'")
	}
	if !contains(prompt, "Do something") {
		t.Error("prompt should contain the ticket instruction")
	}
}

func TestPreprocessFields_BuildQueueFileFromModel(t *testing.T) {
	m := newTestTuiModelWithItems([]list.Item{})
	m.defaultPreprocessPrompt = "Research all the things"

	// Add a queue item with preprocessing fields
	item := tuiTicketItem{
		title:            "Test",
		status:           "created",
		filePath:         "/tmp/test.md",
		workspace:        "test-ws",
		workerStatus:     "pending",
		preprocessPrompt: "Gather context",
		preprocessStatus: "pending",
	}
	m.queue.SetItems([]list.Item{item})

	qf := buildQueueFileFromModel(&m)

	if qf.DefaultPreprocessPrompt != "Research all the things" {
		t.Errorf("DefaultPreprocessPrompt = %q, want %q", qf.DefaultPreprocessPrompt, "Research all the things")
	}
	if len(qf.Tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(qf.Tickets))
	}
	if qf.Tickets[0].PreprocessPrompt != "Gather context" {
		t.Errorf("PreprocessPrompt = %q, want %q", qf.Tickets[0].PreprocessPrompt, "Gather context")
	}
	if qf.Tickets[0].PreprocessStatus != "pending" {
		t.Errorf("PreprocessStatus = %q, want %q", qf.Tickets[0].PreprocessStatus, "pending")
	}
}

func TestPreprocessFields_QueueFileRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.json")

	qf := &QueueFile{
		Name:                    "Test Queue",
		DefaultPreprocessPrompt: "Default instruction",
		Tickets: []QueueTicket{
			{
				Path:             "/tmp/ticket.md",
				Workspace:        "ws",
				Status:           "pending",
				PreprocessPrompt: "Research this",
				PreprocessStatus: "pending",
			},
			{
				Path:             "/tmp/ticket2.md",
				Workspace:        "ws",
				Status:           "pending",
				PreprocessPrompt: "",
				PreprocessStatus: "",
			},
		},
	}

	// Write
	if err := writeQueueFileDataToPath(qf, queuePath); err != nil {
		t.Fatal(err)
	}

	// Read back
	loaded, err := readQueueFileFromPath(queuePath)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.DefaultPreprocessPrompt != "Default instruction" {
		t.Errorf("DefaultPreprocessPrompt = %q, want %q", loaded.DefaultPreprocessPrompt, "Default instruction")
	}
	if loaded.Tickets[0].PreprocessPrompt != "Research this" {
		t.Errorf("Ticket[0].PreprocessPrompt = %q, want %q", loaded.Tickets[0].PreprocessPrompt, "Research this")
	}
	if loaded.Tickets[0].PreprocessStatus != "pending" {
		t.Errorf("Ticket[0].PreprocessStatus = %q, want %q", loaded.Tickets[0].PreprocessStatus, "pending")
	}
	if loaded.Tickets[1].PreprocessPrompt != "" {
		t.Errorf("Ticket[1].PreprocessPrompt should be empty, got %q", loaded.Tickets[1].PreprocessPrompt)
	}
}

func TestMergePreprocessStatuses(t *testing.T) {
	tests := []struct {
		name           string
		tuiStatus      string
		existingStatus string
		wantStatus     string
	}{
		{"worker completed beats pending", "pending", "completed", "completed"},
		{"worker in_progress beats pending", "pending", "in_progress", "in_progress"},
		{"tui pending stays if worker empty", "pending", "", "pending"},
		{"worker completed beats in_progress", "in_progress", "completed", "completed"},
		{"tui completed stays if worker empty", "completed", "", "completed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qf := &QueueFile{
				Tickets: []QueueTicket{
					{Path: "/tmp/t.md", PreprocessStatus: tt.tuiStatus},
				},
			}
			existing := &QueueFile{
				Tickets: []QueueTicket{
					{Path: "/tmp/t.md", PreprocessStatus: tt.existingStatus},
				},
			}
			mergePreprocessStatuses(qf, existing)
			if qf.Tickets[0].PreprocessStatus != tt.wantStatus {
				t.Errorf("got %q, want %q", qf.Tickets[0].PreprocessStatus, tt.wantStatus)
			}
		})
	}
}

func TestMergePreprocessStatuses_NilExisting(t *testing.T) {
	qf := &QueueFile{
		Tickets: []QueueTicket{
			{Path: "/tmp/t.md", PreprocessStatus: "pending"},
		},
	}
	mergePreprocessStatuses(qf, nil)
	if qf.Tickets[0].PreprocessStatus != "pending" {
		t.Errorf("should not modify when existing is nil")
	}
}

func TestTUI_PKey_OpensPreprocessPrompt(t *testing.T) {
	// Set up a queue with one item
	item := tuiTicketItem{
		title:    "Test Ticket",
		status:   "created",
		filePath: "/tmp/test.md",
	}
	m := newTestTuiModelWithItems([]list.Item{})
	m.queue.SetItems([]list.Item{item})
	m.tab = tuiTabQueue
	m.queue.Select(0)
	m.defaultPreprocessPrompt = "Previous instruction"
	// Initialize textarea (required by the p key handler)
	ta := textarea.New()
	ta.SetWidth(60)
	ta.SetHeight(5)
	m.textArea = ta

	// Press 'p' on Queue tab
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	updated := newModel.(tuiModel)

	if updated.mode != tuiModePreprocessPrompt {
		t.Errorf("mode = %d, want tuiModePreprocessPrompt (%d)", updated.mode, tuiModePreprocessPrompt)
	}

	// Text area should be pre-filled with default prompt
	if updated.textArea.Value() != "Previous instruction" {
		t.Errorf("textArea value = %q, want %q", updated.textArea.Value(), "Previous instruction")
	}
}

func TestTUI_PreprocessPrompt_CtrlS_Applies(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.json")

	// Create initial queue file
	qf := &QueueFile{Name: "Test"}
	writeQueueFileDataToPath(qf, queuePath)

	item := tuiTicketItem{
		title:    "Test Ticket",
		status:   "created",
		filePath: "/tmp/test.md",
	}
	m := newTestTuiModelWithItems([]list.Item{})
	m.queue.SetItems([]list.Item{item})
	m.queue.Select(0)
	m.tab = tuiTabQueue
	m.mode = tuiModePreprocessPrompt
	m.activeQueueID = filepath.Base(tmpDir) // so writeQueueFile uses our path
	m.textArea = textarea.New()
	m.textArea.SetValue("Research this topic")

	// Press ctrl+s — but we can't easily write to the real queue path in a test,
	// so just verify the model state changes
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	updated := newModel.(tuiModel)

	if updated.mode != tuiModeList {
		t.Errorf("mode should return to tuiModeList after ctrl+s, got %d", updated.mode)
	}
	if updated.defaultPreprocessPrompt != "Research this topic" {
		t.Errorf("defaultPreprocessPrompt = %q, want %q", updated.defaultPreprocessPrompt, "Research this topic")
	}

	// Check the queue item was updated
	qi := updated.queue.Items()
	if len(qi) != 1 {
		t.Fatalf("expected 1 queue item, got %d", len(qi))
	}
	qItem := qi[0].(tuiTicketItem)
	if qItem.preprocessPrompt != "Research this topic" {
		t.Errorf("preprocessPrompt = %q, want %q", qItem.preprocessPrompt, "Research this topic")
	}
	if qItem.preprocessStatus != "pending" {
		t.Errorf("preprocessStatus = %q, want %q", qItem.preprocessStatus, "pending")
	}
}

func TestTUI_PreprocessPrompt_Esc_Cancels(t *testing.T) {
	m := newTestTuiModelWithItems([]list.Item{})
	m.mode = tuiModePreprocessPrompt
	m.textArea = textarea.New()
	m.textArea.SetValue("some instruction")

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := newModel.(tuiModel)

	if updated.mode != tuiModeList {
		t.Errorf("mode should return to tuiModeList after esc, got %d", updated.mode)
	}
}

func TestTUI_PreprocessStatusIndicator(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"pending", " pp"},
		{"in_progress", " pp..."},
		{"completed", " pp:✓"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run("status_"+tt.status, func(t *testing.T) {
			item := tuiTicketItem{
				title:            "Test",
				status:           "created",
				preprocessStatus: tt.status,
			}
			title := item.Title()
			if tt.want == "" {
				// Should not contain any pp indicator
				if contains(title, " pp") {
					t.Errorf("Title() = %q, should not contain pp indicator", title)
				}
			} else {
				if !contains(title, tt.want) {
					t.Errorf("Title() = %q, should contain %q", title, tt.want)
				}
			}
		})
	}
}

func TestPreprocessFields_OmitEmptyJSON(t *testing.T) {
	qt := QueueTicket{
		Path:      "/tmp/test.md",
		Workspace: "ws",
		Status:    "pending",
	}
	data, err := json.Marshal(qt)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if contains(s, "preprocess_prompt") {
		t.Error("empty PreprocessPrompt should be omitted from JSON")
	}
	if contains(s, "preprocess_status") {
		t.Error("empty PreprocessStatus should be omitted from JSON")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsStr(s, substr)))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
