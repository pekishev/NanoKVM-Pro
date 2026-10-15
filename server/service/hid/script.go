package hid

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	macroImageTimeoutMax = 300000
	macroRunDepthMax     = 1
)

var (
	macroImageNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)
	keySplitRe       = regexp.MustCompile(`[\s+-]+`)
	fKeyRe           = regexp.MustCompile(`^F([1-9]|1[0-9]|2[0-4])$`)
)

var scriptModifiers = map[string]string{
	"CTRL":     "ControlLeft",
	"CONTROL":  "ControlLeft",
	"SHIFT":    "ShiftLeft",
	"ALT":      "AltLeft",
	"GUI":      "MetaLeft",
	"WIN":      "MetaLeft",
	"WINDOWS":  "MetaLeft",
	"META":     "MetaLeft",
	"CMD":      "MetaLeft",
	"COMMAND":  "MetaLeft",
}

var scriptSpecialKeys = map[string]string{
	"ENTER":       "Enter",
	"RETURN":      "Enter",
	"ESC":         "Escape",
	"ESCAPE":      "Escape",
	"BACKSPACE":   "Backspace",
	"BKSP":        "Backspace",
	"TAB":         "Tab",
	"SPACE":       "Space",
	"DELETE":      "Delete",
	"DEL":         "Delete",
	"INSERT":      "Insert",
	"INS":         "Insert",
	"HOME":        "Home",
	"END":         "End",
	"PAGEUP":      "PageUp",
	"PGUP":        "PageUp",
	"PGDN":        "PageDown",
	"PAGEDOWN":    "PageDown",
	"UP":          "ArrowUp",
	"DOWN":        "ArrowDown",
	"LEFT":        "ArrowLeft",
	"RIGHT":       "ArrowRight",
	"CAPSLOCK":    "CapsLock",
	"CAPS":        "CapsLock",
	"PRINTSCREEN": "PrintScreen",
	"PRINT":       "PrintScreen",
	"SCROLLLOCK":  "ScrollLock",
	"PAUSE":       "Pause",
	"BREAK":       "Pause",
	"MENU":        "ContextMenu",
	"APP":         "ContextMenu",
	"NUMLOCK":     "NumLock",
}

var scriptButtons = map[string]string{
	"LEFT":   "left",
	"RIGHT":  "right",
	"MIDDLE": "middle",
	"MID":    "middle",
}

type scriptCommand struct {
	Type   string
	Ms     int
	Text   string
	Codes  []string
	X      float64
	Y      float64
	Button string
	Times  int
	Name   string
}

func parseScript(script string) ([]scriptCommand, int, error) {
	commands := make([]scriptCommand, 0, 8)
	lines := strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n")

	for i, line := range lines {
		lineNo := i + 1
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "REM") || strings.HasPrefix(raw, "#") {
			continue
		}

		cmd, err := parseLine(raw)
		if err != nil {
			return nil, lineNo, err
		}
		commands = append(commands, cmd)
	}

	if len(commands) == 0 {
		return nil, 1, fmt.Errorf("empty script")
	}

	return commands, 0, nil
}

func parseLine(raw string) (scriptCommand, error) {
	command, rest := splitCommand(raw)
	name := strings.ToUpper(command)

	switch name {
	case "DELAY":
		return parseDelay(rest)
	case "STRING", "STRINGLN":
		return parseString(name, rest)
	case "MOVE":
		return parseMove(rest)
	case "CLICK", "DBLCLICK", "RIGHTCLICK", "MIDDLECLICK":
		return parseClick(name, rest)
	case "RUN":
		return parseRun(rest)
	case "WAITIMAGE":
		return parseWaitImage(rest)
	default:
		return parseKeyLine(raw)
	}
}

func parseDelay(rest string) (scriptCommand, error) {
	if rest == "" {
		return scriptCommand{}, fmt.Errorf("DELAY needs 0..60000 ms")
	}
	ms, err := parseIntToken(strings.Fields(rest)[0])
	if err != nil || ms < 0 || ms > 60000 {
		return scriptCommand{}, fmt.Errorf("DELAY needs 0..60000 ms")
	}
	return scriptCommand{Type: "delay", Ms: ms}, nil
}

func parseString(name, rest string) (scriptCommand, error) {
	text := unquote(rest)
	if text == "" {
		return scriptCommand{}, fmt.Errorf("%s needs text", name)
	}
	if name == "STRINGLN" {
		text += "\n"
	}
	return scriptCommand{Type: "string", Text: text}, nil
}

func parseRun(rest string) (scriptCommand, error) {
	target := strings.TrimSpace(unquote(rest))
	if target == "" {
		return scriptCommand{}, fmt.Errorf("RUN needs a macro name")
	}
	if len(target) > 64 {
		return scriptCommand{}, fmt.Errorf("RUN name is too long")
	}
	return scriptCommand{Type: "run", Name: target}, nil
}

func parseWaitImage(rest string) (scriptCommand, error) {
	parts := strings.Fields(rest)
	if len(parts) != 2 {
		return scriptCommand{}, fmt.Errorf("WAITIMAGE needs NAME 0..300000 ms")
	}
	if !macroImageNameRe.MatchString(parts[0]) {
		return scriptCommand{}, fmt.Errorf("WAITIMAGE name must be letters, digits, _ or -")
	}
	ms, err := parseIntToken(parts[1])
	if err != nil || ms < 0 || ms > macroImageTimeoutMax {
		return scriptCommand{}, fmt.Errorf("WAITIMAGE needs NAME 0..300000 ms")
	}
	return scriptCommand{Type: "waitimage", Name: parts[0], Ms: ms}, nil
}

func parseMove(rest string) (scriptCommand, error) {
	x, y, _, err := parsePoint("MOVE", rest, 2)
	if err != nil {
		return scriptCommand{}, err
	}
	return scriptCommand{Type: "move", X: x, Y: y}, nil
}

func parseClick(name, rest string) (scriptCommand, error) {
	x, y, parts, err := parsePoint(name, rest, 3)
	if err != nil {
		if err.Error() != "coordinates must be 0..100 percent" {
			return scriptCommand{}, fmt.Errorf("%s needs X Y [LEFT|RIGHT|MIDDLE]", name)
		}
		return scriptCommand{}, err
	}

	button := "left"
	switch name {
	case "RIGHTCLICK":
		button = "right"
	case "MIDDLECLICK":
		button = "middle"
	}

	if len(parts) > 2 {
		mapped, ok := scriptButtons[strings.ToUpper(parts[2])]
		if !ok {
			return scriptCommand{}, fmt.Errorf("unknown mouse button")
		}
		button = mapped
	}

	times := 1
	if name == "DBLCLICK" {
		times = 2
	}
	return scriptCommand{Type: "click", X: x, Y: y, Button: button, Times: times}, nil
}

func parsePoint(name, rest string, maxParts int) (float64, float64, []string, error) {
	parts := strings.Fields(rest)
	if len(parts) < 2 || len(parts) > maxParts {
		return 0, 0, nil, fmt.Errorf("%s needs X Y", name)
	}

	x, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("coordinates must be 0..100 percent")
	}
	y, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("coordinates must be 0..100 percent")
	}
	if x < 0 || x > 100 || y < 0 || y > 100 {
		return 0, 0, nil, fmt.Errorf("coordinates must be 0..100 percent")
	}
	return x, y, parts, nil
}

func parseKeyLine(raw string) (scriptCommand, error) {
	tokens := keySplitRe.Split(raw, -1)
	filtered := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			filtered = append(filtered, token)
		}
	}
	if len(filtered) == 0 || len(filtered) > 6 {
		return scriptCommand{}, fmt.Errorf("invalid key command")
	}

	codes := make([]string, 0, len(filtered))
	for i, token := range filtered {
		isLast := i == len(filtered)-1
		upper := strings.ToUpper(token)
		var code string
		if !isLast {
			if mapped, ok := scriptModifiers[upper]; ok {
				code = mapped
			}
		}
		if code == "" {
			var ok bool
			code, ok = tokenToCode(token)
			if !ok {
				return scriptCommand{}, fmt.Errorf("unknown key: %s", token)
			}
		}
		if containsCode(codes, code) {
			continue
		}
		codes = append(codes, code)
	}

	mainKey := codes[len(codes)-1]
	if isModifierCode(mainKey) && len(codes) == 1 {
		return scriptCommand{Type: "key", Codes: codes}, nil
	}

	for _, code := range codes {
		if _, ok := hidKeycodes[code]; !ok && !isModifierCode(code) {
			return scriptCommand{}, fmt.Errorf("unknown key: %s", code)
		}
	}

	return scriptCommand{Type: "key", Codes: codes}, nil
}

func tokenToCode(token string) (string, bool) {
	upper := strings.ToUpper(token)
	if mapped, ok := scriptModifiers[upper]; ok {
		return mapped, true
	}
	if mapped, ok := scriptSpecialKeys[upper]; ok {
		return mapped, true
	}
	if fKeyRe.MatchString(upper) {
		return upper, true
	}
	if len(upper) == 1 && upper[0] >= 'A' && upper[0] <= 'Z' {
		return "Key" + upper, true
	}
	if len(token) == 1 && token[0] >= '0' && token[0] <= '9' {
		return "Digit" + token, true
	}
	return "", false
}

func splitCommand(raw string) (string, string) {
	index := strings.IndexFunc(raw, unicode.IsSpace)
	if index < 0 {
		return raw, ""
	}
	return raw[:index], strings.TrimSpace(raw[index+1:])
}

func unquote(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func parseIntToken(raw string) (int, error) {
	if raw == "" {
		return 0, strconv.ErrSyntax
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, strconv.ErrSyntax
		}
	}
	return strconv.Atoi(raw)
}

func containsCode(codes []string, code string) bool {
	for _, existing := range codes {
		if existing == code {
			return true
		}
	}
	return false
}
