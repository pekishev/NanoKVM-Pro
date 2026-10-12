package mcp

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestJPEGSizeAndCrop(t *testing.T) {
	raw := mustJPEG(t, 40, 20)
	w, h := jpegSize(raw)
	if w != 40 || h != 20 {
		t.Fatalf("size = %d×%d", w, h)
	}
	cropped, cw, ch, err := cropJPEG(raw, 10, 5, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	if cw != 8 || ch != 6 {
		t.Fatalf("crop size = %d×%d", cw, ch)
	}
	gotW, gotH := jpegSize(cropped)
	if gotW != 8 || gotH != 6 {
		t.Fatalf("encoded crop = %d×%d", gotW, gotH)
	}
	if _, _, _, err := cropJPEG(raw, 38, 0, 8, 6); err == nil {
		t.Fatal("expected out-of-range crop to fail")
	}
}

func TestPixelToHidCenter(t *testing.T) {
	center := pixelToHid(960, 1920)
	if center < 0x3f00 || center > 0x4100 {
		t.Fatalf("center hid = %d", center)
	}
	if pixelToHid(0, 1920) == 0 {
		t.Fatal("origin should not be HID 0")
	}
	if pixelToHid(1919, 1920) < 0x7f00 {
		t.Fatalf("right edge hid = %d", pixelToHid(1919, 1920))
	}
}

func mustJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
