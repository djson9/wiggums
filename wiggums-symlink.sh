#!/bin/zsh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WIGGUMS_SH="$SCRIPT_DIR/wiggums.sh"

if [ -z "$1" ]; then
  echo "Usage: ./wiggums-symlink.sh <destination-path>"
  echo "Example: ./wiggums-symlink.sh ~/bin/wiggums"
  exit 1
fi

DEST="$1"

# Create parent directory if needed
mkdir -p "$(dirname "$DEST")"

# Remove existing symlink/file if present
if [ -e "$DEST" ] || [ -L "$DEST" ]; then
  rm "$DEST"
fi

ln -s "$WIGGUMS_SH" "$DEST"
echo "Created symlink: $DEST -> $WIGGUMS_SH"
