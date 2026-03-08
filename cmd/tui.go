package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"wiggums/database"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

var activeTabStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57"))
var inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

// queueDelegate wraps DefaultDelegate to color the currently-working item in yellow
// and unread items in subtle green.
type queueDelegate struct {
	list.DefaultDelegate
	currentStyles list.DefaultItemStyles
	unreadStyles  list.DefaultItemStyles
}

// newQueueDelegate creates a delegate that highlights the current working item in yellow.
func newQueueDelegate() queueDelegate {
	d := list.NewDefaultDelegate()

	cs := list.NewDefaultItemStyles()
	cs.NormalTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Padding(0, 0, 0, 2)
	cs.NormalDesc = lipgloss.NewStyle().
		Foreground(lipgloss.Color("228")).
		Padding(0, 0, 0, 2)
	cs.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("226")).
		Foreground(lipgloss.Color("226")).
		Padding(0, 0, 0, 1)
	cs.SelectedDesc = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("228")).
		Foreground(lipgloss.Color("228")).
		Padding(0, 0, 0, 1)
	cs.DimmedTitle = d.Styles.DimmedTitle
	cs.DimmedDesc = d.Styles.DimmedDesc
	cs.FilterMatch = d.Styles.FilterMatch

	// Subtle green styles for unread items
	us := list.NewDefaultItemStyles()
	us.NormalTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("114")).
		Padding(0, 0, 0, 2)
	us.NormalDesc = lipgloss.NewStyle().
		Foreground(lipgloss.Color("114")).
		Padding(0, 0, 0, 2)
	us.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("114")).
		Foreground(lipgloss.Color("114")).
		Padding(0, 0, 0, 1)
	us.SelectedDesc = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("114")).
		Foreground(lipgloss.Color("114")).
		Padding(0, 0, 0, 1)
	us.DimmedTitle = d.Styles.DimmedTitle
	us.DimmedDesc = d.Styles.DimmedDesc
	us.FilterMatch = d.Styles.FilterMatch

	return queueDelegate{DefaultDelegate: d, currentStyles: cs, unreadStyles: us}
}

// Render overrides DefaultDelegate.Render to apply yellow styling for the current item
// and subtle green for unread items.
func (d queueDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if ti, ok := item.(tuiTicketItem); ok {
		if ti.current {
			d.Styles = d.currentStyles
		} else if ti.unread {
			d.Styles = d.unreadStyles
		}
	}
	d.DefaultDelegate.Render(w, m, index, item)
}

// tuiTab represents which tab is currently active.
type tuiTab int

const (
	tuiTabAll   tuiTab = iota // browsing all tickets
	tuiTabQueue               // browsing the work queue
)

// tuiTicketItem implements list.Item for display in the Bubble Tea list.
type tuiTicketItem struct {
	workspace        string
	filename         string
	title            string
	status           string
	filePath         string        // absolute path to the ticket file
	skipVerification bool          // SkipVerification frontmatter value
	minIterations    int           // MinIterations frontmatter value
	createdAt        time.Time     // parsed from epoch prefix in filename
	selected         bool          // whether the ticket is selected for the queue
	current          bool          // whether this is the currently processing ticket in the queue
	workerStatus     string        // worker-reported status: "pending", "working", "completed"
	requestNum       int           // 0 = original ticket, 1+ = additional user request #N
	isDraft          bool          // draft requests are visible but not actionable until activated
	runDuration      time.Duration // total processing time from ticket_runs
	workStartedAt    time.Time     // when the worker started processing this ticket (from queue file)
	comment          string        // short user comment (max 50 chars) displayed in description
	unread           bool          // true when ticket completed but user hasn't pressed 'o' yet
	preprocessPrompt string        // preprocessing instruction for this ticket
	preprocessStatus string        // "pending", "in_progress", "completed"
}

func (t tuiTicketItem) Title() string {
	icon := "○"
	verified := false
	switch {
	case strings.Contains(t.status, "completed + verified"):
		icon = "◆"
		verified = true
	case strings.Contains(t.status, "not completed"):
		// "not completed" contains "completed" — must check before the generic "completed" case
		icon = "○"
	case strings.Contains(t.status, "completed"):
		icon = "◆"
	case strings.Contains(t.status, "in_progress"):
		icon = "▶"
	}
	prefix := ""
	if t.current {
		prefix = ">> "
	}
	suffix := ""
	if verified {
		suffix = " :v"
	}
	displayTitle := t.title
	if t.requestNum > 0 {
		if t.isDraft {
			displayTitle = fmt.Sprintf("↳ draft #%d: %s", t.requestNum, t.title)
		} else {
			displayTitle = fmt.Sprintf("↳ request #%d: %s", t.requestNum, t.title)
		}
	}
	if t.isDraft {
		icon = "✎"
	}
	unreadMarker := ""
	if t.unread {
		unreadMarker = "*"
	}
	// Preprocessing status indicator (only show if a prompt is set)
	ppIndicator := ""
	if t.preprocessPrompt != "" {
		switch t.preprocessStatus {
		case "pending":
			ppIndicator = " pp"
		case "in_progress":
			ppIndicator = " pp..."
		case "completed":
			ppIndicator = " pp:✓"
		}
	}
	return fmt.Sprintf("%s%s%s %s%s%s", prefix, icon, unreadMarker, displayTitle, suffix, ppIndicator)
}

func (t tuiTicketItem) Description() string {
	desc := fmt.Sprintf("[%s] %s", t.workspace, t.status)
	if !t.createdAt.IsZero() {
		desc += fmt.Sprintf(" | %s", t.createdAt.Format("1/2 3:04PM"))
	}
	if t.workerStatus == "working" && !t.workStartedAt.IsZero() {
		elapsed := time.Since(t.workStartedAt)
		desc += fmt.Sprintf(" | %s", formatDurationSeconds(elapsed))
	} else if t.runDuration > 0 {
		desc += fmt.Sprintf(" | %s", formatDuration(t.runDuration))
	}
	if t.requestNum > 0 {
		if t.isDraft {
			desc += fmt.Sprintf(" | draft #%d", t.requestNum)
		} else {
			desc += fmt.Sprintf(" | request #%d", t.requestNum)
		}
	}
	if t.minIterations > 0 {
		desc += fmt.Sprintf(" | min:%d", t.minIterations)
	}
	if t.skipVerification {
		desc += " | skip-verify"
	}
	if t.comment != "" {
		desc += " | " + t.comment
	}
	return desc
}

// formatDurationSeconds formats a duration with second precision like "1m30s", "45s", "2h5m".
// Used for actively-working tickets where second-level granularity is useful.
func formatDurationSeconds(d time.Duration) string {
	d = d.Truncate(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	if m > 0 && s > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	if s > 0 {
		return fmt.Sprintf("%ds", s)
	}
	return "0s"
}

// formatDuration formats a duration as a compact human-readable string like "30m", "1h30m", "2h".
func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	// Less than 1 minute, show seconds
	s := int(d.Truncate(time.Second).Seconds())
	if s > 0 {
		return fmt.Sprintf("%ds", s)
	}
	return "<1m"
}

func (t tuiTicketItem) FilterValue() string {
	return t.title + " " + t.workspace
}

// tuiMode represents the current input mode of the TUI.
type tuiMode int

const (
	tuiModeList                  tuiMode = iota // browsing the ticket list
	tuiModeMinIterInput                         // entering MinIterations value
	tuiModeNewTicketWorkspace                   // wizard step 1: choose workspace
	tuiModeNewTicketName                        // wizard step 2: enter ticket name
	tuiModeNewTicketInstructions                // wizard step 3: enter instructions
	tuiModeAdditionalContext                    // entering additional context for a ticket
	tuiModeHelp                                 // showing help overlay
	tuiModeRenameQueue                          // renaming the work queue
	tuiModeQueuePrompt                          // editing the queue prompt
	tuiModeViewShortcuts                        // viewing queue shortcuts
	tuiModeQueuePicker                          // picking/creating a queue
	tuiModeNewQueueName                         // entering name for new queue
	tuiModeConfirmRemove                        // confirming removal from queue
	tuiModeConfirmReset                         // confirming ticket reset
	tuiModeConfirmActivate                      // confirming draft activation
	tuiModeConfirmDeleteQueue                   // confirming queue deletion
	tuiModeComment                              // entering a comment for a ticket
	tuiModePreprocessPrompt                     // entering preprocessing instruction
)

// tuiQueueItem implements list.Item for display in the queue picker list.
type tuiQueueItem struct {
	id          string // queue file ID (e.g. "default", "my-queue")
	name        string // display name from QueueFile.Name
	pinned      bool
	pinOrder    int  // ordering of pinned queues (lower = first)
	ticketCount int  // number of tickets in the queue
	running     bool // whether the queue is currently running
	active      bool // whether this is the currently loaded queue
}

func (q tuiQueueItem) Title() string {
	pin := "  "
	if q.pinned {
		pin = "* "
	}
	marker := ""
	if q.active {
		marker = " (active)"
	}
	return fmt.Sprintf("%s%s%s", pin, q.name, marker)
}

func (q tuiQueueItem) Description() string {
	parts := []string{q.id}
	parts = append(parts, fmt.Sprintf("%d remaining", q.ticketCount))
	if q.running {
		parts = append(parts, "running")
	}
	return strings.Join(parts, " | ")
}

func (q tuiQueueItem) FilterValue() string {
	return q.name + " " + q.id
}

// pinnedQueue is a lightweight cache of pinned queue metadata for the tab bar.
type pinnedQueue struct {
	id          string
	name        string
	ticketCount int
	running     bool
	pinOrder    int
}

// maxPinnedQueues is the maximum number of queues that can be pinned.
const maxPinnedQueues = 5

// countRemainingTickets returns the number of tickets that are not completed or drafts.
func countRemainingTickets(tickets []QueueTicket) int {
	var remaining int
	for _, t := range tickets {
		if t.IsDraft {
			continue
		}
		switch t.Status {
		case "completed":
			// done
		default:
			remaining++
		}
	}
	return remaining
}

// loadQueuePickerItems reads all queue files and returns sorted tuiQueueItem list items.
// Pinned queues sort first, then alphabetically within each group.
func loadQueuePickerItems(activeID string) []list.Item {
	ids := listQueueIDs()
	var items []tuiQueueItem
	for _, id := range ids {
		qf, err := readQueueFileFromPath(queueFilePathForID(id))
		name := id
		var ticketCount int
		var running, pinned bool
		var pinOrder int
		if err == nil {
			if qf.Name != "" {
				name = qf.Name
			}
			ticketCount = countRemainingTickets(qf.Tickets)
			running = qf.Running
			pinned = qf.Pinned
			pinOrder = qf.PinOrder
		}
		items = append(items, tuiQueueItem{
			id:          id,
			name:        name,
			pinned:      pinned,
			pinOrder:    pinOrder,
			ticketCount: ticketCount,
			running:     running,
			active:      id == activeID,
		})
	}
	// Sort: pinned first (by PinOrder), then alphabetically by name for unpinned
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].pinned != items[j].pinned {
			return items[i].pinned
		}
		if items[i].pinned && items[j].pinned {
			return items[i].pinOrder < items[j].pinOrder
		}
		return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
	})
	result := make([]list.Item, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}

// loadPinnedQueues reads all queue files, filters to pinned, sorts by PinOrder,
// and returns up to maxPinnedQueues entries.
func loadPinnedQueues() []pinnedQueue {
	ids := listQueueIDs()
	var pinned []pinnedQueue
	for _, id := range ids {
		qf, err := readQueueFileFromPath(queueFilePathForID(id))
		if err != nil || !qf.Pinned {
			continue
		}
		name := id
		if qf.Name != "" {
			name = qf.Name
		}
		pinned = append(pinned, pinnedQueue{
			id:          id,
			name:        name,
			ticketCount: countRemainingTickets(qf.Tickets),
			running:     qf.Running,
			pinOrder:    qf.PinOrder,
		})
	}
	sort.SliceStable(pinned, func(i, j int) bool {
		return pinned[i].pinOrder < pinned[j].pinOrder
	})
	if len(pinned) > maxPinnedQueues {
		pinned = pinned[:maxPinnedQueues]
	}
	return pinned
}

// normalizePinOrders ensures all pinned queues have non-zero PinOrder values.
// Called once on startup. Assigns sequential values to any pinned queues with PinOrder 0,
// preserving existing ordering for queues that already have a PinOrder.
func normalizePinOrders() {
	ids := listQueueIDs()
	type pinEntry struct {
		id       string
		path     string
		qf       *QueueFile
		pinOrder int
	}
	var needsFix []pinEntry
	maxOrder := 0
	for _, id := range ids {
		path := queueFilePathForID(id)
		qf, err := readQueueFileFromPath(path)
		if err != nil || !qf.Pinned {
			continue
		}
		if qf.PinOrder > maxOrder {
			maxOrder = qf.PinOrder
		}
		if qf.PinOrder == 0 {
			needsFix = append(needsFix, pinEntry{id: id, path: path, qf: qf})
		}
	}
	// Assign sequential PinOrder to those missing it
	for i, entry := range needsFix {
		entry.qf.PinOrder = maxOrder + i + 1
		writeQueueFileDataToPath(entry.qf, entry.path)
	}
}

// refreshPinnedQueues reloads the pinned queue cache on the model.
func (m *tuiModel) refreshPinnedQueues() {
	m.pinnedQueues = loadPinnedQueues()
}

// isActiveQueuePinned returns true if the active queue is in the pinned list.
func (m *tuiModel) isActiveQueuePinned() bool {
	for _, pq := range m.pinnedQueues {
		if pq.id == m.activeQueueID {
			return true
		}
	}
	return false
}

// countPinnedQueues counts the number of pinned items in a list.
func countPinnedQueues(items []list.Item) int {
	count := 0
	for _, li := range items {
		if item, ok := li.(tuiQueueItem); ok && item.pinned {
			count++
		}
	}
	return count
}

// toggleQueuePin reads a queue file, sets its Pinned field, and writes it back.
// When pinning, assigns the next available PinOrder. When unpinning, clears PinOrder.
func toggleQueuePin(id string, pinned bool) error {
	path := queueFilePathForID(id)
	qf, err := readQueueFileFromPath(path)
	if err != nil {
		return err
	}
	qf.Pinned = pinned
	if pinned {
		qf.PinOrder = nextPinOrder()
	} else {
		qf.PinOrder = 0
	}
	return writeQueueFileDataToPath(qf, path)
}

// nextPinOrder returns one higher than the current maximum PinOrder across all queues.
func nextPinOrder() int {
	ids := listQueueIDs()
	maxOrder := 0
	for _, id := range ids {
		qf, err := readQueueFileFromPath(queueFilePathForID(id))
		if err != nil || !qf.Pinned {
			continue
		}
		if qf.PinOrder > maxOrder {
			maxOrder = qf.PinOrder
		}
	}
	return maxOrder + 1
}

// swapPinOrder swaps the PinOrder of two pinned queues by their IDs.
func swapPinOrder(id1, id2 string) error {
	path1 := queueFilePathForID(id1)
	path2 := queueFilePathForID(id2)
	qf1, err := readQueueFileFromPath(path1)
	if err != nil {
		return err
	}
	qf2, err := readQueueFileFromPath(path2)
	if err != nil {
		return err
	}
	qf1.PinOrder, qf2.PinOrder = qf2.PinOrder, qf1.PinOrder
	if err := writeQueueFileDataToPath(qf1, path1); err != nil {
		return err
	}
	return writeQueueFileDataToPath(qf2, path2)
}

// newTicketState holds the in-progress state for the new ticket wizard.
type newTicketState struct {
	workspace  string
	name       string
	addToQueue bool // auto-add to queue when created from queue tab
}

// tuiModel is the Bubble Tea model for the wiggums TUI.
type tuiModel struct {
	list                      list.Model
	baseDir                   string
	mode                      tuiMode
	tab                       tuiTab        // active tab
	queue                     list.Model    // queue tab list
	queueName                 string        // display name for the queue
	queuePrompt               string        // prompt injected into all queue tickets
	queueShortcuts            []string      // shortcuts memory injected into queue context
	queueRunning              bool          // whether the queue is actively processing
	currentQueueIdx           int           // index of the ticket currently being processed (-1 = none)
	workerLost                bool          // true if worker heartbeat is stale (>30s)
	activeQueueID             string        // ID of the active queue (default: "default")
	pinnedQueues              []pinnedQueue // cached pinned queues for tab bar (max 5)
	queuePickerList           list.Model    // queue picker list (populated lazily on Q press)
	textInput                 textinput.Model
	textArea                  textarea.Model
	newTicket                 newTicketState
	pendingRemovePath         string // filePath of ticket pending removal confirmation
	pendingRemoveRequestNum   int    // requestNum of ticket pending removal confirmation
	pendingResetPath          string // filePath of ticket pending reset confirmation
	pendingActivatePath       string // filePath of draft pending activation
	pendingActivateRequestNum int    // requestNum of draft pending activation
	pendingDeleteQueueID      string // queue ID pending deletion confirmation
	renameQueueID             string // queue ID being renamed (set when entering rename from picker)
	defaultPreprocessPrompt   string // last-used preprocessing prompt for quick reuse
}

func newTuiModel(baseDir string) (tuiModel, error) {
	items, err := loadAllTickets(baseDir)
	if err != nil {
		return tuiModel{}, err
	}

	delegate := newQueueDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Wiggums Tickets"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	// Queue list starts empty
	qDelegate := newQueueDelegate()
	q := list.New([]list.Item{}, qDelegate, 0, 0)
	q.Title = "Work Queue"
	q.SetShowStatusBar(true)
	q.SetFilteringEnabled(true)

	ti := textinput.New()
	ti.Placeholder = "0"
	ti.CharLimit = 5
	ti.Width = 20

	ta := textarea.New()
	ta.Placeholder = "Describe what needs to be done..."
	ta.SetWidth(60)
	ta.SetHeight(5)

	// Queue picker list starts empty (populated lazily on Q press)
	qplDelegate := list.NewDefaultDelegate()
	qpl := list.New([]list.Item{}, qplDelegate, 0, 0)
	qpl.Title = "Queue Picker"

	model := tuiModel{list: l, queue: q, queueName: "Work Queue", currentQueueIdx: -1, baseDir: baseDir, queuePickerList: qpl, textInput: ti, textArea: ta, activeQueueID: "default"}

	// Migrate old queue.json to new queues/<id>.json format if needed
	migrateQueueFile()

	// Ensure all pinned queues have PinOrder set (migration for pre-PinOrder queues)
	normalizePinOrders()

	// Restore queue state from disk if available
	model.restoreQueueState()

	// Load pinned queues for tab bar
	model.refreshPinnedQueues()

	return model, nil
}

// restoreQueueState loads queue state from the queue file on disk and
// restores queue items, name, prompt, and running state.
func (m *tuiModel) restoreQueueState() {
	qf, err := readQueueFileFromPath(queueFilePathForID(m.activeQueueID))
	if err != nil {
		return
	}

	// Restore queue name, prompt, and shortcuts
	if qf.Name != "" {
		m.queueName = qf.Name
		m.queue.Title = qf.Name
	}
	m.queuePrompt = qf.Prompt
	m.queueShortcuts = qf.Shortcuts
	m.defaultPreprocessPrompt = qf.DefaultPreprocessPrompt

	// Build a map of all loaded tickets by filePath:requestNum for fast lookup
	allItems := m.list.Items()
	byKey := make(map[string]int)
	for i, li := range allItems {
		if item, ok := li.(tuiTicketItem); ok {
			byKey[additionalRequestKey(item.filePath, item.requestNum)] = i
		}
	}

	// Restore queue items from the queue file
	var queueItems []list.Item
	for _, qt := range qf.Tickets {
		key := additionalRequestKey(qt.Path, qt.RequestNum)
		if idx, exists := byKey[key]; exists {
			item := allItems[idx].(tuiTicketItem)
			item.selected = true
			item.workerStatus = qt.Status
			if item.requestNum > 0 {
				// Normalize additional-request status from queue state immediately
				// during restore so start/stop actions don't persist stale DB values.
				if mapped := mapQueueStatusToAdditionalRequestStatus(qt.Status); mapped != "" && item.status != mapped {
					item.status = mapped
					if database.DB != nil {
						_ = database.UpdateAdditionalRequestStatus(context.Background(), item.filePath, item.requestNum, mapped)
					}
				}
			}
			item.isDraft = qt.IsDraft
			item.unread = qt.Unread
			if qt.StartedAt > 0 {
				item.workStartedAt = time.Unix(qt.StartedAt, 0)
			}
			if qt.Comment != "" {
				item.comment = qt.Comment
			}
			item.preprocessPrompt = qt.PreprocessPrompt
			item.preprocessStatus = qt.PreprocessStatus
			queueItems = append(queueItems, item)
			// Mark selected in the all-tickets list too
			allItems[idx] = item
		}
	}
	m.list.SetItems(allItems)
	m.queue.SetItems(queueItems)

	// Restore running state only if there are actual queue items
	if len(queueItems) > 0 {
		m.queueRunning = qf.Running
		m.currentQueueIdx = m.computeCurrentQueueIdx()
		if m.queueRunning {
			m.syncQueueCurrentMarker()
		}
	}
}

// statusPriority returns a sort key for ticket status.
// Lower values sort first (incomplete tickets appear before completed).
func statusPriority(status string) int {
	switch {
	case strings.Contains(status, "in_progress"):
		return 0
	case strings.Contains(status, "completed + verified"):
		return 3
	case strings.Contains(status, "not completed"):
		return 1
	case strings.Contains(status, "completed"):
		return 2
	default: // "created", "unknown", etc.
		return 1
	}
}

// sortTicketItems sorts tickets by creation date with most recent first.
func sortTicketItems(items []list.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a := items[i].(tuiTicketItem)
		b := items[j].(tuiTicketItem)
		return a.createdAt.After(b.createdAt)
	})
}

// toggleSkipVerification reads the ticket file and flips the SkipVerification field.
// Returns the new value after toggling.
func toggleSkipVerification(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(content), "\n")
	delimCount := 0
	found := false
	var newVal bool
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			delimCount++
			if delimCount >= 2 {
				if !found {
					// Insert before closing ---
					newLine := "SkipVerification: true"
					lines = append(lines[:i+1], lines[i:]...)
					lines[i] = newLine
					newVal = true
					found = true
				}
				break
			}
			continue
		}
		if delimCount == 1 {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, "skipverification:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					cur := strings.ToLower(strings.TrimSpace(parts[1]))
					oldVal := cur == "true" || cur == "yes" || cur == "1"
					newVal = !oldVal
					lines[i] = fmt.Sprintf("SkipVerification: %t", newVal)
					found = true
				}
				break
			}
		}
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return false, err
	}
	return newVal, nil
}

// setMinIterations reads the ticket file and sets the MinIterations frontmatter field.
func setMinIterations(path string, value int) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	valStr := fmt.Sprintf(`"%d"`, value)
	lines := strings.Split(string(content), "\n")
	delimCount := 0
	found := false
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			delimCount++
			if delimCount >= 2 {
				if !found {
					// Insert before closing ---
					newLine := fmt.Sprintf("MinIterations: %s", valStr)
					lines = append(lines[:i+1], lines[i:]...)
					lines[i] = newLine
					found = true
				}
				break
			}
			continue
		}
		if delimCount == 1 {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, "miniterations:") {
				lines[i] = fmt.Sprintf("MinIterations: %s", valStr)
				found = true
				break
			}
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// setComment writes a Comment value into a ticket's YAML frontmatter.
// If the key already exists it replaces the value; otherwise it inserts before the closing ---.
// An empty comment removes the Comment line from frontmatter.
func setComment(path string, comment string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	delimCount := 0
	found := false
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			delimCount++
			if delimCount >= 2 {
				if !found && comment != "" {
					// Insert before closing ---
					newLine := fmt.Sprintf("Comment: %s", comment)
					lines = append(lines[:i+1], lines[i:]...)
					lines[i] = newLine
				}
				break
			}
			continue
		}
		if delimCount == 1 {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, "comment:") {
				if comment == "" {
					// Remove the line
					lines = append(lines[:i], lines[i+1:]...)
				} else {
					lines[i] = fmt.Sprintf("Comment: %s", comment)
				}
				found = true
				break
			}
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// extractFrontmatterComment extracts the Comment value from YAML frontmatter.
func extractFrontmatterComment(content string) string {
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
			if strings.HasPrefix(lower, "comment:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return ""
}

// appendAdditionalContext adds a new "Additional User Request" section to a ticket file.
// It inserts the section before the "---\nBelow to be filled by agent" divider and
// removes the placeholder text. The original ticket's status is NOT changed.
// When createItem is true, a record is created in SQLite to track the additional request
// as a separate list item.
// isDraft marks the request as a draft. Draft content is stored only in SQLite (not written
// to the ticket file) so that Claude doesn't see it until the draft is activated and processed.
func appendAdditionalContext(path string, text string, createItem bool, isDraft bool) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	contentStr := string(content)

	// Determine request number. Use SQLite max request_num when DB is available
	// to correctly account for drafts that aren't written to the file.
	// Fall back to file section counting when DB is unavailable.
	var num int
	if database.DB != nil {
		maxNum, dbErr := database.MaxRequestNum(context.Background(), path)
		if dbErr == nil {
			num = maxNum + 1
		} else {
			num = countAdditionalRequests(contentStr) + 1
		}
	} else {
		num = countAdditionalRequests(contentStr) + 1
	}

	// Read the ticket's current frontmatter status to persist as original status.
	// This allows restoring the ticket's display status after processing (immutable history).
	originalStatus := strings.ToLower(strings.TrimSpace(extractFrontmatterStatus(contentStr)))
	if idx := strings.Index(originalStatus, ":"); idx >= 0 {
		originalStatus = strings.TrimSpace(originalStatus[idx+1:])
	}

	// For drafts with DB available: store content in SQLite only, don't modify the ticket file.
	// This prevents Claude from seeing draft content when processing the original ticket.
	if isDraft && createItem && database.DB != nil {
		_, _ = database.CreateAdditionalRequest(context.Background(), path, num, true, text, originalStatus)
		return nil
	}

	// Remove placeholder text
	contentStr = strings.Replace(contentStr, "To be populated with further user request\n", "", 1)

	// Build the new section
	timestamp := time.Now().Format("2006-01-02 15:04")
	newSection := fmt.Sprintf("\n### Additional User Request #%d — %s\n%s\n", num, timestamp, text)

	// Insert before the divider
	divider := "---\nBelow to be filled by agent"
	idx := strings.Index(contentStr, divider)
	if idx != -1 {
		contentStr = contentStr[:idx] + newSection + contentStr[idx:]
	} else {
		// If divider not found, append to the end
		contentStr += newSection
	}

	if err := os.WriteFile(path, []byte(contentStr), 0644); err != nil {
		return err
	}

	// Create a record in SQLite to track this additional request's status.
	// Gracefully skip if DB is not initialized (e.g., in tests without DB setup).
	if createItem && database.DB != nil {
		_, _ = database.CreateAdditionalRequest(context.Background(), path, num, false, text, originalStatus)
	}

	return nil
}

// QueueTicket represents a ticket in the persisted queue file.
type QueueTicket struct {
	Path              string `json:"path"`
	Workspace         string `json:"workspace"`
	Status            string `json:"status"`                      // "pending", "working", "completed"
	RequestNum        int    `json:"request_num,omitempty"`       // 0 = original ticket, 1+ = additional request #N
	IsDraft           bool   `json:"is_draft,omitempty"`          // draft items are skipped by the worker
	StartedAt         int64  `json:"started_at,omitempty"`        // Unix timestamp when worker started processing
	Comment           string `json:"comment,omitempty"`           // short user comment (max 50 chars)
	Unread            bool   `json:"unread,omitempty"`            // true when completed but user hasn't pressed 'o' yet
	PreprocessPrompt  string `json:"preprocess_prompt,omitempty"` // preprocessing instruction for this ticket
	PreprocessStatus  string `json:"preprocess_status,omitempty"` // "pending", "in_progress", "completed"
}

// QueueFile is the JSON structure persisted to ~/.wiggums/queue.json.
type QueueFile struct {
	Name                    string        `json:"name"`
	Prompt                  string        `json:"prompt,omitempty"`
	Shortcuts               []string      `json:"shortcuts,omitempty"`
	Tickets                 []QueueTicket `json:"tickets"`
	Running                 bool          `json:"running"`
	LastHeartbeat           int64         `json:"last_heartbeat,omitempty"`            // Unix timestamp of last worker heartbeat
	WorkerPID               int           `json:"worker_pid,omitempty"`                // PID of the worker process for liveness checks
	Pinned                  bool          `json:"pinned,omitempty"`                    // Whether the queue is pinned to the top of the picker
	PinOrder                int           `json:"pin_order,omitempty"`                 // Ordering of pinned queues (lower = first, 0 = unset)
	DefaultPreprocessPrompt string        `json:"default_preprocess_prompt,omitempty"` // last-used preprocessing prompt for quick reuse
}

// queueFilePath returns the path to the default queue file.
// Uses the "default" queue ID for backward compatibility.
func queueFilePath() string {
	return queueFilePathForID("default")
}

// queueFilePathForID returns the path to a specific queue file by ID.
// Queue files live in ~/.wiggums/queues/<id>.json.
func queueFilePathForID(id string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".wiggums", "queues", id+".json")
}

// queueIDFromPath extracts the queue ID from a queue file path (e.g. "/.../.wiggums/queues/default.json" -> "default").
func queueIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".json")
}

// queuesDir returns the path to the queues directory.
func queuesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".wiggums", "queues")
}

// listQueueIDs returns the IDs of all existing queue files.
func listQueueIDs() []string {
	dir := queuesDir()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			ids = append(ids, strings.TrimSuffix(name, ".json"))
		}
	}
	return ids
}

// migrateQueueFile migrates the old ~/.wiggums/queue.json to the new
// ~/.wiggums/queues/default.json format if needed.
func migrateQueueFile() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	oldPath := filepath.Join(home, ".wiggums", "queue.json")
	newPath := queueFilePathForID("default")

	// Check if old file exists and new file does not
	if _, err := os.Stat(oldPath); err != nil {
		return // old file doesn't exist, nothing to migrate
	}
	if _, err := os.Stat(newPath); err == nil {
		return // new file already exists, don't overwrite
	}

	// Create queues directory
	os.MkdirAll(filepath.Dir(newPath), 0755)

	// Move old file to new location
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return
	}
	if err := os.WriteFile(newPath, data, 0644); err != nil {
		return
	}
	os.Remove(oldPath)
}

// buildQueueFileFromModel creates a QueueFile struct from the current TUI model state.
func buildQueueFileFromModel(m *tuiModel) QueueFile {
	qf := QueueFile{
		Name:                    m.queueName,
		Prompt:                  m.queuePrompt,
		Shortcuts:               m.queueShortcuts,
		Running:                 m.queueRunning,
		DefaultPreprocessPrompt: m.defaultPreprocessPrompt,
	}

	for i, qi := range m.queue.Items() {
		item, ok := qi.(tuiTicketItem)
		if !ok {
			continue
		}
		status := item.workerStatus
		if status == "" || status == "pending" {
			// If the ticket's frontmatter status is already completed/verified,
			// mark it as "completed" so the worker skips it but uses it as context.
			// "not completed" contains "completed" — must exclude it.
			isCompleted := strings.Contains(item.status, "completed") && !strings.Contains(item.status, "not completed")
			if isCompleted {
				status = "completed"
			} else {
				status = "pending"
				// Only mark the current item as "working" based on position.
				// Do NOT mark items before currentQueueIdx as "completed" by position —
				// the worker sets completed statuses explicitly. Position-based
				// "completed" is wrong after reordering (unprocessed tickets moved
				// before the current position get incorrectly marked as completed).
				if m.queueRunning && i == m.currentQueueIdx {
					status = "working"
				}
			}
		}
		var startedAt int64
		if !item.workStartedAt.IsZero() {
			startedAt = item.workStartedAt.Unix()
		}
		qf.Tickets = append(qf.Tickets, QueueTicket{
			Path:             item.filePath,
			Workspace:        item.workspace,
			Status:           status,
			RequestNum:       item.requestNum,
			IsDraft:          item.isDraft,
			StartedAt:        startedAt,
			Comment:          item.comment,
			Unread:           item.unread,
			PreprocessPrompt: item.preprocessPrompt,
			PreprocessStatus: item.preprocessStatus,
		})
	}

	return qf
}

// mergeWorkerStatuses preserves the worker's "completed" status from the existing
// queue file. This prevents a race condition where the TUI writes the queue file
// between a worker completion and the next 2-second sync poll, clobbering the
// worker's "completed" back to "pending" or "working".
func mergeWorkerStatuses(qf *QueueFile, existing *QueueFile) {
	if existing == nil {
		return
	}
	existingStatusMap := make(map[string]string)
	for _, et := range existing.Tickets {
		key := additionalRequestKey(et.Path, et.RequestNum)
		existingStatusMap[key] = et.Status
	}
	for i, qt := range qf.Tickets {
		key := additionalRequestKey(qt.Path, qt.RequestNum)
		if existingStatus, ok := existingStatusMap[key]; ok {
			if existingStatus == "completed" && qt.Status != "completed" {
				qf.Tickets[i].Status = existingStatus
			}
		}
	}
}

// mergePreprocessStatuses preserves the preprocessing worker's status updates
// from the existing queue file. Prevents the TUI from clobbering "in_progress"
// or "completed" back to "pending" during a sync cycle.
func mergePreprocessStatuses(qf *QueueFile, existing *QueueFile) {
	if existing == nil {
		return
	}
	existingMap := make(map[string]string)
	for _, et := range existing.Tickets {
		key := additionalRequestKey(et.Path, et.RequestNum)
		existingMap[key] = et.PreprocessStatus
	}
	for i, qt := range qf.Tickets {
		key := additionalRequestKey(qt.Path, qt.RequestNum)
		if existingStatus, ok := existingMap[key]; ok {
			// Only preserve worker's progress if the ticket still has a prompt set.
			// If the prompt was cleared, don't carry over stale status.
			if qt.PreprocessPrompt == "" {
				continue
			}
			// Preserve worker's progress: in_progress beats pending, completed beats all
			if existingStatus == "in_progress" && qt.PreprocessStatus == "pending" {
				qf.Tickets[i].PreprocessStatus = existingStatus
			}
			if existingStatus == "completed" {
				qf.Tickets[i].PreprocessStatus = existingStatus
			}
		}
	}
}

// writeQueueFile persists the current queue state to disk.
// Delegates to writeQueueFileDataToPath for atomic writes.
// Preserves the worker's LastHeartbeat by reading the existing file first.
func (m *tuiModel) writeQueueFile() error {
	path := queueFilePathForID(m.activeQueueID)
	if path == "" {
		return fmt.Errorf("could not determine queue file path")
	}

	devLog("TUI writeQueueFile: Running=%v currentIdx=%d queueLen=%d", m.queueRunning, m.currentQueueIdx, len(m.queue.Items()))
	qf := buildQueueFileFromModel(m)
	// Preserve fields managed outside the TUI model (worker heartbeat, pinned state)
	if existing, err := readQueueFileFromPath(path); err == nil {
		qf.LastHeartbeat = existing.LastHeartbeat
		qf.WorkerPID = existing.WorkerPID
		qf.Pinned = existing.Pinned
		qf.PinOrder = existing.PinOrder
		mergeWorkerStatuses(&qf, existing)
		mergePreprocessStatuses(&qf, existing)
	}
	return writeQueueFileDataToPath(&qf, path)
}

// readQueueFile reads the queue state from disk.
func readQueueFile() (*QueueFile, error) {
	path := queueFilePath()
	if path == "" {
		return nil, fmt.Errorf("could not determine queue file path")
	}
	return readQueueFileFromPath(path)
}

// readQueueFileFromPath reads queue state from a specific path. Used by tests.
func readQueueFileFromPath(path string) (*QueueFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var qf QueueFile
	if err := json.Unmarshal(data, &qf); err != nil {
		return nil, err
	}
	return &qf, nil
}

// writeQueueFileToPath writes queue state to a specific path. Used by tests.
// Delegates to writeQueueFileDataToPath for atomic writes.
// Preserves the worker's LastHeartbeat by reading the existing file first.
func writeQueueFileToPath(m *tuiModel, path string) error {
	qf := buildQueueFileFromModel(m)
	// Preserve fields managed outside the TUI model (worker heartbeat, pinned state)
	if existing, err := readQueueFileFromPath(path); err == nil {
		qf.LastHeartbeat = existing.LastHeartbeat
		qf.WorkerPID = existing.WorkerPID
		qf.Pinned = existing.Pinned
		qf.PinOrder = existing.PinOrder
		mergeWorkerStatuses(&qf, existing)
		mergePreprocessStatuses(&qf, existing)
	}
	return writeQueueFileDataToPath(&qf, path)
}

// shortcutsEqual compares two shortcuts slices by value.
func shortcutsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// estimateTokenCount estimates token count for a string using the ~4 chars per token heuristic.
func estimateTokenCount(s string) int {
	if len(s) == 0 {
		return 0
	}
	return (len(s) + 3) / 4 // round up
}

// queueContextTokens estimates the total token count of injected prompts for the queue.
func (m *tuiModel) queueContextTokens() int {
	total := 0
	if m.queuePrompt != "" {
		total += estimateTokenCount(m.queuePrompt)
	}
	for _, s := range m.queueShortcuts {
		total += estimateTokenCount(s)
	}
	return total
}

// parseEpochFromFilename extracts the Unix epoch timestamp from a ticket filename.
// Ticket filenames follow the pattern: [EPOCH]_[Title].md
func parseEpochFromFilename(filename string) time.Time {
	name := strings.TrimSuffix(filename, ".md")
	if idx := strings.IndexByte(name, '_'); idx > 0 {
		prefix := name[:idx]
		if epoch, err := strconv.ParseInt(prefix, 10, 64); err == nil {
			return time.Unix(epoch, 0)
		}
	}
	return time.Time{}
}

// additionalRequestRe matches "### Additional User Request #N" headers.
var additionalRequestRe = regexp.MustCompile(`(?m)^### Additional User Request #(\d+)`)

// countAdditionalRequests counts "### Additional User Request #N" sections in ticket content.
func countAdditionalRequests(content string) int {
	return len(additionalRequestRe.FindAllString(content, -1))
}

// loadAllTickets scans all workspaces and returns ticket items.
func loadAllTickets(baseDir string) ([]list.Item, error) {
	workspaces, err := listWorkspaceNames(baseDir)
	if err != nil {
		return nil, err
	}

	// Build a map of (ticketPath, requestNum) -> info from SQLite.
	// If the DB isn't initialized yet, we gracefully return empty map.
	reqInfoMap := loadAdditionalRequestStatuses()

	// Load run durations from SQLite. Key: "workspace:ticket_filename".
	durationMap := loadTicketDurations()

	var items []list.Item

	for _, ws := range workspaces {
		ticketsDir := filepath.Join(baseDir, "workspaces", ws, "tickets")
		entries, err := os.ReadDir(ticketsDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.EqualFold(e.Name(), "CLAUDE.md") {
				continue
			}

			fp := filepath.Join(ticketsDir, e.Name())
			content, err := os.ReadFile(fp)
			if err != nil {
				continue
			}

			contentStr := string(content)
			status := strings.ToLower(strings.TrimSpace(extractFrontmatterStatus(contentStr)))
			// Remove the "status:" prefix from the extracted line
			if idx := strings.Index(status, ":"); idx >= 0 {
				status = strings.TrimSpace(status[idx+1:])
			}
			if status == "" {
				status = "unknown"
			}

			// If frontmatter was mutated to "additional_user_request" by the worker,
			// restore the original status from the DB for display (immutable history).
			if status == "additional_user_request" && database.DB != nil {
				if origStatus, err := database.GetOriginalTicketStatus(context.Background(), fp); err == nil && origStatus != "" {
					status = origStatus
				}
			}

			baseTitle := ticketTitle(e.Name())
			created := parseEpochFromFilename(e.Name())

			// Add the original ticket item
			items = append(items, tuiTicketItem{
				workspace:        ws,
				filename:         e.Name(),
				title:            baseTitle,
				status:           status,
				filePath:         fp,
				skipVerification: extractFrontmatterBool(contentStr, "SkipVerification"),
				minIterations:    extractFrontmatterInt(contentStr, "MinIterations"),
				createdAt:        created,
				runDuration:      durationMap[ws+":"+e.Name()],
				comment:          extractFrontmatterComment(contentStr),
			})

			// Parse additional request sections and create separate items.
			// Create items for any ticket that has additional requests (in file or in SQLite).
			fileReqCount := countAdditionalRequests(contentStr)
			dbMaxNum := maxRequestNumFromMap(reqInfoMap, fp)
			if fileReqCount > 0 || dbMaxNum > 0 {
				// Use the max of file section count and DB max request_num.
				// Drafts exist only in SQLite (not in the file), so file count
				// alone would miss them.
				reqCount := fileReqCount
				if dbMaxNum > reqCount {
					reqCount = dbMaxNum
				}
				for i := 1; i <= reqCount; i++ {
					reqStatus := "created"
					reqIsDraft := false
					if info, ok := reqInfoMap[additionalRequestKey(fp, i)]; ok {
						reqStatus = info.status
						reqIsDraft = info.isDraft
					}
					items = append(items, tuiTicketItem{
						workspace:  ws,
						filename:   e.Name(),
						title:      baseTitle,
						status:     reqStatus,
						filePath:   fp,
						createdAt:  created,
						requestNum: i,
						isDraft:    reqIsDraft,
					})
				}
			}
		}
	}

	sortTicketItems(items)
	return items, nil
}

// additionalRequestKey builds a map key for looking up additional request status.
func additionalRequestKey(ticketPath string, requestNum int) string {
	return fmt.Sprintf("%s:%d", ticketPath, requestNum)
}

// additionalRequestInfo holds status and draft info for an additional request.
type additionalRequestInfo struct {
	status  string
	isDraft bool
}

// mapQueueStatusToAdditionalRequestStatus converts queue worker status to the
// canonical additional-request status stored/displayed in SQLite/TUI.
func mapQueueStatusToAdditionalRequestStatus(queueStatus string) string {
	switch queueStatus {
	case "pending":
		return "created"
	case "working":
		return "in_progress"
	case "completed":
		return "completed"
	default:
		return ""
	}
}

// maxRequestNumFromMap returns the highest request number for a given ticket path
// from the reqInfoMap. Returns 0 if no entries exist for this path.
func maxRequestNumFromMap(m map[string]additionalRequestInfo, ticketPath string) int {
	maxNum := 0
	for i := 1; ; i++ {
		if _, ok := m[additionalRequestKey(ticketPath, i)]; !ok {
			break
		}
		maxNum = i
	}
	return maxNum
}

// loadAdditionalRequestStatuses loads all additional request statuses and draft flags
// from SQLite into a map keyed by "ticketPath:requestNum". Returns empty map if DB is unavailable.
func loadAdditionalRequestStatuses() map[string]additionalRequestInfo {
	m := make(map[string]additionalRequestInfo)
	if database.DB == nil {
		return m
	}
	var reqs []database.AdditionalRequest
	err := database.DB.NewSelect().
		Model(&reqs).
		Scan(context.Background())
	if err != nil {
		return m
	}
	for _, r := range reqs {
		m[additionalRequestKey(r.TicketPath, r.RequestNum)] = additionalRequestInfo{
			status:  r.Status,
			isDraft: r.IsDraft,
		}
	}
	return m
}

// loadTicketDurations loads total run durations per ticket from SQLite.
// Returns a map keyed by "workspace:ticket_filename".
func loadTicketDurations() map[string]time.Duration {
	if database.DB == nil {
		return make(map[string]time.Duration)
	}
	durations, err := database.AllTicketDurations(context.Background())
	if err != nil {
		return make(map[string]time.Duration)
	}
	return durations
}

// tickMsg is sent on each poll interval to trigger queue file sync.
type tickMsg time.Time

// pollInterval is how often the TUI checks the queue file for worker updates.
const pollInterval = 2 * time.Second

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m tuiModel) Init() tea.Cmd {
	return tickCmd()
}

// syncFromQueueFile reads the queue file and updates in-memory state to reflect
// worker progress (status changes, current index, running state).
func (m *tuiModel) syncFromQueueFile() {
	m.syncFromQueueFileAtPath(queueFilePathForID(m.activeQueueID))
}

// syncFromQueueFileAtPath is the testable version with an explicit path.
func (m *tuiModel) syncFromQueueFileAtPath(path string) {
	qf, err := readQueueFileFromPath(path)
	if err != nil {
		return
	}

	// Build maps of ticket path:requestNum -> queue file status, draft state, started_at, unread, and preprocessing
	statusMap := make(map[string]string)
	draftMap := make(map[string]bool)
	startedAtMap := make(map[string]int64)
	unreadMap := make(map[string]bool)
	preprocessStatusMap := make(map[string]string)
	for _, qt := range qf.Tickets {
		key := additionalRequestKey(qt.Path, qt.RequestNum)
		statusMap[key] = qt.Status
		draftMap[key] = qt.IsDraft
		startedAtMap[key] = qt.StartedAt
		unreadMap[key] = qt.Unread
		preprocessStatusMap[key] = qt.PreprocessStatus
	}

	// Update queue items with worker status + refresh frontmatter status when changed
	items := m.queue.Items()
	changed := false
	unreadChanged := false // track if any item transitioned to unread (needs disk persist)
	for i, qi := range items {
		item, ok := qi.(tuiTicketItem)
		if !ok {
			continue
		}
		key := additionalRequestKey(item.filePath, item.requestNum)
		// Sync draft state from queue file
		if newDraft, exists := draftMap[key]; exists && item.isDraft != newDraft {
			item.isDraft = newDraft
			changed = true
		}
		// Sync started_at from queue file (worker sets this)
		if sa, exists := startedAtMap[key]; exists && sa > 0 {
			newStart := time.Unix(sa, 0)
			if !newStart.Equal(item.workStartedAt) {
				item.workStartedAt = newStart
				changed = true
			}
		} else if !item.workStartedAt.IsZero() && statusMap[key] != "working" {
			// Clear workStartedAt when ticket is no longer working
			item.workStartedAt = time.Time{}
			changed = true
		}
		// Sync unread state from queue file
		if fileUnread, exists := unreadMap[key]; exists && item.unread != fileUnread {
			item.unread = fileUnread
			changed = true
		}
		// Sync preprocessing status from queue file (preprocessing worker updates this)
		if newPPS, exists := preprocessStatusMap[key]; exists && item.preprocessStatus != newPPS {
			item.preprocessStatus = newPPS
			changed = true
		}
		if newStatus, exists := statusMap[key]; exists {
			oldWorkerStatus := item.workerStatus
			item.workerStatus = newStatus
			if oldWorkerStatus != newStatus {
				changed = true
				// When worker status changes to completed, mark as unread
				// so the user knows there's a new result to look at.
				if newStatus == "completed" {
					item.unread = true
					unreadChanged = true
				}
				// When worker status changes to completed, re-read frontmatter
				// status from disk so ticket icons update correctly.
				if newStatus == "completed" {
					if content, err := os.ReadFile(item.filePath); err == nil {
						freshStatus := strings.ToLower(strings.TrimSpace(extractFrontmatterStatus(string(content))))
						if idx := strings.Index(freshStatus, ":"); idx >= 0 {
							freshStatus = strings.TrimSpace(freshStatus[idx+1:])
						}
						if freshStatus != "" {
							item.status = freshStatus
						}
					}
				}
			}
			// For additional requests, keep status in sync with queue file state.
			// The request's canonical status lives in SQLite, not frontmatter.
			// Run this even when workerStatus hasn't changed to repair stale DB/list
			// statuses after app restarts.
			if item.requestNum > 0 {
				mappedStatus := mapQueueStatusToAdditionalRequestStatus(newStatus)
				if mappedStatus != "" && item.status != mappedStatus {
					item.status = mappedStatus
					changed = true
					if database.DB != nil {
						_ = database.UpdateAdditionalRequestStatus(context.Background(), item.filePath, item.requestNum, mappedStatus)
					}
				}
			}
			items[i] = item
		}
	}
	// Detect items in TUI queue but missing from queue file (race condition recovery).
	// If the worker wrote the queue file from a stale read, TUI-added items may have
	// been clobbered. Re-persist them so the worker picks them up on its next poll.
	for _, qi := range items {
		item, ok := qi.(tuiTicketItem)
		if !ok {
			continue
		}
		key := additionalRequestKey(item.filePath, item.requestNum)
		if _, exists := statusMap[key]; !exists {
			devLog("TUI sync: item %s (req=%d) missing from queue file, re-persisting", filepath.Base(item.filePath), item.requestNum)
			_ = writeQueueFileToPath(m, path)
			changed = true // ensure cross-sync below runs
			break
		}
	}

	if changed {
		m.queue.SetItems(items)
		// Cross-sync workerStatus and refreshed status to the All Tickets list
		for _, qi := range items {
			if item, ok := qi.(tuiTicketItem); ok {
				m.updateAllListItem(item)
			}
		}
		// Persist unread state changes back to disk so they survive the next sync cycle.
		// Without this, the TUI sets unread=true in memory but the file still has false,
		// and the next sync would clobber the in-memory true back to false.
		if unreadChanged {
			_ = m.writeQueueFile()
		}
	}

	// Sync default preprocess prompt from queue file
	if qf.DefaultPreprocessPrompt != m.defaultPreprocessPrompt {
		m.defaultPreprocessPrompt = qf.DefaultPreprocessPrompt
	}

	// Sync shortcuts from queue file (may have been added by CLI)
	if !shortcutsEqual(qf.Shortcuts, m.queueShortcuts) {
		m.queueShortcuts = qf.Shortcuts
	}

	// Sync running state: if the file says stopped (e.g., user pressed S in another TUI),
	// stop this TUI too. The worker itself never sets Running=false.
	if m.queueRunning && !qf.Running {
		devLog("TUI sync: detected Running=false in file, stopping queue (was idx=%d)", m.currentQueueIdx)
		m.queueRunning = false
		m.currentQueueIdx = -1
		m.workerLost = false
		m.syncQueueCurrentMarker()
	}

	// Derive current index from ticket statuses (no persisted CurrentIndex needed)
	newIdx := m.computeCurrentQueueIdx()
	if newIdx != m.currentQueueIdx {
		m.currentQueueIdx = newIdx
		m.bumpDraftsAboveCurrent()
		m.syncQueueCurrentMarker()
	}

	// Detect stale worker: reset "working" tickets when worker is dead or queue stopped.
	// Must run BEFORE ordering enforcement so stale items become mutable first.
	// Primary check is PID liveness since idle workers don't write heartbeats.
	if m.queueRunning && qf.WorkerPID > 0 {
		if isProcessAlive(qf.WorkerPID) {
			m.workerLost = false
		} else {
			m.workerLost = true
			m.resetStaleWorkingTickets()
		}
	} else if m.queueRunning && qf.WorkerPID == 0 {
		// No worker PID recorded — check heartbeat as fallback for backward compat
		if qf.LastHeartbeat > 0 {
			elapsed := time.Now().Unix() - qf.LastHeartbeat
			if elapsed > 30 {
				m.workerLost = true
				m.resetStaleWorkingTickets()
			} else {
				m.workerLost = false
			}
		}
	} else if !m.queueRunning {
		m.workerLost = false
		// Queue stopped — no ticket should be "working". Clean up stale statuses.
		m.resetStaleWorkingTickets()
	}

	// Enforce ordering invariant: mutable items before immutable items.
	// Runs after stale "working" cleanup so cleared items are correctly classified.
	items = m.queue.Items() // re-read in case changed above
	if !validateQueueOrdering(items) {
		newItems := enforceQueueOrdering(items)
		m.queue.SetItems(newItems)
		m.currentQueueIdx = m.computeCurrentQueueIdx()
		m.syncQueueCurrentMarker()
		_ = m.writeQueueFile()
		devLog("TUI syncFromQueueFile: enforced ordering invariant")
	}
}

// isProcessAlive checks if a process with the given PID is still running.
// Uses Signal(0) which is the standard POSIX liveness check.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// resetStaleWorkingTickets resets any "working" tickets back to pending
// when the worker process is confirmed dead. This makes them mutable again
// so queue ordering enforcement can handle them.
func (m *tuiModel) resetStaleWorkingTickets() {
	items := m.queue.Items()
	changed := false
	for i, qi := range items {
		item, ok := qi.(tuiTicketItem)
		if !ok {
			continue
		}
		if item.workerStatus == "working" {
			item.workerStatus = ""
			item.workStartedAt = time.Time{}
			items[i] = item
			changed = true
		}
	}
	if changed {
		m.queue.SetItems(items)
		m.writeQueueFile()
	}
}

// activeList returns a pointer to the currently active list based on the active tab.
func (m *tuiModel) activeList() *list.Model {
	if m.tab == tuiTabQueue {
		return &m.queue
	}
	return &m.list
}

// syncSelectionToAllList ensures the "All Tickets" list reflects the current selection state.
// Called after queue modifications (remove from queue) to keep selection in sync.
func (m *tuiModel) syncSelectionToAllList() {
	// Build a set of filePath:requestNum that are in the queue
	inQueue := make(map[string]bool)
	for _, qi := range m.queue.Items() {
		if item, ok := qi.(tuiTicketItem); ok {
			inQueue[additionalRequestKey(item.filePath, item.requestNum)] = true
		}
	}
	// Update all items in the main list
	items := m.list.Items()
	for i, li := range items {
		if item, ok := li.(tuiTicketItem); ok {
			item.selected = inQueue[additionalRequestKey(item.filePath, item.requestNum)]
			items[i] = item
		}
	}
	m.list.SetItems(items)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Always handle ticks regardless of mode (for queue file sync)
	if _, ok := msg.(tickMsg); ok {
		m.syncFromQueueFile()
		m.refreshPinnedQueues()
		return m, tickCmd()
	}

	// Handle text input modes first
	if m.mode == tuiModeMinIterInput {
		return m.updateMinIterInput(msg)
	}
	if m.mode == tuiModeNewTicketWorkspace || m.mode == tuiModeNewTicketName {
		return m.updateNewTicketTextInput(msg)
	}
	if m.mode == tuiModeNewTicketInstructions {
		return m.updateNewTicketInstructions(msg)
	}
	if m.mode == tuiModeAdditionalContext {
		return m.updateAdditionalContext(msg)
	}
	if m.mode == tuiModeHelp {
		return m.updateHelp(msg)
	}
	if m.mode == tuiModeRenameQueue {
		return m.updateRenameQueue(msg)
	}
	if m.mode == tuiModeQueuePrompt {
		return m.updateQueuePrompt(msg)
	}
	if m.mode == tuiModeViewShortcuts {
		return m.updateViewShortcuts(msg)
	}
	if m.mode == tuiModeQueuePicker {
		return m.updateQueuePicker(msg)
	}
	if m.mode == tuiModeNewQueueName {
		return m.updateNewQueueName(msg)
	}
	if m.mode == tuiModeConfirmRemove {
		return m.updateConfirmRemove(msg)
	}
	if m.mode == tuiModeConfirmReset {
		return m.updateConfirmReset(msg)
	}
	if m.mode == tuiModeConfirmActivate {
		return m.updateConfirmActivate(msg)
	}
	if m.mode == tuiModeConfirmDeleteQueue {
		return m.updateConfirmDeleteQueue(msg)
	}
	if m.mode == tuiModeComment {
		return m.updateComment(msg)
	}
	if m.mode == tuiModePreprocessPrompt {
		return m.updatePreprocessPrompt(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Don't intercept keys when filtering
		al := m.activeList()
		if al.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			// Switch between All Tickets and Queue tabs
			if m.tab == tuiTabAll {
				m.tab = tuiTabQueue
			} else {
				m.tab = tuiTabAll
			}
			return m, nil
		case "1", "2", "3", "4", "5":
			idx := int(msg.String()[0] - '1')
			if idx < len(m.pinnedQueues) {
				pq := m.pinnedQueues[idx]
				if pq.id != m.activeQueueID {
					m.switchQueue(pq.id)
					m.refreshPinnedQueues()
				}
				m.tab = tuiTabQueue
			}
			return m, nil
		case " ":
			// Toggle ticket selection (visual only — does not modify queue)
			if item, ok := al.SelectedItem().(tuiTicketItem); ok {
				item.selected = !item.selected
				idx := al.Index()
				items := al.Items()
				items[idx] = item
				al.SetItems(items)
				// Cross-sync selection state to the other list
				if m.tab == tuiTabAll {
					m.updateQueueItem(item)
				} else {
					m.updateAllListItem(item)
				}
			}
			return m, nil
		case "v":
			// Toggle SkipVerification on the selected ticket (Queue tab only)
			if m.tab == tuiTabQueue {
				if item, ok := m.queue.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
					if newVal, err := toggleSkipVerification(item.filePath); err == nil {
						item.skipVerification = newVal
						idx := m.queue.Index()
						items := m.queue.Items()
						items[idx] = item
						m.queue.SetItems(items)
						m.updateAllListItem(item)
					}
				}
			}
			return m, nil
		case "p":
			// Set preprocessing instruction (Queue tab only)
			if m.tab == tuiTabQueue {
				if _, ok := m.queue.SelectedItem().(tuiTicketItem); ok {
					m.mode = tuiModePreprocessPrompt
					m.textArea.SetValue(m.defaultPreprocessPrompt)
					m.textArea.SetWidth(60)
					m.textArea.SetHeight(5)
					m.textArea.Focus()
					return m, m.textArea.Cursor.BlinkCmd()
				}
			}
			return m, nil
		case "r":
			// Remove ticket from queue (Queue tab only, with confirmation)
			if m.tab == tuiTabQueue {
				if item, ok := m.queue.SelectedItem().(tuiTicketItem); ok {
					m.pendingRemovePath = item.filePath
					m.pendingRemoveRequestNum = item.requestNum
					m.mode = tuiModeConfirmRemove
				}
			}
			return m, nil
		case "d":
			// Queue tab only: if draft → activate (with confirmation), else → remove (with confirmation)
			if m.tab == tuiTabQueue {
				if item, ok := m.queue.SelectedItem().(tuiTicketItem); ok {
					if item.isDraft {
						m.pendingActivatePath = item.filePath
						m.pendingActivateRequestNum = item.requestNum
						m.mode = tuiModeConfirmActivate
					} else {
						m.pendingRemovePath = item.filePath
						m.pendingRemoveRequestNum = item.requestNum
						m.mode = tuiModeConfirmRemove
					}
				}
			}
			return m, nil
		case "o":
			// Open selected ticket in Obsidian and mark as read
			if item, ok := al.SelectedItem().(tuiTicketItem); ok {
				url := rawObsidianURL(item.workspace, item.filename)
				openURL(url)
				if item.unread {
					item.unread = false
					idx := al.Index()
					items := al.Items()
					items[idx] = item
					al.SetItems(items)
					if m.tab == tuiTabAll {
						m.updateQueueItem(item)
					} else {
						m.updateAllListItem(item)
					}
					m.writeQueueFile()
				}
			}
		case "V":
			// Toggle SkipVerification on the selected ticket
			if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
				if newVal, err := toggleSkipVerification(item.filePath); err == nil {
					item.skipVerification = newVal
					idx := al.Index()
					items := al.Items()
					items[idx] = item
					al.SetItems(items)
					// Also update in the other list if present
					if m.tab == tuiTabAll {
						m.updateQueueItem(item)
					} else {
						m.updateAllListItem(item)
					}
				}
			}
			return m, nil
		case "enter":
			// Add additional context to the selected ticket
			if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
				m.mode = tuiModeAdditionalContext
				m.textArea.SetValue("")
				m.textArea.Focus()
				return m, m.textArea.Cursor.BlinkCmd()
			}
			return m, nil
		case "n":
			// Start new ticket wizard
			m.mode = tuiModeNewTicketWorkspace
			m.newTicket = newTicketState{addToQueue: m.tab == tuiTabQueue}
			// Pre-fill workspace from currently selected ticket
			if item, ok := al.SelectedItem().(tuiTicketItem); ok {
				m.newTicket.workspace = item.workspace
			}
			m.textInput.Placeholder = "workspace name"
			m.textInput.CharLimit = 50
			m.textInput.Width = 40
			m.textInput.SetValue(m.newTicket.workspace)
			m.textInput.Focus()
			return m, m.textInput.Cursor.BlinkCmd()
		case "I":
			// Enter MinIterations input mode
			if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
				m.mode = tuiModeMinIterInput
				m.textInput.SetValue(strconv.Itoa(item.minIterations))
				m.textInput.Focus()
				return m, m.textInput.Cursor.BlinkCmd()
			}
			return m, nil
		case "R":
			// Reset ticket to original state (with confirmation)
			if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
				m.pendingResetPath = item.filePath
				m.mode = tuiModeConfirmReset
			}
			return m, nil
		case "?":
			m.mode = tuiModeHelp
			return m, nil
		case "c":
			// Copy absolute path of focused ticket to clipboard
			al := m.activeList()
			if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
				_ = clipboard.WriteAll(item.filePath)
			}
			return m, nil
		case "C":
			// Add/edit comment on the selected ticket
			al := m.activeList()
			if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
				m.mode = tuiModeComment
				m.textInput.Placeholder = "comment (max 50 chars)"
				m.textInput.CharLimit = 50
				m.textInput.Width = 52
				m.textInput.SetValue(item.comment)
				m.textInput.Focus()
				return m, m.textInput.Cursor.BlinkCmd()
			}
			return m, nil
		case "P":
			// Edit queue prompt
			m.mode = tuiModeQueuePrompt
			m.textArea.SetValue(m.queuePrompt)
			m.textArea.Focus()
			return m, m.textArea.Cursor.BlinkCmd()
		case "Q":
			// Open queue picker — build the list lazily each time
			items := loadQueuePickerItems(m.activeQueueID)
			delegate := list.NewDefaultDelegate()
			qpl := list.New(items, delegate, 0, 0)
			qpl.Title = "Queue Picker"
			qpl.SetShowStatusBar(true)
			qpl.SetFilteringEnabled(true)
			qpl.DisableQuitKeybindings()
			qpl.AdditionalShortHelpKeys = func() []key.Binding {
				return []key.Binding{
					key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pin")),
					key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "unpin")),
					key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
					key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
					key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename")),
				}
			}
			// Size to match the main list (docStyle adds the frame on render)
			qpl.SetSize(m.list.Width(), m.list.Height())
			m.queuePickerList = qpl
			m.mode = tuiModeQueuePicker
			return m, nil
		case "s":
			// Start queue processing
			if !m.queueRunning {
				m.queueRunning = true
				// Reset all workerStatuses so the worker processes tickets fresh
				items := m.queue.Items()
				for i, qi := range items {
					if item, ok := qi.(tuiTicketItem); ok {
						item.workerStatus = ""
						items[i] = item
					}
				}
				m.queue.SetItems(items)
				// Enforce ordering before computing indices (mutable before immutable)
				newItems := enforceQueueOrdering(m.queue.Items())
				m.queue.SetItems(newItems)
				m.currentQueueIdx = m.lastPendingQueueIndex()
				devLog("TUI 's' pressed: starting queue, currentQueueIdx=%d, queueLen=%d", m.currentQueueIdx, len(m.queue.Items()))
				m.bumpDraftsAboveCurrent()
				m.syncQueueCurrentMarker()
				_ = m.writeQueueFile()
				return m, nil
			}
			return m, nil
		case "S":
			// Stop queue processing
			devLog("TUI 'S' pressed: manual stop")
			m.queueRunning = false
			m.currentQueueIdx = -1
			m.syncQueueCurrentMarker()
			_ = m.writeQueueFile()
			return m, nil
		case "alt+up":
			// Move selected ticket up in the active list
			idx := al.Index()
			if idx > 0 {
				items := al.Items()
				// Block reorder if either ticket is immutable (completed/active)
				if m.tab == tuiTabQueue {
					curr, currOk := items[idx].(tuiTicketItem)
					swap, swapOk := items[idx-1].(tuiTicketItem)
					if currOk && swapOk && (isTicketImmutable(curr) || isTicketImmutable(swap)) {
						return m, nil
					}
				}
				items[idx], items[idx-1] = items[idx-1], items[idx]
				al.SetItems(items)
				al.Select(idx - 1)
				if m.tab == tuiTabQueue {
					m.currentQueueIdx = m.computeCurrentQueueIdx()
					m.syncQueueCurrentMarker()
					_ = m.writeQueueFile()
				}
			}
			return m, nil
		case "alt+down":
			// Move selected ticket down in the active list
			idx := al.Index()
			items := al.Items()
			if idx < len(items)-1 {
				// Block reorder if either ticket is immutable (completed/active)
				if m.tab == tuiTabQueue {
					curr, currOk := items[idx].(tuiTicketItem)
					swap, swapOk := items[idx+1].(tuiTicketItem)
					if currOk && swapOk && (isTicketImmutable(curr) || isTicketImmutable(swap)) {
						return m, nil
					}
				}
				items[idx], items[idx+1] = items[idx+1], items[idx]
				al.SetItems(items)
				al.Select(idx + 1)
				if m.tab == tuiTabQueue {
					m.currentQueueIdx = m.computeCurrentQueueIdx()
					m.syncQueueCurrentMarker()
					_ = m.writeQueueFile()
				}
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.queue.SetSize(msg.Width-h, msg.Height-v)
		m.queuePickerList.SetSize(msg.Width-h, msg.Height-v)
	}

	// Update the active list
	var cmd tea.Cmd
	if m.tab == tuiTabQueue {
		m.queue, cmd = m.queue.Update(msg)
	} else {
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

// lastPendingQueueIndex returns the index of the last queue ticket that is
// not already completed (i.e., the bottom-most pending ticket). Returns
// len(items)-1 if no pending ticket is found (worker will handle advancing).
// The worker processes bottom-to-top, so this is the starting position.
func (m *tuiModel) lastPendingQueueIndex() int {
	items := m.queue.Items()
	for i := len(items) - 1; i >= 0; i-- {
		if item, ok := items[i].(tuiTicketItem); ok {
			// Skip drafts — they're not actionable
			if item.isDraft {
				devLog("TUI lastPendingQueueIndex: skipping [%d] (draft)", i)
				continue
			}
			// Check if the ticket's frontmatter status indicates completion.
			// "not completed" contains "completed" so we must exclude it.
			isCompleted := strings.Contains(item.status, "completed") && !strings.Contains(item.status, "not completed")
			if !isCompleted {
				devLog("TUI lastPendingQueueIndex: returning %d (frontmatter=%q workerStatus=%q path=%s)", i, item.status, item.workerStatus, filepath.Base(item.filePath))
				return i
			}
			devLog("TUI lastPendingQueueIndex: skipping [%d] (frontmatter=%q — completed)", i, item.status)
		}
	}
	devLog("TUI lastPendingQueueIndex: all completed or empty, returning -1 (len=%d)", len(items))
	return -1
}

// computeCurrentQueueIdx derives the current queue index from ticket statuses.
// Returns the index of the "working" ticket, or the last pending ticket if none
// is working (where the worker will start next), or -1 if queue is not running.
func (m *tuiModel) computeCurrentQueueIdx() int {
	if !m.queueRunning {
		return -1
	}
	for i, qi := range m.queue.Items() {
		if item, ok := qi.(tuiTicketItem); ok && item.workerStatus == "working" {
			return i
		}
	}
	return m.lastPendingQueueIndex()
}

// syncQueueCurrentMarker updates the `current` flag on queue items based on currentQueueIdx.
func (m *tuiModel) syncQueueCurrentMarker() {
	items := m.queue.Items()
	for i, qi := range items {
		if item, ok := qi.(tuiTicketItem); ok {
			item.current = m.queueRunning && i == m.currentQueueIdx
			items[i] = item
		}
	}
	m.queue.SetItems(items)
}

// bumpDraftsAboveCurrent moves any draft items that are below the current working
// ticket to just above it. This ensures drafts don't block queue processing.
// The queue processes bottom-to-top, so "below" means higher index.
// Drafts between currentQueueIdx+1 and end get moved to just before currentQueueIdx.
func (m *tuiModel) bumpDraftsAboveCurrent() {
	if !m.queueRunning || m.currentQueueIdx < 0 {
		return
	}
	items := m.queue.Items()
	if m.currentQueueIdx >= len(items) {
		return
	}

	// Collect drafts below the current item (higher indices)
	var drafts []list.Item
	var rest []list.Item
	for i := m.currentQueueIdx + 1; i < len(items); i++ {
		if item, ok := items[i].(tuiTicketItem); ok && item.isDraft {
			drafts = append(drafts, items[i])
		} else {
			rest = append(rest, items[i])
		}
	}

	if len(drafts) == 0 {
		return
	}

	// Rebuild: items before current + drafts + current item + non-draft items after current
	newItems := make([]list.Item, 0, len(items))
	newItems = append(newItems, items[:m.currentQueueIdx]...)
	newItems = append(newItems, drafts...)
	newItems = append(newItems, items[m.currentQueueIdx]) // current item
	newItems = append(newItems, rest...)

	m.queue.SetItems(newItems)
	m.currentQueueIdx = m.computeCurrentQueueIdx()
	m.syncQueueCurrentMarker()
}

// isTicketFinished returns true if the ticket has a completed/verified frontmatter status
// or a terminal workerStatus (completed). Finished tickets should not be reordered.
func isTicketFinished(item tuiTicketItem) bool {
	// Check frontmatter status: "completed" or "completed + verified" but not "not completed"
	frontmatterDone := strings.Contains(item.status, "completed") && !strings.Contains(item.status, "not completed")
	// Check worker status
	workerDone := item.workerStatus == "completed"
	return frontmatterDone || workerDone
}

// isTicketImmutable returns true if the ticket should not be manually reordered (alt+up/down).
// This includes finished tickets (completed) AND actively worked-on tickets
// (workerStatus == "working"). Both completed and active items are immutable in the queue.
func isTicketImmutable(item tuiTicketItem) bool {
	if isTicketFinished(item) {
		return true
	}
	// Active item: currently being processed by the worker
	return item.workerStatus == "working"
}

// isTicketVisuallyCompleted returns true if the ticket appears completed to the user
// based on frontmatter status. Used for ordering enforcement — matches what the user
// sees (◆ vs ○), not the queue file's workerStatus.
func isTicketVisuallyCompleted(item tuiTicketItem) bool {
	return strings.Contains(item.status, "completed") && !strings.Contains(item.status, "not completed")
}

// validateQueueOrdering checks that no immutable item (completed/active) appears at
// a lower index than any mutable item (uncompleted/draft) in the queue. Returns true
// if the ordering is valid. This is a stateless check based solely on item properties.
func validateQueueOrdering(items []list.Item) bool {
	// Find the minimum index of any immutable item and the maximum index of any
	// mutable (non-immutable) item. If minImmutable < maxMutable, the ordering is invalid.
	minImmutable := -1
	maxMutable := -1
	for i, qi := range items {
		item, ok := qi.(tuiTicketItem)
		if !ok {
			continue
		}
		if isTicketImmutable(item) {
			if minImmutable == -1 || i < minImmutable {
				minImmutable = i
			}
		} else {
			if i > maxMutable {
				maxMutable = i
			}
		}
	}
	// Valid if: no immutable items, no mutable items, or all immutable items
	// come after all mutable items (minImmutable >= maxMutable)
	if minImmutable == -1 || maxMutable == -1 {
		return true
	}
	return minImmutable > maxMutable
}

// enforceQueueOrdering rearranges queue items so that all mutable items (uncompleted/draft)
// come before immutable items (completed/active). Preserves relative order within each group.
func enforceQueueOrdering(items []list.Item) []list.Item {
	if validateQueueOrdering(items) {
		return items
	}

	var mutable []list.Item
	var immutable []list.Item
	for _, qi := range items {
		item, ok := qi.(tuiTicketItem)
		if !ok {
			mutable = append(mutable, qi)
			continue
		}
		if isTicketImmutable(item) {
			immutable = append(immutable, qi)
		} else {
			mutable = append(mutable, qi)
		}
	}

	result := make([]list.Item, 0, len(items))
	result = append(result, mutable...)
	result = append(result, immutable...)

	return result
}

// removeFromQueue removes a ticket from the queue by filePath and requestNum.
func (m *tuiModel) removeFromQueue(filePath string, requestNum int) {
	items := m.queue.Items()
	for i, qi := range items {
		if item, ok := qi.(tuiTicketItem); ok && item.filePath == filePath && item.requestNum == requestNum {
			m.queue.RemoveItem(i)
			m.currentQueueIdx = m.computeCurrentQueueIdx()
			m.syncQueueCurrentMarker()
			return
		}
	}
}

// addSelectedToQueue moves all selected items from the All Tickets list into the queue.
// Items already in the queue are skipped. Selection flags are preserved.
func (m *tuiModel) addSelectedToQueue() {
	// Build a set of filePath:requestNum already in the queue
	existing := make(map[string]bool)
	for _, qi := range m.queue.Items() {
		if item, ok := qi.(tuiTicketItem); ok {
			existing[additionalRequestKey(item.filePath, item.requestNum)] = true
		}
	}
	// Add all selected items not already in queue (insert at top, preserving order)
	insertIdx := 0
	added := 0
	for _, li := range m.list.Items() {
		item, ok := li.(tuiTicketItem)
		if !ok || !item.selected || existing[additionalRequestKey(item.filePath, item.requestNum)] {
			continue
		}
		m.queue.InsertItem(insertIdx, item)
		insertIdx++
		added++
	}
	// Recompute current index from statuses (handles both adjustments and idle→active)
	if added > 0 {
		m.currentQueueIdx = m.computeCurrentQueueIdx()
		m.syncQueueCurrentMarker()
	}
}

// updateQueueItem updates a ticket in the queue list to match the given item.
func (m *tuiModel) updateQueueItem(updated tuiTicketItem) {
	items := m.queue.Items()
	for i, qi := range items {
		if item, ok := qi.(tuiTicketItem); ok && item.filePath == updated.filePath && item.requestNum == updated.requestNum {
			items[i] = updated
			m.queue.SetItems(items)
			return
		}
	}
}

// updateAllListItem updates a ticket in the all tickets list to match the given item.
func (m *tuiModel) updateAllListItem(updated tuiTicketItem) {
	items := m.list.Items()
	for i, li := range items {
		if item, ok := li.(tuiTicketItem); ok && item.filePath == updated.filePath && item.requestNum == updated.requestNum {
			items[i] = updated
			m.list.SetItems(items)
			return
		}
	}
}

// updateMinIterInput handles input when in MinIterations input mode.
func (m tuiModel) updateMinIterInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			val, err := strconv.Atoi(m.textInput.Value())
			if err != nil || val < 0 {
				// Invalid input — just return to list mode
				m.mode = tuiModeList
				m.textInput.Blur()
				return m, nil
			}
			al := m.activeList()
			if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
				if err := setMinIterations(item.filePath, val); err == nil {
					item.minIterations = val
					idx := al.Index()
					items := al.Items()
					items[idx] = item
					al.SetItems(items)
					// Cross-sync to the other list
					if m.tab == tuiTabAll {
						m.updateQueueItem(item)
					} else {
						m.updateAllListItem(item)
					}
				}
			}
			m.mode = tuiModeList
			m.textInput.Blur()
			return m, nil
		case "esc":
			m.mode = tuiModeList
			m.textInput.Blur()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// updateNewTicketTextInput handles input for wizard steps 1 (workspace) and 2 (name).
func (m tuiModel) updateNewTicketTextInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			if val == "" {
				return m, nil // don't advance on empty input
			}
			if m.mode == tuiModeNewTicketWorkspace {
				m.newTicket.workspace = val
				m.mode = tuiModeNewTicketName
				m.textInput.Placeholder = "ticket name"
				m.textInput.CharLimit = 100
				m.textInput.Width = 60
				m.textInput.SetValue("")
				m.textInput.Focus()
				return m, m.textInput.Cursor.BlinkCmd()
			}
			// mode == tuiModeNewTicketName
			m.newTicket.name = val
			m.mode = tuiModeNewTicketInstructions
			m.textInput.Blur()
			m.textArea.SetValue("")
			m.textArea.Focus()
			return m, m.textArea.Cursor.BlinkCmd()
		case "esc":
			if m.mode == tuiModeNewTicketName {
				// Go back to workspace step
				m.mode = tuiModeNewTicketWorkspace
				m.textInput.Placeholder = "workspace name"
				m.textInput.CharLimit = 50
				m.textInput.Width = 40
				m.textInput.SetValue(m.newTicket.workspace)
				m.textInput.Focus()
				return m, m.textInput.Cursor.BlinkCmd()
			}
			// Cancel wizard from workspace step
			m.mode = tuiModeList
			m.textInput.Blur()
			m.resetTextInput()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// updateNewTicketInstructions handles input for wizard step 3 (instructions textarea).
func (m tuiModel) updateNewTicketInstructions(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+s":
			// Save the new ticket
			instructions := strings.TrimSpace(m.textArea.Value())
			if instructions == "" {
				return m, nil
			}
			newPath, err := createTicketFile(m.baseDir, m.newTicket.workspace, m.newTicket.name, instructions)
			if err == nil {
				// Refresh the list and restore queue selections
				if items, loadErr := loadAllTickets(m.baseDir); loadErr == nil {
					m.list.SetItems(items)
					m.syncSelectionToAllList()

					// Auto-add to queue if created from the queue tab
					if m.newTicket.addToQueue {
						for _, li := range m.list.Items() {
							if item, ok := li.(tuiTicketItem); ok && item.filePath == newPath {
								m.queue.InsertItem(0, item)
								// Adjust currentQueueIdx since we inserted before it
								if m.queueRunning && m.currentQueueIdx >= 0 {
									m.currentQueueIdx++
								}
								_ = m.writeQueueFile()
								break
							}
						}
					}
				}
			}
			m.mode = tuiModeList
			m.textArea.Blur()
			m.resetTextInput()
			return m, nil
		case "esc":
			// Go back to name step
			m.mode = tuiModeNewTicketName
			m.textArea.Blur()
			m.textInput.Placeholder = "ticket name"
			m.textInput.CharLimit = 100
			m.textInput.Width = 60
			m.textInput.SetValue(m.newTicket.name)
			m.textInput.Focus()
			return m, m.textInput.Cursor.BlinkCmd()
		}
	}

	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)
	return m, cmd
}

// updateAdditionalContext handles input for the additional context textarea.
func (m tuiModel) updateAdditionalContext(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+s", "ctrl+d":
			isDraft := msg.String() == "ctrl+d"
			text := strings.TrimSpace(m.textArea.Value())
			if text == "" {
				return m, nil
			}
			al := m.activeList()
			if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
				// Always create a queue item so the user sees visible feedback
				createItem := true

				if err := appendAdditionalContext(item.filePath, text, createItem, isDraft); err == nil {
					if createItem {
						// Reload tickets and add new item to queue
						if items, loadErr := loadAllTickets(m.baseDir); loadErr == nil {
							m.list.SetItems(items)
							m.syncSelectionToAllList()
						}

						// Find the newly created request item and add it to the queue.
						// Use DB max request num since drafts may not be in the file.
						var newReqNum int
						if database.DB != nil {
							if maxNum, dbErr := database.MaxRequestNum(context.Background(), item.filePath); dbErr == nil {
								newReqNum = maxNum
							}
						}
						if newReqNum == 0 {
							content, _ := os.ReadFile(item.filePath)
							newReqNum = countAdditionalRequests(string(content))
						}
						if newReqNum > 0 {
							for _, li := range m.list.Items() {
								if ri, ok := li.(tuiTicketItem); ok && ri.filePath == item.filePath && ri.requestNum == newReqNum {
									newItem := ri
									newItem.isDraft = isDraft
									// Always insert at position 0 (visual top of queue).
									// Bottom-to-top processing means position 0 is
									// processed last, giving user time to reorder.
									m.queue.InsertItem(0, newItem)
									if m.queueRunning && m.currentQueueIdx >= 0 {
										m.currentQueueIdx++
									}
									_ = m.writeQueueFile()
									break
								}
							}
						}
					}
				}
			}
			m.mode = tuiModeList
			m.textArea.Blur()
			return m, nil
		case "esc":
			m.mode = tuiModeList
			m.textArea.Blur()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)
	return m, cmd
}

// updateHelp handles input when in help overlay mode.
func (m tuiModel) updateHelp(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "?", "esc", "q":
			m.mode = tuiModeList
			return m, nil
		}
	}
	return m, nil
}

// updateRenameQueue handles input when renaming a queue (from the queue picker).
func (m tuiModel) updateRenameQueue(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			if val == "" {
				// Don't allow empty queue name — return to picker
				m.mode = tuiModeQueuePicker
				m.textInput.Blur()
				m.resetTextInput()
				return m, nil
			}
			// Update the queue file for the renamed queue
			if m.renameQueueID == m.activeQueueID {
				// Active queue: update in-memory state too
				m.queueName = val
				m.queue.Title = val
				_ = m.writeQueueFile()
			} else {
				// Non-active queue: read, update name, write back
				path := queueFilePathForID(m.renameQueueID)
				if qf, err := readQueueFileFromPath(path); err == nil {
					qf.Name = val
					_ = writeQueueFileDataToPath(qf, path)
				}
			}
			// Refresh the picker list to show updated name
			items := loadQueuePickerItems(m.activeQueueID)
			m.queuePickerList.SetItems(items)
			m.refreshPinnedQueues()
			m.mode = tuiModeQueuePicker
			m.textInput.Blur()
			m.resetTextInput()
			return m, nil
		case "esc":
			m.mode = tuiModeQueuePicker
			m.textInput.Blur()
			m.resetTextInput()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// updateQueuePrompt handles input when editing the queue prompt.
func (m tuiModel) updateQueuePrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+s":
			m.queuePrompt = strings.TrimSpace(m.textArea.Value())
			m.mode = tuiModeList
			m.textArea.Blur()
			_ = m.writeQueueFile()
			return m, nil
		case "esc":
			m.mode = tuiModeList
			m.textArea.Blur()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)
	return m, cmd
}

// updateViewShortcuts handles input when viewing queue shortcuts.
func (m tuiModel) updateViewShortcuts(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "c", "q":
			m.mode = tuiModeList
			return m, nil
		}
	}
	return m, nil
}

// updateComment handles input when entering a comment for a ticket.
func (m tuiModel) updateComment(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			comment := strings.TrimSpace(m.textInput.Value())
			al := m.activeList()
			if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath != "" {
				item.comment = comment
				idx := al.Index()
				items := al.Items()
				items[idx] = item
				al.SetItems(items)
				// Cross-sync to the other list
				if m.tab == tuiTabAll {
					m.updateQueueItem(item)
				} else {
					m.updateAllListItem(item)
				}
				// Write comment to frontmatter
				_ = setComment(item.filePath, comment)
				// Persist to queue file if in queue
				_ = m.writeQueueFile()
			}
			m.mode = tuiModeList
			m.textInput.Blur()
			m.resetTextInput()
			return m, nil
		case "esc":
			m.mode = tuiModeList
			m.textInput.Blur()
			m.resetTextInput()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// updatePreprocessPrompt handles input when entering a preprocessing instruction.
func (m tuiModel) updatePreprocessPrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+s":
			prompt := strings.TrimSpace(m.textArea.Value())
			if prompt == "" {
				m.mode = tuiModeList
				m.textArea.Blur()
				return m, nil
			}
			// Save as default for next use
			m.defaultPreprocessPrompt = prompt
			// Apply to the focused item only
			items := m.queue.Items()
			if focusedItem, ok := m.queue.SelectedItem().(tuiTicketItem); ok {
				focusedItem.preprocessPrompt = prompt
				focusedItem.preprocessStatus = "pending"
				items[m.queue.Index()] = focusedItem
				m.queue.SetItems(items)
				_ = m.writeQueueFile()
			}
			m.mode = tuiModeList
			m.textArea.Blur()
			return m, nil
		case "esc":
			m.mode = tuiModeList
			m.textArea.Blur()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)
	return m, cmd
}

// updateQueuePicker handles input when the queue picker list is showing.
// Uses a full bubbles list with filtering, pinning, and queue management.
func (m tuiModel) updateQueuePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Don't intercept keys when filtering or when filter is applied
		// (let the list handle esc to clear filter before we close the picker)
		if m.queuePickerList.FilterState() == list.Filtering {
			break
		}
		key := msg.String()
		switch key {
		case "esc":
			// If a filter is applied, let the list clear it first
			if m.queuePickerList.FilterState() == list.FilterApplied {
				break
			}
			m.mode = tuiModeList
			return m, nil
		case "q", "Q":
			// q/Q closes the picker (don't let bubbles list quit the app)
			m.mode = tuiModeList
			return m, nil
		case "enter":
			// Select the highlighted queue
			if item, ok := m.queuePickerList.SelectedItem().(tuiQueueItem); ok {
				m.switchQueue(item.id)
				m.mode = tuiModeList
			}
			return m, nil
		case "p":
			// Pin the highlighted queue
			if item, ok := m.queuePickerList.SelectedItem().(tuiQueueItem); ok {
				if item.pinned {
					return m, nil // already pinned
				}
				if countPinnedQueues(m.queuePickerList.Items()) >= maxPinnedQueues {
					cmd := m.queuePickerList.NewStatusMessage("Max 5 pinned — unpin one first (P)")
					return m, cmd
				}
				if err := toggleQueuePin(item.id, true); err == nil {
					// Rebuild the list to reflect the change
					items := loadQueuePickerItems(m.activeQueueID)
					m.queuePickerList.SetItems(items)
					m.refreshPinnedQueues()
				}
			}
			return m, nil
		case "P":
			// Unpin the highlighted queue
			if item, ok := m.queuePickerList.SelectedItem().(tuiQueueItem); ok && item.pinned {
				if err := toggleQueuePin(item.id, false); err == nil {
					items := loadQueuePickerItems(m.activeQueueID)
					m.queuePickerList.SetItems(items)
					m.refreshPinnedQueues()
				}
			}
			return m, nil
		case "n":
			// Switch to text input for new queue name
			m.mode = tuiModeNewQueueName
			m.textInput.Placeholder = "new queue name"
			m.textInput.CharLimit = 40
			m.textInput.Width = 40
			m.textInput.SetValue("")
			m.textInput.Focus()
			return m, m.textInput.Cursor.BlinkCmd()
		case "d":
			// Delete the highlighted queue (with confirmation)
			if item, ok := m.queuePickerList.SelectedItem().(tuiQueueItem); ok {
				if item.active {
					cmd := m.queuePickerList.NewStatusMessage("Cannot delete the active queue")
					return m, cmd
				}
				m.pendingDeleteQueueID = item.id
				m.mode = tuiModeConfirmDeleteQueue
			}
			return m, nil
		case "r":
			// Rename the highlighted queue
			if item, ok := m.queuePickerList.SelectedItem().(tuiQueueItem); ok {
				m.renameQueueID = item.id
				m.mode = tuiModeRenameQueue
				m.textInput.Placeholder = "queue name"
				m.textInput.CharLimit = 50
				m.textInput.Width = 40
				m.textInput.SetValue(item.name)
				m.textInput.Focus()
				return m, m.textInput.Cursor.BlinkCmd()
			}
			return m, nil
		case "alt+up":
			// Move pinned queue up (swap PinOrder with previous pinned item)
			idx := m.queuePickerList.Index()
			items := m.queuePickerList.Items()
			if idx > 0 {
				curr, currOk := items[idx].(tuiQueueItem)
				swap, swapOk := items[idx-1].(tuiQueueItem)
				if currOk && swapOk && curr.pinned && swap.pinned {
					if err := swapPinOrder(curr.id, swap.id); err == nil {
						newItems := loadQueuePickerItems(m.activeQueueID)
						m.queuePickerList.SetItems(newItems)
						m.queuePickerList.Select(idx - 1)
						m.refreshPinnedQueues()
					}
				}
			}
			return m, nil
		case "alt+down":
			// Move pinned queue down (swap PinOrder with next pinned item)
			idx := m.queuePickerList.Index()
			items := m.queuePickerList.Items()
			if idx < len(items)-1 {
				curr, currOk := items[idx].(tuiQueueItem)
				swap, swapOk := items[idx+1].(tuiQueueItem)
				if currOk && swapOk && curr.pinned && swap.pinned {
					if err := swapPinOrder(curr.id, swap.id); err == nil {
						newItems := loadQueuePickerItems(m.activeQueueID)
						m.queuePickerList.SetItems(newItems)
						m.queuePickerList.Select(idx + 1)
						m.refreshPinnedQueues()
					}
				}
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.queuePickerList.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.queuePickerList, cmd = m.queuePickerList.Update(msg)
	return m, cmd
}

// updateNewQueueName handles text input for creating a new queue.
func (m tuiModel) updateNewQueueName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			if val == "" {
				m.mode = tuiModeQueuePicker
				m.textInput.Blur()
				m.resetTextInput()
				return m, nil
			}
			// Sanitize queue ID: lowercase, replace spaces with hyphens, keep alphanumeric + hyphens
			id := sanitizeQueueID(val)
			if id == "" {
				m.mode = tuiModeQueuePicker
				m.textInput.Blur()
				m.resetTextInput()
				return m, nil
			}
			// Create the queue file with a minimal entry
			path := queueFilePathForID(id)
			os.MkdirAll(filepath.Dir(path), 0755)
			qf := QueueFile{Name: val}
			writeQueueFileDataToPath(&qf, path)
			// Switch to the new queue
			m.textInput.Blur()
			m.resetTextInput()
			m.switchQueue(id)
			m.queueName = val
			m.queue.Title = val
			_ = m.writeQueueFile()
			m.mode = tuiModeList
			return m, nil
		case "esc":
			m.mode = tuiModeQueuePicker
			m.textInput.Blur()
			m.resetTextInput()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m tuiModel) updateConfirmRemove(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			m.removeFromQueue(m.pendingRemovePath, m.pendingRemoveRequestNum)
			m.syncSelectionToAllList()
			_ = m.writeQueueFile()
			m.pendingRemovePath = ""
			m.pendingRemoveRequestNum = 0
			m.mode = tuiModeList
			return m, nil
		case "n", "N", "esc":
			m.pendingRemovePath = ""
			m.pendingRemoveRequestNum = 0
			m.mode = tuiModeList
			return m, nil
		}
	}
	return m, nil
}

func (m tuiModel) updateConfirmReset(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			if err := resetTicketToCreated(m.pendingResetPath); err == nil {
				// Update in-memory item status
				al := m.activeList()
				if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath == m.pendingResetPath {
					item.status = "created"
					item.workerStatus = ""
					idx := al.Index()
					items := al.Items()
					items[idx] = item
					al.SetItems(items)
					if m.tab == tuiTabAll {
						m.updateQueueItem(item)
					} else {
						m.updateAllListItem(item)
					}
				}
			}
			m.pendingResetPath = ""
			m.mode = tuiModeList
			return m, nil
		case "n", "N", "esc":
			m.pendingResetPath = ""
			m.mode = tuiModeList
			return m, nil
		}
	}
	return m, nil
}

// updateConfirmActivate handles the confirmation dialog for activating a draft.
func (m tuiModel) updateConfirmActivate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			// Activate the draft: flip isDraft to false, then move to the front
			// of the processing queue so it's processed next.
			var activatedItem tuiTicketItem
			foundIdx := -1
			for i, qi := range m.queue.Items() {
				if item, ok := qi.(tuiTicketItem); ok && item.filePath == m.pendingActivatePath && item.requestNum == m.pendingActivateRequestNum {
					item.isDraft = false
					activatedItem = item
					foundIdx = i
					break
				}
			}
			if foundIdx >= 0 {
				// Remove from current position (adjusts currentQueueIdx)
				m.removeFromQueue(m.pendingActivatePath, m.pendingActivateRequestNum)
				// Always insert at position 0 (visual top of queue)
				m.queue.InsertItem(0, activatedItem)
				if m.queueRunning && m.currentQueueIdx >= 0 {
					m.currentQueueIdx++
				}
				m.updateAllListItem(activatedItem)
			}
			// Update SQLite
			if database.DB != nil {
				_ = database.ActivateAdditionalRequest(context.Background(), m.pendingActivatePath, m.pendingActivateRequestNum)
			}
			_ = m.writeQueueFile()
			m.pendingActivatePath = ""
			m.pendingActivateRequestNum = 0
			m.mode = tuiModeList
			return m, nil
		case "n", "N", "esc":
			m.pendingActivatePath = ""
			m.pendingActivateRequestNum = 0
			m.mode = tuiModeList
			return m, nil
		}
	}
	return m, nil
}

// updateConfirmDeleteQueue handles the confirmation dialog for deleting a queue.
func (m tuiModel) updateConfirmDeleteQueue(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			path := queueFilePathForID(m.pendingDeleteQueueID)
			os.Remove(path)
			items := loadQueuePickerItems(m.activeQueueID)
			m.queuePickerList.SetItems(items)
			m.refreshPinnedQueues()
			m.pendingDeleteQueueID = ""
			m.mode = tuiModeQueuePicker
			return m, nil
		case "n", "N", "esc":
			m.pendingDeleteQueueID = ""
			m.mode = tuiModeQueuePicker
			return m, nil
		}
	}
	return m, nil
}

// resetTicketToCreated resets a ticket to its original template state.
// It preserves the Date and user request sections but clears all agent work.
// A backup is saved to ~/.wiggums/backups/ before resetting.
func resetTicketToCreated(ticketPath string) error {
	content, err := os.ReadFile(ticketPath)
	if err != nil {
		return err
	}

	// Save backup
	if err := backupTicket(ticketPath, content); err != nil {
		return err
	}

	// Extract the user content (everything between the closing frontmatter --- and the agent divider)
	lines := strings.Split(string(content), "\n")

	// Find frontmatter boundaries
	var fmEnd int
	dashCount := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			dashCount++
			if dashCount == 2 {
				fmEnd = i
				break
			}
		}
	}

	// Extract the Date from frontmatter
	var dateVal string
	for _, line := range lines[:fmEnd] {
		if strings.HasPrefix(strings.TrimSpace(line), "Date:") {
			dateVal = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Date:"))
			break
		}
	}

	// Extract user content: everything from after frontmatter to the agent divider
	var userContent []string
	agentDividerIdx := -1
	for i := fmEnd + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" && i+1 < len(lines) && strings.Contains(lines[i+1], "Below to be filled by agent") {
			agentDividerIdx = i
			break
		}
		userContent = append(userContent, lines[i])
	}

	// If no agent divider found, preserve all content after frontmatter as user content
	if agentDividerIdx == -1 {
		userContent = lines[fmEnd+1:]
	}

	// Build reset content
	userSection := strings.Join(userContent, "\n")
	// Trim trailing whitespace but ensure one trailing newline
	userSection = strings.TrimRight(userSection, "\n\r\t ")

	reset := fmt.Sprintf(`---
Date: %s
Status: created
Agent:
MinIterations:
CurIteration: 0
SkipVerification: true
UpdatedAt:
---
%s

---
Below to be filled by agent. Agent should not modify above this line.

## Execution Plan
TODO

## Additional Context
TODO

## Commands Run / Actions Taken
TODO

## Findings / Results
TODO

## Verification Commands / Steps
TODO

## Verification Coverage Percent and Potential Further Verification
TODO`, dateVal, userSection)

	return os.WriteFile(ticketPath, []byte(reset), 0644)
}

// backupTicket saves a copy of the ticket to ~/.wiggums/backups/.
func backupTicket(ticketPath string, content []byte) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	backupDir := filepath.Join(home, ".wiggums", "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	base := filepath.Base(ticketPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	backupName := fmt.Sprintf("%s_%d%s", name, time.Now().Unix(), ext)
	backupPath := filepath.Join(backupDir, backupName)
	return os.WriteFile(backupPath, content, 0644)
}

// sanitizeQueueID converts a display name into a valid queue file ID.
func sanitizeQueueID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteRune('-')
		}
	}
	result := b.String()
	// Remove consecutive hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}

// switchQueue saves the current queue state, then loads a different queue by ID.
func (m *tuiModel) switchQueue(newID string) {
	if newID == m.activeQueueID {
		return
	}

	// Save current queue state
	_ = m.writeQueueFile()

	// Clear current queue state
	m.queue.SetItems([]list.Item{})
	m.queueName = "Work Queue"
	m.queuePrompt = ""
	m.queueShortcuts = nil
	m.queueRunning = false
	m.currentQueueIdx = -1
	m.workerLost = false

	// Deselect all items in the all-tickets list
	allItems := m.list.Items()
	for i, li := range allItems {
		if item, ok := li.(tuiTicketItem); ok {
			item.selected = false
			item.workerStatus = ""
			item.current = false
			allItems[i] = item
		}
	}
	m.list.SetItems(allItems)

	// Switch to new queue
	m.activeQueueID = newID

	// Restore the new queue's state from disk
	m.restoreQueueState()
}

// shortcutsText returns the formatted shortcuts overlay content.
func shortcutsText(shortcuts []string) string {
	if len(shortcuts) == 0 {
		return "Queue Shortcuts\n──────────────────────────────────\nNo shortcuts yet.\n\nUse `wiggums add-shortcut <text>` to add shortcuts.\n──────────────────────────────────\nPress c or Esc to close"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Queue Shortcuts (%d)\n", len(shortcuts)))
	b.WriteString("──────────────────────────────────\n")
	for i, s := range shortcuts {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, s))
	}
	b.WriteString("──────────────────────────────────\n")
	b.WriteString("C to clear all | c or Esc to close")
	return b.String()
}

// helpText returns the formatted help overlay content.
func helpText() string {
	return `Keyboard Shortcuts
──────────────────────────────────
 /          Filter tickets
 Enter      Add additional context
            ctrl+s save | ctrl+d draft
 n          New ticket wizard
 o          Open in Obsidian
 Space      Select/deselect ticket
 r          Remove from queue (Queue tab)
 d          Remove / activate draft (Queue tab)
 v          Toggle skip-verification (Queue tab)
 V          Toggle skip-verification
 p          Set preprocessing instruction (Queue tab)
 R          Reset ticket (with backup)
 I          Set min iterations
 Tab        Switch All/Queue tabs
 1-5        Switch to pinned queue
 P          Edit queue prompt
 Q          Queue picker (list view)
            enter select | p pin
            P unpin | n new | d delete
            r rename | / filter | esc close
            alt+up/down reorder pinned
 c          Copy ticket path to clipboard
 C          Add/edit comment on ticket
 s          Start queue
 S          Stop queue
 alt+up     Move ticket up (not finished)
 alt+down   Move ticket down (not finished)
 ?          Show this help
 q          Quit
──────────────────────────────────
Press ? or Esc to close`
}

// resetTextInput restores the textinput to its default MinIterations configuration.
func (m *tuiModel) resetTextInput() {
	m.textInput.Placeholder = "0"
	m.textInput.CharLimit = 5
	m.textInput.Width = 20
}

// createTicketFile creates a new ticket markdown file in the given workspace.
// Returns the full filepath of the created ticket.
func createTicketFile(baseDir, workspace, name, instructions string) (string, error) {
	ticketsDir := filepath.Join(baseDir, "workspaces", workspace, "tickets")
	if err := os.MkdirAll(ticketsDir, 0755); err != nil {
		return "", err
	}

	epoch := time.Now().Unix()
	// Convert name to filename format: spaces and special chars become underscores
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, name)
	filename := fmt.Sprintf("%d_%s.md", epoch, safeName)
	fp := filepath.Join(ticketsDir, filename)

	now := time.Now().Format("2006-01-02 15:04")
	content := fmt.Sprintf(`---
Date: %s
Status: created
Agent:
MinIterations:
CurIteration: 0
SkipVerification: true
UpdatedAt:
---
## Original User Request
%s

## Additional User Request
To be populated with further user request

---
Below to be filled by agent. Agent should not modify above this line.

## Execution Plan
TODO

## Additional Context
TODO

## Commands Run / Actions Taken
TODO

## Findings / Results
TODO

## Verification Commands / Steps
TODO

## Verification Coverage Percent and Potential Further Verification
TODO`, now, instructions)

	return fp, os.WriteFile(fp, []byte(content), 0644)
}

// queueRemainingCount returns the number of queue items that are not completed or drafts.
func (m tuiModel) queueRemainingCount() int {
	var remaining int
	for _, qi := range m.queue.Items() {
		item, ok := qi.(tuiTicketItem)
		if !ok {
			continue
		}
		if item.isDraft {
			continue
		}
		switch item.workerStatus {
		case "completed":
			// done — don't count
		default:
			remaining++
		}
	}
	return remaining
}

// queueStatusBadges returns a string with completed/pending counts for the queue.
func (m tuiModel) queueStatusBadges() string {
	items := m.queue.Items()
	if len(items) == 0 {
		return ""
	}
	var completed, pending, unread int
	for _, qi := range items {
		item, ok := qi.(tuiTicketItem)
		if !ok {
			continue
		}
		switch item.workerStatus {
		case "completed":
			completed++
		default:
			// "pending", "working", or empty — count as pending
			pending++
		}
		if item.unread {
			unread++
		}
	}
	var parts []string
	if completed > 0 {
		parts = append(parts, fmt.Sprintf("◆%d", completed))
	}
	if pending > 0 {
		parts = append(parts, fmt.Sprintf("○%d", pending))
	}
	if unread > 0 {
		parts = append(parts, fmt.Sprintf("%du", unread))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// tabBar renders the tab bar showing All Tickets and Queue tabs.
// When pinned queues exist, they appear as numbered tabs (1:Name (count)).
func (m tuiModel) tabBar() string {
	allText := " All Tickets "
	allLabel := inactiveTabStyle.Render(allText)
	if m.tab == tuiTabAll {
		allLabel = activeTabStyle.Render(allText)
	}

	var parts []string
	parts = append(parts, allLabel)

	// Metadata indicators apply to the active queue only
	metaSuffix := m.activeQueueMeta()

	hasPinned := len(m.pinnedQueues) > 0

	if hasPinned {
		activePinned := m.isActiveQueuePinned()
		for i, pq := range m.pinnedQueues {
			count := pq.ticketCount
			// For the active pinned queue, use live remaining count from the model
			isActive := pq.id == m.activeQueueID
			if isActive {
				count = m.queueRemainingCount()
			}
			text := fmt.Sprintf(" %d:%s (%d) ", i+1, pq.name, count)
			if m.tab == tuiTabQueue && pq.id == m.activeQueueID {
				parts = append(parts, activeTabStyle.Render(text))
			} else {
				parts = append(parts, inactiveTabStyle.Render(text))
			}
		}
		// If the active queue is NOT pinned but we're on the queue tab, show it as trailing unnumbered tab
		if !activePinned && m.tab == tuiTabQueue {
			name := m.queueName
			if name == "" {
				name = "Work Queue"
			}
			queueCount := m.queueRemainingCount()
			parts = append(parts, activeTabStyle.Render(fmt.Sprintf(" %s (%d) ", name, queueCount)))
		}
	} else {
		// No pinned queues — fall back to current single-queue behavior
		queueCount := m.queueRemainingCount()
		name := m.queueName
		if name == "" {
			name = "Work Queue"
		}
		text := fmt.Sprintf(" %s (%d) ", name, queueCount)
		if m.tab == tuiTabQueue {
			parts = append(parts, activeTabStyle.Render(text))
		} else {
			parts = append(parts, inactiveTabStyle.Render(text))
		}
	}

	hint := "(tab to switch)"
	if hasPinned {
		hint = "(tab/1-5)"
	}

	return strings.Join(parts, " | ") + metaSuffix + "    " + hint + "\n"
}

// activeQueueMeta returns the metadata suffix (prompt, shortcuts, tokens, badges, position, run indicators)
// for the currently active queue.
func (m tuiModel) activeQueueMeta() string {
	var meta string
	queueIDIndicator := ""
	if m.activeQueueID != "default" {
		queueIDIndicator = fmt.Sprintf(" [queue:%s]", m.activeQueueID)
	}
	meta += queueIDIndicator
	if m.queuePrompt != "" {
		meta += " [prompt]"
	}
	if len(m.queueShortcuts) > 0 {
		meta += fmt.Sprintf(" [%d shortcuts]", len(m.queueShortcuts))
	}
	if tokens := m.queueContextTokens(); tokens > 0 {
		if tokens >= 1000 {
			meta += fmt.Sprintf(" ~%.1fk tokens", float64(tokens)/1000)
		} else {
			meta += fmt.Sprintf(" ~%d tokens", tokens)
		}
	}
	meta += m.queueStatusBadges()
	if m.queueRunning && m.currentQueueIdx >= 0 && len(m.queue.Items()) > 0 {
		// Bottom-to-top processing: position is how many we've reached from the bottom
		processed := len(m.queue.Items()) - m.currentQueueIdx
		meta += fmt.Sprintf(" [%d/%d]", processed, len(m.queue.Items()))
	}
	if m.queueRunning && m.workerLost {
		meta += " ⚠ Worker Lost"
	} else if m.queueRunning && m.currentQueueIdx < 0 {
		meta += " ▶ Waiting"
	} else if m.queueRunning {
		elapsed := m.currentTicketElapsed()
		if elapsed != "" {
			meta += fmt.Sprintf(" ▶ Running %s", elapsed)
		} else {
			meta += " ▶ Running"
		}
	} else if len(m.queue.Items()) > 0 {
		meta += " ⏹ Stopped"
	}
	return meta
}

// currentTicketElapsed returns the elapsed time for the currently-working ticket
// formatted as "mm:ss". Returns empty string if no ticket is currently working
// or if the start time is not set.
func (m tuiModel) currentTicketElapsed() string {
	if m.currentQueueIdx < 0 || m.currentQueueIdx >= len(m.queue.Items()) {
		return ""
	}
	item, ok := m.queue.Items()[m.currentQueueIdx].(tuiTicketItem)
	if !ok || item.workStartedAt.IsZero() {
		return ""
	}
	elapsed := time.Since(item.workStartedAt)
	totalSeconds := int(elapsed.Seconds())
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func (m tuiModel) View() string {
	tabBarStr := m.tabBar()
	activeView := m.list.View()
	if m.tab == tuiTabQueue {
		activeView = m.queue.View()
	}

	switch m.mode {
	case tuiModeMinIterInput:
		return docStyle.Render(tabBarStr + activeView + "\n\nMinIterations: " + m.textInput.View() + "  (enter to save, esc to cancel)")
	case tuiModeNewTicketWorkspace:
		return docStyle.Render(tabBarStr + activeView + "\n\n── New Ticket (1/3) ──\nWorkspace: " + m.textInput.View() + "  (enter to continue, esc to cancel)")
	case tuiModeNewTicketName:
		return docStyle.Render(tabBarStr + activeView + "\n\n── New Ticket (2/3) ──\nWorkspace: " + m.newTicket.workspace + "\nName: " + m.textInput.View() + "  (enter to continue, esc to go back)")
	case tuiModeNewTicketInstructions:
		return docStyle.Render(tabBarStr + activeView + "\n\n── New Ticket (3/3) ──\nWorkspace: " + m.newTicket.workspace + "\nName: " + m.newTicket.name + "\nInstructions:\n" + m.textArea.View() + "\n(ctrl+s to save, esc to go back)")
	case tuiModeAdditionalContext:
		ticketName := ""
		al := m.list
		if m.tab == tuiTabQueue {
			al = m.queue
		}
		if item, ok := al.SelectedItem().(tuiTicketItem); ok {
			ticketName = item.title
		}
		return docStyle.Render(tabBarStr + activeView + "\n\n── Additional Context ──\nTicket: " + ticketName + "\n" + m.textArea.View() + "\n(ctrl+s to save, ctrl+d as draft, esc to cancel)")
	case tuiModeHelp:
		return docStyle.Render(tabBarStr + activeView + "\n\n" + helpText())
	case tuiModeViewShortcuts:
		return docStyle.Render(tabBarStr + activeView + "\n\n" + shortcutsText(m.queueShortcuts))
	case tuiModeRenameQueue:
		return docStyle.Render(m.queuePickerList.View() + "\n\nRename Queue: " + m.textInput.View() + "  (enter to save, esc to cancel)")
	case tuiModeQueuePrompt:
		return docStyle.Render(tabBarStr + activeView + "\n\n── Queue Prompt ──\n" + m.textArea.View() + "\n(ctrl+s to save, esc to cancel)")
	case tuiModeQueuePicker:
		return docStyle.Render(m.queuePickerList.View())
	case tuiModeNewQueueName:
		return docStyle.Render(tabBarStr + activeView + "\n\nNew Queue Name: " + m.textInput.View() + "  (enter to create, esc to cancel)")
	case tuiModeConfirmRemove:
		removeTitle := m.pendingRemovePath
		for _, qi := range m.queue.Items() {
			if item, ok := qi.(tuiTicketItem); ok && item.filePath == m.pendingRemovePath && item.requestNum == m.pendingRemoveRequestNum {
				removeTitle = item.Title()
				break
			}
		}
		return docStyle.Render(tabBarStr + activeView + "\n\nRemove \"" + removeTitle + "\" from queue? (y/n)")
	case tuiModeConfirmReset:
		resetTitle := m.pendingResetPath
		al := m.activeList()
		if item, ok := al.SelectedItem().(tuiTicketItem); ok && item.filePath == m.pendingResetPath {
			resetTitle = item.title
		}
		return docStyle.Render(tabBarStr + activeView + "\n\nReset \"" + resetTitle + "\" to original state? Backup will be saved. (y/n)")
	case tuiModeConfirmActivate:
		activateTitle := m.pendingActivatePath
		for _, qi := range m.queue.Items() {
			if item, ok := qi.(tuiTicketItem); ok && item.filePath == m.pendingActivatePath && item.requestNum == m.pendingActivateRequestNum {
				activateTitle = item.Title()
				break
			}
		}
		return docStyle.Render(tabBarStr + activeView + "\n\nActivate draft \"" + activateTitle + "\"? This will make it actionable. (y/n)")
	case tuiModeConfirmDeleteQueue:
		return docStyle.Render(m.queuePickerList.View() + "\n\nDelete queue \"" + m.pendingDeleteQueueID + "\"? (y/n)")
	case tuiModeComment:
		return docStyle.Render(tabBarStr + activeView + "\n\n── Comment ──\n" + m.textInput.View() + "  (enter to save, esc to cancel)")
	case tuiModePreprocessPrompt:
		return docStyle.Render(tabBarStr + activeView + "\n\n── Preprocessing Instruction ──\n" + m.textArea.View() + "\n(ctrl+s to apply, esc to cancel)")
	default:
		return docStyle.Render(tabBarStr + activeView)
	}
}

// openURL opens a URL using the system's default handler.
func openURL(url string) {
	// macOS
	c := exec.Command("open", url)
	_ = c.Start()
}

func init() {
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(addRequestCmd)
	addShortcutCmd.Flags().String("queue", "default", "Queue ID to add shortcut to")
	rootCmd.AddCommand(addShortcutCmd)

	// queue command with subcommands
	queueAddCmd.Flags().String("queue", "default", "Queue ID to add ticket to")
	queueLsCmd.Flags().String("queue", "default", "Queue ID to list")
	queueMarkReadCmd.Flags().String("queue", "default", "Queue ID to mark read")
	queueCmd.AddCommand(queueAddCmd)
	queueCmd.AddCommand(queueLsCmd)
	queueCmd.AddCommand(queueMarkReadCmd)
	rootCmd.AddCommand(queueCmd)
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI for managing wiggums tickets",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir, err := resolveBaseDir()
		if err != nil {
			return fmt.Errorf("could not resolve base directory: %w", err)
		}

		// Initialize database for run duration queries and draft tracking.
		if dbErr := database.Init(); dbErr == nil {
			defer database.Close()
			_ = database.Migrate(context.Background())
		}

		m, err := newTuiModel(baseDir)
		if err != nil {
			return fmt.Errorf("could not load tickets: %w", err)
		}

		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error running TUI: %w", err)
		}
		return nil
	},
}

var addRequestCmd = &cobra.Command{
	Use:   "add-request <ticket-path> <text...>",
	Short: "Add an additional request to a ticket file",
	Long: `Appends an "Additional User Request" section to a ticket file.
Useful for programmatic automation — Claude running in a loop can use this
to add context to a ticket when encountering issues.

The text is inserted before the "---\nBelow to be filled by agent" divider
and the ticket status is set to "additional_user_request".`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketPath := args[0]
		text := strings.Join(args[1:], " ")

		// Resolve relative path
		if !filepath.IsAbs(ticketPath) {
			abs, err := filepath.Abs(ticketPath)
			if err != nil {
				return fmt.Errorf("could not resolve path: %w", err)
			}
			ticketPath = abs
		}

		// Verify the file exists
		if _, err := os.Stat(ticketPath); err != nil {
			return fmt.Errorf("ticket file not found: %w", err)
		}

		// Check if ticket is completed — only completed tickets get new list items
		content, err := os.ReadFile(ticketPath)
		if err != nil {
			return fmt.Errorf("could not read ticket: %w", err)
		}
		ticketStatus := extractFrontmatterStatus(string(content))
		isCompleted := strings.Contains(ticketStatus, "completed") && !strings.Contains(ticketStatus, "not completed")

		if err := appendAdditionalContext(ticketPath, text, isCompleted, false); err != nil {
			return fmt.Errorf("failed to add request: %w", err)
		}

		if isCompleted {
			fmt.Printf("Added additional request to %s (new list item created)\n", filepath.Base(ticketPath))
		} else {
			fmt.Printf("Added additional request to %s (appended to ticket, no new list item)\n", filepath.Base(ticketPath))
		}
		return nil
	},
}

// addShortcutToQueueFile appends a shortcut to the default queue file on disk.
// This is used by both the CLI command and can be called by the worker.
func addShortcutToQueueFile(text string) error {
	return addShortcutToQueueFileAtPath(text, queueFilePath())
}

// addShortcutToQueueFileForID appends a shortcut to a specific queue file.
func addShortcutToQueueFileForID(text string, queueID string) error {
	return addShortcutToQueueFileAtPath(text, queueFilePathForID(queueID))
}

// addShortcutToQueueFileAtPath is the testable version with an explicit path.
func addShortcutToQueueFileAtPath(text string, path string) error {
	qf, err := readQueueFileFromPath(path)
	if err != nil {
		// Queue file doesn't exist yet — create a minimal one
		qf = &QueueFile{Name: "Work Queue"}
	}
	qf.Shortcuts = append(qf.Shortcuts, text)
	return writeQueueFileDataToPath(qf, path)
}

var addShortcutCmd = &cobra.Command{
	Use:   "add-shortcut <text...>",
	Short: "Add a shortcut to the queue's shortcuts memory",
	Long: `Appends a shortcut entry to the queue's shortcuts memory.
Claude running in a worker queue can use this to record learnings that persist
across queue tickets. Shortcuts are injected into the prompt context for all
subsequent queue tickets.

Use --queue <id> to specify which queue (default: "default").`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		text := strings.Join(args, " ")
		queueID, _ := cmd.Flags().GetString("queue")
		if queueID == "" {
			queueID = "default"
		}
		if err := addShortcutToQueueFileForID(text, queueID); err != nil {
			return fmt.Errorf("failed to add shortcut: %w", err)
		}
		fmt.Printf("Added shortcut to queue %q: %s\n", queueID, text)
		return nil
	},
}

// markAllReadInQueueFileForID marks all tickets in a queue as read.
func markAllReadInQueueFileForID(queueID string) (int, error) {
	return markAllReadInQueueFileAtPath(queueFilePathForID(queueID))
}

// markAllReadInQueueFileAtPath is the testable version with an explicit path.
// Returns the number of tickets that were marked as read.
func markAllReadInQueueFileAtPath(path string) (int, error) {
	qf, err := readQueueFileFromPath(path)
	if err != nil {
		return 0, err
	}
	count := 0
	for i := range qf.Tickets {
		if qf.Tickets[i].Unread {
			qf.Tickets[i].Unread = false
			count++
		}
	}
	if count == 0 {
		return 0, nil
	}
	return count, writeQueueFileDataToPath(qf, path)
}

var queueMarkReadCmd = &cobra.Command{
	Use:   "mark-read",
	Short: "Mark all items in a queue as read",
	Long: `Marks all unread tickets in the specified queue as read.

Use --queue <id> to specify which queue (default: "default").

Examples:
  wiggums queue mark-read
  wiggums queue mark-read --queue my-sprint`,
	RunE: func(cmd *cobra.Command, args []string) error {
		queueID, _ := cmd.Flags().GetString("queue")
		if queueID == "" {
			queueID = "default"
		}
		count, err := markAllReadInQueueFileForID(queueID)
		if err != nil {
			return fmt.Errorf("failed to mark read in queue %q: %w", queueID, err)
		}
		if count == 0 {
			fmt.Printf("No unread items in queue %q.\n", queueID)
		} else {
			fmt.Printf("Marked %d item(s) as read in queue %q.\n", count, queueID)
		}
		return nil
	},
}

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage ticket queues",
	Long:  `Commands for managing ticket queues: add tickets, list queue contents.`,
}

var queueAddCmd = &cobra.Command{
	Use:   "add <workspace> <title> [instructions...]",
	Short: "Create a ticket and add it to a queue",
	Long: `Creates a new ticket file in the specified workspace and adds it to a queue.

The ticket is created with the standard template and inserted at the top
of the queue (position 0) for processing.

Examples:
  wiggums queue add wiggums "fix login bug" "The login page returns 500"
  wiggums queue add myproject "add tests"
  wiggums queue add wiggums "refactor auth" --queue my-sprint`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		workspace := args[0]
		title := args[1]
		instructions := ""
		if len(args) > 2 {
			instructions = strings.Join(args[2:], " ")
		}
		if instructions == "" {
			instructions = title
		}

		queueID, _ := cmd.Flags().GetString("queue")
		if queueID == "" {
			queueID = "default"
		}

		baseDir, err := resolveBaseDir()
		if err != nil {
			return fmt.Errorf("could not resolve base directory: %w", err)
		}

		// Verify workspace exists
		wsNames, err := listWorkspaceNames(baseDir)
		if err != nil {
			return fmt.Errorf("could not list workspaces: %w", err)
		}
		found := false
		for _, ws := range wsNames {
			if ws == workspace {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("workspace %q not found (available: %s)", workspace, strings.Join(wsNames, ", "))
		}

		// Create the ticket file
		fp, err := createTicketFile(baseDir, workspace, title, instructions)
		if err != nil {
			return fmt.Errorf("failed to create ticket: %w", err)
		}
		fmt.Printf("Created ticket: %s\n", filepath.Base(fp))

		// Add to queue
		if err := addTicketToQueueFileForID(fp, workspace, queueID); err != nil {
			return fmt.Errorf("ticket created but failed to add to queue: %w", err)
		}
		fmt.Printf("Added to queue %q\n", queueID)
		return nil
	},
}

// addTicketToQueueFileForID adds a ticket to a specific queue file.
func addTicketToQueueFileForID(ticketPath, workspace, queueID string) error {
	return addTicketToQueueFileAtPath(ticketPath, workspace, queueFilePathForID(queueID))
}

// addTicketToQueueFileAtPath is the testable version with an explicit path.
func addTicketToQueueFileAtPath(ticketPath, workspace, path string) error {
	qf, err := readQueueFileFromPath(path)
	if err != nil {
		// Queue file doesn't exist yet — create a minimal one
		qf = &QueueFile{Name: "Work Queue"}
	}

	// Check if ticket is already in queue
	for _, t := range qf.Tickets {
		if t.Path == ticketPath {
			return nil // already in queue
		}
	}

	// Insert at position 0 (top) — queue processes bottom-to-top
	ticket := QueueTicket{
		Path:      ticketPath,
		Workspace: workspace,
		Status:    "pending",
	}
	qf.Tickets = append([]QueueTicket{ticket}, qf.Tickets...)

	return writeQueueFileDataToPath(qf, path)
}

var queueLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List tickets in a queue",
	Long: `Lists all tickets in the specified queue with their status.

Use --queue <id> to specify which queue (default: "default").

Examples:
  wiggums queue ls
  wiggums queue ls --queue my-sprint`,
	RunE: func(cmd *cobra.Command, args []string) error {
		queueID, _ := cmd.Flags().GetString("queue")
		if queueID == "" {
			queueID = "default"
		}

		path := queueFilePathForID(queueID)
		qf, err := readQueueFileFromPath(path)
		if err != nil {
			fmt.Printf("No queue %q found (or empty).\n", queueID)
			return nil
		}

		if len(qf.Tickets) == 0 {
			fmt.Printf("Queue %q is empty.\n", queueID)
			return nil
		}

		// Show queue info
		name := qf.Name
		if name == "" {
			name = queueID
		}
		runState := "stopped"
		if qf.Running {
			runState = "running"
		}
		fmt.Printf("Queue: %s (%s)\n", name, runState)
		if qf.Prompt != "" {
			fmt.Printf("Prompt: %s\n", qf.Prompt)
		}
		fmt.Println()

		// List tickets
		for _, t := range qf.Tickets {
			marker := "  "
			if qf.Running && t.Status == "working" {
				marker = ">> "
			}

			statusIcon := "○"
			switch t.Status {
			case "completed":
				statusIcon = "◆"
			case "working":
				statusIcon = "▶"
			case "pending":
				statusIcon = "○"
			}

			ticketName := filepath.Base(t.Path)
			// Strip epoch prefix and .md suffix for cleaner display
			if idx := strings.IndexByte(ticketName, '_'); idx >= 0 {
				ticketName = strings.TrimSuffix(ticketName[idx+1:], ".md")
			}
			ticketName = strings.ReplaceAll(ticketName, "_", " ")

			fmt.Printf("%s%s %s [%s]\n", marker, statusIcon, ticketName, name)
		}

		return nil
	},
}
