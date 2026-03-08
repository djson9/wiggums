package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var yolo bool
var agentFlag string
var ticketFlag string

func init() {
	rootCmd.PersistentFlags().BoolVar(&yolo, "yolo", true, "Use opus model and skip permissions")
	rootCmd.PersistentFlags().StringVar(&agentFlag, "agent", "", "Only process tickets matching this agent name")
	rootCmd.PersistentFlags().StringVar(&ticketFlag, "ticket", "", "Only process this specific ticket (substring match on filename)")
	rootCmd.AddCommand(lsCmd)
}

var rootCmd = &cobra.Command{
	Use:   "wiggums",
	Short: "Ticket automation loop powered by Claude Code",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func completeWorkspaces(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	baseDir, err := resolveBaseDir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	wsRoot := filepath.Join(baseDir, "workspaces")
	entries, err := os.ReadDir(wsRoot)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
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
	return names, cobra.ShellCompDirectiveNoFileComp
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List available workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir, err := resolveBaseDir()
		if err != nil {
			return fmt.Errorf("could not resolve base directory: %w", err)
		}

		wsRoot := filepath.Join(baseDir, "workspaces")
		entries, err := os.ReadDir(wsRoot)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No workspaces found.")
				return nil
			}
			return err
		}

		found := false
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			indexPath := filepath.Join(wsRoot, e.Name(), "index.md")
			if _, err := os.Stat(indexPath); err != nil {
				continue
			}
			dir, _ := readWorkspaceDirectory(indexPath)
			fmt.Printf("  %s", e.Name())
			if dir != "" {
				fmt.Printf("  →  %s", dir)
			}
			fmt.Println()
			found = true
		}

		if !found {
			fmt.Println("No workspaces found.")
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
