package preflight

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
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

// Results contains all preflight check results
type Results struct {
	Checks  []CheckResult
	HasErrors bool
	HasWarnings bool
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
	results := &Results{
		Checks: make([]CheckResult, 0),
	}

	// Check each required tool
	for _, tool := range requiredTools {
		// Skip platform-specific tools
		if tool.Platform != "all" && tool.Platform != runtime.GOOS {
			continue
		}

		result := checkTool(tool)
		results.Checks = append(results.Checks, result)

		if result.Status == StatusError {
			results.HasErrors = true
		} else if result.Status == StatusWarning {
			results.HasWarnings = true
		}
	}

	// Check Capacitor CLI
	capResult := checkCapacitorCLI()
	results.Checks = append(results.Checks, capResult)
	if capResult.Status == StatusError {
		results.HasErrors = true
	}

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
