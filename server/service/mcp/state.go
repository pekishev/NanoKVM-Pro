package mcp

import (
	"context"
	"errors"
	"sync"
	"time"

	"NanoKVM-Server/service/mcp/layout"
)

type sessionState struct {
	mu       sync.Mutex
	layout   layout.Layout
	running  bool
	sent     int
	total    int
	lastErr  string
	frameW   int
	frameH   int
	frameID  int
	hasFrame bool
	cancel   context.CancelFunc
}

var state = &sessionState{layout: layout.EN}

func (s *sessionState) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	fw, fh := s.frameW, s.frameH
	if fw == 0 || fh == 0 {
		fw, fh = hdmiSize()
	}
	layoutName := string(s.layout)
	if layoutName == "" {
		layoutName = "en"
	}
	return map[string]any{
		"ok":           true,
		"connected":    hidConnected(),
		"has_frame":    s.hasFrame || (fw > 0 && fh > 0),
		"frame_width":  fw,
		"frame_height": fh,
		"layout":       layoutName,
		"error":        s.lastErr,
		"frame_id":     s.frameID,
		"type_running": s.running,
		"type_sent":    s.sent,
		"type_total":   s.total,
	}
}

func (s *sessionState) setLayout(next layout.Layout) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.layout = next
}

func (s *sessionState) currentLayout() layout.Layout {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.layout != layout.RU {
		return layout.EN
	}
	return s.layout
}

func (s *sessionState) beginType(parent context.Context, total int) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if parent == nil {
		parent = context.Background()
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(parent)
	s.running = true
	s.sent = 0
	s.total = total
	s.lastErr = ""
	s.cancel = cancel
	return ctx
}

func (s *sessionState) setTypeProgress(sent int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = sent
}

func (s *sessionState) requestStop() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	return s.running
}

func (s *sessionState) waitStopped(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		running := s.running
		s.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *sessionState) finishType(sent int, next layout.Layout, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.sent = sent
	s.layout = next
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if err != nil && !errors.Is(err, errTypeStopped) && !errors.Is(err, context.Canceled) {
		s.lastErr = err.Error()
	}
}

func (s *sessionState) setFrame(width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frameW = width
	s.frameH = height
	s.frameID++
	s.hasFrame = width > 0 && height > 0
	s.lastErr = ""
}

func (s *sessionState) setError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = message
}

func (s *sessionState) frameSize() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frameW > 0 && s.frameH > 0 {
		return s.frameW, s.frameH
	}
	return hdmiSize()
}
