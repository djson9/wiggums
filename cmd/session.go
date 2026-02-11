package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const tmuxSessionName = "wiggums"

// SessionManager wraps tmux operations for the wiggums session.
type SessionManager struct {
	SessionName   string
	CommandPrefix string // optional prefix for the wiggums command (e.g. "with-anthropic-api-key")
}

func NewSessionManager() *SessionManager {
	return &SessionManager{SessionName: tmuxSessionName}
}

// EnsureSession creates the tmux session if it doesn't exist (idempotent).
func (s *SessionManager) EnsureSession() error {
	if s.SessionExists() {
		return nil
	}
	cmd := exec.Command("tmux", "new-session", "-d", "-s", s.SessionName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create tmux session %q: %s", s.SessionName, strings.TrimSpace(string(out)))
	}
	return nil
}

// SessionExists checks if the tmux session exists.
func (s *SessionManager) SessionExists() bool {
	err := exec.Command("tmux", "has-session", "-t", s.SessionName).Run()
	return err == nil
}

// StartWorkspace creates a tmux window for the workspace and runs wiggums in it.
func (s *SessionManager) StartWorkspace(name string) error {
	if s.IsRunning(name) {
		return nil
	}
	// Create a new window named after the workspace
	cmd := exec.Command("tmux", "new-window", "-t", s.SessionName, "-n", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create window %q: %s", name, strings.TrimSpace(string(out)))
	}
	// Send the wiggums command to the window
	wiggumsBin, err := currentBinaryPath()
	if err != nil {
		wiggumsBin = "wiggums"
	}
	var wiggumsCmdStr string
	if s.CommandPrefix != "" {
		wiggumsCmdStr = fmt.Sprintf("%s %s %s", s.CommandPrefix, wiggumsBin, name)
	} else {
		wiggumsCmdStr = fmt.Sprintf("%s %s", wiggumsBin, name)
	}
	sendCmd := exec.Command("tmux", "send-keys", "-t", fmt.Sprintf("%s:%s", s.SessionName, name), wiggumsCmdStr, "Enter")
	if out, err := sendCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start wiggums in window %q: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

// IsRunning checks if a workspace window exists in the tmux session.
func (s *SessionManager) IsRunning(name string) bool {
	windows, err := s.ListWindows()
	if err != nil {
		return false
	}
	for _, w := range windows {
		if w == name {
			return true
		}
	}
	return false
}

// WindowInfo holds the index and name of a tmux window.
type WindowInfo struct {
	Index      int
	Name       string
	Dead       bool   // true if the pane's process has exited
	PaneCmd    string // current command running in the pane (e.g., "wiggums", "zsh")
}

// ListWindowsDetailed returns index, name, and pane-dead status for each window in the tmux session.
func (s *SessionManager) ListWindowsDetailed() ([]WindowInfo, error) {
	if !s.SessionExists() {
		return nil, nil
	}
	out, err := exec.Command("tmux", "list-windows", "-t", s.SessionName, "-F", "#{window_index}:#{window_name}:#{pane_dead}:#{pane_current_command}").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list windows: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	var windows []WindowInfo
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(line, ":", 4)
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		dead := len(parts) >= 3 && parts[2] == "1"
		paneCmd := ""
		if len(parts) >= 4 {
			paneCmd = parts[3]
		}
		windows = append(windows, WindowInfo{Index: idx, Name: parts[1], Dead: dead, PaneCmd: paneCmd})
	}
	return windows, nil
}

// AttachToWindow creates a grouped tmux session and selects the given window,
// then attaches or switches to it. Using a grouped session avoids changing the
// main session's current window, which would affect all other attached clients.
func (s *SessionManager) AttachToWindow(index int) error {
	if !s.SessionExists() {
		return fmt.Errorf("no tmux session %q running", s.SessionName)
	}

	// Clean up stale grouped sessions from previous attach calls
	s.cleanupGroupedSessions()

	// Create a grouped session linked to the main wiggums session.
	// Grouped sessions share the same windows but have independent
	// current-window pointers, so selecting a window here won't
	// affect other clients attached to the main session.
	groupName := fmt.Sprintf("%s-attach-%d", s.SessionName, os.Getpid())
	if out, err := exec.Command("tmux", "new-session", "-d", "-t", s.SessionName, "-s", groupName).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create grouped session: %s", strings.TrimSpace(string(out)))
	}

	// Select the target window within the grouped session
	target := fmt.Sprintf("%s:%d", groupName, index)
	if err := exec.Command("tmux", "select-window", "-t", target).Run(); err != nil {
		exec.Command("tmux", "kill-session", "-t", groupName).Run()
		return fmt.Errorf("failed to select window %d: %w", index, err)
	}

	// If inside tmux, try switch-client first (switches current client's view)
	if os.Getenv("TMUX") != "" {
		if err := exec.Command("tmux", "switch-client", "-t", groupName).Run(); err == nil {
			// Auto-destroy the grouped session when the client switches away
			exec.Command("tmux", "set-option", "-t", groupName, "destroy-unattached", "on").Run()
			return nil
		}
		// switch-client failed (no current client, etc.) — fall through to attach
		// with TMUX stripped from env so tmux doesn't refuse nesting
	}

	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		exec.Command("tmux", "kill-session", "-t", groupName).Run()
		return fmt.Errorf("tmux not found: %w", err)
	}
	// Strip TMUX from env to allow attach even when called from inside tmux
	env := filterEnv(os.Environ(), "TMUX")
	return syscall.Exec(tmuxPath, []string{"tmux", "attach", "-t", groupName}, env)
}

// cleanupGroupedSessions removes stale wiggums-attach-* sessions with no clients.
func (s *SessionManager) cleanupGroupedSessions() {
	prefix := s.SessionName + "-attach-"
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_attached}").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name, attached := parts[0], parts[1]
		if strings.HasPrefix(name, prefix) && attached == "0" {
			exec.Command("tmux", "kill-session", "-t", name).Run()
		}
	}
}

// filterEnv returns a copy of env with the named variable removed.
func filterEnv(env []string, name string) []string {
	prefix := name + "="
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// CapturePaneContent returns the visible text content of a tmux pane.
// Uses tmux capture-pane -p to print content to stdout without attaching.
func (s *SessionManager) CapturePaneContent(windowIndex int) (string, error) {
	if !s.SessionExists() {
		return "", fmt.Errorf("no tmux session %q running", s.SessionName)
	}
	target := fmt.Sprintf("%s:%d", s.SessionName, windowIndex)
	out, err := exec.Command("tmux", "capture-pane", "-t", target, "-p").Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane %d: %w", windowIndex, err)
	}
	return string(out), nil
}

// ListWindows returns the names of all windows in the tmux session.
func (s *SessionManager) ListWindows() ([]string, error) {
	if !s.SessionExists() {
		return nil, nil
	}
	out, err := exec.Command("tmux", "list-windows", "-t", s.SessionName, "-F", "#{window_name}").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list windows: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

// currentBinaryPath returns the path of the currently running binary.
func currentBinaryPath() (string, error) {
	path, err := exec.LookPath("wiggums")
	if err != nil {
		return "", err
	}
	return path, nil
}
