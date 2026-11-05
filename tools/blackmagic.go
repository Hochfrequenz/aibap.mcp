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
	// CreateTransportFallback creates a request where ADT cannot: today a
	// customizing request (W) or transport of copies (T) on SAP_BASIS 7.50.
	// It must create exactly the requested category with the given target
	// (empty: SAP's default), or fail — never a request of another type.
	// devClass is set only for a workbench request (K).
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

// TransportEntryFallback is an optional extension of BlackMagicClient for
// recording and removing transport entries where ADT cannot: on SAP_BASIS
// 7.50 the transport organizer has neither action, and adtler refuses both
// calls with adt.ErrorNotSupported (#616). The handlers detect it with a type
// assertion, so a BlackMagicClient without it keeps compiling and the tools
// keep returning adtler's error.
//
// adtler v0.12.0 also reports a released request as ErrorNotSupported for an
// add (adtler#288), so AddToTransportFallback must fail with a clear message
// for a released request rather than assume the system lacks the action.
type TransportEntryFallback interface {
	// AddToTransportFallback records the object key pgmID/objectType/
	// objectName (as in table E071) in transport, a modifiable request or task.
	AddToTransportFallback(ctx context.Context, pgmID, objectType, objectName, transport string) error
	// RemoveFromTransportFallback removes the object key from task, a task
	// of parentTransport.
	RemoveFromTransportFallback(ctx context.Context, task, parentTransport, pgmID, objectType, objectName string) error
// TransportCopiesFallback is an optional extension of BlackMagicClient for
// filling a request from another request or from a package where ADT cannot:
// the transport organizer offers these actions from ABAP Platform 2105, and
// adtler refuses them with adt.ErrorNotSupported on older systems (#602). The
// handlers detect it with a type assertion, so a BlackMagicClient without it
// keeps compiling and the tools keep returning adtler's error.
//
// adtler v0.12.0 also reports a released target as ErrorNotSupported
// (adtler#288), so both methods must fail with a clear message for a
// released target rather than assume the system lacks the action.
type TransportCopiesFallback interface {
	// CopyTransportObjectsFallback copies the object entries of source into
	// target, as SE09's "include objects" does.
	CopyTransportObjectsFallback(ctx context.Context, source, target string, withDocumentation bool) error
	// AddPackageToTransportFallback adds the objects of pkg (and, with
	// includeSubpackages, of its subpackages) to target.
	AddPackageToTransportFallback(ctx context.Context, pkg, target string, includeSubpackages bool) error
}
