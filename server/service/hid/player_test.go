package hid

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
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

	err := playMacro(context.Background(), proto.Macro{
		Name:   "demo",
		Script: "STRING a\nENTER\nCLICK 50 50 RIGHT",
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
	x, y := hidPos(50), hidPos(50)
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
	if err := writeMacroImage(id, "boot-ok", 0, 0, 20, 20, refPNG); err != nil {
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

func TestRegionMatchesIdenticalCrop(t *testing.T) {
	frame := solidFrame(12, 12, color.RGBA{R: 40, G: 80, B: 120, A: 255})
	ref := &loadedMacroImage{
		X: 0, Y: 0, W: 100, H: 100,
		Width: 4, Height: 4,
		Data: mustPixels(4, 4, color.RGBA{R: 40, G: 80, B: 120, A: 255}),
	}
	fillLuma(ref)
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

func fillLuma(ref *loadedMacroImage) {
	pixels := ref.Width * ref.Height
	ref.Luma = make([]float64, pixels)
	sum := 0.0
	for i := 0; i < pixels; i++ {
		k := i * 4
		ref.Luma[i] = toLuma(ref.Data[k], ref.Data[k+1], ref.Data[k+2])
		sum += ref.Luma[i]
	}
	ref.Mean = sum / float64(pixels)
	variance := 0.0
	for _, value := range ref.Luma {
		d := value - ref.Mean
		variance += d * d
	}
	ref.Std = math.Sqrt(variance / float64(pixels))
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
