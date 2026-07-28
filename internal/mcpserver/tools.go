package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp/tool"
)

// toolSchemas defines the MCP tool schemas for the browser automation tools.
var toolSchemas = map[string]tool.Tool{
	"browser_context_create": {
		Name:        "browser_context_create",
		Description: "Create a new private ephemeral browser context",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"viewport": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"width":  map[string]interface{}{"type": "number"},
						"height": map[string]interface{}{"type": "number"},
						"scale":  map[string]interface{}{"type": "number"},
					},
				},
				"javascriptEnabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Enable JavaScript in the context",
				},
			},
		},
	},

	"browser_context_list": {
		Name:        "browser_context_list",
		Description: "List all browser contexts owned by this connection",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{},
		},
	},

	"browser_context_close": {
		Name:        "browser_context_close",
		Description: "Idempotently close a browser context",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"contextId": map[string]interface{}{
					"type":        "string",
					"description": "The context ID to close",
				},
			},
			"required": []string{"contextId"},
		},
	},

	"browser_navigate": {
		Name:        "browser_navigate",
		Description: "Navigate to a URL and optionally wait for a lifecycle condition",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"contextId": map[string]interface{}{
					"type":        "string",
					"description": "The context ID",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "The URL to navigate to",
				},
				"waitUntil": map[string]interface{}{
					"type":        "string",
					"description": "Wait condition: commit, interactive, or complete",
					"enum":        []string{"commit", "interactive", "complete"},
				},
				"timeoutMs": map[string]interface{}{
					"type":        "number",
					"description": "Navigation timeout in milliseconds",
				},
			},
			"required": []string{"contextId", "url"},
		},
	},

	"browser_snapshot": {
		Name:        "browser_snapshot",
		Description: "Return a bounded semantic page snapshot with opaque element references",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"contextId": map[string]interface{}{
					"type": "string",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Snapshot format: semantic (default)",
					"enum":        []string{"semantic"},
				},
				"maxDepth": map[string]interface{}{
					"type":        "number",
					"description": "Maximum tree depth (default 50, max 50)",
				},
				"maxNodes": map[string]interface{}{
					"type":        "number",
					"description": "Maximum number of nodes (default 5000, max 5000)",
				},
				"includeHidden": map[string]interface{}{
					"type":        "boolean",
					"description": "Include hidden elements",
				},
			},
			"required": []string{"contextId"},
		},
	},

	"browser_screenshot": {
		Name:        "browser_screenshot",
		Description: "Capture the current viewport as a PNG image",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"contextId": map[string]interface{}{
					"type": "string",
				},
				"scope": map[string]interface{}{
					"type":        "string",
					"description": "Screenshot scope: viewport (default)",
					"enum":        []string{"viewport"},
				},
				"omitBackground": map[string]interface{}{
					"type":        "boolean",
					"description": "Omit page background",
				},
			},
			"required": []string{"contextId"},
		},
	},

	"browser_page_info": {
		Name:        "browser_page_info",
		Description: "Get current page URL, title, lifecycle state, revision, and viewport",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"contextId": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{"contextId"},
		},
	},

	"browser_query": {
		Name:        "browser_query",
		Description: "Resolve a locator to element references on the current page",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"contextId": map[string]interface{}{
					"type": "string",
				},
				"role": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":  map[string]interface{}{"type": "string"},
						"exact": map[string]interface{}{"type": "boolean"},
					},
				},
				"css": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{"type": "string"},
					},
				},
				"text": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"value": map[string]interface{}{"type": "string"},
						"exact": map[string]interface{}{"type": "boolean"},
					},
				},
			},
			"required": []string{"contextId"},
		},
	},

	"browser_click": {
		Name:        "browser_click",
		Description: "Activate an element identified by reference",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"contextId": map[string]interface{}{
					"type": "string",
				},
				"ref": map[string]interface{}{
					"type":        "string",
					"description": "Element reference from snapshot or query",
				},
				"button": map[string]interface{}{
					"type":        "string",
					"description": "Mouse button: left, right, middle",
					"enum":        []string{"left", "right", "middle"},
				},
				"timeoutMs": map[string]interface{}{
					"type": "number",
				},
			},
			"required": []string{"contextId", "ref"},
		},
	},

	"browser_type": {
		Name:        "browser_type",
		Description: "Enter text into an element identified by reference",
		InputSchema: tool.InputSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"contextId": map[string]interface{}{
					"type": "string",
				},
				"ref": map[string]interface{}{
					"type": "string",
				},
				"text": map[string]interface{}{
					"type":        "string",
					"description": "Text to type",
				},
				"replace": map[string]interface{}{
					"type":        "boolean",
					"description": "Replace existing text",
				},
				"submit": map[string]interface{}{
					"type":        "boolean",
					"description": "Submit after typing",
				},
			},
			"required": []string{"contextId", "ref", "text"},
		},
	},
}

// GetToolSchemas returns all tool schemas for registration.
func GetToolSchemas() []tool.Tool {
	tools := make([]tool.Tool, 0, len(toolSchemas))
	for _, t := range toolSchemas {
		tools = append(tools, t)
	}
	return tools
}

// GetToolSchema returns a specific tool schema by name.
func GetToolSchema(name string) (tool.Tool, bool) {
	t, ok := toolSchemas[name]
	return t, ok
}
