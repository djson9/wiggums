#!/bin/zsh

export TERM=xterm

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR" || exit 1

# Parse workspace argument
WORKSPACE=""
CLAUDE_ARGS=()

while [[ $# -gt 0 ]]; do
  case $1 in
    -w|--workspace)
      WORKSPACE="$2"
      shift 2
      ;;
    *)
      CLAUDE_ARGS+=("$1")
      shift
      ;;
  esac
done

# Determine workspace directory
if [ -n "$WORKSPACE" ]; then
  WORKSPACE_DIR="$SCRIPT_DIR/workspaces/$WORKSPACE"
  if [ ! -d "$WORKSPACE_DIR" ]; then
    echo "Error: Workspace '$WORKSPACE' not found at $WORKSPACE_DIR"
    echo "Available workspaces:"
    ls -1 "$SCRIPT_DIR/workspaces" 2>/dev/null || echo "  (none)"
    exit 1
  fi
  TICKETS_DIR="$WORKSPACE_DIR/tickets"
else
  # Default to wiggums root (legacy behavior)
  WORKSPACE_DIR="$SCRIPT_DIR"
  TICKETS_DIR="$SCRIPT_DIR/tickets"
fi

echo "Using workspace: $WORKSPACE_DIR"

trap 'exit 130' INT

while :; do
  if [ ! -d "$TICKETS_DIR" ]; then
    echo "Error: No tickets directory found in workspace"
    exit 1
  fi

  # Verify recently completed tickets
  recently_completed=$(find "$TICKETS_DIR" -name "*.md" -not -name "CLAUDE.md" -mmin -60 -exec grep -li "status: completed" {} + 2>/dev/null | xargs grep -L "completed + verified" 2>/dev/null)

  if [ -n "$recently_completed" ]; then
    echo "Running verify.md"
    sed -e "s|{{WIGGUMS_DIR}}|$SCRIPT_DIR|g" -e "s|{{WORKSPACE_DIR}}|$WORKSPACE_DIR|g" "./verify.md" | claude "${CLAUDE_ARGS[@]}"
    [ $? -eq 0 ] && exit 0
    continue
  fi

  remaining=$(grep -riL "status: completed" --include="*.md" --exclude="CLAUDE.md" "$TICKETS_DIR")

  if [ -z "$remaining" ]; then
    echo "✅ All tasks completed, checking again in 10s..."
    sleep 5
    continue
  fi

  echo "⏳ Remaining:"
  echo "$remaining" | xargs -n1 basename

  echo "Running prompt.md"
  sed -e "s|{{WIGGUMS_DIR}}|$SCRIPT_DIR|g" -e "s|{{WORKSPACE_DIR}}|$WORKSPACE_DIR|g" "./prompt.md" | claude "${CLAUDE_ARGS[@]}"
  [ $? -eq 0 ] && exit 0
done
