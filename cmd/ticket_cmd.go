package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var ticketQueueFlag string

func init() {
	ticketCmd.AddCommand(ticketAddCmd)
	ticketCmd.AddCommand(ticketUpdateCmd)
	ticketCmd.AddCommand(ticketViewCmd)
	ticketCmd.AddCommand(ticketCompleteCmd)

	ticketAddCmd.Flags().StringVar(&ticketQueueFlag, "queue", "", "Queue (workspace) to add the ticket to")
	ticketAddCmd.MarkFlagRequired("queue")

	rootCmd.AddCommand(ticketCmd)

	// Add inspect to the existing queueCmd (defined in tui.go)
	queueCmd.AddCommand(queueInspectCmd)
}

var ticketCmd = &cobra.Command{
	Use:   "ticket",
	Short: "Manage tickets",
}

var ticketAddCmd = &cobra.Command{
	Use:   "add [title]",
	Short: "Create a new ticket in a queue and return its ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := args[0]
		return ticketAdd(ticketQueueFlag, title)
	},
}

var ticketUpdateCmd = &cobra.Command{
	Use:   "update [ticket-id]",
	Short: "Add a timestamped update section to a ticket and return the file path for editing",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ticketUpdate(args[0])
	},
}

var ticketCompleteCmd = &cobra.Command{
	Use:   "complete [ticket-id]",
	Short: "Mark a ticket as completed",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ticketComplete(args[0])
	},
}

var ticketViewCmd = &cobra.Command{
	Use:   "view [ticket-id]",
	Short: "View a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ticketView(args[0])
	},
}

var queueInspectCmd = &cobra.Command{
	Use:               "inspect [queue]",
	Short:             "Show tickets in a queue with their status",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorkspaces,
	RunE: func(cmd *cobra.Command, args []string) error {
		return queueInspect(args[0])
	},
}

// ticketAdd creates a new ticket in the specified workspace/queue and prints the ticket ID.
func ticketAdd(queue, title string) error {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return fmt.Errorf("could not resolve base directory: %w", err)
	}

	// Verify workspace exists
	indexPath := filepath.Join(baseDir, "workspaces", queue, "index.md")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return fmt.Errorf("queue %q not found", queue)
	}

	fp, err := createTicketFile(baseDir, queue, title, title)
	if err != nil {
		return fmt.Errorf("failed to create ticket: %w", err)
	}

	// Extract epoch ID from filename
	base := filepath.Base(fp)
	if idx := strings.IndexByte(base, '_'); idx > 0 {
		fmt.Println(base[:idx])
	} else {
		fmt.Println(base)
	}
	return nil
}

// ticketUpdate adds a timestamped update section with empty headers to a ticket,
// then prints the file path and timestamp so the LLM can edit the file directly.
func ticketUpdate(id string) error {
	ticketPath, err := findTicketByID(id)
	if err != nil {
		return err
	}

	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		return fmt.Errorf("could not load ET timezone: %w", err)
	}
	ts := time.Now().In(et)
	updated, err := applyTicketUpdate(ticketPath, ts)
	if err != nil {
		return err
	}

	if err := os.WriteFile(ticketPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to write ticket: %w", err)
	}

	timestamp := ts.Format("2006-01-02 15:04 MST")
	fmt.Printf("Timestamp: %s\n", timestamp)
	fmt.Printf("File: %s\n", ticketPath)
	return nil
}

// applyTicketUpdate builds the updated ticket content by inserting a new
// timestamped section just below the divider line. It returns the new content.
func applyTicketUpdate(ticketPath string, ts time.Time) (string, error) {
	content, err := os.ReadFile(ticketPath)
	if err != nil {
		return "", fmt.Errorf("could not read ticket: %w", err)
	}

	return applyTicketUpdateToContent(string(content), ts)
}

// applyTicketUpdateToContent is the testable core: given raw content and a
// timestamp, it returns the modified content with empty update section headers
// prepended below the divider line.
func applyTicketUpdateToContent(content string, ts time.Time) (string, error) {
	now := ts.Format("2006-01-02 15:04")
	updateSection := fmt.Sprintf("## Update %s\n\n### Approach\n\n\n### Commands Run\n\n\n### Context Breadcrumbs\n\n\n### Findings\n\n", now)

	lines := strings.Split(content, "\n")
	var result []string
	inserted := false

	for i, line := range lines {
		result = append(result, line)
		if !inserted && strings.Contains(line, "Below to be filled by agent") {
			// Consume the following blank line if present
			nextIdx := i + 1
			if nextIdx < len(lines) && strings.TrimSpace(lines[nextIdx]) == "" {
				result = append(result, "") // keep the blank line
				// Mark it so we skip it in the main loop
				lines[nextIdx] = "\x00SKIP"
			} else {
				result = append(result, "")
			}
			result = append(result, updateSection)
			inserted = true
		} else if line == "\x00SKIP" {
			// Already consumed above, remove the marker
			result[len(result)-1] = ""
		}
	}

	if !inserted {
		return "", fmt.Errorf("could not find divider line ('Below to be filled by agent')")
	}

	return strings.Join(result, "\n"), nil
}

// ticketComplete marks a ticket as completed in its frontmatter.
func ticketComplete(id string) error {
	ticketPath, err := findTicketByID(id)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(ticketPath)
	if err != nil {
		return fmt.Errorf("could not read ticket: %w", err)
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
				lines[i] = "Status: completed"
				break
			}
		}
	}

	if err := os.WriteFile(ticketPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("failed to write ticket: %w", err)
	}

	fmt.Printf("Marked ticket %s as completed\n", id)
	return nil
}

// ticketView prints the contents of a ticket, excluding the YAML frontmatter.
func ticketView(id string) error {
	ticketPath, err := findTicketByID(id)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(ticketPath)
	if err != nil {
		return fmt.Errorf("could not read ticket: %w", err)
	}

	fmt.Print(stripFrontmatter(string(content)))
	return nil
}

// findTicketByID searches all workspaces for a ticket file starting with the given ID (epoch prefix).
func findTicketByID(id string) (string, error) {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve base directory: %w", err)
	}

	wsRoot := filepath.Join(baseDir, "workspaces")
	entries, err := os.ReadDir(wsRoot)
	if err != nil {
		return "", fmt.Errorf("could not read workspaces: %w", err)
	}

	for _, ws := range entries {
		if !ws.IsDir() {
			continue
		}
		ticketsDir := filepath.Join(wsRoot, ws.Name(), "tickets")
		tickets, err := os.ReadDir(ticketsDir)
		if err != nil {
			continue
		}
		for _, t := range tickets {
			if strings.HasPrefix(t.Name(), id+"_") && strings.HasSuffix(t.Name(), ".md") {
				return filepath.Join(ticketsDir, t.Name()), nil
			}
		}
	}

	return "", fmt.Errorf("ticket %q not found in any queue", id)
}

// queueInspect lists tickets in a queue by reading the queue JSON file.
func queueInspect(queue string) error {
	path := queueFilePathForID(queue)
	qf, err := readQueueFileFromPath(path)
	if err != nil {
		return fmt.Errorf("queue %q not found", queue)
	}

	name := qf.Name
	if name == "" {
		name = queue
	}

	if len(qf.Tickets) == 0 {
		fmt.Printf("Queue %q has no tickets.\n", name)
		return nil
	}

	runState := "stopped"
	if qf.Running {
		runState = "running"
	}
	fmt.Printf("Queue: %s (%s)\n\n", name, runState)
	fmt.Printf("%-16s  %-25s  %s\n", "ID", "STATUS", "TITLE")
	fmt.Printf("%-16s  %-25s  %s\n", strings.Repeat("-", 16), strings.Repeat("-", 25), strings.Repeat("-", 30))

	for _, t := range qf.Tickets {
		filename := filepath.Base(t.Path)
		id, title := parseTicketFilename(filename)
		status := t.Status
		if status == "" {
			status = "pending"
		}
		fmt.Printf("%-16s  %-25s  %s\n", id, status, title)
	}
	return nil
}

// parseTicketFilename extracts the epoch ID and title from a ticket filename like "1773005328_contracts.md".
func parseTicketFilename(filename string) (id, title string) {
	name := strings.TrimSuffix(filename, ".md")
	idx := strings.Index(name, "_")
	if idx == -1 {
		return name, name
	}
	return name[:idx], strings.ReplaceAll(name[idx+1:], "_", " ")
}
