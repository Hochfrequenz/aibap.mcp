package bwf

import "time"

// Options controls a workflow definition read.
type Options struct {
	Task      string // workflow task ID, normalised to WS + 8 digits
	Version   string // "" = active
	Source    string // "definition" | "runtime"
	Depth     int    // 0..5
	Language  string // SAP 1-char, default "E"
	Raw       bool
	Container bool
	Mermaid   bool
	Today     time.Time // injected for HRP1205 validity
}

// Result is the tool output: the root definition plus resolved subworkflows.
type Result struct {
	Definition
	Subworkflows []Subworkflow `json:"subworkflows"`
}

// Definition is one decoded workflow definition. WfdID is the internal
// definition ID (SWDSHEADER/SWD_HEADER WFD_ID); it equals the task ID except
// for legacy definitions, which have numeric IDs such as 0040000064. Task is
// the workflow task ID (WS + 8 digits).
type Definition struct {
	WfdID               string             `json:"wfd_id"`
	Task                string             `json:"task"`
	Version             string             `json:"version"`
	VersionSource       string             `json:"version_source"` // HRS1205|HRP1205|latest_numbered|legacy_client|requested|runtime
	SourceTables        string             `json:"source_tables"`  // SWDS|SWD
	RuntimeVersion      string             `json:"runtime_version"`
	HasConfiguration    bool               `json:"has_configuration"`
	Language            string             `json:"language"`
	Title               string             `json:"title"`
	ActivatedOn         string             `json:"activated_on"` // YYYY-MM-DD or ""
	Nodes               []Node             `json:"nodes"`
	Edges               []Edge             `json:"edges"`
	Container           []ContainerElement `json:"container"`
	Calls               []Call             `json:"calls"`
	UnresolvedConstants []string           `json:"unresolved_constants"`
	Warnings            []string           `json:"warnings"`
	Mermaid             string             `json:"mermaid,omitempty"`
}

// Node is a workflow step/node.
type Node struct {
	ID                string    `json:"id"`
	Type              string    `json:"type"`
	Kind              string    `json:"kind"`
	Label             string    `json:"label"`
	Condition         string    `json:"condition"`
	ConditionResolved string    `json:"condition_resolved"`
	Task              string    `json:"task"`
	Agent             string    `json:"agent"`
	Operation         string    `json:"operation"`
	OperationResolved string    `json:"operation_resolved"`
	Event             string    `json:"event"`
	BlockID           string    `json:"block_id"`
	ParentBlockID     string    `json:"parent_block_id"`
	Bindings          []Binding `json:"bindings"`
}

// Edge is a directed connection between nodes.
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

// Binding is one data-flow binding of a node.
type Binding struct {
	Direction          string `json:"direction"` // "export"|"import"|raw DATAFLOW
	StepElement        string `json:"step_element"`
	Expression         string `json:"expression"`
	ExpressionResolved string `json:"expression_resolved"`
	BindType           string `json:"bind_type"`
}

// ContainerElement is one workflow container element.
type ContainerElement struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	Category     string `json:"category"`
	TypeKind     string `json:"type_kind"`
	DDICType     string `json:"ddic_type"`
	Import       bool   `json:"import"`
	Export       bool   `json:"export"`
	Mandatory    bool   `json:"mandatory"`
	InitialValue string `json:"initial_value"`
	Props        string `json:"props"`
}

// Call is a subworkflow call from a node. WfdID is the task ID (WS + 8
// digits) of the called workflow, not its internal definition ID.
type Call struct {
	Node  string `json:"node"`
	WfdID string `json:"wfd_id"`
}

// Subworkflow is a resolved called workflow. WfdID is its task ID (WS + 8
// digits); Definition.WfdID holds the internal definition ID.
type Subworkflow struct {
	WfdID      string      `json:"wfd_id"`
	Found      bool        `json:"found"`
	Depth      int         `json:"depth"`
	CalledBy   []string    `json:"called_by"`
	Definition *Definition `json:"definition,omitempty"`
}
