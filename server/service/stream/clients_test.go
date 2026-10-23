package stream

import "testing"

func TestCloseAllClientsRunsRegisteredClosers(t *testing.T) {
	t.Cleanup(func() {
		closerMu.Lock()
		closers = nil
		closerMu.Unlock()
	})

	closerMu.Lock()
	closers = nil
	closerMu.Unlock()

	RegisterClientCloser(func() int { return 2 })
	RegisterClientCloser(func() int { return 3 })

	if got := CloseAllClients(); got != 5 {
		t.Fatalf("closed %d, want 5", got)
	}
}
