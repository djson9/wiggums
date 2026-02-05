# Ralph Wiggums

Ralph Wiggums is an LLM which runs inside of an endless while loop to accomplish user goals.

Named after Ralph Wiggums, representing the power of persistent optimism in overcoming obstacles.

## Usage

```bash
# Run with a workspace
./wiggums.sh --workspace ableton

# Short flag
./wiggums.sh -w ableton

# Pass additional args to claude
./wiggums.sh -w ableton --model opus

# Legacy (uses root tickets/)
./wiggums.sh
```

## Workspace Structure

```
workspaces/
└── <name>/
    ├── tickets/      # ticket files
    └── prompts/shortcuts.md  # workspace-specific shortcuts
```

## Creating a New Workspace

```bash
mkdir -p workspaces/myworkspace/tickets
touch workspaces/myworkspace/shortcuts.md
```

## Install Symlink

```bash
./wiggums-symlink.sh ~/bin/wiggums
```
