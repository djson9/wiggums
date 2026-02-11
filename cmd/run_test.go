package cmd

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
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

// --- Iteration tests ---

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

func TestCheckAndResetIterations_ResetsWhenBelowMin(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: completed\nMinIterations: 3\nCurIteration: 1\n---\nBody\n")

	checkAndResetIterations([]string{path})

	content := readTicket(t, path)
	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "in_progress") {
		t.Errorf("expected status reset to in_progress, got: %s", status)
	}
}

func TestCheckAndResetIterations_KeepsCompletedWhenAtMin(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: completed\nMinIterations: 3\nCurIteration: 3\n---\nBody\n")

	checkAndResetIterations([]string{path})

	content := readTicket(t, path)
	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "completed") {
		t.Errorf("expected status to stay completed, got: %s", status)
	}
}

func TestCheckAndResetIterations_KeepsCompletedWhenAboveMin(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: completed\nMinIterations: 2\nCurIteration: 5\n---\nBody\n")

	checkAndResetIterations([]string{path})

	content := readTicket(t, path)
	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "completed") {
		t.Errorf("expected status to stay completed, got: %s", status)
	}
}

func TestCheckAndResetIterations_NoOpWithoutMinIterations(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: completed\nCurIteration: 1\n---\nBody\n")

	checkAndResetIterations([]string{path})

	content := readTicket(t, path)
	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "completed") {
		t.Errorf("expected status to stay completed (no MinIterations), got: %s", status)
	}
}

// --- Ticket discovery tests ---

func TestFindIncompleteTickets_Basic(t *testing.T) {
	dir := t.TempDir()
	writeTicket(t, dir, "incomplete.md", "---\nStatus: created\n---\nTODO\n")
	writeTicket(t, dir, "done.md", "---\nStatus: completed\n---\nDone\n")

	results, err := findIncompleteTickets(dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 incomplete ticket, got %d", len(results))
	}
	if filepath.Base(results[0]) != "incomplete.md" {
		t.Errorf("expected incomplete.md, got %s", filepath.Base(results[0]))
	}
}

func TestFindIncompleteTickets_ExcludesCLAUDE(t *testing.T) {
	dir := t.TempDir()
	writeTicket(t, dir, "CLAUDE.md", "no status here")
	writeTicket(t, dir, "real.md", "---\nStatus: created\n---\n")

	results, err := findIncompleteTickets(dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 ticket (CLAUDE.md excluded), got %d", len(results))
	}
}

func TestFindIncompleteTickets_AgentFilter(t *testing.T) {
	dir := t.TempDir()
	writeTicket(t, dir, "no-agent.md", "---\nStatus: created\n---\n")
	writeTicket(t, dir, "agent-foo.md", "---\nStatus: created\nAgent: foo\n---\n")
	writeTicket(t, dir, "agent-bar.md", "---\nStatus: created\nAgent: bar\n---\n")
	writeTicket(t, dir, "empty-agent.md", "---\nStatus: created\nAgent:\n---\n")
	writeTicket(t, dir, "completed.md", "---\nStatus: completed\n---\nDone\n")

	t.Run("default run excludes agent tickets", func(t *testing.T) {
		results, err := findIncompleteTickets(dir, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 tickets, got %d: %v", len(results), results)
		}
		for _, r := range results {
			base := filepath.Base(r)
			if base != "no-agent.md" && base != "empty-agent.md" {
				t.Errorf("unexpected ticket: %s", base)
			}
		}
	})

	t.Run("agent run includes only matching", func(t *testing.T) {
		results, err := findIncompleteTickets(dir, "foo", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || filepath.Base(results[0]) != "agent-foo.md" {
			t.Errorf("expected only agent-foo.md, got %v", results)
		}
	})

	t.Run("case insensitive agent matching", func(t *testing.T) {
		results, err := findIncompleteTickets(dir, "Foo", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 ticket, got %d", len(results))
		}
	})
}

func TestFindUnverifiedTickets_RecentCompleted(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "done.md", "---\nStatus: completed\n---\nDone\n")
	touchRecent(t, path)

	results, err := findUnverifiedTickets(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 unverified ticket, got %d", len(results))
	}
}

func TestFindUnverifiedTickets_ExcludesVerified(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "verified.md", "---\nStatus: completed + verified\n---\nDone\n")
	touchRecent(t, path)

	results, err := findUnverifiedTickets(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 unverified tickets, got %d", len(results))
	}
}

func TestFindUnverifiedTickets_ExcludesOld(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "old.md", "---\nStatus: completed\n---\nDone\n")
	touchOld(t, path)

	results, err := findUnverifiedTickets(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 unverified tickets (too old), got %d", len(results))
	}
}

func TestFindUnverifiedTickets_SkipsWhenSkipVerificationTrue(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "skip.md", "---\nStatus: completed\nSkipVerification: true\n---\nDone\n")
	touchRecent(t, path)

	results, err := findUnverifiedTickets(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 unverified tickets (SkipVerification=true), got %d", len(results))
	}

	// Verify it was auto-marked as verified
	content := readTicket(t, path)
	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "completed + verified") {
		t.Errorf("expected auto-verified status, got: %s", status)
	}
}

func TestFindUnverifiedTickets_IncludesWhenSkipVerificationFalse(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "noskip.md", "---\nStatus: completed\nSkipVerification: false\n---\nDone\n")
	touchRecent(t, path)

	results, err := findUnverifiedTickets(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 unverified ticket (SkipVerification=false), got %d", len(results))
	}
}

func TestFindUnverifiedTickets_SkipVerificationRespectsMinIterations(t *testing.T) {
	dir := t.TempDir()
	// SkipVerification=true but MinIterations=5 with only CurIteration=1
	path := writeTicket(t, dir, "skip_iter.md", "---\nStatus: completed\nSkipVerification: true\nMinIterations: 5\nCurIteration: 1\n---\nDone\n")
	touchRecent(t, path)

	results, err := findUnverifiedTickets(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	// Should not be in unverified list (it was reset to in_progress)
	if len(results) != 0 {
		t.Errorf("expected 0 unverified tickets, got %d", len(results))
	}

	// Verify it was reset to in_progress, NOT auto-verified
	content := readTicket(t, path)
	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "in_progress") {
		t.Errorf("expected status reset to in_progress (MinIterations not met), got: %s", status)
	}
}

func TestFindUnverifiedTickets_SkipVerificationAutoVerifiesWhenMinIterationsMet(t *testing.T) {
	dir := t.TempDir()
	// SkipVerification=true, MinIterations=3, CurIteration=3 — should auto-verify
	path := writeTicket(t, dir, "skip_met.md", "---\nStatus: completed\nSkipVerification: true\nMinIterations: 3\nCurIteration: 3\n---\nDone\n")
	touchRecent(t, path)

	results, err := findUnverifiedTickets(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 unverified tickets, got %d", len(results))
	}

	// Verify it was auto-marked as verified
	content := readTicket(t, path)
	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "completed + verified") {
		t.Errorf("expected auto-verified status, got: %s", status)
	}
}

func TestFindUnverifiedTickets_SkipVerificationQuotedMinIterations(t *testing.T) {
	dir := t.TempDir()
	// SkipVerification=true, MinIterations="8" (quoted), CurIteration=1
	path := writeTicket(t, dir, "quoted.md", "---\nStatus: completed\nSkipVerification: true\nMinIterations: \"8\"\nCurIteration: 1\n---\nDone\n")
	touchRecent(t, path)

	results, err := findUnverifiedTickets(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 unverified tickets, got %d", len(results))
	}

	// Verify it was reset to in_progress (quoted "8" is now parsed correctly)
	content := readTicket(t, path)
	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "in_progress") {
		t.Errorf("expected status reset to in_progress (quoted MinIterations not met), got: %s", status)
	}
}

// --- Loop integration tests (with mocks) ---

func TestRunLoop_WorkPath(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	ticketPath := writeTicket(t, ticketsDir, "task.md", "---\nStatus: created\n---\nDo something\n")

	runner := &mockRunner{exitCode: 0}
	// Simulate claude marking ticket as completed during work pass
	runner.onRun = func() {
		content := readTicket(t, ticketPath)
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		os.WriteFile(ticketPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "test prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		notifier:     &mockNotifier{},
		baseDir:      dir,
		ticketsDir:   ticketsDir,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Work pass + verification pass = 2 runner calls
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 runner calls (work + verify), got %d", len(runner.calls))
	}
	if len(loader.calls) != 2 {
		t.Fatalf("expected 2 loader calls (work + verify), got %d", len(loader.calls))
	}
	if loader.calls[0].promptFile != "prompts/prompt.md" {
		t.Errorf("expected work prompt first, got %s", loader.calls[0].promptFile)
	}
	if loader.calls[1].promptFile != "prompts/verify.md" {
		t.Errorf("expected verify prompt second, got %s", loader.calls[1].promptFile)
	}
}

func TestRunLoop_VerifyPath(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	path := writeTicket(t, ticketsDir, "done.md", "---\nStatus: completed\n---\nDone\n")
	touchRecent(t, path)

	runner := &mockRunner{exitCode: 0}
	loader := &mockPromptLoader{result: "verify prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		notifier:     &mockNotifier{},
		baseDir:      dir,
		ticketsDir:   ticketsDir,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(loader.calls) != 1 {
		t.Fatalf("expected 1 loader call, got %d", len(loader.calls))
	}
	if loader.calls[0].promptFile != "prompts/verify.md" {
		t.Errorf("expected verify prompt, got %s", loader.calls[0].promptFile)
	}
}

func TestRunLoop_IterationResetFlow(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	ticketPath := writeTicket(t, ticketsDir, "iterating.md", "---\nStatus: created\nMinIterations: 3\n---\nWork\n")

	runner := &mockRunner{exitCode: 0}
	// Simulate claude marking ticket as completed during each run
	runner.onRun = func() {
		content := readTicket(t, ticketPath)
		// Replace any non-completed status with completed
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		content = strings.Replace(content, "Status: in_progress", "Status: completed", 1)
		os.WriteFile(ticketPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "work prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		notifier:     &mockNotifier{},
		baseDir:      dir,
		ticketsDir:   ticketsDir,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Loop should have continued until MinIterations was met
	content := readTicket(t, ticketPath)
	curIter := extractFrontmatterInt(content, "CurIteration")
	if curIter != 3 {
		t.Errorf("expected CurIteration=3 (MinIterations met), got %d", curIter)
	}

	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "completed") {
		t.Errorf("expected status to stay completed after MinIterations met, got: %s", status)
	}

	// Should have been called 4 times: 3 work passes (one per iteration) + 1 verify pass
	if len(runner.calls) != 4 {
		t.Errorf("expected 4 runner calls (3 iterations + 1 verify), got %d", len(runner.calls))
	}
}

// --- FilePromptLoader tests ---

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
	if !strings.Contains(result, "ticket1.md") {
		t.Error("should contain ticket list")
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
	ticketIdx := strings.Index(result, "ticket1.md")
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
		t.Error("should still contain base prompt")
	}
}

// --- stripFrontmatter tests ---

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			"with frontmatter",
			"---\nAgent: explore\n---\n\n# Explore Agent\nDo stuff",
			"# Explore Agent\nDo stuff",
		},
		{
			"no frontmatter",
			"# Just a prompt\nDo stuff",
			"# Just a prompt\nDo stuff",
		},
		{
			"empty body after frontmatter",
			"---\nAgent: explore\n---\n",
			"",
		},
		{
			"frontmatter with extra whitespace",
			"---\nAgent: explore\n---\n\n\n  Body here  \n",
			"Body here",
		},
		{
			"single delimiter only",
			"---\nno closing delimiter",
			"---\nno closing delimiter",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripFrontmatter(tt.content)
			if got != tt.want {
				t.Errorf("stripFrontmatter() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Shortcuts path tests ---

func TestRunLoop_ShortcutsPathSubstitution_Default(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	ticketPath := writeTicket(t, ticketsDir, "task.md", "---\nStatus: created\n---\nDo something\n")

	runner := &mockRunner{exitCode: 0}
	runner.onRun = func() {
		content := readTicket(t, ticketPath)
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		os.WriteFile(ticketPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "Read {{SHORTCUTS_PATH}} for tips"}
	shortcutsFile := filepath.Join(dir, "prompts", "shortcuts.md")

	cfg := &loopConfig{
		runner:        runner,
		promptLoader:  loader,
		notifier:      &mockNotifier{},
		baseDir:       dir,
		ticketsDir:    ticketsDir,
		shortcutsFile: shortcutsFile,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) < 1 {
		t.Fatalf("expected at least 1 runner call, got %d", len(runner.calls))
	}
	if strings.Contains(runner.calls[0], "{{SHORTCUTS_PATH}}") {
		t.Error("{{SHORTCUTS_PATH}} should have been substituted")
	}
	if !strings.Contains(runner.calls[0], shortcutsFile) {
		t.Errorf("prompt should contain shortcuts path %q, got: %s", shortcutsFile, runner.calls[0])
	}
}

func TestRunLoop_ShortcutsPathSubstitution_Workspace(t *testing.T) {
	dir := t.TempDir()
	wsTicketsDir := filepath.Join(dir, "workspaces", "myproject", "tickets")
	os.MkdirAll(wsTicketsDir, 0755)
	ticketPath := writeTicket(t, wsTicketsDir, "task.md", "---\nStatus: created\n---\nDo something\n")

	runner := &mockRunner{exitCode: 0}
	runner.onRun = func() {
		content := readTicket(t, ticketPath)
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		os.WriteFile(ticketPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "Read {{SHORTCUTS_PATH}} for tips"}
	shortcutsFile := filepath.Join(dir, "workspaces", "myproject", "shortcuts.md")

	cfg := &loopConfig{
		runner:        runner,
		promptLoader:  loader,
		notifier:      &mockNotifier{},
		baseDir:       dir,
		ticketsDir:    wsTicketsDir,
		shortcutsFile: shortcutsFile,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runner.calls[0], shortcutsFile) {
		t.Errorf("prompt should contain workspace shortcuts path %q, got: %s", shortcutsFile, runner.calls[0])
	}
}

func TestRunLoop_ShortcutsPathSubstitution_Verify(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	path := writeTicket(t, ticketsDir, "done.md", "---\nStatus: completed\n---\nDone\n")
	touchRecent(t, path)

	runner := &mockRunner{exitCode: 0}
	loader := &mockPromptLoader{result: "Verify with {{SHORTCUTS_PATH}}"}
	shortcutsFile := filepath.Join(dir, "prompts", "shortcuts.md")

	cfg := &loopConfig{
		runner:        runner,
		promptLoader:  loader,
		notifier:      &mockNotifier{},
		baseDir:       dir,
		ticketsDir:    ticketsDir,
		shortcutsFile: shortcutsFile,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(runner.calls[0], "{{SHORTCUTS_PATH}}") {
		t.Error("{{SHORTCUTS_PATH}} should have been substituted in verify prompt")
	}
	if !strings.Contains(runner.calls[0], shortcutsFile) {
		t.Errorf("verify prompt should contain shortcuts path %q", shortcutsFile)
	}
}

// --- UpdatedAt tests ---

func TestUpdateUpdatedAt_SetsExistingField(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: created\nUpdatedAt:\n---\nBody\n")

	if err := updateUpdatedAt(path); err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, path)
	// Should have a timestamp in YYYY-MM-DD HH:MM format
	now := time.Now().Format("2006-01-02 15:04")
	if !strings.Contains(content, "UpdatedAt: "+now) {
		t.Errorf("expected UpdatedAt: %s, got:\n%s", now, content)
	}
}

func TestUpdateUpdatedAt_ReplacesExistingValue(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: created\nUpdatedAt: 2025-01-01 00:00\n---\nBody\n")

	if err := updateUpdatedAt(path); err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, path)
	now := time.Now().Format("2006-01-02 15:04")
	if !strings.Contains(content, "UpdatedAt: "+now) {
		t.Errorf("expected UpdatedAt: %s, got:\n%s", now, content)
	}
	if strings.Contains(content, "2025-01-01") {
		t.Error("old timestamp should have been replaced")
	}
}

func TestUpdateUpdatedAt_InsertsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := writeTicket(t, dir, "test.md", "---\nStatus: created\n---\nBody\n")

	if err := updateUpdatedAt(path); err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, path)
	now := time.Now().Format("2006-01-02 15:04")
	if !strings.Contains(content, "UpdatedAt: "+now) {
		t.Errorf("expected UpdatedAt to be inserted, got:\n%s", content)
	}
	// Should still have valid frontmatter structure
	if !strings.Contains(content, "---\nStatus: created\nUpdatedAt: "+now+"\n---") {
		t.Errorf("frontmatter structure should be preserved, got:\n%s", content)
	}
}

func TestUpdateUpdatedAt_PreservesBody(t *testing.T) {
	dir := t.TempDir()
	body := "## Execution Plan\nDo the thing\n\n## Results\nIt worked\n"
	path := writeTicket(t, dir, "test.md", "---\nStatus: created\nUpdatedAt:\n---\n"+body)

	if err := updateUpdatedAt(path); err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, path)
	if !strings.Contains(content, body) {
		t.Errorf("body should be preserved, got:\n%s", content)
	}
}

func TestUpdateTicketsTimestamp_MultiplePaths(t *testing.T) {
	dir := t.TempDir()
	p1 := writeTicket(t, dir, "a.md", "---\nStatus: created\nUpdatedAt:\n---\nA\n")
	p2 := writeTicket(t, dir, "b.md", "---\nStatus: in_progress\n---\nB\n")

	updateTicketsTimestamp([]string{p1, p2})

	now := time.Now().Format("2006-01-02 15:04")
	for _, p := range []string{p1, p2} {
		content := readTicket(t, p)
		if !strings.Contains(content, "UpdatedAt: "+now) {
			t.Errorf("%s should have UpdatedAt: %s, got:\n%s", filepath.Base(p), now, content)
		}
	}
}

func TestRunLoop_WorkPath_UpdatesTimestamp(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	ticketPath := writeTicket(t, ticketsDir, "task.md", "---\nStatus: created\nUpdatedAt:\n---\nDo something\n")

	runner := &mockRunner{exitCode: 0}
	runner.onRun = func() {
		content := readTicket(t, ticketPath)
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		os.WriteFile(ticketPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "test prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		notifier:     &mockNotifier{},
		baseDir:      dir,
		ticketsDir:   ticketsDir,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, ticketPath)
	now := time.Now().Format("2006-01-02 15:04")
	if !strings.Contains(content, "UpdatedAt: "+now) {
		t.Errorf("work path should update UpdatedAt, got:\n%s", content)
	}
}

func TestRunLoop_VerifyPath_UpdatesTimestamp(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	ticketPath := writeTicket(t, ticketsDir, "done.md", "---\nStatus: completed\nUpdatedAt:\n---\nDone\n")
	touchRecent(t, ticketPath)

	runner := &mockRunner{exitCode: 0}
	loader := &mockPromptLoader{result: "verify prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		notifier:     &mockNotifier{},
		baseDir:      dir,
		ticketsDir:   ticketsDir,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, ticketPath)
	now := time.Now().Format("2006-01-02 15:04")
	if !strings.Contains(content, "UpdatedAt: "+now) {
		t.Errorf("verify path should update UpdatedAt, got:\n%s", content)
	}
}

// --- Workspace tests ---

func TestReadWorkspaceDirectory(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{"with directory", "---\nDirectory: /home/user/project\n---\n", "/home/user/project"},
		{"empty directory", "---\nDirectory:\n---\n", ""},
		{"no directory field", "---\nStatus: created\n---\n", ""},
		{"spaces in path", "---\nDirectory: /home/user/my project\n---\n", "/home/user/my project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTicket(t, dir, "index.md", tt.content)
			got, err := readWorkspaceDirectory(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestRunLoop_WorkspaceTicketsDir(t *testing.T) {
	// Simulate a workspace: tickets in a subdirectory, not the default
	dir := t.TempDir()
	wsTicketsDir := filepath.Join(dir, "workspaces", "myproject", "tickets")
	os.MkdirAll(wsTicketsDir, 0755)
	ticketPath := writeTicket(t, wsTicketsDir, "ws-task.md", "---\nStatus: created\n---\nWorkspace task\n")

	runner := &mockRunner{exitCode: 0}
	runner.onRun = func() {
		content := readTicket(t, ticketPath)
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		os.WriteFile(ticketPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "workspace prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		notifier:     &mockNotifier{},
		baseDir:      dir,
		ticketsDir:   wsTicketsDir,
		workDir:      "/tmp/fake-project",
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Work pass + verification pass = 2 calls
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 runner calls (work + verify), got %d", len(runner.calls))
	}
	if len(loader.calls) != 2 {
		t.Fatalf("expected 2 loader calls (work + verify), got %d", len(loader.calls))
	}
	// Verify the ticket from the workspace dir was found in the work pass
	if len(loader.calls[0].tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(loader.calls[0].tickets))
	}
	if !strings.Contains(loader.calls[0].tickets[0], "ws-task.md") {
		t.Errorf("expected ws-task.md ticket, got %s", loader.calls[0].tickets[0])
	}
}

func TestRunLoop_WorkspaceWithAgentFilter(t *testing.T) {
	dir := t.TempDir()
	wsTicketsDir := filepath.Join(dir, "workspaces", "myproject", "tickets")
	os.MkdirAll(wsTicketsDir, 0755)
	os.MkdirAll(filepath.Join(dir, "agents"), 0755)

	// Create tickets: one with agent, one without
	writeTicket(t, wsTicketsDir, "no-agent.md", "---\nStatus: created\n---\nGeneral task\n")
	agentPath := writeTicket(t, wsTicketsDir, "agent-local.md", "---\nStatus: created\nAgent: local\n---\nAgent task\n")

	runner := &mockRunner{exitCode: 0}
	runner.onRun = func() {
		content := readTicket(t, agentPath)
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		os.WriteFile(agentPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "agent prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		notifier:     &mockNotifier{},
		baseDir:      dir,
		ticketsDir:   wsTicketsDir,
		agentFilter:  "local",
		workDir:      "/tmp/fake-project",
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Work pass + verify pass = 2 loader calls
	if len(loader.calls) < 1 {
		t.Fatalf("expected at least 1 loader call, got %d", len(loader.calls))
	}
	if len(loader.calls[0].tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(loader.calls[0].tickets))
	}
	if !strings.Contains(loader.calls[0].tickets[0], "agent-local.md") {
		t.Errorf("expected agent-local.md, got %s", loader.calls[0].tickets[0])
	}
	// Should pass agent prompt file path
	if loader.calls[0].agentPromptFile != "agents/local.md" {
		t.Errorf("expected agents/local.md, got %s", loader.calls[0].agentPromptFile)
	}
}

func TestRunLoop_WorkspaceWithoutAgentExcludesAgentTickets(t *testing.T) {
	dir := t.TempDir()
	wsTicketsDir := filepath.Join(dir, "workspaces", "myproject", "tickets")
	os.MkdirAll(wsTicketsDir, 0755)

	// Create tickets: one with agent, one without
	noAgentPath := writeTicket(t, wsTicketsDir, "no-agent.md", "---\nStatus: created\n---\nGeneral task\n")
	writeTicket(t, wsTicketsDir, "agent-local.md", "---\nStatus: created\nAgent: local\n---\nAgent task\n")

	runner := &mockRunner{exitCode: 0}
	runner.onRun = func() {
		content := readTicket(t, noAgentPath)
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		os.WriteFile(noAgentPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "default prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		notifier:     &mockNotifier{},
		baseDir:      dir,
		ticketsDir:   wsTicketsDir,
		agentFilter:  "", // no agent filter = default run
		workDir:      "/tmp/fake-project",
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Work pass + verify pass
	if len(loader.calls) < 1 {
		t.Fatalf("expected at least 1 loader call, got %d", len(loader.calls))
	}
	if len(loader.calls[0].tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(loader.calls[0].tickets))
	}
	if !strings.Contains(loader.calls[0].tickets[0], "no-agent.md") {
		t.Errorf("expected no-agent.md, got %s", loader.calls[0].tickets[0])
	}
	// Should NOT pass agent prompt file
	if loader.calls[0].agentPromptFile != "" {
		t.Errorf("expected empty agent prompt file, got %s", loader.calls[0].agentPromptFile)
	}
}

func TestRunLoop_SingleTicketAtATime(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)

	// Create multiple incomplete tickets
	firstPath := writeTicket(t, ticketsDir, "a_first.md", "---\nStatus: created\n---\nFirst task\n")
	writeTicket(t, ticketsDir, "b_second.md", "---\nStatus: created\n---\nSecond task\n")
	writeTicket(t, ticketsDir, "c_third.md", "---\nStatus: created\n---\nThird task\n")

	runner := &mockRunner{exitCode: 0}
	runner.onRun = func() {
		content := readTicket(t, firstPath)
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		os.WriteFile(firstPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "test prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		notifier:     &mockNotifier{},
		baseDir:      dir,
		ticketsDir:   ticketsDir,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Work pass sends 1 ticket, then verify pass runs. The first work call
	// should only include one ticket (single ticket at a time).
	if len(loader.calls) < 1 {
		t.Fatalf("expected at least 1 loader call, got %d", len(loader.calls))
	}
	if len(loader.calls[0].tickets) != 1 {
		t.Fatalf("expected 1 ticket passed to loader (single ticket at a time), got %d", len(loader.calls[0].tickets))
	}
	// Header should be "Next ticket:" not "Remaining tickets:"
	if loader.calls[0].header != "Next ticket:" {
		t.Errorf("expected header 'Next ticket:', got %q", loader.calls[0].header)
	}
}

// --- State management tests ---

func TestWriteAndReadState(t *testing.T) {
	dir := t.TempDir()

	writeState(dir, "myworkspace", "working", "ticket_123.md")
	state := readState(dir, "myworkspace")

	if state.Status != "working" {
		t.Errorf("expected status 'working', got %q", state.Status)
	}
	if state.Ticket != "ticket_123.md" {
		t.Errorf("expected ticket 'ticket_123.md', got %q", state.Ticket)
	}
}

func TestReadState_Missing(t *testing.T) {
	dir := t.TempDir()

	state := readState(dir, "nonexistent")
	if state.Status != "" {
		t.Errorf("expected empty status for missing state, got %q", state.Status)
	}
}

func TestClearState(t *testing.T) {
	dir := t.TempDir()

	writeState(dir, "myworkspace", "working", "ticket_123.md")
	clearState(dir, "myworkspace")
	state := readState(dir, "myworkspace")

	if state.Status != "" {
		t.Errorf("expected empty status after clear, got %q", state.Status)
	}
}

func TestRunLoop_WritesWorkingState(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	ticketPath := writeTicket(t, ticketsDir, "task.md", "---\nStatus: created\n---\nDo something\n")

	var capturedState WorkspaceState
	callCount := 0
	runner := &mockRunner{exitCode: 0}
	runner.onRun = func() {
		callCount++
		if callCount == 1 {
			// Capture state during the work pass (first call)
			capturedState = readState(dir, "testws")
			// Mark ticket as completed so loop can proceed to verification
			content := readTicket(t, ticketPath)
			content = strings.Replace(content, "Status: created", "Status: completed", 1)
			os.WriteFile(ticketPath, []byte(content), 0644)
		}
	}
	loader := &mockPromptLoader{result: "test prompt"}

	cfg := &loopConfig{
		runner:        runner,
		promptLoader:  loader,
		notifier:      &mockNotifier{},
		baseDir:       dir,
		ticketsDir:    ticketsDir,
		workspaceName: "testws",
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	if capturedState.Status != "working" {
		t.Errorf("expected state 'working' during run, got %q", capturedState.Status)
	}
	if capturedState.Ticket != "task.md" {
		t.Errorf("expected ticket 'task.md' in state, got %q", capturedState.Ticket)
	}

	// After loop exits, state should be cleared (defer clearState)
	finalState := readState(dir, "testws")
	if finalState.Status != "" {
		t.Errorf("expected state cleared after loop exit, got %q", finalState.Status)
	}
}

func TestRunLoop_WritesVerifyingState(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	path := writeTicket(t, ticketsDir, "done.md", "---\nStatus: completed\n---\nDone\n")
	touchRecent(t, path)

	var capturedState WorkspaceState
	runner := &mockRunner{exitCode: 0}
	runner.onRun = func() {
		capturedState = readState(dir, "testws")
	}
	loader := &mockPromptLoader{result: "verify prompt"}

	cfg := &loopConfig{
		runner:        runner,
		promptLoader:  loader,
		notifier:      &mockNotifier{},
		baseDir:       dir,
		ticketsDir:    ticketsDir,
		workspaceName: "testws",
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	if capturedState.Status != "verifying" {
		t.Errorf("expected state 'verifying' during verification, got %q", capturedState.Status)
	}
	if capturedState.Ticket != "done.md" {
		t.Errorf("expected ticket 'done.md' in state, got %q", capturedState.Ticket)
	}
}

func TestResolvePromptFile_FallsBackToRoot(t *testing.T) {
	dir := t.TempDir()
	// Create root prompt
	os.MkdirAll(filepath.Join(dir, "prompts"), 0755)
	os.WriteFile(filepath.Join(dir, "prompts", "prompt.md"), []byte("root"), 0644)

	got := resolvePromptFile(dir, "myws", "prompts/prompt.md")
	if got != "prompts/prompt.md" {
		t.Errorf("expected fallback to root, got %q", got)
	}
}

func TestResolvePromptFile_UsesWorkspacePrompt(t *testing.T) {
	dir := t.TempDir()
	// Create both root and workspace prompts
	os.MkdirAll(filepath.Join(dir, "prompts"), 0755)
	os.WriteFile(filepath.Join(dir, "prompts", "prompt.md"), []byte("root"), 0644)
	os.MkdirAll(filepath.Join(dir, "workspaces", "myws", "prompts"), 0755)
	os.WriteFile(filepath.Join(dir, "workspaces", "myws", "prompts", "prompt.md"), []byte("workspace"), 0644)

	got := resolvePromptFile(dir, "myws", "prompts/prompt.md")
	expected := filepath.Join("workspaces", "myws", "prompts", "prompt.md")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestResolvePromptFile_EmptyWorkspaceName(t *testing.T) {
	dir := t.TempDir()
	got := resolvePromptFile(dir, "", "prompts/prompt.md")
	if got != "prompts/prompt.md" {
		t.Errorf("expected root prompt when workspace empty, got %q", got)
	}
}

func TestRunLoop_WorkspaceScopedPrompt_AppendsNotReplaces(t *testing.T) {
	dir := t.TempDir()
	// Create root prompt
	os.MkdirAll(filepath.Join(dir, "prompts"), 0755)
	os.WriteFile(filepath.Join(dir, "prompts", "prompt.md"), []byte("root prompt content"), 0644)
	// Create workspace-scoped prompt
	os.MkdirAll(filepath.Join(dir, "workspaces", "testws", "prompts"), 0755)
	os.WriteFile(filepath.Join(dir, "workspaces", "testws", "prompts", "prompt.md"), []byte("workspace extra instructions"), 0644)

	ticketsDir := filepath.Join(dir, "workspaces", "testws", "tickets")
	os.MkdirAll(ticketsDir, 0755)
	os.WriteFile(filepath.Join(ticketsDir, "task.md"), []byte("---\nStatus: created\nSkipVerification: true\n---\nDo something"), 0644)

	runner := &mockRunner{}
	runner.onRun = func() {
		os.WriteFile(filepath.Join(ticketsDir, "task.md"), []byte("---\nStatus: completed + verified\nSkipVerification: true\n---\nDone"), 0644)
	}
	loader := &FilePromptLoader{}

	cfg := &loopConfig{
		runner:        runner,
		promptLoader:  loader,
		notifier:      &mockNotifier{},
		baseDir:       dir,
		ticketsDir:    ticketsDir,
		workspaceName: "testws",
		shortcutsFile: filepath.Join(dir, "shortcuts.md"),
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(runner.calls) == 0 {
		t.Fatal("expected at least one runner call")
	}
	// Both root AND workspace content should be present (append, not replace)
	if !strings.Contains(runner.calls[0], "root prompt content") {
		t.Errorf("expected root prompt content in runner call (append behavior)")
	}
	if !strings.Contains(runner.calls[0], "workspace extra instructions") {
		t.Errorf("expected workspace prompt content appended to runner call")
	}
	// Workspace prompt should come after root prompt
	rootIdx := strings.Index(runner.calls[0], "root prompt content")
	wsIdx := strings.Index(runner.calls[0], "workspace extra instructions")
	if rootIdx >= wsIdx {
		t.Errorf("workspace prompt should come after root prompt: root@%d, workspace@%d", rootIdx, wsIdx)
	}
}

// --- workspacePromptPath / loadWorkspacePrompt tests ---

func TestWorkspacePromptPath_ReturnsEmpty_NoWorkspacePrompt(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "prompts"), 0755)
	os.WriteFile(filepath.Join(dir, "prompts", "prompt.md"), []byte("root"), 0644)

	got := workspacePromptPath(dir, "myws", "prompts/prompt.md")
	if got != "" {
		t.Errorf("expected empty string when no workspace prompt exists, got %q", got)
	}
}

func TestWorkspacePromptPath_ReturnsPath_WhenExists(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "workspaces", "myws", "prompts"), 0755)
	os.WriteFile(filepath.Join(dir, "workspaces", "myws", "prompts", "prompt.md"), []byte("ws"), 0644)

	got := workspacePromptPath(dir, "myws", "prompts/prompt.md")
	expected := filepath.Join("workspaces", "myws", "prompts", "prompt.md")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestWorkspacePromptPath_EmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	got := workspacePromptPath(dir, "", "prompts/prompt.md")
	if got != "" {
		t.Errorf("expected empty string for empty workspace name, got %q", got)
	}
}

func TestLoadWorkspacePrompt_ReturnsContent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "workspaces", "myws", "prompts"), 0755)
	os.WriteFile(filepath.Join(dir, "workspaces", "myws", "prompts", "prompt.md"), []byte("custom instructions"), 0644)

	got := loadWorkspacePrompt(dir, "myws", "prompts/prompt.md")
	if got != "custom instructions" {
		t.Errorf("expected 'custom instructions', got %q", got)
	}
}

func TestLoadWorkspacePrompt_ReturnsEmpty_NoFile(t *testing.T) {
	dir := t.TempDir()
	got := loadWorkspacePrompt(dir, "myws", "prompts/prompt.md")
	if got != "" {
		t.Errorf("expected empty string when no workspace prompt, got %q", got)
	}
}

func TestSetStatusToAdditionalUserRequest(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test_ticket.md")
	content := "---\nStatus: completed\nAgent:\n---\n# Test\nBody\n"
	os.WriteFile(p, []byte(content), 0644)

	err := setStatusToAdditionalUserRequest(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "Status: additional_user_request") {
		t.Errorf("expected 'Status: additional_user_request' in file, got:\n%s", string(got))
	}
}

func TestSetStatusToAdditionalUserRequest_FromCreated(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test_ticket.md")
	content := "---\nStatus: created\nAgent:\n---\n# Test\nBody\n"
	os.WriteFile(p, []byte(content), 0644)

	err := setStatusToAdditionalUserRequest(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "Status: additional_user_request") {
		t.Errorf("expected 'Status: additional_user_request' in file, got:\n%s", string(got))
	}
}

func TestRestoreFrontmatterStatus(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test_ticket.md")
	content := "---\nStatus: additional_user_request\nAgent:\n---\n# Test\nBody\n"
	os.WriteFile(p, []byte(content), 0644)

	err := restoreFrontmatterStatus(p, "completed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "Status: completed") {
		t.Errorf("expected 'Status: completed' in file, got:\n%s", string(got))
	}
}

func TestRestoreFrontmatterStatus_CompletedVerified(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test_ticket.md")
	content := "---\nStatus: additional_user_request\nAgent:\n---\n# Test\nBody\n"
	os.WriteFile(p, []byte(content), 0644)

	err := restoreFrontmatterStatus(p, "completed + verified")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "Status: completed + verified") {
		t.Errorf("expected 'Status: completed + verified' in file, got:\n%s", string(got))
	}
}

func TestGetOriginalTicketStatus(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	path := "/tmp/test_ticket.md"

	// Create additional request with original status
	_, err := database.CreateAdditionalRequest(context.Background(), path, 1, false, "test", "completed + verified")
	if err != nil {
		t.Fatalf("create additional request: %v", err)
	}

	// Should return the saved original status
	status, err := database.GetOriginalTicketStatus(context.Background(), path)
	if err != nil {
		t.Fatalf("get original status: %v", err)
	}
	if status != "completed + verified" {
		t.Errorf("expected 'completed + verified', got %q", status)
	}
}

func TestGetOriginalTicketStatus_EmptyWhenNotSet(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	path := "/tmp/test_ticket.md"

	// Create additional request without original status
	_, _ = database.CreateAdditionalRequest(context.Background(), path, 1, false, "test", "")

	// Should return empty (no original status saved)
	status, err := database.GetOriginalTicketStatus(context.Background(), path)
	if err == nil && status != "" {
		t.Errorf("expected empty status when not set, got %q", status)
	}
}

// setupTestDB creates an in-memory SQLite database for testing and returns
// a cleanup function. Sets database.DB globally; caller must defer cleanup.
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

func TestWriteAdditionalRequestContentToFile_WritesFromDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dir := t.TempDir()
	path := filepath.Join(dir, "ticket.md")
	os.WriteFile(path, []byte(`---
Status: completed
---
## Original User Request
Do something

## Additional User Request

---
Below to be filled by agent. Agent should not modify above this line.

## Execution Plan
TODO
`), 0644)

	// Create a record in SQLite with content (simulating a draft that was created)
	_, err := database.CreateAdditionalRequest(context.Background(), path, 1, true, "Please fix the edge case", "")
	if err != nil {
		t.Fatalf("create additional request: %v", err)
	}

	// Write the content to the file at runtime
	err = writeAdditionalRequestContentToFile(path, 1)
	if err != nil {
		t.Fatalf("writeAdditionalRequestContentToFile: %v", err)
	}

	got, _ := os.ReadFile(path)
	contentStr := string(got)

	if !strings.Contains(contentStr, "### Additional User Request #1") {
		t.Error("missing Additional User Request #1 header")
	}
	if !strings.Contains(contentStr, "Please fix the edge case") {
		t.Error("missing request content from DB")
	}

	// Section should be before the divider
	dividerIdx := strings.Index(contentStr, "---\nBelow to be filled by agent")
	requestIdx := strings.Index(contentStr, "### Additional User Request #1")
	if dividerIdx >= 0 && requestIdx > dividerIdx {
		t.Error("additional request should be before the agent divider")
	}
}

func TestWriteAdditionalRequestContentToFile_Idempotent(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dir := t.TempDir()
	path := filepath.Join(dir, "ticket.md")
	// File already has the section
	os.WriteFile(path, []byte(`---
Status: completed
---
## Original User Request
Do something

### Additional User Request #1 — 2026-02-20 10:00
Already here

---
Below to be filled by agent. Agent should not modify above this line.
`), 0644)

	_, _ = database.CreateAdditionalRequest(context.Background(), path, 1, false, "Already here", "")

	// Should be a no-op since the section already exists
	err := writeAdditionalRequestContentToFile(path, 1)
	if err != nil {
		t.Fatalf("writeAdditionalRequestContentToFile: %v", err)
	}

	got, _ := os.ReadFile(path)
	contentStr := string(got)

	// Should only have one #1 section (not duplicated)
	count := strings.Count(contentStr, "### Additional User Request #1")
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of #1, got %d", count)
	}
}

func TestWriteAdditionalRequestContentToFile_NoDB(t *testing.T) {
	// Ensure database.DB is nil
	oldDB := database.DB
	database.DB = nil
	defer func() { database.DB = oldDB }()

	dir := t.TempDir()
	path := filepath.Join(dir, "ticket.md")
	os.WriteFile(path, []byte(`---
Status: completed
---
## Test
`), 0644)

	// Should return nil without error when DB is nil
	err := writeAdditionalRequestContentToFile(path, 1)
	if err != nil {
		t.Fatalf("expected nil error when DB is nil, got: %v", err)
	}

	// File should be unchanged
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), "### Additional User Request") {
		t.Error("should not write anything when DB is nil")
	}
}

func TestWriteAdditionalRequestContentToFile_EmptyContent(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dir := t.TempDir()
	path := filepath.Join(dir, "ticket.md")
	os.WriteFile(path, []byte(`---
Status: completed
---
## Test

---
Below to be filled by agent. Agent should not modify above this line.
`), 0644)

	// Create a record with empty content
	_, _ = database.CreateAdditionalRequest(context.Background(), path, 1, true, "", "")

	err := writeAdditionalRequestContentToFile(path, 1)
	if err != nil {
		t.Fatalf("writeAdditionalRequestContentToFile: %v", err)
	}

	// File should not have a new section (empty content)
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), "### Additional User Request #1") {
		t.Error("should not write section for empty content")
	}
}

// TestPidCaptureFeasibility proves that splitting exec.Cmd.Run() into
// Start() + Wait() gives access to Process.Pid before the subprocess exits.
// This validates the approach for adding PID tracking to ClaudeRunner.
func TestPidCaptureFeasibility(t *testing.T) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "echo", "hello")

	// Start() launches the process and populates cmd.Process
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// PID is available immediately after Start()
	pid := cmd.Process.Pid
	if pid <= 0 {
		t.Fatalf("expected positive PID, got %d", pid)
	}
	t.Logf("Captured PID: %d", pid)

	// Wait() blocks until the process exits
	if err := cmd.Wait(); err != nil {
		t.Fatalf("Wait() failed: %v", err)
	}
}

// TestPidCaptureWithCallback demonstrates the OnPidReady callback pattern
// that ClaudeRunner would use to report PID to callers.
func TestPidCaptureWithCallback(t *testing.T) {
	var capturedPid int
	onPidReady := func(pid int) {
		capturedPid = pid
	}

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sleep", "0.01")

	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Fire callback with PID
	onPidReady(cmd.Process.Pid)

	if err := cmd.Wait(); err != nil {
		t.Fatalf("Wait() failed: %v", err)
	}

	if capturedPid <= 0 {
		t.Fatalf("expected positive PID from callback, got %d", capturedPid)
	}
	t.Logf("Callback received PID: %d", capturedPid)
}

// TestPidCaptureExitCode verifies that the Start()/Wait() split still
// correctly captures exit codes (needed for the existing exit code logic).
func TestPidCaptureExitCode(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		args     []string
		wantCode int
	}{
		{"success", "true", nil, 0},
		{"failure", "false", nil, 1},
		{"exit 42", "sh", []string{"-c", "exit 42"}, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cmd := exec.CommandContext(ctx, tt.cmd, tt.args...)

			if err := cmd.Start(); err != nil {
				t.Fatalf("Start() failed: %v", err)
			}

			pid := cmd.Process.Pid
			if pid <= 0 {
				t.Fatalf("expected positive PID, got %d", pid)
			}

			err := cmd.Wait()
			var gotCode int
			if err == nil {
				gotCode = 0
			} else if exitErr, ok := err.(*exec.ExitError); ok {
				gotCode = exitErr.ExitCode()
			} else {
				t.Fatalf("unexpected error type: %v", err)
			}

			if gotCode != tt.wantCode {
				t.Errorf("expected exit code %d, got %d", tt.wantCode, gotCode)
			}
		})
	}
}
