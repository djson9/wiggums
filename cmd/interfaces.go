package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gen2brain/beeep"
)

// Runner abstracts the execution of claude (or any command) with a prompt.
type Runner interface {
	Run(ctx context.Context, prompt string, args []string) (exitCode int, err error)
}

// PromptLoader abstracts reading and assembling prompt templates.
type PromptLoader interface {
	Load(baseDir, promptFile, header string, tickets []string, agentPromptFile string) (string, error)
}

// Notifier abstracts desktop notifications so tests don't trigger real alerts.
type Notifier interface {
	Notify(title, message, icon string) error
}

// ClaudeRunner is the real Runner implementation that shells out to the claude CLI.
type ClaudeRunner struct {
	WorkDir string // if set, claude runs in this directory
}

func (c *ClaudeRunner) Run(ctx context.Context, prompt string, args []string) (int, error) {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}

	err := cmd.Run()

	// Restore terminal state after subprocess exit. Claude CLI puts the
	// terminal in raw mode; if it exits without restoring, Ctrl+C stops
	// generating SIGINT. stty sane is a no-op on an already-sane terminal.
	sane := exec.Command("stty", "sane")
	sane.Stdin = os.Stdin
	_ = sane.Run()

	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	}
	if ctx.Err() != nil {
		return 130, nil
	}
	return 1, fmt.Errorf("error running claude: %w", err)
}

// FilePromptLoader is the real PromptLoader that reads prompt files from disk.
type FilePromptLoader struct{}

func (f *FilePromptLoader) Load(baseDir, promptFile, header string, tickets []string, agentPromptFile string) (string, error) {
	path := filepath.Join(baseDir, promptFile)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not read %s: %w", promptFile, err)
	}

	prompt := strings.ReplaceAll(string(content), "{{WIGGUMS_DIR}}", baseDir)

	var b strings.Builder
	b.WriteString(prompt)

	if agentPromptFile != "" {
		agentPath := filepath.Join(baseDir, agentPromptFile)
		if agentContent, err := os.ReadFile(agentPath); err == nil {
			agentPrompt := stripFrontmatter(string(agentContent))
			agentPrompt = strings.ReplaceAll(agentPrompt, "{{WIGGUMS_DIR}}", baseDir)
			b.WriteString("\n\n")
			b.WriteString(agentPrompt)
		}
	}

	b.WriteString("\n\n## ")
	b.WriteString(header)
	b.WriteString("\n")
	for _, ticket := range tickets {
		base := filepath.Base(ticket)
		id := strings.TrimSuffix(base, ".md")
		if idx := strings.IndexByte(id, '_'); idx > 0 {
			id = id[:idx]
		}
		b.WriteString("- ")
		b.WriteString(id)
		b.WriteString("\n")
	}

	return b.String(), nil
}

// BeeepNotifier is the real Notifier that sends desktop notifications via beeep.
type BeeepNotifier struct{}

func (b *BeeepNotifier) Notify(title, message, icon string) error {
	return beeep.Notify(title, message, icon)
}

// stripFrontmatter removes YAML frontmatter (between --- delimiters) from content.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return content
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return content
	}
	return strings.TrimSpace(parts[2])
}
