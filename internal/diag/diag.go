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
		detail := d.Detail
		if unknownName(d) {
			detail += " " + Newer()
		}
		if d.Subject == nil {
			out = append(out, Diagnostic{Severity: sev, Summary: d.Summary, Detail: detail})
		} else {
			out = append(out, At(sev, *d.Subject, d.Summary, detail))
		}
	}
	return out
}

// unknownName reports whether d is about a name this dg does not know, and that HCL found no
// close match for, so it is no typo.
func unknownName(d *hcl.Diagnostic) bool {
	return (d.Summary == "Unsupported argument" || d.Summary == "Unsupported block type") &&
		!strings.Contains(d.Detail, "Did you mean")
}

// Newer is the sentence that ends a diagnostic a project written for a newer dg would get.
func Newer() string {
	return fmt.Sprintf("If a newer dg wrote this project, update dg: this one is %s.", version.String())
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
