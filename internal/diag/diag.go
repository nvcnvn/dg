// Package diag holds dg's diagnostics, as printed by dg check and sent by dg serve and the
// language server.
package diag

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/nvcnvn/dg/internal/version"
)

// Severity is error, warning or info.
type Severity string

// Severities.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Info    Severity = "info"
)

// Diagnostic is one problem or note. File is a project-relative slash path, or a path under
// <generic>/ for the built-in pack; it and the positions are empty when there is no location.
type Diagnostic struct {
	Severity  Severity `json:"severity"`
	Summary   string   `json:"summary"`
	Detail    string   `json:"detail,omitempty"`
	File      string   `json:"file,omitempty"`
	Line      int      `json:"line,omitempty"`
	Column    int      `json:"column,omitempty"`
	EndLine   int      `json:"end_line,omitempty"`
	EndColumn int      `json:"end_column,omitempty"`
	// Byte and EndByte are the offsets in the file of the start and end, which the language
	// server converts to its positions: columns count grapheme clusters in HCL files and
	// characters in layout files.
	Byte    int `json:"-"`
	EndByte int `json:"-"`
}

// Diagnostics is a list of diagnostics.
type Diagnostics []Diagnostic

// At returns a diagnostic at rng.
func At(sev Severity, rng hcl.Range, summary, detail string) Diagnostic {
	return Diagnostic{
		Severity: sev, Summary: summary, Detail: detail, File: rng.Filename,
		Line: rng.Start.Line, Column: rng.Start.Column, EndLine: rng.End.Line, EndColumn: rng.End.Column,
		Byte: rng.Start.Byte, EndByte: rng.End.Byte,
	}
}

// FromHCL converts HCL diagnostics.
func FromHCL(diags hcl.Diagnostics) Diagnostics {
	out := make(Diagnostics, 0, len(diags))
	for _, d := range diags {
		sev := Error
		if d.Severity == hcl.DiagWarning {
			sev = Warning
		}
		if d.Subject == nil {
			out = append(out, Diagnostic{Severity: sev, Summary: d.Summary, Detail: d.Detail})
		} else {
			out = append(out, At(sev, *d.Subject, d.Summary, d.Detail))
		}
	}
	return out
}

// Expecting returns body so that decoding it, its blocks and what a partial decode leaves adds to
// each diagnostic about an unknown name, with no close match, the names the decoder expects. HCL's
// diagnostic does not give them.
func Expecting(body hcl.Body) hcl.Body {
	return expectingBody{body}
}

type expectingBody struct{ hcl.Body }

func (b expectingBody) Content(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	content, diags := b.Body.Content(schema)
	for _, d := range diags {
		if d.Summary == "Unsupported argument" || d.Summary == "Unsupported block type" {
			if !strings.Contains(d.Detail, "Did you mean") {
				d.Detail += expected(schema) + " " + UnknownName()
			}
		}
	}
	return expectingBlocks(content), diags
}

func (b expectingBody) PartialContent(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	content, remain, diags := b.Body.PartialContent(schema)
	return expectingBlocks(content), expectingBody{remain}, diags
}

func expectingBlocks(content *hcl.BodyContent) *hcl.BodyContent {
	for _, block := range content.Blocks {
		block.Body = expectingBody{block.Body}
	}
	return content
}

// expected lists the names schema takes, as sentences that follow a diagnostic's detail.
func expected(schema *hcl.BodySchema) string {
	var attrs, blocks []string
	for _, a := range schema.Attributes {
		attrs = append(attrs, a.Name)
	}
	for _, b := range schema.Blocks {
		blocks = append(blocks, b.Type)
	}
	var out string
	if len(attrs) > 0 {
		slices.Sort(attrs)
		out += " Expected arguments: " + strings.Join(attrs, ", ") + "."
	}
	if len(blocks) > 0 {
		slices.Sort(blocks)
		out += " Expected blocks: " + strings.Join(blocks, ", ") + "."
	}
	return out
}

// Newer is the sentence that ends a diagnostic only a project written for a newer dg would get.
func Newer() string {
	return fmt.Sprintf("If a newer dg wrote this project, update dg: this one is %s.", version.String())
}

// UnknownName is the sentence that ends a diagnostic about a name dg does not know, which is most
// often a mistake but may come from a newer dg.
func UnknownName() string {
	return fmt.Sprintf("If the name is right, a newer dg may know it: this one is %s.", version.String())
}

// HasErrors reports whether ds has an error.
func (ds Diagnostics) HasErrors() bool {
	return slices.ContainsFunc(ds, func(d Diagnostic) bool { return d.Severity == Error })
}

// Has reports whether ds has a diagnostic of severity sev.
func (ds Diagnostics) Has(sev Severity) bool {
	return slices.ContainsFunc(ds, func(d Diagnostic) bool { return d.Severity == sev })
}

var rank = map[Severity]int{Error: 0, Warning: 1, Info: 2}

// Sort orders ds by file, position, severity and text.
func (ds Diagnostics) Sort() {
	slices.SortStableFunc(ds, func(a, b Diagnostic) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Column, b.Column),
			cmp.Compare(rank[a.Severity], rank[b.Severity]),
			cmp.Compare(a.Summary, b.Summary),
			cmp.Compare(a.Detail, b.Detail),
		)
	})
}

// String formats d as "file:line:column: severity: summary: detail".
func (d Diagnostic) String() string {
	msg := string(d.Severity) + ": " + d.Summary
	if d.Detail != "" {
		msg += ": " + d.Detail
	}
	if d.File == "" {
		return msg
	}
	return fmt.Sprintf("%s:%d:%d: %s", d.File, d.Line, d.Column, msg)
}
