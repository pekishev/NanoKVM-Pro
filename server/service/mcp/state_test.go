package mcp

import (
	"context"
	"testing"
	"time"
)

func TestTypeProgressShowsInStatus(t *testing.T) {
	state.beginType(context.Background(), 10)
	defer state.finishType(10, state.currentLayout(), nil)

	state.setTypeProgress(4)
	snap := state.snapshot()
	if snap["type_running"] != true {
		t.Fatalf("running = %v", snap["type_running"])
	}
	if snap["type_sent"] != 4 {
		t.Fatalf("sent = %v", snap["type_sent"])
	}
	if snap["type_total"] != 10 {
		t.Fatalf("total = %v", snap["type_total"])
	}
}

func TestRequestStopCancelsType(t *testing.T) {
	ctx := state.beginType(context.Background(), 5)
	if !state.requestStop() {
		t.Fatal("expected running typing")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context was not cancelled")
	}
	state.finishType(1, state.currentLayout(), errTypeStopped)
	if state.requestStop() {
		t.Fatal("expected idle after finish")
	}
	if err := state.snapshot()["error"]; err != "" {
		t.Fatalf("stop should not set status.error, got %v", err)
	}
}
