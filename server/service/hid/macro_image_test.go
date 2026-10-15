package hid

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestMacroImageRoundTrip(t *testing.T) {
	root := t.TempDir()
	previous := macroImageRoot
	macroImageRoot = root
	t.Cleanup(func() { macroImageRoot = previous })

	id := "11111111-1111-4111-8111-111111111111"
	raw := mustPNG(t, 4, 3)

	if err := writeMacroImage(id, "boot-ok", pixelMeta(10, 20, 4, 3, 1920, 1080), raw); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readMacroImage(id, "boot-ok")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.X != 10 || got.Y != 20 || got.W != 4 || got.H != 3 || got.FrameW != 1920 || got.FrameH != 1080 {
		t.Fatalf("rect = %+v", got)
	}

	decoded, err := png.Decode(bytes.NewReader(got.Png))
	if err != nil {
		t.Fatalf("png: %v", err)
	}
	if decoded.Bounds().Dx() != 4 || decoded.Bounds().Dy() != 3 {
		t.Fatalf("size = %v", decoded.Bounds())
	}

	pngPath := filepath.Join(root, id, "boot-ok.png")
	if _, err := os.Stat(pngPath); err != nil {
		t.Fatalf("png file: %v", err)
	}

	if err := removeMacroImages(id); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, id)); !os.IsNotExist(err) {
		t.Fatalf("dir still exists: %v", err)
	}
}

func TestMacroImageRoundTripLegacyPercent(t *testing.T) {
	root := t.TempDir()
	previous := macroImageRoot
	macroImageRoot = root
	t.Cleanup(func() { macroImageRoot = previous })

	id := "11111111-1111-4111-8111-111111111111"
	if err := writeMacroImage(id, "boot-ok", macroImageMeta{X: 10.5, Y: 20, W: 30, H: 8.25}, mustPNG(t, 4, 3)); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readMacroImage(id, "boot-ok")
	if err != nil {
		t.Fatal(err)
	}
	if got.FrameW != 0 || got.PX != 10.5 || got.PH != 8.25 {
		t.Fatalf("legacy = %+v", got)
	}
}

func TestMacroImageRejectsUnsafeInput(t *testing.T) {
	root := t.TempDir()
	previous := macroImageRoot
	macroImageRoot = root
	t.Cleanup(func() { macroImageRoot = previous })

	id := "11111111-1111-4111-8111-111111111111"
	raw := mustPNG(t, 2, 2)

	cases := []struct {
		name string
		fn   func() error
	}{
		{"bad id", func() error { return writeMacroImage("../etc", "boot-ok", percentMeta(0, 0, 10, 10), raw) }},
		{"bad name", func() error { return writeMacroImage(id, "../boot", percentMeta(0, 0, 10, 10), raw) }},
		{"empty name", func() error { return writeMacroImage(id, "", percentMeta(0, 0, 10, 10), raw) }},
		{"rect overflow", func() error { return writeMacroImage(id, "boot-ok", percentMeta(90, 0, 20, 10), raw) }},
		{"pixel overflow", func() error {
			return writeMacroImage(id, "boot-ok", pixelMeta(1900, 0, 50, 2, 1920, 1080), mustPNG(t, 50, 2))
		}},
		{"png size mismatch", func() error { return writeMacroImage(id, "boot-ok", pixelMeta(0, 0, 10, 10, 100, 100), raw) }},
		{"zero size", func() error { return writeMacroImage(id, "boot-ok", percentMeta(0, 0, 0, 10), raw) }},
		{"not png", func() error { return writeMacroImage(id, "boot-ok", percentMeta(0, 0, 10, 10), []byte("nope")) }},
		{"too wide", func() error { return writeMacroImage(id, "boot-ok", percentMeta(0, 0, 10, 10), mustPNG(t, 400, 1)) }},
	}

	for _, tc := range cases {
		if err := tc.fn(); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}

	if _, err := readMacroImage(id, "nope"); err == nil {
		t.Fatal("missing image should fail")
	}
}

func TestCaptureMacroImageUsesHDMIPixels(t *testing.T) {
	t.Cleanup(restorePlayerHooks)
	frame := digitFrame(80, 60, 10, 12, 8, 10, 2)
	grabFrameFn = func() (*rgbFrame, error) { return frame, nil }

	region, ok := mapPixelRect(10, 12, 8, 10, 80, 60, 80, 60)
	if !ok {
		t.Fatal("rect")
	}
	pix := copyCrop(frame, region.x, region.y, region.w, region.h)
	if pix == nil {
		t.Fatal("crop")
	}
	encoded, err := pngFromRGBA(8, 10, pix)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 8 || img.Bounds().Dy() != 10 {
		t.Fatalf("size %v", img.Bounds())
	}
	ref := cropRef(frame, 10, 12, 8, 10)
	matched, hasFrame := regionMatches(frame, ref)
	if !hasFrame || !matched {
		t.Fatalf("matched=%v hasFrame=%v", matched, hasFrame)
	}
}

func percentMeta(x, y, w, h float64) macroImageMeta {
	return macroImageMeta{X: x, Y: y, W: w, H: h}
}

func mustPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
