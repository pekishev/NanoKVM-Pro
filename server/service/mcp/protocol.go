package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"NanoKVM-Server/service/hid"
)

const (
	protocolFallback = "2025-03-26"
	serverName       = "nanokvm"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type contentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
}

type toolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func isNotification(req rpcRequest) bool {
	if len(req.ID) == 0 {
		return true
	}
	return string(req.ID) == "null"
}

func handleRPC(req rpcRequest) *rpcResponse {
	return handleRPCCtx(context.Background(), req)
}

func handleRPCCtx(ctx context.Context, req rpcRequest) *rpcResponse {
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return rpcErr(req.ID, -32600, "invalid JSON-RPC version")
	}
	switch req.Method {
	case "initialize":
		return rpcOK(req.ID, initializeResult(req.Params))
	case "notifications/initialized":
		return nil
	case "notifications/cancelled":
		state.requestStop()
		hid.StopPlayback()
		return nil
	case "ping":
		return rpcOK(req.ID, map[string]any{})
	case "tools/list":
		return rpcOK(req.ID, map[string]any{"tools": toolSpecs()})
	case "tools/call":
		return rpcOK(req.ID, callToolCtx(ctx, req.Params))
	case "resources/list":
		return rpcOK(req.ID, map[string]any{"resources": []any{}})
	case "prompts/list":
		return rpcOK(req.ID, map[string]any{"prompts": []any{}})
	default:
		return rpcErr(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func initializeResult(params json.RawMessage) map[string]any {
	version := protocolFallback
	var body struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &body); err == nil && strings.TrimSpace(body.ProtocolVersion) != "" {
		version = body.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    serverName,
			"version": "1.0.0",
		},
		"instructions": "Drive the remote PC through this NanoKVM: HDMI screenshots and HID keyboard/mouse. click, screenshot crop, and wait_image use integer pixels of the full HDMI frame, origin (0,0) top-left. Never pass fractions 0..1. Call status for frame_width x frame_height and layout (en/ru) before typing. Mixed EN/RU text sends Alt+Shift itself. wait_image waits until a saved or inline HDMI fragment appears.",
	}
}

func rpcOK(id json.RawMessage, result any) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func rpcErr(id json.RawMessage, code int, message string) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func textResult(v any) toolResult {
	text, err := json.Marshal(v)
	if err != nil {
		return errorResult(err.Error())
	}
	return toolResult{Content: []contentItem{{Type: "text", Text: string(text)}}}
}

func errorResult(message string) toolResult {
	return toolResult{
		Content: []contentItem{{Type: "text", Text: message}},
		IsError: true,
	}
}
