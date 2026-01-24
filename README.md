<p align="center">
  <img src="assets/demo.gif" alt="lazycap - Terminal UI for Capacitor Development" width="800">
</p>

<p align="center">
  <strong>The command center for Capacitor & Ionic mobile development</strong>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> •
  <a href="#features">Features</a> •
  <a href="#keyboard-shortcuts">Shortcuts</a> •
  <a href="#ai-integration">AI Integration</a> •
  <a href="#plugins">Plugins</a> •
  <a href="#configuration">Config</a>
</p>

<p align="center">
  <img src="https://img.shields.io/github/v/release/icarus-itcs/lazycap?style=flat-square&color=00ADD8" alt="Release">
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-blue?style=flat-square" alt="Platform">
  <img src="https://img.shields.io/badge/license-MIT%20%2B%20Commons%20Clause-blue?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/built%20with-Go-00ADD8?style=flat-square" alt="Built with Go">
</p>

---

## What is lazycap?

**lazycap** is a beautiful terminal dashboard for [Capacitor](https://capacitorjs.com/) and [Ionic](https://ionicframework.com/) mobile development. It unifies device management, builds, live reload, debugging, and AI assistance into one elegant interface.

No more juggling terminal windows. No more remembering commands. Just `lazycap`.

---

## Quick Start

### Install

```bash
# Homebrew (recommended)
brew tap icarus-itcs/lazycap https://github.com/icarus-itcs/lazycap
brew install lazycap

# Or with Go
go install github.com/icarus-itcs/lazycap@latest
```

### Run

```bash
cd your-capacitor-project
lazycap
```

That's it. lazycap auto-detects your project, discovers devices, and you're ready to go.

---

## Features

### Device Management

View all your development targets in one place:

| Device Type | Support |
|-------------|---------|
| iOS Simulators | Full support with boot/status tracking |
| iOS Physical Devices | USB-connected devices |
| Android Emulators | Full support with state management |
| Android Physical Devices | USB debugging enabled devices |
| Web Browser | Always available |

**Auto-detection** — lazycap finds devices using `xcrun simctl`, `adb`, and `emulator` commands automatically.

### One-Key Actions

Everything you need is a single keystroke away:

- **`r`** — Run on device (with optional live reload)
- **`b`** — Build web assets
- **`s`** — Sync to native projects
- **`o`** — Open in Xcode or Android Studio
- **`x`** — Kill a running process

### Live Reload

Start live reload sessions with smart defaults:

- Configurable port (default: 8100)
- External host detection for physical devices
- Per-device targeting
- Auto-sync before run (optional)

### Smart Framework Detection

lazycap auto-detects your web framework and finds the right dev command:

| Framework | Detection |
|-----------|-----------|
| Vite | `vite.config.*` |
| Ionic | `ionic.config.json` |
| Webpack | `webpack.config.*` |
| Parcel | `.parcelrc` |
| Next.js | `next.config.*` |
| Nuxt | `nuxt.config.*` |

Falls back to common scripts: `dev`, `serve`, `start`, `ionic:serve`, `dev:web`

### Process Management

Monitor all your running processes with real-time logs:

- **Process tabs** — Switch between Run, Build, Sync, Web Dev
- **Live streaming** — Watch logs as they happen
- **Status indicators** — See running, success, failed, or canceled
- **Log actions** — Copy to clipboard or export to file

### 30+ Debug Actions

Press `d` to access powerful cleanup and diagnostic tools:

**iOS/Xcode**
- Clear Derived Data
- Clear Device Support
- Clean iOS Build
- Reset All Simulators
- Deintegrate & Reinstall Pods

**Android**
- Clean Android Build
- Clear Gradle Cache
- Stop Gradle Daemons
- Restart ADB Server
- Wipe Emulator Data

**Node/Web**
- Clear node_modules
- Clear NPM Cache
- Kill Dev Server Ports
- Clear Vite/Webpack Cache
- Force Rebuild

**Nuclear Options**
- Full Project Clean (all caches)
- Fresh Install (clean + npm install + pod install + cap sync)

### Preflight Checks

Automatic environment validation on startup:

- Node.js, npm, npx, git
- Xcode CLI tools & CocoaPods (macOS)
- Android SDK & ADB
- Capacitor CLI
- Version compatibility checks

### Monorepo Support

Working in a monorepo? lazycap discovers all Capacitor projects up to 4 levels deep and lets you switch between them with a project selector.

### Self-Update

Press `U` when an update is available to upgrade lazycap in-place. No package manager needed.

---

## Keyboard Shortcuts

### Core Actions
| Key | Action |
|-----|--------|
| `r` | Run on selected device |
| `b` | Build the project |
| `s` | Sync Capacitor |
| `o` | Open in native IDE |
| `w` | Start web dev server |

### Navigation
| Key | Action |
|-----|--------|
| `Tab` | Switch between panes |
| `↑` `↓` | Navigate lists |
| `←` `→` | Switch process tabs |
| `Enter` | Select / Confirm |
| `Esc` | Back / Cancel |

### Process Management
| Key | Action |
|-----|--------|
| `x` | Kill selected process |
| `c` | Copy logs to clipboard |
| `e` | Export logs to file |

### Panels
| Key | Action |
|-----|--------|
| `d` | Debug tools |
| `P` | Plugins panel |
| `,` | Settings |
| `p` | Preflight checks |
| `?` | Help |

### System
| Key | Action |
|-----|--------|
| `R` | Refresh device list |
| `U` | Update lazycap |
| `q` | Quit (press twice) |

---

## AI Integration

### MCP Server

lazycap exposes functionality via the [Model Context Protocol](https://modelcontextprotocol.io/), allowing AI assistants to control your development environment.

**Add to Claude Code** (`~/.claude/settings.json`):

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

| Tool | Description |
|------|-------------|
| `list_projects` | List all Capacitor projects |
| `list_devices` | Get available devices/emulators |
| `run_on_device` | Run app with optional live reload |
| `sync` | Sync web assets to native |
| `build` | Build web assets |
| `open_ide` | Open Xcode or Android Studio |
| `get_project` | Get project information |
| `get_all_logs` | Get logs with filtering (type, status, search, errors_only) |
| `get_debug_actions` | List debug/cleanup actions |
| `run_debug_action` | Execute a debug action |

**Log Filtering Options:**

The `get_all_logs` tool supports powerful filtering:
- `type` — Filter by process type: "build", "sync", "run", "web", or plugin name like "firebase"
- `status` — Filter by status: "running", "success", "failed", "canceled"
- `search` — Search for text in logs (case-insensitive)
- `errors_only` — Only return lines containing error indicators
- `tail` — Limit lines per process

**Example prompts for Claude:**

> "Run the app on my iPhone simulator with live reload"

> "Sync the project and open it in Xcode"

> "The build failed, check the logs and fix the error"

> "Show me only the error lines from the last build"

> "What's happening with the firebase emulator?"

> "Clear all caches and do a fresh install"

---

## Plugins

lazycap features an extensible plugin system. Press `P` to manage plugins.

### MCP Server Plugin

Exposes lazycap via Model Context Protocol for AI assistant integration.

| Setting | Description | Default |
|---------|-------------|---------|
| `mode` | `tcp` or `stdio` | `tcp` |
| `port` | TCP port | `9315` |
| `autoStart` | Start on launch | `false` |

### Firebase Emulator Plugin

Integrates [Firebase Emulator Suite](https://firebase.google.com/docs/emulator-suite) for local development.

**Supported Emulators:** Auth, Firestore, Realtime Database, Functions, Hosting, Storage, PubSub, Eventarc

| Setting | Description | Default |
|---------|-------------|---------|
| `autoStart` | Start on launch | `false` |
| `exportOnExit` | Export data on stop | `true` |
| `exportPath` | Export directory | `.firebase-export` |
| `uiEnabled` | Enable Emulator UI | `true` |

### Plugin API

Create custom plugins by implementing the `Plugin` interface:

```go
type Plugin interface {
    ID() string
    Name() string
    Description() string
    Version() string

    Init(ctx Context) error
    Start() error
    Stop() error
    IsRunning() bool

    GetSettings() []Setting
    GetStatusLine() string
    GetCommands() []Command
}
```

Plugins can access devices, processes, logs, settings, debug actions, and the event bus.

---

## Configuration

### Settings File

lazycap supports both per-project and global settings:

| Location | File | Priority |
|----------|------|----------|
| **Per-project** | `.lazycap.json` in current directory | Checked first |
| **Global** | `~/.config/lazycap/settings.json` | Fallback |

When you modify settings, they're saved to the currently loaded config file (shown at the bottom of the settings panel). New settings default to per-project config in the current directory.

Press `,` to open the settings panel.

### Run Options

| Setting | Description | Default |
|---------|-------------|---------|
| `liveReloadDefault` | Enable live reload by default | `false` |
| `liveReloadPort` | Live reload port | `8100` |
| `externalHost` | External IP (auto-detect if empty) | — |
| `autoSync` | Sync before running | `false` |
| `autoBuild` | Build before syncing | `false` |
| `clearLogsOnRun` | Clear logs on new run | `true` |

### Build Options

| Setting | Description | Default |
|---------|-------------|---------|
| `buildCommand` | Custom build command | auto-detect |
| `productionBuild` | Use production mode | `false` |
| `sourceMaps` | Generate source maps | `true` |
| `buildTimeout` | Timeout in seconds | `300` |

### iOS Options

| Setting | Description | Default |
|---------|-------------|---------|
| `iosScheme` | Xcode scheme | — |
| `iosConfiguration` | Debug or Release | `Debug` |
| `iosAutoSigning` | Automatic signing | `true` |
| `iosTeamId` | Development team ID | — |

### Android Options

| Setting | Description | Default |
|---------|-------------|---------|
| `androidFlavor` | Build flavor | — |
| `androidBuildType` | debug or release | `debug` |
| `androidSdkPath` | Custom SDK path | — |

### Web Dev Options

| Setting | Description | Default |
|---------|-------------|---------|
| `webDevCommand` | Dev server command | auto-detect |
| `webDevPort` | Dev server port | — |
| `webOpenBrowser` | Auto-open browser | `false` |
| `webHttps` | Use HTTPS | `false` |

### UI Options

| Setting | Description | Default |
|---------|-------------|---------|
| `compactMode` | Compact UI layout | `false` |
| `showTimestamps` | Timestamps in logs | `false` |
| `showSpinners` | Animated spinners | `true` |
| `colorTheme` | dark, light, system | `dark` |
| `maxLogLines` | Max lines per log | `5000` |

---

## CLI Commands

```bash
lazycap              # Launch the TUI dashboard
lazycap version      # Show version, commit, build date
lazycap devices      # List devices in table format
lazycap mcp          # Run as MCP server
lazycap --demo       # Demo mode with mock data
lazycap --verbose    # Verbose output
lazycap --config     # Custom config file path
```

---

## Platform Support

| Platform | Architecture | Status |
|----------|--------------|--------|
| macOS | Apple Silicon (arm64) | Tested |
| macOS | Intel (amd64) | Supported |
| Linux | 64-bit (amd64) | Supported |
| Linux | ARM64 | Supported |
| Linux | 32-bit / ARMv7 | Supported |
| Windows | 64-bit / ARM64 | Untested |
| FreeBSD | 64-bit | Supported |

---

## Requirements

**Core:**
- Node.js 18+ with npm/yarn/pnpm
- Capacitor project (capacitor.config.ts/js/json)

**iOS Development (macOS only):**
- Xcode with command line tools
- CocoaPods
- iOS Simulator or physical device

**Android Development:**
- Android Studio with SDK
- ADB
- Emulator or physical device (USB debugging enabled)

---

## Troubleshooting

### Debug Log

Check `/tmp/lazycap-debug.log` for detailed information.

### Common Issues

**Devices not showing?**
- Run preflight checks (`p`) to verify tools are installed
- For iOS: Ensure Xcode CLI is installed (`xcode-select --install`)
- For Android: Ensure ADB is running (`adb devices`)

**Live reload not connecting?**
- Check `externalHost` setting for physical devices
- Verify firewall allows the live reload port

**Build failing?**
- Use debug tools (`d`) to clean caches
- Try "Fresh Install" for a complete reset

---

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

**Ways to help:**
- Report bugs and suggest features
- Submit pull requests
- Improve documentation
- Create and share plugins
- Test on different platforms

### Development

```bash
git clone https://github.com/icarus-itcs/lazycap.git
cd lazycap
go mod download
make build
./bin/lazycap
```

---

## License

**MIT + Commons Clause** — Free to use, modify, and share. You just can't sell it.

- Use lazycap for any project (personal or commercial)
- Modify and fork the code
- Contribute improvements back
- Share with others

The Commons Clause prevents reselling lazycap or offering it as a paid service.

See [LICENSE](LICENSE) for details. Copyright (c) 2025 Icarus, Inc.

---

## Acknowledgments

- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) — The fun, functional framework for terminal UIs
- Inspired by [lazygit](https://github.com/jesseduffield/lazygit) and [lazydocker](https://github.com/jesseduffield/lazydocker)
- Designed for the [Capacitor](https://capacitorjs.com/) community

---

<p align="center">
  Made with care by <a href="https://icarus.inc">Icarus, Inc.</a>
</p>
