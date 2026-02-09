package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(workspaceCmd)
}

var workspaceCmd = &cobra.Command{
	Use:     "workspace [name] [-- claude args...]",
	Aliases: []string{"w"},
	Short:   "Run the ticket loop for a workspace",
	Long:    "Runs the ticket processing loop scoped to a workspace's tickets directory,\nwith claude working in the workspace's configured external directory.",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWorkspace(args[0], args[1:])
	},
}

// runWorkspace starts the ticket loop for a named workspace.
func runWorkspace(name string, claudeArgs []string) error {
	baseDir, err := resolveBaseDir()
	if err != nil {
		return fmt.Errorf("could not resolve base directory: %w", err)
	}

	wsDir := filepath.Join(baseDir, "workspaces", name)
	indexPath := filepath.Join(wsDir, "index.md")

	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return fmt.Errorf("workspace %q not found (no %s)", name, indexPath)
	}

	workDir, err := readWorkspaceDirectory(indexPath)
	if err != nil {
		return err
	}
	if workDir == "" {
		return fmt.Errorf("workspace %q has no Directory set in %s", name, indexPath)
	}

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return fmt.Errorf("workspace directory %q does not exist", workDir)
	}

	ticketsDir := filepath.Join(wsDir, "tickets")
	os.MkdirAll(ticketsDir, 0755)

	fmt.Printf("Workspace: %s\n", name)
	fmt.Printf("Working directory: %s\n", workDir)

	return startLoop(claudeArgs, "", ticketsDir, workDir)
}

// readWorkspaceDirectory reads the Directory field from a workspace index.md frontmatter.
func readWorkspaceDirectory(indexPath string) (string, error) {
	f, err := os.Open(indexPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
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
			if strings.HasPrefix(lower, "directory:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1]), nil
				}
			}
		}
	}
	return "", nil
}
