package cap

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/icarus-itcs/lazycap/internal/device"
)

// UpgradeInfo contains information about available upgrades
type UpgradeInfo struct {
	CurrentVersion string
	LatestVersion  string
	HasUpgrade     bool
}

// Project represents a Capacitor project
type Project struct {
	Name       string
	AppID      string
	WebDir     string
	HasAndroid bool
	HasIOS     bool
	ConfigPath string
	RootDir    string
}

// CapacitorConfig represents the capacitor.config.json/ts structure
type CapacitorConfig struct {
	AppID   string `json:"appId"`
	AppName string `json:"appName"`
	WebDir  string `json:"webDir"`
}

// IsCapacitorProject checks if current directory is a Capacitor project
func IsCapacitorProject() bool {
	configFiles := []string{
		"capacitor.config.ts",
		"capacitor.config.js",
		"capacitor.config.json",
	}

	for _, f := range configFiles {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}

// IsCapacitorProjectAt checks if a directory contains a Capacitor project
func IsCapacitorProjectAt(dir string) bool {
	configFiles := []string{
		"capacitor.config.ts",
		"capacitor.config.js",
		"capacitor.config.json",
	}

	for _, f := range configFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}

// DiscoverProjects finds all Capacitor projects in the current directory and subdirectories
// It searches up to maxDepth levels deep (default 3)
func DiscoverProjects(maxDepth int) ([]*Project, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	var projects []*Project
	visited := make(map[string]bool)

	// Check current directory first
	if IsCapacitorProject() {
		if p, err := LoadProject(); err == nil {
			projects = append(projects, p)
			visited[cwd] = true
		}
	}

	// Search subdirectories
	err = walkDirWithDepth(cwd, 0, maxDepth, func(path string, d os.DirEntry, depth int) error {
		if !d.IsDir() {
			return nil
		}

		// Skip common non-project directories
		name := d.Name()
		if name == "node_modules" || name == ".git" || name == "dist" ||
			name == "build" || name == ".next" || name == ".nuxt" ||
			name == "ios" || name == "android" || name == "vendor" {
			return filepath.SkipDir
		}

		fullPath := path
		if visited[fullPath] {
			return nil
		}

		if IsCapacitorProjectAt(fullPath) {
			if p, err := LoadProjectAt(fullPath); err == nil {
				projects = append(projects, p)
				visited[fullPath] = true
				return filepath.SkipDir // Don't search inside found projects
			}
		}

		return nil
	})

	if err != nil {
		return projects, err // Return what we found even if walk had errors
	}

	return projects, nil
}

// walkDirWithDepth walks a directory tree with depth tracking
func walkDirWithDepth(root string, currentDepth, maxDepth int, fn func(path string, d os.DirEntry, depth int) error) error {
	if currentDepth > maxDepth {
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil // Ignore permission errors, etc.
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if err := fn(path, entry, currentDepth); err != nil {
			if err == filepath.SkipDir {
				continue
			}
			return err
		}

		if entry.IsDir() && currentDepth < maxDepth {
			walkDirWithDepth(path, currentDepth+1, maxDepth, fn)
		}
	}

	return nil
}

// LoadProject loads the current Capacitor project configuration
func LoadProject() (*Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	return LoadProjectAt(cwd)
}

// LoadProjectAt loads a Capacitor project configuration from a specific directory
func LoadProjectAt(dir string) (*Project, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	project := &Project{
		RootDir: absDir,
	}

	// Find and load config
	configFiles := []string{
		"capacitor.config.json",
		"capacitor.config.ts",
		"capacitor.config.js",
	}

	for _, f := range configFiles {
		path := filepath.Join(absDir, f)
		if _, err := os.Stat(path); err == nil {
			project.ConfigPath = path
			break
		}
	}

	if project.ConfigPath == "" {
		return nil, fmt.Errorf("no capacitor config found in %s", absDir)
	}

	// Try to get config via Capacitor CLI (handles ts/js configs)
	config, err := getCapacitorConfigAt(absDir)
	if err != nil {
		// Fallback to reading JSON directly
		if strings.HasSuffix(project.ConfigPath, ".json") {
			data, err := os.ReadFile(project.ConfigPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read config: %w", err)
			}
			var cfg CapacitorConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config: %w", err)
			}
			project.Name = cfg.AppName
			project.AppID = cfg.AppID
			project.WebDir = cfg.WebDir
		}
	} else {
		project.Name = config.AppName
		project.AppID = config.AppID
		project.WebDir = config.WebDir
	}

	// Check for platform directories
	if _, err := os.Stat(filepath.Join(absDir, "android")); err == nil {
		project.HasAndroid = true
	}
	if _, err := os.Stat(filepath.Join(absDir, "ios")); err == nil {
		project.HasIOS = true
	}

	// Default name if not set
	if project.Name == "" {
		project.Name = filepath.Base(absDir)
	}

	return project, nil
}

// getCapacitorConfig gets config using the Capacitor CLI
func getCapacitorConfig() (*CapacitorConfig, error) {
	cmd := exec.Command("npx", "cap", "config", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var config CapacitorConfig
	if err := json.Unmarshal(output, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// getCapacitorConfigAt gets config using the Capacitor CLI from a specific directory
func getCapacitorConfigAt(dir string) (*CapacitorConfig, error) {
	cmd := exec.Command("npx", "cap", "config", "--json")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var config CapacitorConfig
	if err := json.Unmarshal(output, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// ListDevices returns all available devices/emulators
func ListDevices() ([]device.Device, error) {
	var devices []device.Device

	// Add Web device first (always available)
	devices = append(devices, device.Device{
		ID:       "web",
		Name:     "Web Browser",
		Platform: "web",
		Online:   true,
		IsWeb:    true,
	})

	// Get iOS devices (macOS only) - show iOS before Android
	iosDevices, err := listIOSDevices()
	if err == nil {
		devices = append(devices, iosDevices...)
	}

	// Get Android devices
	androidDevices, err := listAndroidDevices()
	if err == nil {
		devices = append(devices, androidDevices...)
	}

	return devices, nil
}

// listAndroidDevices lists Android devices via adb
func listAndroidDevices() ([]device.Device, error) {
	var devices []device.Device

	// Check if adb is available
	if _, err := exec.LookPath("adb"); err != nil {
		return devices, nil // adb not installed, skip
	}

	// Get connected devices
	cmd := exec.Command("adb", "devices", "-l")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines[1:] { // Skip header
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		id := parts[0]
		status := parts[1]

		d := device.Device{
			ID:        id,
			Platform:  "android",
			Online:    status == "device",
			IsEmulator: strings.HasPrefix(id, "emulator"),
		}

		// Try to get device name
		for _, part := range parts[2:] {
			if strings.HasPrefix(part, "model:") {
				d.Name = strings.TrimPrefix(part, "model:")
				break
			}
			if strings.HasPrefix(part, "device:") {
				d.Name = strings.TrimPrefix(part, "device:")
			}
		}

		if d.Name == "" {
			d.Name = id
		}

		devices = append(devices, d)
	}

	// Get emulators
	emulators, _ := listAndroidEmulators()
	
	// Add offline emulators that aren't already running
	for _, emu := range emulators {
		found := false
		for _, d := range devices {
			if d.Name == emu.Name {
				found = true
				break
			}
		}
		if !found {
			devices = append(devices, emu)
		}
	}

	return devices, nil
}

// listAndroidEmulators lists available Android emulators
func listAndroidEmulators() ([]device.Device, error) {
	var devices []device.Device

	cmd := exec.Command("emulator", "-list-avds")
	output, err := cmd.Output()
	if err != nil {
		return devices, nil // emulator command not available
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}

		devices = append(devices, device.Device{
			ID:         name,
			Name:       name,
			Platform:   "android",
			Online:     false,
			IsEmulator: true,
		})
	}

	return devices, nil
}

// listIOSDevices lists iOS devices and simulators
func listIOSDevices() ([]device.Device, error) {
	var devices []device.Device

	// Check if xcrun is available (macOS only)
	if _, err := exec.LookPath("xcrun"); err != nil {
		return devices, nil
	}

	// Get simulators
	cmd := exec.Command("xcrun", "simctl", "list", "devices", "-j")
	output, err := cmd.Output()
	if err != nil {
		return devices, nil
	}

	var result struct {
		Devices map[string][]struct {
			UDID      string `json:"udid"`
			Name      string `json:"name"`
			State     string `json:"state"`
			IsAvailable bool `json:"isAvailable"`
		} `json:"devices"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return devices, nil
	}

	for runtime, sims := range result.Devices {
		// Only include iOS simulators
		if !strings.Contains(runtime, "iOS") {
			continue
		}

		for _, sim := range sims {
			if !sim.IsAvailable {
				continue
			}

			devices = append(devices, device.Device{
				ID:         sim.UDID,
				Name:       sim.Name,
				Platform:   "ios",
				Online:     sim.State == "Booted",
				IsEmulator: true,
			})
		}
	}

	return devices, nil
}

// Run runs the app on a device
func Run(deviceID string, platform string, liveReload bool) error {
	return RunAt("", deviceID, platform, liveReload)
}

// RunAt runs the app on a device from a specific project directory
func RunAt(projectDir string, deviceID string, platform string, liveReload bool) error {
	args := []string{"cap", "run", platform, "--target", deviceID}
	if liveReload {
		args = append(args, "-l")
	}

	cmd := exec.Command("npx", args...)
	if projectDir != "" {
		cmd.Dir = projectDir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Sync syncs web assets to native projects
func Sync(platform string) error {
	return SyncAt("", platform)
}

// SyncAt syncs web assets to native projects from a specific directory
func SyncAt(projectDir string, platform string) error {
	args := []string{"cap", "sync"}
	if platform != "" {
		args = append(args, platform)
	}

	cmd := exec.Command("npx", args...)
	if projectDir != "" {
		cmd.Dir = projectDir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Build builds the web assets
func Build() error {
	return BuildAt("")
}

// BuildAt builds the web assets from a specific directory
func BuildAt(projectDir string) error {
	// Try common build commands
	buildCmds := [][]string{
		{"npm", "run", "build"},
		{"yarn", "build"},
		{"pnpm", "build"},
	}

	for _, args := range buildCmds {
		if _, err := exec.LookPath(args[0]); err == nil {
			cmd := exec.Command(args[0], args[1:]...)
			if projectDir != "" {
				cmd.Dir = projectDir
			}
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}

	return fmt.Errorf("no build command found")
}

// Open opens the native project in the IDE
func Open(platform string) error {
	return OpenAt("", platform)
}

// OpenAt opens the native project in the IDE from a specific directory
func OpenAt(projectDir string, platform string) error {
	cmd := exec.Command("npx", "cap", "open", platform)
	if projectDir != "" {
		cmd.Dir = projectDir
	}
	return cmd.Run()
}

// BootDevice boots a simulator/emulator
func BootDevice(deviceID string, platform string, isEmulator bool) error {
	switch platform {
	case "ios":
		// Boot iOS simulator using simctl
		cmd := exec.Command("xcrun", "simctl", "boot", deviceID)
		return cmd.Run()
	case "android":
		if isEmulator {
			// Start Android emulator in background
			// deviceID for emulators is the AVD name
			cmd := exec.Command("emulator", "-avd", deviceID, "-no-snapshot-load")
			cmd.Stdout = nil
			cmd.Stderr = nil
			return cmd.Start() // Start in background, don't wait
		}
		// Physical Android devices can't be "booted" from here
		return fmt.Errorf("cannot boot physical Android device - please connect and enable USB debugging")
	}
	return fmt.Errorf("unknown platform: %s", platform)
}

// IsDeviceBooted checks if a device is currently online/booted
func IsDeviceBooted(deviceID string, platform string) bool {
	switch platform {
	case "ios":
		cmd := exec.Command("xcrun", "simctl", "list", "devices", "-j")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		// Quick check - if deviceID and "Booted" appear near each other
		return strings.Contains(string(output), deviceID) &&
			strings.Contains(string(output), `"state" : "Booted"`)
	case "android":
		cmd := exec.Command("adb", "devices")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		// Check if device ID appears with "device" status
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, deviceID) && strings.Contains(line, "device") {
				return true
			}
		}
	}
	return false
}

// CheckForUpgrade checks if a Capacitor upgrade is available
func CheckForUpgrade() (*UpgradeInfo, error) {
	info := &UpgradeInfo{}

	// Get current installed version
	cmd := exec.Command("npm", "list", "@capacitor/core", "--json")
	output, err := cmd.Output()
	if err != nil {
		// Try to parse anyway, npm list returns error if not found
		if len(output) == 0 {
			return nil, fmt.Errorf("capacitor not installed")
		}
	}

	var installed struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(output, &installed); err == nil {
		if dep, ok := installed.Dependencies["@capacitor/core"]; ok {
			info.CurrentVersion = dep.Version
		}
	}

	// Get latest version from npm
	cmd = exec.Command("npm", "view", "@capacitor/core", "version")
	output, err = cmd.Output()
	if err != nil {
		return info, nil // Return what we have
	}
	info.LatestVersion = strings.TrimSpace(string(output))

	// Compare versions (simple string comparison works for semver)
	if info.CurrentVersion != "" && info.LatestVersion != "" {
		info.HasUpgrade = info.CurrentVersion != info.LatestVersion
	}

	return info, nil
}

// PerformUpgrade runs the Capacitor upgrade process
func PerformUpgrade() error {
	// Use npm to update capacitor packages
	cmd := exec.Command("npm", "install", "@capacitor/core@latest", "@capacitor/cli@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WebDevOptions contains options for running the web dev server
type WebDevOptions struct {
	Command     string
	Port        int
	Host        string
	Https       bool
	OpenBrowser bool
	BrowserPath string
}

// DetectWebDevCommand detects the appropriate dev server command
func DetectWebDevCommand() string {
	// Check package.json for scripts
	data, err := os.ReadFile("package.json")
	if err != nil {
		// No package.json, try common direct commands
		return detectFallbackCommand()
	}

	var pkg struct {
		Scripts      map[string]string `json:"scripts"`
		Dependencies map[string]string `json:"dependencies"`
		DevDeps      map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return detectFallbackCommand()
	}

	// Priority order of common dev scripts
	scriptPriority := []string{
		"dev",        // Vite, Nuxt, common
		"serve",      // Vue CLI
		"start",      // Create React App, general
		"develop",    // Gatsby
		"dev:web",    // Ionic
		"ionic:serve", // Ionic
		"watch",      // Some setups
	}

	for _, script := range scriptPriority {
		if _, ok := pkg.Scripts[script]; ok {
			return "npm run " + script
		}
	}

	// No known script found, try to detect framework and use npx directly
	allDeps := make(map[string]bool)
	for k := range pkg.Dependencies {
		allDeps[k] = true
	}
	for k := range pkg.DevDeps {
		allDeps[k] = true
	}

	// Check for common frameworks
	if allDeps["vite"] {
		return "npx vite"
	}
	if allDeps["@ionic/cli"] {
		return "npx ionic serve"
	}
	if allDeps["webpack-dev-server"] {
		return "npx webpack serve"
	}
	if allDeps["parcel"] {
		return "npx parcel"
	}

	return detectFallbackCommand()
}

// detectFallbackCommand tries to find a working dev command
func detectFallbackCommand() string {
	// Check if vite.config exists
	viteConfigs := []string{"vite.config.js", "vite.config.ts", "vite.config.mjs"}
	for _, cfg := range viteConfigs {
		if _, err := os.Stat(cfg); err == nil {
			return "npx vite"
		}
	}

	// Check for ionic.config.json
	if _, err := os.Stat("ionic.config.json"); err == nil {
		return "npx ionic serve"
	}

	// Check for webpack.config
	webpackConfigs := []string{"webpack.config.js", "webpack.config.ts"}
	for _, cfg := range webpackConfigs {
		if _, err := os.Stat(cfg); err == nil {
			return "npx webpack serve"
		}
	}

	// Default to vite as it's most common with Capacitor
	return "npx vite"
}

// RunWebDev starts the web development server
func RunWebDev(opts WebDevOptions) (*exec.Cmd, error) {
	command := opts.Command
	if command == "" {
		command = DetectWebDevCommand()
	}

	// Parse command into parts
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	// Build args with host/port if supported
	args := parts[1:]

	// Many frameworks support --port and --host flags
	if opts.Port > 0 {
		args = append(args, "--port", fmt.Sprintf("%d", opts.Port))
	}
	if opts.Host != "" && opts.Host != "localhost" {
		args = append(args, "--host", opts.Host)
	}

	cmd := exec.Command(parts[0], args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cmd, nil
}

// OpenBrowser opens the given URL in the default browser or specified browser
func OpenBrowser(url string, browserPath string) error {
	var cmd *exec.Cmd

	if browserPath != "" {
		cmd = exec.Command(browserPath, url)
	} else {
		// Use system default browser
		switch {
		case fileExists("/usr/bin/open"): // macOS
			cmd = exec.Command("open", url)
		case fileExists("/usr/bin/xdg-open"): // Linux
			cmd = exec.Command("xdg-open", url)
		default:
			return fmt.Errorf("no browser command found")
		}
	}

	return cmd.Start()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetWebDevURL returns the URL for the web dev server
func GetWebDevURL(opts WebDevOptions) string {
	scheme := "http"
	if opts.Https {
		scheme = "https"
	}
	host := opts.Host
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	port := opts.Port
	if port == 0 {
		port = 5173 // Vite default
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}

// WaitForPort waits for a port to become available (server listening)
func WaitForPort(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("localhost:%d", port)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// KillPort kills any process using the specified port
// Returns true if a process was killed
func KillPort(port int) bool {
	if port <= 0 {
		return false
	}
	// Use lsof to find processes on the port
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return false // No process on port
	}

	killed := false
	pids := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, pid := range pids {
		if pid != "" {
			if err := exec.Command("kill", "-9", pid).Run(); err == nil {
				killed = true
			}
		}
	}
	return killed
}
