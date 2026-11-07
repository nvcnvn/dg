package project

import (
	"errors"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/style"
)

// Installed is an installed pack (spec 4.7).
type Installed struct {
	Name string
	Dir  string
	// Source and Version are those of the pack block, "" when it leaves one out.
	Source, Version string
	// Range is the pack block in dg.hcl.
	Range hcl.Range
	// files is nil when the pack could not be read, which leaves it not loaded.
	files fstest.MapFS
	// kinds, edgeKinds and shapes hold what the pack defines, as its author writes the names.
	kinds, edgeKinds, shapes map[string]bool
	// specUnparsed is set when spec.hcl has syntax errors, so kinds and edgeKinds may be incomplete.
	specUnparsed bool
}

func (in *Installed) prefixed(name string) string {
	return in.Name + "-" + name
}

// installedOf returns the installed pack that holds project file rel, or nil. It ignores case,
// since on a case-insensitive disk packs/AWS is packs/aws.
func (p *Project) installedOf(rel string) *Installed {
	lower := strings.ToLower(rel)
	for _, in := range p.Installed {
		if strings.HasPrefix(lower, strings.ToLower(in.Dir)+"/") {
			return in
		}
	}
	return nil
}

// ShortKind returns kind's name without the prefix of the installed pack that defines it, if one does.
func (p *Project) ShortKind(kind string) string {
	if k := p.Spec.Kinds[kind]; k != nil {
		if in := p.installedOf(k.Range.Filename); in != nil {
			return strings.TrimPrefix(kind, in.Name+"-")
		}
	}
	return kind
}

// ProjectKind returns the name that a kind, or an edge kind, written as name in project file has
// for the project: with the prefix when the file belongs to an installed pack that defines it.
func (p *Project) ProjectKind(file, name string, edge bool) string {
	in := p.installedOf(file)
	if in == nil || edge && !in.edgeKinds[name] || !edge && !in.kinds[name] {
		return name
	}
	return in.prefixed(name)
}

// readLock reads dg.lock, a project without one having an empty lock, and reports whether what
// it returns can be trusted to say a pack is not locked.
func (l *loader) readLock() (*Lock, bool, error) {
	lock := &Lock{}
	if info, err := fs.Stat(l.fsys, LockFile); err == nil && info.IsDir() {
		rng := hcl.Range{Filename: LockFile, Start: hcl.InitialPos, End: hcl.InitialPos}
		l.diags = append(l.diags, spec.Errorf(rng, "Not a file", "%s is a directory.", LockFile))
		return lock, false, nil
	}
	src, err := fs.ReadFile(l.fsys, LockFile)
	if errors.Is(err, fs.ErrNotExist) {
		return lock, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	lock, d := DecodeLock(src, LockFile)
	l.diags = append(l.diags, d...)
	return lock, !d.HasErrors(), nil
}

// install checks the pack blocks against dg.lock and the packs' directories, and reads the packs.
func (l *loader) install(blocks []packBlock, lock *Lock, lockValid bool) {
	p := l.p
	for _, b := range blocks {
		if problem := LocalNameProblem(b.Name); problem != "" {
			l.diags = append(l.diags, spec.Errorf(b.Range, "Invalid ID", "The pack name %q %s.", b.Name, problem))
			continue
		}
		dir := path.Join("packs", b.Name)
		if other := p.installedOf(dir + "/"); other != nil {
			l.diags = append(l.diags, spec.Errorf(b.Range, "Duplicate definition", "The pack %q needs the directory %s, which the pack at line %d of this file has; names that differ only by case share a directory.", b.Name, dir, other.Range.Start.Line))
			continue
		}
		in := &Installed{Name: b.Name, Dir: dir, Range: b.Range, kinds: map[string]bool{}, edgeKinds: map[string]bool{}, shapes: map[string]bool{}}
		p.Installed = append(p.Installed, in)
		if b.Source == nil {
			l.diags = append(l.diags, spec.Errorf(b.Range, "Missing required argument", `The argument "source" is required.`))
		} else {
			in.Source = *b.Source
		}
		if b.Version == nil {
			l.diags = append(l.diags, spec.Errorf(b.Range, "Missing required argument", `The argument "version" is required.`))
		} else {
			in.Version = *b.Version
		}
		entry, locked := lock.Packs[b.Name]
		switch {
		case b.Source == nil || b.Version == nil:
		case !locked && lockValid:
			l.diags = append(l.diags, spec.Errorf(b.Range, "Pack not locked", "%s has no entry for the pack %q, so dg pack install cannot restore it. Run dg pack add to install it.", LockFile, b.Name))
		case locked && (entry.Source != *b.Source || entry.Version != *b.Version):
			l.diags = append(l.diags, spec.Errorf(b.Range, "Pack not locked", "%s has the pack %q as %s at %s, not %s at %s. Run dg pack update to install what this block asks for, or restore the block; dg pack install installs what %s holds.", LockFile, b.Name, entry.Source, entry.Version, *b.Source, *b.Version, LockFile))
		}
		files, err := ReadPack(l.fsys, in.Dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			l.diags = append(l.diags, spec.Errorf(b.Range, "Pack not installed", "The directory %s does not exist. Run dg pack install to restore it.", in.Dir))
		case err != nil:
			l.diags = append(l.diags, spec.Errorf(b.Range, "Pack not installed", "The pack %q cannot be read: %v. Run dg pack install to repair it.", b.Name, err))
		default:
			in.files = files
			if hash := PackHash(files); locked && hash != entry.Hash {
				l.diags = append(l.diags, spec.Warnf(b.Range, "Pack changed", "The files of the pack %q no longer match their hash in %s, so this is not the pack that was installed. Run dg pack install to restore it.", b.Name, LockFile))
			}
		}
	}
}

// parsePack is parse for a file of an installed pack, recorded in Files when it exists. It reads
// from the pack's own files, so that file() reaches nothing else.
func (l *loader) parsePack(in *Installed, name, role string, decode decodeFunc) {
	rel := path.Join(in.Dir, name)
	if err := l.parse(in.files, "pack", name, rel, decode); err == nil {
		l.p.Files = append(l.p.Files, File{Path: rel, Role: role})
	}
}

// clash reports that pack in yields a name defined at prev, where a definition of generic or of
// an earlier installed pack is.
func (l *loader) clash(in *Installed, what, name string, prev hcl.Range) {
	other := l.p.installedOf(prev.Filename)
	if other == nil {
		l.diags = append(l.diags, spec.Errorf(in.Range, "Name clash", "The pack %q yields the %s %q, which the generic pack defines. Give the pack another local name: dg pack add --as.", in.Name, what, name))
		return
	}
	for _, at := range []*Installed{other, in} {
		l.diags = append(l.diags, spec.Errorf(at.Range, "Name clash", "The packs %q and %q both yield the %s %q. Give one another local name: dg pack add --as.", other.Name, in.Name, what, name))
	}
}

func isGeneric(rng hcl.Range) bool {
	return strings.HasPrefix(rng.Filename, genericDir+"/")
}

// packSpec loads the spec of pack in, giving what it defines the pack's prefix.
func (l *loader) packSpec(in *Installed) {
	sp := spec.New()
	l.parsePack(in, "spec.hcl", RoleSpec, sp.Decode)
	in.specUnparsed = slices.Contains(l.p.Unparsed, path.Join(in.Dir, "spec.hcl"))
	target := l.p.Spec
	for name := range sp.Kinds {
		in.kinds[name] = true
	}
	for name := range sp.EdgeKinds {
		in.edgeKinds[name] = true
	}
	for _, short := range slices.Sorted(maps.Keys(sp.Kinds)) {
		k := sp.Kinds[short]
		parent := target.Kinds[k.Extends]
		switch {
		case in.kinds[k.Extends]:
			k.Extends = in.prefixed(k.Extends)
		case k.Extends == "":
			k.Extends = spec.RootKind
		case parent == nil || !isGeneric(parent.Range):
			l.diags = append(l.diags, spec.Errorf(k.Range, "Outside the pack", "Kind %q extends %q; a pack extends its own kinds and the generic pack's.", short, k.Extends))
			k.Extends = spec.RootKind
		}
		k.Name = in.prefixed(short)
		if prev := target.Kinds[k.Name]; prev != nil {
			l.clash(in, "kind", k.Name, prev.Range)
		}
		target.Kinds[k.Name] = k
	}
	for _, short := range slices.Sorted(maps.Keys(sp.EdgeKinds)) {
		k := sp.EdgeKinds[short]
		k.Name = in.prefixed(short)
		if prev := target.EdgeKinds[k.Name]; prev != nil {
			l.clash(in, "edge kind", k.Name, prev.Range)
		}
		target.EdgeKinds[k.Name] = k
	}
}

// packStyle loads the styles of pack in, whose spec is loaded, giving what it defines and the
// names it uses of its own the pack's prefix.
func (l *loader) packStyle(in *Installed) {
	st := style.New()
	l.parsePack(in, "style.hcl", RoleStyle, st.Decode)
	target := l.p.Styles
	for name := range st.Shapes {
		in.shapes[name] = true
	}
	for _, short := range slices.Sorted(maps.Keys(st.Shapes)) {
		sh := st.Shapes[short]
		sh.Name = in.prefixed(short)
		if prev := target.Shapes[sh.Name]; prev != nil {
			l.clash(in, "shape", sh.Name, prev.Range)
		}
		target.Shapes[sh.Name] = sh
	}
	for _, r := range st.Node {
		if l.packLabel(in, &r, "style", in.kinds, "kinds") {
			l.packShape(in, r)
			target.Node = l.packRule(target.Node, "style", r)
		}
	}
	for _, r := range st.Edge {
		if l.packLabel(in, &r, "edge_style", in.edgeKinds, "edge kinds") {
			target.Edge = l.packRule(target.Edge, "edge_style", r)
		}
	}
	for _, r := range st.Overlay {
		r.Label = in.prefixed(r.Label)
		for _, prev := range target.Overlay {
			if prev.Label == r.Label {
				l.clash(in, "overlay style", r.Label, prev.Range)
			}
		}
		target.Overlay = append(target.Overlay, r)
	}
}

// packLabel gives the label of rule r the pack's prefix when it names a kind in own, and reports
// whether the rule is kept: its label is one of those kinds or a tag. A label cannot be judged
// while spec.hcl does not parse.
func (l *loader) packLabel(in *Installed, r *style.Rule, block string, own map[string]bool, what string) bool {
	if own[r.Label] {
		r.Label = in.prefixed(r.Label)
		return true
	}
	if strings.HasPrefix(r.Label, "tag.") {
		return true
	}
	if in.specUnparsed {
		return false
	}
	l.diags = append(l.diags, spec.Errorf(r.Range, "Outside the pack", "The %s label %q names none of the pack's %s; a pack styles its own kinds and tags.", block, r.Label, what))
	return false
}

// packShape resolves the shape property of rule r: the pack's own shape, or else a built-in
// shape or the generic pack's.
func (l *loader) packShape(in *Installed, r style.Rule) {
	set, ok := r.Props["shape"]
	if !ok {
		return
	}
	name := set.Value.AsString()
	sh := l.p.Styles.Shapes[name]
	switch {
	case in.shapes[name]:
		r.Props["shape"] = style.Setting{Value: cty.StringVal(in.prefixed(name)), Range: set.Range}
	case !slices.Contains(style.BuiltinShapes, name) && (sh == nil || !isGeneric(sh.Range)):
		l.diags = append(l.diags, spec.Errorf(set.Range, "Outside the pack", "The shape %q is none of the pack's, a built-in shape or the generic pack's.", name))
		delete(r.Props, "shape")
	}
}

// packRule appends rule r of a pack to rules. A rule with its label there is generic's, which it
// overrides, or another installed pack's on the same tag, which is worth a warning.
func (l *loader) packRule(rules []style.Rule, block string, r style.Rule) []style.Rule {
	for _, prev := range rules {
		switch {
		case prev.Label != r.Label:
		case isGeneric(prev.Range):
			l.p.Styles.Overrides = append(l.p.Styles.Overrides, spec.Override{What: block, Name: r.Label, Range: r.Range, Prev: prev.Range})
		case strings.HasPrefix(r.Label, "tag."):
			at := prev.Range
			l.diags = append(l.diags, spec.Warnf(r.Range, "Tag styled by two packs", "The %s %q is also set at %s:%d:%d; this pack loads later, so its properties win.", block, r.Label, at.Filename, at.Start.Line, at.Start.Column))
		}
	}
	return append(rules, r)
}
