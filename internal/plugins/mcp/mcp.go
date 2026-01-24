package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/icarus-itcs/lazycap/internal/plugin"
)

const (
	PluginID      = "mcp-server"
	PluginName    = "MCP Server"
	PluginVersion = "1.0.0"
	PluginAuthor  = "lazycap"
)

// MCPPlugin implements the MCP (Model Context Protocol) server
type MCPPlugin struct {
	mu       sync.RWMutex
	ctx      plugin.Context
	running  bool
	listener net.Listener
	mode     string // "stdio" or "tcp"
	port     int
	stopCh   chan struct{}
}

// New creates a new MCP plugin instance
func New() *MCPPlugin {
	return &MCPPlugin{
		mode:   "tcp",
		port:   9315,
		stopCh: make(chan struct{}),
	}
}

// Register registers the plugin with the global registry
func Register() error {
	return plugin.Register(New())
}

// Plugin interface implementation

func (p *MCPPlugin) ID() string      { return PluginID }
func (p *MCPPlugin) Name() string    { return PluginName }
func (p *MCPPlugin) Version() string { return PluginVersion }
func (p *MCPPlugin) Author() string  { return PluginAuthor }
func (p *MCPPlugin) Description() string {
	return "Exposes lazycap functionality via MCP protocol for AI assistants"
}

func (p *MCPPlugin) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

func (p *MCPPlugin) GetSettings() []plugin.Setting {
	return []plugin.Setting{
		{
			Key:         "mode",
			Name:        "Server Mode",
			Description: "How to expose the MCP server",
			Type:        "choice",
			Default:     "tcp",
			Choices:     []string{"tcp", "stdio"},
		},
		{
			Key:         "port",
			Name:        "TCP Port",
			Description: "Port for TCP mode",
			Type:        "int",
			Default:     9315,
		},
		{
			Key:         "autoStart",
			Name:        "Auto Start",
			Description: "Start server automatically",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (p *MCPPlugin) OnSettingChange(key string, value interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch key {
	case "mode":
		if s, ok := value.(string); ok {
			p.mode = s
		}
	case "port":
		if n, ok := value.(float64); ok {
			p.port = int(n)
		} else if n, ok := value.(int); ok {
			p.port = n
		}
	}
}

func (p *MCPPlugin) GetStatusLine() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.running {
		return ""
	}

	if p.mode == "tcp" {
		return fmt.Sprintf("MCP :%d", p.port)
	}
	return "MCP stdio"
}

func (p *MCPPlugin) GetCommands() []plugin.Command {
	return nil // No custom commands
}

func (p *MCPPlugin) GetProcessIDs() []int {
	return nil // MCP is a TCP server, no external processes
}

func (p *MCPPlugin) Init(ctx plugin.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx

	// Load settings
	if mode := ctx.GetPluginSetting(PluginID, "mode"); mode != nil {
		if s, ok := mode.(string); ok {
			p.mode = s
		}
	}
	if port := ctx.GetPluginSetting(PluginID, "port"); port != nil {
		if n, ok := port.(float64); ok {
			p.port = int(n)
		}
	}

	return nil
}

func (p *MCPPlugin) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = true
	p.stopCh = make(chan struct{})
	mode := p.mode
	port := p.port
	p.mu.Unlock()

	if mode == "stdio" {
		go p.runStdio()
	} else {
		if err := p.startTCP(port); err != nil {
			p.mu.Lock()
			p.running = false
			p.mu.Unlock()
			return err
		}
	}

	p.ctx.Log(PluginID, fmt.Sprintf("MCP server started (mode: %s)", mode))
	return nil
}

func (p *MCPPlugin) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false

	// Signal stop
	close(p.stopCh)

	// Close listener if TCP
	if p.listener != nil {
		_ = p.listener.Close()
		p.listener = nil
	}
	p.mu.Unlock()

	p.ctx.Log(PluginID, "MCP server stopped")
	return nil
}

// TCP server implementation

func (p *MCPPlugin) startTCP(port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	p.mu.Lock()
	p.listener = listener
	p.mu.Unlock()

	go p.acceptConnections(listener)
	return nil
}

func (p *MCPPlugin) acceptConnections(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-p.stopCh:
				return
			default:
				continue
			}
		}
		go p.handleConnection(conn)
	}
}

func (p *MCPPlugin) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)

	for scanner.Scan() {
		select {
		case <-p.stopCh:
			return
		default:
		}

		line := scanner.Text()
		response := p.handleRequest(line)
		_ = encoder.Encode(response)
	}
}

// Stdio server implementation

func (p *MCPPlugin) runStdio() {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		select {
		case <-p.stopCh:
			return
		default:
		}

		line := scanner.Text()
		response := p.handleRequest(line)
		_ = encoder.Encode(response)
	}
}

// MCP Protocol types

type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// handleRequest processes an MCP request and returns a response
func (p *MCPPlugin) handleRequest(line string) MCPResponse {
	var req MCPRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			Error:   &MCPError{Code: -32700, Message: "Parse error"},
		}
	}

	response := MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		response.Result = p.handleInitialize()
	case "tools/list":
		response.Result = p.handleToolsList()
	case "tools/call":
		response.Result, response.Error = p.handleToolsCall(req.Params)
	default:
		response.Error = &MCPError{Code: -32601, Message: "Method not found"}
	}

	return response
}

func (p *MCPPlugin) handleInitialize() map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]interface{}{
			"name":        "lazycap",
			"version":     PluginVersion,
			"description": "Capacitor/Ionic mobile app development tools - controls native builds, device deployment, emulators, and Firebase services",
		},
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
	}
}

func (p *MCPPlugin) handleToolsList() map[string]interface{} {
	tools := []ToolInfo{
		{
			Name:        "list_devices",
			Description: "[Capacitor] List iOS Simulators, Android Emulators, and physical devices available for app deployment. Shows device name, platform, OS version, and online status.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "run_on_device",
			Description: "[Capacitor] Deploy and run the app on a device/emulator using 'npx cap run'. Builds web assets, syncs to native platform, compiles native code, and launches on the target device.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"deviceId": map[string]interface{}{
						"type":        "string",
						"description": "Device ID from list_devices (e.g., 'iPhone-15-Pro' or 'emulator-5554')",
					},
					"liveReload": map[string]interface{}{
						"type":        "boolean",
						"description": "Enable live reload - app auto-refreshes when web code changes",
					},
				},
				"required": []string{"deviceId"},
			},
		},
		{
			Name:        "run_web",
			Description: "[Web Dev] Start the web development server (npm run dev / ionic serve). Opens the app in browser for rapid web development without native compilation.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "sync",
			Description: "[Capacitor] Run 'npx cap sync' to copy web assets (HTML/CSS/JS) to native iOS/Android projects and update native plugins. Required after npm install or web code changes before native build.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"platform": map[string]interface{}{
						"type":        "string",
						"description": "Platform to sync: 'ios', 'android', or omit for both platforms",
					},
				},
			},
		},
		{
			Name:        "build",
			Description: "[Web Build] Run the web build command (npm run build) to compile and bundle the web application. Creates production-ready assets in the build output directory.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "open_ide",
			Description: "[Capacitor] Open native project in IDE using 'npx cap open'. Opens Xcode for iOS development or Android Studio for Android development.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"platform": map[string]interface{}{
						"type":        "string",
						"description": "'ios' to open Xcode, 'android' to open Android Studio",
					},
				},
				"required": []string{"platform"},
			},
		},
		{
			Name:        "get_processes",
			Description: "[Process Manager] List all running and completed processes including Capacitor builds, syncs, device runs, web server, and Firebase emulators. Shows process ID, name, status, and runtime.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_logs",
			Description: "[Process Manager] Get output logs for a specific process. Use to see build output, compilation errors, runtime logs, or Firebase emulator output.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"processId": map[string]interface{}{
						"type":        "string",
						"description": "Process ID from get_processes",
					},
				},
				"required": []string{"processId"},
			},
		},
		{
			Name:        "get_all_logs",
			Description: "[Process Manager] Get logs from all processes with filtering. Use to diagnose Capacitor build errors, Xcode/Gradle compilation failures, runtime crashes, or Firebase issues.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Filter by type: 'build' (web build), 'sync' (cap sync), 'run' (device deployment), 'web' (dev server), 'firebase' (emulators)",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"running", "success", "failed", "canceled"},
						"description": "Filter by process status",
					},
					"search": map[string]interface{}{
						"type":        "string",
						"description": "Search for text pattern in logs (case-insensitive). Find specific errors or messages.",
					},
					"errors_only": map[string]interface{}{
						"type":        "boolean",
						"description": "Only return error lines (error, failed, exception, panic, fatal)",
					},
					"tail": map[string]interface{}{
						"type":        "integer",
						"description": "Limit to last N lines per process",
					},
				},
			},
		},
		{
			Name:        "kill_process",
			Description: "[Process Manager] Terminate a running process. Use to stop a stuck build, kill the web dev server, or stop Firebase emulators.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"processId": map[string]interface{}{
						"type":        "string",
						"description": "Process ID from get_processes",
					},
				},
				"required": []string{"processId"},
			},
		},
		{
			Name:        "get_debug_actions",
			Description: "[Debug Tools] List available debug and cleanup actions for troubleshooting Capacitor/iOS/Android issues. Includes cache clearing, dependency reinstall, and platform reset options.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "run_debug_action",
			Description: "[Debug Tools] Execute a debug action like clearing Xcode derived data, resetting Android build cache, reinstalling node_modules, or cleaning Capacitor platforms.",
			InputSchema: map[string]interface{}{
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
			Name:        "get_settings",
			Description: "[Configuration] Get lazycap settings including default platform, build commands, live reload preferences, and enabled MCP tools.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "set_setting",
			Description: "[Configuration] Change a lazycap setting value.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type":        "string",
						"description": "Setting key from get_settings",
					},
					"value": map[string]interface{}{
						"description": "New value for the setting",
					},
				},
				"required": []string{"key", "value"},
			},
		},
		{
			Name:        "get_project",
			Description: "[Project Info] Get Capacitor project details including app name, app ID (bundle identifier), platforms configured (ios/android), project root path, and capacitor.config.ts settings.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}

	return map[string]interface{}{
		"tools": tools,
	}
}

func (p *MCPPlugin) handleToolsCall(params json.RawMessage) (interface{}, *MCPError) {
	var call struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	switch call.Name {
	case "list_devices":
		return p.toolListDevices()
	case "run_on_device":
		return p.toolRunOnDevice(call.Arguments)
	case "run_web":
		return p.toolRunWeb()
	case "sync":
		return p.toolSync(call.Arguments)
	case "build":
		return p.toolBuild()
	case "open_ide":
		return p.toolOpenIDE(call.Arguments)
	case "get_processes":
		return p.toolGetProcesses()
	case "get_logs":
		return p.toolGetLogs(call.Arguments)
	case "get_all_logs":
		return p.toolGetAllLogs(call.Arguments)
	case "kill_process":
		return p.toolKillProcess(call.Arguments)
	case "get_debug_actions":
		return p.toolGetDebugActions()
	case "run_debug_action":
		return p.toolRunDebugAction(call.Arguments)
	case "get_settings":
		return p.toolGetSettings()
	case "set_setting":
		return p.toolSetSetting(call.Arguments)
	case "get_project":
		return p.toolGetProject()
	default:
		return nil, &MCPError{Code: -32601, Message: "Unknown tool: " + call.Name}
	}
}

// Tool implementations

func (p *MCPPlugin) toolListDevices() (interface{}, *MCPError) {
	devices := p.ctx.GetDevices()
	result := make([]map[string]interface{}, len(devices))
	for i, d := range devices {
		result[i] = map[string]interface{}{
			"id":         d.ID,
			"name":       d.Name,
			"platform":   d.Platform,
			"online":     d.Online,
			"isEmulator": d.IsEmulator,
			"isWeb":      d.IsWeb,
		}
	}
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": toJSON(result)}}}, nil
}

func (p *MCPPlugin) toolRunOnDevice(args map[string]interface{}) (interface{}, *MCPError) {
	deviceID, _ := args["deviceId"].(string)
	liveReload, _ := args["liveReload"].(bool)

	if deviceID == "" {
		return nil, &MCPError{Code: -32602, Message: "deviceId required"}
	}

	if err := p.ctx.RunOnDevice(deviceID, liveReload); err != nil {
		return nil, &MCPError{Code: -32000, Message: err.Error()}
	}

	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": "Started run on " + deviceID}}}, nil
}

func (p *MCPPlugin) toolRunWeb() (interface{}, *MCPError) {
	if err := p.ctx.RunWeb(); err != nil {
		return nil, &MCPError{Code: -32000, Message: err.Error()}
	}
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": "Web dev server started"}}}, nil
}

func (p *MCPPlugin) toolSync(args map[string]interface{}) (interface{}, *MCPError) {
	platform, _ := args["platform"].(string)
	if err := p.ctx.Sync(platform); err != nil {
		return nil, &MCPError{Code: -32000, Message: err.Error()}
	}
	msg := "Sync started"
	if platform != "" {
		msg = "Sync started for " + platform
	}
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": msg}}}, nil
}

func (p *MCPPlugin) toolBuild() (interface{}, *MCPError) {
	if err := p.ctx.Build(); err != nil {
		return nil, &MCPError{Code: -32000, Message: err.Error()}
	}
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": "Build started"}}}, nil
}

func (p *MCPPlugin) toolOpenIDE(args map[string]interface{}) (interface{}, *MCPError) {
	platform, _ := args["platform"].(string)
	if platform == "" {
		return nil, &MCPError{Code: -32602, Message: "platform required"}
	}
	if err := p.ctx.OpenIDE(platform); err != nil {
		return nil, &MCPError{Code: -32000, Message: err.Error()}
	}
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": "Opening " + platform + " IDE"}}}, nil
}

func (p *MCPPlugin) toolGetProcesses() (interface{}, *MCPError) {
	processes := p.ctx.GetProcesses()
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": toJSON(processes)}}}, nil
}

func (p *MCPPlugin) toolGetLogs(args map[string]interface{}) (interface{}, *MCPError) {
	processID, _ := args["processId"].(string)
	if processID == "" {
		return nil, &MCPError{Code: -32602, Message: "processId required"}
	}
	logs := p.ctx.GetProcessLogs(processID)
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": toJSON(logs)}}}, nil
}

func (p *MCPPlugin) toolGetAllLogs(args map[string]interface{}) (interface{}, *MCPError) {
	allLogs := p.ctx.GetAllLogs()
	processes := p.ctx.GetProcesses()

	// Parse filter arguments
	typeFilter, _ := args["type"].(string)
	statusFilter, _ := args["status"].(string)
	searchPattern, _ := args["search"].(string)
	errorsOnly, _ := args["errors_only"].(bool)
	tail := 0
	if t, ok := args["tail"].(float64); ok {
		tail = int(t)
	}

	// Error patterns for errors_only filter
	errorPatterns := []string{"error", "Error", "ERROR", "failed", "Failed", "FAILED", "exception", "Exception", "panic", "Panic", "PANIC", "fatal", "Fatal", "FATAL"}

	// Build result with filtered processes and logs
	result := make(map[string]interface{})

	for _, proc := range processes {
		// Filter by type (partial match on process name)
		if typeFilter != "" {
			if !containsIgnoreCase(proc.Name, typeFilter) && !containsIgnoreCase(proc.Command, typeFilter) {
				continue
			}
		}

		// Filter by status
		if statusFilter != "" && proc.Status != statusFilter {
			continue
		}

		logs, exists := allLogs[proc.ID]
		if !exists {
			continue
		}

		// Apply search filter
		if searchPattern != "" {
			filtered := make([]string, 0)
			for _, line := range logs {
				if containsIgnoreCase(line, searchPattern) {
					filtered = append(filtered, line)
				}
			}
			logs = filtered
		}

		// Apply errors_only filter
		if errorsOnly {
			filtered := make([]string, 0)
			for _, line := range logs {
				for _, pattern := range errorPatterns {
					if strings.Contains(line, pattern) {
						filtered = append(filtered, line)
						break
					}
				}
			}
			logs = filtered
		}

		// Skip processes with no matching logs after filtering
		if len(logs) == 0 && (searchPattern != "" || errorsOnly) {
			continue
		}

		// Apply tail limit
		if tail > 0 && len(logs) > tail {
			logs = logs[len(logs)-tail:]
		}

		result[proc.ID] = map[string]interface{}{
			"name":    proc.Name,
			"status":  proc.Status,
			"command": proc.Command,
			"logs":    logs,
		}
	}

	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": toJSON(result)}}}, nil
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func (p *MCPPlugin) toolKillProcess(args map[string]interface{}) (interface{}, *MCPError) {
	processID, _ := args["processId"].(string)
	if processID == "" {
		return nil, &MCPError{Code: -32602, Message: "processId required"}
	}
	if err := p.ctx.KillProcess(processID); err != nil {
		return nil, &MCPError{Code: -32000, Message: err.Error()}
	}
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": "Process killed"}}}, nil
}

func (p *MCPPlugin) toolGetDebugActions() (interface{}, *MCPError) {
	actions := p.ctx.GetDebugActions()
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
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": toJSON(result)}}}, nil
}

func (p *MCPPlugin) toolRunDebugAction(args map[string]interface{}) (interface{}, *MCPError) {
	actionID, _ := args["actionId"].(string)
	if actionID == "" {
		return nil, &MCPError{Code: -32602, Message: "actionId required"}
	}
	result := p.ctx.RunDebugAction(actionID)
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": toJSON(result)}}}, nil
}

func (p *MCPPlugin) toolGetSettings() (interface{}, *MCPError) {
	settings := p.ctx.GetSettings()
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": toJSON(settings)}}}, nil
}

func (p *MCPPlugin) toolSetSetting(args map[string]interface{}) (interface{}, *MCPError) {
	key, _ := args["key"].(string)
	value := args["value"]
	if key == "" {
		return nil, &MCPError{Code: -32602, Message: "key required"}
	}
	if err := p.ctx.SetSetting(key, value); err != nil {
		return nil, &MCPError{Code: -32000, Message: err.Error()}
	}
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": "Setting updated"}}}, nil
}

func (p *MCPPlugin) toolGetProject() (interface{}, *MCPError) {
	project := p.ctx.GetProject()
	if project == nil {
		return nil, &MCPError{Code: -32000, Message: "No project loaded"}
	}
	result := map[string]interface{}{
		"name":       project.Name,
		"appId":      project.AppID,
		"webDir":     project.WebDir,
		"hasAndroid": project.HasAndroid,
		"hasIOS":     project.HasIOS,
		"rootDir":    project.RootDir,
	}
	return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": toJSON(result)}}}, nil
}

func toJSON(v interface{}) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
