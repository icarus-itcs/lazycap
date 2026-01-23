package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"

	"lazycap/internal/plugin"
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

func (p *MCPPlugin) ID() string          { return PluginID }
func (p *MCPPlugin) Name() string        { return PluginName }
func (p *MCPPlugin) Version() string     { return PluginVersion }
func (p *MCPPlugin) Author() string      { return PluginAuthor }
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
		p.listener.Close()
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
	defer conn.Close()

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
		encoder.Encode(response)
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
		encoder.Encode(response)
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
			"name":    "lazycap",
			"version": PluginVersion,
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
			Description: "List all available devices, emulators, and simulators",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "run_on_device",
			Description: "Run the app on a specific device",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"deviceId": map[string]interface{}{
						"type":        "string",
						"description": "Device ID to run on",
					},
					"liveReload": map[string]interface{}{
						"type":        "boolean",
						"description": "Enable live reload",
					},
				},
				"required": []string{"deviceId"},
			},
		},
		{
			Name:        "run_web",
			Description: "Start the web development server",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "sync",
			Description: "Sync web assets to native platforms",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"platform": map[string]interface{}{
						"type":        "string",
						"description": "Platform to sync (ios, android, or empty for all)",
					},
				},
			},
		},
		{
			Name:        "build",
			Description: "Build the web assets",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "open_ide",
			Description: "Open the native project in IDE (Xcode or Android Studio)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"platform": map[string]interface{}{
						"type":        "string",
						"description": "Platform to open (ios or android)",
					},
				},
				"required": []string{"platform"},
			},
		},
		{
			Name:        "get_processes",
			Description: "Get list of running and completed processes",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_logs",
			Description: "Get logs for a specific process",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"processId": map[string]interface{}{
						"type":        "string",
						"description": "Process ID to get logs for",
					},
				},
				"required": []string{"processId"},
			},
		},
		{
			Name:        "kill_process",
			Description: "Kill a running process",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"processId": map[string]interface{}{
						"type":        "string",
						"description": "Process ID to kill",
					},
				},
				"required": []string{"processId"},
			},
		},
		{
			Name:        "get_debug_actions",
			Description: "Get list of available debug/cleanup actions",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "run_debug_action",
			Description: "Run a debug/cleanup action",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"actionId": map[string]interface{}{
						"type":        "string",
						"description": "Debug action ID to run",
					},
				},
				"required": []string{"actionId"},
			},
		},
		{
			Name:        "get_settings",
			Description: "Get current settings",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "set_setting",
			Description: "Change a setting value",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type":        "string",
						"description": "Setting key",
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
			Description: "Get project information",
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
