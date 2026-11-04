package lsp

import (
	"cmp"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/view"
)

// Symbol types: the IDs go to definition and find references know.
const (
	symNode     = "node"
	symEdge     = "edge"
	symKind     = "kind"
	symEdgeKind = "edge_kind"
	symView     = "view"
)

// occurrence is where an ID is defined or referenced.
type occurrence struct {
	typ  string
	id   string
	file string
	rng  Range
	def  bool
}

// index is every occurrence of an ID in a project's HCL and layout files, sorted by file and
// position.
type index struct {
	occs []occurrence
}

type indexer struct {
	l    *loaded
	occs []occurrence
}

// traversalRoots are the roots of the traversals that name an ID, and what they name.
var traversalRoots = map[string]string{
	"node": symNode, "children": symNode, "descendants": symNode,
	"edge": symEdge, "kind": symKind, "edge_kind": symEdgeKind,
}

func buildIndex(l *loaded) *index {
	p := l.p
	ix := &indexer{l: l}
	for _, f := range p.Files {
		switch f.Role {
		case project.RoleModel, project.RoleViews, project.RoleSpec, project.RoleStyle:
			file, _ := hclsyntax.ParseConfig([]byte(l.texts[f.Path]), f.Path, hcl.InitialPos)
			if body, ok := file.Body.(*hclsyntax.Body); ok {
				ix.body(f.Role, body, nil)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Layouts)) {
		layout := p.Layouts[id]
		file := path.Join(p.LayoutDir, id+".layout.yaml")
		ix.occs = append(ix.occs, occurrence{typ: symView, id: id, file: file})
		for key, pl := range layout.Place {
			ix.layoutKey(symNode, key, pl.Range)
		}
		for key, h := range layout.EdgeHints {
			ix.layoutKey(symEdge, key, h.Range)
		}
	}
	slices.SortFunc(ix.occs, func(a, b occurrence) int {
		return cmp.Or(strings.Compare(a.file, b.file), cmp.Compare(a.rng.Start.Line, b.rng.Start.Line),
			cmp.Compare(a.rng.Start.Character, b.rng.Start.Character))
	})
	return &index{occs: ix.occs}
}

func (ix *indexer) layoutKey(typ, id string, rng hcl.Range) {
	key := view.EncodeID(id)
	if strings.HasPrefix(ix.l.texts[rng.Filename][rng.Start.Byte:], `"`) {
		key = `"` + key + `"`
	}
	rng.End.Byte = rng.Start.Byte + len(key)
	ix.add(typ, id, rng, false)
}

func (ix *indexer) add(typ, id string, rng hcl.Range, def bool) {
	ix.occs = append(ix.occs, occurrence{typ: typ, id: id, file: rng.Filename, def: def,
		rng: ix.l.linesOf(rng.Filename).rng(rng.Start.Byte, rng.End.Byte)})
}

// inside returns rng without its first and last n bytes, such as the quotes of a label.
func inside(rng hcl.Range, n int) hcl.Range {
	rng.Start.Byte += n
	rng.End.Byte = max(rng.End.Byte-n, rng.Start.Byte)
	return rng
}

// body indexes the blocks and attributes of body, in the blocks of types outer.
func (ix *indexer) body(role string, body *hclsyntax.Body, outer []string) {
	parent := ""
	if len(outer) > 0 {
		parent = outer[len(outer)-1]
	}
	for _, b := range body.Blocks {
		if len(b.Labels) == 1 {
			ix.block(role, b, parent)
		}
		ix.body(role, b.Body, append(slices.Clone(outer), b.Type))
	}
	for _, name := range slices.Sorted(maps.Keys(body.Attributes)) {
		a := body.Attributes[name]
		if role == project.RoleModel && name == "kind" && (parent == "node" || parent == "edge") ||
			role == project.RoleSpec && name == "extends" && parent == "kind" {
			if lit, ok := stringLiteral(a.Expr); ok {
				typ := symKind
				if parent == "edge" {
					typ = symEdgeKind
				}
				ix.add(typ, lit.Val.AsString(), lit.SrcRange, false)
			}
		}
		if role != project.RoleModel && role != project.RoleViews {
			continue
		}
		hclsyntax.VisitAll(a.Expr, func(n hclsyntax.Node) hcl.Diagnostics {
			t, ok := n.(*hclsyntax.ScopeTraversalExpr)
			if !ok || len(t.Traversal) < 2 {
				return nil
			}
			typ := traversalRoots[t.Traversal.RootName()]
			if role == project.RoleModel && typ != symNode || typ == "" {
				return nil
			}
			id, ok := spec.StepName(t.Traversal[1])
			if !ok {
				return nil
			}
			rng := t.Traversal[1].SourceRange()
			if _, attr := t.Traversal[1].(hcl.TraverseAttr); attr {
				rng.Start.Byte++
			} else {
				rng = inside(rng, 2)
			}
			ix.add(typ, id, rng, false)
			return nil
		})
	}
}

// block indexes the label of block b, inside a block of type parent.
func (ix *indexer) block(role string, b *hclsyntax.Block, parent string) {
	label, rng := b.Labels[0], b.LabelRanges[0]
	if strings.HasPrefix(ix.l.texts[rng.Filename][rng.Start.Byte:], `"`) {
		rng = inside(rng, 1)
	}
	switch {
	case role == project.RoleModel && parent == "" && (b.Type == "node" || b.Type == "edge"):
		ix.add(b.Type, label, rng, true)
	case role == project.RoleSpec && parent == "" && (b.Type == "kind" || b.Type == "edge_kind"):
		ix.add(b.Type, label, rng, true)
	case role == project.RoleViews && parent == "" && b.Type == "view":
		ix.add(symView, label, rng, true)
	case role == project.RoleViews && parent == "view" && b.Type == "node":
		ix.add("node", label, rng, false)
	case b.Type == "style" || b.Type == "edge_style":
		if role != project.RoleStyle && !(role == project.RoleViews && parent == "view") {
			return
		}
		form, id, ok := strings.Cut(label, ".")
		switch {
		case !ok && b.Type == "style":
			ix.add(symKind, label, rng, false)
		case !ok:
			ix.add(symEdgeKind, label, rng, false)
		case role == project.RoleViews && (form == "node" && b.Type == "style" || form == "edge" && b.Type == "edge_style"):
			rng.Start.Byte += len(form) + 1
			ix.add(form, id, rng, false)
		}
	}
}

// stringLiteral returns expr when it is a string without interpolation.
func stringLiteral(expr hclsyntax.Expression) (*hclsyntax.LiteralValueExpr, bool) {
	t, ok := expr.(*hclsyntax.TemplateExpr)
	if !ok || len(t.Parts) != 1 {
		return nil, false
	}
	lit, ok := t.Parts[0].(*hclsyntax.LiteralValueExpr)
	if !ok || !lit.Val.IsKnown() || lit.Val.IsNull() || !lit.Val.Type().Equals(cty.String) {
		return nil, false
	}
	return lit, true
}

// at returns the occurrence at pos in file.
func (ix *index) at(file string, pos Position) (occurrence, bool) {
	for _, o := range ix.occs {
		if o.file == file && o.rng.contains(pos) && o.rng != (Range{}) {
			return o, true
		}
	}
	return occurrence{}, false
}

// of returns the occurrences of the ID of type typ.
func (ix *index) of(typ, id string) []occurrence {
	var out []occurrence
	for _, o := range ix.occs {
		if o.typ == typ && o.id == id {
			out = append(out, o)
		}
	}
	return out
}
