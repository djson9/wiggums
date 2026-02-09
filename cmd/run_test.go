package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	results, err := findIncompleteTickets(dir, "")
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

	results, err := findIncompleteTickets(dir, "")
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
		results, err := findIncompleteTickets(dir, "")
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
		results, err := findIncompleteTickets(dir, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || filepath.Base(results[0]) != "agent-foo.md" {
			t.Errorf("expected only agent-foo.md, got %v", results)
		}
	})

	t.Run("case insensitive agent matching", func(t *testing.T) {
		results, err := findIncompleteTickets(dir, "Foo")
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

	results, err := findUnverifiedTickets(dir)
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

	results, err := findUnverifiedTickets(dir)
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

	results, err := findUnverifiedTickets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 unverified tickets (too old), got %d", len(results))
	}
}

// --- Loop integration tests (with mocks) ---

func TestRunLoop_WorkPath(t *testing.T) {
	dir := t.TempDir()
	ticketsDir := filepath.Join(dir, "tickets")
	os.MkdirAll(ticketsDir, 0755)
	writeTicket(t, ticketsDir, "task.md", "---\nStatus: created\n---\nDo something\n")

	runner := &mockRunner{exitCode: 0}
	loader := &mockPromptLoader{result: "test prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		baseDir:      dir,
		ticketsDir:   ticketsDir,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(runner.calls))
	}
	if len(loader.calls) != 1 {
		t.Fatalf("expected 1 loader call, got %d", len(loader.calls))
	}
	if loader.calls[0].promptFile != "prompts/prompt.md" {
		t.Errorf("expected work prompt, got %s", loader.calls[0].promptFile)
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
	// Simulate claude marking ticket as completed during its run
	runner.onRun = func() {
		content := readTicket(t, ticketPath)
		content = strings.Replace(content, "Status: created", "Status: completed", 1)
		os.WriteFile(ticketPath, []byte(content), 0644)
	}
	loader := &mockPromptLoader{result: "work prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		baseDir:      dir,
		ticketsDir:   ticketsDir,
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	content := readTicket(t, ticketPath)
	curIter := extractFrontmatterInt(content, "CurIteration")
	if curIter != 1 {
		t.Errorf("expected CurIteration=1, got %d", curIter)
	}

	status := extractFrontmatterStatus(content)
	if !strings.Contains(strings.ToLower(status), "in_progress") {
		t.Errorf("expected status reset to in_progress, got: %s", status)
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
	writeTicket(t, wsTicketsDir, "ws-task.md", "---\nStatus: created\n---\nWorkspace task\n")

	runner := &mockRunner{exitCode: 0}
	loader := &mockPromptLoader{result: "workspace prompt"}

	cfg := &loopConfig{
		runner:       runner,
		promptLoader: loader,
		baseDir:      dir,
		ticketsDir:   wsTicketsDir,
		workDir:      "/tmp/fake-project",
	}

	err := runLoop(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(runner.calls))
	}
	if len(loader.calls) != 1 {
		t.Fatalf("expected 1 loader call, got %d", len(loader.calls))
	}
	// Verify the ticket from the workspace dir was found
	if len(loader.calls[0].tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(loader.calls[0].tickets))
	}
	if !strings.Contains(loader.calls[0].tickets[0], "ws-task.md") {
		t.Errorf("expected ws-task.md ticket, got %s", loader.calls[0].tickets[0])
	}
}
