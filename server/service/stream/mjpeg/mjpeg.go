package mjpeg

import (
	"context"
	"sync"
	"time"

	"NanoKVM-Server/common"
	"NanoKVM-Server/service/stream"

	"github.com/gin-gonic/gin"
)

var (
	clientsMu sync.Mutex
	clients   = map[*gin.Context]context.CancelFunc{}
)

func init() {
	stream.RegisterClientCloser(closeAll)
}

func closeAll() int {
	clientsMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(clients))
	for _, cancel := range clients {
		if cancel != nil {
			cancels = append(cancels, cancel)
		}
	}
	clientsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return len(cancels)
}

func Connect(c *gin.Context) {
	c.Header("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Pragma", "no-cache")
	c.Header("X-Server-Date", time.Now().Format(time.RFC1123))

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	clientsMu.Lock()
	clients[c] = cancel
	clientsMu.Unlock()
	defer func() {
		clientsMu.Lock()
		delete(clients, c)
		clientsMu.Unlock()
	}()

	hub := stream.GetVideoHub()
	hub.SetMode(common.STREAM_TYPE_MJPEG)
	subscription := hub.Subscribe(common.STREAM_TYPE_MJPEG)
	defer subscription.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-subscription.Frames:
			if !ok {
				return
			}
			if frame.Result < 0 || !hub.IsCurrent(frame) || !hub.IsMode(common.STREAM_TYPE_MJPEG) {
				continue
			}
			if err := writeFrame(c, frame.Data); err != nil {
				return
			}
		}
	}
}
