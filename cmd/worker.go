package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"wiggums/database"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func init() {
	workerCmd.Flags().String("queue", "default", "Queue ID to watch (default: \"default\")")
	rootCmd.AddCommand(workerCmd)
}

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Process tickets from the TUI queue",
	Long: `Watches a queue file and processes tickets sequentially.
Start the TUI in one terminal and the worker in another. Press 's' in the TUI to
start the queue, and the worker will pick up tickets automatically.

Use --queue <id> to specify which queue to watch (default: "default").
If no --queue flag is given, an interactive picker shows available queues.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("queue") {
			queueID, _ := cmd.Flags().GetString("queue")
			if queueID == "" {
				queueID = "default"
			}
			return runWorkerForQueue(queueID)
		}
		// No explicit --queue: launch interactive picker
		queueID, err := runWorkerPicker()
		if err != nil {
			return err
		}
		if queueID == "" {
			return nil // user cancelled
		}
		return runWorkerForQueue(queueID)
	},
}

// workerPickerModel is a minimal Bubble Tea model for selecting a queue.
type workerPickerModel struct {
	list       list.Model
	selectedID string
	quitting   bool
}

// loadWorkerPickerItems loads queue items sorted by ticket count descending.
func loadWorkerPickerItems() []list.Item {
	items := loadQueuePickerItems("")
	// Re-sort by ticketCount descending (most items first)
	sort.SliceStable(items, func(i, j int) bool {
		qi := items[i].(tuiQueueItem)
		qj := items[j].(tuiQueueItem)
		return qi.ticketCount > qj.ticketCount
	})
	return items
}

func newWorkerPickerModel() workerPickerModel {
	items := loadWorkerPickerItems()
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Select a queue to watch"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	return workerPickerModel{list: l}
}

func (m workerPickerModel) Init() tea.Cmd {
	return nil
}

func (m workerPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		// Don't intercept keys when filtering
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(tuiQueueItem); ok {
				m.selectedID = item.id
				m.quitting = true
				return m, tea.Quit
			}
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m workerPickerModel) View() string {
	if m.quitting {
		return ""
	}
	return m.list.View()
}

// runWorkerPicker runs the interactive queue picker and returns the selected queue ID.
// Returns empty string if the user cancelled.
func runWorkerPicker() (string, error) {
	m := newWorkerPickerModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("picker error: %w", err)
	}
	final := result.(workerPickerModel)
	return final.selectedID, nil
}

func runWorkerForQueue(queueID string) error {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return fmt.Errorf("could not resolve base directory: %w", err)
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
	}

	fmt.Printf("Worker started. Watching queue %q for work...\n", queueID)
	fmt.Println("Press Ctrl+C to stop.")

	runner := &ClaudeRunner{WorkDir: baseDir}
	promptLoader := &FilePromptLoader{}
	notifier := &BeeepNotifier{}

	return workerLoopForQueue(ctx, baseDir, runner, promptLoader, notifier, queueID)
}

// workerLoop polls the default queue file and processes tickets when the queue is running.
func workerLoop(ctx context.Context, baseDir string, runner Runner, promptLoader PromptLoader, notifier Notifier) error {
	path := queueFilePath()
	return workerLoopWithPath(ctx, baseDir, runner, promptLoader, notifier, path)
}

// workerLoopForQueue polls a specific queue by ID.
func workerLoopForQueue(ctx context.Context, baseDir string, runner Runner, promptLoader PromptLoader, notifier Notifier, queueID string) error {
	path := queueFilePathForID(queueID)
	return workerLoopWithPath(ctx, baseDir, runner, promptLoader, notifier, path)
}

// workerLoopWithPath is the testable version that accepts a queue file path.
func workerLoopWithPath(ctx context.Context, baseDir string, runner Runner, promptLoader PromptLoader, notifier Notifier, queuePath string) error {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nWorker stopped.")
			return nil
		default:
		}

		qf, err := readQueueFileFromPath(queuePath)
		if err != nil {
			// Queue file doesn't exist yet, or is invalid — wait and retry
			if err := sleepWithContext(ctx, 2*time.Second); err != nil {
				return nil
			}
			continue
		}

		if !qf.Running {
			// Queue explicitly stopped — poll again
			devLog("WORKER poll: Running=%v — sleeping", qf.Running)
			if err := sleepWithContext(ctx, 2*time.Second); err != nil {
				return nil
			}
			continue
		}

		// Compute next ticket to process from statuses (stateless)
		nextIdx := nextPendingTicketIndex(qf)
		if nextIdx < 0 {
			// No pending tickets — wait for new items (don't stop the queue).
			// Don't write heartbeat here — it does a full read-modify-write of the
			// queue file which can clobber TUI-added items (race condition).
			// The TUI uses WorkerPID liveness checks instead.
			devLog("WORKER poll: no pending tickets (total=%d) — waiting for new items", len(qf.Tickets))
			if err := sleepWithContext(ctx, 2*time.Second); err != nil {
				return nil
			}
			continue
		}

		ticket := qf.Tickets[nextIdx]
		devLog("WORKER ticket[%d]: status=%q path=%s (computed from statuses)", nextIdx, ticket.Status, filepath.Base(ticket.Path))

		fmt.Printf("\n─── Processing ticket: %s ───\n", filepath.Base(ticket.Path))
		fmt.Printf("Queue: %s | Position: %d/%d\n", qf.Name, nextIdx+1, len(qf.Tickets))

		// Reconciliation: check ticket frontmatter before running Claude.
		// If a previous worker died after Claude wrote status but before the queue
		// file was updated, the queue still shows "working"/"pending" but the ticket
		// file already says "completed" or "completed + verified". In that case, skip
		// the work pass and just advance (or run verification only).
		if reconciled := workerReconcileTicket(ctx, baseDir, ticket, runner, promptLoader, notifier, queuePath); reconciled {
			continue
		}

		// Re-read queue file before writing "working" status to avoid clobbering
		// TUI changes (same pattern as all other worker write paths).
		workingQf, readErr := readQueueFileFromPath(queuePath)
		if readErr != nil {
			workingQf = qf
		}
		workingIdx := findTicketIdx(workingQf, ticket.Path, ticket.RequestNum)
		if workingIdx < 0 {
			// Ticket was removed from queue between read and write — skip
			fmt.Printf("Ticket %s was removed from queue before processing started, skipping\n", filepath.Base(ticket.Path))
			continue
		}
		workingQf.Tickets[workingIdx].Status = "working"
		workingQf.Tickets[workingIdx].StartedAt = time.Now().Unix()
		writeWorkerHeartbeatOnQf(workingQf)
		writeQueueFileDataToPath(workingQf, queuePath)
		// Update qf to the fresh copy for use later in prompt building
		qf = workingQf

		// Resolve workspace directory for this ticket
		workDir := resolveWorkDirForTicket(baseDir, ticket.Workspace)

		// Build claude args
		claudeArgs := []string{}
		if yolo {
			claudeArgs = append(claudeArgs, "--model", "opus", "--dangerously-skip-permissions")
		}
		if workDir != "" {
			claudeArgs = append(claudeArgs, "--add-dir", workDir)
		}

		os.Setenv("TERM", "xterm")

		// Build the prompt
		workFile := "prompts/prompt.md"
		shortcutsFile := filepath.Join(baseDir, "workspaces", ticket.Workspace, "shortcuts.md")

		prompt, err := promptLoader.Load(baseDir, workFile, "Next ticket:", []string{ticket.Path}, "")
		if err != nil {
			fmt.Printf("Error building prompt: %v\n", err)
			// Re-read queue file to avoid clobbering TUI changes
			errQf, readErr := readQueueFileFromPath(queuePath)
			if readErr != nil {
				errQf = qf
			}
			errIdx := findTicketIdx(errQf, ticket.Path, ticket.RequestNum)
			if errIdx >= 0 {
				errQf.Tickets[errIdx].Status = "pending"
				writeWorkerHeartbeatOnQf(errQf)
				writeQueueFileDataToPath(errQf, queuePath)
			} else {
				// Ticket was removed from queue during prompt loading — don't advance
				fmt.Printf("Ticket %s was removed from queue during prompt loading, skipping advance\n", filepath.Base(ticket.Path))
			}
			continue
		}

		// Snapshot frontmatter status before running Claude so we can restore
		// on non-zero exit (prevents partially-written "completed" from being
		// auto-verified via SkipVerification on retry).
		var savedOriginalStatus string
		if fContent, readErr := os.ReadFile(ticket.Path); readErr == nil {
			s := extractFrontmatterStatus(string(fContent))
			if idx := strings.Index(s, ":"); idx >= 0 {
				savedOriginalStatus = strings.TrimSpace(s[idx+1:])
			}
		}

		// For additional requests, prefer the DB-stored original status (crash-resilient).
		if ticket.RequestNum > 0 {
			if database.DB != nil {
				if orig, dbErr := database.GetOriginalTicketStatus(ctx, ticket.Path); dbErr == nil && orig != "" {
					savedOriginalStatus = orig
				}
			}
			// Write additional request content to ticket file if not already present.
			// Draft content is stored only in SQLite at creation time and written
			// to the file here at runtime so Claude doesn't see it prematurely.
			if err := writeAdditionalRequestContentToFile(ticket.Path, ticket.RequestNum); err != nil {
				fmt.Printf("Warning: could not write additional request content to file: %v\n", err)
			}
			if err := setStatusToAdditionalUserRequest(ticket.Path); err != nil {
				fmt.Printf("Warning: could not set additional_user_request status: %v\n", err)
			}
		}

		// Increment iteration AFTER prompt is built successfully.
		// If we incremented before prompt loading, failed loads would still
		// bump CurIteration, causing premature MinIterations completion.
		incrementCurIteration(ticket.Path)
		prompt = strings.ReplaceAll(prompt, "{{SHORTCUTS_PATH}}", shortcutsFile)
		prompt = strings.ReplaceAll(prompt, "{{WORKSPACE}}", ticket.Workspace)
		prompt = strings.ReplaceAll(prompt, "{{QUEUE_ID}}", queueIDFromPath(queuePath))

		// Append queue prompt if set
		if qf.Prompt != "" {
			prompt += "\n\n## Queue Prompt\n" + qf.Prompt
		}

		// Append queue shortcuts if any
		if len(qf.Shortcuts) > 0 {
			prompt += "\n\n## Queue Shortcuts\nThese are learnings from previous tickets in this queue:\n"
			for _, s := range qf.Shortcuts {
				prompt += "- " + s + "\n"
			}
			prompt += "\nYou can add new shortcuts with: `wiggums add-shortcut <text>`\n"
		}

		// Append workspace prompt if available
		wsWorkPrompt := loadWorkspacePrompt(baseDir, ticket.Workspace, "prompts/prompt.md")
		if wsWorkPrompt != "" {
			prompt += "\n\n" + wsWorkPrompt
		}

		// Track run in DB
		var runID int64
		if database.DB != nil {
			if id, err := database.StartRun(ctx, ticket.Workspace, filepath.Base(ticket.Path), "working", ticketCreatedAtPtr(filepath.Base(ticket.Path))); err == nil {
				runID = id
			}
		}

		exitCode, err := runner.Run(ctx, prompt, claudeArgs)
		devLog("WORKER run finished: exitCode=%d err=%v ticket=%s", exitCode, err, filepath.Base(ticket.Path))

		// Complete DB run
		if database.DB != nil && runID > 0 {
			ec := exitCode
			if err != nil {
				ec = 1
			}
			_ = database.CompleteRun(ctx, runID, ec)
		}

		// Update timestamp
		updateTicketsTimestamp([]string{ticket.Path})

		// Re-read queue file to avoid clobbering TUI changes made during processing
		freshQf, readErr := readQueueFileFromPath(queuePath)
		if readErr != nil {
			// If we can't re-read, fall back to the stale copy
			freshQf = qf
		}

		// Find the ticket by path in the fresh queue (index may have shifted)
		ticketIdx := findTicketIdx(freshQf, ticket.Path, ticket.RequestNum)

		// If the ticket was removed from the queue during processing,
		// don't advance — just continue the loop and re-read state.
		if ticketIdx < 0 {
			fmt.Printf("Ticket %s was removed from queue during processing, skipping advance\n", filepath.Base(ticket.Path))
			workerRestoreOriginalStatus(ticket, savedOriginalStatus)
			continue
		}

		if err != nil {
			fmt.Printf("Error running claude: %v, will retry\n", err)
			devLog("WORKER err path: keeping ticket[%d] as pending for retry, err=%v", ticketIdx, err)
			freshQf.Tickets[ticketIdx].Status = "pending"
			writeWorkerHeartbeatOnQf(freshQf)
			writeQueueFileDataToPath(freshQf, queuePath)
			workerRestoreOriginalStatus(ticket, savedOriginalStatus)
			continue
		}

		// On non-zero exit (interrupt, signal kill, error), restore the ticket
		// status to what it was before Claude ran. This prevents a partially-written
		// "completed" from being auto-verified via SkipVerification.
		treatAsSuccess := exitCode == 0
		if exitCode != 0 && savedOriginalStatus != "" {
			content, readFileErr := os.ReadFile(ticket.Path)
			if readFileErr == nil {
				fmStatus := extractFrontmatterStatus(string(content))
				fmLower := strings.ToLower(fmStatus)
				isCompleted := strings.Contains(fmLower, "completed") && !strings.Contains(fmLower, "not completed")
				if isCompleted {
					fmt.Printf("Claude exited with code %d, restoring ticket %s status to %q\n", exitCode, filepath.Base(ticket.Path), savedOriginalStatus)
					devLog("WORKER non-zero exit: code=%d, restoring status from %q to %q", exitCode, fmStatus, savedOriginalStatus)
					restoreFrontmatterStatus(ticket.Path, savedOriginalStatus)
				}
			}
		}

		if treatAsSuccess {
			// Check MinIterations before marking as completed
			if shouldReprocess := workerCheckMinIterations(ticket.Path); shouldReprocess {
				fmt.Printf("Ticket %s: MinIterations not met, re-processing\n", filepath.Base(ticket.Path))
				resetStatusToInProgress(ticket.Path)
				freshQf.Tickets[ticketIdx].Status = "working"
				writeWorkerHeartbeatOnQf(freshQf)
				writeQueueFileDataToPath(freshQf, queuePath)
				// Don't advance — re-process this ticket
				// Restore original status so TUI shows correct status between retries
				workerRestoreOriginalStatus(ticket, savedOriginalStatus)
				continue
			}

			// Run verification pass
			verified := workerRunVerification(ctx, baseDir, ticket, runner, promptLoader, notifier, claudeArgs, queuePath)
			if !verified {
				// Verification failed — keep ticket as "working" for re-processing
				fmt.Printf("Ticket %s: verification failed, re-processing\n", filepath.Base(ticket.Path))
				// Re-read queue file again since verification may have taken time
				freshQf2, readErr2 := readQueueFileFromPath(queuePath)
				if readErr2 != nil {
					freshQf2 = freshQf
				}
				ticketIdx2 := findTicketIdx(freshQf2, ticket.Path, ticket.RequestNum)
				if ticketIdx2 >= 0 {
					freshQf2.Tickets[ticketIdx2].Status = "working"
					writeWorkerHeartbeatOnQf(freshQf2)
					writeQueueFileDataToPath(freshQf2, queuePath)
				} else {
					fmt.Printf("Ticket %s was removed from queue during verification, skipping\n", filepath.Base(ticket.Path))
				}
				workerRestoreOriginalStatus(ticket, savedOriginalStatus)
				continue
			}

			fmt.Printf("Ticket completed and verified: %s\n", filepath.Base(ticket.Path))
			// Re-read fresh since verification took time
			freshQf, readErr = readQueueFileFromPath(queuePath)
			if readErr != nil {
				freshQf = qf
			}
			ticketIdx = findTicketIdx(freshQf, ticket.Path, ticket.RequestNum)
			if ticketIdx < 0 {
				fmt.Printf("Ticket %s was removed from queue during verification, skipping advance\n", filepath.Base(ticket.Path))
				workerRestoreOriginalStatus(ticket, savedOriginalStatus)
				continue
			}
			freshQf.Tickets[ticketIdx].Status = "completed"
			freshQf.Tickets[ticketIdx].Unread = true

			// Sync additional request status to DB so TUI displays correct status on reload
			if ticket.RequestNum > 0 && database.DB != nil {
				_ = database.UpdateAdditionalRequestStatus(ctx, ticket.Path, ticket.RequestNum, "completed")
			}

			if notifier != nil {
				_ = notifier.Notify("Wiggums Worker", fmt.Sprintf("Verified: %s", filepath.Base(ticket.Path)), "")
			}
		} else {
			fmt.Printf("Claude exited with code %d for: %s, will retry\n", exitCode, filepath.Base(ticket.Path))
			devLog("WORKER non-zero exit: code=%d, frontmatter not completed, keeping ticket[%d] as pending for retry", exitCode, ticketIdx)
			freshQf.Tickets[ticketIdx].Status = "pending"
		}

		// Combine status update + advance into a single atomic write to
		// prevent the TUI from clobbering changes between two separate writes.
		devLog("WORKER final write: ticket[%d] status=%q, about to advance", ticketIdx, freshQf.Tickets[ticketIdx].Status)
		advanceQueueIndex(freshQf)
		writeWorkerHeartbeatOnQf(freshQf)
		devLog("WORKER final write: Running=%v tickets=%d", freshQf.Running, len(freshQf.Tickets))
		writeQueueFileDataToPath(freshQf, queuePath)

		// Restore original frontmatter status after processing additional request.
		// This ensures the original ticket's history is immutable — it stays "completed"
		// even though we temporarily changed it to "additional_user_request" for Claude.
		workerRestoreOriginalStatus(ticket, savedOriginalStatus)
	}
}

// workerRestoreOriginalStatus restores the ticket's frontmatter status after processing
// an additional request. This preserves history immutability — the original ticket stays
// "completed" even after an additional request is processed against it.
// No-op for non-additional-request tickets or when no saved status is available.
func workerRestoreOriginalStatus(ticket QueueTicket, savedOriginalStatus string) {
	if ticket.RequestNum > 0 && savedOriginalStatus != "" {
		if err := restoreFrontmatterStatus(ticket.Path, savedOriginalStatus); err != nil {
			fmt.Printf("Warning: could not restore original frontmatter status: %v\n", err)
		}
	}
}

// advanceQueueIndex enforces queue ordering after a ticket completes.
// Does NOT write to disk — the caller is responsible for writing. This allows combining status updates
// and index advances into a single atomic write.
// nextPendingTicketIndex computes the next ticket to process from statuses alone.
// Bottom-to-top processing: returns the last (highest-index) ticket that needs work.
// Returns -1 if no tickets need processing.
func nextPendingTicketIndex(qf *QueueFile) int {
	for i := len(qf.Tickets) - 1; i >= 0; i-- {
		t := qf.Tickets[i]
		if t.IsDraft {
			continue // drafts are not actionable
		}
		if t.Status == "pending" || t.Status == "working" || t.Status == "" {
			return i
		}
	}
	return -1
}

// allTicketsProcessed returns true if every ticket in the queue has a terminal
// status (completed) and there's nothing left for the worker to do.
func allTicketsProcessed(qf *QueueFile) bool {
	if len(qf.Tickets) == 0 {
		return true
	}
	for _, t := range qf.Tickets {
		if t.IsDraft {
			continue // drafts don't count as pending work
		}
		if t.Status != "completed" {
			return false
		}
	}
	return true
}

func advanceQueueIndex(qf *QueueFile) {
	enforceQueueFileOrdering(qf)
}

// enforceQueueFileOrdering rearranges QueueFile tickets so that all mutable
// tickets (pending/working/draft) come before immutable tickets (completed).
// Preserves relative order within each group.
func enforceQueueFileOrdering(qf *QueueFile) {
	if len(qf.Tickets) <= 1 {
		return
	}

	// Check if already valid: no immutable ticket at a lower index than any mutable ticket
	minImmutable := -1
	maxMutable := -1
	for i, t := range qf.Tickets {
		if isQueueTicketImmutable(t) {
			if minImmutable == -1 || i < minImmutable {
				minImmutable = i
			}
		} else {
			if i > maxMutable {
				maxMutable = i
			}
		}
	}
	if minImmutable == -1 || maxMutable == -1 || minImmutable > maxMutable {
		return // already valid
	}

	// Partition: mutable first, immutable second
	var mutable, immutable []QueueTicket
	for _, t := range qf.Tickets {
		if isQueueTicketImmutable(t) {
			immutable = append(immutable, t)
		} else {
			mutable = append(mutable, t)
		}
	}

	reordered := make([]QueueTicket, 0, len(qf.Tickets))
	reordered = append(reordered, mutable...)
	reordered = append(reordered, immutable...)
	qf.Tickets = reordered

	devLog("WORKER enforceQueueFileOrdering: reordered %d mutable + %d immutable tickets", len(mutable), len(immutable))
}

// isQueueTicketImmutable returns true if a QueueTicket should not be reordered
// above mutable items. Completed tickets are immutable.
func isQueueTicketImmutable(t QueueTicket) bool {
	return t.Status == "completed"
}

// advanceQueue moves to the next ticket in the queue, or stops if done.
func advanceQueue(qf *QueueFile) {
	path := queueFilePath()
	advanceQueueToPath(qf, path)
}

// advanceQueueToPath moves to the next ticket, writing to the specified path.
func advanceQueueToPath(qf *QueueFile, path string) {
	advanceQueueIndex(qf)
	writeQueueFileDataToPath(qf, path)
}

// writeQueueFileData writes a QueueFile struct to the default queue file path.
func writeQueueFileData(qf *QueueFile) error {
	path := queueFilePath()
	if path == "" {
		return fmt.Errorf("could not determine queue file path")
	}
	return writeQueueFileDataToPath(qf, path)
}

// writeQueueFileDataToPath writes a QueueFile struct to a specific path.
// Uses atomic write (temp file + rename) to prevent corruption from concurrent
// TUI/worker writes or mid-write crashes.
func writeQueueFileDataToPath(qf *QueueFile, path string) error {
	devLog("writeQueueFileDataToPath: Running=%v tickets=%d path=%s", qf.Running, len(qf.Tickets), filepath.Base(path))
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(qf, "", "  ")
	if err != nil {
		return err
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmp, err := os.CreateTemp(dir, ".queue-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, path)
}

// writeWorkerHeartbeat updates the LastHeartbeat timestamp in the queue file.
func writeWorkerHeartbeat(queuePath string) {
	qf, err := readQueueFileFromPath(queuePath)
	if err != nil {
		return
	}
	qf.LastHeartbeat = time.Now().Unix()
	qf.WorkerPID = os.Getpid()
	writeQueueFileDataToPath(qf, queuePath)
}

// writeWorkerHeartbeatOnQf sets the heartbeat on an existing QueueFile struct.
func writeWorkerHeartbeatOnQf(qf *QueueFile) {
	qf.LastHeartbeat = time.Now().Unix()
	qf.WorkerPID = os.Getpid()
}

// resolveWorkDirForTicket resolves the external working directory for a ticket's workspace.
func resolveWorkDirForTicket(baseDir, workspace string) string {
	indexPath := filepath.Join(baseDir, "workspaces", workspace, "index.md")
	workDir, err := readWorkspaceDirectory(indexPath)
	if err != nil || workDir == "" {
		return ""
	}
	return workDir
}

// buildPreviousTicketContext creates a context string describing previously completed
// tickets in the queue so the current ticket can reference their work.
// Stateless: looks at ticket statuses to identify completed tickets.
func buildPreviousTicketContext(qf *QueueFile) string {
	var completed []QueueTicket
	for _, t := range qf.Tickets {
		if t.Status == "completed" {
			completed = append(completed, t)
		}
	}
	if len(completed) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Previous Queue Tickets (completed)\n")
	b.WriteString("The following tickets were processed earlier in this queue:\n\n")
	for _, t := range completed {
		b.WriteString(fmt.Sprintf("- %s\n", t.Path))
	}
	return b.String()
}

// findTicketIdxByPath finds a ticket in the queue file by its path. Returns -1 if not found.
func findTicketIdxByPath(qf *QueueFile, path string) int {
	for i, t := range qf.Tickets {
		if t.Path == path {
			return i
		}
	}
	return -1
}

// findTicketIdx finds a ticket in the queue by path and request number.
func findTicketIdx(qf *QueueFile, path string, requestNum int) int {
	for i, t := range qf.Tickets {
		if t.Path == path && t.RequestNum == requestNum {
			return i
		}
	}
	return -1
}

// workerRunVerification runs the verification pass for a completed ticket.
// Returns true if the ticket passes verification (or verification is skipped).
// Returns false if verification fails (ticket should be re-processed).
func workerRunVerification(ctx context.Context, baseDir string, ticket QueueTicket, runner Runner, promptLoader PromptLoader, notifier Notifier, claudeArgs []string, queuePath string) bool {
	// Read ticket content to check SkipVerification
	content, err := os.ReadFile(ticket.Path)
	if err != nil {
		fmt.Printf("Warning: could not read ticket for verification check: %v\n", err)
		return true // can't check, assume verified
	}
	contentStr := string(content)

	if extractFrontmatterBool(contentStr, "SkipVerification") {
		fmt.Printf("Ticket %s: SkipVerification=true, auto-marking as verified\n", filepath.Base(ticket.Path))
		markAsVerified(ticket.Path)
		return true
	}

	// Build verification prompt
	verifyFile := "prompts/verify.md"

	// Check if verify prompt exists before attempting verification.
	// If it doesn't exist, skip verification (treat as verified).
	verifyPath := filepath.Join(baseDir, verifyFile)
	if _, statErr := os.Stat(verifyPath); os.IsNotExist(statErr) {
		fmt.Printf("Ticket %s: no verify prompt (%s), skipping verification\n", filepath.Base(ticket.Path), verifyFile)
		return true
	}

	fmt.Printf("\n─── Verifying ticket: %s ───\n", filepath.Base(ticket.Path))

	prompt, err := promptLoader.Load(baseDir, verifyFile, "Tickets to verify:", []string{ticket.Path}, "")
	if err != nil {
		fmt.Printf("Error building verify prompt: %v\n", err)
		return true // can't load prompt, skip verification
	}

	shortcutsFile := filepath.Join(baseDir, "workspaces", ticket.Workspace, "shortcuts.md")
	prompt = strings.ReplaceAll(prompt, "{{SHORTCUTS_PATH}}", shortcutsFile)
	prompt = strings.ReplaceAll(prompt, "{{WORKSPACE}}", ticket.Workspace)
	prompt = strings.ReplaceAll(prompt, "{{QUEUE_ID}}", queueIDFromPath(queuePath))

	// Append workspace verify prompt if available
	wsVerifyPrompt := loadWorkspacePrompt(baseDir, ticket.Workspace, "prompts/verify.md")
	if wsVerifyPrompt != "" {
		prompt += "\n\n" + wsVerifyPrompt
	}

	exitCode, err := runner.Run(ctx, prompt, claudeArgs)

	// Update timestamp after verification
	updateTicketsTimestamp([]string{ticket.Path})

	if err != nil {
		fmt.Printf("Error running verification: %v\n", err)
		return false
	}

	if exitCode == 0 {
		fmt.Printf("Ticket %s: verification passed\n", filepath.Base(ticket.Path))
		markAsVerified(ticket.Path)
		return true
	}

	// Claude CLI exits with -1 ~95% of the time in pipe mode (killed by signal).
	// Check if the verifier updated the frontmatter before being killed.
	// If the ticket is already "completed + verified", treat as success.
	if exitCode != 0 {
		verifyContent, readErr := os.ReadFile(ticket.Path)
		if readErr == nil {
			fmStatus := strings.ToLower(strings.TrimSpace(extractFrontmatterStatus(string(verifyContent))))
			if strings.Contains(fmStatus, "completed") && strings.Contains(fmStatus, "verified") {
				fmt.Printf("Ticket %s: verification exited %d but frontmatter says completed + verified, treating as passed\n", filepath.Base(ticket.Path), exitCode)
				return true
			}
		}
	}

	fmt.Printf("Ticket %s: verification failed (exit code %d)\n", filepath.Base(ticket.Path), exitCode)
	return false
}

// workerReconcileTicket checks ticket frontmatter before running Claude.
// If a previous worker died after Claude wrote a completion status but before
// the queue file was updated, the queue still shows "working"/"pending" while
// the ticket is already done. This function detects that case and handles it
// without re-running Claude.
//
// Returns true if the ticket was reconciled (caller should continue the loop).
// Returns false if the ticket needs normal processing.
func workerReconcileTicket(ctx context.Context, baseDir string, ticket QueueTicket, runner Runner, promptLoader PromptLoader, notifier Notifier, queuePath string) bool {
	// Skip reconciliation for additional requests — the frontmatter status
	// belongs to the original ticket, not the additional request. The additional
	// request's state is tracked in SQLite, not frontmatter.
	if ticket.RequestNum > 0 {
		return false
	}

	content, err := os.ReadFile(ticket.Path)
	if err != nil {
		return false // can't read, proceed normally
	}
	fmStatus := strings.ToLower(strings.TrimSpace(extractFrontmatterStatus(string(content))))

	isVerified := strings.Contains(fmStatus, "completed") && strings.Contains(fmStatus, "verified")
	isCompleted := strings.Contains(fmStatus, "completed") && !strings.Contains(fmStatus, "not completed")

	if !isCompleted {
		return false // ticket not completed, needs normal processing
	}

	if isVerified {
		// Already completed + verified — just advance the queue
		fmt.Printf("Reconcile: ticket %s already completed + verified, advancing queue\n", filepath.Base(ticket.Path))
		freshQf, readErr := readQueueFileFromPath(queuePath)
		if readErr != nil {
			return false
		}
		idx := findTicketIdx(freshQf, ticket.Path, ticket.RequestNum)
		if idx < 0 {
			return true // removed from queue
		}
		freshQf.Tickets[idx].Status = "completed"
		freshQf.Tickets[idx].Unread = true
		advanceQueueIndex(freshQf)
		writeWorkerHeartbeatOnQf(freshQf)
		writeQueueFileDataToPath(freshQf, queuePath)
		return true
	}

	// Completed but not verified — check MinIterations first
	if shouldReprocess := workerCheckMinIterations(ticket.Path); shouldReprocess {
		fmt.Printf("Reconcile: ticket %s completed in frontmatter but MinIterations not met, processing normally\n", filepath.Base(ticket.Path))
		return false
	}

	// Run verification only (skip the work pass)
	fmt.Printf("Reconcile: ticket %s already completed, running verification only\n", filepath.Base(ticket.Path))

	// Resolve workspace directory for claude args
	workDir := resolveWorkDirForTicket(baseDir, ticket.Workspace)
	claudeArgs := []string{}
	if yolo {
		claudeArgs = append(claudeArgs, "--model", "opus", "--dangerously-skip-permissions")
	}
	if workDir != "" {
		claudeArgs = append(claudeArgs, "--add-dir", workDir)
	}

	// Mark as working in queue while verifying
	freshQf, readErr := readQueueFileFromPath(queuePath)
	if readErr != nil {
		return false
	}
	idx := findTicketIdx(freshQf, ticket.Path, ticket.RequestNum)
	if idx < 0 {
		return true // removed from queue
	}
	freshQf.Tickets[idx].Status = "working"
	freshQf.Tickets[idx].StartedAt = time.Now().Unix()
	writeWorkerHeartbeatOnQf(freshQf)
	writeQueueFileDataToPath(freshQf, queuePath)

	verified := workerRunVerification(ctx, baseDir, ticket, runner, promptLoader, notifier, claudeArgs, queuePath)

	// Re-read queue file after verification (may have taken time)
	freshQf2, readErr2 := readQueueFileFromPath(queuePath)
	if readErr2 != nil {
		return true
	}
	idx2 := findTicketIdx(freshQf2, ticket.Path, ticket.RequestNum)
	if idx2 < 0 {
		return true // removed from queue during verification
	}

	if verified {
		fmt.Printf("Reconcile: ticket %s verified successfully\n", filepath.Base(ticket.Path))
		freshQf2.Tickets[idx2].Status = "completed"
		freshQf2.Tickets[idx2].Unread = true
		if notifier != nil {
			_ = notifier.Notify("Wiggums Worker", fmt.Sprintf("Reconciled & verified: %s", filepath.Base(ticket.Path)), "")
		}
	} else {
		fmt.Printf("Reconcile: ticket %s verification failed, will retry\n", filepath.Base(ticket.Path))
		freshQf2.Tickets[idx2].Status = "pending"
	}

	advanceQueueIndex(freshQf2)
	writeWorkerHeartbeatOnQf(freshQf2)
	writeQueueFileDataToPath(freshQf2, queuePath)
	return true
}

// workerCheckMinIterations reads a ticket file and returns true if MinIterations
// is set and CurIteration has not yet reached it, meaning the ticket should be
// re-processed rather than marked as completed.
func workerCheckMinIterations(ticketPath string) bool {
	content, err := os.ReadFile(ticketPath)
	if err != nil {
		return false
	}
	contentStr := string(content)
	minIter := extractFrontmatterInt(contentStr, "MinIterations")
	if minIter == 0 {
		return false
	}
	curIter := extractFrontmatterInt(contentStr, "CurIteration")
	return curIter < minIter
}
