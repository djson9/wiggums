package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wiggums/database"
)

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

// loadWorkspacePrompt reads the workspace-scoped prompt content if it exists.
// Returns the content string (empty if no workspace prompt).
func loadWorkspacePrompt(baseDir, workspaceName, promptFile string) string {
	if workspaceName == "" {
		return ""
	}
	wsPath := filepath.Join("workspaces", workspaceName, promptFile)
	content, err := os.ReadFile(filepath.Join(baseDir, wsPath))
	if err != nil {
		return ""
	}
	return string(content)
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
