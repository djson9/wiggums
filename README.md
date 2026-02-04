# Wiggums

Automated ticket runner that uses Claude to work through tickets.

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
    └── shortcuts.md  # workspace-specific shortcuts
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
