// Package spec holds node kinds and edge kinds and their inheritance.
package spec

import (
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

// RootKind is the kind every node kind extends.
const RootKind = "node"

// DefaultEdgeKind is the kind of an edge that names none.
const DefaultEdgeKind = "default"

// Kind is a node kind.
type Kind struct {
	Name      string
	Extends   string
	Container *bool
	Range     hcl.Range
}

// EdgeKind is an edge kind.
type EdgeKind struct {
	Name  string
	Range hcl.Range
}

// Spec is the set of kinds loaded from packs and the project.
type Spec struct {
	Kinds     map[string]*Kind
	EdgeKinds map[string]*EdgeKind
}

// New returns an empty Spec.
func New() *Spec {
	return &Spec{Kinds: map[string]*Kind{}, EdgeKinds: map[string]*EdgeKind{}}
}

type file struct {
	Kinds []struct {
		Name      string    `hcl:"name,label"`
		Extends   *string   `hcl:"extends,optional"`
		Container *bool     `hcl:"container,optional"`
		Range     hcl.Range `hcl:",def_range"`
	} `hcl:"kind,block"`
	EdgeKinds []struct {
		Name  string    `hcl:"name,label"`
		Range hcl.Range `hcl:",def_range"`
	} `hcl:"edge_kind,block"`
}

// Decode adds the kind and edge_kind blocks of body to s, replacing definitions from earlier files.
func (s *Spec) Decode(body hcl.Body) hcl.Diagnostics {
	var f file
	diags := gohcl.DecodeBody(body, nil, &f)
	kinds := map[string]hcl.Range{}
	for _, k := range f.Kinds {
		if prev, ok := kinds[k.Name]; ok {
			diags = append(diags, duplicate("kind", k.Name, k.Range, prev))
			continue
		}
		kinds[k.Name] = k.Range
		kind := &Kind{Name: k.Name, Container: k.Container, Range: k.Range}
		if k.Name != RootKind {
			kind.Extends = RootKind
		}
		if k.Extends != nil {
			kind.Extends = *k.Extends
		}
		s.Kinds[k.Name] = kind
	}
	edgeKinds := map[string]hcl.Range{}
	for _, k := range f.EdgeKinds {
		if prev, ok := edgeKinds[k.Name]; ok {
			diags = append(diags, duplicate("edge kind", k.Name, k.Range, prev))
			continue
		}
		edgeKinds[k.Name] = k.Range
		s.EdgeKinds[k.Name] = &EdgeKind{Name: k.Name, Range: k.Range}
	}
	return diags
}

func duplicate(what, name string, rng, prev hcl.Range) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Duplicate definition",
		Detail:   fmt.Sprintf("The %s %q is already defined at line %d of this file.", what, name, prev.Start.Line),
		Subject:  rng.Ptr(),
	}
}

// Validate reports kinds that extend an undefined kind or are in an extends cycle.
func (s *Spec) Validate() hcl.Diagnostics {
	var diags hcl.Diagnostics
	for _, name := range slices.Sorted(maps.Keys(s.Kinds)) {
		k := s.Kinds[name]
		if k.Extends == "" {
			continue
		}
		if _, ok := s.Kinds[k.Extends]; !ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Undefined kind",
				Detail:   fmt.Sprintf("Kind %q extends undefined kind %q.", name, k.Extends),
				Subject:  k.Range.Ptr(),
			})
			continue
		}
		if s.inCycle(name) {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Extends cycle",
				Detail:   fmt.Sprintf("Kind %q extends itself through its ancestors.", name),
				Subject:  k.Range.Ptr(),
			})
		}
	}
	return diags
}

func (s *Spec) inCycle(name string) bool {
	seen := map[string]bool{}
	for k := s.Kinds[name]; k != nil && !seen[k.Name]; k = s.Kinds[k.Extends] {
		seen[k.Name] = true
		if k.Extends == name {
			return true
		}
	}
	return false
}

// IsContainer reports whether kind is a container, as set by the nearest kind in its lineage.
func (s *Spec) IsContainer(kind string) bool {
	chain := s.Lineage(kind)
	for i := len(chain) - 1; i >= 0; i-- {
		if c := chain[i].Container; c != nil {
			return *c
		}
	}
	return false
}

// EdgeLineage returns the defined edge kinds from default down to name.
func (s *Spec) EdgeLineage(name string) []*EdgeKind {
	var chain []*EdgeKind
	if name != DefaultEdgeKind && s.EdgeKinds[DefaultEdgeKind] != nil {
		chain = append(chain, s.EdgeKinds[DefaultEdgeKind])
	}
	if k := s.EdgeKinds[name]; k != nil {
		chain = append(chain, k)
	}
	return chain
}

// Lineage returns the defined kinds from the root down to name, stopping at a cycle.
func (s *Spec) Lineage(name string) []*Kind {
	var chain []*Kind
	seen := map[string]bool{}
	for k := s.Kinds[name]; k != nil && !seen[k.Name]; k = s.Kinds[k.Extends] {
		seen[k.Name] = true
		chain = append([]*Kind{k}, chain...)
	}
	return chain
}
