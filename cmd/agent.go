package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	agentCmd.AddCommand(agentDefineCmd)
	agentCmd.AddCommand(agentListCmd)
	rootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agents",
}

var agentDefineCmd = &cobra.Command{
	Use:   "define [name]",
	Short: "Define a new agent type",
	Long:  "Creates an agent template in templates/agents/ for use with 'wiggums run agent [name]'.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.ToLower(strings.TrimSpace(args[0]))
		if name == "" {
			return fmt.Errorf("agent name cannot be empty")
		}

		baseDir, err := resolveBaseDir()
		if err != nil {
			return fmt.Errorf("could not resolve base directory: %w", err)
		}

		agentsDir := filepath.Join(baseDir, "templates", "agents")
		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			return fmt.Errorf("could not create agents directory: %w", err)
		}

		agentFile := filepath.Join(agentsDir, name+".md")
		if _, err := os.Stat(agentFile); err == nil {
			return fmt.Errorf("agent '%s' already exists at %s", name, agentFile)
		}

		content := fmt.Sprintf("---\nAgent: %s\n---\n", name)
		if err := os.WriteFile(agentFile, []byte(content), 0644); err != nil {
			return fmt.Errorf("could not write agent template: %w", err)
		}

		fmt.Printf("Created agent template: %s\n", agentFile)
		fmt.Printf("Tickets with 'Agent: %s' in frontmatter will be picked up by 'wiggums run agent %s'\n", name, name)
		return nil
	},
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List defined agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir, err := resolveBaseDir()
		if err != nil {
			return fmt.Errorf("could not resolve base directory: %w", err)
		}

		agentsDir := filepath.Join(baseDir, "templates", "agents")
		entries, err := os.ReadDir(agentsDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No agents defined. Use 'wiggums agent define [name]' to create one.")
				return nil
			}
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No agents defined. Use 'wiggums agent define [name]' to create one.")
			return nil
		}

		fmt.Println("Defined agents:")
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") {
				name := strings.TrimSuffix(e.Name(), ".md")
				fmt.Printf("  - %s\n", name)
			}
		}
		return nil
	},
}
