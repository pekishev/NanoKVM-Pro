package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"NanoKVM-Server/proto"
	"NanoKVM-Server/service/hid"
	"NanoKVM-Server/service/mcp/hotkeys"
	"NanoKVM-Server/service/mcp/layout"
)

func toolSpecs() []toolSpec {
	pixels := map[string]any{"type": "integer"}
	return []toolSpec{
		{
			Name:        "status",
			Description: "Connection state without a screenshot. connected is HID. has_frame means HDMI size is known. frame_width and frame_height are the full-frame pixels for click/screenshot. layout is en or ru for type_text/type_keys.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "screenshot",
			Description: "Capture the HDMI frame as JPEG. With no arguments, the whole frame. Otherwise a rectangle in integer pixels of that full frame: x, y, width, height, origin (0,0) top-left. Not fractions 0..1. Click coordinates stay in full-frame pixels, not the crop.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"x": pixels, "y": pixels, "width": pixels, "height": pixels,
				},
			},
		},
		{
			Name:        "click",
			Description: "Click the remote PC. x and y are integer pixels of the full HDMI frame, origin (0,0) top-left. Same frame as screenshot without crop. Not fractions 0..1. button: left, right, or middle.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"x", "y"},
				"properties": map[string]any{
					"x":      pixels,
					"y":      pixels,
					"button": map[string]any{"type": "string", "enum": []string{"left", "right", "middle"}},
				},
			},
		},
		{
			Name:        "type_keys",
			Description: "Type up to 200 characters as HID keypresses. Quotes \" and ' are kept. layout is the host keyboard layout from status: en or ru. Mixed text sends Alt+Shift itself. Click the target field first. Longer text: type_text.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"text"},
				"properties": map[string]any{
					"text":   map[string]any{"type": "string"},
					"layout": map[string]any{"type": "string", "enum": []string{"en", "ru"}},
				},
			},
		},
		{
			Name:        "type_text",
			Description: "Type long text as HID keypresses, about 40 characters per second. Quotes \" and ' are kept. Do not use NanoKVM paste. layout is en or ru from status. Mixed text sends Alt+Shift itself. Waits until typing finishes. Click the target field first.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"text"},
				"properties": map[string]any{
					"text":   map[string]any{"type": "string"},
					"layout": map[string]any{"type": "string", "enum": []string{"en", "ru"}},
				},
			},
		},
		{
			Name:        "stop_type",
			Description: "Stop type_text or type_keys that is currently running. Safe if nothing is typing. Does not need HID; returns after the current key or immediately if idle.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "press",
			Description: "Press a key or chord, as on a virtual keyboard. Examples: Ctrl+C, Ctrl+L, Ctrl+U, Alt+Tab, Backspace, Delete, Enter, Tab, Esc, F1-F12, Up, Down, Left, Right, Win+E. Several in a row: Ctrl+A, Delete. repeat repeats the whole sequence (useful for Backspace). This is physical keys, not text: use type_keys or type_text to type.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"keys"},
				"properties": map[string]any{
					"keys":   map[string]any{"type": "string"},
					"repeat": map[string]any{"type": "integer"},
				},
			},
		},
		{
			Name:        "wait_image",
			Description: "Wait until an HDMI fragment appears. Either a saved WAITIMAGE (name, optional id or macro) or an inline PNG/JPEG (png plus x, y, width, height in full-frame pixels). Fragment at most 320×320. timeout_ms 0..300000, default 20000. Same matching as WAITIMAGE in HID macros.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":         map[string]any{"type": "string"},
					"id":           map[string]any{"type": "string"},
					"macro":        map[string]any{"type": "string"},
					"png":          map[string]any{"type": "string"},
					"x":            pixels,
					"y":            pixels,
					"width":        pixels,
					"height":       pixels,
					"frame_width":  pixels,
					"frame_height": pixels,
					"timeout_ms":   map[string]any{"type": "integer"},
				},
			},
		},
	}
}

func callTool(params json.RawMessage) toolResult {
	return callToolCtx(context.Background(), params)
}

func callToolCtx(ctx context.Context, params json.RawMessage) toolResult {
	var call toolCallParams
	if err := json.Unmarshal(params, &call); err != nil || call.Name == "" {
		return errorResult("invalid tools/call parameters")
	}
	args := map[string]any{}
	if len(call.Arguments) > 0 && string(call.Arguments) != "null" {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return errorResult("invalid tool arguments")
		}
	}
	switch call.Name {
	case "status":
		return textResult(state.snapshot())
	case "screenshot":
		return toolScreenshot(args)
	case "click":
		return toolClick(args)
	case "type_keys":
		return toolType(ctx, args, true)
	case "type_text":
		return toolType(ctx, args, false)
	case "stop_type":
		return toolStopType()
	case "press":
		return toolPress(args)
	case "wait_image":
		return toolWaitImage(ctx, args)
	default:
		return errorResult("unknown tool: " + call.Name)
	}
}

func toolScreenshot(args map[string]any) toolResult {
	data, err := grabMJPEG()
	if err != nil {
		state.setError(err.Error())
		return errorResult(err.Error())
	}
	frameW, frameH := jpegSize(data)
	if frameW == 0 || frameH == 0 {
		frameW, frameH = hdmiSize()
	}
	state.setFrame(frameW, frameH)

	x, hasX, err := optionalInt(args, "x")
	if err != nil {
		return errorResult(err.Error())
	}
	y, hasY, err := optionalInt(args, "y")
	if err != nil {
		return errorResult(err.Error())
	}
	w, hasW, err := optionalInt(args, "width")
	if err != nil {
		return errorResult(err.Error())
	}
	h, hasH, err := optionalInt(args, "height")
	if err != nil {
		return errorResult(err.Error())
	}
	given := []bool{hasX, hasY, hasW, hasH}
	anyGiven := false
	allGiven := true
	for _, ok := range given {
		anyGiven = anyGiven || ok
		allGiven = allGiven && ok
	}

	note := fmt.Sprintf("full screen, %d×%d. click and crop use pixels of the full HDMI frame %d×%d, (0,0) top-left", frameW, frameH, frameW, frameH)
	out := data
	if anyGiven {
		if !allGiven {
			return errorResult("crop needs x, y, width and height in full-frame pixels")
		}
		cropped, cw, ch, cropErr := cropJPEG(data, x, y, w, h)
		if cropErr != nil {
			return errorResult(cropErr.Error())
		}
		out = cropped
		note = fmt.Sprintf("region x=%d y=%d %d×%d of %d×%d. click uses full-frame pixels, not this crop", x, y, cw, ch, frameW, frameH)
	}
	return toolResult{
		Content: []contentItem{
			{Type: "text", Text: note},
			{Type: "image", Data: base64.StdEncoding.EncodeToString(out), MIMEType: "image/jpeg"},
		},
	}
}

func toolClick(args map[string]any) toolResult {
	x, ok, err := optionalInt(args, "x")
	if err != nil || !ok {
		return errorResult("x and y are full-frame HDMI pixels, (0,0) top-left")
	}
	y, ok, err := optionalInt(args, "y")
	if err != nil || !ok {
		return errorResult("x and y are full-frame HDMI pixels, (0,0) top-left")
	}
	button := "left"
	if raw, exists := args["button"]; exists && raw != nil {
		button = strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
	}
	fw, fh := state.frameSize()
	if fw == 0 || fh == 0 {
		data, grabErr := grabMJPEG()
		if grabErr != nil {
			state.setError(grabErr.Error())
			return errorResult(grabErr.Error())
		}
		fw, fh = jpegSize(data)
		if fw == 0 || fh == 0 {
			fw, fh = hdmiSize()
		}
		state.setFrame(fw, fh)
	}
	err = withHID(func() error {
		return clickPixels(x, y, button, fw, fh)
	})
	if err != nil {
		state.setError(err.Error())
		return errorResult(err.Error())
	}
	return textResult(map[string]any{
		"ok":           true,
		"x":            x,
		"y":            y,
		"button":       button,
		"frame_width":  fw,
		"frame_height": fh,
	})
}

func toolType(ctx context.Context, args map[string]any, short bool) toolResult {
	text, _ := args["text"].(string)
	layoutName := "en"
	if raw, ok := args["layout"].(string); ok && raw != "" {
		layoutName = raw
	}
	start, ok := layout.Parse(layoutName)
	if !ok {
		return errorResult("layout: en or ru")
	}
	if short && utf8.RuneCountInString(text) > layout.KeyTypeMax {
		return errorResult(fmt.Sprintf("short typing is at most %d characters; use type_text for longer text", layout.KeyTypeMax))
	}
	if text == "" {
		return textResult(map[string]any{"ok": true, "empty": true, "layout": string(start), "sent": 0, "total": 0})
	}
	total := utf8.RuneCountInString(text)
	var sent int
	current := start
	err := withHID(func() error {
		typeCtx := state.beginType(ctx, total)
		var typeErr error
		defer func() { state.finishType(sent, current, typeErr) }()
		sent, current, typeErr = typeText(typeCtx, text, start)
		return typeErr
	})
	if err != nil {
		if errors.Is(err, errTypeStopped) || errors.Is(err, context.Canceled) {
			return textResult(map[string]any{
				"ok":      false,
				"stopped": true,
				"sent":    sent,
				"total":   total,
				"layout":  string(current),
			})
		}
		return errorResult(err.Error())
	}
	return textResult(map[string]any{
		"ok":     true,
		"sent":   sent,
		"total":  total,
		"layout": string(current),
	})
}

func toolStopType() toolResult {
	wasRunning := state.requestStop()
	if wasRunning {
		state.waitStopped(2 * time.Second)
	}
	snap := state.snapshot()
	return textResult(map[string]any{
		"ok":           true,
		"stopped":      wasRunning,
		"type_running": snap["type_running"],
		"type_sent":    snap["type_sent"],
		"type_total":   snap["type_total"],
	})
}

func toolPress(args map[string]any) toolResult {
	keys, _ := args["keys"].(string)
	repeat := 1
	if raw, ok := args["repeat"]; ok && raw != nil {
		n, err := asInt(raw)
		if err != nil {
			return errorResult(err.Error())
		}
		repeat = n
	}
	chords, err := hotkeys.ParseKeys(keys)
	if err != nil {
		return errorResult(err.Error())
	}
	err = withHID(func() error {
		return pressChords(chords, repeat)
	})
	if err != nil {
		state.setError(err.Error())
		return errorResult(err.Error())
	}
	labels := make([]string, 0, len(chords))
	for _, chord := range chords {
		labels = append(labels, chord.Label)
	}
	label := strings.Join(labels, ", ")
	if repeat > 1 {
		label = fmt.Sprintf("%s ×%d", label, repeat)
	}
	return textResult(map[string]any{"ok": true, "keys": label, "repeat": repeat})
}

func toolWaitImage(ctx context.Context, args map[string]any) toolResult {
	req := proto.WaitImageReq{
		ID:    stringArg(args, "id"),
		Macro: stringArg(args, "macro"),
		Name:  stringArg(args, "name"),
		Png:   stringArg(args, "png"),
	}
	x, hasX, err := optionalInt(args, "x")
	if err != nil {
		return errorResult(err.Error())
	}
	y, hasY, err := optionalInt(args, "y")
	if err != nil {
		return errorResult(err.Error())
	}
	w, hasW, err := optionalInt(args, "width")
	if err != nil {
		return errorResult(err.Error())
	}
	h, hasH, err := optionalInt(args, "height")
	if err != nil {
		return errorResult(err.Error())
	}
	fw, _, err := optionalInt(args, "frame_width")
	if err != nil {
		return errorResult(err.Error())
	}
	if fw == 0 {
		fw, _, err = optionalInt(args, "frameW")
		if err != nil {
			return errorResult(err.Error())
		}
	}
	fh, _, err := optionalInt(args, "frame_height")
	if err != nil {
		return errorResult(err.Error())
	}
	if fh == 0 {
		fh, _, err = optionalInt(args, "frameH")
		if err != nil {
			return errorResult(err.Error())
		}
	}
	ms, hasMS, err := optionalInt(args, "timeout_ms")
	if err != nil {
		return errorResult(err.Error())
	}
	if !hasMS {
		ms, hasMS, err = optionalInt(args, "ms")
		if err != nil {
			return errorResult(err.Error())
		}
	}

	req.X, req.Y, req.W, req.H = x, y, w, h
	req.FrameW, req.FrameH = fw, fh
	if hasMS {
		req.Ms = &ms
	}
	if req.Png != "" && (!hasX || !hasY || !hasW || !hasH) {
		return errorResult("wait_image png needs x, y, width and height in full-frame pixels")
	}

	spec, err := hid.WaitImageSpecFromReq(req)
	if err != nil {
		return errorResult("wait_image needs a saved name or png with x, y, width, height")
	}

	var name string
	err = withHID(func() error {
		var playErr error
		name, playErr = hid.PlayWaitImage(ctx, spec)
		return playErr
	})
	if err != nil {
		if errors.Is(err, hid.ErrBusy) {
			return errorResult("keyboard is busy")
		}
		if errors.Is(err, hid.ErrWaitImageTimeout) {
			label := spec.Name
			if label == "" {
				label = name
			}
			if label != "" {
				return errorResult("WAITIMAGE " + label + " timed out")
			}
			return errorResult(err.Error())
		}
		if errors.Is(err, hid.ErrWaitImageNotFound) {
			return errorResult("image not found")
		}
		if errors.Is(err, hid.ErrWaitImageNoFrame) {
			return errorResult("HDMI frame unavailable")
		}
		if errors.Is(err, hid.ErrStopped) || errors.Is(err, context.Canceled) {
			return textResult(map[string]any{"ok": false, "stopped": true})
		}
		state.setError(err.Error())
		return errorResult(err.Error())
	}
	return textResult(map[string]any{"ok": true, "matched": true, "name": name})
}

func stringArg(args map[string]any, key string) string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func optionalInt(args map[string]any, key string) (int, bool, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	n, err := asInt(raw)
	return n, true, err
}

func asInt(raw any) (int, error) {
	switch v := raw.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fmt.Errorf("invalid number")
		}
		return int(math.Round(v)), nil
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf("invalid number")
		}
		return int(math.Round(f)), nil
	case string:
		var n float64
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%f", &n); err != nil {
			return 0, fmt.Errorf("invalid number")
		}
		return int(math.Round(n)), nil
	default:
		return 0, fmt.Errorf("invalid number")
	}
}
