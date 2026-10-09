package tools

import (
	"context"
	"testing"
	"time"
)

func TestShutdownRunsHooksInOrder(t *testing.T) {
	s := NewShutdown()
	var got []int
	s.add(func(context.Context) { got = append(got, 1) })
	s.add(func(context.Context) { got = append(got, 2) })
	s.Run(context.Background())
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("got %v", got)
	}
	var nilShutdown *Shutdown
	nilShutdown.Run(context.Background()) // must not panic
}

func TestDebugSessionsShutdownReturnsWhenTheContextEnds(t *testing.T) {
	m := &debugSessions{timings: currentDebugTimings()}
	m.startMu.Lock() // a cleanup that never gets the lock
	defer m.startMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	m.shutdown(ctx)
	if time.Since(start) > time.Second {
		t.Error("shutdown must give up when its context ends")
	}
}
