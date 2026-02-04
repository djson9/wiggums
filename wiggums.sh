#!/bin/zsh

export TERM=xterm

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR" || exit 1

TICKETS_DIR="$SCRIPT_DIR/tickets"

echo "Using tickets directory: $TICKETS_DIR"

trap 'exit 130' INT

while :; do
  if [ ! -d "$TICKETS_DIR" ]; then
    echo "Error: No tickets directory found"
    exit 1
  fi

  # Verify recently completed tickets
  recently_completed=$(find "$TICKETS_DIR" -name "*.md" -not -name "CLAUDE.md" -mmin -60 -exec grep -li "status: completed" {} + 2>/dev/null | xargs grep -L "completed + verified" 2>/dev/null)

  if [ -n "$recently_completed" ]; then
    echo "Running verify.md"
    sed "s|{{WIGGUMS_DIR}}|$SCRIPT_DIR|g" "./verify.md" | claude
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
  sed "s|{{WIGGUMS_DIR}}|$SCRIPT_DIR|g" "./prompt.md" | claude
  [ $? -eq 0 ] && exit 0
done
