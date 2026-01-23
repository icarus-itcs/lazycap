package plugin

import (
	"github.com/icarus-itcs/lazycap/internal/cap"
	"github.com/icarus-itcs/lazycap/internal/debug"
	"github.com/icarus-itcs/lazycap/internal/device"
	"github.com/icarus-itcs/lazycap/internal/settings"
)

// Context provides plugins access to lazycap functionality
type Context interface {
	// Project Information
	GetProject() *cap.Project
	GetDevices() []device.Device
	GetSelectedDevice() *device.Device
	RefreshDevices() error

	// Build & Run Actions
	RunOnDevice(deviceID string, liveReload bool) error
	RunWeb() error
	Sync(platform string) error
	Build() error
	OpenIDE(platform string) error
	KillProcess(processID string) error

	// Process Management
	GetProcesses() []ProcessInfo
	GetProcessLogs(processID string) []string
	GetAllLogs() map[string][]string

	// Settings
	GetSettings() *settings.Settings
	GetSetting(key string) interface{}
	SetSetting(key string, value interface{}) error
	SaveSettings() error

	// Debug Actions
	GetDebugActions() []debug.Action
	RunDebugAction(actionID string) debug.Result

	// Plugin Settings (namespaced per plugin)
	GetPluginSetting(pluginID, key string) interface{}
	SetPluginSetting(pluginID, key string, value interface{}) error

	// Events
	Subscribe(event EventType, handler EventHandler) UnsubscribeFunc
	Emit(event EventType, data interface{})

	// Logging
	Log(pluginID string, message string)
	LogError(pluginID string, err error)
}

// ProcessInfo contains information about a running process
type ProcessInfo struct {
	ID        string
	Name      string
	Command   string
	Status    string // "running", "success", "failed", "cancelled"
	StartTime int64
	EndTime   int64
}

// EventType represents different events plugins can subscribe to
type EventType string

const (
	// Lifecycle events
	EventAppStarted     EventType = "app:started"
	EventAppStopping    EventType = "app:stopping"

	// Device events
	EventDevicesChanged EventType = "devices:changed"
	EventDeviceSelected EventType = "device:selected"
	EventDeviceBooted   EventType = "device:booted"

	// Process events
	EventProcessStarted  EventType = "process:started"
	EventProcessOutput   EventType = "process:output"
	EventProcessFinished EventType = "process:finished"

	// Build events
	EventBuildStarted  EventType = "build:started"
	EventBuildFinished EventType = "build:finished"
	EventSyncStarted   EventType = "sync:started"
	EventSyncFinished  EventType = "sync:finished"

	// Settings events
	EventSettingChanged EventType = "setting:changed"
)

// EventHandler is a callback for events
type EventHandler func(data interface{})

// UnsubscribeFunc removes an event subscription
type UnsubscribeFunc func()

// DeviceSelectedEvent is emitted when a device is selected
type DeviceSelectedEvent struct {
	Device *device.Device
}

// ProcessStartedEvent is emitted when a process starts
type ProcessStartedEvent struct {
	ProcessID string
	Name      string
	Command   string
}

// ProcessOutputEvent is emitted when a process outputs text
type ProcessOutputEvent struct {
	ProcessID string
	Line      string
}

// ProcessFinishedEvent is emitted when a process completes
type ProcessFinishedEvent struct {
	ProcessID string
	Success   bool
	Error     error
}

// SettingChangedEvent is emitted when a setting changes
type SettingChangedEvent struct {
	Key      string
	OldValue interface{}
	NewValue interface{}
}
