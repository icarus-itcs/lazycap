package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/icarus-itcs/lazycap/internal/update"
)

// CheckResult represents the result of a single check
type CheckResult struct {
	Name    string
	Status  Status
	Message string
	Path    string
}

// Status represents the status of a check
type Status int

const (
	StatusOK Status = iota
	StatusWarning
	StatusError
)

// Discovery represents a discovered project or configuration
type Discovery struct {
	Type    string // "capacitor", "firebase", "ionic", etc.
	Name    string
	Path    string
	Details string
}

// Results contains all preflight check results
type Results struct {
	Checks      []CheckResult
	Discoveries []Discovery
	HasErrors   bool
	HasWarnings bool
	Version     string
	UpdateInfo  *update.Info
}

// RequiredTool defines a tool to check for
type RequiredTool struct {
	Name     string
	Command  string
	Required bool
	Platform string // "all", "darwin", "linux", "windows"
}

var requiredTools = []RequiredTool{
	// Core tools
	{Name: "Node.js", Command: "node", Required: true, Platform: "all"},
	{Name: "npm", Command: "npm", Required: true, Platform: "all"},
	{Name: "npx", Command: "npx", Required: true, Platform: "all"},
	{Name: "git", Command: "git", Required: true, Platform: "all"},

	// iOS tools (macOS only)
	{Name: "Xcode CLI", Command: "xcrun", Required: false, Platform: "darwin"},
	{Name: "CocoaPods", Command: "pod", Required: false, Platform: "darwin"},
	{Name: "iOS Simulator", Command: "xcrun simctl help", Required: false, Platform: "darwin"},

	// Android tools
	{Name: "Android ADB", Command: "adb", Required: false, Platform: "all"},
	{Name: "Android Emulator", Command: "emulator", Required: false, Platform: "all"},
}

// Run executes all preflight checks
func Run() *Results {
	return RunAt("")
}

// RunAt executes all preflight checks from a specific directory
func RunAt(baseDir string) *Results {
	results := &Results{
		Checks:      make([]CheckResult, 0),
		Discoveries: make([]Discovery, 0),
	}

	// Check each required tool
	for _, tool := range requiredTools {
		// Skip platform-specific tools
		if tool.Platform != "all" && tool.Platform != runtime.GOOS {
			continue
		}

		result := checkTool(tool)
		results.Checks = append(results.Checks, result)

		switch result.Status {
		case StatusError:
			results.HasErrors = true
		case StatusWarning:
			results.HasWarnings = true
		}
	}

	// Check Capacitor CLI
	capResult := checkCapacitorCLI()
	results.Checks = append(results.Checks, capResult)
	if capResult.Status == StatusError {
		results.HasErrors = true
	}

	// Discover projects and configurations
	results.Discoveries = discoverProjects(baseDir)

	return results
}

func checkTool(tool RequiredTool) CheckResult {
	result := CheckResult{
		Name: tool.Name,
	}

	// Handle commands with arguments (like "xcrun simctl help")
	parts := strings.Fields(tool.Command)
	cmdName := parts[0]

	path, err := exec.LookPath(cmdName)
	if err != nil {
		if tool.Required {
			result.Status = StatusError
			result.Message = "Not found - required"
		} else {
			result.Status = StatusWarning
			result.Message = "Not found - optional"
		}
		return result
	}

	result.Path = path

	// If command has args, try to run it to verify it works
	if len(parts) > 1 {
		cmd := exec.Command(parts[0], parts[1:]...)
		if err := cmd.Run(); err != nil {
			result.Status = StatusWarning
			result.Message = fmt.Sprintf("Found but may not work: %v", err)
			return result
		}
	}

	// Get version if possible
	version := getToolVersion(cmdName)
	if version != "" {
		result.Message = version
	} else {
		result.Message = "OK"
	}
	result.Status = StatusOK

	return result
}

func checkCapacitorCLI() CheckResult {
	result := CheckResult{
		Name: "Capacitor CLI",
	}

	// Check if npx cap works
	cmd := exec.Command("npx", "cap", "--version")
	output, err := cmd.Output()
	if err != nil {
		result.Status = StatusError
		result.Message = "Not installed - run: npm install @capacitor/cli"
		return result
	}

	result.Status = StatusOK
	result.Message = "v" + strings.TrimSpace(string(output))
	result.Path = "npx cap"
	return result
}

func getToolVersion(cmd string) string {
	var versionArgs []string

	switch cmd {
	case "node":
		versionArgs = []string{"--version"}
	case "npm", "npx":
		versionArgs = []string{"--version"}
	case "git":
		versionArgs = []string{"--version"}
	case "pod":
		versionArgs = []string{"--version"}
	case "adb":
		versionArgs = []string{"version"}
	case "xcrun":
		// xcrun --version doesn't give useful output, skip
		return ""
	default:
		versionArgs = []string{"--version"}
	}

	out, err := exec.Command(cmd, versionArgs...).Output()
	if err != nil {
		return ""
	}

	version := strings.TrimSpace(string(out))
	// Clean up version string - take first line only
	if idx := strings.Index(version, "\n"); idx != -1 {
		version = version[:idx]
	}
	// Remove common prefixes
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "git version ")

	if len(version) > 30 {
		version = version[:30] + "..."
	}

	return version
}

// Summary returns a short summary of the results
func (r *Results) Summary() string {
	ok := 0
	warn := 0
	fail := 0

	for _, c := range r.Checks {
		switch c.Status {
		case StatusOK:
			ok++
		case StatusWarning:
			warn++
		case StatusError:
			fail++
		}
	}

	if fail > 0 {
		return fmt.Sprintf("%d errors, %d warnings", fail, warn)
	}
	if warn > 0 {
		return fmt.Sprintf("%d warnings", warn)
	}
	return fmt.Sprintf("%d checks passed", ok)
}

// SetVersionInfo sets version and update information on the results
func (r *Results) SetVersionInfo(version string, info *update.Info) {
	r.Version = version
	r.UpdateInfo = info
}

// VersionCheck returns a CheckResult for the current version
func (r *Results) VersionCheck() CheckResult {
	result := CheckResult{
		Name: "lazycap",
	}

	if r.Version == "" || r.Version == "dev" {
		result.Status = StatusOK
		result.Message = "dev (development build)"
		return result
	}

	result.Status = StatusOK
	result.Message = "v" + r.Version

	if r.UpdateInfo != nil && r.UpdateInfo.UpdateAvailable {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("v%s (v%s available, press U to update)", r.Version, r.UpdateInfo.LatestVersion)
	}

	return result
}

// discoverProjects finds Capacitor projects, Firebase configs, etc.
func discoverProjects(baseDir string) []Discovery {
	var discoveries []Discovery //nolint:prealloc // size unknown, grows dynamically

	if baseDir == "" {
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			return discoveries
		}
	}

	// Walk directory tree (max 4 levels deep)
	discoveries = append(discoveries, walkForDiscoveries(baseDir, 0, 4)...)

	return discoveries
}

func walkForDiscoveries(dir string, depth, maxDepth int) []Discovery {
	discoveries := make([]Discovery, 0)

	if depth > maxDepth {
		return discoveries
	}

	// Check for Capacitor project
	capConfigs := []string{"capacitor.config.ts", "capacitor.config.js", "capacitor.config.json"}
	for _, cfg := range capConfigs {
		configPath := filepath.Join(dir, cfg)
		if _, err := os.Stat(configPath); err == nil {
			name := getProjectName(dir)
			details := cfg
			// Check for platforms
			var platforms []string
			if _, err := os.Stat(filepath.Join(dir, "ios")); err == nil {
				platforms = append(platforms, "ios")
			}
			if _, err := os.Stat(filepath.Join(dir, "android")); err == nil {
				platforms = append(platforms, "android")
			}
			if len(platforms) > 0 {
				details += " [" + strings.Join(platforms, ", ") + "]"
			}
			discoveries = append(discoveries, Discovery{
				Type:    "capacitor",
				Name:    name,
				Path:    dir,
				Details: details,
			})
			break
		}
	}

	// Check for Firebase
	firebasePath := filepath.Join(dir, "firebase.json")
	if _, err := os.Stat(firebasePath); err == nil {
		name := filepath.Base(dir)
		details := detectFirebaseServices(firebasePath)
		discoveries = append(discoveries, Discovery{
			Type:    "firebase",
			Name:    name,
			Path:    dir,
			Details: details,
		})
	}

	// Check for Ionic
	ionicPath := filepath.Join(dir, "ionic.config.json")
	if _, err := os.Stat(ionicPath); err == nil {
		name := filepath.Base(dir)
		discoveries = append(discoveries, Discovery{
			Type:    "ionic",
			Name:    name,
			Path:    dir,
			Details: "ionic.config.json",
		})
	}

	// Check for Vite
	viteConfigs := []string{"vite.config.ts", "vite.config.js", "vite.config.mjs"}
	for _, cfg := range viteConfigs {
		vitePath := filepath.Join(dir, cfg)
		if _, err := os.Stat(vitePath); err == nil {
			// Only add if not already discovered as capacitor (to avoid duplicates)
			hasCapacitor := false
			for _, d := range discoveries {
				if d.Type == "capacitor" && d.Path == dir {
					hasCapacitor = true
					break
				}
			}
			if !hasCapacitor {
				discoveries = append(discoveries, Discovery{
					Type:    "vite",
					Name:    filepath.Base(dir),
					Path:    dir,
					Details: cfg,
				})
			}
			break
		}
	}

	// Recurse into subdirectories
	entries, err := os.ReadDir(dir)
	if err != nil {
		return discoveries
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip common non-project directories
		if name == "node_modules" || name == ".git" || name == "dist" || name == "build" ||
			name == "ios" || name == "android" || name == ".firebase" || name == "Pods" ||
			strings.HasPrefix(name, ".") {
			continue
		}
		subDiscoveries := walkForDiscoveries(filepath.Join(dir, name), depth+1, maxDepth)
		discoveries = append(discoveries, subDiscoveries...)
	}

	return discoveries
}

func getProjectName(dir string) string {
	// Try to get name from package.json
	pkgPath := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		// Simple JSON parsing for name field
		content := string(data)
		if idx := strings.Index(content, `"name"`); idx != -1 {
			rest := content[idx+6:]
			if colonIdx := strings.Index(rest, `"`); colonIdx != -1 {
				rest = rest[colonIdx+1:]
				if endIdx := strings.Index(rest, `"`); endIdx != -1 {
					return rest[:endIdx]
				}
			}
		}
	}
	return filepath.Base(dir)
}

func detectFirebaseServices(configPath string) string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "firebase.json"
	}

	content := string(data)
	var services []string

	if strings.Contains(content, `"hosting"`) {
		services = append(services, "hosting")
	}
	if strings.Contains(content, `"functions"`) {
		services = append(services, "functions")
	}
	if strings.Contains(content, `"firestore"`) {
		services = append(services, "firestore")
	}
	if strings.Contains(content, `"storage"`) {
		services = append(services, "storage")
	}
	if strings.Contains(content, `"emulators"`) {
		services = append(services, "emulators")
	}
	if strings.Contains(content, `"auth"`) {
		services = append(services, "auth")
	}

	if len(services) > 0 {
		return strings.Join(services, ", ")
	}
	return "firebase.json"
}
