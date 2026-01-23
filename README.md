<p align="center">
  <img src="assets/hero.png" alt="lazycap - Terminal UI for Capacitor Development" width="800">
</p>

<p align="center">
  <strong>A slick terminal UI for Capacitor & Ionic mobile development</strong>
</p>

<p align="center">
  <a href="#installation">Installation</a> •
  <a href="#features">Features</a> •
  <a href="#usage">Usage</a> •
  <a href="#plugins">Plugins</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#contributing">Contributing</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-blue" alt="Platform">
  <img src="https://img.shields.io/badge/license-Icarus%20Source%20Available-green" alt="License">
  <img src="https://img.shields.io/badge/built%20with-Go-00ADD8" alt="Built with Go">
  <img src="https://img.shields.io/badge/made%20by-Icarus%2C%20Inc.-purple" alt="Made by Icarus, Inc.">
</p>

---

## What is lazycap?

**lazycap** is a beautiful terminal-based dashboard for [Capacitor](https://capacitorjs.com/) and [Ionic](https://ionicframework.com/) mobile development. It brings together device management, builds, live reload, and debugging into one unified interface — no more juggling multiple terminal windows.

Think of it as the control center for your mobile development workflow.

## Features

- **Device Management** — View all connected iOS simulators, Android emulators, and physical devices in one place
- **One-Key Actions** — Run, build, sync, and open IDE with single keystrokes
- **Live Reload** — Start live reload sessions with automatic device targeting
- **Web Development** — Integrated web dev server with smart framework detection
- **Process Management** — Monitor running processes with real-time log streaming
- **Debug Tools** — Built-in cleanup and diagnostic actions for common issues
- **Plugin System** — Extend functionality with MCP Server, Firebase Emulator, and more
- **Preflight Checks** — Automatic environment validation on startup
- **Beautiful UI** — Capacitor-inspired design with intuitive keyboard navigation

## Installation

### Using Go

```bash
go install github.com/icarus-itcs/lazycap@latest
```

### From Source

```bash
git clone https://github.com/icarus-itcs/lazycap.git
cd lazycap
make install
```

### Using Homebrew

```bash
brew tap icarus-itcs/lazycap https://github.com/icarus-itcs/lazycap
brew install lazycap
```

## Requirements

- **Go 1.21+** (for building from source)
- **Node.js 18+** with npm/yarn/pnpm
- **Capacitor project** (capacitor.config.ts/js/json)

For iOS development:
- macOS with Xcode installed
- iOS Simulator or physical device

For Android development:
- Android Studio with SDK
- Android Emulator or physical device with USB debugging

## Usage

Navigate to your Capacitor project directory and run:

```bash
lazycap
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `r` | Run on selected device |
| `b` | Build the project |
| `s` | Sync Capacitor |
| `o` | Open in native IDE |
| `x` | Kill selected process |
| `R` | Refresh device list |
| `d` | Open debug tools |
| `P` | Open plugins panel |
| `,` | Open settings |
| `p` | Preflight checks |
| `Tab` | Switch between panes |
| `↑/↓` | Navigate lists |
| `←/→` | Switch process tabs |
| `c` | Copy logs to clipboard |
| `e` | Export logs to file |
| `q` | Quit (press twice to confirm) |

### CLI Commands

```bash
# Show version information
lazycap version

# List available devices
lazycap devices

# Run as MCP server for AI assistant integration
lazycap mcp
```

## Plugins

lazycap features a powerful plugin system for extending functionality. Plugins can control all aspects of the application including devices, processes, settings, and debug actions.

### Built-in Plugins

#### MCP Server

Exposes lazycap functionality via the [Model Context Protocol](https://modelcontextprotocol.io/), allowing AI assistants like Claude to control your development environment.

**Claude Code Integration:**

Add lazycap to your Claude Code MCP settings (`~/.claude/settings.json`):

```json
{
  "mcpServers": {
    "lazycap": {
      "command": "lazycap",
      "args": ["mcp"]
    }
  }
}
```

Or run lazycap in your Capacitor project directory with the working directory specified:

```json
{
  "mcpServers": {
    "lazycap": {
      "command": "lazycap",
      "args": ["mcp"],
      "cwd": "/path/to/your/capacitor-project"
    }
  }
}
```

**Available Tools:**
- `list_devices` — Get all available devices and emulators
- `run_on_device` — Run app on a specific device with optional live reload
- `sync` — Sync web assets to native projects
- `build` — Build the web assets
- `open_ide` — Open Xcode or Android Studio
- `get_project` — Get current Capacitor project info
- `get_debug_actions` — List available debug/cleanup actions
- `run_debug_action` — Execute a debug action

**Plugin Configuration (for TUI mode):**
| Setting | Description | Default |
|---------|-------------|---------|
| `mode` | Server mode: `tcp` or `stdio` | `tcp` |
| `port` | TCP port (when mode is tcp) | `9315` |
| `autoStart` | Start server on launch | `false` |

#### Firebase Emulator

Integrates [Firebase Emulator Suite](https://firebase.google.com/docs/emulator-suite) for local development with Firebase services.

**Features:**
- Auto-detects `firebase.json` in your project
- Manages emulator lifecycle (start/stop)
- Supports data import/export
- Shows running emulator status in header

**Configuration:**
| Setting | Description | Default |
|---------|-------------|---------|
| `autoStart` | Start emulators on launch | `false` |
| `importPath` | Path to import data from | — |
| `exportOnExit` | Export data when stopping | `true` |
| `exportPath` | Path to export data to | `.firebase-export` |
| `uiEnabled` | Enable Emulator UI | `true` |

### Managing Plugins

Press `P` to open the plugins panel where you can:
- View all installed plugins and their status
- Start/stop plugins with `Enter`
- Enable/disable plugins with `e`

Plugin settings are persisted in `~/.config/lazycap/plugins.json`.

### Creating Plugins

lazycap plugins implement the `Plugin` interface:

```go
type Plugin interface {
    ID() string
    Name() string
    Description() string
    Version() string
    Author() string

    Init(ctx Context) error
    Start() error
    Stop() error
    IsRunning() bool

    GetSettings() []Setting
    OnSettingChange(key string, value interface{})
    GetStatusLine() string
    GetCommands() []Command
}
```

Plugins have access to the full application context including:
- Device listing and selection
- Process management and logs
- Settings (read/write)
- Debug actions
- Event bus for reactive updates

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed plugin development documentation.

## Configuration

### Settings

Press `,` to open the settings panel. Settings are organized by category:

**General**
- Live reload default behavior
- Verbose output

**Web Development**
- Dev server command (auto-detected or custom)
- Port and host configuration
- Auto-open browser
- HTTPS mode

Settings are stored in `~/.config/lazycap/settings.json`.

### Project Configuration

lazycap automatically detects your Capacitor configuration from:
- `capacitor.config.ts`
- `capacitor.config.js`
- `capacitor.config.json`

## Debug Tools

Press `d` to access built-in debug and cleanup tools:

**Cache & Build**
- Clean npm cache
- Clean Capacitor build artifacts
- Reset watchman
- Clear Metro bundler cache

**iOS**
- Clean Xcode derived data
- Reset iOS simulators
- Clear CocoaPods cache

**Android**
- Clean Gradle cache
- Kill ADB server
- Clear Android build cache

**Project**
- Reinstall node_modules
- Reset Capacitor plugins
- Full project clean

## Reporting Issues

Found a bug or have a feature request? We'd love to hear from you!

1. **Search existing issues** — Your issue might already be reported
2. **Create a new issue** — Use our issue templates for bugs or feature requests
3. **Include details** — OS, lazycap version (`lazycap version`), and steps to reproduce

[Open an Issue](https://github.com/icarus-itcs/lazycap/issues/new/choose)

### Debug Information

When reporting bugs, include output from:

```bash
lazycap version
```

And check the debug log at `/tmp/lazycap-debug.log` for detailed information.

## Contributing

We welcome contributions from the community! lazycap is open source and we appreciate help in making it better.

### Ways to Contribute

- **Report Bugs** — Found something broken? Let us know!
- **Suggest Features** — Have an idea? Open a feature request
- **Submit PRs** — Code contributions are welcome
- **Improve Docs** — Help us make the documentation better
- **Create Plugins** — Build and share plugins with the community

### Development Setup

```bash
# Clone the repository
git clone https://github.com/icarus-itcs/lazycap.git
cd lazycap

# Install dependencies
go mod download

# Build
make build

# Run locally
./bin/lazycap
```

### Code Guidelines

- Follow standard Go conventions and formatting (`go fmt`)
- Write clear commit messages
- Add tests for new functionality
- Update documentation as needed

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

## License

lazycap is developed by **Icarus, Inc.** and released under the **Icarus Source Available License**.

### You CAN:
- Use lazycap for personal and commercial projects
- Modify the source code for your own use
- Contribute improvements back to the project
- Share lazycap with others (free of charge)

### You CANNOT:
- Sell lazycap or derivatives
- Offer lazycap as a paid service
- Remove or modify license/attribution notices
- Create competing commercial products based on this code

See [LICENSE](LICENSE) for the full license text.

## Acknowledgments

- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) — The fun, functional framework for terminal UIs
- Inspired by [lazygit](https://github.com/jesseduffield/lazygit) and [lazydocker](https://github.com/jesseduffield/lazydocker)
- Designed for the [Capacitor](https://capacitorjs.com/) community

---

<p align="center">
  Made with ⚡ by <a href="https://icarus.inc">Icarus, Inc.</a>
</p>
