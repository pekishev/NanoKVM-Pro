package mcp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"time"
	"unicode/utf8"

	"NanoKVM-Server/service/hid"
	"NanoKVM-Server/service/mcp/hotkeys"
	"NanoKVM-Server/service/mcp/layout"
	"NanoKVM-Server/service/vm/jiggler"
)

var errTypeStopped = errors.New("typing stopped")

const (
	keyHold   = 8 * time.Millisecond
	keyGap    = 8 * time.Millisecond
	hidSettle = 8 * time.Millisecond

	hidEnter = 40
)

type hidTap struct {
	modifier, keycode byte
	hold              time.Duration
}

func tapSequence(modifier, keycode byte) []hidTap {
	seq := make([]hidTap, 0, 4)
	if modifier != 0 {
		seq = append(seq, hidTap{modifier, 0, hidSettle})
	}
	seq = append(seq, hidTap{modifier, keycode, keyHold})
	if modifier != 0 {
		seq = append(seq, hidTap{modifier, 0, hidSettle})
		seq = append(seq, hidTap{0, 0, hidSettle + keyGap})
		return seq
	}
	seq = append(seq, hidTap{0, 0, keyGap})
	return seq
}

func withHID(fn func() error) error {
	if !hid.TryLockHID() {
		return fmt.Errorf("keyboard is busy")
	}
	defer hid.UnlockHID()
	return fn()
}

func hidConnected() bool {
	if _, err := os.Stat(hid.HID0); err == nil {
		return true
	}
	return hid.GetHid().KeyboardAvailable()
}

func writeKey(ctx context.Context, modifier, keycode byte) error {
	report := []byte{modifier, 0, keycode, 0, 0, 0, 0, 0}
	var last error
	for attempt := 0; attempt < 8; attempt++ {
		last = hid.GetHid().WriteHid0Err(report)
		if last == nil {
			jiggler.GetJiggler().Update()
			return nil
		}
		if err := sleepCtx(ctx, 5*time.Millisecond); err != nil {
			return err
		}
	}
	return last
}

func releaseKey(ctx context.Context) error {
	return writeKey(ctx, 0, 0)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if ctx == nil {
		time.Sleep(d)
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return errTypeStopped
	case <-t.C:
		return nil
	}
}

func tapKey(ctx context.Context, modifier, keycode byte) error {
	for _, step := range tapSequence(modifier, keycode) {
		if err := writeKey(ctx, step.modifier, step.keycode); err != nil {
			_ = releaseKey(ctx)
			return err
		}
		if err := sleepCtx(ctx, step.hold); err != nil {
			_ = releaseKey(ctx)
			return err
		}
	}
	return nil
}

func altShift(ctx context.Context) error {
	if err := writeKey(ctx, 0x04, 0); err != nil {
		_ = releaseKey(ctx)
		return err
	}
	if err := sleepCtx(ctx, 30*time.Millisecond); err != nil {
		_ = releaseKey(ctx)
		return err
	}
	if err := writeKey(ctx, 0x04|0x02, 0); err != nil {
		_ = releaseKey(ctx)
		return err
	}
	if err := sleepCtx(ctx, 50*time.Millisecond); err != nil {
		_ = releaseKey(ctx)
		return err
	}
	if err := releaseKey(ctx); err != nil {
		return err
	}
	return sleepCtx(ctx, 100*time.Millisecond)
}

func pixelToHid(px, size int) uint16 {
	if size <= 0 {
		return 0
	}
	n := (float64(px) + 0.5) / float64(size)
	if n < 0 {
		n = 0
	}
	if n > 1 {
		n = 1
	}
	return uint16(math.Floor(0x7fff*n) + 1)
}

func absMouse(buttons byte, x, y uint16) {
	hid.GetHid().WriteHid2([]byte{
		buttons,
		byte(x),
		byte(x >> 8),
		byte(y),
		byte(y >> 8),
		0,
	})
	jiggler.GetJiggler().Update()
}

func clickPixels(x, y int, button string, frameW, frameH int) error {
	if frameW < 1 || frameH < 1 {
		return fmt.Errorf("no HDMI frame")
	}
	if x < 0 || y < 0 || x >= frameW || y >= frameH {
		return fmt.Errorf("click is outside the HDMI frame")
	}
	var hidBtn byte
	switch button {
	case "", "left":
		hidBtn = 0x01
	case "right":
		hidBtn = 0x02
	case "middle":
		hidBtn = 0x04
	default:
		return fmt.Errorf("button: left, right or middle")
	}
	if err := hid.GetHid().EnsureMouse(); err != nil {
		return err
	}
	hx := pixelToHid(x, frameW)
	hy := pixelToHid(y, frameH)
	absMouse(0, hx, hy)
	time.Sleep(30 * time.Millisecond)
	absMouse(hidBtn, hx, hy)
	time.Sleep(50 * time.Millisecond)
	absMouse(0, hx, hy)
	return nil
}

func typeText(ctx context.Context, text string, start layout.Layout) (sent int, current layout.Layout, err error) {
	current = start
	segments := layout.Plan(text, start)
	total := utf8.RuneCountInString(text)
	if total == 0 {
		return 0, current, nil
	}
	if err := hid.GetHid().EnsureKeyboard(); err != nil {
		return 0, current, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer releaseKey(ctx)

	runes := []rune(text)
	for _, segment := range segments {
		if err := ctx.Err(); err != nil {
			return sent, current, errTypeStopped
		}
		if segment.Layout != current {
			if err := altShift(ctx); err != nil {
				return sent, current, err
			}
			current = segment.Layout
		}
		for i, ch := range []rune(segment.Keys) {
			if err := ctx.Err(); err != nil {
				return sent, current, errTypeStopped
			}
			if ch == '\n' {
				err = tapKey(ctx, 0, hidEnter)
			} else if mod, code, ok := layout.USHID(ch); ok {
				err = tapKey(ctx, mod, code)
			} else {
				continue
			}
			if err != nil {
				return sent, current, err
			}
			if i < len(segment.Src) {
				sent = segment.Src[i] + 1
				state.setTypeProgress(sent)
			}
		}
	}
	if sent < len(runes) {
		sent = len(runes)
	}
	return sent, current, nil
}

func pressChords(chords []hotkeys.Chord, repeat int) error {
	if repeat < 1 || repeat > hotkeys.RepeatMax {
		return fmt.Errorf("repeat must be between 1 and %d", hotkeys.RepeatMax)
	}
	if err := hid.GetHid().EnsureKeyboard(); err != nil {
		return err
	}
	ctx := context.Background()
	defer releaseKey(ctx)
	for i := 0; i < repeat; i++ {
		for _, chord := range chords {
			if chord.Modifier != 0 {
				if err := writeKey(ctx, chord.Modifier, 0); err != nil {
					return err
				}
				time.Sleep(30 * time.Millisecond)
			}
			if chord.Keycode != 0 {
				if err := writeKey(ctx, chord.Modifier, chord.Keycode); err != nil {
					return err
				}
				time.Sleep(40 * time.Millisecond)
			} else {
				time.Sleep(50 * time.Millisecond)
			}
			if err := releaseKey(ctx); err != nil {
				return err
			}
			time.Sleep(30 * time.Millisecond)
		}
	}
	return nil
}
