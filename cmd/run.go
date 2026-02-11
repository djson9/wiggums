package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wiggums/database"
)

// startLoop wires up real implementations and starts the main loop.
// ticketsDirOverride and workDir are optional; when empty, defaults are used.
func startLoop(args []string, agentFilter, ticketsDirOverride, workDir, workspaceName string) error {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return fmt.Errorf("could not resolve base directory: %w", err)
	}

	claudeArgs := args
	if yolo {
		claudeArgs = append([]string{"--model", "opus", "--dangerously-skip-permissions"}, claudeArgs...)
	}
	runnerWorkDir := baseDir
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
		runner:        &ClaudeRunner{WorkDir: runnerWorkDir},
		promptLoader:  &FilePromptLoader{},
		notifier:      &BeeepNotifier{},
		baseDir:       baseDir,
		ticketsDir:    ticketsDir,
		claudeArgs:    claudeArgs,
		agentFilter:   agentFilter,
		ticketFilter:  ticketFlag,
		workDir:       workDir,
		shortcutsFile: shortcutsFile,
		workspaceName: workspaceName,
	}

	// Initialize database for run tracking
	if err := database.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not init database: %v\n", err)
	} else {
		defer database.Close()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Migrate DB schema
	if database.DB != nil {
		if err := database.Migrate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not migrate database: %v\n", err)
		}
		// Close any stale open runs from previous crashes
		if workspaceName != "" {
			_ = database.CompleteOpenRuns(ctx, workspaceName, -1)
		}
	}

	return runLoop(ctx, cfg)
}

// runLoop is the main ticket processing loop. It is separated from cobra
// wiring so it can be tested with mock Runner and PromptLoader.
func runLoop(ctx context.Context, cfg *loopConfig) error {
	fmt.Printf("Using tickets directory: %s\n", cfg.ticketsDir)
	if cfg.agentFilter != "" {
		fmt.Printf("Agent filter: %s\n", cfg.agentFilter)
	}
	if cfg.ticketFilter != "" {
		fmt.Printf("Ticket filter: %s\n", cfg.ticketFilter)
	}
	if len(cfg.claudeArgs) > 0 {
		fmt.Printf("Claude args: %s\n", strings.Join(cfg.claudeArgs, " "))
	}

	// Clean up state file when the loop exits
	if cfg.workspaceName != "" {
		defer clearState(cfg.baseDir, cfg.workspaceName)
	}

	for {
		select {
		case <-ctx.Done():
			os.Exit(130)
		default:
		}

		if _, err := os.Stat(cfg.ticketsDir); os.IsNotExist(err) {
			fmt.Println("Error: No tickets directory found")
			os.Exit(1)
		}

		// Find recently completed but unverified tickets
		unverified, err := findUnverifiedTickets(cfg.ticketsDir, cfg.ticketFilter)
		if err != nil {
			return fmt.Errorf("error finding unverified tickets: %w", err)
		}

		// Determine agent prompt file path (empty string if no agent)
		agentPromptFile := ""
		if cfg.agentFilter != "" {
			agentPromptFile = filepath.Join("agents", cfg.agentFilter+".md")
		}

		if len(unverified) > 0 {
			verifyFile := "prompts/verify.md"
			wsVerifyPrompt := loadWorkspacePrompt(cfg.baseDir, cfg.workspaceName, "prompts/verify.md")
			if wsVerifyPrompt != "" {
				fmt.Printf("Running %s + workspace prompt\n", verifyFile)
			} else {
				fmt.Printf("Running %s\n", verifyFile)
			}
			for _, f := range unverified {
				fmt.Println(filepath.Base(f))
			}

			// Write state: verifying
			if cfg.workspaceName != "" {
				names := make([]string, len(unverified))
				for i, f := range unverified {
					names[i] = filepath.Base(f)
				}
				writeState(cfg.baseDir, cfg.workspaceName, "verifying", strings.Join(names, ", "))
			}

			// Track runs in DB
			var runIDs []int64
			if database.DB != nil && cfg.workspaceName != "" {
				for _, f := range unverified {
					if id, err := database.StartRun(ctx, cfg.workspaceName, filepath.Base(f), "verifying", ticketCreatedAtPtr(filepath.Base(f))); err == nil {
						runIDs = append(runIDs, id)
					}
				}
			}

			prompt, err := cfg.promptLoader.Load(cfg.baseDir, verifyFile, "Tickets to verify:", unverified, agentPromptFile)
			if err != nil {
				return fmt.Errorf("error building verify prompt: %w", err)
			}
			if wsVerifyPrompt != "" {
				prompt += "\n\n" + wsVerifyPrompt
			}
			prompt = strings.ReplaceAll(prompt, "{{SHORTCUTS_PATH}}", cfg.shortcutsFile)

			exitCode, err := cfg.runner.Run(ctx, prompt, cfg.claudeArgs)
			if err != nil {
				completeRunIDs(ctx, runIDs, 1)
				return err
			}
			completeRunIDs(ctx, runIDs, exitCode)
			updateTicketsTimestamp(unverified)
			if exitCode == 0 {
				// Check if any verified tickets need more iterations
				resetOccurred := checkAndResetIterations(unverified)
				if resetOccurred {
					fmt.Println("MinIterations not yet met after verification, continuing loop...")
					continue
				}
				names := make([]string, len(unverified))
				for i, f := range unverified {
					names[i] = filepath.Base(f)
				}
				if cfg.notifier != nil {
				_ = cfg.notifier.Notify("Wiggums: Ticket Verified", strings.Join(names, ", "), "")
			}
				return nil
			}
			fmt.Printf("Claude exited with code %d, retrying in 5s...\n", exitCode)
			if err := sleepWithContext(ctx, 5*time.Second); err != nil {
				os.Exit(130)
			}
			continue
		}

		// Find remaining incomplete tickets (filtered by agent)
		allRemaining, err := findIncompleteTickets(cfg.ticketsDir, cfg.agentFilter, cfg.ticketFilter)
		if err != nil {
			return fmt.Errorf("error finding incomplete tickets: %w", err)
		}

		if len(allRemaining) == 0 {
			// Write state: idle
			if cfg.workspaceName != "" {
				writeState(cfg.baseDir, cfg.workspaceName, "idle", "")
			}
			fmt.Println("All tasks completed.")
			return nil
		}

		// Only work on one ticket at a time so that the DB started_at
		// timestamp accurately reflects when this specific ticket began.
		remaining := allRemaining[0:1]

		fmt.Printf("Next ticket: %s\n", filepath.Base(remaining[0]))
		if len(allRemaining) > 1 {
			fmt.Printf("(%d more ticket(s) in backlog)\n", len(allRemaining)-1)
		}

		workFile := "prompts/prompt.md"
		wsWorkPrompt := loadWorkspacePrompt(cfg.baseDir, cfg.workspaceName, "prompts/prompt.md")
		if agentPromptFile != "" {
			if wsWorkPrompt != "" {
				fmt.Printf("Running %s + workspace prompt + %s\n", workFile, agentPromptFile)
			} else {
				fmt.Printf("Running %s + %s\n", workFile, agentPromptFile)
			}
		} else {
			if wsWorkPrompt != "" {
				fmt.Printf("Running %s + workspace prompt\n", workFile)
			} else {
				fmt.Printf("Running %s\n", workFile)
			}
		}

		// Increment CurIteration for the ticket being worked on
		incrementCurIteration(remaining[0])

		// Write state: working
		if cfg.workspaceName != "" {
			writeState(cfg.baseDir, cfg.workspaceName, "working", filepath.Base(remaining[0]))
		}

		// Track run in DB — started_at is now accurate per-ticket
		var runIDs []int64
		if database.DB != nil && cfg.workspaceName != "" {
			if id, err := database.StartRun(ctx, cfg.workspaceName, filepath.Base(remaining[0]), "working", ticketCreatedAtPtr(filepath.Base(remaining[0]))); err == nil {
				runIDs = append(runIDs, id)
			}
		}

		prompt, err := cfg.promptLoader.Load(cfg.baseDir, workFile, "Next ticket:", remaining, agentPromptFile)
		if err != nil {
			return fmt.Errorf("error building work prompt: %w", err)
		}
		if wsWorkPrompt != "" {
			prompt += "\n\n" + wsWorkPrompt
		}
		prompt = strings.ReplaceAll(prompt, "{{SHORTCUTS_PATH}}", cfg.shortcutsFile)

		exitCode, err := cfg.runner.Run(ctx, prompt, cfg.claudeArgs)
		if err != nil {
			completeRunIDs(ctx, runIDs, 1)
			return err
		}
		completeRunIDs(ctx, runIDs, exitCode)
		updateTicketsTimestamp(remaining)

		// Check iteration thresholds and reset status if needed
		resetOccurred := checkAndResetIterations(remaining)

		if exitCode == 0 {
			if resetOccurred {
				fmt.Println("MinIterations not yet met, continuing loop...")
				continue
			}
			// Even if no reset occurred, check if any tickets still have remaining iterations
			if hasRemainingIterations(remaining) {
				fmt.Println("Tickets still have remaining iterations, continuing loop...")
				continue
			}
			// Continue the loop so verification can run on the next iteration.
			// findUnverifiedTickets() at the top of the loop will pick up the
			// just-completed ticket and run the verification prompt.
			fmt.Println("Work pass completed, continuing to verification...")
			continue
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
			if hasSubdirs(dir, "tickets", "prompts") || hasSubdirs(dir, "workspaces", "prompts") {
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
func findUnverifiedTickets(ticketsDir string, ticketFilter string) ([]string, error) {
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

		// Filter by ticket name if specified
		if ticketFilter != "" {
			if !strings.Contains(strings.ToLower(d.Name()), strings.ToLower(ticketFilter)) {
				return nil
			}
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
		contentStr := string(content)
		status := strings.ToLower(extractFrontmatterStatus(contentStr))
		if !strings.Contains(status, "completed") {
			return nil
		}
		if strings.Contains(status, "completed + verified") {
			return nil
		}
		// If SkipVerification is true, auto-mark as verified — but only if
		// MinIterations is satisfied (or not set).
		if extractFrontmatterBool(contentStr, "SkipVerification") {
			minIter := extractFrontmatterInt(contentStr, "MinIterations")
			curIter := extractFrontmatterInt(contentStr, "CurIteration")
			if minIter > 0 && curIter < minIter {
				// Iterations not yet met — reset to in_progress instead of verifying
				fmt.Printf("Ticket %s: SkipVerification=true but iteration %d/%d not met, resetting to in_progress\n", d.Name(), curIter, minIter)
				resetStatusToInProgress(path)
				return nil
			}
			fmt.Printf("Ticket %s: SkipVerification=true, auto-marking as verified\n", d.Name())
			markAsVerified(path)
			return nil
		}
		results = append(results, path)
		return nil
	})

	return results, err
}

// findIncompleteTickets finds .md files in ticketsDir (recursively) that
// do NOT contain "status: completed" (case-insensitive), excluding CLAUDE.md.
// When agentFilter is empty (default run), tickets with a non-empty Agent field are excluded.
// When agentFilter is set, only tickets whose Agent field matches are included.
func findIncompleteTickets(ticketsDir string, agentFilter string, ticketFilter string) ([]string, error) {
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

		// Filter by ticket name if specified
		if ticketFilter != "" {
			if !strings.Contains(strings.ToLower(d.Name()), strings.ToLower(ticketFilter)) {
				return nil
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		contentStr := string(content)

		// If ticket has remaining iterations (minIterations - curIterations > 0),
		// always include it regardless of status.
		minIter := extractFrontmatterInt(contentStr, "MinIterations")
		curIter := extractFrontmatterInt(contentStr, "CurIteration")
		remainingIter := minIter - curIter

		if strings.Contains(strings.ToLower(contentStr), "status: completed") {
			if remainingIter > 0 {
				// Iterations still pending — reset status so it gets processed
				fmt.Printf("Ticket %s: %d iterations remaining, resetting to in_progress\n", d.Name(), remainingIter)
				resetStatusToInProgress(path)
			} else {
				return nil
			}
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

// ticketCreatedAtPtr parses the epoch from a ticket filename and returns
// a *time.Time pointer suitable for database.StartRun. Returns nil if
// the filename doesn't contain a valid epoch prefix.
func ticketCreatedAtPtr(filename string) *time.Time {
	t := parseEpochFromFilename(filename)
	if t.IsZero() {
		return nil
	}
	return &t
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
					raw := strings.TrimSpace(parts[1])
				// Strip surrounding quotes (e.g. "8" → 8)
				raw = strings.Trim(raw, "\"'")
				val, err := strconv.Atoi(raw)
					if err == nil {
						return val
					}
				}
			}
		}
	}
	return 0
}

// extractFrontmatterBool extracts a boolean value for the given key from YAML frontmatter.
// Returns true for values like "true", "yes", "1" (case-insensitive). Returns false otherwise.
func extractFrontmatterBool(content string, key string) bool {
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
					val := strings.ToLower(strings.TrimSpace(parts[1]))
					return val == "true" || val == "yes" || val == "1"
				}
			}
		}
	}
	return false
}

// markAsVerified reads a ticket file and updates its status from "completed" to "completed + verified".
func markAsVerified(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

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
				lines[i] = "Status: completed + verified"
				break
			}
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// resetStatusToInProgress reads a ticket file and sets its status to "in_progress".
func resetStatusToInProgress(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

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
				lines[i] = "Status: in_progress"
				break
			}
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// setStatusToAdditionalUserRequest reads a ticket file and sets its status to "additional_user_request".
// Called by the worker at run time when processing an additional request item (RequestNum > 0).
func setStatusToAdditionalUserRequest(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

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
				lines[i] = "Status: additional_user_request"
				break
			}
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// restoreFrontmatterStatus reads a ticket file and sets its frontmatter Status field to the given value.
// Used to restore the original ticket status after the worker processes an additional request,
// ensuring the ticket's historical status is immutable.
func restoreFrontmatterStatus(path string, status string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

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
				lines[i] = "Status: " + status
				break
			}
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// writeAdditionalRequestContentToFile writes the content of an additional request from SQLite
// into the ticket file at runtime. This is called by the worker before processing an additional
// request whose content was deferred (e.g., drafts that were stored only in SQLite).
// It's idempotent: if the section already exists in the file, it does nothing.
func writeAdditionalRequestContentToFile(path string, requestNum int) error {
	if database.DB == nil {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	contentStr := string(content)

	// Check if the section already exists in the file (idempotent)
	sectionHeader := fmt.Sprintf("### Additional User Request #%d", requestNum)
	if strings.Contains(contentStr, sectionHeader) {
		return nil
	}

	// Read content from SQLite
	reqContent, err := database.GetAdditionalRequestContent(context.Background(), path, requestNum)
	if err != nil {
		return fmt.Errorf("could not read additional request content from DB: %w", err)
	}
	if reqContent == "" {
		return nil // No content to write
	}

	// Remove placeholder text
	contentStr = strings.Replace(contentStr, "To be populated with further user request\n", "", 1)

	// Build the new section
	timestamp := time.Now().Format("2006-01-02 15:04")
	newSection := fmt.Sprintf("\n### Additional User Request #%d — %s\n%s\n", requestNum, timestamp, reqContent)

	// Insert before the divider
	divider := "---\nBelow to be filled by agent"
	idx := strings.Index(contentStr, divider)
	if idx != -1 {
		contentStr = contentStr[:idx] + newSection + contentStr[idx:]
	} else {
		contentStr += newSection
	}

	return os.WriteFile(path, []byte(contentStr), 0644)
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
// Returns true if any ticket was reset (so the loop should continue).
func checkAndResetIterations(tickets []string) bool {
	anyReset := false
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
		anyReset = true
	}
	return anyReset
}

// hasRemainingIterations checks if any ticket still has minIterations - curIterations > 0.
func hasRemainingIterations(tickets []string) bool {
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
		if minIter-curIter > 0 {
			return true
		}
	}
	return false
}

// updateUpdatedAt sets the UpdatedAt field in a ticket's frontmatter to the
// current local time. If the field doesn't exist, it is inserted before the
// closing --- delimiter.
func updateUpdatedAt(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	now := time.Now().Format("2006-01-02 15:04")
	lines := strings.Split(string(content), "\n")
	delimCount := 0
	found := false
	secondDelimIdx := -1

	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			delimCount++
			if delimCount == 2 {
				secondDelimIdx = i
				if !found {
					break
				}
			}
			continue
		}
		if delimCount == 1 {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, "updatedat:") {
				lines[i] = "UpdatedAt: " + now
				found = true
			}
		}
	}

	if !found && secondDelimIdx > 0 {
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:secondDelimIdx]...)
		newLines = append(newLines, "UpdatedAt: "+now)
		newLines = append(newLines, lines[secondDelimIdx:]...)
		lines = newLines
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// updateTicketsTimestamp updates UpdatedAt for all given ticket paths.
func updateTicketsTimestamp(tickets []string) {
	for _, path := range tickets {
		if err := updateUpdatedAt(path); err != nil {
			fmt.Printf("Warning: could not update UpdatedAt for %s: %v\n", filepath.Base(path), err)
		}
	}
}

// workspacePromptPath returns the relative path to a workspace-scoped prompt
// if it exists, or empty string if not found.
// For example, if workspaceName is "skyvern" and promptFile is "prompts/prompt.md",
// it checks "workspaces/skyvern/prompts/prompt.md".
func workspacePromptPath(baseDir, workspaceName, promptFile string) string {
	if workspaceName != "" {
		wsPath := filepath.Join("workspaces", workspaceName, promptFile)
		if _, err := os.Stat(filepath.Join(baseDir, wsPath)); err == nil {
			return wsPath
		}
	}
	return ""
}

// loadWorkspacePrompt reads the workspace-scoped prompt content if it exists.
// Returns the content string (empty if no workspace prompt).
func loadWorkspacePrompt(baseDir, workspaceName, promptFile string) string {
	wsPath := workspacePromptPath(baseDir, workspaceName, promptFile)
	if wsPath == "" {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(baseDir, wsPath))
	if err != nil {
		return ""
	}
	return string(content)
}

// resolvePromptFile is a backward-compatible wrapper that returns the
// workspace prompt path if it exists, otherwise the root prompt path.
// Deprecated: prefer workspacePromptPath + loadWorkspacePrompt for append behavior.
func resolvePromptFile(baseDir, workspaceName, promptFile string) string {
	if ws := workspacePromptPath(baseDir, workspaceName, promptFile); ws != "" {
		return ws
	}
	return promptFile
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

// WorkspaceState represents the current activity of a workspace's run loop.
type WorkspaceState struct {
	Status    string `json:"status"`               // "working", "verifying", "idle"
	Ticket    string `json:"ticket"`               // ticket filename (empty when idle)
	StartedAt string `json:"started_at,omitempty"` // RFC3339 timestamp when work started
}

// stateDir returns the path to the .state directory under baseDir, creating it if needed.
func stateDir(baseDir string) string {
	dir := filepath.Join(baseDir, ".state")
	os.MkdirAll(dir, 0755)
	return dir
}

// writeState writes the workspace state to a JSON file.
func writeState(baseDir, workspaceName, status, ticket string) {
	startedAt := ""
	if status == "working" || status == "verifying" {
		// Preserve existing started_at if already working, otherwise set now
		existing := readState(baseDir, workspaceName)
		if existing.StartedAt != "" && (existing.Status == "working" || existing.Status == "verifying") {
			startedAt = existing.StartedAt
		} else {
			startedAt = time.Now().Format(time.RFC3339)
		}
	}
	state := WorkspaceState{Status: status, Ticket: ticket, StartedAt: startedAt}
	data, _ := json.Marshal(state)
	path := filepath.Join(stateDir(baseDir), workspaceName+".json")
	os.WriteFile(path, data, 0644)
}

// readState reads the workspace state from its JSON file.
// Returns a zero-value WorkspaceState if the file doesn't exist or can't be read.
func readState(baseDir, workspaceName string) WorkspaceState {
	path := filepath.Join(baseDir, ".state", workspaceName+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceState{}
	}
	var state WorkspaceState
	if err := json.Unmarshal(data, &state); err != nil {
		return WorkspaceState{}
	}
	return state
}

// clearState removes the workspace state file.
func clearState(baseDir, workspaceName string) {
	path := filepath.Join(baseDir, ".state", workspaceName+".json")
	os.Remove(path)
}

// completeRunIDs marks a batch of ticket runs as completed in the DB.
func completeRunIDs(ctx context.Context, ids []int64, exitCode int) {
	if database.DB == nil {
		return
	}
	for _, id := range ids {
		_ = database.CompleteRun(ctx, id, exitCode)
	}
}
