package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner abstracts the execution of claude (or any command) with a prompt.
type Runner interface {
	Run(ctx context.Context, prompt string, args []string) (exitCode int, err error)
}

// PromptLoader abstracts reading and assembling prompt templates.
type PromptLoader interface {
	Load(baseDir, promptFile, header string, tickets []string, agentPromptFile string) (string, error)
}

// loopConfig holds all dependencies and parameters for the main loop.
type loopConfig struct {
	runner       Runner
	promptLoader PromptLoader
	baseDir      string
	ticketsDir   string
	claudeArgs   []string
	agentFilter  string
}

// ClaudeRunner is the real Runner implementation that shells out to the claude CLI.
type ClaudeRunner struct{}

func (c *ClaudeRunner) Run(ctx context.Context, prompt string, args []string) (int, error) {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
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
			agentPrompt := strings.ReplaceAll(string(agentContent), "{{WIGGUMS_DIR}}", baseDir)
			b.WriteString("\n\n")
			b.WriteString(agentPrompt)
		}
	}

	b.WriteString("\n\n## ")
	b.WriteString(header)
	b.WriteString("\n")
	for _, ticket := range tickets {
		b.WriteString("- ")
		b.WriteString(ticket)
		b.WriteString("\n")
	}

	return b.String(), nil
}
