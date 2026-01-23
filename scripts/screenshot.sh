#!/bin/bash
#
# Generate a beautiful screenshot of lazycap for the README
# Usage: ./scripts/screenshot.sh
#
# This script launches lazycap in demo mode with mock data,
# then helps you capture a native macOS screenshot.
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
OUTPUT_PATH="$PROJECT_ROOT/assets/hero.png"

echo "⚡ lazycap Screenshot Generator"
echo ""

# Ensure assets directory exists
mkdir -p "$PROJECT_ROOT/assets"

# Build lazycap first
echo "🔨 Building lazycap..."
cd "$PROJECT_ROOT"
go build -o "$PROJECT_ROOT/bin/lazycap" .

echo ""
echo "📸 Screenshot Instructions:"
echo ""
echo "   1. lazycap will launch in demo mode with mock data"
echo "   2. Resize the terminal window to your desired size"
echo "   3. Press Cmd+Shift+4, then press Space"
echo "   4. Click on the terminal window to capture it"
echo "   5. The screenshot will be saved to your Desktop"
echo "   6. Move it to: $OUTPUT_PATH"
echo ""
echo "   Tip: For best results, use a terminal theme like:"
echo "   - Catppuccin Mocha"
echo "   - Dracula"
echo "   - One Dark"
echo ""
read -p "Press Enter to launch lazycap in demo mode..."

# Run lazycap in demo mode
"$PROJECT_ROOT/bin/lazycap" --demo

echo ""
echo "🎉 Done! Don't forget to move your screenshot to:"
echo "   $OUTPUT_PATH"
echo ""
echo "Then update the README if needed."
