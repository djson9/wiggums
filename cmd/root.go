package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var yolo bool

func init() {
	rootCmd.PersistentFlags().BoolVar(&yolo, "yolo", true, "Use opus model and skip permissions")
	rootCmd.AddCommand(lsCmd)
}

var rootCmd = &cobra.Command{
	Use:   "wiggums [workspace]",
	Short: "Ticket automation loop powered by Claude Code",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runWorkspace(args[0], args[1:])
	},
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
