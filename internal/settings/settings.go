package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings contains all user preferences
type Settings struct {
	// === RUN OPTIONS ===
	LiveReloadDefault   bool   `json:"liveReloadDefault"`   // Enable live reload by default
	ExternalHost        string `json:"externalHost"`        // External IP for live reload (empty = auto)
	LiveReloadPort      int    `json:"liveReloadPort"`      // Port for live reload server
	AutoSync            bool   `json:"autoSync"`            // Sync before running
	AutoBuild           bool   `json:"autoBuild"`           // Build before syncing
	DefaultPlatform     string `json:"defaultPlatform"`     // Preferred platform: "ios", "android", ""
	RunInBackground     bool   `json:"runInBackground"`     // Don't block UI during run
	ClearLogsOnRun      bool   `json:"clearLogsOnRun"`      // Clear process logs on new run

	// === BUILD OPTIONS ===
	BuildCommand        string `json:"buildCommand"`        // Custom build command (empty = auto-detect)
	ProductionBuild     bool   `json:"productionBuild"`     // Use production build by default
	SourceMaps          bool   `json:"sourceMaps"`          // Generate source maps
	BuildTimeout        int    `json:"buildTimeout"`        // Build timeout in seconds

	// === iOS OPTIONS ===
	IOSScheme           string `json:"iosScheme"`           // Xcode scheme name
	IOSConfiguration    string `json:"iosConfiguration"`    // Debug or Release
	IOSSimulator        string `json:"iosSimulator"`        // Default simulator UDID
	IOSTeamID           string `json:"iosTeamId"`           // Development team ID
	IOSDerivedData      string `json:"iosDerivedData"`      // Custom derived data path
	IOSCodeSignIdentity string `json:"iosCodeSignIdentity"` // Code signing identity
	IOSAutoSigningBool  bool   `json:"iosAutoSigning"`      // Use automatic signing

	// === ANDROID OPTIONS ===
	AndroidDevice       string `json:"androidDevice"`       // Default device/emulator ID
	AndroidFlavor       string `json:"androidFlavor"`       // Build flavor
	AndroidBuildType    string `json:"androidBuildType"`    // debug or release
	AndroidKeystorePath string `json:"androidKeystorePath"` // Path to release keystore
	AndroidSDKPath      string `json:"androidSdkPath"`      // Custom Android SDK path
	AndroidStudioPath   string `json:"androidStudioPath"`   // Path to Android Studio

	// === WEB OPTIONS ===
	WebDevCommand       string `json:"webDevCommand"`       // Dev server command (empty = auto-detect)
	WebDevPort          int    `json:"webDevPort"`          // Dev server port
	WebOpenBrowser      bool   `json:"webOpenBrowser"`      // Auto-open browser on start
	WebBrowserPath      string `json:"webBrowserPath"`      // Custom browser path
	WebHost             string `json:"webHost"`             // Dev server host (localhost, 0.0.0.0)
	WebHttps            bool   `json:"webHttps"`            // Use HTTPS for dev server

	// === UI OPTIONS ===
	ShowSpinners        bool   `json:"showSpinners"`        // Show animated spinners
	CompactMode         bool   `json:"compactMode"`         // Compact UI layout
	ShowTimestamps      bool   `json:"showTimestamps"`      // Timestamps in logs
	MaxLogLines         int    `json:"maxLogLines"`         // Max lines per process log
	ColorTheme          string `json:"colorTheme"`          // "dark", "light", "system"
	ShowDeviceIcons     bool   `json:"showDeviceIcons"`     // Show emoji icons for devices
	ShowPlatformBadges  bool   `json:"showPlatformBadges"`  // Show iOS/Android badges
	LogFontSize         string `json:"logFontSize"`         // "small", "normal", "large"

	// === BEHAVIOR ===
	ConfirmBeforeKill   bool   `json:"confirmBeforeKill"`   // Confirm before killing process
	AutoScrollLogs      bool   `json:"autoScrollLogs"`      // Auto-scroll to bottom
	RefreshOnFocus      bool   `json:"refreshOnFocus"`      // Refresh devices on focus
	CheckForUpgrades    bool   `json:"checkForUpgrades"`    // Check Capacitor upgrades
	NotifyOnComplete    bool   `json:"notifyOnComplete"`    // System notification when done
	SoundOnComplete     bool   `json:"soundOnComplete"`     // Play sound when done
	AutoOpenIDE         bool   `json:"autoOpenIde"`         // Auto-open IDE on error
	KeepProcessHistory  int    `json:"keepProcessHistory"`  // Number of old processes to keep

	// === SYNC OPTIONS ===
	SyncOnSave          bool   `json:"syncOnSave"`          // Sync when web files change
	SyncTimeout         int    `json:"syncTimeout"`         // Sync timeout in seconds
	CopyWebDir          bool   `json:"copyWebDir"`          // Copy web dir on sync
	UpdateNative        bool   `json:"updateNative"`        // Update native deps on sync
	PodInstall          bool   `json:"podInstall"`          // Run pod install on iOS sync

	// === PATHS & ENVIRONMENT ===
	NodePath            string `json:"nodePath"`            // Custom node path
	NpmPath             string `json:"npmPath"`             // Custom npm path
	NpxPath             string `json:"npxPath"`             // Custom npx path
	PodPath             string `json:"podPath"`             // Custom pod path
	XcodePath           string `json:"xcodePath"`           // Custom Xcode path
	ShellPath           string `json:"shellPath"`           // Shell to use for commands
	WorkingDirectory    string `json:"workingDirectory"`    // Override working directory

	// === ADVANCED ===
	VerboseLogging      bool              `json:"verboseLogging"`      // Verbose output
	DebugMode           bool              `json:"debugMode"`           // Debug mode
	PreRunCommand       string            `json:"preRunCommand"`       // Command to run before each run
	PostRunCommand      string            `json:"postRunCommand"`      // Command to run after each run
	PreBuildCommand     string            `json:"preBuildCommand"`     // Command to run before build
	PostBuildCommand    string            `json:"postBuildCommand"`    // Command to run after build
	EnvironmentVars     map[string]string `json:"environmentVars"`     // Custom env vars
	CapacitorConfigPath string            `json:"capacitorConfigPath"` // Custom config path
	DisableTelemetry    bool              `json:"disableTelemetry"`    // Disable telemetry

	// === PLUGIN SETTINGS ===
	EnableHotReload     bool   `json:"enableHotReload"`     // Enable hot module reload
	PreserveState       bool   `json:"preserveState"`       // Preserve app state on reload
	InlineSourceMaps    bool   `json:"inlineSourceMaps"`    // Inline source maps
	MinifyBuilds        bool   `json:"minifyBuilds"`        // Minify production builds

	// === EXPERIMENTAL ===
	EnableBetaFeatures  bool   `json:"enableBetaFeatures"`  // Enable beta features
	ParallelBuilds      bool   `json:"parallelBuilds"`      // Build platforms in parallel
	CacheBuilds         bool   `json:"cacheBuilds"`         // Cache build artifacts
	IncrementalSync     bool   `json:"incrementalSync"`     // Only sync changed files
}

// DefaultSettings returns settings with sensible defaults
func DefaultSettings() *Settings {
	return &Settings{
		// Run options
		LiveReloadDefault:   false,
		ExternalHost:        "",
		LiveReloadPort:      8100,
		AutoSync:            false,
		AutoBuild:           false,
		DefaultPlatform:     "",
		RunInBackground:     false,
		ClearLogsOnRun:      false,

		// Build options
		BuildCommand:        "",
		ProductionBuild:     false,
		SourceMaps:          true,
		BuildTimeout:        300,

		// iOS options
		IOSScheme:           "",
		IOSConfiguration:    "Debug",
		IOSSimulator:        "",
		IOSTeamID:           "",
		IOSDerivedData:      "",
		IOSCodeSignIdentity: "",
		IOSAutoSigningBool:  true,

		// Android options
		AndroidDevice:       "",
		AndroidFlavor:       "",
		AndroidBuildType:    "debug",
		AndroidKeystorePath: "",
		AndroidSDKPath:      "",
		AndroidStudioPath:   "",

		// Web options
		WebDevCommand:       "",
		WebDevPort:          5173,
		WebOpenBrowser:      true,
		WebBrowserPath:      "",
		WebHost:             "localhost",
		WebHttps:            false,

		// UI options
		ShowSpinners:        true,
		CompactMode:         false,
		ShowTimestamps:      true,
		MaxLogLines:         5000,
		ColorTheme:          "dark",
		ShowDeviceIcons:     true,
		ShowPlatformBadges:  true,
		LogFontSize:         "normal",

		// Behavior
		ConfirmBeforeKill:   false,
		AutoScrollLogs:      true,
		RefreshOnFocus:      true,
		CheckForUpgrades:    true,
		NotifyOnComplete:    false,
		SoundOnComplete:     false,
		AutoOpenIDE:         false,
		KeepProcessHistory:  10,

		// Sync options
		SyncOnSave:          false,
		SyncTimeout:         120,
		CopyWebDir:          true,
		UpdateNative:        true,
		PodInstall:          true,

		// Paths
		NodePath:            "",
		NpmPath:             "",
		NpxPath:             "",
		PodPath:             "",
		XcodePath:           "",
		ShellPath:           "",
		WorkingDirectory:    "",

		// Advanced
		VerboseLogging:      false,
		DebugMode:           false,
		PreRunCommand:       "",
		PostRunCommand:      "",
		PreBuildCommand:     "",
		PostBuildCommand:    "",
		EnvironmentVars:     make(map[string]string),
		CapacitorConfigPath: "",
		DisableTelemetry:    false,

		// Plugin settings
		EnableHotReload:     true,
		PreserveState:       true,
		InlineSourceMaps:    false,
		MinifyBuilds:        true,

		// Experimental
		EnableBetaFeatures:  false,
		ParallelBuilds:      false,
		CacheBuilds:         false,
		IncrementalSync:     false,
	}
}

// configDir returns the config directory path
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lazycap"), nil
}

// configPath returns the full path to the settings file
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// Load loads settings from disk, returning defaults if not found
func Load() (*Settings, error) {
	path, err := configPath()
	if err != nil {
		return DefaultSettings(), err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return DefaultSettings(), err
	}

	settings := DefaultSettings()
	if err := json.Unmarshal(data, settings); err != nil {
		return DefaultSettings(), err
	}

	return settings, nil
}

// Save saves settings to disk
func (s *Settings) Save() error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Category represents a group of settings
type Category struct {
	Name     string
	Icon     string
	Settings []SettingInfo
}

// SettingInfo describes a setting for the UI
type SettingInfo struct {
	Key         string
	Name        string
	Description string
	Type        string   // "bool", "string", "int", "choice"
	Choices     []string // For "choice" type
}

// GetCategories returns all settings organized by category
func GetCategories() []Category {
	return []Category{
		{
			Name: "Run",
			Icon: "▶",
			Settings: []SettingInfo{
				{Key: "liveReloadDefault", Name: "Live Reload Default", Description: "Enable live reload by default when running", Type: "bool"},
				{Key: "autoSync", Name: "Auto Sync", Description: "Automatically sync before running", Type: "bool"},
				{Key: "autoBuild", Name: "Auto Build", Description: "Automatically build before syncing", Type: "bool"},
				{Key: "defaultPlatform", Name: "Default Platform", Description: "Preferred platform for commands", Type: "choice", Choices: []string{"", "ios", "android"}},
				{Key: "clearLogsOnRun", Name: "Clear Logs on Run", Description: "Clear log output when starting new run", Type: "bool"},
				{Key: "runInBackground", Name: "Run in Background", Description: "Don't block UI during run operations", Type: "bool"},
			},
		},
		{
			Name: "Build",
			Icon: "🔨",
			Settings: []SettingInfo{
				{Key: "buildCommand", Name: "Build Command", Description: "Custom build command (empty = auto-detect)", Type: "string"},
				{Key: "productionBuild", Name: "Production Build", Description: "Use production build by default", Type: "bool"},
				{Key: "sourceMaps", Name: "Source Maps", Description: "Generate source maps", Type: "bool"},
				{Key: "buildTimeout", Name: "Build Timeout", Description: "Build timeout in seconds", Type: "int"},
				{Key: "minifyBuilds", Name: "Minify Builds", Description: "Minify production builds", Type: "bool"},
			},
		},
		{
			Name: "iOS",
			Icon: "",
			Settings: []SettingInfo{
				{Key: "iosConfiguration", Name: "Configuration", Description: "Build configuration", Type: "choice", Choices: []string{"Debug", "Release"}},
				{Key: "iosScheme", Name: "Scheme", Description: "Xcode scheme name", Type: "string"},
				{Key: "iosSimulator", Name: "Default Simulator", Description: "Default simulator UDID", Type: "string"},
				{Key: "iosTeamId", Name: "Team ID", Description: "Development team ID", Type: "string"},
				{Key: "iosAutoSigning", Name: "Auto Signing", Description: "Use automatic code signing", Type: "bool"},
				{Key: "iosDerivedData", Name: "Derived Data Path", Description: "Custom derived data path", Type: "string"},
			},
		},
		{
			Name: "Android",
			Icon: "🤖",
			Settings: []SettingInfo{
				{Key: "androidBuildType", Name: "Build Type", Description: "Build type", Type: "choice", Choices: []string{"debug", "release"}},
				{Key: "androidFlavor", Name: "Flavor", Description: "Build flavor", Type: "string"},
				{Key: "androidDevice", Name: "Default Device", Description: "Default device/emulator ID", Type: "string"},
				{Key: "androidSdkPath", Name: "SDK Path", Description: "Custom Android SDK path", Type: "string"},
				{Key: "androidKeystorePath", Name: "Keystore Path", Description: "Path to release keystore", Type: "string"},
			},
		},
		{
			Name: "Web",
			Icon: "🌐",
			Settings: []SettingInfo{
				{Key: "webDevCommand", Name: "Dev Command", Description: "Dev server command (empty = auto-detect)", Type: "string"},
				{Key: "webDevPort", Name: "Dev Port", Description: "Dev server port", Type: "int"},
				{Key: "webHost", Name: "Host", Description: "Dev server host", Type: "choice", Choices: []string{"localhost", "0.0.0.0"}},
				{Key: "webOpenBrowser", Name: "Open Browser", Description: "Auto-open browser on start", Type: "bool"},
				{Key: "webHttps", Name: "Use HTTPS", Description: "Use HTTPS for dev server", Type: "bool"},
				{Key: "webBrowserPath", Name: "Browser Path", Description: "Custom browser executable path", Type: "string"},
			},
		},
		{
			Name: "Sync",
			Icon: "🔄",
			Settings: []SettingInfo{
				{Key: "syncOnSave", Name: "Sync on Save", Description: "Sync when web files change", Type: "bool"},
				{Key: "syncTimeout", Name: "Sync Timeout", Description: "Sync timeout in seconds", Type: "int"},
				{Key: "copyWebDir", Name: "Copy Web Dir", Description: "Copy web directory on sync", Type: "bool"},
				{Key: "updateNative", Name: "Update Native", Description: "Update native dependencies", Type: "bool"},
				{Key: "podInstall", Name: "Pod Install", Description: "Run pod install on iOS sync", Type: "bool"},
				{Key: "incrementalSync", Name: "Incremental Sync", Description: "Only sync changed files (experimental)", Type: "bool"},
			},
		},
		{
			Name: "Live Reload",
			Icon: "⚡",
			Settings: []SettingInfo{
				{Key: "externalHost", Name: "External Host", Description: "External IP for live reload (empty = auto)", Type: "string"},
				{Key: "liveReloadPort", Name: "Port", Description: "Port for live reload server", Type: "int"},
				{Key: "enableHotReload", Name: "Hot Reload", Description: "Enable hot module replacement", Type: "bool"},
				{Key: "preserveState", Name: "Preserve State", Description: "Preserve app state on reload", Type: "bool"},
			},
		},
		{
			Name: "UI",
			Icon: "🎨",
			Settings: []SettingInfo{
				{Key: "showSpinners", Name: "Show Spinners", Description: "Show animated process spinners", Type: "bool"},
				{Key: "compactMode", Name: "Compact Mode", Description: "Use compact UI layout", Type: "bool"},
				{Key: "showTimestamps", Name: "Timestamps", Description: "Show timestamps in logs", Type: "bool"},
				{Key: "maxLogLines", Name: "Max Log Lines", Description: "Maximum lines per process", Type: "int"},
				{Key: "showDeviceIcons", Name: "Device Icons", Description: "Show emoji icons for devices", Type: "bool"},
				{Key: "showPlatformBadges", Name: "Platform Badges", Description: "Show iOS/Android badges", Type: "bool"},
				{Key: "colorTheme", Name: "Color Theme", Description: "UI color theme", Type: "choice", Choices: []string{"dark", "light", "system"}},
			},
		},
		{
			Name: "Behavior",
			Icon: "⚙",
			Settings: []SettingInfo{
				{Key: "confirmBeforeKill", Name: "Confirm Kill", Description: "Confirm before killing process", Type: "bool"},
				{Key: "autoScrollLogs", Name: "Auto Scroll", Description: "Auto-scroll logs to bottom", Type: "bool"},
				{Key: "refreshOnFocus", Name: "Refresh on Focus", Description: "Refresh devices on focus change", Type: "bool"},
				{Key: "checkForUpgrades", Name: "Check Upgrades", Description: "Check for Capacitor upgrades", Type: "bool"},
				{Key: "notifyOnComplete", Name: "Notify on Complete", Description: "System notification when done", Type: "bool"},
				{Key: "soundOnComplete", Name: "Sound on Complete", Description: "Play sound when done", Type: "bool"},
				{Key: "autoOpenIde", Name: "Auto Open IDE", Description: "Open IDE on build error", Type: "bool"},
				{Key: "keepProcessHistory", Name: "Process History", Description: "Old processes to keep", Type: "int"},
			},
		},
		{
			Name: "Paths",
			Icon: "📁",
			Settings: []SettingInfo{
				{Key: "nodePath", Name: "Node Path", Description: "Custom node executable path", Type: "string"},
				{Key: "npmPath", Name: "npm Path", Description: "Custom npm executable path", Type: "string"},
				{Key: "npxPath", Name: "npx Path", Description: "Custom npx executable path", Type: "string"},
				{Key: "podPath", Name: "Pod Path", Description: "Custom pod executable path", Type: "string"},
				{Key: "xcodePath", Name: "Xcode Path", Description: "Custom Xcode path", Type: "string"},
				{Key: "shellPath", Name: "Shell Path", Description: "Shell for running commands", Type: "string"},
				{Key: "workingDirectory", Name: "Working Directory", Description: "Override working directory", Type: "string"},
			},
		},
		{
			Name: "Hooks",
			Icon: "🪝",
			Settings: []SettingInfo{
				{Key: "preRunCommand", Name: "Pre-Run", Description: "Command to run before each run", Type: "string"},
				{Key: "postRunCommand", Name: "Post-Run", Description: "Command to run after each run", Type: "string"},
				{Key: "preBuildCommand", Name: "Pre-Build", Description: "Command to run before build", Type: "string"},
				{Key: "postBuildCommand", Name: "Post-Build", Description: "Command to run after build", Type: "string"},
			},
		},
		{
			Name: "Advanced",
			Icon: "🔧",
			Settings: []SettingInfo{
				{Key: "verboseLogging", Name: "Verbose Logging", Description: "Enable verbose output", Type: "bool"},
				{Key: "debugMode", Name: "Debug Mode", Description: "Enable debug mode", Type: "bool"},
				{Key: "disableTelemetry", Name: "Disable Telemetry", Description: "Disable usage telemetry", Type: "bool"},
				{Key: "capacitorConfigPath", Name: "Config Path", Description: "Custom capacitor config path", Type: "string"},
				{Key: "enableBetaFeatures", Name: "Beta Features", Description: "Enable experimental features", Type: "bool"},
				{Key: "parallelBuilds", Name: "Parallel Builds", Description: "Build platforms in parallel", Type: "bool"},
				{Key: "cacheBuilds", Name: "Cache Builds", Description: "Cache build artifacts", Type: "bool"},
			},
		},
	}
}

// GetAllSettings returns a flat list of all settings
func GetAllSettings() []SettingInfo {
	var all []SettingInfo
	for _, cat := range GetCategories() {
		all = append(all, cat.Settings...)
	}
	return all
}

// GetBool gets a boolean setting by key
func (s *Settings) GetBool(key string) bool {
	switch key {
	case "liveReloadDefault":
		return s.LiveReloadDefault
	case "autoSync":
		return s.AutoSync
	case "autoBuild":
		return s.AutoBuild
	case "runInBackground":
		return s.RunInBackground
	case "clearLogsOnRun":
		return s.ClearLogsOnRun
	case "productionBuild":
		return s.ProductionBuild
	case "sourceMaps":
		return s.SourceMaps
	case "minifyBuilds":
		return s.MinifyBuilds
	case "iosAutoSigning":
		return s.IOSAutoSigningBool
	case "syncOnSave":
		return s.SyncOnSave
	case "copyWebDir":
		return s.CopyWebDir
	case "updateNative":
		return s.UpdateNative
	case "podInstall":
		return s.PodInstall
	case "incrementalSync":
		return s.IncrementalSync
	case "enableHotReload":
		return s.EnableHotReload
	case "preserveState":
		return s.PreserveState
	case "showSpinners":
		return s.ShowSpinners
	case "compactMode":
		return s.CompactMode
	case "showTimestamps":
		return s.ShowTimestamps
	case "showDeviceIcons":
		return s.ShowDeviceIcons
	case "showPlatformBadges":
		return s.ShowPlatformBadges
	case "confirmBeforeKill":
		return s.ConfirmBeforeKill
	case "autoScrollLogs":
		return s.AutoScrollLogs
	case "refreshOnFocus":
		return s.RefreshOnFocus
	case "checkForUpgrades":
		return s.CheckForUpgrades
	case "notifyOnComplete":
		return s.NotifyOnComplete
	case "soundOnComplete":
		return s.SoundOnComplete
	case "autoOpenIde":
		return s.AutoOpenIDE
	case "verboseLogging":
		return s.VerboseLogging
	case "debugMode":
		return s.DebugMode
	case "disableTelemetry":
		return s.DisableTelemetry
	case "enableBetaFeatures":
		return s.EnableBetaFeatures
	case "parallelBuilds":
		return s.ParallelBuilds
	case "cacheBuilds":
		return s.CacheBuilds
	case "inlineSourceMaps":
		return s.InlineSourceMaps
	case "webOpenBrowser":
		return s.WebOpenBrowser
	case "webHttps":
		return s.WebHttps
	}
	return false
}

// SetBool sets a boolean setting by key
func (s *Settings) SetBool(key string, value bool) {
	switch key {
	case "liveReloadDefault":
		s.LiveReloadDefault = value
	case "autoSync":
		s.AutoSync = value
	case "autoBuild":
		s.AutoBuild = value
	case "runInBackground":
		s.RunInBackground = value
	case "clearLogsOnRun":
		s.ClearLogsOnRun = value
	case "productionBuild":
		s.ProductionBuild = value
	case "sourceMaps":
		s.SourceMaps = value
	case "minifyBuilds":
		s.MinifyBuilds = value
	case "iosAutoSigning":
		s.IOSAutoSigningBool = value
	case "syncOnSave":
		s.SyncOnSave = value
	case "copyWebDir":
		s.CopyWebDir = value
	case "updateNative":
		s.UpdateNative = value
	case "podInstall":
		s.PodInstall = value
	case "incrementalSync":
		s.IncrementalSync = value
	case "enableHotReload":
		s.EnableHotReload = value
	case "preserveState":
		s.PreserveState = value
	case "showSpinners":
		s.ShowSpinners = value
	case "compactMode":
		s.CompactMode = value
	case "showTimestamps":
		s.ShowTimestamps = value
	case "showDeviceIcons":
		s.ShowDeviceIcons = value
	case "showPlatformBadges":
		s.ShowPlatformBadges = value
	case "confirmBeforeKill":
		s.ConfirmBeforeKill = value
	case "autoScrollLogs":
		s.AutoScrollLogs = value
	case "refreshOnFocus":
		s.RefreshOnFocus = value
	case "checkForUpgrades":
		s.CheckForUpgrades = value
	case "notifyOnComplete":
		s.NotifyOnComplete = value
	case "soundOnComplete":
		s.SoundOnComplete = value
	case "autoOpenIde":
		s.AutoOpenIDE = value
	case "verboseLogging":
		s.VerboseLogging = value
	case "debugMode":
		s.DebugMode = value
	case "disableTelemetry":
		s.DisableTelemetry = value
	case "enableBetaFeatures":
		s.EnableBetaFeatures = value
	case "parallelBuilds":
		s.ParallelBuilds = value
	case "cacheBuilds":
		s.CacheBuilds = value
	case "inlineSourceMaps":
		s.InlineSourceMaps = value
	case "webOpenBrowser":
		s.WebOpenBrowser = value
	case "webHttps":
		s.WebHttps = value
	}
}

// GetString gets a string setting by key
func (s *Settings) GetString(key string) string {
	switch key {
	case "defaultPlatform":
		return s.DefaultPlatform
	case "buildCommand":
		return s.BuildCommand
	case "iosScheme":
		return s.IOSScheme
	case "iosConfiguration":
		return s.IOSConfiguration
	case "iosSimulator":
		return s.IOSSimulator
	case "iosTeamId":
		return s.IOSTeamID
	case "iosDerivedData":
		return s.IOSDerivedData
	case "iosCodeSignIdentity":
		return s.IOSCodeSignIdentity
	case "androidDevice":
		return s.AndroidDevice
	case "androidFlavor":
		return s.AndroidFlavor
	case "androidBuildType":
		return s.AndroidBuildType
	case "androidKeystorePath":
		return s.AndroidKeystorePath
	case "androidSdkPath":
		return s.AndroidSDKPath
	case "androidStudioPath":
		return s.AndroidStudioPath
	case "externalHost":
		return s.ExternalHost
	case "colorTheme":
		return s.ColorTheme
	case "logFontSize":
		return s.LogFontSize
	case "nodePath":
		return s.NodePath
	case "npmPath":
		return s.NpmPath
	case "npxPath":
		return s.NpxPath
	case "podPath":
		return s.PodPath
	case "xcodePath":
		return s.XcodePath
	case "shellPath":
		return s.ShellPath
	case "workingDirectory":
		return s.WorkingDirectory
	case "preRunCommand":
		return s.PreRunCommand
	case "postRunCommand":
		return s.PostRunCommand
	case "preBuildCommand":
		return s.PreBuildCommand
	case "postBuildCommand":
		return s.PostBuildCommand
	case "capacitorConfigPath":
		return s.CapacitorConfigPath
	case "webDevCommand":
		return s.WebDevCommand
	case "webBrowserPath":
		return s.WebBrowserPath
	case "webHost":
		return s.WebHost
	}
	return ""
}

// SetString sets a string setting by key
func (s *Settings) SetString(key string, value string) {
	switch key {
	case "defaultPlatform":
		s.DefaultPlatform = value
	case "buildCommand":
		s.BuildCommand = value
	case "iosScheme":
		s.IOSScheme = value
	case "iosConfiguration":
		s.IOSConfiguration = value
	case "iosSimulator":
		s.IOSSimulator = value
	case "iosTeamId":
		s.IOSTeamID = value
	case "iosDerivedData":
		s.IOSDerivedData = value
	case "iosCodeSignIdentity":
		s.IOSCodeSignIdentity = value
	case "androidDevice":
		s.AndroidDevice = value
	case "androidFlavor":
		s.AndroidFlavor = value
	case "androidBuildType":
		s.AndroidBuildType = value
	case "androidKeystorePath":
		s.AndroidKeystorePath = value
	case "androidSdkPath":
		s.AndroidSDKPath = value
	case "androidStudioPath":
		s.AndroidStudioPath = value
	case "externalHost":
		s.ExternalHost = value
	case "colorTheme":
		s.ColorTheme = value
	case "logFontSize":
		s.LogFontSize = value
	case "nodePath":
		s.NodePath = value
	case "npmPath":
		s.NpmPath = value
	case "npxPath":
		s.NpxPath = value
	case "podPath":
		s.PodPath = value
	case "xcodePath":
		s.XcodePath = value
	case "shellPath":
		s.ShellPath = value
	case "workingDirectory":
		s.WorkingDirectory = value
	case "preRunCommand":
		s.PreRunCommand = value
	case "postRunCommand":
		s.PostRunCommand = value
	case "preBuildCommand":
		s.PreBuildCommand = value
	case "postBuildCommand":
		s.PostBuildCommand = value
	case "capacitorConfigPath":
		s.CapacitorConfigPath = value
	case "webDevCommand":
		s.WebDevCommand = value
	case "webBrowserPath":
		s.WebBrowserPath = value
	case "webHost":
		s.WebHost = value
	}
}

// GetInt gets an int setting by key
func (s *Settings) GetInt(key string) int {
	switch key {
	case "liveReloadPort":
		return s.LiveReloadPort
	case "buildTimeout":
		return s.BuildTimeout
	case "maxLogLines":
		return s.MaxLogLines
	case "syncTimeout":
		return s.SyncTimeout
	case "keepProcessHistory":
		return s.KeepProcessHistory
	case "webDevPort":
		return s.WebDevPort
	}
	return 0
}

// SetInt sets an int setting by key
func (s *Settings) SetInt(key string, value int) {
	switch key {
	case "liveReloadPort":
		s.LiveReloadPort = value
	case "buildTimeout":
		s.BuildTimeout = value
	case "maxLogLines":
		s.MaxLogLines = value
	case "syncTimeout":
		s.SyncTimeout = value
	case "keepProcessHistory":
		s.KeepProcessHistory = value
	case "webDevPort":
		s.WebDevPort = value
	}
}

// ToggleBool toggles a boolean setting
func (s *Settings) ToggleBool(key string) bool {
	newValue := !s.GetBool(key)
	s.SetBool(key, newValue)
	return newValue
}

// CycleChoice cycles through choices for a choice setting
func (s *Settings) CycleChoice(key string, choices []string) string {
	current := s.GetString(key)
	for i, choice := range choices {
		if choice == current {
			next := (i + 1) % len(choices)
			s.SetString(key, choices[next])
			return choices[next]
		}
	}
	// If current not found, set to first
	if len(choices) > 0 {
		s.SetString(key, choices[0])
		return choices[0]
	}
	return ""
}

// ConfigPath returns the path to the config file
func ConfigPath() (string, error) {
	return configPath()
}
