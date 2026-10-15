//go:build linux && cgo

package hid

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"time"

	"NanoKVM-Server/common"
)

const snapshotQuality uint16 = 100

func grabHDMIFrame() (*rgbFrame, error) {
	data, err := grabMJPEG()
	if err != nil {
		return nil, err
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid JPEG frame")
	}
	width, height, pix := imagePixels(img)
	if width < 1 || height < 1 {
		return nil, fmt.Errorf("no HDMI frame")
	}
	return &rgbFrame{Width: width, Height: height, Pix: pix}, nil
}

func grabMJPEG() ([]byte, error) {
	screen := common.GetScreen()
	screen.RefreshSize()
	if screen.Width == 0 || screen.Height == 0 {
		return nil, fmt.Errorf("no HDMI frame")
	}

	vision := common.GetKvmVision()
	prev := vision.StreamType
	vision.SetStreamType(uint8(common.STREAM_TYPE_MJPEG))
	defer vision.SetStreamType(prev)

	var last error
	for i := 0; i < 12; i++ {
		data, result := vision.ReadMjpeg(screen.Width, screen.Height, snapshotQuality)
		if result >= 0 && len(data) > 0 {
			return data, nil
		}
		last = fmt.Errorf("MJPEG frame not available")
		time.Sleep(50 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("MJPEG frame not available")
	}
	return nil, last
}
