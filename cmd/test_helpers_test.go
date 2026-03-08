package cmd

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wiggums/database"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// --- Mocks ---

type mockRunner struct {
	calls    []string // prompts received
	exitCode int
	err      error
	// onRun is called during Run if set, allowing tests to mutate ticket
	// files mid-loop (e.g. simulate claude marking a ticket completed).
	onRun func()
}

func (m *mockRunner) Run(ctx context.Context, prompt string, args []string) (int, error) {
	m.calls = append(m.calls, prompt)
	if m.onRun != nil {
		m.onRun()
	}
	return m.exitCode, m.err
}

type mockPromptLoader struct {
	result string
	calls  []mockPromptCall
}

type mockPromptCall struct {
	promptFile      string
	header          string
	tickets         []string
	agentPromptFile string
}

func (m *mockPromptLoader) Load(baseDir, promptFile, header string, tickets []string, agentPromptFile string) (string, error) {
	m.calls = append(m.calls, mockPromptCall{
		promptFile:      promptFile,
		header:          header,
		tickets:         tickets,
		agentPromptFile: agentPromptFile,
	})
	return m.result, nil
}

type mockNotifier struct {
	calls []string
}

func (m *mockNotifier) Notify(title, message, icon string) error {
	m.calls = append(m.calls, title+": "+message)
	return nil
}

// --- Helpers ---

func writeTicket(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readTicket(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func touchRecent(t *testing.T, path string) {
	t.Helper()
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
}

func touchOld(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func setupTestDB(t *testing.T) func() {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	database.DB = bun.NewDB(sqldb, sqlitedialect.New())
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate test DB: %v", err)
	}
	return func() {
		database.DB.Close()
		database.DB = nil
	}
}

// --- Frontmatter parsing tests ---

func TestExtractFrontmatterStatus(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{"completed", "---\nStatus: completed\n---\n", "Status: completed"},
		{"verified", "---\nStatus: completed + verified\n---\n", "Status: completed + verified"},
		{"created", "---\nStatus: created\n---\n", "Status: created"},
		{"no frontmatter", "just text", ""},
		{"no status", "---\nTitle: foo\n---\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFrontmatterStatus(tt.content)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestExtractFrontmatterAgent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"empty agent", "---\nStatus: created\nAgent:\n---\n", ""},
		{"agent set", "---\nStatus: created\nAgent: explore\n---\n", "explore"},
		{"agent with spaces", "---\nStatus: created\nAgent:  investigate \n---\n", "investigate"},
		{"no agent field", "---\nStatus: created\n---\n", ""},
		{"no frontmatter", "Just some text", ""},
		{"case insensitive", "---\nagent: Explore\n---\n", "Explore"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFrontmatterAgent(tt.content)
			if got != tt.want {
				t.Errorf("extractFrontmatterAgent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractFrontmatterInt(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		key      string
		expected int
	}{
		{"found", "---\nMinIterations: 5\n---\n", "MinIterations", 5},
		{"not found", "---\nStatus: created\n---\n", "MinIterations", 0},
		{"zero", "---\nMinIterations: 0\n---\n", "MinIterations", 0},
		{"invalid", "---\nMinIterations: abc\n---\n", "MinIterations", 0},
		{"double quoted", "---\nMinIterations: \"8\"\n---\n", "MinIterations", 8},
		{"single quoted", "---\nMinIterations: '3'\n---\n", "MinIterations", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFrontmatterInt(tt.content, tt.key)
			if got != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, got)
			}
		})
	}
}

func TestExtractFrontmatterBool(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		key      string
		expected bool
	}{
		{"true", "---\nSkipVerification: true\n---\n", "SkipVerification", true},
		{"false", "---\nSkipVerification: false\n---\n", "SkipVerification", false},
		{"yes", "---\nSkipVerification: yes\n---\n", "SkipVerification", true},
		{"one", "---\nSkipVerification: 1\n---\n", "SkipVerification", true},
		{"not found", "---\nStatus: created\n---\n", "SkipVerification", false},
		{"empty", "---\nSkipVerification:\n---\n", "SkipVerification", false},
		{"case insensitive key", "---\nskipverification: true\n---\n", "SkipVerification", true},
		{"case insensitive value", "---\nSkipVerification: True\n---\n", "SkipVerification", true},
		{"no frontmatter", "Just text", "SkipVerification", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFrontmatterBool(tt.content, tt.key)
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestMarkAsVerified(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: completed\n---\nBody\n")

	if err := markAsVerified(path); err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, path)
	if !strings.Contains(content, "Status: completed + verified") {
		t.Errorf("expected status to be 'completed + verified', got:\n%s", content)
	}
}

func TestIncrementCurIteration_AddsField(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: created\nMinIterations: 3\n---\nBody\n")

	if err := incrementCurIteration(path); err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, path)
	if !strings.Contains(content, "CurIteration: 1") {
		t.Errorf("expected CurIteration: 1, got:\n%s", content)
	}
}

func TestIncrementCurIteration_IncrementsExisting(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: created\nCurIteration: 2\n---\nBody\n")

	if err := incrementCurIteration(path); err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, path)
	if !strings.Contains(content, "CurIteration: 3") {
		t.Errorf("expected CurIteration: 3, got:\n%s", content)
	}
}

func TestFilePromptLoader_Basic(t *testing.T) {
	baseDir := t.TempDir()
	os.MkdirAll(filepath.Join(baseDir, "prompts"), 0755)
	os.WriteFile(filepath.Join(baseDir, "prompts", "prompt.md"), []byte("Base prompt for {{WIGGUMS_DIR}}"), 0644)

	loader := &FilePromptLoader{}
	tickets := []string{"/tmp/ticket1.md", "/tmp/ticket2.md"}

	result, err := loader.Load(baseDir, "prompts/prompt.md", "Remaining tickets:", tickets, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Base prompt for "+baseDir) {
		t.Error("should contain base prompt with substituted dir")
	}
	if !strings.Contains(result, "- ticket1\n") {
		t.Error("should contain ticket ID")
	}
}

func TestFilePromptLoader_WithAgentPrompt(t *testing.T) {
	baseDir := t.TempDir()
	os.MkdirAll(filepath.Join(baseDir, "prompts"), 0755)
	os.MkdirAll(filepath.Join(baseDir, "agents"), 0755)
	os.WriteFile(filepath.Join(baseDir, "prompts", "prompt.md"), []byte("Base prompt for {{WIGGUMS_DIR}}"), 0644)
	os.WriteFile(filepath.Join(baseDir, "agents", "explore.md"), []byte("---\nAgent: explore\n---\n\n# Explore Agent\nExplore instructions for {{WIGGUMS_DIR}}"), 0644)

	loader := &FilePromptLoader{}
	tickets := []string{"/tmp/ticket1.md"}

	result, err := loader.Load(baseDir, "prompts/prompt.md", "Remaining tickets:", tickets, "agents/explore.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Base prompt for "+baseDir) {
		t.Error("should contain base prompt")
	}
	if !strings.Contains(result, "# Explore Agent") {
		t.Error("should contain agent prompt body")
	}
	if !strings.Contains(result, "Explore instructions for "+baseDir) {
		t.Error("agent prompt should have {{WIGGUMS_DIR}} substituted")
	}
	if strings.Contains(result, "Agent: explore") {
		t.Error("frontmatter should be stripped from agent prompt")
	}

	// Verify order: base prompt, then agent prompt, then tickets
	baseIdx := strings.Index(result, "Base prompt")
	agentIdx := strings.Index(result, "Explore Agent")
	ticketIdx := strings.Index(result, "- ticket1\n")
	if baseIdx >= agentIdx || agentIdx >= ticketIdx {
		t.Errorf("wrong order: base@%d, agent@%d, ticket@%d", baseIdx, agentIdx, ticketIdx)
	}
}

func TestFilePromptLoader_MissingAgentPromptSkipped(t *testing.T) {
	baseDir := t.TempDir()
	os.MkdirAll(filepath.Join(baseDir, "prompts"), 0755)
	os.WriteFile(filepath.Join(baseDir, "prompts", "prompt.md"), []byte("Base prompt"), 0644)

	loader := &FilePromptLoader{}
	result, err := loader.Load(baseDir, "prompts/prompt.md", "Remaining tickets:", []string{"/tmp/t.md"}, "agents/nonexistent.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Base prompt") {
		t.Error("should contain base prompt even when agent file missing")
	}
}
