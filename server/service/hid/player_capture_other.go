//go:build !linux || !cgo

package hid

import "fmt"

func grabHDMIFrame() (*rgbFrame, error) {
	return nil, fmt.Errorf("HDMI capture is only available on the device")
}
