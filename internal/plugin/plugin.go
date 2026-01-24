package plugin

import (
	"fmt"
	"sync"
)

// Plugin is the interface all plugins must implement
type Plugin interface {
	// Metadata
	ID() string
	Name() string
	Description() string
	Version() string
	Author() string

	// Lifecycle
	Init(ctx Context) error
	Start() error
	Stop() error
	IsRunning() bool

	// Settings
	GetSettings() []Setting
	OnSettingChange(key string, value interface{})

	// UI Integration (optional)
	GetStatusLine() string  // Short status for the UI header
	GetCommands() []Command // Additional keyboard commands

	// Resource tracking (optional)
	GetProcessIDs() []int // PIDs of external processes spawned by this plugin
}

// Setting represents a plugin configuration option
type Setting struct {
	Key         string
	Name        string
	Description string
	Type        string // "bool", "string", "int", "choice"
	Default     interface{}
	Choices     []string // For "choice" type
}

// Command represents a keyboard command added by a plugin
type Command struct {
	Key         string
	Name        string
	Description string
	Handler     func() error
}

// PluginInfo contains metadata about a plugin
type PluginInfo struct {
	ID          string
	Name        string
	Description string
	Version     string
	Author      string
	Enabled     bool
	Running     bool
}

// Registry manages plugin registration and discovery
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	order   []string // Maintains registration order
}

// Global registry instance
var globalRegistry = &Registry{
	plugins: make(map[string]Plugin),
	order:   make([]string, 0),
}

// Register adds a plugin to the registry
func Register(p Plugin) error {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	id := p.ID()
	if _, exists := globalRegistry.plugins[id]; exists {
		return fmt.Errorf("plugin %s already registered", id)
	}

	globalRegistry.plugins[id] = p
	globalRegistry.order = append(globalRegistry.order, id)
	return nil
}

// Get returns a plugin by ID
func Get(id string) (Plugin, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	p, ok := globalRegistry.plugins[id]
	return p, ok
}

// All returns all registered plugins in registration order
func All() []Plugin {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	plugins := make([]Plugin, 0, len(globalRegistry.order))
	for _, id := range globalRegistry.order {
		if p, ok := globalRegistry.plugins[id]; ok {
			plugins = append(plugins, p)
		}
	}
	return plugins
}

// List returns info about all registered plugins
func List() []PluginInfo {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(globalRegistry.order))
	for _, id := range globalRegistry.order {
		if p, ok := globalRegistry.plugins[id]; ok {
			infos = append(infos, PluginInfo{
				ID:          p.ID(),
				Name:        p.Name(),
				Description: p.Description(),
				Version:     p.Version(),
				Author:      p.Author(),
				Running:     p.IsRunning(),
			})
		}
	}
	return infos
}
