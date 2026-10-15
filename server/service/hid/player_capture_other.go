//go:build !linux || !cgo

package hid

import "fmt"

func hdmiFrameSize() (int, int, error) {
	return 0, 0, fmt.Errorf("HDMI capture is only available on the device")
}

func grabHDMIFrame() (*rgbFrame, error) {
	return nil, fmt.Errorf("HDMI capture is only available on the device")
}
