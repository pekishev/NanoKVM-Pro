package mjpeg

import (
	"context"
	"time"

	"NanoKVM-Server/service/stream"

	"github.com/gin-gonic/gin"
)

var streamer = NewStreamer()

func init() {
	stream.RegisterClientCloser(streamer.CloseAll)
}

func Connect(c *gin.Context) {
	c.Header("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Pragma", "no-cache")
	c.Header("X-Server-Date", time.Now().Format(time.RFC1123))

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	streamer.AddClient(c, cancel)
	defer streamer.RemoveClient(c)

	<-ctx.Done()
}
