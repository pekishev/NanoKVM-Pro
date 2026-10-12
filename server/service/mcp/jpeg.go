package mcp

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
)

func jpegSize(data []byte) (int, int) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0
	}
	i := 2
	for i+4 < len(data) {
		if data[i] != 0xFF {
			i++
			continue
		}
		for i < len(data) && data[i] == 0xFF {
			i++
		}
		if i >= len(data) {
			break
		}
		marker := data[i]
		i++
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}
		if i+2 > len(data) {
			break
		}
		length := int(data[i])<<8 | int(data[i+1])
		if length < 2 || i+length > len(data) {
			break
		}
		if marker >= 0xC0 && marker <= 0xC3 {
			if i+7 > len(data) {
				break
			}
			height := int(data[i+3])<<8 | int(data[i+4])
			width := int(data[i+5])<<8 | int(data[i+6])
			return width, height
		}
		i += length
	}
	return 0, 0
}

func cropJPEG(data []byte, x, y, width, height int) ([]byte, int, int, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("invalid JPEG frame")
	}
	bounds := img.Bounds()
	frameW := bounds.Dx()
	frameH := bounds.Dy()
	if x < 0 || y < 0 || width <= 0 || height <= 0 || x+width > frameW || y+height > frameH {
		return nil, frameW, frameH, fmt.Errorf("crop is outside the HDMI frame")
	}
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	left := bounds.Min.X + x
	top := bounds.Min.Y + y
	rect := image.Rect(left, top, left+width, top+height)
	cropped := img
	if sub, ok := img.(subImager); ok {
		cropped = sub.SubImage(rect)
	} else {
		return nil, frameW, frameH, fmt.Errorf("cannot crop JPEG frame")
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, cropped, &jpeg.Options{Quality: 90}); err != nil {
		return nil, frameW, frameH, err
	}
	return out.Bytes(), width, height, nil
}
