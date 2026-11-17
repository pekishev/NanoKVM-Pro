//go:build linux && cgo

package hid

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"

	"NanoKVM-Server/common"
	"NanoKVM-Server/service/vin"
)

const snapshotQuality = 100

func hdmiFrameSize() (int, int, error) {
	screen := common.GetScreen()
	screen.RefreshSize()
	if screen.Width == 0 || screen.Height == 0 {
		return 0, 0, fmt.Errorf("no HDMI frame")
	}
	return int(screen.Width), int(screen.Height), nil
}

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
	return vin.GrabJPEG(context.Background(), snapshotQuality)
}
