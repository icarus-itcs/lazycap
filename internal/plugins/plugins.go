package plugins

import (
	"lazycap/internal/plugins/firebase"
	"lazycap/internal/plugins/mcp"
)

// RegisterAll registers all built-in plugins
func RegisterAll() error {
	// Register MCP Server plugin
	if err := mcp.Register(); err != nil {
		return err
	}

	// Register Firebase Emulator plugin
	if err := firebase.Register(); err != nil {
		return err
	}

	return nil
}
