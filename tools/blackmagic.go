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
	UpdateCustomizing(ctx context.Context, table string, entries []CustomizingEntry) error
}

// CustomizingEntry represents a row to write into a customizing table.
// Keys identify the row; Values are the fields to set.
type CustomizingEntry struct {
	Keys   map[string]string `json:"keys"`
	Values map[string]string `json:"values"`
}
