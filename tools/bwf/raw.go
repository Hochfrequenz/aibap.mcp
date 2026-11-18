package bwf

// rawNode is one SWDSMNODES (or SWD_NODES) row, trimmed to the columns the decoder uses.
// EvtName and EvtType hold the raw EVT_NAME and EVT_TYPE columns.
type rawNode struct {
	ID, Type, Task, BlockID, ParentBlockID, ContCmd, Return, ReturnCode,
	ExcClass, MinTokens, ExpTokens, EvtName, EvtType, EvtOType, CaseConte string
}

// rawLine is one edge row. ReturnCode is only filled by the SWD_LINES model.
type rawLine struct{ From, To, ReturnCode string }

// rawModel holds the raw rows of one workflow definition version.
type rawModel struct {
	Model                          string // "SWDS" | "SWD"
	WfdID, Version, MasterLanguage string
	ActivatedOn                    string
	Nodes                          []rawNode
	Lines                          []rawLine
	Texts, Conds, Exprs, Bindings  []Row
	CntIdx, ContXML, TaskTexts     []Row
}
