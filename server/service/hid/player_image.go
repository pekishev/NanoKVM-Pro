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
	searchPX           = 2
	matchGrid          = 3
	minZNCC            = 0.75
	maxCellResidual    = 1.7
	maxCellChroma      = 50
	minContrast        = 0.5
	maxContrast        = 2
	maxMAE             = 28
	flatStd            = 4
	flatMAE            = 12
)

type rgbFrame struct {
	Width  int
	Height int
	Pix    []byte
}

type loadedMacroImage struct {
	X, Y, W, H float64
	Width      int
	Height     int
	Data       []byte
	Luma       []float64
	Mean       float64
	Std        float64
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
	luma := make([]float64, width*height)
	sum := 0.0
	for i := 0; i < width*height; i++ {
		k := i * 4
		luma[i] = toLuma(pix[k], pix[k+1], pix[k+2])
		sum += luma[i]
	}
	mean := sum / float64(width*height)
	variance := 0.0
	for _, value := range luma {
		d := value - mean
		variance += d * d
	}
	return &loadedMacroImage{
		X:      stored.X,
		Y:      stored.Y,
		W:      stored.W,
		H:      stored.H,
		Width:  width,
		Height: height,
		Data:   pix,
		Luma:   luma,
		Mean:   mean,
		Std:    math.Sqrt(variance / float64(width*height)),
	}, nil
}

func regionMatches(frame *rgbFrame, ref *loadedMacroImage) (matched bool, hasFrame bool) {
	if frame == nil || ref == nil || frame.Width < 1 || frame.Height < 1 {
		return false, false
	}
	region, ok := regionPixels(frame.Width, frame.Height, ref.X, ref.Y, ref.W, ref.H)
	if !ok {
		return false, false
	}

	kx := float64(ref.Width) / float64(region.w)
	ky := float64(ref.Height) / float64(region.h)
	left := maxInt(0, region.x-int(math.Ceil(float64(searchPX)/kx)))
	top := maxInt(0, region.y-int(math.Ceil(float64(searchPX)/ky)))
	right := minInt(frame.Width, region.x+region.w+int(math.Ceil(float64(searchPX)/kx)))
	bottom := minInt(frame.Height, region.y+region.h+int(math.Ceil(float64(searchPX)/ky)))

	winW := maxInt(ref.Width, int(math.Round(float64(right-left)*kx)))
	winH := maxInt(ref.Height, int(math.Round(float64(bottom-top)*ky)))
	win := scaleCrop(frame, left, top, right-left, bottom-top, winW, winH)
	if win == nil {
		return false, false
	}

	ox := int(math.Round(float64(region.x-left) * kx))
	oy := int(math.Round(float64(region.y-top) * ky))
	maxDx := minInt(winW-ref.Width, ox+searchPX)
	maxDy := minInt(winH-ref.Height, oy+searchPX)
	for dy := maxInt(0, oy-searchPX); dy <= maxDy; dy++ {
		for dx := maxInt(0, ox-searchPX); dx <= maxDx; dx++ {
			if matchesAt(win, winW, dx, dy, ref) {
				return true, true
			}
		}
	}
	return false, true
}

func matchesAt(win []byte, winW, dx, dy int, ref *loadedMacroImage) bool {
	w, h := ref.Width, ref.Height
	pixels := w * h
	live := make([]float64, pixels)
	cellChroma := make([]float64, matchGrid*matchGrid)
	cellCount := make([]int, matchGrid*matchGrid)
	sum := 0.0
	colorDiff := 0.0

	for y := 0; y < h; y++ {
		row := (y * matchGrid / h) * matchGrid
		for x := 0; x < w; x++ {
			i := y*w + x
			j := ((y+dy)*winW + x + dx) * 4
			k := i * 4
			r, g, b := win[j], win[j+1], win[j+2]
			l := toLuma(r, g, b)
			live[i] = l
			sum += l
			colorDiff += math.Abs(float64(r)-float64(ref.Data[k])) +
				math.Abs(float64(g)-float64(ref.Data[k+1])) +
				math.Abs(float64(b)-float64(ref.Data[k+2]))

			rl := ref.Luma[i]
			cell := row + (x * matchGrid / w)
			cellChroma[cell] += math.Max(
				math.Abs(float64(r)-l-(float64(ref.Data[k])-rl)),
				math.Max(
					math.Abs(float64(g)-l-(float64(ref.Data[k+1])-rl)),
					math.Abs(float64(b)-l-(float64(ref.Data[k+2])-rl)),
				),
			)
			cellCount[cell]++
		}
	}

	mae := colorDiff / float64(pixels*3)
	if mae > maxMAE {
		return false
	}
	for c, chroma := range cellChroma {
		if cellCount[c] > 0 && chroma/float64(cellCount[c]) > maxCellChroma {
			return false
		}
	}

	mean := sum / float64(pixels)
	variance := 0.0
	for _, value := range live {
		d := value - mean
		variance += d * d
	}
	std := math.Sqrt(variance / float64(pixels))

	if ref.Std < flatStd {
		return mae <= flatMAE && std < ref.Std+flatStd
	}
	if std < ref.Std*minContrast || std > ref.Std*maxContrast {
		return false
	}

	cellSum := make([]float64, matchGrid*matchGrid)
	cov := 0.0
	for y := 0; y < h; y++ {
		row := (y * matchGrid / h) * matchGrid
		for x := 0; x < w; x++ {
			i := y*w + x
			a := (live[i] - mean) / std
			b := (ref.Luma[i] - ref.Mean) / ref.Std
			cov += a * b
			cell := row + (x * matchGrid / w)
			d := a - b
			cellSum[cell] += d * d
		}
	}
	if cov/float64(pixels) < minZNCC {
		return false
	}
	for c, residual := range cellSum {
		if cellCount[c] > 0 && residual/float64(cellCount[c]) > maxCellResidual {
			return false
		}
	}
	return true
}

type pixelRect struct {
	x, y, w, h int
}

func regionPixels(frameW, frameH int, x, y, w, h float64) (pixelRect, bool) {
	if frameW < 1 || frameH < 1 {
		return pixelRect{}, false
	}
	left := clampInt(int(math.Round(float64(frameW)*x/100)), 0, frameW-1)
	top := clampInt(int(math.Round(float64(frameH)*y/100)), 0, frameH-1)
	right := clampInt(int(math.Round(float64(frameW)*(x+w)/100)), left+1, frameW)
	bottom := clampInt(int(math.Round(float64(frameH)*(y+h)/100)), top+1, frameH)
	return pixelRect{x: left, y: top, w: right - left, h: bottom - top}, true
}

func scaleCrop(frame *rgbFrame, x, y, w, h, dw, dh int) []byte {
	if dw < 1 || dh < 1 || w < 1 || h < 1 {
		return nil
	}
	out := make([]byte, dw*dh*4)
	for py := 0; py < dh; py++ {
		sy := float64(y) + (float64(py)+0.5)*float64(h)/float64(dh) - 0.5
		for px := 0; px < dw; px++ {
			sx := float64(x) + (float64(px)+0.5)*float64(w)/float64(dw) - 0.5
			r, g, b, a := sampleBilinear(frame, sx, sy)
			i := (py*dw + px) * 4
			out[i] = r
			out[i+1] = g
			out[i+2] = b
			out[i+3] = a
		}
	}
	return out
}

func sampleBilinear(frame *rgbFrame, x, y float64) (byte, byte, byte, byte) {
	if frame.Width < 1 || frame.Height < 1 {
		return 0, 0, 0, 255
	}
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := x0 + 1
	y1 := y0 + 1
	fx := x - float64(x0)
	fy := y - float64(y0)

	r00, g00, b00, a00 := pixelAt(frame, x0, y0)
	r10, g10, b10, a10 := pixelAt(frame, x1, y0)
	r01, g01, b01, a01 := pixelAt(frame, x0, y1)
	r11, g11, b11, a11 := pixelAt(frame, x1, y1)

	r0 := lerp(r00, r10, fx)
	r1 := lerp(r01, r11, fx)
	g0 := lerp(g00, g10, fx)
	g1 := lerp(g01, g11, fx)
	b0 := lerp(b00, b10, fx)
	b1 := lerp(b01, b11, fx)
	a0 := lerp(a00, a10, fx)
	a1 := lerp(a01, a11, fx)
	return roundByte(lerp(r0, r1, fy)), roundByte(lerp(g0, g1, fy)), roundByte(lerp(b0, b1, fy)), roundByte(lerp(a0, a1, fy))
}

func pixelAt(frame *rgbFrame, x, y int) (float64, float64, float64, float64) {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= frame.Width {
		x = frame.Width - 1
	}
	if y >= frame.Height {
		y = frame.Height - 1
	}
	i := (y*frame.Width + x) * 4
	return float64(frame.Pix[i]), float64(frame.Pix[i+1]), float64(frame.Pix[i+2]), float64(frame.Pix[i+3])
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

func toLuma(r, g, b byte) float64 {
	return float64(r)*0.299 + float64(g)*0.587 + float64(b)*0.114
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func roundByte(value float64) byte {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return byte(math.Round(value))
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
