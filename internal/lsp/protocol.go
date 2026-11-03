package lsp

// The parts of the Language Server Protocol dg uses.

// Position is a zero-based line and a column in UTF-16 code units.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a range between two positions, the end exclusive.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

func (r Range) contains(p Position) bool {
	return !before(p, r.Start) && !before(r.End, p)
}

func before(a, b Position) bool {
	return a.Line < b.Line || a.Line == b.Line && a.Character < b.Character
}

// Location is a range in a document.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type textDocumentID struct {
	URI string `json:"uri"`
}

type positionParams struct {
	TextDocument textDocumentID `json:"textDocument"`
	Position     Position       `json:"position"`
}

type didOpenParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
		Text    string `json:"text"`
	} `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	} `json:"textDocument"`
	ContentChanges []struct {
		Range *Range `json:"range"`
		Text  string `json:"text"`
	} `json:"contentChanges"`
}

type referenceParams struct {
	positionParams
	Context struct {
		IncludeDeclaration bool `json:"includeDeclaration"`
	} `json:"context"`
}

type renameParams struct {
	positionParams
	NewName string `json:"newName"`
}

type documentParams struct {
	TextDocument textDocumentID `json:"textDocument"`
}

// Diagnostic severities.
const (
	severityError       = 1
	severityWarning     = 2
	severityInformation = 3
)

type diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

// Completion item kinds.
const (
	itemField      = 5
	itemVariable   = 6
	itemClass      = 7
	itemInterface  = 8
	itemProperty   = 10
	itemKeyword    = 14
	itemEnumMember = 20
)

type completionItem struct {
	Label         string    `json:"label"`
	Kind          int       `json:"kind"`
	Detail        string    `json:"detail,omitempty"`
	Documentation string    `json:"documentation,omitempty"`
	FilterText    string    `json:"filterText,omitempty"`
	TextEdit      *TextEdit `json:"textEdit,omitempty"`
	Command       *command  `json:"command,omitempty"`
}

type command struct {
	Title   string `json:"title"`
	Command string `json:"command"`
}

// retrigger asks the editor to show completions again after an item that ends with a dot.
var retrigger = &command{Title: "Complete", Command: "editor.action.triggerSuggest"}

type hover struct {
	Contents markupContent `json:"contents"`
	Range    Range         `json:"range"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// TextEdit replaces a range of a document.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type workspaceEdit struct {
	DocumentChanges []any `json:"documentChanges"`
}

type textDocumentEdit struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version"`
	} `json:"textDocument"`
	Edits []TextEdit `json:"edits"`
}

type fileOperation struct {
	Kind   string `json:"kind"`
	URI    string `json:"uri,omitempty"`
	OldURI string `json:"oldUri,omitempty"`
	NewURI string `json:"newUri,omitempty"`
}

type prepareRenameResult struct {
	Range       Range  `json:"range"`
	Placeholder string `json:"placeholder"`
}
