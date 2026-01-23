# Contributing to lazycap

Thank you for your interest in contributing to lazycap! This guide will help you get started with development, understand the codebase, and submit high-quality contributions.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Architecture Overview](#architecture-overview)
- [Creating Plugins](#creating-plugins)
- [Code Style](#code-style)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Commit Convention](#commit-convention)
- [Issue Guidelines](#issue-guidelines)

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment. Be kind, constructive, and professional in all interactions.

## Getting Started

### Prerequisites

- **Go 1.21+** — [Download Go](https://golang.org/dl/)
- **Make** — Optional but recommended
- **A Capacitor project** — For testing functionality
- **Git** — For version control

### Quick Start

```bash
# 1. Fork and clone
git clone https://github.com/YOUR_USERNAME/lazycap.git
cd lazycap

# 2. Install dependencies
go mod download

# 3. Build
make build

# 4. Run (from a Capacitor project directory)
./bin/lazycap
```

## Development Setup

### Building

```bash
# Standard build
make build

# Build with version info
make build VERSION=1.0.0

# Install to $GOPATH/bin
make install
```

### Running During Development

```bash
# Run directly
go run .

# Run with hot reload (requires air)
make dev

# Run tests
make test

# Run linter
make lint
```

### Development with Hot Reload

Install [air](https://github.com/cosmtrek/air) for live reloading:

```bash
go install github.com/cosmtrek/air@latest
make dev
```

## Architecture Overview

```
lazycap/
├── cmd/lazycap/          # CLI entry point and commands
├── internal/
│   ├── cap/              # Capacitor CLI integration
│   ├── debug/            # Debug tools and actions
│   ├── device/           # Device discovery and management
│   ├── plugin/           # Plugin system core
│   │   ├── plugin.go     # Plugin interface and registry
│   │   ├── context.go    # Plugin context interface
│   │   ├── manager.go    # Plugin lifecycle management
│   │   └── context_impl.go # Context implementation
│   ├── plugins/          # Built-in plugins
│   │   ├── mcp/          # MCP Server plugin
│   │   └── firebase/     # Firebase Emulator plugin
│   ├── preflight/        # Environment validation
│   ├── settings/         # User settings management
│   └── ui/               # Bubble Tea TUI
│       ├── model.go      # Main app model
│       ├── process.go    # Process management
│       └── styles.go     # UI styling
├── assets/               # Images and static files
└── main.go               # Application entry
```

### Key Components

| Package | Description |
|---------|-------------|
| `cmd/lazycap` | Cobra CLI commands and app initialization |
| `internal/ui` | Bubble Tea model, views, and keyboard handling |
| `internal/cap` | Capacitor CLI wrapper and project detection |
| `internal/device` | iOS/Android device and emulator discovery |
| `internal/plugin` | Plugin system core with interface and manager |
| `internal/plugins/*` | Built-in plugin implementations |
| `internal/settings` | User preferences and configuration |
| `internal/debug` | Debug tools and cleanup actions |

### Data Flow

```
User Input → Bubble Tea Model → Update Function → Commands
     ↑                                   ↓
     └─────── View Function ←── State Update
                   ↓
              Plugin Events (via EventBus)
```

## Creating Plugins

lazycap features an extensible plugin system. Plugins can interact with all aspects of the application.

### Plugin Interface

```go
type Plugin interface {
    // Metadata
    ID() string          // Unique identifier (e.g., "my-plugin")
    Name() string        // Display name
    Description() string // Short description
    Version() string     // Semantic version
    Author() string      // Author name

    // Lifecycle
    Init(ctx Context) error  // Initialize with context
    Start() error            // Start the plugin
    Stop() error             // Stop the plugin
    IsRunning() bool         // Check if running

    // Configuration
    GetSettings() []Setting                    // Declare settings
    OnSettingChange(key string, value interface{}) // Handle changes
    GetStatusLine() string                     // Status bar text
    GetCommands() []Command                    // Keyboard commands
}
```

### Plugin Context

Plugins receive a `Context` interface providing access to:

```go
type Context interface {
    // Project
    GetProject() *cap.Project

    // Devices
    GetDevices() []device.Device
    GetSelectedDevice() *device.Device
    RefreshDevices() error

    // Actions
    RunOnDevice(deviceID string, liveReload bool) error
    RunWeb() error
    Sync(platform string) error
    Build() error
    OpenIDE(platform string) error

    // Processes
    GetProcesses() []ProcessInfo
    GetProcessLogs(processID string) []string
    KillProcess(processID string) error

    // Settings
    GetSettings() *settings.Settings
    GetSetting(key string) interface{}
    SetSetting(key string, value interface{}) error
    GetPluginSetting(pluginID, key string) interface{}
    SetPluginSetting(pluginID, key string, value interface{}) error

    // Events
    Subscribe(event EventType, handler EventHandler) UnsubscribeFunc
    Emit(event EventType, data interface{})

    // Logging
    Log(pluginID string, message string)
    LogError(pluginID string, err error)
}
```

### Example Plugin

```go
package myplugin

import "github.com/icarus-itcs/lazycap/internal/plugin"

const PluginID = "my-plugin"

type MyPlugin struct {
    ctx     plugin.Context
    running bool
}

func New() *MyPlugin {
    return &MyPlugin{}
}

func Register() error {
    return plugin.Register(New())
}

func (p *MyPlugin) ID() string          { return PluginID }
func (p *MyPlugin) Name() string        { return "My Plugin" }
func (p *MyPlugin) Description() string { return "Does something cool" }
func (p *MyPlugin) Version() string     { return "1.0.0" }
func (p *MyPlugin) Author() string      { return "Your Name" }
func (p *MyPlugin) IsRunning() bool     { return p.running }

func (p *MyPlugin) Init(ctx plugin.Context) error {
    p.ctx = ctx
    return nil
}

func (p *MyPlugin) Start() error {
    p.running = true
    p.ctx.Log(PluginID, "Plugin started")
    return nil
}

func (p *MyPlugin) Stop() error {
    p.running = false
    p.ctx.Log(PluginID, "Plugin stopped")
    return nil
}

func (p *MyPlugin) GetSettings() []plugin.Setting {
    return []plugin.Setting{
        {
            Key:         "autoStart",
            Name:        "Auto Start",
            Description: "Start automatically",
            Type:        "bool",
            Default:     false,
        },
    }
}

func (p *MyPlugin) OnSettingChange(key string, value interface{}) {
    // Handle setting changes
}

func (p *MyPlugin) GetStatusLine() string {
    if p.running {
        return "MyPlugin active"
    }
    return ""
}

func (p *MyPlugin) GetCommands() []plugin.Command {
    return []plugin.Command{
        {
            Key:         "M",
            Name:        "MyAction",
            Description: "Do my action",
            Handler: func() error {
                // Handle the command
                return nil
            },
        },
    }
}
```

### Registering Your Plugin

Add to `internal/plugins/plugins.go`:

```go
import "github.com/icarus-itcs/lazycap/internal/plugins/myplugin"

func RegisterAll() error {
    // ... existing plugins
    if err := myplugin.Register(); err != nil {
        return err
    }
    return nil
}
```

## Code Style

- **Formatting**: Run `go fmt` before committing
- **Linting**: Run `make lint` to check for issues
- **Naming**: Follow Go conventions (camelCase, exported = PascalCase)
- **Comments**: Document exported functions and types
- **Error handling**: Return errors, don't panic

## Testing

```bash
# Run all tests
make test

# Run with coverage
make coverage

# Run specific package tests
go test ./internal/cap/...
```

## Pull Request Process

1. **Fork** the repository

2. **Create a feature branch**:
   ```bash
   git checkout -b feature/my-feature
   ```

3. **Make changes** with clear, atomic commits

4. **Test** your changes thoroughly

5. **Push** and open a PR:
   ```bash
   git push origin feature/my-feature
   ```

6. **Fill out** the PR template

7. **Wait** for review and address feedback

### PR Checklist

- [ ] Code follows project style guidelines
- [ ] Tests added/updated for changes
- [ ] Documentation updated if needed
- [ ] Commit messages follow convention
- [ ] PR description explains the "why"

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Description |
|--------|-------------|
| `feat:` | New feature |
| `fix:` | Bug fix |
| `docs:` | Documentation only |
| `style:` | Formatting, no code change |
| `refactor:` | Code restructuring |
| `test:` | Adding/updating tests |
| `chore:` | Maintenance tasks |
| `perf:` | Performance improvement |

**Examples:**
```
feat: add Firebase Emulator plugin
fix: correct device detection on M1 Macs
docs: update plugin development guide
refactor: extract device discovery into separate package
```

## Issue Guidelines

### Bug Reports

Include:
- lazycap version (`lazycap version`)
- Operating system and version
- Steps to reproduce
- Expected vs actual behavior
- Debug logs if applicable (`/tmp/lazycap-debug.log`)

### Feature Requests

Include:
- Clear description of the feature
- Use case / motivation
- Potential implementation approach (optional)

### Questions

For general questions, open a Discussion instead of an Issue.

---

## Need Help?

- **Issues**: [GitHub Issues](https://github.com/icarus-itcs/lazycap/issues)
- **Discussions**: [GitHub Discussions](https://github.com/icarus-itcs/lazycap/discussions)

Thank you for contributing to lazycap!
