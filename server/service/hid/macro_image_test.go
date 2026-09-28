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

	if err := writeMacroImage(id, "boot-ok", 10.5, 20, 30, 8.25, raw); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readMacroImage(id, "boot-ok")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.X != 10.5 || got.Y != 20 || got.W != 30 || got.H != 8.25 {
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
		{"bad id", func() error { return writeMacroImage("../etc", "boot-ok", 0, 0, 10, 10, raw) }},
		{"bad name", func() error { return writeMacroImage(id, "../boot", 0, 0, 10, 10, raw) }},
		{"empty name", func() error { return writeMacroImage(id, "", 0, 0, 10, 10, raw) }},
		{"rect overflow", func() error { return writeMacroImage(id, "boot-ok", 90, 0, 20, 10, raw) }},
		{"zero size", func() error { return writeMacroImage(id, "boot-ok", 0, 0, 0, 10, raw) }},
		{"not png", func() error { return writeMacroImage(id, "boot-ok", 0, 0, 10, 10, []byte("nope")) }},
		{"too wide", func() error { return writeMacroImage(id, "boot-ok", 0, 0, 10, 10, mustPNG(t, 400, 1)) }},
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
