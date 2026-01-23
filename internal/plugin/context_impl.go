package plugin

import (
	"fmt"
	"sync"
	"time"

	"github.com/icarus-itcs/lazycap/internal/cap"
	"github.com/icarus-itcs/lazycap/internal/debug"
	"github.com/icarus-itcs/lazycap/internal/device"
	"github.com/icarus-itcs/lazycap/internal/settings"
)

// PluginLogEntry represents a log entry from a plugin
type PluginLogEntry struct {
	PluginID string
	Message  string
	Time     time.Time
}

// AppContext implements Context interface for the main application
// This bridges the plugin system with the UI model
type AppContext struct {
	mu sync.RWMutex

	// Core app components (set by the UI)
	project  *cap.Project
	settings *settings.Settings
	manager  *Manager

	// Callbacks to UI (set by the UI)
	onGetDevices        func() []device.Device
	onGetSelectedDevice func() *device.Device
	onRefreshDevices    func() error
	onRunOnDevice       func(deviceID string, liveReload bool) error
	onRunWeb            func() error
	onSync              func(platform string) error
	onBuild             func() error
	onOpenIDE           func(platform string) error
	onKillProcess       func(processID string) error
	onGetProcesses      func() []ProcessInfo
	onGetProcessLogs    func(processID string) []string
	onLog               func(source, message string)

	// Process logs cache
	processLogs map[string][]string

	// Plugin log channel for async log delivery to UI
	logChan chan PluginLogEntry
}

// NewAppContext creates a new application context
func NewAppContext(manager *Manager) *AppContext {
	return &AppContext{
		manager:     manager,
		processLogs: make(map[string][]string),
		logChan:     make(chan PluginLogEntry, 100), // Buffered channel for logs
	}
}

// GetLogChannel returns the log channel for the UI to consume
func (c *AppContext) GetLogChannel() <-chan PluginLogEntry {
	return c.logChan
}

// SetProject sets the current project
func (c *AppContext) SetProject(project *cap.Project) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.project = project
}

// SetSettings sets the settings instance
func (c *AppContext) SetSettings(s *settings.Settings) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settings = s
}

// SetCallbacks sets all the UI callback functions
func (c *AppContext) SetCallbacks(
	getDevices func() []device.Device,
	getSelectedDevice func() *device.Device,
	refreshDevices func() error,
	runOnDevice func(deviceID string, liveReload bool) error,
	runWeb func() error,
	sync func(platform string) error,
	build func() error,
	openIDE func(platform string) error,
	killProcess func(processID string) error,
	getProcesses func() []ProcessInfo,
	getProcessLogs func(processID string) []string,
	log func(source, message string),
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onGetDevices = getDevices
	c.onGetSelectedDevice = getSelectedDevice
	c.onRefreshDevices = refreshDevices
	c.onRunOnDevice = runOnDevice
	c.onRunWeb = runWeb
	c.onSync = sync
	c.onBuild = build
	c.onOpenIDE = openIDE
	c.onKillProcess = killProcess
	c.onGetProcesses = getProcesses
	c.onGetProcessLogs = getProcessLogs
	c.onLog = log
}

// Context interface implementation

func (c *AppContext) GetProject() *cap.Project {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.project
}

func (c *AppContext) GetDevices() []device.Device {
	c.mu.RLock()
	fn := c.onGetDevices
	c.mu.RUnlock()

	if fn != nil {
		return fn()
	}
	return nil
}

func (c *AppContext) GetSelectedDevice() *device.Device {
	c.mu.RLock()
	fn := c.onGetSelectedDevice
	c.mu.RUnlock()

	if fn != nil {
		return fn()
	}
	return nil
}

func (c *AppContext) RefreshDevices() error {
	c.mu.RLock()
	fn := c.onRefreshDevices
	c.mu.RUnlock()

	if fn != nil {
		return fn()
	}
	return fmt.Errorf("refresh not available")
}

func (c *AppContext) RunOnDevice(deviceID string, liveReload bool) error {
	c.mu.RLock()
	fn := c.onRunOnDevice
	c.mu.RUnlock()

	if fn != nil {
		return fn(deviceID, liveReload)
	}
	return fmt.Errorf("run not available")
}

func (c *AppContext) RunWeb() error {
	c.mu.RLock()
	fn := c.onRunWeb
	c.mu.RUnlock()

	if fn != nil {
		return fn()
	}
	return fmt.Errorf("run web not available")
}

func (c *AppContext) Sync(platform string) error {
	c.mu.RLock()
	fn := c.onSync
	c.mu.RUnlock()

	if fn != nil {
		return fn(platform)
	}
	return fmt.Errorf("sync not available")
}

func (c *AppContext) Build() error {
	c.mu.RLock()
	fn := c.onBuild
	c.mu.RUnlock()

	if fn != nil {
		return fn()
	}
	return fmt.Errorf("build not available")
}

func (c *AppContext) OpenIDE(platform string) error {
	c.mu.RLock()
	fn := c.onOpenIDE
	c.mu.RUnlock()

	if fn != nil {
		return fn(platform)
	}
	return fmt.Errorf("open IDE not available")
}

func (c *AppContext) KillProcess(processID string) error {
	c.mu.RLock()
	fn := c.onKillProcess
	c.mu.RUnlock()

	if fn != nil {
		return fn(processID)
	}
	return fmt.Errorf("kill not available")
}

func (c *AppContext) GetProcesses() []ProcessInfo {
	c.mu.RLock()
	fn := c.onGetProcesses
	c.mu.RUnlock()

	if fn != nil {
		return fn()
	}
	return nil
}

func (c *AppContext) GetProcessLogs(processID string) []string {
	c.mu.RLock()
	fn := c.onGetProcessLogs
	c.mu.RUnlock()

	if fn != nil {
		return fn(processID)
	}
	return nil
}

func (c *AppContext) GetAllLogs() map[string][]string {
	processes := c.GetProcesses()
	result := make(map[string][]string)
	for _, p := range processes {
		result[p.ID] = c.GetProcessLogs(p.ID)
	}
	return result
}

func (c *AppContext) GetSettings() *settings.Settings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.settings
}

func (c *AppContext) GetSetting(key string) interface{} {
	c.mu.RLock()
	s := c.settings
	c.mu.RUnlock()

	if s == nil {
		return nil
	}

	// Try different types
	if v := s.GetBool(key); v {
		return v
	}
	if v := s.GetString(key); v != "" {
		return v
	}
	if v := s.GetInt(key); v != 0 {
		return v
	}
	return nil
}

func (c *AppContext) SetSetting(key string, value interface{}) error {
	c.mu.RLock()
	s := c.settings
	c.mu.RUnlock()

	if s == nil {
		return fmt.Errorf("settings not available")
	}

	switch v := value.(type) {
	case bool:
		s.SetBool(key, v)
	case string:
		s.SetString(key, v)
	case int:
		s.SetInt(key, v)
	case float64:
		s.SetInt(key, int(v))
	default:
		return fmt.Errorf("unsupported setting type")
	}

	return nil
}

func (c *AppContext) SaveSettings() error {
	c.mu.RLock()
	s := c.settings
	c.mu.RUnlock()

	if s == nil {
		return fmt.Errorf("settings not available")
	}
	return s.Save()
}

func (c *AppContext) GetDebugActions() []debug.Action {
	return debug.GetActions()
}

func (c *AppContext) RunDebugAction(actionID string) debug.Result {
	return debug.RunAction(actionID)
}

func (c *AppContext) GetPluginSetting(pluginID, key string) interface{} {
	if c.manager == nil {
		return nil
	}
	return c.manager.GetPluginSetting(pluginID, key)
}

func (c *AppContext) SetPluginSetting(pluginID, key string, value interface{}) error {
	if c.manager == nil {
		return fmt.Errorf("plugin manager not available")
	}
	return c.manager.SetPluginSetting(pluginID, key, value)
}

func (c *AppContext) Subscribe(event EventType, handler EventHandler) UnsubscribeFunc {
	if c.manager == nil {
		return func() {}
	}
	return c.manager.GetEventBus().Subscribe(event, handler)
}

func (c *AppContext) Emit(event EventType, data interface{}) {
	if c.manager != nil {
		c.manager.GetEventBus().Emit(event, data)
	}
}

func (c *AppContext) Log(pluginID string, message string) {
	// Send to log channel (non-blocking)
	select {
	case c.logChan <- PluginLogEntry{
		PluginID: pluginID,
		Message:  message,
		Time:     time.Now(),
	}:
	default:
		// Channel full, drop log to avoid blocking
	}
}

func (c *AppContext) LogError(pluginID string, err error) {
	c.Log(pluginID, fmt.Sprintf("ERROR: %v", err))
}

// AddProcessLog adds a log line for a process (called by UI)
func (c *AppContext) AddProcessLog(processID, line string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.processLogs[processID] = append(c.processLogs[processID], line)

	// Emit event
	if c.manager != nil {
		c.manager.GetEventBus().Emit(EventProcessOutput, ProcessOutputEvent{
			ProcessID: processID,
			Line:      line,
		})
	}
}

// NotifyProcessStarted emits a process started event
func (c *AppContext) NotifyProcessStarted(processID, name, command string) {
	if c.manager != nil {
		c.manager.GetEventBus().Emit(EventProcessStarted, ProcessStartedEvent{
			ProcessID: processID,
			Name:      name,
			Command:   command,
		})
	}
}

// NotifyProcessFinished emits a process finished event
func (c *AppContext) NotifyProcessFinished(processID string, success bool, err error) {
	if c.manager != nil {
		c.manager.GetEventBus().Emit(EventProcessFinished, ProcessFinishedEvent{
			ProcessID: processID,
			Success:   success,
			Error:     err,
		})
	}
}

// NotifyDevicesChanged emits a devices changed event
func (c *AppContext) NotifyDevicesChanged() {
	if c.manager != nil {
		c.manager.GetEventBus().Emit(EventDevicesChanged, nil)
	}
}

// NotifyDeviceSelected emits a device selected event
func (c *AppContext) NotifyDeviceSelected(dev *device.Device) {
	if c.manager != nil {
		c.manager.GetEventBus().Emit(EventDeviceSelected, DeviceSelectedEvent{
			Device: dev,
		})
	}
}

// GetStartTime helper for process info
func GetStartTime(t time.Time) int64 {
	return t.Unix()
}
