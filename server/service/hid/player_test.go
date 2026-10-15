package hid

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"NanoKVM-Server/proto"
)

func restorePlayerHooks() {
	ensureKeyboardFn = func() error { return GetHid().EnsureKeyboard() }
	ensureMouseFn = func() error { return GetHid().EnsureMouse() }
	writeKeyboardFn = func(data []byte) error { return GetHid().WriteHid0Err(data) }
	writeMouseFn = func(data []byte) error {
		GetHid().WriteHid2(data)
		return nil
	}
	sleepFn = sleepCtx
	nowFn = time.Now
	grabFrameFn = grabHDMIFrame
	loadImageFn = loadMacroImageRef
	frameSizeFn = hdmiFrameSize
}

func TestPlayMacroSendsHIDReports(t *testing.T) {
	t.Cleanup(restorePlayerHooks)

	var keys [][]byte
	var mouse [][]byte
	ensureKeyboardFn = func() error { return nil }
	ensureMouseFn = func() error { return nil }
	writeKeyboardFn = func(data []byte) error {
		keys = append(keys, append([]byte(nil), data...))
		return nil
	}
	writeMouseFn = func(data []byte) error {
		mouse = append(mouse, append([]byte(nil), data...))
		return nil
	}
	sleepFn = func(context.Context, time.Duration) error { return nil }
	frameSizeFn = func() (int, int, error) { return 1920, 1080, nil }

	err := playMacro(context.Background(), proto.Macro{
		Name:   "demo",
		Script: "STRING a\nENTER\nCLICK 960 540 RIGHT",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) < 4 {
		t.Fatalf("keyboard reports = %d, %#v", len(keys), keys)
	}
	if keys[0][2] != hidKeycodes["KeyA"] {
		t.Fatalf("first key = %v", keys[0])
	}
	if !bytes.Equal(keys[1], make([]byte, 8)) {
		t.Fatalf("key a release = %v", keys[1])
	}

	if len(mouse) != 3 {
		t.Fatalf("mouse reports = %d, %#v", len(mouse), mouse)
	}
	x, y := hidPixel(960, 1920), hidPixel(540, 1080)
	if mouse[1][0] != 0x02 {
		t.Fatalf("right button down = %v", mouse[1])
	}
	if mouse[1][1] != byte(x) || mouse[1][3] != byte(y) {
		t.Fatalf("click pos = %v want %d %d", mouse[1], x, y)
	}
	if mouse[2][0] != 0 {
		t.Fatalf("button up = %v", mouse[2])
	}
}

func TestPlayMacroRunsNestedOnce(t *testing.T) {
	t.Cleanup(restorePlayerHooks)
	ensureKeyboardFn = func() error { return nil }
	ensureMouseFn = func() error { return nil }
	writeKeyboardFn = func([]byte) error { return nil }
	writeMouseFn = func([]byte) error { return nil }
	sleepFn = func(context.Context, time.Duration) error { return nil }

	macros := []proto.Macro{
		{ID: "1", Name: "Outer", Script: "RUN Inner"},
		{ID: "2", Name: "Inner", Script: "RUN Outer"},
	}
	err := playMacro(context.Background(), macros[0], macros)
	if err == nil || err.Error() != "Inner: nested RUN is limited to 1 level" {
		t.Fatalf("err = %v", err)
	}
}

func TestPlayMacroStopCancelsDelay(t *testing.T) {
	t.Cleanup(restorePlayerHooks)
	ensureKeyboardFn = func() error { return nil }
	ensureMouseFn = func() error { return nil }
	writeKeyboardFn = func([]byte) error { return nil }
	writeMouseFn = func([]byte) error { return nil }
	sleepFn = sleepCtx

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := playMacro(ctx, proto.Macro{Name: "wait", Script: "DELAY 5000"}, nil)
	if !errors.Is(err, errMacroStopped) {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitImageMatchesReference(t *testing.T) {
	t.Cleanup(restorePlayerHooks)
	root := t.TempDir()
	previous := macroImageRoot
	macroImageRoot = root
	t.Cleanup(func() { macroImageRoot = previous })

	id := "11111111-1111-4111-8111-111111111111"
	frame := solidFrame(20, 20, color.RGBA{R: 10, G: 20, B: 200, A: 255})
	refPNG := mustPNGBytes(t, 4, 4, color.RGBA{R: 10, G: 20, B: 200, A: 255})
	if err := writeMacroImage(id, "boot-ok", pixelMeta(0, 0, 4, 4, 20, 20), refPNG); err != nil {
		t.Fatal(err)
	}

	ensureKeyboardFn = func() error { return nil }
	ensureMouseFn = func() error { return nil }
	writeKeyboardFn = func([]byte) error { return nil }
	writeMouseFn = func([]byte) error { return nil }
	sleepFn = func(context.Context, time.Duration) error { return nil }
	grabFrameFn = func() (*rgbFrame, error) { return frame, nil }

	err := playMacro(context.Background(), proto.Macro{
		ID:     id,
		Name:   "boot",
		Script: "WAITIMAGE boot-ok 1000",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLookupMacroByName(t *testing.T) {
	dir := t.TempDir()
	previous := macroFile
	macroFile = filepath.Join(dir, "macros.json")
	t.Cleanup(func() { macroFile = previous })

	if err := os.WriteFile(macroFile, []byte(`{"macros":[{"id":"abc","name":"Boot PC","script":"ENTER"}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	got, _, err := lookupMacro("", "boot pc")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "abc" {
		t.Fatalf("got %+v", got)
	}
	if _, _, err := lookupMacro("", "missing"); !errors.Is(err, errMacroNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRegionMatchesExactPixels(t *testing.T) {
	two := digitFrame(100, 100, 40, 50, 20, 16, 2)
	one := digitFrame(100, 100, 40, 50, 20, 16, 1)
	ref := cropRef(two, 40, 50, 20, 16)

	matched, hasFrame := regionMatches(two, ref)
	if !hasFrame || !matched {
		t.Fatalf("digit 2 at saved pixels: matched=%v hasFrame=%v", matched, hasFrame)
	}

	matched, hasFrame = regionMatches(one, ref)
	if !hasFrame {
		t.Fatal("expected an HDMI frame")
	}
	if matched {
		t.Fatal("digit 1 must not match a saved 2")
	}

	ref.Y = 34
	matched, hasFrame = regionMatches(two, ref)
	if !hasFrame {
		t.Fatal("expected an HDMI frame")
	}
	if matched {
		t.Fatal("must compare the saved pixels, not search nearby")
	}
}

func TestRegionMatchesIdenticalCrop(t *testing.T) {
	frame := solidFrame(12, 12, color.RGBA{R: 40, G: 80, B: 120, A: 255})
	ref := cropRef(frame, 0, 0, 4, 4)
	matched, hasFrame := regionMatches(frame, ref)
	if !hasFrame || !matched {
		t.Fatalf("matched=%v hasFrame=%v", matched, hasFrame)
	}

	other := solidFrame(12, 12, color.RGBA{R: 200, G: 10, B: 10, A: 255})
	matched, hasFrame = regionMatches(other, ref)
	if !hasFrame || matched {
		t.Fatalf("different color matched=%v hasFrame=%v", matched, hasFrame)
	}
}

func digitFrame(fw, fh, x, y, w, h, digit int) *rgbFrame {
	pix := mustPixels(fw, fh, color.RGBA{R: 236, G: 236, B: 236, A: 255})
	for yy := 0; yy < 6; yy++ {
		for xx := 0; xx < w; xx++ {
			setRGBA(pix, fw, x+xx, y+yy, color.RGBA{R: 40, G: 60, B: 180, A: 255})
		}
	}
	ox, oy := x+6, y+7
	switch digit {
	case 1:
		for yy := 0; yy < 8; yy++ {
			setRGBA(pix, fw, ox+2, oy+yy, color.RGBA{A: 255})
		}
	default:
		for xx := 0; xx < 5; xx++ {
			setRGBA(pix, fw, ox+xx, oy, color.RGBA{A: 255})
			setRGBA(pix, fw, ox+xx, oy+3, color.RGBA{A: 255})
			setRGBA(pix, fw, ox+xx, oy+7, color.RGBA{A: 255})
		}
		for yy := 1; yy < 3; yy++ {
			setRGBA(pix, fw, ox+4, oy+yy, color.RGBA{A: 255})
		}
		for yy := 4; yy < 7; yy++ {
			setRGBA(pix, fw, ox, oy+yy, color.RGBA{A: 255})
		}
	}
	return &rgbFrame{Width: fw, Height: fh, Pix: pix}
}

func cropRef(frame *rgbFrame, x, y, w, h int) *loadedMacroImage {
	return &loadedMacroImage{
		X:      x,
		Y:      y,
		W:      w,
		H:      h,
		FrameW: frame.Width,
		FrameH: frame.Height,
		Width:  w,
		Height: h,
		Data:   copyCrop(frame, x, y, w, h),
	}
}

func pixelMeta(x, y, w, h, frameW, frameH int) macroImageMeta {
	return macroImageMeta{
		X: float64(x), Y: float64(y), W: float64(w), H: float64(h),
		FrameW: frameW, FrameH: frameH,
	}
}

func setRGBA(pix []byte, w, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= w {
		return
	}
	i := (y*w + x) * 4
	if i < 0 || i+3 >= len(pix) {
		return
	}
	pix[i] = c.R
	pix[i+1] = c.G
	pix[i+2] = c.B
	pix[i+3] = c.A
}

func solidFrame(w, h int, c color.RGBA) *rgbFrame {
	return &rgbFrame{Width: w, Height: h, Pix: mustPixels(w, h, c)}
}

func mustPixels(w, h int, c color.RGBA) []byte {
	pix := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		pix[i*4] = c.R
		pix[i*4+1] = c.G
		pix[i*4+2] = c.B
		pix[i*4+3] = c.A
	}
	return pix
}

func mustPNGBytes(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
