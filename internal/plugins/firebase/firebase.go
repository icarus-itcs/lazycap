package firebase

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/icarus-itcs/lazycap/internal/plugin"
)

const (
	PluginID      = "firebase-emulator"
	PluginName    = "Firebase Emulator"
	PluginVersion = "1.1.1"
	PluginAuthor  = "lazycap"
)

// Available Firebase emulator services
var AvailableEmulators = []string{
	"auth",
	"firestore",
	"database",
	"functions",
	"hosting",
	"storage",
	"pubsub",
	"eventarc",
}

// Default ports for Firebase emulators
var DefaultEmulatorPorts = map[string]int{
	"auth":      9099,
	"firestore": 8080,
	"database":  9000,
	"functions": 5001,
	"hosting":   5000,
	"storage":   9199,
	"pubsub":    8087,
	"eventarc":  9299,
	"ui":        4000,
}

// EmulatorStatus represents the status of an emulator
type EmulatorStatus struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Running bool   `json:"running"`
}

// FirebasePlugin integrates Firebase Emulator Suite with lazycap
type FirebasePlugin struct {
	mu           sync.RWMutex
	ctx          plugin.Context
	running      bool
	cmd          *exec.Cmd
	stopCh       chan struct{}
	outputCh     chan string
	emulators    []EmulatorStatus
	configPath   string
	importPath   string
	exportOnExit bool
	exportPath   string
	uiEnabled    bool
	lastError    string

	// Per-emulator enable settings
	enabledEmulators map[string]bool
}

// New creates a new Firebase Emulator plugin instance
func New() *FirebasePlugin {
	// Default: enable common emulators
	enabled := make(map[string]bool)
	for _, emu := range AvailableEmulators {
		// Enable common ones by default
		switch emu {
		case "auth", "firestore", "functions", "storage":
			enabled[emu] = true
		default:
			enabled[emu] = false
		}
	}

	return &FirebasePlugin{
		stopCh:           make(chan struct{}),
		outputCh:         make(chan string, 100),
		exportOnExit:     true,
		exportPath:       ".firebase-export",
		uiEnabled:        true,
		enabledEmulators: enabled,
	}
}

// Register registers the plugin with the global registry
func Register() error {
	return plugin.Register(New())
}

// Plugin interface implementation

func (p *FirebasePlugin) ID() string      { return PluginID }
func (p *FirebasePlugin) Name() string    { return PluginName }
func (p *FirebasePlugin) Version() string { return PluginVersion }
func (p *FirebasePlugin) Author() string  { return PluginAuthor }
func (p *FirebasePlugin) Description() string {
	return "Integrates Firebase Emulator Suite for local development"
}

func (p *FirebasePlugin) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

func (p *FirebasePlugin) GetSettings() []plugin.Setting {
	settings := make([]plugin.Setting, 0, 4+len(AvailableEmulators))
	settings = append(settings,
		plugin.Setting{
			Key:         "importPath",
			Name:        "Import Data Path",
			Description: "Path to import emulator data from on start",
			Type:        "string",
			Default:     "",
		},
		plugin.Setting{
			Key:         "exportOnExit",
			Name:        "Export on Exit",
			Description: "Export emulator data when stopping",
			Type:        "bool",
			Default:     true,
		},
		plugin.Setting{
			Key:         "exportPath",
			Name:        "Export Path",
			Description: "Path to export emulator data to",
			Type:        "string",
			Default:     ".firebase-export",
		},
		plugin.Setting{
			Key:         "uiEnabled",
			Name:        "Enable Emulator UI",
			Description: "Enable the Firebase Emulator UI (usually at localhost:4000)",
			Type:        "bool",
			Default:     true,
		},
	)

	// Add per-emulator settings
	for _, emu := range AvailableEmulators {
		defaultEnabled := false
		switch emu {
		case "auth", "firestore", "functions", "storage":
			defaultEnabled = true
		}

		settings = append(settings, plugin.Setting{
			Key:         "emulator:" + emu,
			Name:        titleCase(emu) + " Emulator",
			Description: fmt.Sprintf("Enable %s emulator", emu),
			Type:        "bool",
			Default:     defaultEnabled,
		})
	}

	return settings
}

// titleCase capitalizes the first letter of a string
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (p *FirebasePlugin) OnSettingChange(key string, value interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch key {
	case "importPath":
		if s, ok := value.(string); ok {
			p.importPath = s
		}
	case "exportOnExit":
		if b, ok := value.(bool); ok {
			p.exportOnExit = b
		}
	case "exportPath":
		if s, ok := value.(string); ok {
			p.exportPath = s
		}
	case "uiEnabled":
		if b, ok := value.(bool); ok {
			p.uiEnabled = b
		}
	default:
		// Check for emulator:* settings
		if strings.HasPrefix(key, "emulator:") {
			emuName := strings.TrimPrefix(key, "emulator:")
			if b, ok := value.(bool); ok {
				p.enabledEmulators[emuName] = b
			}
		}
	}
}

func (p *FirebasePlugin) GetStatusLine() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.running {
		if p.lastError != "" {
			return "Firebase: " + p.lastError
		}
		return ""
	}

	count := 0
	for _, e := range p.emulators {
		if e.Running {
			count++
		}
	}

	if count == 0 {
		return "Firebase starting..."
	}
	return fmt.Sprintf("Firebase (%d)", count)
}

func (p *FirebasePlugin) GetCommands() []plugin.Command {
	return []plugin.Command{
		{
			Key:         "F",
			Name:        "Firebase",
			Description: "Toggle Firebase Emulators",
			Handler: func() error {
				if p.IsRunning() {
					return p.Stop()
				}
				return p.Start()
			},
		},
	}
}

func (p *FirebasePlugin) GetProcessIDs() []int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.cmd != nil && p.cmd.Process != nil {
		return []int{p.cmd.Process.Pid}
	}
	return nil
}

func (p *FirebasePlugin) Init(ctx plugin.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	p.lastError = ""

	// Load settings
	if importPath := ctx.GetPluginSetting(PluginID, "importPath"); importPath != nil {
		if s, ok := importPath.(string); ok {
			p.importPath = s
		}
	}
	if exportOnExit := ctx.GetPluginSetting(PluginID, "exportOnExit"); exportOnExit != nil {
		if b, ok := exportOnExit.(bool); ok {
			p.exportOnExit = b
		}
	}
	if exportPath := ctx.GetPluginSetting(PluginID, "exportPath"); exportPath != nil {
		if s, ok := exportPath.(string); ok {
			p.exportPath = s
		}
	}
	if uiEnabled := ctx.GetPluginSetting(PluginID, "uiEnabled"); uiEnabled != nil {
		if b, ok := uiEnabled.(bool); ok {
			p.uiEnabled = b
		}
	}

	// Load per-emulator settings
	for _, emu := range AvailableEmulators {
		if enabled := ctx.GetPluginSetting(PluginID, "emulator:"+emu); enabled != nil {
			if b, ok := enabled.(bool); ok {
				p.enabledEmulators[emu] = b
			}
		}
	}

	// Detect Firebase config
	p.detectFirebaseConfig()

	return nil
}

func (p *FirebasePlugin) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return nil
	}
	if p.ctx == nil {
		p.mu.Unlock()
		return fmt.Errorf("plugin not initialized - call Init() first")
	}
	p.lastError = ""
	p.mu.Unlock()

	// Check if firebase CLI is installed
	if _, err := exec.LookPath("firebase"); err != nil {
		p.mu.Lock()
		p.lastError = "firebase CLI not found"
		p.mu.Unlock()
		return fmt.Errorf("firebase CLI not installed. Install with: npm install -g firebase-tools")
	}

	// Get enabled emulators
	enabledList := p.getEnabledEmulatorsList()
	if len(enabledList) == 0 {
		p.mu.Lock()
		p.lastError = "no emulators enabled"
		p.mu.Unlock()
		return fmt.Errorf("no emulators enabled. Enable emulators in plugin settings")
	}

	p.mu.Lock()
	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	// Start emulators
	if err := p.startEmulators(enabledList); err != nil {
		p.mu.Lock()
		p.running = false
		p.lastError = err.Error()
		p.mu.Unlock()
		return err
	}

	if p.ctx != nil {
		p.ctx.Log(PluginID, "Firebase emulators started")
	}
	return nil
}

func (p *FirebasePlugin) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}

	p.running = false

	// Close stopCh if it's not nil and not already closed
	if p.stopCh != nil {
		select {
		case <-p.stopCh:
			// Already closed
		default:
			close(p.stopCh)
		}
	}

	if p.cmd != nil && p.cmd.Process != nil {
		// Try graceful shutdown first
		_ = p.cmd.Process.Signal(os.Interrupt)

		// Wait for the process to exit
		go func() {
			_ = p.cmd.Wait()
		}()
	}

	ctx := p.ctx
	p.mu.Unlock()

	if ctx != nil {
		ctx.Log(PluginID, "Firebase emulators stopped")
	}
	return nil
}

// Internal methods

func (p *FirebasePlugin) getEnabledEmulatorsList() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var enabled []string
	for emu, isEnabled := range p.enabledEmulators {
		if isEnabled {
			enabled = append(enabled, emu)
		}
	}
	return enabled
}

func (p *FirebasePlugin) detectFirebaseConfig() {
	// Look for firebase.json in multiple locations (monorepo support)
	if p.ctx == nil {
		return
	}
	project := p.ctx.GetProject()

	// Get starting directory
	var startDir string
	if project != nil {
		startDir = project.RootDir
	} else {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return
		}
	}

	// Search locations in order of priority:
	// 1. Project root
	// 2. Current working directory
	// 3. Parent directories (up to 3 levels)
	// 4. Common monorepo subdirectories (firebase/, functions/, backend/)
	searchPaths := []string{
		startDir,
	}

	// Add current working directory if different
	cwd, _ := os.Getwd()
	if cwd != startDir {
		searchPaths = append(searchPaths, cwd)
	}

	// Add parent directories
	dir := startDir
	for i := 0; i < 3; i++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		searchPaths = append(searchPaths, parent)
		dir = parent
	}

	// Add common monorepo subdirectories from cwd
	commonSubdirs := []string{"firebase", "functions", "backend", "server"}
	for _, subdir := range commonSubdirs {
		searchPaths = append(searchPaths, filepath.Join(cwd, subdir))
	}

	// Search for firebase.json
	for _, searchPath := range searchPaths {
		configPath := filepath.Join(searchPath, "firebase.json")
		if _, err := os.Stat(configPath); err == nil {
			p.configPath = configPath
			p.loadFirebaseConfig(configPath)
			return
		}
	}
}

func (p *FirebasePlugin) loadFirebaseConfig(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var config struct {
		Emulators map[string]struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		} `json:"emulators"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return
	}

	p.emulators = make([]EmulatorStatus, 0)
	for name, emu := range config.Emulators {
		if name == "ui" || name == "singleProjectMode" {
			continue
		}
		host := emu.Host
		if host == "" {
			host = "localhost"
		}
		p.emulators = append(p.emulators, EmulatorStatus{
			Name:    name,
			Host:    host,
			Port:    emu.Port,
			Running: false,
		})
	}
}

func (p *FirebasePlugin) startEmulators(enabledList []string) error {
	// Kill any processes using the required ports
	p.killPortBlockers(enabledList)

	args := []string{"emulators:start"}

	// Add --only flag with enabled emulators
	if len(enabledList) > 0 {
		args = append(args, "--only", strings.Join(enabledList, ","))
	}

	// Add import path if specified
	p.mu.RLock()
	importPath := p.importPath
	exportOnExit := p.exportOnExit
	exportPath := p.exportPath
	p.mu.RUnlock()

	if importPath != "" {
		args = append(args, "--import", importPath)
	}

	if exportOnExit && exportPath != "" {
		args = append(args, "--export-on-exit", exportPath)
	}

	cmd := exec.Command("firebase", args...)

	// Set working directory to where firebase.json is located
	p.mu.RLock()
	configPath := p.configPath
	ctx := p.ctx
	p.mu.RUnlock()

	if configPath != "" {
		// Use the directory containing firebase.json
		cmd.Dir = filepath.Dir(configPath)
	} else if ctx != nil {
		// Fallback to project root
		project := ctx.GetProject()
		if project != nil {
			cmd.Dir = project.RootDir
		}
	}

	// Log the command being run
	if ctx != nil {
		cmdStr := "firebase " + strings.Join(args, " ")
		ctx.Log(PluginID, fmt.Sprintf("Starting: %s", cmdStr))
		if cmd.Dir != "" {
			ctx.Log(PluginID, fmt.Sprintf("Working directory: %s", cmd.Dir))
		}
	}

	// Capture output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start firebase emulators: %w", err)
	}

	p.mu.Lock()
	p.cmd = cmd
	// Initialize emulators list based on enabled list
	p.emulators = make([]EmulatorStatus, 0, len(enabledList))
	for _, name := range enabledList {
		p.emulators = append(p.emulators, EmulatorStatus{
			Name:    name,
			Host:    "localhost",
			Port:    0, // Will be detected from output
			Running: false,
		})
	}
	p.mu.Unlock()

	// Read output in goroutines
	go p.readOutput(stdout)
	go p.readOutput(stderr)

	// Wait for process in goroutine
	go func() {
		_ = cmd.Wait()
		p.mu.Lock()
		p.running = false
		p.cmd = nil
		// Mark all emulators as stopped
		for i := range p.emulators {
			p.emulators[i].Running = false
		}
		p.mu.Unlock()
	}()

	return nil
}

// killPortBlockers kills any processes using the ports needed by the enabled emulators
func (p *FirebasePlugin) killPortBlockers(enabledList []string) {
	p.mu.RLock()
	ctx := p.ctx
	configuredEmulators := p.emulators
	configPath := p.configPath
	p.mu.RUnlock()

	// Build a map of configured ports from firebase.json
	configuredPorts := make(map[string]int)
	for _, emu := range configuredEmulators {
		if emu.Port > 0 {
			configuredPorts[emu.Name] = emu.Port
		}
	}

	// Also read UI port from firebase.json if available
	if configPath != "" {
		if uiPort := p.getUIPortFromConfig(configPath); uiPort > 0 {
			configuredPorts["ui"] = uiPort
		}
	}

	// Collect ports to free - prefer configured ports, fall back to defaults
	portsToFree := make([]int, 0)
	for _, emu := range enabledList {
		if port, ok := configuredPorts[emu]; ok && port > 0 {
			portsToFree = append(portsToFree, port)
		} else if port, ok := DefaultEmulatorPorts[emu]; ok {
			portsToFree = append(portsToFree, port)
		}
	}

	// Always include UI port
	if uiPort, ok := configuredPorts["ui"]; ok && uiPort > 0 {
		portsToFree = append(portsToFree, uiPort)
	} else {
		portsToFree = append(portsToFree, DefaultEmulatorPorts["ui"])
	}

	for _, port := range portsToFree {
		// Use lsof to find processes using this port
		cmd := exec.Command("lsof", "-t", "-i", fmt.Sprintf(":%d", port))
		output, err := cmd.Output()
		if err != nil {
			continue // No process using this port
		}

		// Kill each PID found
		pids := strings.Fields(strings.TrimSpace(string(output)))
		for _, pidStr := range pids {
			var pid int
			if _, err := fmt.Sscanf(pidStr, "%d", &pid); err == nil && pid > 0 {
				if ctx != nil {
					ctx.Log(PluginID, fmt.Sprintf("Killing process %d using port %d", pid, port))
				}
				// Kill the process
				killCmd := exec.Command("kill", "-9", pidStr)
				_ = killCmd.Run()
			}
		}
	}
}

// getUIPortFromConfig reads the UI port from firebase.json
func (p *FirebasePlugin) getUIPortFromConfig(configPath string) int {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return 0
	}

	var config struct {
		Emulators struct {
			UI struct {
				Port int `json:"port"`
			} `json:"ui"`
		} `json:"emulators"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return 0
	}

	return config.Emulators.UI.Port
}

func (p *FirebasePlugin) readOutput(reader interface{ Read([]byte) (int, error) }) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		p.mu.RLock()
		stopCh := p.stopCh
		ctx := p.ctx
		p.mu.RUnlock()

		if stopCh != nil {
			select {
			case <-stopCh:
				return
			default:
			}
		}

		line := scanner.Text()

		// Parse emulator status from output
		p.parseEmulatorStatus(line)

		// Log to lazycap
		if ctx != nil {
			ctx.Log(PluginID, line)
		}
	}
}

func (p *FirebasePlugin) parseEmulatorStatus(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Firebase emulator output contains lines like:
	// "✔  firestore: Firestore Emulator UI at http://127.0.0.1:4000/firestore"
	// "✔  All emulators ready! It is now safe to connect your app."
	// "i  firestore: Firestore Emulator logging to firestore-debug.log"
	// "✔  functions: Using node@18 from host."

	lowerLine := strings.ToLower(line)

	for i, emu := range p.emulators {
		// Check if this emulator is mentioned as running
		emuLower := strings.ToLower(emu.Name)
		if strings.Contains(lowerLine, emuLower+":") ||
			strings.Contains(lowerLine, emuLower+" emulator") {
			// Check for success indicators
			if strings.Contains(line, "✔") || strings.Contains(lowerLine, "ready") ||
				strings.Contains(lowerLine, "running") || strings.Contains(lowerLine, "listening") {
				p.emulators[i].Running = true
			}
		}
	}

	// Check for "All emulators ready" message
	if strings.Contains(lowerLine, "all emulators ready") {
		for i := range p.emulators {
			p.emulators[i].Running = true
		}
	}
}

// GetEmulatorStatus returns the current status of all emulators
func (p *FirebasePlugin) GetEmulatorStatus() []EmulatorStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]EmulatorStatus, len(p.emulators))
	copy(result, p.emulators)
	return result
}

// GetEmulatorURL returns the URL for a specific emulator
func (p *FirebasePlugin) GetEmulatorURL(name string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, emu := range p.emulators {
		if emu.Name == name && emu.Running {
			return fmt.Sprintf("http://%s:%d", emu.Host, emu.Port)
		}
	}
	return ""
}

// IsFirebaseProject returns true if the project has Firebase configured
func (p *FirebasePlugin) IsFirebaseProject() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.configPath != ""
}

// GetLastError returns the last error message
func (p *FirebasePlugin) GetLastError() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastError
}
