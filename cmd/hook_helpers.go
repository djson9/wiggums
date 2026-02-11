package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var dryRun bool

func init() {
	rootCmd.AddCommand(hookHelpersCmd)
	hookHelpersCmd.AddCommand(shouldStopCmd)
	shouldStopCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print decision without killing")
}

var hookHelpersCmd = &cobra.Command{
	Use:   "hook-helpers",
	Short: "Helper commands for Claude Code hooks",
}

// hookInput is the JSON that Claude Code sends to hooks on stdin.
type hookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
}

// transcriptEntry is a single line from the JSONL transcript.
type transcriptEntry struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

// transcriptMessage is the message field within a transcript entry.
type transcriptMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

var shouldStopCmd = &cobra.Command{
	Use:   "should-stop",
	Short: "Decide whether to kill the Claude process on Stop event",
	Long: `Reads Claude Code hook JSON from stdin, parses the session transcript,
and kills the parent process if the session should terminate.

Logic:
  - 1 human message (normal wiggums loop): kill
  - >1 human messages, last is /wiggums-continue: kill (user is done)
  - >1 human messages, last is something else: don't kill (user is chatting)

Tool results appear as "type":"user" in transcripts but have array content.
Real human messages have string content. This command filters accordingly.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Read hook input from stdin
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}

		var hi hookInput
		if err := json.Unmarshal(input, &hi); err != nil {
			return fmt.Errorf("failed to parse hook input: %w", err)
		}

		if hi.TranscriptPath == "" {
			return fmt.Errorf("no transcript_path in hook input")
		}

		// Parse transcript for real human messages
		humanMessages, err := parseHumanMessages(hi.TranscriptPath)
		if err != nil {
			return fmt.Errorf("failed to parse transcript: %w", err)
		}

		shouldKill := false
		reason := ""

		if len(humanMessages) <= 1 {
			shouldKill = true
			reason = fmt.Sprintf("single-turn, %d human messages", len(humanMessages))
		} else {
			lastMsg := humanMessages[len(humanMessages)-1]
			if strings.Contains(lastMsg, "wiggums-continue") {
				shouldKill = true
				reason = fmt.Sprintf("wiggums-continue, %d human messages", len(humanMessages))
			} else {
				reason = fmt.Sprintf("user chatting, %d human messages, last=%q", len(humanMessages), lastMsg[:min(len(lastMsg), 60)])
			}
		}

		if dryRun {
			if shouldKill {
				fmt.Printf("KILL (%s)\n", reason)
			} else {
				fmt.Printf("DON'T KILL (%s)\n", reason)
			}
			return nil
		}

		if shouldKill {
			// Exit 10 to signal the calling hook script to kill
			os.Exit(10)
		}
		return nil
	},
}

// parseHumanMessages reads a JSONL transcript file and returns the content
// of real human messages (those with string content, not tool result arrays).
func parseHumanMessages(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []string
	scanner := bufio.NewScanner(f)
	// Increase buffer size for long lines (transcript entries can be large)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry transcriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type != "user" {
			continue
		}

		var msg transcriptMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}

		// Real human messages have string content.
		// Tool results have array content (e.g. [{"type":"tool_result",...}]).
		// We check if content starts with '"' (JSON string) vs '[' (JSON array).
		trimmed := strings.TrimSpace(string(msg.Content))
		if len(trimmed) == 0 || trimmed[0] != '"' {
			continue
		}

		var content string
		if err := json.Unmarshal(msg.Content, &content); err != nil {
			continue
		}

		messages = append(messages, content)
	}

	return messages, scanner.Err()
}

