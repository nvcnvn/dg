package project

import (
	"encoding/json"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// Vocabulary is what a project's packs, model and views define, as dg vocabulary prints it.
type Vocabulary struct {
	// Packs names the loaded packs in load order; the project's own spec.hcl loads after them.
	Packs     []string   `json:"packs"`
	Kinds     []KindInfo `json:"kinds"`
	EdgeKinds []string   `json:"edge_kinds"`
	Tags      []string   `json:"tags"`
	Views     []ViewInfo `json:"views"`
}

// KindInfo is a kind with what it inherits; Pack names the pack that defines it, empty for the
// project's own spec.hcl, and Layout is a container kind's layout policy (spec 7.3).
type KindInfo struct {
	Name            string     `json:"name"`
	Pack            string     `json:"pack,omitempty"`
	Extends         string     `json:"extends,omitempty"`
	Container       bool       `json:"container,omitempty"`
	Layout          string     `json:"layout,omitempty"`
	Rows            bool       `json:"rows,omitempty"`
	Ports           []string   `json:"ports,omitempty"`
	DefaultPortSide string     `json:"default_port_side,omitempty"`
	Properties      []PropInfo `json:"properties,omitempty"`
}

// PropInfo is a property a kind declares.
type PropInfo struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Default json.RawMessage `json:"default,omitempty"`
}

// ViewInfo is a view.
type ViewInfo struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

// Vocabulary returns the kinds and edge kinds p defines, the tags its model uses or its styles
// select, and its views.
func (p *Project) Vocabulary() Vocabulary {
	sp := p.Spec
	v := Vocabulary{Packs: []string{packName(genericDir)}, Kinds: []KindInfo{}, EdgeKinds: slices.Sorted(maps.Keys(sp.EdgeKinds)), Tags: []string{}, Views: []ViewInfo{}}
	for _, dir := range p.PackDirs {
		v.Packs = append(v.Packs, packName(dir))
	}
	for _, name := range slices.Sorted(maps.Keys(sp.Kinds)) {
		k := KindInfo{Name: name, Pack: packName(path.Dir(sp.Kinds[name].Range.Filename)), Extends: sp.Kinds[name].Extends, Container: sp.IsContainer(name), Rows: sp.HasRows(name), Ports: sp.Ports(name)}
		if set, ok := p.Styles.ForNode(sp, name, nil)["layout"]; ok && k.Container {
			k.Layout = set.Value.AsString()
		}
		if len(k.Ports) > 0 {
			k.DefaultPortSide = sp.DefaultPortSide(name)
		}
		for _, anc := range sp.Lineage(name) {
			for _, prop := range anc.Properties {
				info := PropInfo{Name: prop.Name, Type: prop.Type.FriendlyName()}
				if prop.Default != cty.NilVal {
					info.Default, _ = ctyjson.Marshal(prop.Default, prop.Default.Type())
				}
				k.Properties = append(k.Properties, info)
			}
		}
		v.Kinds = append(v.Kinds, k)
	}
	tags := map[string]bool{}
	for _, n := range p.Model.Nodes {
		for _, t := range n.Tags {
			tags[t] = true
		}
	}
	for _, e := range p.Model.Edges {
		for _, t := range e.Tags {
			tags[t] = true
		}
	}
	for _, r := range slices.Concat(p.Styles.Node, p.Styles.Edge) {
		if t, ok := strings.CutPrefix(r.Label, "tag."); ok {
			tags[t] = true
		}
	}
	v.Tags = append(v.Tags, slices.Sorted(maps.Keys(tags))...)
	for _, id := range slices.Sorted(maps.Keys(p.Views)) {
		v.Views = append(v.Views, ViewInfo{ID: id, Title: p.Views[id].Title})
	}
	return v
}

// packName names the pack in dir, the directory a spec.hcl is in: the last element of its path,
// or nothing for the project root.
func packName(dir string) string {
	switch dir {
	case genericDir:
		return "generic"
	case ".":
		return ""
	}
	return path.Base(dir)
}
