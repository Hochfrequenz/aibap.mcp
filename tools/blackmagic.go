package tools

import "context"

// BlackMagicClient channels the old ways to accomplish what the sanctioned
// ADT REST paths cannot. Its methods walk forbidden corridors — undocumented,
// unsupported, and whispered about only in debug traces.
//
// Every invocation carries a price: a dependency on arcane knowledge that may
// shift without warning. Use it only when the light of REST has failed you.
//
// Pass nil to reject the dark arts entirely.
type BlackMagicClient interface {
	ReleaseTransportFallback(ctx context.Context, transportNumber string) error
	CreateTransportFallback(ctx context.Context, category, target, description, devClass string) (string, error)
}
