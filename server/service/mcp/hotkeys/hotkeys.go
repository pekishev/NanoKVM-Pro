package hotkeys

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	RepeatMax   = 40
	SequenceMax = 32
)

type Chord struct {
	Modifier byte
	Keycode  byte
	Label    string
}

var (
	spaceRun = regexp.MustCompile(`[\s_\-]+`)
	seqSplit = regexp.MustCompile(`[\n;]+`)
)

var modifiers = map[string]struct {
	bit   byte
	label string
}{
	"ctrl":         {0x01, "Ctrl"},
	"control":      {0x01, "Ctrl"},
	"lctrl":        {0x01, "Ctrl"},
	"leftctrl":     {0x01, "Ctrl"},
	"controlleft":  {0x01, "Ctrl"},
	"rctrl":        {0x10, "RCtrl"},
	"rightctrl":    {0x10, "RCtrl"},
	"controlright": {0x10, "RCtrl"},
	"shift":        {0x02, "Shift"},
	"lshift":       {0x02, "Shift"},
	"leftshift":    {0x02, "Shift"},
	"shiftleft":    {0x02, "Shift"},
	"rshift":       {0x20, "RShift"},
	"rightshift":   {0x20, "RShift"},
	"shiftright":   {0x20, "RShift"},
	"alt":          {0x04, "Alt"},
	"lalt":         {0x04, "Alt"},
	"leftalt":      {0x04, "Alt"},
	"altleft":      {0x04, "Alt"},
	"ralt":         {0x40, "RAlt"},
	"rightalt":     {0x40, "RAlt"},
	"altright":     {0x40, "RAlt"},
	"altgr":        {0x40, "RAlt"},
	"win":          {0x08, "Win"},
	"windows":      {0x08, "Win"},
	"meta":         {0x08, "Win"},
	"cmd":          {0x08, "Win"},
	"super":        {0x08, "Win"},
	"gui":          {0x08, "Win"},
	"winleft":      {0x08, "Win"},
	"metaleft":     {0x08, "Win"},
	"winright":     {0x80, "RWin"},
	"metaright":    {0x80, "RWin"},
	"rwin":         {0x80, "RWin"},
}

var modOrder = []byte{0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80}

var modCanon = map[byte]string{
	0x01: "Ctrl",
	0x02: "Shift",
	0x04: "Alt",
	0x08: "Win",
	0x10: "RCtrl",
	0x20: "RShift",
	0x40: "RAlt",
	0x80: "RWin",
}

var keys map[string]struct {
	code  byte
	label string
}

func init() {
	keys = make(map[string]struct {
		code  byte
		label string
	}, 160)
	for i, ch := range "abcdefghijklmnopqrstuvwxyz" {
		code := byte(0x04 + i)
		label := string(unicode.ToUpper(ch))
		keys[string(ch)] = struct {
			code  byte
			label string
		}{code, label}
		keys["key"+string(ch)] = struct {
			code  byte
			label string
		}{code, label}
	}
	for i, ch := range "123456789" {
		code := byte(0x1E + i)
		s := string(ch)
		keys[s] = struct {
			code  byte
			label string
		}{code, s}
		keys["digit"+s] = struct {
			code  byte
			label string
		}{code, s}
	}
	keys["0"] = struct {
		code  byte
		label string
	}{0x27, "0"}
	keys["digit0"] = struct {
		code  byte
		label string
	}{0x27, "0"}

	named := map[string]struct {
		code  byte
		label string
	}{
		"esc":            {0x29, "Esc"},
		"escape":         {0x29, "Esc"},
		"tab":            {0x2B, "Tab"},
		"enter":          {0x28, "Enter"},
		"return":         {0x28, "Enter"},
		"space":          {0x2C, "Space"},
		"spacebar":       {0x2C, "Space"},
		"backspace":      {0x2A, "Backspace"},
		"bksp":           {0x2A, "Backspace"},
		"bs":             {0x2A, "Backspace"},
		"delete":         {0x4C, "Delete"},
		"del":            {0x4C, "Delete"},
		"insert":         {0x49, "Insert"},
		"ins":            {0x49, "Insert"},
		"home":           {0x4A, "Home"},
		"end":            {0x4D, "End"},
		"pageup":         {0x4B, "PageUp"},
		"pgup":           {0x4B, "PageUp"},
		"pagedown":       {0x4E, "PageDown"},
		"pgdn":           {0x4E, "PageDown"},
		"up":             {0x52, "Up"},
		"arrowup":        {0x52, "Up"},
		"down":           {0x51, "Down"},
		"arrowdown":      {0x51, "Down"},
		"left":           {0x50, "Left"},
		"arrowleft":      {0x50, "Left"},
		"right":          {0x4F, "Right"},
		"arrowright":     {0x4F, "Right"},
		"capslock":       {0x39, "CapsLock"},
		"caps":           {0x39, "CapsLock"},
		"menu":           {0x65, "Menu"},
		"apps":           {0x65, "Menu"},
		"contextmenu":    {0x65, "Menu"},
		"printscreen":    {0x46, "PrintScreen"},
		"prtsc":          {0x46, "PrintScreen"},
		"scrolllock":     {0x47, "ScrollLock"},
		"pause":          {0x48, "Pause"},
		"numlock":        {0x53, "NumLock"},
		"`":              {0x35, "`"},
		"backquote":      {0x35, "`"},
		"grave":          {0x35, "`"},
		"-":              {0x2D, "-"},
		"minus":          {0x2D, "-"},
		"=":              {0x2E, "="},
		"equal":          {0x2E, "="},
		"[":              {0x2F, "["},
		"bracketleft":    {0x2F, "["},
		"]":              {0x30, "]"},
		"bracketright":   {0x30, "]"},
		`\`:              {0x31, `\`},
		"backslash":      {0x31, `\`},
		";":              {0x33, ";"},
		"semicolon":      {0x33, ";"},
		"'":              {0x34, "'"},
		"quote":          {0x34, "'"},
		",":              {0x36, ","},
		"comma":          {0x36, ","},
		".":              {0x37, "."},
		"period":         {0x37, "."},
		"/":              {0x38, "/"},
		"slash":          {0x38, "/"},
		"numpadenter":    {0x58, "NumEnter"},
		"numpadadd":      {0x57, "Num+"},
		"numpadsubtract": {0x56, "Num-"},
		"numpadmultiply": {0x55, "Num*"},
		"numpaddivide":   {0x54, "Num/"},
		"numpaddecimal":  {0x63, "Num."},
	}
	for name, info := range named {
		keys[name] = info
	}
	for i := 1; i <= 12; i++ {
		keys[fmt.Sprintf("f%d", i)] = struct {
			code  byte
			label string
		}{byte(0x3A + i - 1), fmt.Sprintf("F%d", i)}
	}
	for i := 0; i < 10; i++ {
		code := byte(0x58 + i)
		if i == 0 {
			code = 0x62
		}
		keys[fmt.Sprintf("numpad%d", i)] = struct {
			code  byte
			label string
		}{code, fmt.Sprintf("Num%d", i)}
	}
}

func token(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	compact := spaceRun.ReplaceAllString(text, "")
	if compact == "" {
		return text
	}
	return compact
}

func ParseChord(text string) (Chord, error) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return Chord{}, fmt.Errorf("empty key combination")
	}
	parts := strings.Split(raw, "+")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
		if parts[i] == "" {
			return Chord{}, fmt.Errorf("invalid combination %q", raw)
		}
	}
	var modifier byte
	var keycode byte
	var keyLabel string
	seen := map[byte]bool{}
	var seenOrder []byte
	for _, part := range parts {
		name := token(part)
		if info, ok := modifiers[name]; ok {
			modifier |= info.bit
			if !seen[info.bit] {
				seen[info.bit] = true
				seenOrder = append(seenOrder, info.bit)
			}
			continue
		}
		if keycode != 0 {
			return Chord{}, fmt.Errorf("one key per combination, not two: %s", raw)
		}
		found, ok := keys[name]
		if !ok {
			return Chord{}, fmt.Errorf("unknown key %q", part)
		}
		keycode = found.code
		keyLabel = found.label
	}
	if modifier == 0 && keycode == 0 {
		return Chord{}, fmt.Errorf("invalid combination %q", raw)
	}
	var labels []string
	for _, bit := range modOrder {
		if modifier&bit != 0 {
			labels = append(labels, modCanon[bit])
		}
	}
	if keyLabel != "" {
		labels = append(labels, keyLabel)
	}
	return Chord{Modifier: modifier, Keycode: keycode, Label: strings.Join(labels, "+")}, nil
}

func splitSequence(text string) []string {
	chunks := seqSplit.Split(strings.TrimSpace(text), -1)
	var out []string
	for _, chunk := range chunks {
		body := strings.TrimSpace(chunk)
		buf := strings.Builder{}
		i := 0
		for i < len(body) {
			if body[i] == ',' && buf.Len() > 0 && !strings.HasSuffix(strings.TrimRight(buf.String(), " \t"), "+") {
				rest := strings.TrimSpace(body[i+1:])
				if rest != "" {
					out = append(out, strings.TrimSpace(buf.String()))
					buf.Reset()
					i++
					for i < len(body) && body[i] == ' ' {
						i++
					}
					continue
				}
			}
			buf.WriteByte(body[i])
			i++
		}
		if strings.TrimSpace(buf.String()) != "" {
			out = append(out, strings.TrimSpace(buf.String()))
		}
	}
	filtered := make([]string, 0, len(out))
	for _, item := range out {
		if item != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func ParseKeys(value string) ([]Chord, error) {
	parts := splitSequence(value)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty key combination")
	}
	if len(parts) > SequenceMax {
		return nil, fmt.Errorf("at most %d combinations at once", SequenceMax)
	}
	chords := make([]Chord, 0, len(parts))
	for _, part := range parts {
		chord, err := ParseChord(part)
		if err != nil {
			return nil, err
		}
		chords = append(chords, chord)
	}
	return chords, nil
}
