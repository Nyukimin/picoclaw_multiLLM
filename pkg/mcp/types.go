package mcp

// MCPRequest は MCP サーバーへのリクエスト
type MCPRequest struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// MCPResponse は MCP サーバーからのレスポンス
type MCPResponse struct {
	Result map[string]interface{} `json:"result,omitempty"`
	Error  *MCPError              `json:"error,omitempty"`
}

// MCPError は MCP エラー
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool は MCP ツール定義
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ToolListResponse は tools/list のレスポンス
type ToolListResponse struct {
	Tools []Tool `json:"tools"`
}

// ToolCallRequest は tools/call のリクエスト
type ToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResponse は tools/call のレスポンス
type ToolCallResponse struct {
	Content []map[string]interface{} `json:"content"`
}
