//go:build linux && cgo

package mcp

import (
	"context"

	"NanoKVM-Server/common"
	"NanoKVM-Server/service/vin"
)

const snapshotQuality = 100

func hdmiSize() (int, int) {
	screen := common.GetScreen()
	screen.RefreshSize()
	return int(screen.Width), int(screen.Height)
}

func grabMJPEG() ([]byte, error) {
	return vin.GrabJPEG(context.Background(), snapshotQuality)
}
