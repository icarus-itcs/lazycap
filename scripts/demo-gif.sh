#!/bin/bash
#
# Generate an animated GIF demo of lazycap for the README
# Usage: ./scripts/demo-gif.sh
#
# Requires VHS: brew install vhs ttyd ffmpeg
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
OUTPUT_PATH="$PROJECT_ROOT/assets/demo.gif"

echo "⚡ lazycap Demo GIF Generator"
echo ""

# Check dependencies
for cmd in vhs ttyd ffmpeg; do
    if ! command -v $cmd &> /dev/null; then
        echo "📦 Installing $cmd..."
        brew install $cmd
    fi
done

# Ensure assets directory exists
mkdir -p "$PROJECT_ROOT/assets"

# Build lazycap first
echo "🔨 Building lazycap..."
cd "$PROJECT_ROOT"
go build -o "$PROJECT_ROOT/bin/lazycap" .

# Create the VHS tape file
TAPE_FILE=$(mktemp /tmp/lazycap-demo.XXXXXX.tape)

cat > "$TAPE_FILE" << EOF
# lazycap Demo GIF
Output "$OUTPUT_PATH"

# Terminal settings
Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set FontFamily "SF Mono"
Set FontSize 14
Set Width 1200
Set Height 800
Set Padding 20
Set WindowBarSize 40
Set TypingSpeed 50ms

# Start recording - run lazycap in demo mode
Type "$PROJECT_ROOT/bin/lazycap --demo"
Enter

# Wait for UI to load
Sleep 3s

# Navigate through devices
Down
Sleep 800ms
Down
Sleep 800ms
Up
Sleep 800ms

# Switch to logs pane
Tab
Sleep 1s

# Switch back
Tab
Sleep 800ms

# Open settings
Type ","
Sleep 2s

# Close settings
Escape
Sleep 1s

# Open debug panel
Type "d"
Sleep 2s

# Close debug
Escape
Sleep 1s

# Open plugins
Type "P"
Sleep 2s

# Close plugins
Escape
Sleep 1s

# Final pause
Sleep 2s

# Quit
Type "q"
Sleep 500ms
Type "q"
EOF

echo "🎬 Generating demo GIF..."
echo "   Output: $OUTPUT_PATH"
echo "   This may take a minute..."
echo ""

# Run VHS
vhs "$TAPE_FILE"

# Cleanup
rm -f "$TAPE_FILE"

# Check if GIF was created
if [[ -f "$OUTPUT_PATH" ]]; then
    echo ""
    echo "✅ Demo GIF saved to: $OUTPUT_PATH"
    echo ""

    SIZE=$(ls -lh "$OUTPUT_PATH" | awk '{print $5}')
    echo "   Size: $SIZE"

    if command -v open &> /dev/null; then
        read -p "Open the GIF? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            open "$OUTPUT_PATH"
        fi
    fi
else
    echo "❌ Demo GIF generation failed"
    exit 1
fi

echo ""
echo "🎉 Done! Add this to your README:"
echo '   ![lazycap demo](assets/demo.gif)'
