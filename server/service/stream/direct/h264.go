package direct

import (
	"net/http"
	"sync"
	"time"

	"NanoKVM-Server/service/stream"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	connsMu sync.Mutex
	conns   = map[*websocket.Conn]struct{}{}
)

func init() {
	stream.RegisterClientCloser(closeAll)
}

func closeAll() int {
	connsMu.Lock()
	list := make([]*websocket.Conn, 0, len(conns))
	for ws := range conns {
		list = append(list, ws)
	}
	connsMu.Unlock()
	for _, ws := range list {
		_ = ws.Close()
	}
	return len(list)
}

func Connect(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Errorf("failed to upgrade to websocket: %s", err)
		return
	}
	connsMu.Lock()
	conns[ws] = struct{}{}
	connsMu.Unlock()
	defer func() {
		connsMu.Lock()
		delete(conns, ws)
		connsMu.Unlock()
		_ = ws.Close()
		log.Debugf("h264 websocket disconnected: %s", ws.RemoteAddr())
	}()
	log.Debugf("h264 websocket connected: %s", ws.RemoteAddr())

	_ = ws.SetReadDeadline(time.Time{})

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() { defer close(stopped); writeStream(ws, done) }()
	defer func() { close(done); _ = ws.Close(); <-stopped }()

	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			log.Debugf("failed to read message (client disconnected): %s", err)
			return
		}
	}
}
