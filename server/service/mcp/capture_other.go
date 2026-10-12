//go:build !linux || !cgo

package mcp

import "fmt"

func hdmiSize() (int, int) {
	return 0, 0
}

func grabMJPEG() ([]byte, error) {
	return nil, fmt.Errorf("HDMI capture is only available on the device")
}
