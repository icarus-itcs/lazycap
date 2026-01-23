package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Manager handles plugin lifecycle and coordination
type Manager struct {
	mu       sync.RWMutex
	ctx      Context
	enabled  map[string]bool
	running  map[string]bool                   // tracks which plugins should auto-start (were running when app closed)
	settings map[string]map[string]interface{} // pluginID -> key -> value
	events   *EventBus
	started  bool
}

// NewManager creates a new plugin manager
func NewManager() *Manager {
	return &Manager{
		enabled:  make(map[string]bool),
		running:  make(map[string]bool),
		settings: make(map[string]map[string]interface{}),
		events:   NewEventBus(),
	}
}

// SetContext sets the plugin context (called by the app after initialization)
func (m *Manager) SetContext(ctx Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
}

// GetContext returns the plugin context
func (m *Manager) GetContext() Context {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ctx
}

// GetEventBus returns the event bus for publishing/subscribing to events
func (m *Manager) GetEventBus() *EventBus {
	return m.events
}

// LoadConfig loads plugin enabled states and settings from disk
func (m *Manager) LoadConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	configPath, err := m.configPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		// No config yet, use defaults
		m.initDefaults()
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read plugin config: %w", err)
	}

	var config struct {
		Enabled  map[string]bool                   `json:"enabled"`
		Running  map[string]bool                   `json:"running"`
		Settings map[string]map[string]interface{} `json:"settings"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse plugin config: %w", err)
	}

	m.enabled = config.Enabled
	m.running = config.Running
	m.settings = config.Settings

	if m.enabled == nil {
		m.enabled = make(map[string]bool)
	}
	if m.running == nil {
		m.running = make(map[string]bool)
	}
	if m.settings == nil {
		m.settings = make(map[string]map[string]interface{})
	}

	return nil
}

// SaveConfig saves plugin enabled states and settings to disk
func (m *Manager) SaveConfig() error {
	m.mu.RLock()
	config := struct {
		Enabled  map[string]bool                   `json:"enabled"`
		Running  map[string]bool                   `json:"running"`
		Settings map[string]map[string]interface{} `json:"settings"`
	}{
		Enabled:  m.enabled,
		Running:  m.running,
		Settings: m.settings,
	}
	m.mu.RUnlock()

	configPath, err := m.configPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal plugin config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

func (m *Manager) configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lazycap", "plugins.json"), nil
}

func (m *Manager) initDefaults() {
	// Enable built-in plugins by default
	for _, p := range All() {
		m.enabled[p.ID()] = true
	}
}

// IsEnabled checks if a plugin is enabled
func (m *Manager) IsEnabled(pluginID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	enabled, ok := m.enabled[pluginID]
	if !ok {
		return true // Default to enabled for new plugins
	}
	return enabled
}

// SetEnabled enables or disables a plugin
func (m *Manager) SetEnabled(pluginID string, enabled bool) error {
	m.mu.Lock()
	wasEnabled := m.enabled[pluginID]
	m.enabled[pluginID] = enabled
	m.mu.Unlock()

	// Handle state change
	p, ok := Get(pluginID)
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	if enabled && !wasEnabled && m.started {
		// Start the plugin
		if err := p.Init(m.ctx); err != nil {
			return fmt.Errorf("failed to init plugin %s: %w", pluginID, err)
		}
		if err := p.Start(); err != nil {
			return fmt.Errorf("failed to start plugin %s: %w", pluginID, err)
		}
	} else if !enabled && wasEnabled && p.IsRunning() {
		// Stop the plugin
		if err := p.Stop(); err != nil {
			return fmt.Errorf("failed to stop plugin %s: %w", pluginID, err)
		}
	}

	return m.SaveConfig()
}

// GetPluginSetting gets a plugin-specific setting
func (m *Manager) GetPluginSetting(pluginID, key string) interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if settings, ok := m.settings[pluginID]; ok {
		return settings[key]
	}
	return nil
}

// SetPluginSetting sets a plugin-specific setting
func (m *Manager) SetPluginSetting(pluginID, key string, value interface{}) error {
	m.mu.Lock()
	if m.settings[pluginID] == nil {
		m.settings[pluginID] = make(map[string]interface{})
	}
	m.settings[pluginID][key] = value
	m.mu.Unlock()

	// Notify the plugin
	if p, ok := Get(pluginID); ok {
		p.OnSettingChange(key, value)
	}

	return m.SaveConfig()
}

// GetAllPluginSettings returns all settings for a plugin
func (m *Manager) GetAllPluginSettings(pluginID string) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]interface{})
	if settings, ok := m.settings[pluginID]; ok {
		for k, v := range settings {
			result[k] = v
		}
	}
	return result
}

// InitAll initializes all enabled plugins with the given context
func (m *Manager) InitAll(ctx Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()

	// Load config first
	if err := m.LoadConfig(); err != nil {
		// Log but don't fail
		fmt.Fprintf(os.Stderr, "Warning: failed to load plugin config: %v\n", err)
	}

	for _, p := range All() {
		if !m.IsEnabled(p.ID()) {
			continue
		}

		if err := p.Init(ctx); err != nil {
			// Log but don't fail on plugin init errors
			ctx.LogError(p.ID(), fmt.Errorf("init failed: %w", err))
		}
	}

	return nil
}

// StartAll starts all enabled and initialized plugins
func (m *Manager) StartAll() error {
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	for _, p := range All() {
		if !m.IsEnabled(p.ID()) {
			continue
		}

		if err := p.Start(); err != nil {
			// Log but don't fail on plugin start errors
			if m.ctx != nil {
				m.ctx.LogError(p.ID(), fmt.Errorf("start failed: %w", err))
			}
		}
	}

	// Emit app started event
	m.events.Emit(EventAppStarted, nil)

	return nil
}

// StartAutoStart starts plugins that were running when the app was last closed
func (m *Manager) StartAutoStart() {
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	for _, p := range All() {
		if !m.IsEnabled(p.ID()) {
			continue
		}

		// Check if plugin was running when app was last closed
		if m.WasRunning(p.ID()) {
			if err := p.Start(); err != nil {
				if m.ctx != nil {
					m.ctx.LogError(p.ID(), fmt.Errorf("auto-start failed: %w", err))
				}
			}
		}
	}

	// Emit app started event
	m.events.Emit(EventAppStarted, nil)
}

// WasRunning returns true if the plugin was running when the app was last closed
func (m *Manager) WasRunning(pluginID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running[pluginID]
}

// SetRunning records whether a plugin is running (for persistence across app restarts)
func (m *Manager) SetRunning(pluginID string, running bool) error {
	m.mu.Lock()
	m.running[pluginID] = running
	m.mu.Unlock()
	return m.SaveConfig()
}

// StopAll stops all running plugins
func (m *Manager) StopAll() error {
	// Emit app stopping event
	m.events.Emit(EventAppStopping, nil)

	m.mu.Lock()
	m.started = false
	m.mu.Unlock()

	var lastErr error
	for _, p := range All() {
		if p.IsRunning() {
			if err := p.Stop(); err != nil {
				lastErr = err
			}
		}
	}

	return lastErr
}

// GetEnabledPlugins returns all enabled plugins
func (m *Manager) GetEnabledPlugins() []Plugin {
	var enabled []Plugin
	for _, p := range All() {
		if m.IsEnabled(p.ID()) {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetRunningPlugins returns all currently running plugins
func (m *Manager) GetRunningPlugins() []Plugin {
	var running []Plugin
	for _, p := range All() {
		if p.IsRunning() {
			running = append(running, p)
		}
	}
	return running
}

// EventBus handles event pub/sub for plugins
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]EventHandler
	nextID      int
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[EventType][]EventHandler),
	}
}

// Subscribe adds an event handler
func (eb *EventBus) Subscribe(event EventType, handler EventHandler) UnsubscribeFunc {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.subscribers[event] = append(eb.subscribers[event], handler)
	index := len(eb.subscribers[event]) - 1

	return func() {
		eb.mu.Lock()
		defer eb.mu.Unlock()
		// Remove handler by setting to nil (preserves indices)
		if index < len(eb.subscribers[event]) {
			eb.subscribers[event][index] = nil
		}
	}
}

// Emit sends an event to all subscribers
func (eb *EventBus) Emit(event EventType, data interface{}) {
	eb.mu.RLock()
	handlers := eb.subscribers[event]
	eb.mu.RUnlock()

	for _, handler := range handlers {
		if handler != nil {
			// Run handlers in goroutines to prevent blocking
			go handler(data)
		}
	}
}
