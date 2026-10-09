package tools

import (
	"context"
	"sync"
)

// Shutdown collects what the tools must do when the server process ends
// (stdin EOF, SIGINT, SIGTERM). main runs it before logging out of SAP: the
// debug cleanup still needs the SAP sessions (#558).
type Shutdown struct {
	mu    sync.Mutex
	hooks []func(context.Context)
}

// NewShutdown returns an empty hook list.
func NewShutdown() *Shutdown { return &Shutdown{} }

func (s *Shutdown) add(fn func(context.Context)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks = append(s.hooks, fn)
}

// Run calls every hook in registration order; ctx bounds them. A nil
// Shutdown does nothing.
func (s *Shutdown) Run(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	hooks := append([]func(context.Context){}, s.hooks...)
	s.mu.Unlock()
	for _, h := range hooks {
		h(ctx)
	}
}
