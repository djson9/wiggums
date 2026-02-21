package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	preprocessCmd.Flags().String("queue", "default", "Queue ID to watch (default: \"default\")")
	rootCmd.AddCommand(preprocessCmd)
}

var preprocessCmd = &cobra.Command{
	Use:   "preprocess",
	Short: "Run preprocessing on tickets that have preprocessing instructions",
	Long: `Watches a queue file for tickets with preprocessing instructions and
runs a Claude Code instance to preprocess them using subagents.
Start the TUI in one terminal, set preprocessing instructions with 'p',
and run this worker in another terminal.

Use --queue <id> to specify which queue to watch (default: "default").
If no --queue flag is given, an interactive picker shows available queues.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("queue") {
			queueID, _ := cmd.Flags().GetString("queue")
			if queueID == "" {
				queueID = "default"
			}
			return runPreprocessForQueue(queueID)
		}
		// No explicit --queue: launch interactive picker
		queueID, err := runWorkerPicker()
		if err != nil {
			return err
		}
		if queueID == "" {
			return nil // user cancelled
		}
		return runPreprocessForQueue(queueID)
	},
}

func runPreprocessForQueue(queueID string) error {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return fmt.Errorf("could not resolve base directory: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Printf("Preprocessing worker started. Watching queue %q...\n", queueID)
	fmt.Println("Press Ctrl+C to stop.")

	runner := &ClaudeRunner{WorkDir: baseDir}

	return preprocessLoop(ctx, baseDir, runner, queueID)
}

// preprocessTicket pairs a queue ticket with its index for tracking.
type preprocessTicket struct {
	index  int
	ticket QueueTicket
}

// preprocessLoop polls the queue file and runs Claude to preprocess tickets.
// It collects all pending preprocessing tickets, runs ONE Claude instance that
// uses subagents to process them in parallel, then re-scans for new work.
func preprocessLoop(ctx context.Context, baseDir string, runner Runner, queueID string) error {
	queuePath := queueFilePathForID(queueID)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nPreprocessing worker stopped.")
			return nil
		default:
		}

		qf, err := readQueueFileFromPath(queuePath)
		if err != nil {
			if err := sleepWithContext(ctx, 2*time.Second); err != nil {
				return nil
			}
			continue
		}

		// Collect tickets needing preprocessing
		var pending []preprocessTicket
		for i, t := range qf.Tickets {
			if t.PreprocessPrompt != "" && t.PreprocessStatus != "completed" && t.PreprocessStatus != "in_progress" {
				pending = append(pending, preprocessTicket{
					index:  i,
					ticket: t,
				})
			}
		}

		if len(pending) == 0 {
			if err := sleepWithContext(ctx, 2*time.Second); err != nil {
				return nil
			}
			continue
		}

		fmt.Printf("\n─── Found %d tickets to preprocess ───\n", len(pending))
		for _, pt := range pending {
			fmt.Printf("  %s: %s\n", filepath.Base(pt.ticket.Path), pt.ticket.PreprocessPrompt)
		}

		// Mark all pending as "in_progress" in queue file
		freshQf, readErr := readQueueFileFromPath(queuePath)
		if readErr != nil {
			freshQf = qf
		}
		for _, pt := range pending {
			idx := findTicketIdx(freshQf, pt.ticket.Path, pt.ticket.RequestNum)
			if idx >= 0 {
				freshQf.Tickets[idx].PreprocessStatus = "in_progress"
			}
		}
		writeQueueFileDataToPath(freshQf, queuePath)

		// Build the preprocessing prompt
		prompt := buildPreprocessPrompt(baseDir, pending, queuePath)

		// Build claude args with --add-dir for each unique workspace directory
		claudeArgs := []string{}
		if yolo {
			claudeArgs = append(claudeArgs, "--model", "opus", "--dangerously-skip-permissions")
		}
		addedDirs := make(map[string]bool)
		for _, pt := range pending {
			workDir := resolveWorkDirForTicket(baseDir, pt.ticket.Workspace)
			if workDir != "" && !addedDirs[workDir] {
				claudeArgs = append(claudeArgs, "--add-dir", workDir)
				addedDirs[workDir] = true
			}
		}

		os.Setenv("TERM", "xterm")

		exitCode, err := runner.Run(ctx, prompt, claudeArgs)
		fmt.Printf("\nPreprocessing Claude exited: code=%d err=%v\n", exitCode, err)

		// Check each ticket for preprocessing completion by looking for ## Preprocessing section
		freshQf2, readErr2 := readQueueFileFromPath(queuePath)
		if readErr2 != nil {
			continue
		}
		completedCount := 0
		for _, pt := range pending {
			idx := findTicketIdx(freshQf2, pt.ticket.Path, pt.ticket.RequestNum)
			if idx < 0 {
				continue
			}
			if checkPreprocessingComplete(pt.ticket.Path) {
				freshQf2.Tickets[idx].PreprocessStatus = "completed"
				completedCount++
			} else {
				// Reset to pending so it gets retried next loop
				freshQf2.Tickets[idx].PreprocessStatus = "pending"
			}
		}
		writeQueueFileDataToPath(freshQf2, queuePath)
		fmt.Printf("Preprocessing: %d/%d tickets completed\n", completedCount, len(pending))

		// Loop back to check for any new preprocessing work
	}
}

// buildPreprocessPrompt creates the prompt for the preprocessing Claude instance.
// It lists all tickets needing preprocessing with their instructions.
func buildPreprocessPrompt(baseDir string, tickets []preprocessTicket, queuePath string) string {
	// Try to load the template file
	templatePath := filepath.Join(baseDir, "prompts", "preprocess.md")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		templateContent = []byte(defaultPreprocessTemplate())
	}

	prompt := strings.ReplaceAll(string(templateContent), "{{WIGGUMS_DIR}}", baseDir)
	prompt = strings.ReplaceAll(prompt, "{{QUEUE_PATH}}", queuePath)

	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n## Tickets to Preprocess\n\n")

	for i, pt := range tickets {
		b.WriteString(fmt.Sprintf("### Ticket %d\n", i+1))
		b.WriteString(fmt.Sprintf("- **Path**: `%s`\n", pt.ticket.Path))
		b.WriteString(fmt.Sprintf("- **Workspace**: %s\n", pt.ticket.Workspace))
		b.WriteString(fmt.Sprintf("- **Instruction**: %s\n\n", pt.ticket.PreprocessPrompt))
	}

	return b.String()
}

// checkPreprocessingComplete checks if a ticket file has a ## Preprocessing section with content.
func checkPreprocessingComplete(ticketPath string) bool {
	content, err := os.ReadFile(ticketPath)
	if err != nil {
		return false
	}
	contentStr := string(content)
	idx := strings.Index(contentStr, "## Preprocessing")
	if idx < 0 {
		return false
	}
	// Check there's content after the heading
	after := strings.TrimSpace(contentStr[idx+len("## Preprocessing"):])
	return len(after) > 0
}

func defaultPreprocessTemplate() string {
	return `# Preprocessing

**Wiggums Directory:** ` + "`{{WIGGUMS_DIR}}`" + `

You are a preprocessing agent. Your job is to preprocess tickets before they are worked on by the main worker agent.

## Instructions

For each ticket listed below, launch a background Task subagent. Use run_in_background: true on every Task call so all tickets are processed concurrently.

Each subagent should:
1. Read the ticket file at the given path
2. Follow the preprocessing instruction provided for that ticket
3. Write the results to a ` + "`## Preprocessing`" + ` section in the ticket file
   - Append this section AFTER the existing content
   - Do NOT modify any existing content in the file
   - Do NOT modify the YAML frontmatter

## Execution Flow

1. Launch ALL ticket subagents in a single message using run_in_background: true
2. After launching, poll each background task using TaskOutput to check completion
3. Once all tasks complete, re-read the queue JSON file at ` + "`{{QUEUE_PATH}}`" + `
4. Look for tickets with "preprocess_prompt" set and "preprocess_status" = "pending"
5. If found, launch new background subagents for the new batch
6. Repeat until no new pending preprocessing tickets exist
7. Then exit

IMPORTANT:
- ALWAYS use run_in_background: true on Task calls so tickets process concurrently
- Launch ALL subagents in a single message (multiple Task calls in one response)
- Each subagent gets ONE ticket to work on
- Write results directly to the ticket file
- Do NOT modify the queue JSON file — the Go worker manages queue state
- Do NOT modify any part of the ticket besides appending the ## Preprocessing section
- Do NOT modify the YAML frontmatter or status fields
`
}
