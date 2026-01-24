package lazycap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/icarus-itcs/lazycap/internal/cap"
	"github.com/icarus-itcs/lazycap/internal/debug"
	"github.com/icarus-itcs/lazycap/internal/plugin"
	"github.com/icarus-itcs/lazycap/internal/plugins"
	"github.com/icarus-itcs/lazycap/internal/settings"
	"github.com/icarus-itcs/lazycap/internal/ui"
)

var (
	appVersion string
	appCommit  string
	appDate    string
	demoMode   bool
)

var rootCmd = &cobra.Command{
	Use:   "lazycap",
	Short: "A slick terminal UI for Capacitor & Ionic development",
	Long: `lazycap is a terminal UI for Capacitor/Ionic mobile development.
Manage devices, emulators, builds, and live reload from one beautiful interface.

Navigate to your Capacitor project directory and run 'lazycap' to get started.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if demoMode {
			return runDemoMode()
		}
		return runApp()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("lazycap %s\n", appVersion)
		fmt.Printf("  commit: %s\n", appCommit)
		fmt.Printf("  built:  %s\n", appDate)
	},
}

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List available devices and emulators",
	RunE: func(cmd *cobra.Command, args []string) error {
		devices, err := cap.ListDevices()
		if err != nil {
			return err
		}
		for _, d := range devices {
			status := "offline"
			if d.Online {
				status = "online"
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", d.ID, d.Name, d.Platform, status)
		}
		return nil
	},
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run MCP server for AI assistant integration",
	Long: `Run lazycap as an MCP (Model Context Protocol) server.
This allows AI assistants like Claude to control lazycap.

Add to your Claude Code settings (~/.claude/settings.json):

{
  "mcpServers": {
    "lazycap": {
      "command": "lazycap",
      "args": ["mcp"]
    }
  }
}`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMCPServer()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(devicesCmd)
	rootCmd.AddCommand(mcpCmd)

	// Global flags
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file (default: .lazycap.yaml)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.Flags().BoolVar(&demoMode, "demo", false, "run in demo mode with mock data (for screenshots)")
}

func Execute(version, commit, date string) error {
	appVersion = version
	appCommit = commit
	appDate = date
	rootCmd.Version = version
	return rootCmd.Execute()
}

func runDemoMode() error {
	// Create mock project
	project := &cap.Project{
		Name:       "my-awesome-app",
		RootDir:    "/demo",
		HasIOS:     true,
		HasAndroid: true,
	}

	// Register plugins (still useful for demo)
	_ = plugins.RegisterAll()
	pluginManager := plugin.NewManager()
	appContext := plugin.NewAppContext(pluginManager)
	appContext.SetProject(project)

	// Create model with demo devices
	model := ui.NewDemoModel(project, pluginManager, appContext, appVersion)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running demo: %w", err)
	}

	return nil
}

func runApp() error {
	// Discover Capacitor projects (supports monorepos with nested projects)
	// Searches current directory and up to 4 levels deep
	projects, err := cap.DiscoverProjects(4)
	if err != nil {
		// Log warning but continue - we may have found some projects
		fmt.Fprintf(os.Stderr, "Warning: error during project discovery: %v\n", err)
	}

	if len(projects) == 0 {
		return fmt.Errorf("no Capacitor projects found\n\nMake sure you're in a directory containing capacitor.config.ts/js/json\nor in a parent directory of Capacitor projects (monorepo support)")
	}

	// Use first project as active (UI will show selector if multiple)
	project := projects[0]

	// Register all built-in plugins
	if err := plugins.RegisterAll(); err != nil {
		return fmt.Errorf("failed to register plugins: %w", err)
	}

	// Create plugin manager
	pluginManager := plugin.NewManager()

	// Create plugin context (bridges plugins with UI)
	appContext := plugin.NewAppContext(pluginManager)
	appContext.SetProject(project)

	// Initialize and run the TUI with plugin support (pass all discovered projects)
	model := ui.NewModelWithProjects(projects, pluginManager, appContext, appVersion)
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Handle graceful shutdown for plugins
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		// Save which plugins were running before stopping
		for _, p := range plugin.All() {
			_ = pluginManager.SetRunning(p.ID(), p.IsRunning())
		}
		_ = pluginManager.StopAll()
		os.Exit(0)
	}()

	// Initialize all plugins with context
	if err := pluginManager.InitAll(appContext); err != nil {
		// Log but don't fail - plugins are optional
		fmt.Fprintf(os.Stderr, "Warning: some plugins failed to initialize: %v\n", err)
	}

	// Start auto-start plugins
	pluginManager.StartAutoStart()

	// Run the TUI
	// Note: gracefulShutdown() in the model handles saving plugin state and stopping them
	// when user quits via 'q' or ctrl+c within the TUI
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running app: %w", err)
	}

	// If we get here, the TUI exited cleanly (gracefulShutdown already handled plugin state)
	return nil
}

// runMCPServer runs lazycap as a standalone MCP server in stdio mode
func runMCPServer() error {
	// Check if stdin is a terminal (interactive mode)
	if isTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(os.Stderr, "lazycap MCP server running in stdio mode.")
		fmt.Fprintln(os.Stderr, "This server is designed for AI assistant integration.")
		fmt.Fprintln(os.Stderr, "Add to your Claude Code settings (~/.claude/settings.json):")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, `  "mcpServers": { "lazycap": { "command": "lazycap", "args": ["mcp"] } }`)
		fmt.Fprintln(os.Stderr, "")
	}

	// Discover all Capacitor projects (current dir + subdirectories)
	projects, _ := cap.DiscoverProjects(3)

	if isTerminal(os.Stdin.Fd()) {
		if len(projects) == 0 {
			fmt.Fprintln(os.Stderr, "No Capacitor projects found. Some tools may not work.")
		} else {
			fmt.Fprintf(os.Stderr, "Found %d Capacitor project(s):\n", len(projects))
			for _, p := range projects {
				fmt.Fprintf(os.Stderr, "  - %s (%s)\n", p.Name, p.RootDir)
			}
		}
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Waiting for JSON-RPC input on stdin... (Ctrl+C to exit)")
	}

	// Load settings
	userSettings, _ := settings.Load()

	// Create a simple MCP context that doesn't need the full UI
	mcpCtx := &mcpContext{
		projects: projects,
		settings: userSettings,
	}

	// Run MCP server in stdio mode
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Text()
		// Skip empty lines
		if line == "" {
			continue
		}
		response := handleMCPRequest(mcpCtx, line)
		_ = encoder.Encode(response)
	}

	return nil
}

// isTerminal checks if the given file descriptor is a terminal
func isTerminal(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// mcpContext provides context for MCP operations without the full UI
type mcpContext struct {
	projects []*cap.Project
	settings *settings.Settings
}

// getProject returns a project by name or path, or the first/only project if not specified
func (ctx *mcpContext) getProject(nameOrPath string) *cap.Project {
	if len(ctx.projects) == 0 {
		return nil
	}

	// If no name specified, return first project
	if nameOrPath == "" {
		return ctx.projects[0]
	}

	// Search by name or path
	for _, p := range ctx.projects {
		if p.Name == nameOrPath || p.RootDir == nameOrPath {
			return p
		}
		// Also check if it's a relative path match
		if strings.HasSuffix(p.RootDir, "/"+nameOrPath) || strings.HasSuffix(p.RootDir, "\\"+nameOrPath) {
			return p
		}
	}

	return nil
}

// MCP types
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func handleMCPRequest(ctx *mcpContext, line string) mcpResponse {
	var req mcpRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32700, Message: "Parse error"},
		}
	}

	response := mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		response.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]interface{}{
				"name":    "lazycap",
				"version": appVersion,
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		}
	case "notifications/initialized":
		// Client acknowledged initialization, no response needed
		response.Result = map[string]interface{}{}
	case "tools/list":
		response.Result = handleToolsList(ctx)
	case "tools/call":
		response.Result, response.Error = handleToolsCall(ctx, req.Params)
	default:
		response.Error = &mcpError{Code: -32601, Message: "Method not found: " + req.Method}
	}

	return response
}

func handleToolsList(ctx *mcpContext) map[string]interface{} {
	allTools := []map[string]interface{}{
		{
			"name":        "list_projects",
			"description": "List all discovered Capacitor projects in the current directory and subdirectories",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "list_devices",
			"description": "List all available devices, emulators, and simulators for running the app",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "run_on_device",
			"description": "Run the Capacitor app on a specific device or emulator",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name or path (from list_projects). Optional if only one project exists.",
					},
					"deviceId": map[string]interface{}{
						"type":        "string",
						"description": "Device ID to run on (get from list_devices)",
					},
					"platform": map[string]interface{}{
						"type":        "string",
						"description": "Platform: ios, android, or web",
					},
					"liveReload": map[string]interface{}{
						"type":        "boolean",
						"description": "Enable live reload for development",
					},
				},
				"required": []string{"deviceId", "platform"},
			},
		},
		{
			"name":        "sync",
			"description": "Sync web assets to native iOS/Android projects",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name or path (from list_projects). Optional if only one project exists.",
					},
					"platform": map[string]interface{}{
						"type":        "string",
						"description": "Platform to sync: ios, android, or empty for all",
					},
				},
			},
		},
		{
			"name":        "build",
			"description": "Build the web assets (npm run build)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name or path (from list_projects). Optional if only one project exists.",
					},
				},
			},
		},
		{
			"name":        "open_ide",
			"description": "Open the native project in Xcode (iOS) or Android Studio (Android)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name or path (from list_projects). Optional if only one project exists.",
					},
					"platform": map[string]interface{}{
						"type":        "string",
						"description": "Platform: ios or android",
					},
				},
				"required": []string{"platform"},
			},
		},
		{
			"name":        "get_project",
			"description": "Get information about a Capacitor project",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name or path (from list_projects). Optional if only one project exists.",
					},
				},
			},
		},
		{
			"name":        "get_debug_actions",
			"description": "List available debug and cleanup actions",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "run_debug_action",
			"description": "Run a debug or cleanup action (like clearing caches, killing processes)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"actionId": map[string]interface{}{
						"type":        "string",
						"description": "Action ID from get_debug_actions",
					},
				},
				"required": []string{"actionId"},
			},
		},
		{
			"name":        "get_all_logs",
			"description": "Get logs from lazycap debug log file. Use this to diagnose build errors, runtime issues, or understand what happened. Supports filtering by text search and limiting output.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"search": map[string]interface{}{
						"type":        "string",
						"description": "Search for text pattern in logs (case-insensitive)",
					},
					"errors_only": map[string]interface{}{
						"type":        "boolean",
						"description": "Only return lines containing error indicators (error, failed, exception, panic, fatal)",
					},
					"tail": map[string]interface{}{
						"type":        "integer",
						"description": "Number of lines to return from end of log (default: 100)",
					},
				},
			},
		},
		{
			"name":        "run_command",
			"description": "Run a shell command in a Capacitor project directory and return the output. Use for npm commands, checking files, or running custom scripts.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name or path (from list_projects). Optional if only one project exists.",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Shell command to run",
					},
				},
				"required": []string{"command"},
			},
		},
	}

	// Filter tools based on settings
	var tools []map[string]interface{}
	for _, tool := range allTools {
		toolName, _ := tool["name"].(string)
		if ctx.settings.IsMCPToolEnabled(toolName) {
			tools = append(tools, tool)
		}
	}

	return map[string]interface{}{
		"tools": tools,
	}
}

func handleToolsCall(ctx *mcpContext, params json.RawMessage) (interface{}, *mcpError) {
	var call struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &mcpError{Code: -32602, Message: "Invalid params"}
	}

	// Check if the tool is enabled
	if !ctx.settings.IsMCPToolEnabled(call.Name) {
		return nil, &mcpError{Code: -32601, Message: fmt.Sprintf("Tool '%s' is disabled. Enable it in lazycap settings.", call.Name)}
	}

	switch call.Name {
	case "list_projects":
		if len(ctx.projects) == 0 {
			return mcpContent("No Capacitor projects found. Make sure you're in or near a Capacitor project directory."), nil
		}
		result := make([]map[string]interface{}, len(ctx.projects))
		for i, p := range ctx.projects {
			result[i] = map[string]interface{}{
				"name":       p.Name,
				"appId":      p.AppID,
				"rootDir":    p.RootDir,
				"hasIOS":     p.HasIOS,
				"hasAndroid": p.HasAndroid,
			}
		}
		return mcpContent(toJSON(result)), nil

	case "list_devices":
		devices, err := cap.ListDevices()
		if err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		result := make([]map[string]interface{}, len(devices))
		for i, d := range devices {
			result[i] = map[string]interface{}{
				"id":         d.ID,
				"name":       d.Name,
				"platform":   d.Platform,
				"online":     d.Online,
				"isEmulator": d.IsEmulator,
			}
		}
		return mcpContent(toJSON(result)), nil

	case "run_on_device":
		projectName, _ := call.Arguments["project"].(string)
		deviceID, _ := call.Arguments["deviceId"].(string)
		platform, _ := call.Arguments["platform"].(string)
		liveReload, _ := call.Arguments["liveReload"].(bool)
		if deviceID == "" || platform == "" {
			return nil, &mcpError{Code: -32602, Message: "deviceId and platform required"}
		}
		project := ctx.getProject(projectName)
		if project == nil {
			return nil, &mcpError{Code: -32000, Message: "No Capacitor project found. Use list_projects to see available projects."}
		}
		if err := cap.RunAt(project.RootDir, deviceID, platform, liveReload); err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		return mcpContent(fmt.Sprintf("Started app '%s' on device %s", project.Name, deviceID)), nil

	case "sync":
		projectName, _ := call.Arguments["project"].(string)
		platform, _ := call.Arguments["platform"].(string)
		project := ctx.getProject(projectName)
		if project == nil {
			return nil, &mcpError{Code: -32000, Message: "No Capacitor project found. Use list_projects to see available projects."}
		}
		if err := cap.SyncAt(project.RootDir, platform); err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		msg := fmt.Sprintf("Sync completed for '%s'", project.Name)
		if platform != "" {
			msg = fmt.Sprintf("Sync completed for '%s' (%s)", project.Name, platform)
		}
		return mcpContent(msg), nil

	case "build":
		projectName, _ := call.Arguments["project"].(string)
		project := ctx.getProject(projectName)
		if project == nil {
			return nil, &mcpError{Code: -32000, Message: "No Capacitor project found. Use list_projects to see available projects."}
		}
		if err := cap.BuildAt(project.RootDir); err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		return mcpContent(fmt.Sprintf("Build completed for '%s'", project.Name)), nil

	case "open_ide":
		projectName, _ := call.Arguments["project"].(string)
		platform, _ := call.Arguments["platform"].(string)
		if platform == "" {
			return nil, &mcpError{Code: -32602, Message: "platform required"}
		}
		project := ctx.getProject(projectName)
		if project == nil {
			return nil, &mcpError{Code: -32000, Message: "No Capacitor project found. Use list_projects to see available projects."}
		}
		if err := cap.OpenAt(project.RootDir, platform); err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		return mcpContent(fmt.Sprintf("Opened %s IDE for '%s'", platform, project.Name)), nil

	case "get_project":
		projectName, _ := call.Arguments["project"].(string)
		project := ctx.getProject(projectName)
		if project == nil {
			return nil, &mcpError{Code: -32000, Message: "No Capacitor project found. Use list_projects to see available projects."}
		}
		result := map[string]interface{}{
			"name":       project.Name,
			"appId":      project.AppID,
			"rootDir":    project.RootDir,
			"webDir":     project.WebDir,
			"hasIOS":     project.HasIOS,
			"hasAndroid": project.HasAndroid,
		}
		return mcpContent(toJSON(result)), nil

	case "get_debug_actions":
		actions := debug.GetActions()
		result := make([]map[string]interface{}, len(actions))
		for i, a := range actions {
			result[i] = map[string]interface{}{
				"id":          a.ID,
				"name":        a.Name,
				"description": a.Description,
				"category":    a.Category,
				"dangerous":   a.Dangerous,
			}
		}
		return mcpContent(toJSON(result)), nil

	case "run_debug_action":
		actionID, _ := call.Arguments["actionId"].(string)
		if actionID == "" {
			return nil, &mcpError{Code: -32602, Message: "actionId required"}
		}
		result := debug.RunAction(actionID)
		if !result.Success {
			return nil, &mcpError{Code: -32000, Message: result.Message}
		}
		return mcpContent(result.Message), nil

	case "get_all_logs":
		return handleGetAllLogs(call.Arguments)

	case "run_command":
		return handleRunCommand(ctx, call.Arguments)

	default:
		return nil, &mcpError{Code: -32601, Message: "Unknown tool: " + call.Name}
	}
}

// handleGetAllLogs reads and filters the lazycap debug log
func handleGetAllLogs(args map[string]interface{}) (interface{}, *mcpError) {
	logPath := "/tmp/lazycap-debug.log"

	// Read log file
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return mcpContent("No debug log found at " + logPath + ". Run lazycap TUI first to generate logs."), nil
		}
		return nil, &mcpError{Code: -32000, Message: "Failed to read log: " + err.Error()}
	}

	lines := strings.Split(string(data), "\n")

	// Parse filter arguments
	searchPattern, _ := args["search"].(string)
	errorsOnly, _ := args["errors_only"].(bool)
	tail := 100 // default
	if t, ok := args["tail"].(float64); ok {
		tail = int(t)
	}

	// Error patterns for errors_only filter
	errorPatterns := []string{"error", "Error", "ERROR", "failed", "Failed", "FAILED", "exception", "Exception", "panic", "Panic", "fatal", "Fatal"}

	// Apply filters
	var filtered []string
	for _, line := range lines {
		if line == "" {
			continue
		}

		// Apply search filter
		if searchPattern != "" && !containsIgnoreCase(line, searchPattern) {
			continue
		}

		// Apply errors_only filter
		if errorsOnly {
			hasError := false
			for _, pattern := range errorPatterns {
				if strings.Contains(line, pattern) {
					hasError = true
					break
				}
			}
			if !hasError {
				continue
			}
		}

		filtered = append(filtered, line)
	}

	// Apply tail limit
	if tail > 0 && len(filtered) > tail {
		filtered = filtered[len(filtered)-tail:]
	}

	if len(filtered) == 0 {
		return mcpContent("No matching log entries found."), nil
	}

	return mcpContent(strings.Join(filtered, "\n")), nil
}

// handleRunCommand runs a shell command in the project directory
func handleRunCommand(ctx *mcpContext, args map[string]interface{}) (interface{}, *mcpError) {
	projectName, _ := args["project"].(string)
	command, _ := args["command"].(string)

	if command == "" {
		return nil, &mcpError{Code: -32602, Message: "command required"}
	}

	// Get working directory
	workDir := ""
	project := ctx.getProject(projectName)
	if project != nil {
		workDir = project.RootDir
	}

	// Run command
	output, err := cap.RunShellCommand(workDir, command)
	if err != nil {
		return mcpContent(fmt.Sprintf("Command failed: %s\n\nOutput:\n%s", err.Error(), output)), nil
	}

	return mcpContent(output), nil
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func mcpContent(text string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	}
}

func toJSON(v interface{}) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
