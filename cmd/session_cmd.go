package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"wiggums/database"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// ANSI color codes
const (
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiDim       = "\033[2m"
	ansiDimYellow = "\033[33m"
	ansiBold   = "\033[1m"
	ansiReset  = "\033[0m"

	// Obsidian vault name (the wiggums directory is the vault)
	obsidianVault = "wiggums"
	ansiCyan   = "\033[36m"
)

// isPaneIdle returns true if the pane's current command is a shell (not wiggums/claude).
func isPaneIdle(paneCmd string) bool {
	switch paneCmd {
	case "zsh", "bash", "fish", "sh", "dash", "":
		return true
	}
	return false
}

// obsidianLink wraps text in an OSC 8 terminal hyperlink to the Obsidian note.
// Only emits links when stdout is a real terminal (not piped through watch).
func obsidianLink(text, workspaceName, filename string) string {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return text
	}
	url := rawObsidianURL(workspaceName, filename)
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
}

// rawObsidianURL returns the plain obsidian:// URL for a ticket.
func rawObsidianURL(workspaceName, filename string) string {
	filePath := fmt.Sprintf("workspaces/%s/tickets/%s", workspaceName, strings.TrimSuffix(filename, ".md"))
	return fmt.Sprintf("obsidian://open?vault=%s&file=%s", obsidianVault, filePath)
}

// rawObsidianIndexURL returns the plain obsidian:// URL for a workspace's index.md.
func rawObsidianIndexURL(workspaceName string) string {
	filePath := fmt.Sprintf("workspaces/%s/index", workspaceName)
	return fmt.Sprintf("obsidian://open?vault=%s&file=%s", obsidianVault, filePath)
}

// ticketTitle converts a ticket filename to a human-readable title.
// "1771436012_AGSUP-2809_MD_SUI_Registration_Request.md" -> "AGSUP-2809 MD SUI Registration Request"
func ticketTitle(filename string) string {
	name := strings.TrimSuffix(filename, ".md")

	// Strip leading epoch seconds (digits followed by underscore)
	if idx := strings.IndexByte(name, '_'); idx > 0 {
		prefix := name[:idx]
		allDigits := true
		for _, r := range prefix {
			if !unicode.IsDigit(r) {
				allDigits = false
				break
			}
		}
		if allDigits {
			name = name[idx+1:]
		}
	}

	return strings.ReplaceAll(name, "_", " ")
}

// TicketInfo holds parsed ticket metadata for display.
type TicketInfo struct {
	Filename  string
	Title     string
	Status    string // raw status from frontmatter
	CreatedAt time.Time
	UpdatedAt time.Time
}

// scanWorkspaceTickets reads ticket files from a workspace's tickets dir
// and returns categorized slices: in-progress and recently completed (last 24h).
func scanWorkspaceTickets(baseDir, workspaceName string) (inProgress []TicketInfo, completed []TicketInfo) {
	ticketsDir := filepath.Join(baseDir, "workspaces", workspaceName, "tickets")
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-24 * time.Hour)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.EqualFold(e.Name(), "CLAUDE.md") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(ticketsDir, e.Name()))
		if err != nil {
			continue
		}
		contentStr := string(content)

		status := strings.ToLower(extractFrontmatterStatus(contentStr))
		updatedAtStr := extractFrontmatterField(contentStr, "updatedat")
		dateStr := extractFrontmatterField(contentStr, "date")

		var updatedAt time.Time
		if updatedAtStr != "" {
			if t, err := time.ParseInLocation("2006-01-02 15:04", updatedAtStr, time.Local); err == nil {
				updatedAt = t
			}
		}

		var createdAt time.Time
		if dateStr != "" {
			// Try "2006-01-02 15:04" then "2006-01-02"
			if t, err := time.ParseInLocation("2006-01-02 15:04", dateStr, time.Local); err == nil {
				createdAt = t
			} else if t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local); err == nil {
				createdAt = t
			}
		}

		info := TicketInfo{
			Filename:  e.Name(),
			Title:     ticketTitle(e.Name()),
			Status:    status,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}

		if strings.Contains(status, "completed") {
			if !updatedAt.IsZero() && updatedAt.After(cutoff) {
				completed = append(completed, info)
			}
		} else {
			inProgress = append(inProgress, info)
		}
	}

	// Sort completed by UpdatedAt descending (most recent first)
	sort.Slice(completed, func(i, j int) bool {
		return completed[i].UpdatedAt.After(completed[j].UpdatedAt)
	})

	return
}

// hasRecentActivity returns true if any ticket in the workspace was updated after cutoff.
func hasRecentActivity(baseDir, workspaceName string, cutoff time.Time) bool {
	// Check DB first
	if database.DB != nil {
		if has, err := database.HasRecentRuns(context.Background(), workspaceName, cutoff); err == nil {
			return has
		}
	}

	// Fall back to frontmatter scanning
	ticketsDir := filepath.Join(baseDir, "workspaces", workspaceName, "tickets")
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.EqualFold(e.Name(), "CLAUDE.md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(ticketsDir, e.Name()))
		if err != nil {
			continue
		}
		updatedAtStr := extractFrontmatterField(string(content), "updatedat")
		if updatedAtStr != "" {
			if t, err := time.ParseInLocation("2006-01-02 15:04", updatedAtStr, time.Local); err == nil {
				if t.After(cutoff) {
					return true
				}
			}
		}
	}
	return false
}

// hasDBRuns returns true if the DB has any runs recorded for this workspace.
func hasDBRuns(workspace string) bool {
	if database.DB == nil {
		return false
	}
	runs, err := database.RecentRuns(context.Background(), workspace, 1)
	return err == nil && len(runs) > 0
}

// renderTicketsFromDB renders ticket details using the DB run history.
func renderTicketsFromDB(workspace string) {
	ctx := context.Background()

	// Active (in-progress) runs
	active, err := database.ActiveRuns(ctx, workspace)
	if err == nil {
		for _, r := range active {
			title := obsidianLink(ticketTitle(r.Ticket), workspace, r.Ticket)
			elapsed := ansiDimYellow + formatElapsed(time.Since(r.StartedAt)) + ansiReset
			fmt.Printf("      %s%s%s  %s\n", ansiGreen, title, ansiReset, elapsed)
			fmt.Printf("      %s%s%s\n", ansiDim, rawObsidianURL(workspace, r.Ticket), ansiReset)
		}
	}

	// Recently completed runs (last 24h)
	since := time.Now().Add(-24 * time.Hour)
	completed, err := database.RecentCompletedRuns(ctx, workspace, since)
	if err == nil && len(completed) > 0 {
		// Deduplicate: only show the most recent run per ticket
		seen := make(map[string]bool)
		var unique []database.TicketRun
		for _, r := range completed {
			if !seen[r.Ticket] {
				seen[r.Ticket] = true
				unique = append(unique, r)
			}
		}

		fmt.Printf("\n      %s── Completed ──%s\n", ansiDim, ansiReset)
		for _, r := range unique {
			title := obsidianLink(ticketTitle(r.Ticket), workspace, r.Ticket)
			duration := ""
			if r.CompletedAt != nil {
				duration = formatElapsed(r.CompletedAt.Sub(r.StartedAt))
			}
			timeStr := ""
			if r.CompletedAt != nil {
				timeStr = formatTimeET(*r.CompletedAt)
			}
			fmt.Printf("      %s%s%s  %s%s  %s%s\n",
				ansiDim, title, ansiReset,
				ansiDim, timeStr, duration, ansiReset)
			fmt.Printf("      %s%s%s\n", ansiDim, rawObsidianURL(workspace, r.Ticket), ansiReset)
		}
	}
}

// renderTicketsFromFrontmatter renders ticket details using frontmatter (fallback).
func renderTicketsFromFrontmatter(baseDir, workspace string) {
	inProgress, completed := scanWorkspaceTickets(baseDir, workspace)

	// Use workspace state's StartedAt for current run elapsed time
	state := readState(baseDir, workspace)
	var runStart time.Time
	if state.StartedAt != "" {
		runStart, _ = time.Parse(time.RFC3339, state.StartedAt)
	}

	for _, t := range inProgress {
		title := obsidianLink(t.Title, workspace, t.Filename)
		elapsed := ""
		if !runStart.IsZero() {
			elapsed = "  " + ansiDimYellow + formatElapsed(time.Since(runStart)) + ansiReset
		}
		fmt.Printf("      %s%s%s%s\n", ansiGreen, title, ansiReset, elapsed)
		fmt.Printf("      %s%s%s\n", ansiDim, rawObsidianURL(workspace, t.Filename), ansiReset)
	}

	if len(completed) > 0 {
		fmt.Printf("\n      %s── Completed ──%s\n", ansiDim, ansiReset)
		for _, t := range completed {
			title := obsidianLink(t.Title, workspace, t.Filename)
			timeStr := ""
			if !t.UpdatedAt.IsZero() {
				timeStr = ansiDim + formatTimeET(t.UpdatedAt) + ansiReset
			}
			fmt.Printf("      %s%s%s  %s\n", ansiDim, title, ansiReset, timeStr)
			fmt.Printf("      %s%s%s\n", ansiDim, rawObsidianURL(workspace, t.Filename), ansiReset)
		}
	}
}

// extractFrontmatterField extracts a named field value from YAML frontmatter.
func extractFrontmatterField(content, key string) string {
	keyLower := strings.ToLower(key) + ":"
	lines := strings.Split(content, "\n")
	delimCount := 0
	for _, line := range lines {
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
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return ""
}

// formatElapsed formats a duration as a human-readable string like "25m" or "1h 30m".
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// formatTimeET formats a time as "3:04 PM ET".
func formatTimeET(t time.Time) string {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.Local
	}
	return t.In(loc).Format("3:04 PM") + " ET"
}

func init() {
	sessionCmd.AddCommand(sessionLsCmd)
	sessionCmd.AddCommand(sessionAttachCmd)
	sessionCmd.AddCommand(sessionPaneCmd)
	rootCmd.AddCommand(sessionCmd)
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage wiggums tmux sessions",
}

var sessionLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List running wiggums workspace sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		sm := NewSessionManager()

		if !sm.SessionExists() {
			fmt.Println("No wiggums tmux session running.")
			fmt.Println("Start one with: wiggums start")
			return nil
		}

		// Init DB for run history
		if err := database.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not init database: %v\n", err)
		} else {
			defer database.Close()
			if err := database.Migrate(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not migrate database: %v\n", err)
			}
		}

		baseDir, err := resolveBaseDir()
		if err != nil {
			return err
		}
		allWorkspaces, err := listWorkspaceNames(baseDir)
		if err != nil {
			return err
		}

		windows, err := sm.ListWindowsDetailed()
		if err != nil {
			return err
		}

		// Build maps
		windowIndex := make(map[string]int)
		windowDead := make(map[string]bool)
		windowPaneCmd := make(map[string]string)
		for _, w := range windows {
			windowIndex[w.Name] = w.Index
			windowDead[w.Name] = w.Dead
			windowPaneCmd[w.Name] = w.PaneCmd
		}

		fmt.Printf("tmux session: %s\n\n", sm.SessionName)

			hideCutoff := time.Now().Add(-8 * time.Hour)
		hidden := 0

		for _, ws := range allWorkspaces {
			idx, hasWindow := windowIndex[ws]

			// Determine if this workspace is inactive (stopped, idle, not started, exited)
			isInactive := !hasWindow || windowDead[ws] || isPaneIdle(windowPaneCmd[ws])
			if !isInactive {
				state := readState(baseDir, ws)
				if state.Status == "idle" {
					isInactive = true
				}
			}

			// For inactive workspaces, check if any ticket was touched in the last 8h
			if isInactive {
				if !hasRecentActivity(baseDir, ws, hideCutoff) {
					hidden++
					continue
				}
			}

			if !hasWindow {
				fmt.Printf("      %s○ %s — not started%s\n", ansiDim, ws, ansiReset)
				fmt.Printf("      %s%s%s\n", ansiDim, rawObsidianIndexURL(ws), ansiReset)
				continue
			}

			if windowDead[ws] {
				fmt.Printf("  [%d] %s✕ %s — exited%s\n", idx, ansiRed, ws, ansiReset)
				fmt.Printf("      %s%s%s\n", ansiDim, rawObsidianIndexURL(ws), ansiReset)
				continue
			}

			if isPaneIdle(windowPaneCmd[ws]) {
				fmt.Printf("  [%d] %s■ %s — stopped%s\n", idx, ansiDim, ws, ansiReset)
				fmt.Printf("      %s%s%s\n", ansiDim, rawObsidianIndexURL(ws), ansiReset)
				continue
			}

			state := readState(baseDir, ws)

			switch state.Status {
			case "working":
				elapsed := ""
				if state.StartedAt != "" {
					if t, err := time.Parse(time.RFC3339, state.StartedAt); err == nil {
						elapsed = " " + ansiDimYellow + formatElapsed(time.Since(t)) + ansiReset
					}
				}
				fmt.Printf("  [%d] %s●%s %s%s%s — %sworking%s%s\n",
					idx, ansiGreen, ansiReset, ansiBold, ws, ansiReset, ansiGreen, ansiReset, elapsed)
			case "verifying":
				elapsed := ""
				if state.StartedAt != "" {
					if t, err := time.Parse(time.RFC3339, state.StartedAt); err == nil {
						elapsed = " " + ansiDimYellow + formatElapsed(time.Since(t)) + ansiReset
					}
				}
				fmt.Printf("  [%d] %s●%s %s%s%s — %sverifying%s%s\n",
					idx, ansiYellow, ansiReset, ansiBold, ws, ansiReset, ansiYellow, ansiReset, elapsed)
			case "idle":
				fmt.Printf("  [%d] %s●%s %s%s%s — %sidle%s\n",
					idx, ansiCyan, ansiReset, ansiBold, ws, ansiReset, ansiDim, ansiReset)
			default:
				fmt.Printf("  [%d] %s●%s %s%s%s — running\n",
					idx, ansiGreen, ansiReset, ansiBold, ws, ansiReset)
			}
			fmt.Printf("      %s%s%s\n", ansiDim, rawObsidianIndexURL(ws), ansiReset)

			// Show ticket details — prefer DB runs, fall back to frontmatter
			if database.DB != nil && hasDBRuns(ws) {
				renderTicketsFromDB(ws)
			} else {
				renderTicketsFromFrontmatter(baseDir, ws)
			}

			fmt.Println()
		}

		// Show unknown windows
		for _, w := range windows {
			known := false
			for _, ws := range allWorkspaces {
				if w.Name == ws {
					known = true
					break
				}
			}
			if !known && w.Name != "bash" && w.Name != "zsh" {
				fmt.Printf("  [%d] %s? %s — unknown window%s\n", w.Index, ansiDim, w.Name, ansiReset)
			}
		}

		if hidden > 0 {
			fmt.Printf("%s%d workspace(s) hidden (no activity in 8h)%s\n", ansiDim, hidden, ansiReset)
		}
		fmt.Printf("\nAttach with: wiggums session attach <index>\n")

		return nil
	},
}

var sessionAttachCmd = &cobra.Command{
	Use:   "attach <window-index>",
	Short: "Attach to a specific tmux window by index",
	Long: `Attach to the wiggums tmux session with the specified window selected.
Use "wiggums session ls" to see available window indices.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		idx, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid window index %q: must be an integer", args[0])
		}

		sm := NewSessionManager()

		if !sm.SessionExists() {
			fmt.Println("No wiggums tmux session running.")
			fmt.Println("Start one with: wiggums start")
			return nil
		}

		// Verify the window index exists
		windows, err := sm.ListWindowsDetailed()
		if err != nil {
			return err
		}
		found := false
		var windowName string
		for _, w := range windows {
			if w.Index == idx {
				found = true
				windowName = w.Name
				break
			}
		}
		if !found {
			fmt.Printf("No window with index %d.\n", idx)
			fmt.Println("Use \"wiggums session ls\" to see available windows.")
			return nil
		}

		fmt.Printf("Attaching to window [%d] %s...\n", idx, windowName)
		return sm.AttachToWindow(idx)
	},
}

var sessionPaneCmd = &cobra.Command{
	Use:   "pane [window-index]",
	Short: "Print tmux pane contents without attaching",
	Long: `Print the visible contents of a tmux pane to stdout.
If a window index is provided, shows that window's pane.
Without arguments, shows pane contents for all windows.

Use "wiggums session ls" to see available window indices.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sm := NewSessionManager()

		if !sm.SessionExists() {
			fmt.Println("No wiggums tmux session running.")
			fmt.Println("Start one with: wiggums start")
			return nil
		}

		windows, err := sm.ListWindowsDetailed()
		if err != nil {
			return err
		}

		if len(args) == 1 {
			// Show a single window's pane
			idx, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid window index %q: must be an integer", args[0])
			}
			var windowName string
			found := false
			for _, w := range windows {
				if w.Index == idx {
					found = true
					windowName = w.Name
					break
				}
			}
			if !found {
				fmt.Printf("No window with index %d.\n", idx)
				fmt.Println("Use \"wiggums session ls\" to see available windows.")
				return nil
			}
			content, err := sm.CapturePaneContent(idx)
			if err != nil {
				return err
			}
			fmt.Printf("=== [%d] %s ===\n", idx, windowName)
			fmt.Print(strings.TrimRight(content, "\n"))
			fmt.Println()
			return nil
		}

		// No args: show all windows
		for _, w := range windows {
			content, err := sm.CapturePaneContent(w.Index)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "  error capturing [%d] %s: %v\n", w.Index, w.Name, err)
				continue
			}
			fmt.Printf("=== [%d] %s ===\n", w.Index, w.Name)
			fmt.Print(strings.TrimRight(content, "\n"))
			fmt.Printf("\n\n")
		}
		return nil
	},
}
