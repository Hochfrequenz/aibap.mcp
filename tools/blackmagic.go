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
	UpdateCustomizing(ctx context.Context, table string, entries []CustomizingEntry, transport string) error
	CreateObjectFallback(ctx context.Context, objectType, name, pkg, description, transport string) error
}

// CustomizingEntry represents a row to write into a customizing table.
// Keys identify the row. Values are the fields to set when Op is "upsert"
// (or empty — upsert is the default). For Op == "delete", Values must be
// empty and the row matching Keys is removed.
type CustomizingEntry struct {
	Keys   map[string]string `json:"keys"`
	Values map[string]string `json:"values,omitempty"`
	Op     string            `json:"op,omitempty"` // "" or "upsert" → upsert; "delete" → delete
}
