package spec

import (
	"fmt"
	"regexp"
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
)

var simpleID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// Reserved are the words that cannot be simple node or edge IDs (spec 6.1).
var Reserved = []string{"all", "node", "kind", "tag", "children", "descendants", "edge_kind", "port", "row", "lift"}

// IsSimpleID reports whether id is a simple ID (spec 6.1).
func IsSimpleID(id string) bool {
	return simpleID.MatchString(id)
}

// NameProblem says why s is not a valid quoted ID, port or row name, or returns "".
func NameProblem(s string) string {
	switch {
	case s == "":
		return "is empty"
	case utf8.RuneCountInString(s) > 200:
		return "is longer than 200 characters"
	case slices.ContainsFunc([]rune(s), unicode.IsControl):
		return "contains a control character"
	}
	return ""
}

// CheckID reports a node or edge ID that is reserved or not a valid quoted ID.
func CheckID(what, id string, rng hcl.Range) hcl.Diagnostics {
	problem := NameProblem(id)
	if slices.Contains(Reserved, id) {
		problem = "is reserved"
	}
	if problem == "" {
		return nil
	}
	return hcl.Diagnostics{Errorf(rng, "Invalid ID", "The %s ID %q %s.", what, id, problem)}
}

// CheckSimple reports an ID that must be simple and is not.
func CheckSimple(what, id string, rng hcl.Range) hcl.Diagnostics {
	if IsSimpleID(id) {
		return nil
	}
	return hcl.Diagnostics{Errorf(rng, "Invalid ID", "The %s ID %q is not a simple ID (letters, digits, _ and -, starting with a letter or _).", what, id)}
}

// Errorf returns an error diagnostic at rng.
func Errorf(rng hcl.Range, summary, format string, args ...any) *hcl.Diagnostic {
	return &hcl.Diagnostic{Severity: hcl.DiagError, Summary: summary, Detail: fmt.Sprintf(format, args...), Subject: rng.Ptr()}
}

// Warnf returns a warning diagnostic at rng.
func Warnf(rng hcl.Range, summary, format string, args ...any) *hcl.Diagnostic {
	return &hcl.Diagnostic{Severity: hcl.DiagWarning, Summary: summary, Detail: fmt.Sprintf(format, args...), Subject: rng.Ptr()}
}

// DecodeExpr is gohcl.DecodeExpression, except that an expression that fails to evaluate
// reports only that failure and not also that its unknown result has the wrong type.
func DecodeExpr(expr hcl.Expression, ctx *hcl.EvalContext, target any) hcl.Diagnostics {
	if _, diags := expr.Value(ctx); diags.HasErrors() {
		return diags
	}
	return gohcl.DecodeExpression(expr, ctx, target)
}

// StepName returns the name in a traversal step: .name or ["name"].
func StepName(step hcl.Traverser) (string, bool) {
	switch step := step.(type) {
	case hcl.TraverseAttr:
		return step.Name, true
	case hcl.TraverseIndex:
		if step.Key.Type() == cty.String && step.Key.IsKnown() && !step.Key.IsNull() {
			return step.Key.AsString(), true
		}
	}
	return "", false
}
