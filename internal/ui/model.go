package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/icarus-itcs/lazycap/internal/cap"
	"github.com/icarus-itcs/lazycap/internal/debug"
	"github.com/icarus-itcs/lazycap/internal/device"
	"github.com/icarus-itcs/lazycap/internal/plugin"
	"github.com/icarus-itcs/lazycap/internal/preflight"
	"github.com/icarus-itcs/lazycap/internal/settings"
	"github.com/icarus-itcs/lazycap/internal/update"
)

// Status indicator styles
var (
	statusOnlineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00"))
	statusOfflineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
)

// Comprehensive ANSI escape sequence regex - handles:
// - CSI sequences: \x1b[...X (including private modes like ?25l, ?25h)
// - OSC sequences: \x1b]...BEL or \x1b]...ST
// - DCS/PM/APC sequences
// - Simple escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[PX^_].*?\x1b\\|\x1b.`)

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
	projects    []*cap.Project // All discovered projects (for monorepo support)
	upgradeInfo *cap.UpgradeInfo

	// Version and updates
	version    string
	updateInfo *update.Info
	updating   bool

	// Project selector (for monorepos with multiple projects)
	showProjectSelector bool
	projectCursor       int

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
	settings            *settings.Settings
	showSettings        bool
	settingsCursor      int
	settingsCategory    int
	editingSettingKey   string // Key of setting being edited (empty = not editing)
	editingSettingValue string // Current text value being edited
	editingSettingType  string // Type of setting being edited (string/int)

	// Plugins
	pluginManager       *plugin.Manager
	pluginContext       *plugin.AppContext
	showPlugins         bool
	pluginCursor        int
	showPluginSettings  bool
	pluginSettingCursor int

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

	// Memory tracking
	memoryUsage uint64 // Total memory in bytes (lazycap + child processes)
}

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Tab        key.Binding
	Run        key.Binding
	Sync       key.Binding
	Build      key.Binding
	Open       key.Binding
	Kill       key.Binding
	Refresh    key.Binding
	Upgrade    key.Binding
	SelfUpdate key.Binding
	Help       key.Binding
	Quit       key.Binding
	Left       key.Binding
	Right      key.Binding
	Copy       key.Binding
	Export     key.Binding
	Preflight  key.Binding
	Settings   key.Binding
	Debug      key.Binding
	Plugins    key.Binding
	Enter      key.Binding
	Workspace  key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Tab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
		Run:        key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run")),
		Sync:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		Build:      key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "build")),
		Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open IDE")),
		Kill:       key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "kill")),
		Refresh:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
		Upgrade:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "upgrade")),
		SelfUpdate: key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "update lazycap")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Left:       key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←", "prev tab")),
		Right:      key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→", "next tab")),
		Copy:       key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy logs")),
		Export:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "export logs")),
		Preflight:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "preflight")),
		Settings:   key.NewBinding(key.WithKeys(","), key.WithHelp(",", "settings")),
		Debug:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "debug")),
		Plugins:    key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "plugins")),
		Enter:      key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter", "toggle")),
		Workspace:  key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "projects")),
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
func NewModel(project *cap.Project, version string) Model {
	return NewModelWithPlugins(project, nil, nil, version)
}

// NewModelWithProjects creates a new model with multiple discovered projects
func NewModelWithProjects(projects []*cap.Project, pluginMgr *plugin.Manager, appCtx *plugin.AppContext, version string) Model {
	var activeProject *cap.Project
	showSelector := false

	if len(projects) == 1 {
		activeProject = projects[0]
	} else if len(projects) > 1 {
		// Multiple projects found - show selector
		activeProject = projects[0]
		showSelector = true
	}

	m := NewModelWithPlugins(activeProject, pluginMgr, appCtx, version)
	m.projects = projects
	m.showProjectSelector = showSelector
	return m
}

// NewModelWithPlugins creates a new model with plugin support
func NewModelWithPlugins(project *cap.Project, pluginMgr *plugin.Manager, appCtx *plugin.AppContext, version string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(capBlue)

	// Run preflight checks
	preflightResults := preflight.Run()

	// Load settings
	userSettings, _ := settings.Load()

	m := Model{
		project:          project,
		version:          version,
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
						_ = p.Cmd.Process.Kill()
						return nil
					}
				}
				return fmt.Errorf("process %s not found or not running", processID)
			},
			// GetProcesses
			func() []plugin.ProcessInfo {
				infos := make([]plugin.ProcessInfo, 0, len(m.processes))
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
						status = "canceled"
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
func NewDemoModel(project *cap.Project, pluginMgr *plugin.Manager, appCtx *plugin.AppContext, version string) Model {
	m := NewModelWithPlugins(project, pluginMgr, appCtx, version)
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
			Command:   "npx cap run ios -l --target iphone-15-pro-max",
			Status:    ProcessRunning,
			StartTime: now.Add(-5 * time.Minute),
			Logs: []string{
				"[14:27:12] $ npx cap run ios -l --target iphone-15-pro-max",
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
			Command:   "npx cap run android -l --target pixel-8-pro",
			Status:    ProcessRunning,
			StartTime: now.Add(-3 * time.Minute),
			Logs: []string{
				"[14:29:12] $ npx cap run android -l --target pixel-8-pro",
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

type pluginLogMsg struct {
	pluginID string
	message  string
	time     time.Time
}

type updateCheckedMsg struct {
	info *update.Info
	err  error
}

type selfUpdateMsg struct {
	err error
}

type memoryTickMsg struct{}

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

func checkForUpdate(version string) tea.Cmd {
	return func() tea.Msg {
		info, err := update.Check(version)
		return updateCheckedMsg{info: info, err: err}
	}
}

func (m *Model) doSelfUpdate() tea.Cmd {
	return func() tea.Msg {
		if m.updateInfo == nil || !m.updateInfo.UpdateAvailable {
			return selfUpdateMsg{err: fmt.Errorf("no update available")}
		}
		err := update.SelfUpdate(m.updateInfo)
		return selfUpdateMsg{err: err}
	}
}

// scheduleMemoryUpdate schedules the next memory update tick
func scheduleMemoryUpdate() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return memoryTickMsg{}
	})
}

// calculateMemoryUsage computes total memory usage (lazycap + child processes + plugins)
func (m *Model) calculateMemoryUsage() uint64 {
	var totalBytes uint64

	// Get lazycap's own memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	totalBytes += memStats.Alloc

	// Get memory usage of child processes (capacitor runs, builds, syncs)
	for _, p := range m.processes {
		if p.Status == ProcessRunning && p.Cmd != nil && p.Cmd.Process != nil {
			pid := p.Cmd.Process.Pid
			childMem := getProcessMemory(pid)
			totalBytes += childMem
		}
	}

	// Get memory usage of plugin processes (Firebase emulators, etc.)
	for _, p := range plugin.All() {
		if p.IsRunning() {
			for _, pid := range p.GetProcessIDs() {
				childMem := getProcessMemory(pid)
				totalBytes += childMem
			}
		}
	}

	return totalBytes
}

// getProcessMemory gets memory usage for a process and all its descendants by PID
func getProcessMemory(pid int) uint64 {
	return getProcessTreeMemory(pid, make(map[int]bool))
}

// getProcessTreeMemory recursively gets memory for a process and all its children
func getProcessTreeMemory(pid int, visited map[int]bool) uint64 {
	if visited[pid] {
		return 0
	}
	visited[pid] = true

	var totalMem uint64

	// Get this process's memory
	cmd := exec.Command("ps", "-o", "rss=", "-p", fmt.Sprintf("%d", pid))
	output, err := cmd.Output()
	if err == nil {
		var rssKB uint64
		_, _ = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &rssKB)
		totalMem += rssKB * 1024
	}

	// Find child processes using pgrep
	pgrepCmd := exec.Command("pgrep", "-P", fmt.Sprintf("%d", pid))
	pgrepOutput, err := pgrepCmd.Output()
	if err == nil {
		childPids := strings.Fields(strings.TrimSpace(string(pgrepOutput)))
		for _, childPidStr := range childPids {
			var childPid int
			if _, err := fmt.Sscanf(childPidStr, "%d", &childPid); err == nil && childPid > 0 {
				totalMem += getProcessTreeMemory(childPid, visited)
			}
		}
	}

	return totalMem
}

// formatBytes formats bytes as human-readable string (KB, MB, GB)
func formatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
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

// removeProcess removes a process from the list by ID
func (m *Model) removeProcess(processID string) {
	for i, p := range m.processes {
		if p.ID == processID {
			// Close the output channel if it exists
			if ch, ok := m.outputChans[processID]; ok {
				close(ch)
				delete(m.outputChans, processID)
			}
			// Remove from slice
			m.processes = append(m.processes[:i], m.processes[i+1:]...)
			// Adjust selected index if needed
			if m.selectedProcess >= len(m.processes) && m.selectedProcess > 0 {
				m.selectedProcess--
			}
			// Update viewport
			m.updateLogViewport()
			return
		}
	}
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

// listenForPluginLogs creates a command that waits for plugin logs from the channel
func listenForPluginLogs(ctx *plugin.AppContext) tea.Cmd {
	if ctx == nil {
		return nil
	}
	return func() tea.Msg {
		logChan := ctx.GetLogChannel()
		if logChan == nil {
			return nil
		}
		entry, ok := <-logChan
		if !ok {
			return nil
		}
		return pluginLogMsg{
			pluginID: entry.PluginID,
			message:  entry.Message,
			time:     entry.Time,
		}
	}
}

// gracefulShutdown kills all running processes and stops plugins
func (m *Model) gracefulShutdown() {
	// Stop all running processes
	for _, p := range m.processes {
		if p.Status == ProcessRunning && p.Cmd != nil && p.Cmd.Process != nil {
			_ = p.Cmd.Process.Kill()
		}
	}

	// Record which plugins were running before stopping them
	if m.pluginManager != nil {
		for _, p := range plugin.All() {
			_ = m.pluginManager.SetRunning(p.ID(), p.IsRunning())
		}
		_ = m.pluginManager.StopAll()
	}
}

// Init starts the app
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		loadDevices,
		checkUpgrade,
		checkForUpdate(m.version),
		m.spinner.Tick,
		setTerminalTitle(m.getTerminalTitle()),
		scheduleMemoryUpdate(),
	}
	// Start listening for plugin logs if plugin context is available
	if m.pluginContext != nil {
		cmds = append(cmds, listenForPluginLogs(m.pluginContext))
	}
	return tea.Batch(cmds...)
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

		// Handle project selector input
		if m.showProjectSelector {
			return m.handleProjectSelectorInput(msg)
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
			m.showProjectSelector = false
			m.pluginCursor = 0
			return m, nil

		case key.Matches(msg, m.keys.Workspace):
			// Only show if there are multiple projects
			if len(m.projects) > 1 {
				m.showProjectSelector = !m.showProjectSelector
				m.showHelp = false
				m.showPreflight = false
				m.showSettings = false
				m.showDebug = false
				m.showPlugins = false
				// Set cursor to current project
				for i, p := range m.projects {
					if m.project != nil && p.RootDir == m.project.RootDir {
						m.projectCursor = i
						break
					}
				}
			} else {
				m.setStatus("Only one project in workspace")
			}
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
		case key.Matches(msg, m.keys.SelfUpdate):
			if m.updateInfo != nil && m.updateInfo.UpdateAvailable && !m.updating {
				m.updating = true
				m.setStatus(fmt.Sprintf("Updating to v%s...", m.updateInfo.LatestVersion))
				return m, m.doSelfUpdate()
			} else if m.updateInfo == nil {
				m.setStatus("Checking for updates...")
				return m, checkForUpdate(m.version)
			} else if !m.updateInfo.UpdateAvailable {
				m.setStatus(fmt.Sprintf("Already on latest version (v%s)", update.VersionString(m.version)))
			}
			return m, nil
		case key.Matches(msg, m.keys.Kill):
			p := m.getSelectedProcess()
			if p != nil {
				// Kill the process if running
				if p.Status == ProcessRunning && p.Cmd != nil && p.Cmd.Process != nil {
					_ = p.Cmd.Process.Kill()
				}
				// Remove the process from the list (remove the tab)
				m.removeProcess(p.ID)
				m.setStatus(fmt.Sprintf("Removed %s", p.Name))
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

	case updateCheckedMsg:
		if msg.err == nil && msg.info != nil {
			m.updateInfo = msg.info
		}

	case selfUpdateMsg:
		m.updating = false
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Update failed: %s", msg.err))
		} else {
			m.setStatus("Update complete! Restart lazycap to use the new version.")
		}

	case memoryTickMsg:
		m.memoryUsage = m.calculateMemoryUsage()
		cmds = append(cmds, scheduleMemoryUpdate())

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

	case pluginLogMsg:
		// Find or create a process tab for this plugin
		var pluginProcess *Process
		processID := "plugin-" + msg.pluginID
		for _, p := range m.processes {
			if p.ID == processID {
				pluginProcess = p
				break
			}
		}
		if pluginProcess == nil {
			// Create a new process tab for this plugin
			pluginName := msg.pluginID
			// Capitalize first letter for display
			if len(pluginName) > 0 {
				pluginName = strings.ToUpper(pluginName[:1]) + pluginName[1:]
			}
			pluginProcess = &Process{
				ID:        processID,
				Name:      pluginName,
				Command:   "plugin:" + msg.pluginID,
				Status:    ProcessRunning,
				StartTime: time.Now(),
				Logs:      []string{},
			}
			m.processes = append(m.processes, pluginProcess)
		}
		// Add the log line with timestamp
		ts := msg.time.Format("15:04:05")
		pluginProcess.AddLog(fmt.Sprintf("[%s] %s", ts, msg.message))
		// Check if this is a "stopped" message to mark process as finished
		lowerMsg := strings.ToLower(msg.message)
		if strings.Contains(lowerMsg, "stopped") || strings.Contains(lowerMsg, "shutdown") {
			pluginProcess.Status = ProcessSuccess
			pluginProcess.EndTime = time.Now()
		}
		// Check for errors
		if strings.HasPrefix(msg.message, "ERROR:") || strings.Contains(lowerMsg, "error:") {
			pluginProcess.Status = ProcessFailed
		}
		// Update viewport if this plugin's logs are being viewed
		if m.getSelectedProcess() == pluginProcess {
			m.updateLogViewport()
		}
		// Continue listening for more plugin logs
		cmds = append(cmds, listenForPluginLogs(m.pluginContext))
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
	paneWidth := m.width - 36 - 6
	paneHeight := m.height - 9 // Account for header + status bar
	if paneHeight < 5 {
		paneHeight = 5
	}
	// Match renderRight() viewport sizing: account for border (2) + tab bar (1)
	m.logViewport.Width = paneWidth - 4
	m.logViewport.Height = paneHeight - 3
}

func (m *Model) updateLogViewport() {
	p := m.getSelectedProcess()
	if p == nil {
		m.logViewport.SetContent(logEmptyStyle.Render("\n  Run a command to see output here..."))
		return
	}
	// Pre-wrap content to viewport width so line count matches visual height
	content := strings.Join(p.Logs, "\n")
	if m.logViewport.Width > 0 {
		content = lipgloss.NewStyle().Width(m.logViewport.Width).Render(content)
	}
	m.logViewport.SetContent(content)
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

// getProjectDir returns the current project's root directory, or empty string if not set
func (m *Model) getProjectDir() string {
	if m.project != nil {
		return m.project.RootDir
	}
	return ""
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
			if m.project != nil && m.project.HasIOS {
				return m.startOpenCommand("ios")
			} else if m.project != nil && m.project.HasAndroid {
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
		args = append(args, "-l")

		// Get port and host from settings
		port := m.settings.GetInt("liveReloadPort")
		host := m.settings.GetString("externalHost")

		if port > 0 {
			args = append(args, "--port", fmt.Sprintf("%d", port))
		}
		if host != "" {
			args = append(args, "--host", host)
		}
		name = shortName + " (live)"

		// For live reload, we need to start the dev server AND the cap run command
		// Start dev server first, then run cap command
		p := m.createProcess(name, "vite + cap run")
		projectDir := m.getProjectDir()

		// Kill any existing process on the port first
		if port > 0 {
			if cap.KillPort(port) {
				p.AddLog(fmt.Sprintf("Killed existing process on port %d", port))
			}
		}

		return func() tea.Msg {
			ch := make(chan string, 100)

			// Build the combined command that starts vite in background and then runs cap
			viteHost := "0.0.0.0" // Bind to all interfaces for external access
			if host != "" {
				viteHost = host
			}

			// Command: start vite in background, wait for it, then run cap
			// Use /bin/sleep for macOS compatibility
			cmdStr := fmt.Sprintf(
				"npx vite --host %s --port %d --strictPort & VITE_PID=$!; /bin/sleep 3; npx %s; kill $VITE_PID 2>/dev/null",
				viteHost,
				port,
				strings.Join(args, " "),
			)

			shell := os.Getenv("SHELL")
			if shell == "" {
				shell = "/bin/zsh"
			}

			shellCmd := fmt.Sprintf("source ~/.zshrc 2>/dev/null; source ~/.zprofile 2>/dev/null; %s", cmdStr)
			cmd := exec.Command(shell, "-c", shellCmd)
			cmd.Env = os.Environ()

			if projectDir != "" {
				cmd.Dir = projectDir
			} else if cwd, err := os.Getwd(); err == nil {
				cmd.Dir = cwd
			}

			return runCmdWithPipes(p.ID, cmd, ch)
		}
	}

	p := m.createProcess(name, "npx "+strings.Join(args, " "))
	return runCmd(p.ID, m.getProjectDir(), "npx", args...)
}

func (m *Model) startSyncCommand(platform string) tea.Cmd {
	args := []string{"cap", "sync"}
	if platform != "" {
		args = append(args, platform)
	}
	p := m.createProcess("Sync", "npx "+strings.Join(args, " "))
	return runCmd(p.ID, m.getProjectDir(), "npx", args...)
}

func (m *Model) startBuildCommand() tea.Cmd {
	p := m.createProcess("Build", "npm run build")
	return runCmd(p.ID, m.getProjectDir(), "npm", "run", "build")
}

func (m *Model) startOpenCommand(platform string) tea.Cmd {
	p := m.createProcess("Open", "npx cap open "+platform)
	return runCmd(p.ID, m.getProjectDir(), "npx", "cap", "open", platform)
}

func (m *Model) startUpgrade() tea.Cmd {
	p := m.createProcess("Upgrade", "npm install @capacitor/core@latest @capacitor/cli@latest")
	return runCmd(p.ID, m.getProjectDir(), "npm", "install", "@capacitor/core@latest", "@capacitor/cli@latest")
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
				_ = cap.OpenBrowser(url, browserPath)
			}
		}()
	}

	// Run the command directly - let the dev server use its own defaults
	// The command should be the full command like "npm run dev" or "npx vite"
	return runWebCmd(p.ID, m.getProjectDir(), command, port, host)
}

func runCmd(processID, workDir, name string, args ...string) tea.Cmd {
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

		// Set working directory - use project dir if provided, otherwise cwd
		if workDir != "" {
			cmd.Dir = workDir
		} else if cwd, err := os.Getwd(); err == nil {
			cmd.Dir = cwd
		}

		return runCmdWithPipes(processID, cmd, ch)
	}
}

// runWebCmd runs a web dev server command with proper port/host handling
func runWebCmd(processID, workDir, command string, port int, host string) tea.Cmd {
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

		// Set working directory - use project dir if provided, otherwise cwd
		if workDir != "" {
			cmd.Dir = workDir
		} else if cwd, err := os.Getwd(); err == nil {
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
		_ = cmd.Wait()
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

	if m.showProjectSelector {
		return m.renderProjectSelector()
	}

	// Build the view
	left := m.renderLeft()
	right := m.renderRight()

	main := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		m.renderHeader(),
		m.renderStatusBar(),
		"",
		main,
		"",
		m.renderHelp(),
	)
}

func (m *Model) renderHeader() string {
	// Logo with version
	versionStr := update.VersionString(m.version)
	logo := "  " + LogoCompact() + " " + mutedStyle.Render("v"+versionStr)

	// Project info (with nil check)
	projectName := "No project"
	if m.project != nil {
		projectName = m.project.Name
	}
	project := projectStyle.Render(projectName)

	// Workspace indicator (when multiple projects)
	var workspaceHint string
	if len(m.projects) > 1 {
		workspaceHint = mutedStyle.Render(fmt.Sprintf(" (1/%d)", len(m.projects)))
		// Find actual position
		for i, p := range m.projects {
			if m.project != nil && p.RootDir == m.project.RootDir {
				workspaceHint = mutedStyle.Render(fmt.Sprintf(" (%d/%d W=switch)", i+1, len(m.projects)))
				break
			}
		}
	}

	// Platforms
	var platforms []string
	if m.project != nil && m.project.HasIOS {
		platforms = append(platforms, iosBadge.Render("iOS"))
	}
	if m.project != nil && m.project.HasAndroid {
		platforms = append(platforms, androidBadge.Render("Android"))
	}
	platformStr := strings.Join(platforms, " ")

	// Capacitor upgrade notice
	var upgrade string
	if m.upgradeInfo != nil && m.upgradeInfo.HasUpgrade {
		upgrade = upgradeStyle.Render(fmt.Sprintf("  ↑ v%s available", m.upgradeInfo.LatestVersion))
	}

	// lazycap update notice
	var lazycapUpdate string
	if m.updateInfo != nil && m.updateInfo.UpdateAvailable {
		lazycapUpdate = "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#00d4ff")).Render(
			fmt.Sprintf("↑ lazycap v%s (U=update)", m.updateInfo.LatestVersion))
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

	// Status message (show for 3 seconds)
	var statusMsg string
	if m.statusMessage != "" && time.Since(m.statusTime) < 3*time.Second {
		statusMsg = "  " + successStyle.Render(m.statusMessage)
	}

	headerLine := fmt.Sprintf("%s  %s%s  %s%s%s%s%s", logo, project, workspaceHint, platformStr, upgrade, lazycapUpdate, preflightIndicator, statusMsg)

	return headerLine
}

// renderStatusBar creates a status bar showing service states
func (m *Model) renderStatusBar() string {
	var statusItems []string

	// Device count
	onlineCount := 0
	for _, d := range m.devices {
		if d.Online {
			onlineCount++
		}
	}
	if m.loading {
		statusItems = append(statusItems, m.spinner.View()+" Devices...")
	} else {
		deviceStatus := fmt.Sprintf("%d/%d", onlineCount, len(m.devices))
		if onlineCount > 0 {
			statusItems = append(statusItems, statusOnlineStyle.Render("●")+" Devices "+mutedStyle.Render(deviceStatus))
		} else {
			statusItems = append(statusItems, statusOfflineStyle.Render("○")+" Devices "+mutedStyle.Render(deviceStatus))
		}
	}

	// Process count
	runningCount := 0
	for _, p := range m.processes {
		if p.Status == ProcessRunning {
			runningCount++
		}
	}
	if runningCount > 0 {
		statusItems = append(statusItems, m.spinner.View()+" "+fmt.Sprintf("%d running", runningCount))
	} else if len(m.processes) > 0 {
		statusItems = append(statusItems, statusOfflineStyle.Render("○")+" "+mutedStyle.Render("idle"))
	}

	// Plugin statuses with more detail
	if m.pluginManager != nil {
		allPlugins := plugin.All()

		// MCP Server status with tool count
		mcpPluginFound := false
		for _, p := range allPlugins {
			if p.ID() == "mcp-server" {
				mcpPluginFound = true
				enabledCount, totalCount := m.settings.GetMCPToolCount()
				toolInfo := fmt.Sprintf("(%d/%d)", enabledCount, totalCount)

				if p.IsRunning() {
					statusLine := p.GetStatusLine()
					if statusLine != "" {
						statusItems = append(statusItems, statusOnlineStyle.Render("●")+" "+statusLine+" "+mutedStyle.Render(toolInfo))
					} else {
						statusItems = append(statusItems, statusOnlineStyle.Render("●")+" MCP "+mutedStyle.Render(toolInfo))
					}
				} else if m.pluginManager.IsEnabled(p.ID()) {
					statusItems = append(statusItems, statusOfflineStyle.Render("○")+" "+mutedStyle.Render("MCP off")+" "+mutedStyle.Render(toolInfo))
				}
				break
			}
		}
		// Show MCP status even without the plugin (for CLI-only MCP usage)
		if !mcpPluginFound && m.settings.MCPEnabled {
			enabledCount, totalCount := m.settings.GetMCPToolCount()
			toolInfo := fmt.Sprintf("(%d/%d tools)", enabledCount, totalCount)
			statusItems = append(statusItems, mutedStyle.Render("MCP CLI")+" "+mutedStyle.Render(toolInfo))
		}

		// Firebase status
		for _, p := range allPlugins {
			if p.ID() == "firebase-emulator" {
				if p.IsRunning() {
					statusLine := p.GetStatusLine()
					if statusLine != "" {
						statusItems = append(statusItems, statusOnlineStyle.Render("●")+" "+statusLine)
					} else {
						statusItems = append(statusItems, statusOnlineStyle.Render("●")+" Firebase")
					}
				} else {
					// Check if firebase.json exists to show as available but off
					if m.project != nil {
						firebasePath := filepath.Join(m.project.RootDir, "firebase.json")
						if _, err := os.Stat(firebasePath); err == nil {
							statusItems = append(statusItems, statusOfflineStyle.Render("○")+" "+mutedStyle.Render("Firebase off"))
						}
					}
				}
				break
			}
		}

		// Other running plugins
		for _, p := range allPlugins {
			if p.ID() == "mcp-server" || p.ID() == "firebase-emulator" {
				continue // Already handled above
			}
			if p.IsRunning() {
				statusLine := p.GetStatusLine()
				if statusLine != "" {
					statusItems = append(statusItems, statusOnlineStyle.Render("●")+" "+statusLine)
				}
			}
		}
	}

	// Memory usage
	if m.memoryUsage > 0 {
		memStr := formatBytes(m.memoryUsage)
		statusItems = append(statusItems, mutedStyle.Render("Mem "+memStr))
	}

	if len(statusItems) == 0 {
		return ""
	}

	// Join with separator
	statusBar := "  " + strings.Join(statusItems, "  "+mutedStyle.Render("│")+"  ")
	return statusBar
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

	paneHeight := m.height - 9 // Account for header + status bar
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
	paneHeight := m.height - 9 // Account for header + status bar
	if paneHeight < 5 {
		paneHeight = 5
	}
	if paneWidth < 20 {
		paneWidth = 20
	}

	m.logViewport.Width = paneWidth - 4
	m.logViewport.Height = paneHeight - 3 // Account for border (2) + tab bar (1)

	// Show welcome screen when no processes
	if len(m.processes) == 0 {
		return m.renderWelcome(paneWidth, paneHeight)
	}

	// Process tabs - show max 3 at a time, centered on selected
	const maxTabs = 3

	startIdx := 0
	endIdx := len(m.processes)

	if len(m.processes) > maxTabs {
		// Center around selected tab
		startIdx = m.selectedProcess - maxTabs/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + maxTabs
		if endIdx > len(m.processes) {
			endIdx = len(m.processes)
			startIdx = endIdx - maxTabs
		}
	}

	var tabParts []string

	// Left overflow indicator
	if startIdx > 0 {
		tabParts = append(tabParts, mutedStyle.Render(fmt.Sprintf("◀%d", startIdx)))
	}

	for i := startIdx; i < endIdx; i++ {
		p := m.processes[i]
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
		if len(name) > 14 {
			name = name[:12] + ".."
		}

		// Simple format: selected gets highlight, others are muted
		if i == m.selectedProcess {
			tabParts = append(tabParts, fmt.Sprintf("%s [%s]", icon, lipgloss.NewStyle().Foreground(capBlue).Bold(true).Render(name)))
		} else {
			tabParts = append(tabParts, fmt.Sprintf("%s %s", icon, mutedStyle.Render(name)))
		}
	}

	// Right overflow indicator
	if endIdx < len(m.processes) {
		tabParts = append(tabParts, mutedStyle.Render(fmt.Sprintf("%d▶", len(m.processes)-endIdx)))
	}

	tabBar := strings.Join(tabParts, " │ ")

	// Logs
	logContent := m.logViewport.View()

	inner := lipgloss.JoinVertical(lipgloss.Left, tabBar, logContent)

	if m.focus == FocusLogs {
		return activeLogPaneStyle.Width(paneWidth).Height(paneHeight).Render(inner)
	}
	return logPaneStyle.Width(paneWidth).Height(paneHeight).Render(inner)
}

func (m *Model) renderWelcome(width, height int) string {
	textStyle := lipgloss.NewStyle().Foreground(capLight).Bold(true)

	logo := lipgloss.JoinVertical(lipgloss.Center,
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

	lines := make([]string, 0, len(m.preflightResults.Checks)+10)
	lines = append(lines, "")
	lines = append(lines, title)
	lines = append(lines, "")

	// Status icons
	okIcon := successStyle.Render("✓")
	warnIcon := lipgloss.NewStyle().Foreground(warnColor).Render("!")
	errIcon := errorStyle.Render("✗")

	nameStyle := lipgloss.NewStyle().Width(20)
	pathStyle := mutedStyle

	// Update version info in preflight results for display
	m.preflightResults.SetVersionInfo(m.version, m.updateInfo)

	// Version check first
	versionCheck := m.preflightResults.VersionCheck()
	var vIcon string
	var vMsgStyle lipgloss.Style
	switch versionCheck.Status {
	case preflight.StatusOK:
		vIcon = okIcon
		vMsgStyle = successStyle
	case preflight.StatusWarning:
		vIcon = warnIcon
		vMsgStyle = lipgloss.NewStyle().Foreground(warnColor)
	default:
		vIcon = okIcon
		vMsgStyle = successStyle
	}
	lines = append(lines, fmt.Sprintf("  %s %s %s", vIcon, nameStyle.Render(versionCheck.Name), vMsgStyle.Render(versionCheck.Message)))
	lines = append(lines, "")

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

	// Discoveries section
	if len(m.preflightResults.Discoveries) > 0 {
		lines = append(lines, "")
		lines = append(lines, "")
		discTitle := lipgloss.NewStyle().
			Foreground(capBlue).
			Bold(true).
			Render("  📦 Discovered Projects")
		lines = append(lines, discTitle)
		lines = append(lines, "")

		// Group discoveries by type
		typeIcons := map[string]string{
			"capacitor": "⚡",
			"firebase":  "🔥",
			"ionic":     "💠",
			"vite":      "⚡",
		}
		typeColors := map[string]lipgloss.Color{
			"capacitor": capBlue,
			"firebase":  lipgloss.Color("#FFA000"),
			"ionic":     lipgloss.Color("#3880FF"),
			"vite":      lipgloss.Color("#646CFF"),
		}

		for _, disc := range m.preflightResults.Discoveries {
			icon := typeIcons[disc.Type]
			if icon == "" {
				icon = "📁"
			}
			color := typeColors[disc.Type]
			if color == "" {
				color = capLight
			}

			typeStyle := lipgloss.NewStyle().Foreground(color).Width(12)
			nameStyle := lipgloss.NewStyle().Foreground(capLight).Bold(true)

			line := fmt.Sprintf("  %s %s %s",
				icon,
				typeStyle.Render(disc.Type),
				nameStyle.Render(disc.Name))

			if disc.Details != "" {
				line += "  " + mutedStyle.Render(disc.Details)
			}

			// Show relative path if not current dir
			cwd, _ := os.Getwd()
			if disc.Path != cwd && disc.Path != "" {
				relPath, err := filepath.Rel(cwd, disc.Path)
				if err == nil && relPath != "." {
					line += "  " + pathStyle.Render("./"+relPath)
				}
			}

			lines = append(lines, line)
		}
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

	// If we're editing a text field, handle text input
	if m.editingSettingKey != "" {
		switch msg.String() {
		case "enter":
			// Save the edited value
			if m.editingSettingType == "int" {
				var intVal int
				_, _ = fmt.Sscanf(m.editingSettingValue, "%d", &intVal)
				m.settings.SetInt(m.editingSettingKey, intVal)
			} else {
				m.settings.SetString(m.editingSettingKey, m.editingSettingValue)
			}
			_ = m.settings.Save()
			m.setStatus(fmt.Sprintf("Saved: %s", m.editingSettingValue))
			m.editingSettingKey = ""
			m.editingSettingValue = ""
			m.editingSettingType = ""
			return m, nil
		case "esc":
			// Cancel editing
			m.editingSettingKey = ""
			m.editingSettingValue = ""
			m.editingSettingType = ""
			m.setStatus("Canceled")
			return m, nil
		case "backspace":
			if len(m.editingSettingValue) > 0 {
				m.editingSettingValue = m.editingSettingValue[:len(m.editingSettingValue)-1]
			}
			return m, nil
		case "ctrl+c":
			m.gracefulShutdown()
			return m, tea.Quit
		default:
			// Add character to value (filter for int type)
			char := msg.String()
			if len(char) == 1 {
				if m.editingSettingType == "int" {
					// Only allow digits
					if char >= "0" && char <= "9" {
						m.editingSettingValue += char
					}
				} else {
					// Allow any printable character
					m.editingSettingValue += char
				}
			}
			return m, nil
		}
	}

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
			_ = m.settings.Save()
			m.setStatus(fmt.Sprintf("%s: %v", setting.Name, m.settings.GetBool(setting.Key)))
		case "choice":
			newVal := m.settings.CycleChoice(setting.Key, setting.Choices)
			_ = m.settings.Save()
			displayVal := newVal
			if displayVal == "" {
				displayVal = "(auto)"
			}
			m.setStatus(fmt.Sprintf("%s: %s", setting.Name, displayVal))
		case "string":
			// Enter edit mode for string
			m.editingSettingKey = setting.Key
			m.editingSettingValue = m.settings.GetString(setting.Key)
			m.editingSettingType = "string"
			m.setStatus("Type new value, Enter to save, Esc to cancel")
		case "int":
			// Enter edit mode for int
			m.editingSettingKey = setting.Key
			m.editingSettingValue = fmt.Sprintf("%d", m.settings.GetInt(setting.Key))
			m.editingSettingType = "int"
			m.setStatus("Type new value, Enter to save, Esc to cancel")
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

	lines := make([]string, 0, 20)
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

		// Check if this setting is being edited
		isEditing := m.editingSettingKey == s.Key

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
			if isEditing {
				// Show editing value with cursor
				valueStr = m.editingSettingValue + "█"
				valueStyle = lipgloss.NewStyle().Foreground(capBlue).Bold(true)
			} else {
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
			}
		case "int":
			if isEditing {
				// Show editing value with cursor
				valueStr = m.editingSettingValue + "█"
				valueStyle = lipgloss.NewStyle().Foreground(capBlue).Bold(true)
			} else {
				val := m.settings.GetInt(s.Key)
				valueStr = fmt.Sprintf("%d", val)
				valueStyle = lipgloss.NewStyle().Foreground(capCyan)
			}
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

	// Config file path - show actual loaded config with local/global indicator
	configPathStr := m.settings.GetConfigPath()
	if configPathStr == "" {
		configPathStr, _ = settings.ConfigPath()
	}
	configType := "global"
	if m.settings.IsLocalConfig() {
		configType = "local"
	}
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("  Config (%s): %s", configType, configPathStr)))

	// Help
	lines = append(lines, "")
	var helpLine string
	if m.editingSettingKey != "" {
		// Show editing controls
		helpLine = helpStyle.Render("  ") +
			helpKeyStyle.Render("type") + helpStyle.Render(" to edit  ") +
			helpKeyStyle.Render("enter") + helpStyle.Render(" save  ") +
			helpKeyStyle.Render("esc") + helpStyle.Render(" cancel  ") +
			helpKeyStyle.Render("backspace") + helpStyle.Render(" delete")
	} else {
		helpLine = helpStyle.Render("  ") +
			helpKeyStyle.Render("←/→") + helpStyle.Render(" category  ") +
			helpKeyStyle.Render("↑/↓") + helpStyle.Render(" select  ") +
			helpKeyStyle.Render("enter") + helpStyle.Render(" toggle/edit  ") +
			helpKeyStyle.Render("esc") + helpStyle.Render(" close  ") +
			helpKeyStyle.Render("q") + helpStyle.Render(" quit")
	}
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

func (m Model) handleProjectSelectorInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	case "esc":
		// Close selector (keep current project)
		m.showProjectSelector = false
		return m, nil

	case "up", "k":
		if m.projectCursor > 0 {
			m.projectCursor--
		}
		return m, nil

	case "down", "j":
		if m.projectCursor < len(m.projects)-1 {
			m.projectCursor++
		}
		return m, nil

	case "enter", " ":
		// Select project and close selector
		if len(m.projects) > 0 && m.projectCursor < len(m.projects) {
			m.project = m.projects[m.projectCursor]
			m.showProjectSelector = false

			// Update plugin context with new project
			if m.pluginContext != nil {
				m.pluginContext.SetProject(m.project)
			}

			m.setStatus(fmt.Sprintf("Switched to %s", m.project.Name))

			// Refresh devices for this project
			return m, loadDevices
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handlePluginsInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pluginManager == nil {
		// No plugin manager, just close
		m.showPlugins = false
		return m, nil
	}

	allPlugins := plugin.All()

	// If showing plugin settings, handle that input
	if m.showPluginSettings {
		return m.handlePluginSettingsInput(msg)
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
					// Remember that this plugin should not auto-start
					_ = m.pluginManager.SetRunning(p.ID(), false)
				}
			} else {
				if err := p.Start(); err != nil {
					m.setStatus(fmt.Sprintf("Failed to start %s: %v", p.Name(), err))
				} else {
					m.setStatus(fmt.Sprintf("Started %s", p.Name()))
					// Remember that this plugin should auto-start next time
					_ = m.pluginManager.SetRunning(p.ID(), true)
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
					// Clear the running state when disabled
					_ = m.pluginManager.SetRunning(p.ID(), false)
				} else {
					m.setStatus(fmt.Sprintf("Enabled %s", p.Name()))
				}
			}
		}
		return m, nil

	case "s":
		// Open plugin settings
		if len(allPlugins) > 0 && m.pluginCursor < len(allPlugins) {
			p := allPlugins[m.pluginCursor]
			settings := p.GetSettings()
			if len(settings) > 0 {
				m.showPluginSettings = true
				m.pluginSettingCursor = 0
			} else {
				m.setStatus(fmt.Sprintf("%s has no configurable settings", p.Name()))
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handlePluginSettingsInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	allPlugins := plugin.All()
	if m.pluginCursor >= len(allPlugins) {
		m.showPluginSettings = false
		return m, nil
	}

	p := allPlugins[m.pluginCursor]
	settings := p.GetSettings()

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

	case "esc", "s":
		m.showPluginSettings = false
		return m, nil

	case "up", "k":
		if m.pluginSettingCursor > 0 {
			m.pluginSettingCursor--
		}
		return m, nil

	case "down", "j":
		if m.pluginSettingCursor < len(settings)-1 {
			m.pluginSettingCursor++
		}
		return m, nil

	case "enter", " ":
		// Toggle boolean settings
		if m.pluginSettingCursor < len(settings) {
			setting := settings[m.pluginSettingCursor]
			if setting.Type == "bool" {
				currentVal := m.pluginManager.GetPluginSetting(p.ID(), setting.Key)
				newVal := true
				if b, ok := currentVal.(bool); ok {
					newVal = !b
				}
				_ = m.pluginManager.SetPluginSetting(p.ID(), setting.Key, newVal)
				p.OnSettingChange(setting.Key, newVal)

				if newVal {
					m.setStatus(fmt.Sprintf("%s: ON", setting.Name))
				} else {
					m.setStatus(fmt.Sprintf("%s: OFF", setting.Name))
				}
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) renderProjectSelector() string {
	// Title
	title := lipgloss.NewStyle().
		Foreground(capBlue).
		Bold(true).
		Render("  📁 Select Project")

	var lines []string
	lines = append(lines, "")
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("  Found %d Capacitor projects in this workspace", len(m.projects))))
	lines = append(lines, "")

	if len(m.projects) == 0 {
		lines = append(lines, "  "+errorStyle.Render("No Capacitor projects found"))
		lines = append(lines, "")
		lines = append(lines, mutedStyle.Render("  Make sure you're in a directory with capacitor.config.ts/js/json"))
	} else {
		for i, p := range m.projects {
			isSelected := i == m.projectCursor
			isCurrent := m.project != nil && p.RootDir == m.project.RootDir

			// Platform indicators
			var platforms []string
			if p.HasIOS {
				platforms = append(platforms, "iOS")
			}
			if p.HasAndroid {
				platforms = append(platforms, "Android")
			}
			platformStr := ""
			if len(platforms) > 0 {
				platformStr = " [" + strings.Join(platforms, ", ") + "]"
			}

			// Get relative path for display
			cwd, _ := os.Getwd()
			relPath, err := filepath.Rel(cwd, p.RootDir)
			if err != nil {
				relPath = p.RootDir
			}
			if relPath == "." {
				relPath = "(current directory)"
			} else {
				relPath = "./" + relPath
			}

			if isSelected {
				arrow := lipgloss.NewStyle().Foreground(capBlue).Bold(true).Render("▶")
				nameStyled := lipgloss.NewStyle().Foreground(capCyan).Bold(true).Render(p.Name)
				currentMarker := ""
				if isCurrent {
					currentMarker = " " + successStyle.Render("(active)")
				}

				lines = append(lines, fmt.Sprintf(" %s %s%s%s", arrow, nameStyled, mutedStyle.Render(platformStr), currentMarker))
				lines = append(lines, fmt.Sprintf("      %s", mutedStyle.Render(relPath)))
				if p.AppID != "" {
					lines = append(lines, fmt.Sprintf("      %s", mutedStyle.Render("ID: "+p.AppID)))
				}
			} else {
				prefix := "  "
				name := p.Name
				currentMarker := ""
				if isCurrent {
					currentMarker = " " + successStyle.Render("●")
				}
				lines = append(lines, fmt.Sprintf("%s %s%s%s", prefix, name, mutedStyle.Render(platformStr), currentMarker))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("  "+
		helpKeyStyle.Render("↑/↓")+" navigate  "+
		helpKeyStyle.Render("enter")+" select  "+
		helpKeyStyle.Render("esc")+" close"))

	return strings.Join(lines, "\n")
}

func (m *Model) renderPlugins() string {
	if m.pluginManager == nil {
		lines := make([]string, 0, 4)
		lines = append(lines, "")
		lines = append(lines, "  "+errorStyle.Render("Plugin system not available"))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("  Press "+helpKeyStyle.Render("esc")+" to close"))
		return strings.Join(lines, "\n")
	}

	// If showing plugin settings, render that instead
	if m.showPluginSettings {
		return m.renderPluginSettings()
	}

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

				// Show settings count hint
				settings := p.GetSettings()
				if len(settings) > 0 {
					lines = append(lines, fmt.Sprintf("      %s", mutedStyle.Render(fmt.Sprintf("Settings: %d (press s to configure)", len(settings)))))
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
		helpKeyStyle.Render("s") + helpStyle.Render(" settings  ") +
		helpKeyStyle.Render("e") + helpStyle.Render(" enable/disable  ") +
		helpKeyStyle.Render("esc") + helpStyle.Render(" close")
	lines = append(lines, helpLine)

	return strings.Join(lines, "\n")
}

func (m *Model) renderPluginSettings() string {
	allPlugins := plugin.All()
	if m.pluginCursor >= len(allPlugins) {
		return ""
	}

	p := allPlugins[m.pluginCursor]
	settings := p.GetSettings()

	// Title
	title := lipgloss.NewStyle().
		Foreground(capBlue).
		Bold(true).
		Render(fmt.Sprintf("  ⚙ %s Settings", p.Name()))

	var lines []string
	lines = append(lines, "")
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("  Configure %s plugin options", p.Name())))
	lines = append(lines, "")

	// Calculate max name width for alignment
	maxNameWidth := 0
	for _, s := range settings {
		if len(s.Name) > maxNameWidth {
			maxNameWidth = len(s.Name)
		}
	}
	if maxNameWidth > 25 {
		maxNameWidth = 25
	}
	nameStyle := lipgloss.NewStyle().Width(maxNameWidth + 2)

	for i, s := range settings {
		isSelected := i == m.pluginSettingCursor

		var valueStr string
		var valueStyle lipgloss.Style

		currentVal := m.pluginManager.GetPluginSetting(p.ID(), s.Key)

		switch s.Type {
		case "bool":
			boolVal := false
			if currentVal != nil {
				if b, ok := currentVal.(bool); ok {
					boolVal = b
				}
			} else if defVal, ok := s.Default.(bool); ok {
				boolVal = defVal
			}

			if boolVal {
				valueStr = "✓ ON"
				valueStyle = successStyle
			} else {
				valueStr = "○ OFF"
				valueStyle = mutedStyle
			}
		case "string":
			strVal := ""
			if currentVal != nil {
				if str, ok := currentVal.(string); ok {
					strVal = str
				}
			} else if defVal, ok := s.Default.(string); ok {
				strVal = defVal
			}

			if strVal == "" {
				valueStr = "(not set)"
				valueStyle = mutedStyle
			} else {
				if len(strVal) > 20 {
					strVal = strVal[:17] + "..."
				}
				valueStr = strVal
				valueStyle = lipgloss.NewStyle().Foreground(capCyan)
			}
		default:
			valueStr = fmt.Sprintf("%v", currentVal)
			valueStyle = mutedStyle
		}

		name := s.Name
		if len(name) > 25 {
			name = name[:22] + "..."
		}

		nameRendered := nameStyle.Render(name)
		valueRendered := valueStyle.Render(valueStr)

		if isSelected {
			// Highlight selected row
			arrow := lipgloss.NewStyle().Foreground(capBlue).Bold(true).Render("▶")
			lines = append(lines, fmt.Sprintf(" %s %s  %s", arrow, nameRendered, valueRendered))
			lines = append(lines, fmt.Sprintf("      %s", mutedStyle.Render(s.Description)))
		} else {
			lines = append(lines, fmt.Sprintf("   %s  %s", nameRendered, valueRendered))
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
		helpKeyStyle.Render("enter") + helpStyle.Render(" toggle  ") +
		helpKeyStyle.Render("esc") + helpStyle.Render(" back  ") +
		helpKeyStyle.Render("q") + helpStyle.Render(" quit")
	lines = append(lines, helpLine)

	return strings.Join(lines, "\n")
}
