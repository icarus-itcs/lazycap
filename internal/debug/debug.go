package debug

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Action represents a debug/cleanup action
type Action struct {
	ID          string
	Name        string
	Description string
	Category    string
	Platform    string // "all", "darwin", "linux", "windows"
	Dangerous   bool   // Requires extra confirmation
}

// Result represents the result of running an action
type Result struct {
	Success bool
	Message string
	Details string
}

// GetActions returns all available debug actions
func GetActions() []Action {
	actions := []Action{
		// Xcode / iOS
		{
			ID:          "xcode-derived-data",
			Name:        "Clear Xcode Derived Data",
			Description: "Removes ~/Library/Developer/Xcode/DerivedData - fixes most Xcode build issues",
			Category:    "iOS/Xcode",
			Platform:    "darwin",
			Dangerous:   false,
		},
		{
			ID:          "xcode-device-support",
			Name:        "Clear Device Support",
			Description: "Removes ~/Library/Developer/Xcode/iOS DeviceSupport - fixes device connection issues",
			Category:    "iOS/Xcode",
			Platform:    "darwin",
			Dangerous:   false,
		},
		{
			ID:          "xcode-archives",
			Name:        "Clear Xcode Archives",
			Description: "Removes ~/Library/Developer/Xcode/Archives - frees up disk space",
			Category:    "iOS/Xcode",
			Platform:    "darwin",
			Dangerous:   true,
		},
		{
			ID:          "ios-build-clean",
			Name:        "Clean iOS Build",
			Description: "Runs xcodebuild clean in ios/ folder",
			Category:    "iOS/Xcode",
			Platform:    "darwin",
			Dangerous:   false,
		},
		{
			ID:          "pod-cache-clean",
			Name:        "Clear CocoaPods Cache",
			Description: "Runs pod cache clean --all and removes Pods folder",
			Category:    "iOS/Xcode",
			Platform:    "darwin",
			Dangerous:   false,
		},
		{
			ID:          "pod-deintegrate",
			Name:        "Deintegrate & Reinstall Pods",
			Description: "Removes all CocoaPods from project and reinstalls fresh",
			Category:    "iOS/Xcode",
			Platform:    "darwin",
			Dangerous:   false,
		},
		{
			ID:          "simulators-reset",
			Name:        "Reset All Simulators",
			Description: "Erases all content and settings from all simulators",
			Category:    "iOS/Xcode",
			Platform:    "darwin",
			Dangerous:   true,
		},
		{
			ID:          "simulator-kill",
			Name:        "Kill Simulator Processes",
			Description: "Force kills all simulator processes",
			Category:    "iOS/Xcode",
			Platform:    "darwin",
			Dangerous:   false,
		},

		// Android
		{
			ID:          "android-clean",
			Name:        "Clean Android Build",
			Description: "Runs ./gradlew clean in android/ folder",
			Category:    "Android",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "gradle-cache",
			Name:        "Clear Gradle Cache",
			Description: "Removes ~/.gradle/caches - fixes most Gradle issues",
			Category:    "Android",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "gradle-stop",
			Name:        "Stop Gradle Daemons",
			Description: "Stops all running Gradle daemon processes",
			Category:    "Android",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "android-build-cache",
			Name:        "Clear Android Build Cache",
			Description: "Removes android/.gradle and android/app/build folders",
			Category:    "Android",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "adb-kill",
			Name:        "Restart ADB Server",
			Description: "Kills and restarts the ADB server",
			Category:    "Android",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "emulator-wipe",
			Name:        "Wipe Emulator Data",
			Description: "Wipes data from all Android emulators",
			Category:    "Android",
			Platform:    "all",
			Dangerous:   true,
		},

		// Node/NPM
		{
			ID:          "node-modules",
			Name:        "Clear node_modules",
			Description: "Removes node_modules folder and reinstalls",
			Category:    "Node/NPM",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "npm-cache",
			Name:        "Clear NPM Cache",
			Description: "Runs npm cache clean --force",
			Category:    "Node/NPM",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "package-lock",
			Name:        "Regenerate package-lock",
			Description: "Removes package-lock.json and regenerates it",
			Category:    "Node/NPM",
			Platform:    "all",
			Dangerous:   false,
		},

		// Capacitor
		{
			ID:          "cap-sync-force",
			Name:        "Force Capacitor Sync",
			Description: "Removes native web assets and re-syncs",
			Category:    "Capacitor",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "cap-update",
			Name:        "Update Capacitor Plugins",
			Description: "Updates all Capacitor packages to latest",
			Category:    "Capacitor",
			Platform:    "all",
			Dangerous:   false,
		},

		// Web Build
		{
			ID:          "web-cache",
			Name:        "Clear Web Build Cache",
			Description: "Removes dist/, www/, .cache/, .parcel-cache/",
			Category:    "Web Build",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "vite-cache",
			Name:        "Clear Vite Cache",
			Description: "Removes node_modules/.vite folder",
			Category:    "Web Build",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "web-kill-port",
			Name:        "Kill Dev Server Ports",
			Description: "Kills processes on common dev ports (3000, 5173, 8080, 8100)",
			Category:    "Web Build",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "web-rebuild",
			Name:        "Force Rebuild Web Assets",
			Description: "Clears build cache and rebuilds web assets from scratch",
			Category:    "Web Build",
			Platform:    "all",
			Dangerous:   false,
		},

		// System
		{
			ID:          "watchman-cache",
			Name:        "Clear Watchman Cache",
			Description: "Runs watchman watch-del-all",
			Category:    "System",
			Platform:    "all",
			Dangerous:   false,
		},
		{
			ID:          "tmp-clean",
			Name:        "Clear Temp Files",
			Description: "Removes lazycap temp/log files",
			Category:    "System",
			Platform:    "all",
			Dangerous:   false,
		},

		// Nuclear options
		{
			ID:          "full-clean",
			Name:        "Full Project Clean",
			Description: "Removes ALL caches: node_modules, ios/Pods, android/.gradle, build folders",
			Category:    "Nuclear",
			Platform:    "all",
			Dangerous:   true,
		},
		{
			ID:          "fresh-install",
			Name:        "Fresh Install",
			Description: "Full clean + npm install + pod install + cap sync",
			Category:    "Nuclear",
			Platform:    "all",
			Dangerous:   true,
		},
	}

	// Filter by platform
	var filtered []Action
	for _, a := range actions {
		if a.Platform == "all" || a.Platform == runtime.GOOS {
			filtered = append(filtered, a)
		}
	}

	return filtered
}

// GetCategories returns unique categories in order
func GetCategories() []string {
	if runtime.GOOS == "darwin" {
		return []string{"iOS/Xcode", "Android", "Node/NPM", "Capacitor", "Web Build", "System", "Nuclear"}
	}
	return []string{"Android", "Node/NPM", "Capacitor", "Web Build", "System", "Nuclear"}
}

// RunAction executes a debug action
func RunAction(id string) Result {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	switch id {
	// iOS/Xcode
	case "xcode-derived-data":
		path := filepath.Join(home, "Library/Developer/Xcode/DerivedData")
		return removeDir(path, "Derived Data")

	case "xcode-device-support":
		path := filepath.Join(home, "Library/Developer/Xcode/iOS DeviceSupport")
		return removeDir(path, "Device Support")

	case "xcode-archives":
		path := filepath.Join(home, "Library/Developer/Xcode/Archives")
		return removeDir(path, "Archives")

	case "ios-build-clean":
		return runCommand(filepath.Join(cwd, "ios"), "xcodebuild", "clean")

	case "pod-cache-clean":
		// Clean pod cache
		runCommand(cwd, "pod", "cache", "clean", "--all")
		// Remove Pods folder
		podsPath := filepath.Join(cwd, "ios/Pods")
		removeDir(podsPath, "Pods")
		// Remove Podfile.lock
		os.Remove(filepath.Join(cwd, "ios/Podfile.lock"))
		return Result{Success: true, Message: "CocoaPods cache cleared", Details: "Run 'pod install' in ios/ folder to reinstall"}

	case "pod-deintegrate":
		iosDir := filepath.Join(cwd, "ios")
		runCommand(iosDir, "pod", "deintegrate")
		runCommand(iosDir, "pod", "install")
		return Result{Success: true, Message: "Pods deintegrated and reinstalled"}

	case "simulators-reset":
		return runCommand(cwd, "xcrun", "simctl", "erase", "all")

	case "simulator-kill":
		runCommand(cwd, "killall", "Simulator")
		runCommand(cwd, "killall", "com.apple.CoreSimulator.CoreSimulatorService")
		return Result{Success: true, Message: "Simulator processes killed"}

	// Android
	case "android-clean":
		androidDir := filepath.Join(cwd, "android")
		if runtime.GOOS == "windows" {
			return runCommand(androidDir, "gradlew.bat", "clean")
		}
		return runCommand(androidDir, "./gradlew", "clean")

	case "gradle-cache":
		path := filepath.Join(home, ".gradle/caches")
		return removeDir(path, "Gradle caches")

	case "gradle-stop":
		androidDir := filepath.Join(cwd, "android")
		if runtime.GOOS == "windows" {
			return runCommand(androidDir, "gradlew.bat", "--stop")
		}
		return runCommand(androidDir, "./gradlew", "--stop")

	case "android-build-cache":
		removeDir(filepath.Join(cwd, "android/.gradle"), "android/.gradle")
		removeDir(filepath.Join(cwd, "android/app/build"), "android/app/build")
		return Result{Success: true, Message: "Android build cache cleared"}

	case "adb-kill":
		runCommand(cwd, "adb", "kill-server")
		return runCommand(cwd, "adb", "start-server")

	case "emulator-wipe":
		// List AVDs and wipe each one
		cmd := exec.Command("emulator", "-list-avds")
		output, err := cmd.Output()
		if err != nil {
			return Result{Success: false, Message: "Failed to list emulators"}
		}
		avds := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, avd := range avds {
			if avd != "" {
				exec.Command("emulator", "-avd", avd, "-wipe-data", "-no-window", "-no-boot-anim").Start()
			}
		}
		return Result{Success: true, Message: fmt.Sprintf("Wiped %d emulators", len(avds))}

	// Node/NPM
	case "node-modules":
		removeDir(filepath.Join(cwd, "node_modules"), "node_modules")
		return runCommand(cwd, "npm", "install")

	case "npm-cache":
		return runCommand(cwd, "npm", "cache", "clean", "--force")

	case "package-lock":
		os.Remove(filepath.Join(cwd, "package-lock.json"))
		return runCommand(cwd, "npm", "install")

	// Capacitor
	case "cap-sync-force":
		// Remove native web assets
		removeDir(filepath.Join(cwd, "ios/App/App/public"), "ios web assets")
		removeDir(filepath.Join(cwd, "android/app/src/main/assets/public"), "android web assets")
		return runCommand(cwd, "npx", "cap", "sync")

	case "cap-update":
		return runCommand(cwd, "npm", "update", "@capacitor/core", "@capacitor/cli", "@capacitor/ios", "@capacitor/android")

	// Web Build
	case "web-cache":
		removeDir(filepath.Join(cwd, "dist"), "dist")
		removeDir(filepath.Join(cwd, "www"), "www")
		removeDir(filepath.Join(cwd, ".cache"), ".cache")
		removeDir(filepath.Join(cwd, ".parcel-cache"), ".parcel-cache")
		return Result{Success: true, Message: "Web build cache cleared"}

	case "vite-cache":
		return removeDir(filepath.Join(cwd, "node_modules/.vite"), "Vite cache")

	case "web-kill-port":
		// Kill processes on common dev ports
		ports := []string{"3000", "5173", "8080", "8100"}
		killedCount := 0
		for _, port := range ports {
			// Use lsof to find and kill processes
			cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%s", port))
			output, err := cmd.Output()
			if err == nil && len(output) > 0 {
				pids := strings.Split(strings.TrimSpace(string(output)), "\n")
				for _, pid := range pids {
					if pid != "" {
						exec.Command("kill", "-9", pid).Run()
						killedCount++
					}
				}
			}
		}
		if killedCount > 0 {
			return Result{Success: true, Message: fmt.Sprintf("Killed %d processes on dev ports", killedCount)}
		}
		return Result{Success: true, Message: "No processes found on dev ports"}

	case "web-rebuild":
		// Clear build cache and rebuild
		removeDir(filepath.Join(cwd, "dist"), "dist")
		removeDir(filepath.Join(cwd, "www"), "www")
		removeDir(filepath.Join(cwd, ".cache"), ".cache")
		removeDir(filepath.Join(cwd, "node_modules/.vite"), "Vite cache")
		removeDir(filepath.Join(cwd, "node_modules/.cache"), "Babel cache")
		// Try to rebuild
		return runCommand(cwd, "npm", "run", "build")

	// System
	case "watchman-cache":
		return runCommand(cwd, "watchman", "watch-del-all")

	case "tmp-clean":
		os.Remove("/tmp/lazycap-debug.log")
		// Clean any lazycap temp files
		files, _ := filepath.Glob("/tmp/lazycap-*")
		for _, f := range files {
			os.Remove(f)
		}
		return Result{Success: true, Message: "Temp files cleared"}

	// Nuclear
	case "full-clean":
		removeDir(filepath.Join(cwd, "node_modules"), "node_modules")
		removeDir(filepath.Join(cwd, "ios/Pods"), "ios/Pods")
		removeDir(filepath.Join(cwd, "ios/build"), "ios/build")
		removeDir(filepath.Join(cwd, "android/.gradle"), "android/.gradle")
		removeDir(filepath.Join(cwd, "android/app/build"), "android/app/build")
		removeDir(filepath.Join(cwd, "dist"), "dist")
		removeDir(filepath.Join(cwd, "www"), "www")
		os.Remove(filepath.Join(cwd, "package-lock.json"))
		os.Remove(filepath.Join(cwd, "ios/Podfile.lock"))
		return Result{Success: true, Message: "Full project clean complete", Details: "Run 'npm install' then 'npx cap sync' to rebuild"}

	case "fresh-install":
		// Full clean first
		RunAction("full-clean")
		// Reinstall
		runCommand(cwd, "npm", "install")
		if runtime.GOOS == "darwin" {
			runCommand(filepath.Join(cwd, "ios"), "pod", "install")
		}
		runCommand(cwd, "npx", "cap", "sync")
		return Result{Success: true, Message: "Fresh install complete"}

	default:
		return Result{Success: false, Message: "Unknown action: " + id}
	}
}

func removeDir(path, name string) Result {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return Result{Success: true, Message: fmt.Sprintf("%s not found (already clean)", name)}
	}
	if err != nil {
		return Result{Success: false, Message: fmt.Sprintf("Error checking %s: %v", name, err)}
	}

	// Calculate size before removing
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err := os.RemoveAll(path); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("Failed to remove %s: %v", name, err)}
	}

	sizeStr := formatSize(size)
	if info.IsDir() {
		return Result{Success: true, Message: fmt.Sprintf("Removed %s", name), Details: fmt.Sprintf("Freed %s", sizeStr)}
	}
	return Result{Success: true, Message: fmt.Sprintf("Removed %s (%s)", name, sizeStr)}
}

func runCommand(dir, name string, args ...string) Result {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Success: false,
			Message: fmt.Sprintf("Command failed: %s %s", name, strings.Join(args, " ")),
			Details: string(output),
		}
	}
	return Result{
		Success: true,
		Message: fmt.Sprintf("Ran: %s %s", name, strings.Join(args, " ")),
		Details: strings.TrimSpace(string(output)),
	}
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
