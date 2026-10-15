package hid

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"time"
)

const (
	imagePollMS        = 200 * time.Millisecond
	imageConfirmFrames = 2
	imageConfirmMS     = 100 * time.Millisecond
	maxMeanAbs         = 6
	changeDelta        = 20
	maxChanged         = 0.08
)

type rgbFrame struct {
	Width  int
	Height int
	Pix    []byte
}

type loadedMacroImage struct {
	X, Y, W, H     int
	FrameW, FrameH int
	Percent        bool
	PX, PY, PW, PH float64
	Width          int
	Height         int
	Data           []byte
}

func loadMacroImageRef(id, name string) (*loadedMacroImage, error) {
	stored, err := readMacroImage(id, name)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(stored.Png))
	if err != nil {
		return nil, err
	}
	width, height, pix := imagePixels(img)
	ref := &loadedMacroImage{
		Width:  width,
		Height: height,
		Data:   pix,
	}
	if stored.FrameW > 0 && stored.FrameH > 0 {
		ref.X = stored.X
		ref.Y = stored.Y
		ref.W = stored.W
		ref.H = stored.H
		ref.FrameW = stored.FrameW
		ref.FrameH = stored.FrameH
	} else {
		ref.Percent = true
		ref.PX, ref.PY, ref.PW, ref.PH = stored.PX, stored.PY, stored.PW, stored.PH
	}
	return ref, nil
}

func regionMatches(frame *rgbFrame, ref *loadedMacroImage) (matched bool, hasFrame bool) {
	if frame == nil || ref == nil || frame.Width < 1 || frame.Height < 1 || ref.Width < 1 || ref.Height < 1 {
		return false, false
	}
	region, ok := refRect(frame, ref)
	if !ok {
		return false, true
	}
	live := extractRect(frame, region, ref.Width, ref.Height)
	if live == nil {
		return false, true
	}
	return pixelsMatch(live, ref.Data), true
}

func refRect(frame *rgbFrame, ref *loadedMacroImage) (pixelRect, bool) {
	if ref.Percent {
		return regionPixels(frame.Width, frame.Height, ref.PX, ref.PY, ref.PW, ref.PH)
	}
	if ref.FrameW < 1 || ref.FrameH < 1 {
		return pixelRect{}, false
	}
	if frame.Width == ref.FrameW && frame.Height == ref.FrameH {
		return clampPixelRect(ref.X, ref.Y, ref.W, ref.H, frame.Width, frame.Height)
	}
	return mapPixelRect(ref.X, ref.Y, ref.W, ref.H, ref.FrameW, ref.FrameH, frame.Width, frame.Height)
}

func pixelsMatch(live, ref []byte) bool {
	if len(live) == 0 || len(live) != len(ref) || len(ref)%4 != 0 {
		return false
	}
	n := len(ref) / 4
	sum := 0.0
	changed := 0
	for i := 0; i < n; i++ {
		k := i * 4
		dr := absByte(live[k], ref[k])
		dg := absByte(live[k+1], ref[k+1])
		db := absByte(live[k+2], ref[k+2])
		sum += dr + dg + db
		if dr > changeDelta || dg > changeDelta || db > changeDelta {
			changed++
		}
	}
	mae := sum / float64(n*3)
	return mae <= maxMeanAbs && float64(changed)/float64(n) <= maxChanged
}

func extractRect(frame *rgbFrame, region pixelRect, dw, dh int) []byte {
	if dw < 1 || dh < 1 {
		return nil
	}
	if region.w == dw && region.h == dh {
		return copyCrop(frame, region.x, region.y, region.w, region.h)
	}
	out := make([]byte, dw*dh*4)
	for py := 0; py < dh; py++ {
		sy := region.y + py*region.h/dh
		for px := 0; px < dw; px++ {
			sx := region.x + px*region.w/dw
			src := (sy*frame.Width + sx) * 4
			dst := (py*dw + px) * 4
			copy(out[dst:dst+4], frame.Pix[src:src+4])
		}
	}
	return out
}

func copyCrop(frame *rgbFrame, x, y, w, h int) []byte {
	if frame == nil || x < 0 || y < 0 || w < 1 || h < 1 {
		return nil
	}
	if x+w > frame.Width || y+h > frame.Height {
		return nil
	}
	out := make([]byte, w*h*4)
	for row := 0; row < h; row++ {
		src := ((y+row)*frame.Width + x) * 4
		copy(out[row*w*4:(row+1)*w*4], frame.Pix[src:src+w*4])
	}
	return out
}

type pixelRect struct {
	x, y, w, h int
}

func mapPixelRect(x, y, w, h, srcW, srcH, dstW, dstH int) (pixelRect, bool) {
	if w < 1 || h < 1 || srcW < 1 || srcH < 1 || dstW < 1 || dstH < 1 {
		return pixelRect{}, false
	}
	if srcW == dstW && srcH == dstH {
		return clampPixelRect(x, y, w, h, dstW, dstH)
	}
	left := x * dstW / srcW
	top := y * dstH / srcH
	right := (x + w) * dstW / srcW
	bottom := (y + h) * dstH / srcH
	return clampPixelRect(left, top, right-left, bottom-top, dstW, dstH)
}

func clampPixelRect(x, y, w, h, fw, fh int) (pixelRect, bool) {
	if w < 1 || h < 1 || fw < 1 || fh < 1 {
		return pixelRect{}, false
	}
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x >= fw || y >= fh || w < 1 || h < 1 {
		return pixelRect{}, false
	}
	if x+w > fw {
		w = fw - x
	}
	if y+h > fh {
		h = fh - y
	}
	if w < 1 || h < 1 || w > maxMacroImageEdge || h > maxMacroImageEdge {
		return pixelRect{}, false
	}
	return pixelRect{x: x, y: y, w: w, h: h}, true
}

func regionPixels(frameW, frameH int, x, y, w, h float64) (pixelRect, bool) {
	if frameW < 1 || frameH < 1 {
		return pixelRect{}, false
	}
	left := clampInt(int(math.Round(float64(frameW)*x/100)), 0, frameW-1)
	top := clampInt(int(math.Round(float64(frameH)*y/100)), 0, frameH-1)
	right := clampInt(int(math.Round(float64(frameW)*(x+w)/100)), left+1, frameW)
	bottom := clampInt(int(math.Round(float64(frameH)*(y+h)/100)), top+1, frameH)
	return clampPixelRect(left, top, right-left, bottom-top, frameW, frameH)
}

func imagePixels(img image.Image) (int, int, []byte) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pix := make([]byte, width*height*4)
	i := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			pix[i] = byte(r >> 8)
			pix[i+1] = byte(g >> 8)
			pix[i+2] = byte(b >> 8)
			pix[i+3] = byte(a >> 8)
			i += 4
		}
	}
	return width, height, pix
}

func absByte(a, b byte) float64 {
	if a > b {
		return float64(a - b)
	}
	return float64(b - a)
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
