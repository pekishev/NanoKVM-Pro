package stream

import "sync"

// ClientCloser disconnects stream viewers and returns how many connections were closed.
type ClientCloser func() int

var (
	closerMu sync.Mutex
	closers  []ClientCloser
)

func RegisterClientCloser(fn ClientCloser) {
	if fn == nil {
		return
	}

	closerMu.Lock()
	closers = append(closers, fn)
	closerMu.Unlock()
}

func CloseAllClients() int {
	closerMu.Lock()
	fns := append([]ClientCloser(nil), closers...)
	closerMu.Unlock()

	closed := 0
	for _, fn := range fns {
		closed += fn()
	}
	return closed
}
