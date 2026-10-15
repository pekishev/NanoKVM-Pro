package hid

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"NanoKVM-Server/proto"
)

const (
	keyDownMS     = 50 * time.Millisecond
	keyHoldMS     = 80 * time.Millisecond
	keyGapMS      = 80 * time.Millisecond
	commandGapMS  = 100 * time.Millisecond
	mouseMoveMS   = 30 * time.Millisecond
	mouseClickMS  = 50 * time.Millisecond
	mouseRepeatMS = 80 * time.Millisecond
)

var errMacroStopped = errors.New("macro stopped")

type absMouse struct {
	x, y uint16
}

var (
	ensureKeyboardFn = func() error { return GetHid().EnsureKeyboard() }
	ensureMouseFn    = func() error { return GetHid().EnsureMouse() }
	writeKeyboardFn  = func(data []byte) error { return GetHid().WriteHid0Err(data) }
	writeMouseFn     = func(data []byte) error {
		GetHid().WriteHid2(data)
		return nil
	}
	sleepFn      = sleepCtx
	nowFn        = time.Now
	grabFrameFn  = grabHDMIFrame
	loadImageFn  = loadMacroImageRef
)

func playMacro(ctx context.Context, macro proto.Macro, macros []proto.Macro) error {
	if err := sleepFn(ctx, 80*time.Millisecond); err != nil {
		return err
	}
	return playCommands(ctx, macro, macros, &absMouse{}, 0)
}

func playCommands(ctx context.Context, macro proto.Macro, macros []proto.Macro, mouse *absMouse, depth int) error {
	commands, line, err := parseScript(macro.Script)
	if err != nil {
		return fmt.Errorf("%s: line %d: %w", macro.Name, line, err)
	}

	for _, command := range commands {
		if err := ctx.Err(); err != nil {
			return errMacroStopped
		}
		if err := playCommand(ctx, command, macro, macros, mouse, depth); err != nil {
			return err
		}
		if command.Type != "delay" {
			if err := sleepFn(ctx, commandGapMS); err != nil {
				return err
			}
		}
	}
	return nil
}

func playCommand(
	ctx context.Context,
	command scriptCommand,
	macro proto.Macro,
	macros []proto.Macro,
	mouse *absMouse,
	depth int,
) error {
	switch command.Type {
	case "move":
		if err := ensureMouseFn(); err != nil {
			return err
		}
		mouse.x, mouse.y = hidPos(command.X), hidPos(command.Y)
		return moveTo(ctx, mouse.x, mouse.y)
	case "click":
		if err := ensureMouseFn(); err != nil {
			return err
		}
		mouse.x, mouse.y = hidPos(command.X), hidPos(command.Y)
		return clickAt(ctx, mouse.x, mouse.y, command.Button, command.Times)
	case "key":
		if err := ensureKeyboardFn(); err != nil {
			return err
		}
		return pressCodes(ctx, command.Codes)
	case "string":
		if err := ensureKeyboardFn(); err != nil {
			return err
		}
		return typeText(ctx, command.Text)
	case "delay":
		return sleepFn(ctx, time.Duration(command.Ms)*time.Millisecond)
	case "run":
		return playNestedMacro(ctx, command.Name, macro, macros, mouse, depth)
	case "waitimage":
		return waitForImage(ctx, macro, command.Name, command.Ms)
	default:
		return fmt.Errorf("%s: unknown command", macro.Name)
	}
}

func playNestedMacro(ctx context.Context, name string, parent proto.Macro, macros []proto.Macro, mouse *absMouse, depth int) error {
	if depth >= macroRunDepthMax {
		return fmt.Errorf("%s: nested RUN is limited to 1 level", parent.Name)
	}
	child, ok := findNamedMacro(macros, name)
	if !ok {
		return fmt.Errorf("%s: macro not found: %s", parent.Name, name)
	}
	return playCommands(ctx, child, macros, mouse, depth+1)
}

func findNamedMacro(macros []proto.Macro, name string) (proto.Macro, bool) {
	needle := strings.TrimSpace(name)
	for _, macro := range macros {
		if strings.EqualFold(strings.TrimSpace(macro.Name), needle) {
			return macro, true
		}
	}
	return proto.Macro{}, false
}

func waitForImage(ctx context.Context, macro proto.Macro, name string, timeoutMs int) error {
	if macro.ID == "" {
		return fmt.Errorf("%s: image not found: %s", macro.Name, name)
	}
	ref, err := loadImageFn(macro.ID, name)
	if err != nil {
		return fmt.Errorf("%s: image not found: %s", macro.Name, name)
	}

	deadline := nowFn().Add(time.Duration(timeoutMs) * time.Millisecond)
	sawFrame := false
	streak := 0

	for {
		if err := ctx.Err(); err != nil {
			return errMacroStopped
		}
		matched, hasFrame := regionMatches(grabCurrentFrame(), ref)
		if hasFrame {
			sawFrame = true
		}
		if matched {
			streak++
			if streak >= imageConfirmFrames {
				return nil
			}
			if err := sleepFn(ctx, imageConfirmMS); err != nil {
				return err
			}
			continue
		}
		streak = 0

		now := nowFn()
		if !now.Before(deadline) {
			if !sawFrame {
				return fmt.Errorf("%s: HDMI frame unavailable", macro.Name)
			}
			return fmt.Errorf("%s: WAITIMAGE %s timed out", macro.Name, name)
		}
		wait := imagePollMS
		if remaining := deadline.Sub(now); remaining < wait {
			wait = remaining
		}
		if err := sleepFn(ctx, wait); err != nil {
			return err
		}
	}
}

func grabCurrentFrame() *rgbFrame {
	frame, err := grabFrameFn()
	if err != nil || frame == nil {
		return nil
	}
	return frame
}

func moveTo(ctx context.Context, x, y uint16) error {
	if err := writeMouseFn(mouseReport(0, x, y)); err != nil {
		return err
	}
	return sleepFn(ctx, mouseMoveMS)
}

func clickAt(ctx context.Context, x, y uint16, button string, times int) error {
	bit := mouseButtonBit(button)
	if err := writeMouseFn(mouseReport(0, x, y)); err != nil {
		return err
	}
	if err := sleepFn(ctx, mouseMoveMS); err != nil {
		return err
	}
	for i := 0; i < times; i++ {
		if err := writeMouseFn(mouseReport(bit, x, y)); err != nil {
			return err
		}
		if err := sleepFn(ctx, mouseClickMS); err != nil {
			return err
		}
		if err := writeMouseFn(mouseReport(0, x, y)); err != nil {
			return err
		}
		if i < times-1 {
			if err := sleepFn(ctx, mouseRepeatMS); err != nil {
				return err
			}
		}
	}
	return nil
}

func pressCodes(ctx context.Context, codes []string) error {
	kb := keyboardReport{}
	for _, code := range codes {
		if err := writeKeyboardFn(kb.keyDown(code)); err != nil {
			_ = writeKeyboardFn(kb.reset())
			return err
		}
		if err := sleepFn(ctx, keyDownMS); err != nil {
			_ = writeKeyboardFn(kb.reset())
			return err
		}
	}
	if err := sleepFn(ctx, keyHoldMS); err != nil {
		_ = writeKeyboardFn(kb.reset())
		return err
	}
	return writeKeyboardFn(kb.reset())
}

func typeText(ctx context.Context, text string) error {
	for _, ch := range text {
		codes := charToCodes(ch)
		if codes == nil {
			continue
		}
		if err := pressCodes(ctx, codes); err != nil {
			return err
		}
		if err := sleepFn(ctx, keyGapMS); err != nil {
			return err
		}
	}
	return nil
}

func hidPos(percent float64) uint16 {
	n := percent / 100
	if n < 0 {
		n = 0
	}
	if n > 1 {
		n = 1
	}
	v := int(math.Floor(0x7fff*n) + 1)
	if v < 0 {
		v = 0
	}
	if v > 0x7fff {
		v = 0x7fff
	}
	return uint16(v)
}

func mouseButtonBit(button string) byte {
	switch button {
	case "right":
		return 0x02
	case "middle":
		return 0x04
	default:
		return 0x01
	}
}

func mouseReport(buttons byte, x, y uint16) []byte {
	return []byte{
		buttons,
		byte(x),
		byte(x >> 8),
		byte(y),
		byte(y >> 8),
		0,
	}
}

type keyboardReport struct {
	modifier byte
	keys     []byte
}

func (k *keyboardReport) keyDown(code string) []byte {
	if bit, ok := hidModifiers[code]; ok {
		k.modifier |= bit
	} else if key, ok := hidKeycodes[code]; ok {
		if !containsKey(k.keys, key) && len(k.keys) < 6 {
			k.keys = append(k.keys, key)
		}
	}
	return k.build()
}

func (k *keyboardReport) reset() []byte {
	k.modifier = 0
	k.keys = nil
	return k.build()
}

func (k *keyboardReport) build() []byte {
	report := []byte{k.modifier, 0, 0, 0, 0, 0, 0, 0}
	for i, key := range k.keys {
		if i >= 6 {
			break
		}
		report[2+i] = key
	}
	return report
}

func containsKey(keys []byte, key byte) bool {
	for _, existing := range keys {
		if existing == key {
			return true
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		if ctx != nil && ctx.Err() != nil {
			return errMacroStopped
		}
		return nil
	}
	if ctx == nil {
		time.Sleep(d)
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errMacroStopped
	case <-timer.C:
		return nil
	}
}
