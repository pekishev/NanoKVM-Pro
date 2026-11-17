package ai

import (
	"context"

	"NanoKVM-Server/service/vin"
)

type SnapshotRequest = vin.SnapshotRequest
type SnapshotHeader = vin.SnapshotHeader

func CaptureSnapshotContext(ctx context.Context, req SnapshotRequest) (SnapshotHeader, []byte, error) {
	return vin.CaptureSnapshotContext(ctx, req)
}
