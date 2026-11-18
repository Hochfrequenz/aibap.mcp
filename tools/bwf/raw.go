package bwf

// rawNode is one node row (SWDSMNODES, SWD_MNODES or SWD_NODES), trimmed to
// the columns the decoder uses. EvtName and EvtType hold the raw EVT_NAME and
// EVT_TYPE columns. Param1–Param3 are only filled by the SWD_NODES model.
type rawNode struct {
	ID, Type, Task, BlockID, ParentBlockID, ContCmd, Return, ReturnCode,
	ExcClass, MinTokens, ExpTokens, EvtName, EvtType, EvtOType, CaseConte string
	Param1, Param2, Param3 string
}

// rawLine is one edge row. LineType and ReturnCode are only filled by the
// SWD_LINES model.
type rawLine struct{ From, To, LineType, ReturnCode string }

// rawModel holds the raw rows of one workflow definition version.
type rawModel struct {
	// Model is the node model: "SWDS" is the block model with 4-character node
	// types (SWDSMNODES, SWD_MNODES); "SWD" is the runtime-copy model with
	// 1-character node types (SWD_NODES) and outcomes on the lines.
	Model                          string
	TablePrefix                    string // modelSWDS | "SWD_": where the rows were read
	WfdID, Version, MasterLanguage string
	Language                       string // requested language
	ActivatedOn                    string
	Nodes                          []rawNode
	Lines                          []rawLine
	Texts, Conds, Exprs, Bindings  []Row
	CntIdx, ContXML, TaskTexts     []Row
	ElemTexts                      []Row // SWDSWFCTXT / SWD_WFCTXT container element texts
}
