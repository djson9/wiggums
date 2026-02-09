package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	runCmd.AddCommand(runAgentCmd)
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run [-- claude args...]",
	Short: "Run the ticket processing loop",
	Long:  "Runs an infinite loop finding incomplete tickets and piping prompts to Claude Code.\nExcludes tickets that have an Agent property set.",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return startLoop(args, "", "", "")
	},
}

var runAgentCmd = &cobra.Command{
	Use:   "agent [agent-name] [-- claude args...]",
	Short: "Run the ticket processing loop for a specific agent",
	Long:  "Runs an infinite loop finding incomplete tickets that match the given agent name.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return startLoop(args[1:], args[0], "", "")
	},
}

// startLoop wires up real implementations and starts the main loop.
// ticketsDirOverride and workDir are optional; when empty, defaults are used.
func startLoop(args []string, agentFilter, ticketsDirOverride, workDir string) error {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return fmt.Errorf("could not resolve base directory: %w", err)
	}

	claudeArgs := args
	if yolo {
		claudeArgs = append([]string{"--model", "opus", "--dangerously-skip-permissions"}, claudeArgs...)
	}
	if workDir != "" {
		claudeArgs = append([]string{"--add-dir", workDir}, claudeArgs...)
	}

	os.Setenv("TERM", "xterm")

	ticketsDir := filepath.Join(baseDir, "tickets")
	if ticketsDirOverride != "" {
		ticketsDir = ticketsDirOverride
	}

	shortcutsFile := filepath.Join(baseDir, "prompts", "shortcuts.md")
	if ticketsDirOverride != "" {
		shortcutsFile = filepath.Join(filepath.Dir(ticketsDirOverride), "shortcuts.md")
	}

	cfg := &loopConfig{
		runner:        &ClaudeRunner{WorkDir: baseDir},
		promptLoader:  &FilePromptLoader{},
		baseDir:       baseDir,
		ticketsDir:    ticketsDir,
		claudeArgs:    claudeArgs,
		agentFilter:   agentFilter,
		workDir:       workDir,
		shortcutsFile: shortcutsFile,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	return runLoop(ctx, cfg)
}

// runLoop is the main ticket processing loop. It is separated from cobra
// wiring so it can be tested with mock Runner and PromptLoader.
func runLoop(ctx context.Context, cfg *loopConfig) error {
	fmt.Printf("Using tickets directory: %s\n", cfg.ticketsDir)
	if cfg.agentFilter != "" {
		fmt.Printf("Agent filter: %s\n", cfg.agentFilter)
	}
	if len(cfg.claudeArgs) > 0 {
		fmt.Printf("Claude args: %s\n", strings.Join(cfg.claudeArgs, " "))
	}

	for {
		if _, err := os.Stat(cfg.ticketsDir); os.IsNotExist(err) {
			fmt.Println("Error: No tickets directory found")
			os.Exit(1)
		}

		// Find recently completed but unverified tickets
		unverified, err := findUnverifiedTickets(cfg.ticketsDir)
		if err != nil {
			return fmt.Errorf("error finding unverified tickets: %w", err)
		}

		// Determine agent prompt file path (empty string if no agent)
		agentPromptFile := ""
		if cfg.agentFilter != "" {
			agentPromptFile = filepath.Join("agents", cfg.agentFilter+".md")
		}

		if len(unverified) > 0 {
			fmt.Println("Running prompts/verify.md")
			for _, f := range unverified {
				fmt.Println(filepath.Base(f))
			}

			prompt, err := cfg.promptLoader.Load(cfg.baseDir, "prompts/verify.md", "Tickets to verify:", unverified, agentPromptFile)
			if err != nil {
				return fmt.Errorf("error building verify prompt: %w", err)
			}
			prompt = strings.ReplaceAll(prompt, "{{SHORTCUTS_PATH}}", cfg.shortcutsFile)

			exitCode, err := cfg.runner.Run(ctx, prompt, cfg.claudeArgs)
			if err != nil {
				return err
			}
			if exitCode == 0 {
				return nil
			}
			fmt.Printf("Claude exited with code %d, retrying in 5s...\n", exitCode)
			if err := sleepWithContext(ctx, 5*time.Second); err != nil {
				os.Exit(130)
			}
			continue
		}

		// Find remaining incomplete tickets (filtered by agent)
		remaining, err := findIncompleteTickets(cfg.ticketsDir, cfg.agentFilter)
		if err != nil {
			return fmt.Errorf("error finding incomplete tickets: %w", err)
		}

		if len(remaining) == 0 {
			fmt.Println("All tasks completed, checking again in 10s...")
			if err := sleepWithContext(ctx, 5*time.Second); err != nil {
				os.Exit(130)
			}
			continue
		}

		fmt.Println("Remaining:")
		for _, f := range remaining {
			fmt.Println(filepath.Base(f))
		}

		if agentPromptFile != "" {
			fmt.Printf("Running prompts/prompt.md + %s\n", agentPromptFile)
		} else {
			fmt.Println("Running prompts/prompt.md")
		}

		// Increment CurIteration for tickets that have MinIterations
		for _, f := range remaining {
			incrementCurIteration(f)
		}

		prompt, err := cfg.promptLoader.Load(cfg.baseDir, "prompts/prompt.md", "Remaining tickets:", remaining, agentPromptFile)
		if err != nil {
			return fmt.Errorf("error building work prompt: %w", err)
		}
		prompt = strings.ReplaceAll(prompt, "{{SHORTCUTS_PATH}}", cfg.shortcutsFile)

		exitCode, err := cfg.runner.Run(ctx, prompt, cfg.claudeArgs)
		if err != nil {
			return err
		}

		// Check iteration thresholds and reset status if needed
		checkAndResetIterations(remaining)

		if exitCode == 0 {
			return nil
		}
		fmt.Printf("Claude exited with code %d, retrying in 5s...\n", exitCode)
		if err := sleepWithContext(ctx, 5*time.Second); err != nil {
			os.Exit(130)
		}
	}
}

// resolveBaseDir finds the wiggums directory by checking the executable's
// location first, then falling back to the current working directory.
func resolveBaseDir() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		real, err := filepath.EvalSymlinks(exe)
		if err == nil {
			dir := filepath.Dir(real)
			if hasSubdirs(dir, "tickets", "prompts") {
				return dir, nil
			}
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return cwd, nil
}

func hasSubdirs(base string, names ...string) bool {
	for _, name := range names {
		info, err := os.Stat(filepath.Join(base, name))
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

// findUnverifiedTickets finds .md files in ticketsDir (recursively) that:
// - are not CLAUDE.md
// - were modified within the last 60 minutes
// - contain "status: completed" (case-insensitive)
// - do NOT have "completed + verified" in their YAML frontmatter status
func findUnverifiedTickets(ticketsDir string) ([]string, error) {
	var results []string
	cutoff := time.Now().Add(-60 * time.Minute)

	err := filepath.WalkDir(ticketsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") || strings.EqualFold(d.Name(), "CLAUDE.md") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lower := strings.ToLower(string(content))
		if !strings.Contains(lower, "status: completed") {
			return nil
		}

		status := extractFrontmatterStatus(string(content))
		if !strings.Contains(strings.ToLower(status), "completed + verified") {
			results = append(results, path)
		}
		return nil
	})

	return results, err
}

// findIncompleteTickets finds .md files in ticketsDir (recursively) that
// do NOT contain "status: completed" (case-insensitive), excluding CLAUDE.md.
// When agentFilter is empty (default run), tickets with a non-empty Agent field are excluded.
// When agentFilter is set, only tickets whose Agent field matches are included.
func findIncompleteTickets(ticketsDir string, agentFilter string) ([]string, error) {
	var results []string

	err := filepath.WalkDir(ticketsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") || strings.EqualFold(d.Name(), "CLAUDE.md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		contentStr := string(content)
		if strings.Contains(strings.ToLower(contentStr), "status: completed") {
			return nil
		}

		ticketAgent := extractFrontmatterAgent(contentStr)

		if agentFilter == "" {
			// Default run: exclude tickets that have an agent assigned
			if ticketAgent != "" {
				return nil
			}
		} else {
			// Agent run: only include tickets matching the agent filter
			if !strings.EqualFold(ticketAgent, agentFilter) {
				return nil
			}
		}

		results = append(results, path)
		return nil
	})

	return results, err
}

// extractFrontmatterStatus extracts the status value from YAML frontmatter
// (the section between the first and second --- delimiters).
func extractFrontmatterStatus(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	delimCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			delimCount++
			if delimCount >= 2 {
				break
			}
			continue
		}
		if delimCount == 1 {
			lower := strings.ToLower(line)
			if strings.HasPrefix(strings.TrimSpace(lower), "status:") {
				return line
			}
		}
	}
	return ""
}

// extractFrontmatterAgent extracts the agent value from YAML frontmatter.
// Returns the trimmed agent name, or empty string if not set.
func extractFrontmatterAgent(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	delimCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			delimCount++
			if delimCount >= 2 {
				break
			}
			continue
		}
		if delimCount == 1 {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, "agent:") {
				// Extract value after "Agent:"
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
				return ""
			}
		}
	}
	return ""
}

// extractFrontmatterInt extracts an integer value for the given key from YAML frontmatter.
// Returns 0 if the key is not found or not a valid integer.
func extractFrontmatterInt(content string, key string) int {
	scanner := bufio.NewScanner(strings.NewReader(content))
	delimCount := 0
	keyLower := strings.ToLower(key) + ":"
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			delimCount++
			if delimCount >= 2 {
				break
			}
			continue
		}
		if delimCount == 1 {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, keyLower) {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					val, err := strconv.Atoi(strings.TrimSpace(parts[1]))
					if err == nil {
						return val
					}
				}
			}
		}
	}
	return 0
}

// incrementCurIteration reads a ticket file, increments CurIteration in the
// frontmatter (adding it if missing), and writes the file back.
func incrementCurIteration(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	delimCount := 0
	found := false
	secondDelimIdx := -1

	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			delimCount++
			if delimCount == 2 {
				secondDelimIdx = i
				break
			}
			continue
		}
		if delimCount == 1 {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, "curiteration:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					cur, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
					lines[i] = parts[0] + ": " + strconv.Itoa(cur+1)
					found = true
				}
			}
		}
	}

	if !found && secondDelimIdx > 0 {
		// Insert CurIteration: 1 before the closing ---
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:secondDelimIdx]...)
		newLines = append(newLines, "CurIteration: 1")
		newLines = append(newLines, lines[secondDelimIdx:]...)
		lines = newLines
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// checkAndResetIterations checks all tickets that were just worked on.
// If a ticket has MinIterations set and CurIteration < MinIterations,
// and its status is "completed", reset it to "in_progress".
func checkAndResetIterations(tickets []string) {
	for _, path := range tickets {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		contentStr := string(content)
		minIter := extractFrontmatterInt(contentStr, "MinIterations")
		if minIter == 0 {
			continue
		}
		curIter := extractFrontmatterInt(contentStr, "CurIteration")
		if curIter >= minIter {
			continue
		}

		// Check if status was set to completed
		status := extractFrontmatterStatus(contentStr)
		if !strings.Contains(strings.ToLower(status), "completed") {
			continue
		}

		// Reset status back to in_progress
		lines := strings.Split(contentStr, "\n")
		delimCount := 0
		for i, line := range lines {
			if strings.TrimSpace(line) == "---" {
				delimCount++
				continue
			}
			if delimCount == 1 {
				lower := strings.ToLower(strings.TrimSpace(line))
				if strings.HasPrefix(lower, "status:") {
					lines[i] = "Status: in_progress"
					break
				}
			}
		}
		fmt.Printf("Ticket %s: iteration %d/%d, resetting status to in_progress\n", filepath.Base(path), curIter, minIter)
		os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
	}
}

// sleepWithContext sleeps for the given duration, returning an error if the
// context is cancelled (e.g. Ctrl+C).
func sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
