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

	"lazycap/internal/plugin"
)

const (
	PluginID      = "firebase-emulator"
	PluginName    = "Firebase Emulator"
	PluginVersion = "1.0.0"
	PluginAuthor  = "lazycap"
)

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
	projectID    string
	configPath   string
	autoStart    bool
	importPath   string
	exportOnExit bool
}

// New creates a new Firebase Emulator plugin instance
func New() *FirebasePlugin {
	return &FirebasePlugin{
		stopCh:       make(chan struct{}),
		outputCh:     make(chan string, 100),
		autoStart:    false,
		exportOnExit: true,
	}
}

// Register registers the plugin with the global registry
func Register() error {
	return plugin.Register(New())
}

// Plugin interface implementation

func (p *FirebasePlugin) ID() string          { return PluginID }
func (p *FirebasePlugin) Name() string        { return PluginName }
func (p *FirebasePlugin) Version() string     { return PluginVersion }
func (p *FirebasePlugin) Author() string      { return PluginAuthor }
func (p *FirebasePlugin) Description() string {
	return "Integrates Firebase Emulator Suite for local development"
}

func (p *FirebasePlugin) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

func (p *FirebasePlugin) GetSettings() []plugin.Setting {
	return []plugin.Setting{
		{
			Key:         "autoStart",
			Name:        "Auto Start",
			Description: "Start emulators when lazycap starts",
			Type:        "bool",
			Default:     false,
		},
		{
			Key:         "importPath",
			Name:        "Import Data Path",
			Description: "Path to import emulator data from on start",
			Type:        "string",
			Default:     "",
		},
		{
			Key:         "exportOnExit",
			Name:        "Export on Exit",
			Description: "Export emulator data when stopping",
			Type:        "bool",
			Default:     true,
		},
		{
			Key:         "exportPath",
			Name:        "Export Path",
			Description: "Path to export emulator data to",
			Type:        "string",
			Default:     ".firebase-export",
		},
		{
			Key:         "uiEnabled",
			Name:        "Enable Emulator UI",
			Description: "Enable the Firebase Emulator UI",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (p *FirebasePlugin) OnSettingChange(key string, value interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch key {
	case "autoStart":
		if b, ok := value.(bool); ok {
			p.autoStart = b
		}
	case "importPath":
		if s, ok := value.(string); ok {
			p.importPath = s
		}
	case "exportOnExit":
		if b, ok := value.(bool); ok {
			p.exportOnExit = b
		}
	}
}

func (p *FirebasePlugin) GetStatusLine() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.running {
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

func (p *FirebasePlugin) Init(ctx plugin.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx

	// Load settings
	if autoStart := ctx.GetPluginSetting(PluginID, "autoStart"); autoStart != nil {
		if b, ok := autoStart.(bool); ok {
			p.autoStart = b
		}
	}
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

	// Check if Firebase is configured
	if p.configPath == "" {
		p.mu.Unlock()
		return fmt.Errorf("no firebase.json found in project")
	}

	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	// Start emulators
	if err := p.startEmulators(); err != nil {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
		return err
	}

	p.ctx.Log(PluginID, "Firebase emulators started")
	return nil
}

func (p *FirebasePlugin) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}

	p.running = false
	close(p.stopCh)

	if p.cmd != nil && p.cmd.Process != nil {
		// Try graceful shutdown first
		p.cmd.Process.Signal(os.Interrupt)

		// Force kill after a moment if needed
		go func() {
			// Give it time to shutdown gracefully
			// The process should export data if exportOnExit is enabled
			p.cmd.Wait()
		}()
	}
	p.mu.Unlock()

	p.ctx.Log(PluginID, "Firebase emulators stopped")
	return nil
}

// Internal methods

func (p *FirebasePlugin) detectFirebaseConfig() {
	// Look for firebase.json in project root
	project := p.ctx.GetProject()
	if project == nil {
		return
	}

	configPath := filepath.Join(project.RootDir, "firebase.json")
	if _, err := os.Stat(configPath); err == nil {
		p.configPath = configPath
		p.loadFirebaseConfig(configPath)
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

func (p *FirebasePlugin) startEmulators() error {
	args := []string{"emulators:start"}

	// Add import path if specified
	p.mu.RLock()
	importPath := p.importPath
	exportOnExit := p.exportOnExit
	p.mu.RUnlock()

	if importPath != "" {
		args = append(args, "--import", importPath)
	}

	if exportOnExit {
		exportPath := ".firebase-export"
		if ep := p.ctx.GetPluginSetting(PluginID, "exportPath"); ep != nil {
			if s, ok := ep.(string); ok && s != "" {
				exportPath = s
			}
		}
		args = append(args, "--export-on-exit", exportPath)
	}

	cmd := exec.Command("firebase", args...)

	// Get project root for working directory
	project := p.ctx.GetProject()
	if project != nil {
		cmd.Dir = project.RootDir
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
	p.mu.Unlock()

	// Read output in goroutines
	go p.readOutput(stdout)
	go p.readOutput(stderr)

	// Wait for process in goroutine
	go func() {
		cmd.Wait()
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

func (p *FirebasePlugin) readOutput(reader interface{ Read([]byte) (int, error) }) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		select {
		case <-p.stopCh:
			return
		default:
		}

		line := scanner.Text()

		// Parse emulator status from output
		p.parseEmulatorStatus(line)

		// Log to lazycap
		p.ctx.Log(PluginID, line)
	}
}

func (p *FirebasePlugin) parseEmulatorStatus(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Firebase emulator output contains lines like:
	// "✔  firestore: Firestore Emulator UI at http://127.0.0.1:4000/firestore"
	// "✔  All emulators ready! It is now safe to connect your app."

	for i, emu := range p.emulators {
		// Check if this emulator is mentioned as running
		if strings.Contains(line, emu.Name+":") && strings.Contains(line, "Emulator") {
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

// readOutput helper for *os.File (fix the type)
func (p *FirebasePlugin) readOutputPipe(pipe interface{ Read([]byte) (int, error) }) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		select {
		case <-p.stopCh:
			return
		default:
		}

		line := scanner.Text()
		p.parseEmulatorStatus(line)
		p.ctx.Log(PluginID, line)
	}
}
