package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInitializeAndToolList(t *testing.T) {
	resp := handleRPC(rpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}`),
	})
	if resp == nil || resp.Error != nil {
		t.Fatalf("initialize: %+v", resp)
	}
	raw, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(raw), `"protocolVersion":"2025-03-26"`) {
		t.Fatalf("result = %s", raw)
	}

	listed := handleRPC(rpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage("2"),
		Method:  "tools/list",
	})
	body, _ := json.Marshal(listed.Result)
	for _, name := range []string{"status", "screenshot", "click", "type_keys", "type_text", "stop_type", "press"} {
		if !strings.Contains(string(body), `"name":"`+name+`"`) {
			t.Fatalf("missing tool %s in %s", name, body)
		}
	}
}

func TestStatusTool(t *testing.T) {
	result := callTool(json.RawMessage(`{"name":"status","arguments":{}}`))
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("%+v", result)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ok", "connected", "has_frame", "frame_width", "frame_height", "layout"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing %s in %v", key, payload)
		}
	}
}

func TestTypeKeysRejectsLongText(t *testing.T) {
	text, _ := json.Marshal(strings.Repeat("a", 201))
	result := callTool(json.RawMessage(`{"name":"type_keys","arguments":{"text":` + string(text) + `}}`))
	if !result.IsError {
		t.Fatal("expected too-long error")
	}
}

func TestPressUnknownKey(t *testing.T) {
	result := callTool(json.RawMessage(`{"name":"press","arguments":{"keys":"Ctrl+Nope"}}`))
	if !result.IsError {
		t.Fatal("expected unknown key error")
	}
}

func TestStopTypeWhenIdle(t *testing.T) {
	result := callTool(json.RawMessage(`{"name":"stop_type","arguments":{}}`))
	if result.IsError {
		t.Fatalf("%+v", result)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["stopped"] != false {
		t.Fatalf("%v", payload)
	}
}

func TestUnknownTool(t *testing.T) {
	result := callTool(json.RawMessage(`{"name":"read_terminal","arguments":{}}`))
	if !result.IsError {
		t.Fatal("expected unknown tool")
	}
}

func TestNotificationHasNoResponse(t *testing.T) {
	resp := handleRPC(rpcRequest{JSONRPC: "2.0", Method: "notifications/initialized"})
	if resp != nil {
		t.Fatalf("got %+v", resp)
	}
}
