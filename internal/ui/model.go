package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lazycap/internal/cap"
	"lazycap/internal/debug"
	"lazycap/internal/device"
	"lazycap/internal/plugin"
	"lazycap/internal/preflight"
	"lazycap/internal/settings"
)

// Comprehensive ANSI escape sequence regex - handles:
// - CSI sequences: \x1b[...X (including private modes like ?25l, ?25h)
// - OSC sequences: \x1b]...BEL or \x1b]...ST
// - DCS/PM/APC sequences
// - Simple escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[PX^_].*?\x1b\\|\x1b.`)

// Debug logging
var (
	debugFile    *os.File
	debugFileMu  sync.Mutex
	debugLogPath = "/tmp/lazycap-debug.log"
)

func debugLog(format string, args ...interface{}) {
	debugFileMu.Lock()
	defer debugFileMu.Unlock()
	if debugFile == nil {
		var err error
		debugFile, err = os.OpenFile(debugLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return
		}
	}
	timestamp := time.Now().Format("15:04:05.000")
	fmt.Fprintf(debugFile, "[%s] %s\n", timestamp, fmt.Sprintf(format, args...))
	debugFile.Sync()
}

// setTerminalTitle sets the terminal tab/window title
func setTerminalTitle(title string) tea.Cmd {
	return tea.SetWindowTitle(title)
}

// getTerminalTitle generates the terminal title based on current state
func (m *Model) getTerminalTitle() string {
	// Count running processes
	running := 0
	for _, p := range m.processes {
		if p.Status == ProcessRunning {
			running++
		}
	}

	projectName := "lazycap"
	if m.project != nil && m.project.Name != "" {
		projectName = m.project.Name
	}

	if running > 0 {
		if running == 1 {
			// Show what's running
			for _, p := range m.processes {
				if p.Status == ProcessRunning {
					return fmt.Sprintf("⚡ %s - %s...", projectName, p.Name)
				}
			}
		}
		return fmt.Sprintf("⚡ %s - %d running", projectName, running)
	}

	if m.loading {
		return fmt.Sprintf("⚡ %s - loading...", projectName)
	}

	return fmt.Sprintf("⚡ %s - %d devices", projectName, len(m.devices))
}

// Focus tracks which pane is active
type Focus int

const (
	FocusDevices Focus = iota
	FocusLogs
)

// Model is the main app state
type Model struct {
	project     *cap.Project
	upgradeInfo *cap.UpgradeInfo

	// Devices
	devices        []device.Device
	selectedDevice int

	// Processes (tabs above logs)
	processes       []*Process
	selectedProcess int
	nextProcessID   int
	outputChans     map[string]chan string

	// Preflight checks
	preflightResults *preflight.Results
	showPreflight    bool

	// Settings
	settings         *settings.Settings
	showSettings     bool
	settingsCursor   int
	settingsCategory int

	// Plugins
	pluginManager *plugin.Manager
	pluginContext *plugin.AppContext
	showPlugins   bool
	pluginCursor  int

	// UI
	focus         Focus
	logViewport   viewport.Model
	spinner       spinner.Model
	help          help.Model
	keys          keyMap
	width         int
	height        int
	loading       bool
	showHelp      bool
	statusMessage string
	statusTime    time.Time

	// Quit confirmation
	confirmQuit bool
	quitTime    time.Time

	// Debug panel
	showDebug       bool
	debugActions    []debug.Action
	debugCursor     int
	debugCategory   int
	debugConfirm    bool
	debugResult     *debug.Result
	debugResultTime time.Time
}

type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Tab       key.Binding
	Run       key.Binding
	Sync      key.Binding
	Build     key.Binding
	Open      key.Binding
	Kill      key.Binding
	Refresh   key.Binding
	Upgrade   key.Binding
	Help      key.Binding
	Quit      key.Binding
	Left      key.Binding
	Right     key.Binding
	Copy      key.Binding
	Export    key.Binding
	Preflight key.Binding
	Settings  key.Binding
	Debug     key.Binding
	Plugins   key.Binding
	Enter     key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Tab:       key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
		Run:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run")),
		Sync:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		Build:     key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "build")),
		Open:      key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open IDE")),
		Kill:      key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "kill")),
		Refresh:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
		Upgrade:   key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "upgrade")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Left:      key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←", "prev tab")),
		Right:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→", "next tab")),
		Copy:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy logs")),
		Export:    key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "export logs")),
		Preflight: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "preflight")),
		Settings:  key.NewBinding(key.WithKeys(","), key.WithHelp(",", "settings")),
		Debug:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "debug")),
		Plugins:   key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "plugins")),
		Enter:     key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter", "toggle")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Run, k.Build, k.Sync, k.Tab, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Tab},
		{k.Run, k.Sync, k.Build},
		{k.Open, k.Kill, k.Refresh},
		{k.Help, k.Quit},
	}
}

// NewModel creates a new model (without plugin support)
func NewModel(project *cap.Project) Model {
	return NewModelWithPlugins(project, nil, nil)
}

// NewModelWithPlugins creates a new model with plugin support
func NewModelWithPlugins(project *cap.Project, pluginMgr *plugin.Manager, appCtx *plugin.AppContext) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(capBlue)

	// Run preflight checks
	preflightResults := preflight.Run()

	// Load settings
	userSettings, _ := settings.Load()

	m := Model{
		project:          project,
		focus:            FocusDevices,
		spinner:          s,
		logViewport:      viewport.New(0, 0),
		help:             help.New(),
		keys:             defaultKeyMap(),
		loading:          true,
		processes:        make([]*Process, 0),
		outputChans:      make(map[string]chan string),
		nextProcessID:    1,
		preflightResults: preflightResults,
		showPreflight:    preflightResults.HasErrors, // Show automatically if errors
		settings:         userSettings,
		pluginManager:    pluginMgr,
		pluginContext:    appCtx,
	}

	// Set up plugin context callbacks if plugins are enabled
	if appCtx != nil {
		appCtx.SetSettings(userSettings)
		appCtx.SetCallbacks(
			// GetDevices
			func() []device.Device { return m.devices },
			// GetSelectedDevice
			func() *device.Device { return m.getSelectedDevice() },
			// RefreshDevices
			func() error {
				// Trigger device refresh - this is called from plugins
				return nil
			},
			// RunOnDevice
			func(deviceID string, liveReload bool) error {
				for _, d := range m.devices {
					if d.ID == deviceID {
						// This would need to integrate with the tea.Cmd system
						return nil
					}
				}
				return fmt.Errorf("device %s not found", deviceID)
			},
			// RunWeb
			func() error { return nil },
			// Sync
			func(platform string) error { return nil },
			// Build
			func() error { return nil },
			// OpenIDE
			func(platform string) error { return nil },
			// KillProcess
			func(processID string) error {
				for _, p := range m.processes {
					if p.ID == processID && p.Status == ProcessRunning && p.Cmd != nil && p.Cmd.Process != nil {
						p.Cmd.Process.Kill()
						return nil
					}
				}
				return fmt.Errorf("process %s not found or not running", processID)
			},
			// GetProcesses
			func() []plugin.ProcessInfo {
				var infos []plugin.ProcessInfo
				for _, p := range m.processes {
					var status string
					switch p.Status {
					case ProcessRunning:
						status = "running"
					case ProcessSuccess:
						status = "success"
					case ProcessFailed:
						status = "failed"
					case ProcessCancelled:
						status = "cancelled"
					default:
						status = "unknown"
					}
					infos = append(infos, plugin.ProcessInfo{
						ID:        p.ID,
						Name:      p.Name,
						Command:   p.Command,
						Status:    status,
						StartTime: p.StartTime.Unix(),
					})
				}
				return infos
			},
			// GetProcessLogs
			func(processID string) []string {
				for _, p := range m.processes {
					if p.ID == processID {
						return p.Logs
					}
				}
				return nil
			},
			// Log
			func(source, message string) {
				m.addLog(fmt.Sprintf("[%s] %s", source, message))
			},
		)
	}

	return m
}

// NewDemoModel creates a model with mock data for screenshots/demos
func NewDemoModel(project *cap.Project, pluginMgr *plugin.Manager, appCtx *plugin.AppContext) Model {
	m := NewModelWithPlugins(project, pluginMgr, appCtx)
	m.loading = false

	// Mock devices - mix of physical devices, emulators, and web
	m.devices = []device.Device{
		// Web
		{ID: "web-dev", Name: "Web Browser", Platform: "web", IsWeb: true, Online: true},
		// iOS Physical Devices
		{ID: "00008101-ABC123DEF456", Name: "John's iPhone 15 Pro", Platform: "ios", IsEmulator: false, Online: true, OSVersion: "17.2"},
		{ID: "00008103-XYZ789GHI012", Name: "iPad Air (5th gen)", Platform: "ios", IsEmulator: false, Online: true, OSVersion: "17.1"},
		// iOS Simulators
		{ID: "iphone-15-pro-max", Name: "iPhone 15 Pro Max", Platform: "ios", IsEmulator: true, Online: true, OSVersion: "17.2"},
		{ID: "iphone-14", Name: "iPhone 14", Platform: "ios", IsEmulator: true, Online: true, OSVersion: "16.4"},
		{ID: "ipad-pro-12", Name: "iPad Pro (12.9-inch)", Platform: "ios", IsEmulator: true, Online: false, OSVersion: "17.2"},
		{ID: "iphone-se", Name: "iPhone SE (3rd gen)", Platform: "ios", IsEmulator: true, Online: false, OSVersion: "17.2"},
		// Android Physical Devices
		{ID: "R5CT1234ABC", Name: "Galaxy S24 Ultra", Platform: "android", IsEmulator: false, Online: true, APILevel: "34"},
		// Android Emulators
		{ID: "pixel-8-pro", Name: "Pixel 8 Pro API 34", Platform: "android", IsEmulator: true, Online: true, APILevel: "34"},
		{ID: "pixel-7", Name: "Pixel 7 API 33", Platform: "android", IsEmulator: true, Online: false, APILevel: "33"},
		{ID: "pixel-fold", Name: "Pixel Fold API 34", Platform: "android", IsEmulator: true, Online: false, APILevel: "34"},
	}

	// Mock multiple processes showing activity
	now := time.Now()
	m.processes = []*Process{
		// Currently running live reload on iPhone
		{
			ID:        "p1",
			Name:      "iPhone 15 Pro (live)",
			Command:   "npx cap run ios -l --external --target iphone-15-pro-max",
			Status:    ProcessRunning,
			StartTime: now.Add(-5 * time.Minute),
			Logs: []string{
				"[14:27:12] $ npx cap run ios -l --external --target iphone-15-pro-max",
				"",
				"[info] Starting live reload server...",
				"[info] Building web assets...",
				"",
				"> my-awesome-app@2.1.0 build",
				"> vite build",
				"",
				"vite v5.0.12 building for production...",
				"✓ 482 modules transformed.",
				"dist/index.html                   0.52 kB │ gzip:  0.31 kB",
				"dist/assets/index-Dk3mW9.css    28.43 kB │ gzip:  6.18 kB",
				"dist/assets/vendor-Ha8xQ2.js   186.24 kB │ gzip: 58.92 kB",
				"dist/assets/index-Bf3x9k.js    142.36 kB │ gzip: 45.82 kB",
				"✓ built in 4.18s",
				"",
				"[info] Syncing to iOS...",
				"✔ Copying web assets from dist to ios/App/App/public",
				"✔ Creating capacitor.config.json in ios/App/App",
				"✔ copy ios",
				"✔ update ios",
				"",
				"[info] Launching on iPhone 15 Pro Max...",
				"[info] Installing app...",
				"[info] App installed successfully",
				"[info] Launching app...",
				"",
				"  ➜  Local:   http://192.168.1.42:5173/",
				"  ➜  Network: http://192.168.1.42:5173/",
				"",
				"[14:28:45] App launched on iPhone 15 Pro Max",
				"[14:28:46] Live reload connected",
				"[14:29:02] [HMR] Updated: src/views/Home.vue",
				"[14:30:15] [HMR] Updated: src/components/Header.vue",
				"[14:31:33] [HMR] Updated: src/views/Settings.vue",
				"[14:32:01] [HMR] Updated: src/components/UserCard.vue",
			},
		},
		// Running on Android
		{
			ID:        "p2",
			Name:      "Pixel 8 Pro (live)",
			Command:   "npx cap run android -l --external --target pixel-8-pro",
			Status:    ProcessRunning,
			StartTime: now.Add(-3 * time.Minute),
			Logs: []string{
				"[14:29:12] $ npx cap run android -l --external --target pixel-8-pro",
				"",
				"[info] Starting live reload server...",
				"[info] Syncing to Android...",
				"✔ Copying web assets from dist to android/app/src/main/assets/public",
				"✔ copy android",
				"✔ update android",
				"",
				"[info] Building Android app...",
				"[info] Installing on Pixel 8 Pro...",
				"[info] App installed successfully",
				"[info] Launching app...",
				"",
				"  ➜  Local:   http://192.168.1.42:5173/",
				"  ➜  Network: http://192.168.1.42:5173/",
				"",
				"[14:30:15] App launched on Pixel 8 Pro",
				"[14:30:16] Live reload connected",
				"[14:31:33] [HMR] Updated: src/views/Settings.vue",
				"[14:32:01] [HMR] Updated: src/components/UserCard.vue",
			},
		},
		// Completed build
		{
			ID:        "p3",
			Name:      "Build",
			Command:   "npm run build",
			Status:    ProcessSuccess,
			StartTime: now.Add(-10 * time.Minute),
			EndTime:   now.Add(-9 * time.Minute),
			Logs: []string{
				"[14:22:00] $ npm run build",
				"",
				"> my-awesome-app@2.1.0 build",
				"> vite build",
				"",
				"vite v5.0.12 building for production...",
				"transforming (142) src/components/App.vue",
				"transforming (284) src/views/Home.vue",
				"transforming (396) src/composables/useAuth.ts",
				"✓ 482 modules transformed.",
				"dist/index.html                   0.52 kB │ gzip:  0.31 kB",
				"dist/assets/index-Dk3mW9.css    28.43 kB │ gzip:  6.18 kB",
				"dist/assets/vendor-Ha8xQ2.js   186.24 kB │ gzip: 58.92 kB",
				"dist/assets/index-Bf3x9k.js    142.36 kB │ gzip: 45.82 kB",
				"✓ built in 4.18s",
				"",
				"[14:22:05] ✓ Build completed successfully",
			},
		},
		// Completed sync
		{
			ID:        "p4",
			Name:      "Sync iOS",
			Command:   "npx cap sync ios",
			Status:    ProcessSuccess,
			StartTime: now.Add(-8 * time.Minute),
			EndTime:   now.Add(-7 * time.Minute),
			Logs: []string{
				"[14:24:00] $ npx cap sync ios",
				"",
				"✔ Copying web assets from dist to ios/App/App/public",
				"✔ Creating capacitor.config.json in ios/App/App",
				"✔ copy ios",
				"[info] Updating iOS plugins...",
				"  Found 8 Capacitor plugins for ios:",
				"    @capacitor/app@5.0.6",
				"    @capacitor/camera@5.0.7",
				"    @capacitor/haptics@5.0.6",
				"    @capacitor/keyboard@5.0.6",
				"    @capacitor/push-notifications@5.0.7",
				"    @capacitor/share@5.0.6",
				"    @capacitor/splash-screen@5.0.6",
				"    @capacitor/status-bar@5.0.6",
				"✔ update ios",
				"",
				"[info] Updating iOS native dependencies...",
				"[info] Running pod install...",
				"Analyzing dependencies",
				"Downloading dependencies",
				"Generating Pods project",
				"Integrating client project",
				"Pod installation complete!",
				"",
				"[14:24:32] ✓ Sync completed successfully",
			},
		},
		// Completed Android sync
		{
			ID:        "p5",
			Name:      "Sync Android",
			Command:   "npx cap sync android",
			Status:    ProcessSuccess,
			StartTime: now.Add(-7 * time.Minute),
			EndTime:   now.Add(-6 * time.Minute),
			Logs: []string{
				"[14:25:00] $ npx cap sync android",
				"",
				"✔ Copying web assets from dist to android/app/src/main/assets/public",
				"✔ Creating capacitor.config.json in android/app/src/main/assets",
				"✔ copy android",
				"[info] Updating Android plugins...",
				"  Found 8 Capacitor plugins for android:",
				"    @capacitor/app@5.0.6",
				"    @capacitor/camera@5.0.7",
				"    @capacitor/haptics@5.0.6",
				"    @capacitor/keyboard@5.0.6",
				"    @capacitor/push-notifications@5.0.7",
				"    @capacitor/share@5.0.6",
				"    @capacitor/splash-screen@5.0.6",
				"    @capacitor/status-bar@5.0.6",
				"✔ update android",
				"",
				"[14:25:18] ✓ Sync completed successfully",
			},
		},
	}
	m.selectedProcess = 0
	m.updateLogViewport()

	// Hide preflight for clean screenshot
	m.showPreflight = false
	m.preflightResults = &preflight.Results{
		HasErrors:   false,
		HasWarnings: false,
	}

	return m
}

// Messages
type devicesLoadedMsg struct{ devices []device.Device }
type upgradeCheckedMsg struct{ info *cap.UpgradeInfo }
type errMsg struct{ err error }
type processStartedMsg struct {
	processID  string
	cmd        *exec.Cmd
	outputChan chan string
}
type processOutputMsg struct {
	processID string
	line      string
}
type processFinishedMsg struct {
	processID string
	err       error
}
type deviceBootedMsg struct {
	device     *device.Device
	liveReload bool
	err        error
}

// Commands
func loadDevices() tea.Msg {
	devices, err := cap.ListDevices()
	if err != nil {
		return errMsg{err}
	}
	return devicesLoadedMsg{devices}
}

func checkUpgrade() tea.Msg {
	info, _ := cap.CheckForUpgrade()
	return upgradeCheckedMsg{info}
}

func (m *Model) getSelectedDevice() *device.Device {
	if len(m.devices) == 0 || m.selectedDevice >= len(m.devices) {
		return nil
	}
	return &m.devices[m.selectedDevice]
}

func (m *Model) getSelectedProcess() *Process {
	if len(m.processes) == 0 || m.selectedProcess >= len(m.processes) {
		return nil
	}
	return m.processes[m.selectedProcess]
}

func (m *Model) hasRunningProcesses() bool {
	for _, p := range m.processes {
		if p.Status == ProcessRunning {
			return true
		}
	}
	return false
}

func waitForOutput(processID string, ch chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return processFinishedMsg{processID: processID}
		}
		return processOutputMsg{processID: processID, line: line}
	}
}

func bootDevice(dev *device.Device, liveReload bool) tea.Cmd {
	return func() tea.Msg {
		if err := cap.BootDevice(dev.ID, dev.Platform, dev.IsEmulator); err != nil {
			return deviceBootedMsg{device: dev, liveReload: liveReload, err: err}
		}
		for i := 0; i < 60; i++ {
			time.Sleep(time.Second)
			if cap.IsDeviceBooted(dev.ID, dev.Platform) {
				dev.Online = true
				return deviceBootedMsg{device: dev, liveReload: liveReload}
			}
		}
		return deviceBootedMsg{device: dev, liveReload: liveReload, err: fmt.Errorf("timeout")}
	}
}

// gracefulShutdown kills all running processes and stops plugins
func (m *Model) gracefulShutdown() {
	// Stop all running processes
	for _, p := range m.processes {
		if p.Status == ProcessRunning && p.Cmd != nil && p.Cmd.Process != nil {
			p.Cmd.Process.Kill()
		}
	}

	// Stop all plugins
	if m.pluginManager != nil {
		m.pluginManager.StopAll()
	}
}

// Init starts the app
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadDevices,
		checkUpgrade,
		m.spinner.Tick,
		setTerminalTitle(m.getTerminalTitle()),
	)
}

// Update handles all messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Reset quit confirmation on any non-quit key (unless within timeout)
		if !key.Matches(msg, m.keys.Quit) && m.confirmQuit {
			if time.Since(m.quitTime) > 3*time.Second {
				m.confirmQuit = false
			}
		}

		// Handle settings mode input
		if m.showSettings {
			return m.handleSettingsInput(msg)
		}

		// Handle debug mode input
		if m.showDebug {
			return m.handleDebugInput(msg)
		}

		// Handle plugins panel input
		if m.showPlugins {
			return m.handlePluginsInput(msg)
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			// Check if Ctrl+C (force quit)
			if msg.String() == "ctrl+c" {
				m.gracefulShutdown()
				return m, tea.Quit
			}

			// Regular 'q' key - require confirmation
			if m.confirmQuit && time.Since(m.quitTime) < 3*time.Second {
				// Second press within 3 seconds - actually quit
				m.gracefulShutdown()
				return m, tea.Quit
			}

			// First press - show warning
			m.confirmQuit = true
			m.quitTime = time.Now()
			running := 0
			for _, p := range m.processes {
				if p.Status == ProcessRunning {
					running++
				}
			}
			if running > 0 {
				m.setStatus(fmt.Sprintf("⚠ %d process running! Press q again to quit", running))
			} else {
				m.setStatus("Press q again to quit")
			}
			return m, nil

		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			m.showPreflight = false
			return m, nil

		case key.Matches(msg, m.keys.Preflight):
			m.showPreflight = !m.showPreflight
			m.showHelp = false
			m.showSettings = false
			return m, nil

		case key.Matches(msg, m.keys.Settings):
			m.showSettings = !m.showSettings
			m.showHelp = false
			m.showPreflight = false
			m.showDebug = false
			m.settingsCursor = 0
			m.settingsCategory = 0
			return m, nil

		case key.Matches(msg, m.keys.Debug):
			m.showDebug = !m.showDebug
			m.showHelp = false
			m.showPreflight = false
			m.showSettings = false
			m.showPlugins = false
			m.debugActions = debug.GetActions()
			m.debugCursor = 0
			m.debugCategory = 0
			m.debugConfirm = false
			m.debugResult = nil
			return m, nil

		case key.Matches(msg, m.keys.Plugins):
			m.showPlugins = !m.showPlugins
			m.showHelp = false
			m.showPreflight = false
			m.showSettings = false
			m.showDebug = false
			m.pluginCursor = 0
			return m, nil

		case key.Matches(msg, m.keys.Tab):
			if m.focus == FocusDevices {
				m.focus = FocusLogs
			} else {
				m.focus = FocusDevices
			}
			return m, nil

		case key.Matches(msg, m.keys.Up):
			if m.focus == FocusDevices {
				if m.selectedDevice > 0 {
					m.selectedDevice--
				}
			} else {
				m.logViewport.LineUp(3)
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			if m.focus == FocusDevices {
				if m.selectedDevice < len(m.devices)-1 {
					m.selectedDevice++
				}
			} else {
				m.logViewport.LineDown(3)
			}
			return m, nil

		case key.Matches(msg, m.keys.Left):
			if m.focus == FocusLogs && m.selectedProcess > 0 {
				m.selectedProcess--
				m.updateLogViewport()
			}
			return m, nil

		case key.Matches(msg, m.keys.Right):
			if m.focus == FocusLogs && m.selectedProcess < len(m.processes)-1 {
				m.selectedProcess++
				m.updateLogViewport()
			}
			return m, nil

		case key.Matches(msg, m.keys.Run):
			// Use live reload setting
			liveReload := m.settings.GetBool("liveReloadDefault")
			return m, m.runAction("run", liveReload)
		case key.Matches(msg, m.keys.Sync):
			return m, m.runAction("sync", false)
		case key.Matches(msg, m.keys.Build):
			return m, m.runAction("build", false)
		case key.Matches(msg, m.keys.Open):
			return m, m.runAction("open", false)
		case key.Matches(msg, m.keys.Refresh):
			m.loading = true
			return m, tea.Batch(loadDevices, checkUpgrade)
		case key.Matches(msg, m.keys.Upgrade):
			if m.upgradeInfo != nil && m.upgradeInfo.HasUpgrade {
				return m, m.startUpgrade()
			}
		case key.Matches(msg, m.keys.Kill):
			p := m.getSelectedProcess()
			if p != nil && p.Status == ProcessRunning && p.Cmd != nil && p.Cmd.Process != nil {
				p.Cmd.Process.Kill()
				p.Status = ProcessCancelled
				p.EndTime = time.Now()
				p.AddLog("Killed by user")
				m.updateLogViewport()
			}
			return m, nil

		case key.Matches(msg, m.keys.Copy):
			p := m.getSelectedProcess()
			if p != nil && len(p.Logs) > 0 {
				content := strings.Join(p.Logs, "\n")
				if err := clipboard.WriteAll(content); err != nil {
					m.setStatus("Copy failed: " + err.Error())
				} else {
					m.setStatus(fmt.Sprintf("Copied %d lines to clipboard", len(p.Logs)))
				}
			} else {
				m.setStatus("No logs to copy")
			}
			return m, nil

		case key.Matches(msg, m.keys.Export):
			p := m.getSelectedProcess()
			if p != nil && len(p.Logs) > 0 {
				filename := fmt.Sprintf("lazycap-%s-%s.log", p.Name, time.Now().Format("20060102-150405"))
				exportPath := filepath.Join(os.TempDir(), filename)
				content := strings.Join(p.Logs, "\n")
				if err := os.WriteFile(exportPath, []byte(content), 0644); err != nil {
					m.setStatus("Export failed: " + err.Error())
				} else {
					m.setStatus("Exported to " + exportPath)
				}
			} else {
				m.setStatus("No logs to export")
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case devicesLoadedMsg:
		m.loading = false
		m.devices = msg.devices
		cmds = append(cmds, setTerminalTitle(m.getTerminalTitle()))

	case upgradeCheckedMsg:
		m.upgradeInfo = msg.info

	case processStartedMsg:
		for _, p := range m.processes {
			if p.ID == msg.processID {
				p.Cmd = msg.cmd
				p.OutputChan = msg.outputChan
				m.outputChans[msg.processID] = msg.outputChan
				break
			}
		}
		cmds = append(cmds, waitForOutput(msg.processID, msg.outputChan), m.spinner.Tick, setTerminalTitle(m.getTerminalTitle()))

	case processOutputMsg:
		for _, p := range m.processes {
			if p.ID == msg.processID && msg.line != "" {
				clean := strings.TrimSpace(ansiRegex.ReplaceAllString(msg.line, ""))
				if clean != "" {
					p.AddLog(clean)
				}
				if m.getSelectedProcess() == p {
					m.updateLogViewport()
				}
				break
			}
		}
		if ch, ok := m.outputChans[msg.processID]; ok {
			cmds = append(cmds, waitForOutput(msg.processID, ch))
		}
		cmds = append(cmds, m.spinner.Tick)

	case processFinishedMsg:
		for _, p := range m.processes {
			if p.ID == msg.processID && p.Status == ProcessRunning {
				if msg.err != nil {
					p.Status = ProcessFailed
					p.AddLog(fmt.Sprintf("Error: %v", msg.err))
				} else {
					p.Status = ProcessSuccess
					p.AddLog("✓ Done")
				}
				p.EndTime = time.Now()
				break
			}
		}
		delete(m.outputChans, msg.processID)
		m.updateLogViewport()
		cmds = append(cmds, setTerminalTitle(m.getTerminalTitle()))

	case deviceBootedMsg:
		if msg.err != nil {
			m.addLog(fmt.Sprintf("Boot failed: %v", msg.err))
			return m, nil
		}
		for i, d := range m.devices {
			if d.ID == msg.device.ID {
				m.devices[i].Online = true
				break
			}
		}
		return m, m.startRunCommand(msg.device, msg.liveReload)

	case errMsg:
		m.loading = false
		m.addLog(fmt.Sprintf("Error: %v", msg.err))
	}

	if m.hasRunningProcesses() && len(cmds) == 0 {
		cmds = append(cmds, m.spinner.Tick)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) updateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	logWidth := m.width - 36 - 6
	logHeight := m.height - 8
	if logHeight < 5 {
		logHeight = 5
	}
	m.logViewport.Width = logWidth
	m.logViewport.Height = logHeight
}

func (m *Model) updateLogViewport() {
	p := m.getSelectedProcess()
	if p == nil {
		m.logViewport.SetContent(logEmptyStyle.Render("\n  Run a command to see output here..."))
		return
	}
	m.logViewport.SetContent(strings.Join(p.Logs, "\n"))
	m.logViewport.GotoBottom()
}

func (m *Model) addLog(line string) {
	ts := time.Now().Format("15:04:05")
	if len(m.processes) == 0 {
		m.processes = append(m.processes, &Process{
			ID: "system", Name: "System", Status: ProcessSuccess,
			StartTime: time.Now(), Logs: []string{fmt.Sprintf("[%s] %s", ts, line)},
		})
		m.selectedProcess = 0
	} else {
		m.processes[0].AddLog(fmt.Sprintf("[%s] %s", ts, line))
	}
	m.updateLogViewport()
}

func (m *Model) setStatus(msg string) {
	m.statusMessage = msg
	m.statusTime = time.Now()
}

func (m *Model) createProcess(name, command string) *Process {
	id := fmt.Sprintf("p%d", m.nextProcessID)
	m.nextProcessID++
	p := &Process{
		ID: id, Name: name, Command: command, Status: ProcessRunning,
		StartTime: time.Now(),
		Logs:      []string{fmt.Sprintf("[%s] $ %s", time.Now().Format("15:04:05"), command)},
	}
	m.processes = append(m.processes, p)
	m.selectedProcess = len(m.processes) - 1
	m.updateLogViewport()
	return p
}

func (m *Model) runAction(action string, liveReload bool) tea.Cmd {
	dev := m.getSelectedDevice()

	switch action {
	case "run":
		if dev == nil {
			m.addLog("No device selected")
			return nil
		}
		// Handle web platform
		if dev.IsWeb {
			return m.startWebDevCommand()
		}
		if !dev.Online {
			m.addLog(fmt.Sprintf("Booting %s...", dev.Name))
			p := m.createProcess("Boot "+dev.Name, "xcrun simctl boot")
			p.AddLog("Waiting for simulator...")
			return tea.Batch(bootDevice(dev, liveReload), m.spinner.Tick)
		}
		return m.startRunCommand(dev, liveReload)
	case "sync":
		platform := ""
		if dev != nil {
			platform = dev.Platform
		}
		return m.startSyncCommand(platform)
	case "build":
		return m.startBuildCommand()
	case "open":
		if dev == nil {
			if m.project.HasIOS {
				return m.startOpenCommand("ios")
			} else if m.project.HasAndroid {
				return m.startOpenCommand("android")
			}
			m.addLog("No platform available")
			return nil
		}
		// Handle web platform - open browser
		if dev.IsWeb {
			url := cap.GetWebDevURL(cap.WebDevOptions{
				Port:  m.settings.GetInt("webDevPort"),
				Host:  m.settings.GetString("webHost"),
				Https: m.settings.GetBool("webHttps"),
			})
			browserPath := m.settings.GetString("webBrowserPath")
			if err := cap.OpenBrowser(url, browserPath); err != nil {
				m.addLog(fmt.Sprintf("Failed to open browser: %v", err))
			} else {
				m.addLog(fmt.Sprintf("Opened browser to %s", url))
			}
			return nil
		}
		return m.startOpenCommand(dev.Platform)
	}
	return nil
}

func (m *Model) startRunCommand(dev *device.Device, liveReload bool) tea.Cmd {
	// Include device name in process name for easy identification
	shortName := dev.Name
	if len(shortName) > 15 {
		shortName = shortName[:13] + ".."
	}

	name := shortName
	args := []string{"cap", "run", dev.Platform, "--target", dev.ID}
	if liveReload {
		args = append(args, "-l", "--external")
		name = shortName + " (live)"
	}
	p := m.createProcess(name, "npx "+strings.Join(args, " "))
	return runCmd(p.ID, "npx", args...)
}

func (m *Model) startSyncCommand(platform string) tea.Cmd {
	args := []string{"cap", "sync"}
	if platform != "" {
		args = append(args, platform)
	}
	p := m.createProcess("Sync", "npx "+strings.Join(args, " "))
	return runCmd(p.ID, "npx", args...)
}

func (m *Model) startBuildCommand() tea.Cmd {
	p := m.createProcess("Build", "npm run build")
	return runCmd(p.ID, "npm", "run", "build")
}

func (m *Model) startOpenCommand(platform string) tea.Cmd {
	p := m.createProcess("Open", "npx cap open "+platform)
	return runCmd(p.ID, "npx", "cap", "open", platform)
}

func (m *Model) startUpgrade() tea.Cmd {
	p := m.createProcess("Upgrade", "npm install @capacitor/core@latest @capacitor/cli@latest")
	return runCmd(p.ID, "npm", "install", "@capacitor/core@latest", "@capacitor/cli@latest")
}

func (m *Model) startWebDevCommand() tea.Cmd {
	// Get web settings
	command := m.settings.GetString("webDevCommand")
	if command == "" {
		command = cap.DetectWebDevCommand()
	}
	port := m.settings.GetInt("webDevPort")
	host := m.settings.GetString("webHost")
	openBrowser := m.settings.GetBool("webOpenBrowser")
	browserPath := m.settings.GetString("webBrowserPath")
	https := m.settings.GetBool("webHttps")

	p := m.createProcess("Web", command)

	// Kill any process using the port first
	if cap.KillPort(port) {
		p.AddLog(fmt.Sprintf("Killed existing process on port %d", port))
	}

	// Build URL for status message
	url := cap.GetWebDevURL(cap.WebDevOptions{
		Port:  port,
		Host:  host,
		Https: https,
	})

	p.AddLog(fmt.Sprintf("Starting dev server at %s", url))

	// If open browser is enabled, wait for server to be ready then open
	if openBrowser {
		go func() {
			// Wait for the server to be ready (poll the port)
			ready := cap.WaitForPort(port, 30*time.Second)
			if ready {
				cap.OpenBrowser(url, browserPath)
			}
		}()
	}

	// Run the command directly - let the dev server use its own defaults
	// The command should be the full command like "npm run dev" or "npx vite"
	return runWebCmd(p.ID, command, port, host)
}

func runCmd(processID, name string, args ...string) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan string, 100)

		// Build the command string
		cmdStr := name
		for _, arg := range args {
			if strings.Contains(arg, " ") {
				cmdStr += fmt.Sprintf(" %q", arg)
			} else {
				cmdStr += " " + arg
			}
		}

		// Run through user's shell with full environment
		// Using 'source' to load shell config ensures proper PATH
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}

		// Source the profile explicitly and run command
		shellCmd := fmt.Sprintf("source ~/.zshrc 2>/dev/null; source ~/.zprofile 2>/dev/null; %s", cmdStr)
		cmd := exec.Command(shell, "-c", shellCmd)

		// Inherit full environment
		cmd.Env = os.Environ()

		// Set working directory
		if cwd, err := os.Getwd(); err == nil {
			cmd.Dir = cwd
		}

		return runCmdWithPipes(processID, cmd, ch)
	}
}

// runWebCmd runs a web dev server command with proper port/host handling
func runWebCmd(processID, command string, port int, host string) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan string, 100)

		// Build the full command
		// For npm/yarn/pnpm run commands, we need to use -- to pass args to the script
		cmdStr := command

		// Check if we need to add port/host args
		hasExtraArgs := port > 0 || (host != "" && host != "localhost")

		if hasExtraArgs {
			// For npm/yarn/pnpm run commands, add -- separator
			if strings.HasPrefix(command, "npm run") ||
			   strings.HasPrefix(command, "yarn run") ||
			   strings.HasPrefix(command, "pnpm run") ||
			   strings.HasPrefix(command, "yarn ") ||
			   strings.HasPrefix(command, "pnpm ") {
				cmdStr += " --"
			}

			if port > 0 {
				cmdStr += fmt.Sprintf(" --port %d", port)
			}
			if host != "" && host != "localhost" {
				cmdStr += fmt.Sprintf(" --host %s", host)
			}
		}

		// Run through user's shell
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}

		shellCmd := fmt.Sprintf("source ~/.zshrc 2>/dev/null; source ~/.zprofile 2>/dev/null; %s", cmdStr)
		cmd := exec.Command(shell, "-c", shellCmd)
		cmd.Env = os.Environ()

		if cwd, err := os.Getwd(); err == nil {
			cmd.Dir = cwd
		}

		return runCmdWithPipes(processID, cmd, ch)
	}
}

// runCmdWithPipes runs command using pipes instead of PTY
func runCmdWithPipes(processID string, cmd *exec.Cmd, ch chan string) tea.Msg {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		close(ch)
		return processFinishedMsg{processID: processID, err: err}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		close(ch)
		return processFinishedMsg{processID: processID, err: err}
	}

	if err := cmd.Start(); err != nil {
		close(ch)
		return processFinishedMsg{processID: processID, err: err}
	}

	// Read both stdout and stderr
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case ch <- scanner.Text():
			default:
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			select {
			case ch <- scanner.Text():
			default:
			}
		}
	}()

	go func() {
		cmd.Wait()
		close(ch)
	}()

	return processStartedMsg{processID: processID, cmd: cmd, outputChan: ch}
}

// View renders the UI
func (m Model) View() string {
	if m.showHelp {
		return m.help.View(m.keys)
	}

	if m.showPreflight {
		return m.renderPreflight()
	}

	if m.showSettings {
		return m.renderSettings()
	}

	if m.showDebug {
		return m.renderDebug()
	}

	if m.showPlugins {
		return m.renderPlugins()
	}

	// Build the view
	left := m.renderLeft()
	right := m.renderRight()

	main := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		m.renderHeader(),
		"",
		main,
		"",
		m.renderHelp(),
	)
}

func (m *Model) renderHeader() string {
	// Logo
	logo := "  " + LogoCompact()

	// Project info
	project := projectStyle.Render(m.project.Name)

	// Platforms
	var platforms []string
	if m.project.HasIOS {
		platforms = append(platforms, iosBadge.Render("iOS"))
	}
	if m.project.HasAndroid {
		platforms = append(platforms, androidBadge.Render("Android"))
	}
	platformStr := strings.Join(platforms, " ")

	// Status
	var status string
	if m.loading {
		status = m.spinner.View() + " loading..."
	} else if m.hasRunningProcesses() {
		count := 0
		for _, p := range m.processes {
			if p.Status == ProcessRunning {
				count++
			}
		}
		status = fmt.Sprintf("%s %d running", m.spinner.View(), count)
	} else {
		status = mutedStyle.Render(fmt.Sprintf("%d devices", len(m.devices)))
	}

	// Upgrade notice
	var upgrade string
	if m.upgradeInfo != nil && m.upgradeInfo.HasUpgrade {
		upgrade = upgradeStyle.Render(fmt.Sprintf("  ↑ v%s available", m.upgradeInfo.LatestVersion))
	}

	// Preflight indicator
	var preflightIndicator string
	if m.preflightResults != nil {
		if m.preflightResults.HasErrors {
			preflightIndicator = "  " + errorStyle.Render("⚠ preflight errors")
		} else if m.preflightResults.HasWarnings {
			preflightIndicator = "  " + lipgloss.NewStyle().Foreground(warnColor).Render("⚠ preflight warnings")
		}
	}

	// Plugin status indicators
	var pluginStatus string
	if m.pluginManager != nil {
		runningPlugins := m.pluginManager.GetRunningPlugins()
		if len(runningPlugins) > 0 {
			var statusParts []string
			for _, p := range runningPlugins {
				if sl := p.GetStatusLine(); sl != "" {
					statusParts = append(statusParts, sl)
				}
			}
			if len(statusParts) > 0 {
				pluginStatus = "  " + mutedStyle.Render(strings.Join(statusParts, " • "))
			}
		}
	}

	// Status message (show for 3 seconds)
	var statusMsg string
	if m.statusMessage != "" && time.Since(m.statusTime) < 3*time.Second {
		statusMsg = "  " + successStyle.Render(m.statusMessage)
	}

	return fmt.Sprintf("%s  %s  %s  %s%s%s%s%s", logo, project, platformStr, status, upgrade, preflightIndicator, pluginStatus, statusMsg)
}

func (m *Model) renderLeft() string {
	title := titleStyle.Render("DEVICES")

	var items []string
	for i, d := range m.devices {
		// Status indicator
		var status string
		if d.Online {
			status = onlineStyle.Render("●")
		} else {
			status = offlineStyle.Render("○")
		}

		// Platform badge
		var platform string
		switch d.Platform {
		case "ios":
			platform = iosBadge.Render("iOS")
		case "android":
			platform = androidBadge.Render("And")
		case "web":
			platform = webBadge.Render("Web")
		}

		// Device type indicator
		var deviceType string
		if d.IsWeb {
			deviceType = mutedStyle.Render("dev")
		} else if d.IsEmulator {
			deviceType = mutedStyle.Render("sim")
		} else {
			deviceType = mutedStyle.Render("dev")
		}

		// Device name - truncate if needed
		name := d.Name
		if len(name) > 18 {
			name = name[:15] + "..."
		}

		// Build the line
		isSelected := i == m.selectedDevice
		isFocused := m.focus == FocusDevices

		if isSelected && isFocused {
			// Selected and focused: arrow indicator + cyan text
			arrow := lipgloss.NewStyle().Foreground(capBlue).Bold(true).Render("▶")
			nameStyled := lipgloss.NewStyle().Foreground(capCyan).Bold(true).Render(name)
			line := fmt.Sprintf(" %s %s %s %s  %s", arrow, status, platform, nameStyled, deviceType)
			items = append(items, line)
		} else if isSelected {
			// Selected but not focused: subtle highlight
			arrow := mutedStyle.Render("▶")
			nameStyled := lipgloss.NewStyle().Foreground(capLight).Render(name)
			line := fmt.Sprintf(" %s %s %s %s  %s", arrow, status, platform, nameStyled, deviceType)
			items = append(items, line)
		} else {
			// Not selected
			nameStyled := lipgloss.NewStyle().Foreground(capLight).Render(name)
			line := fmt.Sprintf("   %s %s %s  %s", status, platform, nameStyled, deviceType)
			items = append(items, line)
		}
	}

	if len(items) == 0 {
		items = append(items, mutedStyle.Render("  No devices found"))
		items = append(items, "")
		items = append(items, mutedStyle.Render("  Press R to refresh"))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, items...)

	paneHeight := m.height - 8
	if paneHeight < 5 {
		paneHeight = 5
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, "", content)

	if m.focus == FocusDevices {
		return activePaneStyle.Width(32).Height(paneHeight).Render(inner)
	}
	return inactivePaneStyle.Width(32).Height(paneHeight).Render(inner)
}

func (m *Model) renderRight() string {
	paneWidth := m.width - 36 - 6
	paneHeight := m.height - 8
	if paneHeight < 5 {
		paneHeight = 5
	}
	if paneWidth < 20 {
		paneWidth = 20
	}

	m.logViewport.Width = paneWidth - 4
	m.logViewport.Height = paneHeight - 4

	// Show welcome screen when no processes
	if len(m.processes) == 0 {
		return m.renderWelcome(paneWidth, paneHeight)
	}

	// Process tabs - simple text-based tabs
	var tabParts []string

	for i, p := range m.processes {
		// Status icon
		var icon string
		switch p.Status {
		case ProcessRunning:
			icon = m.spinner.View()
		case ProcessSuccess:
			icon = successStyle.Render("✓")
		case ProcessFailed:
			icon = failedStyle.Render("✗")
		case ProcessCancelled:
			icon = mutedStyle.Render("○")
		}

		name := p.Name
		if len(name) > 12 {
			name = name[:10] + ".."
		}

		// Simple format: selected gets highlight, others are muted
		if i == m.selectedProcess {
			// Selected: bright with underline effect using brackets
			tabParts = append(tabParts, fmt.Sprintf("%s [%s]", icon, lipgloss.NewStyle().Foreground(capBlue).Bold(true).Render(name)))
		} else {
			// Unselected: muted
			tabParts = append(tabParts, fmt.Sprintf("%s %s", icon, mutedStyle.Render(name)))
		}
	}

	tabBar := strings.Join(tabParts, "  │  ")

	// Logs
	logContent := m.logViewport.View()

	inner := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", logContent)

	if m.focus == FocusLogs {
		return activeLogPaneStyle.Width(paneWidth).Height(paneHeight).Render(inner)
	}
	return logPaneStyle.Width(paneWidth).Height(paneHeight).Render(inner)
}

func (m *Model) renderWelcome(width, height int) string {
	// Capacitor-style lightning bolt logo
	boltStyle := lipgloss.NewStyle().Foreground(capBlue).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(capLight).Bold(true)

	// Lightning bolt ASCII art (Capacitor logo)
	logo := lipgloss.JoinVertical(lipgloss.Center,
		"",
		boltStyle.Render("        ██████████"),
		boltStyle.Render("       ██████████"),
		boltStyle.Render("      ██████████"),
		boltStyle.Render("     ██████████"),
		boltStyle.Render("    ████████████████"),
		boltStyle.Render("        ██████████"),
		boltStyle.Render("         ██████████"),
		boltStyle.Render("          ██████████"),
		boltStyle.Render("           █████████"),
		"",
		textStyle.Render("lazycap"),
		mutedStyle.Render("Capacitor Dashboard"),
		"",
		"",
		mutedStyle.Render("Select a device and press"),
		helpKeyStyle.Render("r")+mutedStyle.Render(" to run  •  ")+helpKeyStyle.Render(",")+mutedStyle.Render(" for settings"),
		"",
	)

	// Center the logo in the pane
	centered := lipgloss.Place(width-4, height-4, lipgloss.Center, lipgloss.Center, logo)

	if m.focus == FocusLogs {
		return activeLogPaneStyle.Width(width).Height(height).Render(centered)
	}
	return logPaneStyle.Width(width).Height(height).Render(centered)
}

func (m *Model) renderHelp() string {
	keys := []string{
		helpKeyStyle.Render("r") + " run",
		helpKeyStyle.Render("b") + " build",
		helpKeyStyle.Render("s") + " sync",
		helpKeyStyle.Render("o") + " open",
		helpKeyStyle.Render("x") + " kill",
		helpKeyStyle.Render("d") + " debug",
		helpKeyStyle.Render("P") + " plugins",
		helpKeyStyle.Render(",") + " settings",
		helpKeyStyle.Render("q") + " quit",
	}
	return helpStyle.Render("  " + strings.Join(keys, "  "))
}

func (m *Model) renderPreflight() string {
	title := lipgloss.NewStyle().
		Foreground(capBlue).
		Bold(true).
		MarginBottom(1).
		Render("  ⚡ Preflight Checks")

	var lines []string
	lines = append(lines, "")
	lines = append(lines, title)
	lines = append(lines, "")

	// Status icons
	okIcon := successStyle.Render("✓")
	warnIcon := lipgloss.NewStyle().Foreground(warnColor).Render("!")
	errIcon := errorStyle.Render("✗")

	nameStyle := lipgloss.NewStyle().Width(20)
	pathStyle := mutedStyle

	for _, check := range m.preflightResults.Checks {
		var icon string
		var msgStyle lipgloss.Style

		switch check.Status {
		case preflight.StatusOK:
			icon = okIcon
			msgStyle = successStyle
		case preflight.StatusWarning:
			icon = warnIcon
			msgStyle = lipgloss.NewStyle().Foreground(warnColor)
		case preflight.StatusError:
			icon = errIcon
			msgStyle = errorStyle
		}

		name := nameStyle.Render(check.Name)
		msg := msgStyle.Render(check.Message)

		line := fmt.Sprintf("  %s %s %s", icon, name, msg)
		if check.Path != "" && check.Status == preflight.StatusOK {
			line += "  " + pathStyle.Render(check.Path)
		}
		lines = append(lines, line)
	}

	lines = append(lines, "")
	lines = append(lines, "")

	// Summary
	summary := m.preflightResults.Summary()
	if m.preflightResults.HasErrors {
		lines = append(lines, "  "+errorStyle.Render("⚠ "+summary))
		lines = append(lines, "")
		lines = append(lines, "  "+mutedStyle.Render("Some required tools are missing. Please install them to continue."))
	} else if m.preflightResults.HasWarnings {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(warnColor).Render("⚠ "+summary))
		lines = append(lines, "")
		lines = append(lines, "  "+mutedStyle.Render("Some optional tools are missing. Some features may not work."))
	} else {
		lines = append(lines, "  "+successStyle.Render("✓ "+summary))
		lines = append(lines, "")
		lines = append(lines, "  "+mutedStyle.Render("All systems go!"))
	}

	lines = append(lines, "")
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("  Press "+helpKeyStyle.Render("p")+" to close  •  "+helpKeyStyle.Render("q")+" to quit"))

	return strings.Join(lines, "\n")
}

func (m Model) handleSettingsInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	categories := settings.GetCategories()
	currentCategory := categories[m.settingsCategory]

	switch msg.String() {
	case "ctrl+c":
		m.gracefulShutdown()
		return m, tea.Quit

	case "q":
		// Require confirmation
		if m.confirmQuit && time.Since(m.quitTime) < 3*time.Second {
			m.gracefulShutdown()
			return m, tea.Quit
		}
		m.confirmQuit = true
		m.quitTime = time.Now()
		m.setStatus("Press q again to quit")
		return m, nil

	case "esc", ",":
		m.showSettings = false
		return m, nil

	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		} else if m.settingsCategory > 0 {
			// Move to previous category
			m.settingsCategory--
			m.settingsCursor = len(categories[m.settingsCategory].Settings) - 1
		}
		return m, nil

	case "down", "j":
		if m.settingsCursor < len(currentCategory.Settings)-1 {
			m.settingsCursor++
		} else if m.settingsCategory < len(categories)-1 {
			// Move to next category
			m.settingsCategory++
			m.settingsCursor = 0
		}
		return m, nil

	case "left", "h":
		if m.settingsCategory > 0 {
			m.settingsCategory--
			m.settingsCursor = 0
		}
		return m, nil

	case "right", "l":
		if m.settingsCategory < len(categories)-1 {
			m.settingsCategory++
			m.settingsCursor = 0
		}
		return m, nil

	case "enter", " ":
		// Toggle or cycle the current setting
		setting := currentCategory.Settings[m.settingsCursor]
		switch setting.Type {
		case "bool":
			m.settings.ToggleBool(setting.Key)
			m.settings.Save()
			m.setStatus(fmt.Sprintf("%s: %v", setting.Name, m.settings.GetBool(setting.Key)))
		case "choice":
			newVal := m.settings.CycleChoice(setting.Key, setting.Choices)
			m.settings.Save()
			displayVal := newVal
			if displayVal == "" {
				displayVal = "(auto)"
			}
			m.setStatus(fmt.Sprintf("%s: %s", setting.Name, displayVal))
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) renderSettings() string {
	categories := settings.GetCategories()

	// Title
	title := lipgloss.NewStyle().
		Foreground(capBlue).
		Bold(true).
		Render("  ⚡ Settings")

	var lines []string
	lines = append(lines, "")
	lines = append(lines, title)
	lines = append(lines, "")

	// Category tabs
	var tabs []string
	for i, cat := range categories {
		tabText := fmt.Sprintf(" %s %s ", cat.Icon, cat.Name)
		if i == m.settingsCategory {
			tabs = append(tabs, activeTabStyle.Render(tabText))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(tabText))
		}
	}
	lines = append(lines, "  "+strings.Join(tabs, " "))
	lines = append(lines, "")

	// Current category settings
	currentCategory := categories[m.settingsCategory]

	// Calculate max widths for alignment
	maxNameWidth := 0
	for _, s := range currentCategory.Settings {
		if len(s.Name) > maxNameWidth {
			maxNameWidth = len(s.Name)
		}
	}
	nameStyle := lipgloss.NewStyle().Width(maxNameWidth + 2)

	for i, s := range currentCategory.Settings {
		var valueStr string
		var valueStyle lipgloss.Style

		switch s.Type {
		case "bool":
			val := m.settings.GetBool(s.Key)
			if val {
				valueStr = "✓ ON"
				valueStyle = successStyle
			} else {
				valueStr = "○ OFF"
				valueStyle = mutedStyle
			}
		case "string":
			val := m.settings.GetString(s.Key)
			if val == "" {
				valueStr = "(not set)"
				valueStyle = mutedStyle
			} else {
				if len(val) > 25 {
					val = val[:22] + "..."
				}
				valueStr = val
				valueStyle = lipgloss.NewStyle().Foreground(capCyan)
			}
		case "int":
			val := m.settings.GetInt(s.Key)
			valueStr = fmt.Sprintf("%d", val)
			valueStyle = lipgloss.NewStyle().Foreground(capCyan)
		case "choice":
			val := m.settings.GetString(s.Key)
			if val == "" {
				valueStr = "(auto)"
			} else {
				valueStr = val
			}
			valueStyle = lipgloss.NewStyle().Foreground(capCyan)
		}

		name := nameStyle.Render(s.Name)
		value := valueStyle.Render(valueStr)
		desc := mutedStyle.Render(s.Description)

		line := fmt.Sprintf("  %s  %s  %s", name, value, desc)

		if i == m.settingsCursor {
			// Highlight selected row
			line = lipgloss.NewStyle().
				Foreground(capDark).
				Background(capBlue).
				Bold(true).
				Render(fmt.Sprintf("▶ %s  %s  %s", nameStyle.Render(s.Name), valueStr, s.Description))
		}

		lines = append(lines, line)
	}

	// Padding
	for len(lines) < 20 {
		lines = append(lines, "")
	}

	// Config file path
	configPathStr, _ := settings.ConfigPath()
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("  Config: %s", configPathStr)))

	// Help
	lines = append(lines, "")
	helpLine := helpStyle.Render("  ") +
		helpKeyStyle.Render("←/→") + helpStyle.Render(" category  ") +
		helpKeyStyle.Render("↑/↓") + helpStyle.Render(" select  ") +
		helpKeyStyle.Render("enter") + helpStyle.Render(" toggle  ") +
		helpKeyStyle.Render("esc") + helpStyle.Render(" close  ") +
		helpKeyStyle.Render("q") + helpStyle.Render(" quit")
	lines = append(lines, helpLine)

	return strings.Join(lines, "\n")
}


func (m Model) handleDebugInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	categories := debug.GetCategories()
	
	// Filter actions for current category
	var currentActions []debug.Action
	for _, a := range m.debugActions {
		if a.Category == categories[m.debugCategory] {
			currentActions = append(currentActions, a)
		}
	}

	switch msg.String() {
	case "ctrl+c":
		m.gracefulShutdown()
		return m, tea.Quit

	case "q":
		if m.confirmQuit && time.Since(m.quitTime) < 3*time.Second {
			m.gracefulShutdown()
			return m, tea.Quit
		}
		m.confirmQuit = true
		m.quitTime = time.Now()
		m.setStatus("Press q again to quit")
		return m, nil

	case "esc", "d":
		m.showDebug = false
		m.debugConfirm = false
		return m, nil

	case "up", "k":
		if m.debugCursor > 0 {
			m.debugCursor--
			m.debugConfirm = false
		}
		return m, nil

	case "down", "j":
		if m.debugCursor < len(currentActions)-1 {
			m.debugCursor++
			m.debugConfirm = false
		}
		return m, nil

	case "left", "h":
		if m.debugCategory > 0 {
			m.debugCategory--
			m.debugCursor = 0
			m.debugConfirm = false
		}
		return m, nil

	case "right", "l":
		if m.debugCategory < len(categories)-1 {
			m.debugCategory++
			m.debugCursor = 0
			m.debugConfirm = false
		}
		return m, nil

	case "enter", " ":
		if len(currentActions) == 0 {
			return m, nil
		}
		
		action := currentActions[m.debugCursor]
		
		// Dangerous actions require confirmation
		if action.Dangerous && !m.debugConfirm {
			m.debugConfirm = true
			m.setStatus("⚠ Press enter again to confirm: " + action.Name)
			return m, nil
		}
		
		// Run the action
		m.debugConfirm = false
		result := debug.RunAction(action.ID)
		m.debugResult = &result
		m.debugResultTime = time.Now()
		
		if result.Success {
			m.setStatus("✓ " + result.Message)
		} else {
			m.setStatus("✗ " + result.Message)
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) renderDebug() string {
	categories := debug.GetCategories()

	// Title
	title := lipgloss.NewStyle().
		Foreground(capBlue).
		Bold(true).
		Render("  🔧 Debug & Cleanup Tools")

	var lines []string
	lines = append(lines, "")
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("  Common fixes for build issues, cache problems, and device troubles"))
	lines = append(lines, "")

	// Category tabs
	var tabs []string
	for i, cat := range categories {
		tabText := fmt.Sprintf(" %s ", cat)
		if i == m.debugCategory {
			tabs = append(tabs, activeTabStyle.Render(tabText))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(tabText))
		}
	}
	lines = append(lines, "  "+strings.Join(tabs, " "))
	lines = append(lines, "")

	// Filter actions for current category
	var currentActions []debug.Action
	for _, a := range m.debugActions {
		if a.Category == categories[m.debugCategory] {
			currentActions = append(currentActions, a)
		}
	}

	// Actions list
	for i, action := range currentActions {
		isSelected := i == m.debugCursor

		// Warning indicator for dangerous actions
		var dangerIcon string
		if action.Dangerous {
			dangerIcon = lipgloss.NewStyle().Foreground(warnColor).Render("⚠ ")
		} else {
			dangerIcon = "  "
		}

		name := action.Name
		desc := action.Description

		if isSelected {
			// Highlight selected
			arrow := lipgloss.NewStyle().Foreground(capBlue).Bold(true).Render("▶")
			nameStyled := lipgloss.NewStyle().Foreground(capCyan).Bold(true).Render(name)
			
			lines = append(lines, fmt.Sprintf(" %s%s%s", arrow, dangerIcon, nameStyled))
			lines = append(lines, fmt.Sprintf("      %s", mutedStyle.Render(desc)))
			
			// Show confirmation prompt for dangerous actions
			if action.Dangerous && m.debugConfirm {
				lines = append(lines, fmt.Sprintf("      %s", lipgloss.NewStyle().Foreground(warnColor).Bold(true).Render("Press enter again to confirm")))
			}
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(capLight).Render(name)
			lines = append(lines, fmt.Sprintf("  %s%s", dangerIcon, nameStyled))
		}
	}

	if len(currentActions) == 0 {
		lines = append(lines, mutedStyle.Render("  No actions available for this category"))
	}

	// Padding
	for len(lines) < 18 {
		lines = append(lines, "")
	}

	// Show last result if recent
	if m.debugResult != nil && time.Since(m.debugResultTime) < 10*time.Second {
		lines = append(lines, "")
		if m.debugResult.Success {
			lines = append(lines, "  "+successStyle.Render("✓ "+m.debugResult.Message))
		} else {
			lines = append(lines, "  "+errorStyle.Render("✗ "+m.debugResult.Message))
		}
		if m.debugResult.Details != "" {
			// Truncate details
			details := m.debugResult.Details
			if len(details) > 60 {
				details = details[:57] + "..."
			}
			lines = append(lines, "    "+mutedStyle.Render(details))
		}
	}

	// Help
	lines = append(lines, "")
	helpLine := helpStyle.Render("  ") +
		helpKeyStyle.Render("←/→") + helpStyle.Render(" category  ") +
		helpKeyStyle.Render("↑/↓") + helpStyle.Render(" select  ") +
		helpKeyStyle.Render("enter") + helpStyle.Render(" run  ") +
		helpKeyStyle.Render("esc") + helpStyle.Render(" close  ") +
		helpKeyStyle.Render("q") + helpStyle.Render(" quit")
	lines = append(lines, helpLine)
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("  ⚠ = requires confirmation"))

	return strings.Join(lines, "\n")
}

func (m Model) handlePluginsInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pluginManager == nil {
		// No plugin manager, just close
		m.showPlugins = false
		return m, nil
	}

	allPlugins := plugin.All()

	switch msg.String() {
	case "ctrl+c":
		m.gracefulShutdown()
		return m, tea.Quit

	case "q":
		if m.confirmQuit && time.Since(m.quitTime) < 3*time.Second {
			m.gracefulShutdown()
			return m, tea.Quit
		}
		m.confirmQuit = true
		m.quitTime = time.Now()
		m.setStatus("Press q again to quit")
		return m, nil

	case "esc", "P":
		m.showPlugins = false
		return m, nil

	case "up", "k":
		if m.pluginCursor > 0 {
			m.pluginCursor--
		}
		return m, nil

	case "down", "j":
		if m.pluginCursor < len(allPlugins)-1 {
			m.pluginCursor++
		}
		return m, nil

	case "enter", " ":
		// Toggle plugin running state
		if len(allPlugins) > 0 && m.pluginCursor < len(allPlugins) {
			p := allPlugins[m.pluginCursor]
			if p.IsRunning() {
				if err := p.Stop(); err != nil {
					m.setStatus(fmt.Sprintf("Failed to stop %s: %v", p.Name(), err))
				} else {
					m.setStatus(fmt.Sprintf("Stopped %s", p.Name()))
				}
			} else {
				if err := p.Start(); err != nil {
					m.setStatus(fmt.Sprintf("Failed to start %s: %v", p.Name(), err))
				} else {
					m.setStatus(fmt.Sprintf("Started %s", p.Name()))
				}
			}
		}
		return m, nil

	case "e":
		// Toggle enabled state
		if len(allPlugins) > 0 && m.pluginCursor < len(allPlugins) {
			p := allPlugins[m.pluginCursor]
			enabled := m.pluginManager.IsEnabled(p.ID())
			if err := m.pluginManager.SetEnabled(p.ID(), !enabled); err != nil {
				m.setStatus(fmt.Sprintf("Failed to toggle %s: %v", p.Name(), err))
			} else {
				if enabled {
					m.setStatus(fmt.Sprintf("Disabled %s", p.Name()))
				} else {
					m.setStatus(fmt.Sprintf("Enabled %s", p.Name()))
				}
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) renderPlugins() string {
	// Title
	title := lipgloss.NewStyle().
		Foreground(capBlue).
		Bold(true).
		Render("  🔌 Plugins")

	var lines []string
	lines = append(lines, "")
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("  Manage lazycap plugins and extensions"))
	lines = append(lines, "")

	if m.pluginManager == nil {
		lines = append(lines, "  "+errorStyle.Render("Plugin system not available"))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("  Press "+helpKeyStyle.Render("esc")+" to close"))
		return strings.Join(lines, "\n")
	}

	allPlugins := plugin.All()

	if len(allPlugins) == 0 {
		lines = append(lines, mutedStyle.Render("  No plugins installed"))
	} else {
		for i, p := range allPlugins {
			isSelected := i == m.pluginCursor
			isEnabled := m.pluginManager.IsEnabled(p.ID())
			isRunning := p.IsRunning()

			// Status indicator
			var status string
			if isRunning {
				status = successStyle.Render("● running")
			} else if isEnabled {
				status = mutedStyle.Render("○ stopped")
			} else {
				status = mutedStyle.Render("○ disabled")
			}

			// Plugin info
			name := p.Name()
			version := p.Version()
			desc := p.Description()

			if isSelected {
				// Highlight selected
				arrow := lipgloss.NewStyle().Foreground(capBlue).Bold(true).Render("▶")
				nameStyled := lipgloss.NewStyle().Foreground(capCyan).Bold(true).Render(name)

				lines = append(lines, fmt.Sprintf(" %s %s  %s  %s", arrow, nameStyled, mutedStyle.Render("v"+version), status))
				lines = append(lines, fmt.Sprintf("      %s", mutedStyle.Render(desc)))

				// Show status line if plugin has one
				if statusLine := p.GetStatusLine(); statusLine != "" {
					lines = append(lines, fmt.Sprintf("      %s", lipgloss.NewStyle().Foreground(capCyan).Render(statusLine)))
				}

				// Show available commands
				commands := p.GetCommands()
				if len(commands) > 0 {
					var cmdStrs []string
					for _, cmd := range commands {
						cmdStrs = append(cmdStrs, fmt.Sprintf("%s=%s", cmd.Key, cmd.Name))
					}
					lines = append(lines, fmt.Sprintf("      %s", mutedStyle.Render("Keys: "+strings.Join(cmdStrs, ", "))))
				}
				lines = append(lines, "")
			} else {
				nameStyled := lipgloss.NewStyle().Foreground(capLight).Render(name)
				lines = append(lines, fmt.Sprintf("   %s  %s  %s", nameStyled, mutedStyle.Render("v"+version), status))
			}
		}
	}

	// Padding
	for len(lines) < 18 {
		lines = append(lines, "")
	}

	// Help
	lines = append(lines, "")
	helpLine := helpStyle.Render("  ") +
		helpKeyStyle.Render("↑/↓") + helpStyle.Render(" select  ") +
		helpKeyStyle.Render("enter") + helpStyle.Render(" start/stop  ") +
		helpKeyStyle.Render("e") + helpStyle.Render(" enable/disable  ") +
		helpKeyStyle.Render("esc") + helpStyle.Render(" close  ") +
		helpKeyStyle.Render("q") + helpStyle.Render(" quit")
	lines = append(lines, helpLine)

	return strings.Join(lines, "\n")
}
