#!/usr/bin/env bash
#
# tbible installer
# Installs the tbible script and (optionally) a desktop entry + icon.

set -euo pipefail

VERSION="1.0.0"
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
BIN_DIR="${XDG_BIN_HOME:-$HOME/.local/bin}"
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}"
DESKTOP_DIR="$DATA_DIR/applications"
ICON_DIR="$DATA_DIR/icons/hicolor/256x256/apps"
DB_PATH="${DB_FILE:-$DATA_DIR/terminal-bible/bible.db}"

install_desktop=false
if [ "${1:-}" = "--desktop" ]; then
    install_desktop=true
fi

echo "== tbible $VERSION installer =="

# 1. Main script
mkdir -p "$BIN_DIR"
install -m 0755 "$SCRIPT_DIR/tbible" "$BIN_DIR/tbible"
echo "installed: $BIN_DIR/tbible"

# 2. Database (build if missing, or if built from an older schema)
if [ ! -f "$DB_PATH" ]; then
    echo "building database (kjv -> $DB_PATH) ..."
    python3 "$SCRIPT_DIR/scripts/build_db.py" --db "$DB_PATH"
elif ! sqlite3 "$DB_PATH" "SELECT book_key FROM verses LIMIT 1;" >/dev/null 2>&1; then
    echo "database schema is outdated (missing verses.book_key), rebuilding ..."
    python3 "$SCRIPT_DIR/scripts/build_db.py" --db "$DB_PATH"
else
    echo "database already exists: $DB_PATH (delete to rebuild)"
fi

# 3. Optional desktop entry + icon
if [ "$install_desktop" = true ]; then
    mkdir -p "$DESKTOP_DIR" "$ICON_DIR"
    install -m 0755 "$SCRIPT_DIR/packaging/Bible.desktop" "$DESKTOP_DIR/Bible.desktop"
    install -m 0644 "$SCRIPT_DIR/packaging/tbible.svg" "$ICON_DIR/tbible.svg"
    echo "installed: $DESKTOP_DIR/Bible.desktop"
    echo "installed: $ICON_DIR/tbible.svg"
fi

echo "done. run: tbible"
command -v tbible >/dev/null 2>&1 || echo "note: $BIN_DIR/tbible is not on your PATH"