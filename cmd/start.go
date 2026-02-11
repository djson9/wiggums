package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(startAPICmd)
}

// startWorkspaces is the shared handler for start and start-api.
func startWorkspaces(args []string, commandPrefix string) error {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return fmt.Errorf("could not resolve base directory: %w", err)
	}

	workspaces, err := listWorkspaceNames(baseDir)
	if err != nil {
		return err
	}

	// Filter to requested workspaces if args provided
	if len(args) > 0 {
		requested := make(map[string]bool)
		for _, a := range args {
			requested[a] = true
		}
		var filtered []string
		for _, ws := range workspaces {
			if requested[ws] {
				filtered = append(filtered, ws)
			}
		}
		// Warn about unknown workspaces
		for _, a := range args {
			found := false
			for _, ws := range workspaces {
				if ws == a {
					found = true
					break
				}
			}
			if !found {
				fmt.Fprintf(os.Stderr, "Warning: workspace %q not found, skipping\n", a)
			}
		}
		workspaces = filtered
	}

	if len(workspaces) == 0 {
		fmt.Println("No workspaces to start.")
		return nil
	}

	sm := NewSessionManager()
	sm.CommandPrefix = commandPrefix
	if err := sm.EnsureSession(); err != nil {
		return err
	}
	fmt.Printf("tmux session: %s\n", sm.SessionName)
	if commandPrefix != "" {
		fmt.Printf("command prefix: %s\n", commandPrefix)
	}

	for _, ws := range workspaces {
		if sm.IsRunning(ws) {
			fmt.Printf("  %s — already running\n", ws)
			continue
		}
		if err := sm.StartWorkspace(ws); err != nil {
			fmt.Fprintf(os.Stderr, "  %s — error: %v\n", ws, err)
			continue
		}
		fmt.Printf("  %s — started\n", ws)
	}

	fmt.Printf("\nAttach with: tmux attach -t %s\n", sm.SessionName)
	return nil
}

var startCmd = &cobra.Command{
	Use:   "start [workspace...]",
	Short: "Start wiggums in tmux (one session, each workspace as a tab)",
	Long: `Creates a tmux session called "wiggums" and starts each workspace
as a window (tab) within it. If workspace names are provided, only those
are started. Otherwise all workspaces are started.`,
	ValidArgsFunction: completeWorkspaces,
	RunE: func(cmd *cobra.Command, args []string) error {
		return startWorkspaces(args, "")
	},
}

var startAPICmd = &cobra.Command{
	Use:   "start-api [workspace...]",
	Short: "Start wiggums in tmux using the Anthropic API key",
	Long: `Same as "start" but wraps each workspace with "with-anthropic-api-key"
so Claude uses the API key instead of a subscription.`,
	ValidArgsFunction: completeWorkspaces,
	RunE: func(cmd *cobra.Command, args []string) error {
		return startWorkspaces(args, "with-anthropic-api-key")
	},
}

// listWorkspaceNames returns the names of all valid workspaces.
func listWorkspaceNames(baseDir string) ([]string, error) {
	wsRoot := filepath.Join(baseDir, "workspaces")
	entries, err := os.ReadDir(wsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		indexPath := filepath.Join(wsRoot, e.Name(), "index.md")
		if _, err := os.Stat(indexPath); err != nil {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}
