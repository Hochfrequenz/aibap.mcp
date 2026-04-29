package tools

import "github.com/Hochfrequenz/adtler/adt"

// Result types used by tools whose output does not have a natural adtler
// struct to return. Used with mcp.NewToolResultJSON (to populate
// StructuredContent) and mcp.WithOutputSchema (to advertise the output shape
// on the tool definition).
//
// Types that are tightly coupled to a single handler (e.g. RollbackResult,
// NavigationResult, VerifyResult, BAdIImplementationWithXML) continue to
// live next to their registration; this file collects the types used by
// more than one handler or whose registration file would otherwise grow
// its own results block.
//
// Tools that return an adtler struct directly do not need a type here.

// SourceResult preserves the {source, etag} shape that source.go has used
// since the original stringly-typed implementation. Uses lowercase json
// tags; adt.SourceResult has Go-style PascalCase field names without tags.
type SourceResult struct {
	Source string `json:"source"`
	ETag   string `json:"etag"`
}

type IncludeSourceResult struct {
	Source  string `json:"source"`
	ETag    string `json:"etag"`
	Include string `json:"include"`
}

type SetIncludeSourceResult struct {
	ETag    string `json:"etag"`
	Include string `json:"include"`
}

type SourceMultiResult struct {
	Total      int                `json:"total"`
	Succeeded  int                `json:"succeeded"`
	Failed     int                `json:"failed"`
	TotalLines int                `json:"total_lines"`
	Results    []SourceMultiEntry `json:"results"`
}

type SourceMultiEntry struct {
	ObjectURI string `json:"object_uri"`
	Source    string `json:"source,omitempty"`
	ETag      string `json:"etag,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ObjectCreateResult struct {
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

type ObjectDeleteResult struct {
	URI     string `json:"uri"`
	Deleted bool   `json:"deleted"`
}

type LockResult struct {
	Handle string `json:"handle"`
}

type UnlockResult struct {
	URI      string `json:"uri"`
	Unlocked bool   `json:"unlocked"`
}

type AddToTransportResult struct {
	ObjectURI string `json:"object_uri"`
	Transport string `json:"transport"`
	Added     bool   `json:"added"`
}

type RemoveFromTransportResult struct {
	TaskNumber string `json:"task_number"`
	ObjectName string `json:"object_name"`
	Removed    bool   `json:"removed"`
}

type CreateTransportResult struct {
	TransportNumber string `json:"transport_number"`
	Description     string `json:"description"`
}

type CreateTransportTaskResult struct {
	TaskNumber      string `json:"task_number"`
	ParentTransport string `json:"parent_transport"`
	Description     string `json:"description"`
}

type ReleaseTransportResult struct {
	Transport   string `json:"transport"`
	Released    bool   `json:"released"`
	ViaFallback bool   `json:"via_fallback,omitempty"`
}

type DeleteTransportResult struct {
	Transport string `json:"transport"`
	Deleted   bool   `json:"deleted"`
}

type PrettyPrintResult struct {
	Formatted string `json:"formatted"`
}

type VersionSourceResult struct {
	Source string `json:"source"`
}

type DocumentationResult struct {
	Documentation string `json:"documentation"`
}

type SelectSystemResult struct {
	System  string `json:"system"`
	Message string `json:"message"`
}

type BreakpointRemoveResult struct {
	Removed bool   `json:"removed"`
	Message string `json:"message,omitempty"`
}

type DebugListenerStopResult struct {
	Stopped bool `json:"stopped"`
}

type DebugAttachResult struct {
	DebuggeeID string `json:"debuggee_id"`
	Attached   bool   `json:"attached"`
}

type DebugStartResult struct {
	BreakpointID string `json:"breakpoint_id"`
	Status       string `json:"status"`
	DebuggeeID   string `json:"debuggee_id"`
}

type UpdateCustomizingResult struct {
	Status string `json:"status"`
	Table  string `json:"table"`
}

type SetEnhancementImplementationResult struct {
	Status string `json:"status"`
}

type ExportPackageResult struct {
	Package      string `json:"package"`
	Path         string `json:"path"`
	ZipSizeBytes int    `json:"zip_size_bytes"`
	Format       string `json:"format"`
}

type ExportPackagesEntry struct {
	Package      string `json:"package"`
	Path         string `json:"path,omitempty"`
	ZipSizeBytes int    `json:"zip_size_bytes,omitempty"`
	Error        string `json:"error,omitempty"`
	Exported     bool   `json:"exported"`
}

type ExportPackagesResult struct {
	Pattern           string                `json:"pattern"`
	FoundBeforeFilter int                   `json:"found_before_filter,omitempty"`
	FoundAfterFilter  int                   `json:"found_after_filter,omitempty"`
	Exported          int                   `json:"exported"`
	Format            string                `json:"format,omitempty"`
	Message           string                `json:"message,omitempty"`
	Results           []ExportPackagesEntry `json:"results,omitempty"`
}

type SetSourceFromFileResult struct {
	Success    bool   `json:"success"`
	Lines      int    `json:"lines"`
	Locked     bool   `json:"locked"`
	LockHandle string `json:"lock_handle"`
	ETag       string `json:"etag"`
}

type PatchSourceResult struct {
	Success    bool   `json:"success"`
	LineDelta  int    `json:"line_delta"`
	Locked     bool   `json:"locked"`
	LockHandle string `json:"lock_handle"`
	ETag       string `json:"etag"`
}

type SetMessagesResult struct {
	Success       bool   `json:"success"`
	MessageClass  string `json:"message_class"`
	MessagesCount int    `json:"messages_count"`
}

type SyntaxCheckBatchResult struct {
	Total         int                      `json:"total"`
	Clean         int                      `json:"clean"`
	TotalErrors   int                      `json:"total_errors"`
	TotalWarnings int                      `json:"total_warnings"`
	Results       []adt.ObjectSyntaxResult `json:"results"`
}

type UnitTestBatchEntry struct {
	ObjectURI  string          `json:"object_uri"`
	TestResult *adt.TestResult `json:"test_result,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type UnitTestBatchResult struct {
	TotalObjects int                  `json:"total_objects"`
	TotalPassed  int                  `json:"total_passed"`
	TotalFailed  int                  `json:"total_failed"`
	Results      []UnitTestBatchEntry `json:"results"`
}

type ObjectInfoBatchEntry struct {
	ObjectURI string          `json:"object_uri"`
	Info      *adt.ObjectInfo `json:"info,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type ObjectInfoBatchResult struct {
	Total     int                    `json:"total"`
	Succeeded int                    `json:"succeeded"`
	Failed    int                    `json:"failed"`
	Results   []ObjectInfoBatchEntry `json:"results"`
}

type ObjectExistsResult struct {
	Exists      bool   `json:"exists"`
	ObjectURI   string `json:"object_uri"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

type ObjectExistsBatchEntry struct {
	ObjectURI string `json:"object_uri"`
	Exists    bool   `json:"exists"`
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
}

type ObjectExistsBatchResult struct {
	Total   int                      `json:"total"`
	Found   int                      `json:"found"`
	Missing int                      `json:"missing"`
	Results []ObjectExistsBatchEntry `json:"results"`
}

type WhereUsedBatchEntry struct {
	ObjectURI  string           `json:"object_uri"`
	References []adt.ObjectInfo `json:"references"`
	Error      string           `json:"error,omitempty"`
}

type WhereUsedBatchResult struct {
	Total           int                   `json:"total"`
	TotalReferences int                   `json:"total_references"`
	Results         []WhereUsedBatchEntry `json:"results"`
}

type ObjectDependency struct {
	Name    string `json:"name"`
	UseType string `json:"use_type"`
}

type ObjectDependenciesResult struct {
	ObjectType   string             `json:"object_type"`
	ObjectName   string             `json:"object_name"`
	Count        int                `json:"count"`
	Dependencies []ObjectDependency `json:"dependencies"`
}

// Wrapper result types for tools that previously returned top-level JSON
// arrays. MCP 2025-06-18 requires CallToolResult.structuredContent to be a
// JSON object; nesting the slice in a named struct satisfies the spec and
// lets WithOutputSchema advertise the shape. See issue #351.

type SearchObjectsResult struct {
	Count   int              `json:"count"`
	Results []adt.ObjectInfo `json:"results"`
}

// WhereUsedSingleResult is the single-URI branch of where_used. The
// array-URI branch returns WhereUsedBatchResult (different shape).
type WhereUsedSingleResult struct {
	Count      int              `json:"count"`
	References []adt.ObjectInfo `json:"references"`
}

// SyntaxCheckSingleResult is the single-URI branch of syntax_check. The
// array-URI branch returns SyntaxCheckBatchResult.
type SyntaxCheckSingleResult struct {
	Count    int                 `json:"count"`
	Messages []adt.SyntaxMessage `json:"messages"`
}

type ListShortDumpsResult struct {
	Count int                   `json:"count"`
	Dumps []adt.ShortDumpHeader `json:"dumps"`
}

type ShortDumpDetailsResult struct {
	Count int             `json:"count"`
	Dumps []adt.ShortDump `json:"dumps"`
}

type CompletionsResult struct {
	Count int                  `json:"count"`
	Items []adt.CompletionItem `json:"items"`
}

type BrowsePackageResult struct {
	Count   int              `json:"count"`
	Objects []adt.ObjectInfo `json:"objects"`
}

type TableFieldsResult struct {
	TableName string          `json:"table_name"`
	Count     int             `json:"count"`
	Fields    []adt.FieldInfo `json:"fields"`
}

type SearchMessagesResult struct {
	Count   int                       `json:"count"`
	Results []adt.MessageSearchResult `json:"results"`
}

type InactiveObjectsResult struct {
	Count   int              `json:"count"`
	Objects []adt.ObjectInfo `json:"objects"`
}

type VersionHistoryResult struct {
	Count    int               `json:"count"`
	Versions []adt.VersionInfo `json:"versions"`
}

type TransportRequestsResult struct {
	Count      int                    `json:"count"`
	Transports []adt.TransportRequest `json:"transports"`
}

type TransportObjectsResult struct {
	Transport string                `json:"transport"`
	Count     int                   `json:"count"`
	Objects   []adt.TransportObject `json:"objects"`
}
