//go:build linux && cgo

package mcp

import (
	"fmt"
	"time"

	"NanoKVM-Server/common"
)

const snapshotQuality uint16 = 100

func hdmiSize() (int, int) {
	screen := common.GetScreen()
	screen.RefreshSize()
	return int(screen.Width), int(screen.Height)
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
