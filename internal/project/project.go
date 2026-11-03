// Package project reads dg.hcl, finds and loads a project's files, and knows which are read-only.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/model"
	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/style"
	"github.com/nvcnvn/dg/internal/view"
	"github.com/nvcnvn/dg/packs/generic"
)

// File roles.
const (
	RoleProject = "project"
	RoleSpec    = "spec"
	RoleStyle   = "style"
	RoleModel   = "model"
	RoleViews   = "views"
	RoleLayout  = "layout"
)

// File is a project file that was loaded: Path is slash-separated and relative to the project.
type File struct {
	Path string
	Role string
}

// Project is a loaded project.
type Project struct {
	Dir       string
	Name      string
	Files     []File
	ReadOnly  []string
	ViewsFile string
	LayoutDir string
	Font      string
	FontRange hcl.Range
	// ConfigInvalid is set when dg.hcl has errors, so ReadOnly may be incomplete.
	ConfigInvalid bool
	view.Sources
}

// ProjectFile is the name of the project file.
const ProjectFile = "dg.hcl"

// genericDir names the built-in pack's files in diagnostics; no project path starts with "<".
const genericDir = "<generic>"

type projectBlock struct {
	Name      string         `hcl:"name,label"`
	Packs     *hcl.Attribute `hcl:"packs,optional"`
	Models    *hcl.Attribute `hcl:"models,optional"`
	ReadOnly  *hcl.Attribute `hcl:"read_only,optional"`
	Views     *hcl.Attribute `hcl:"views,optional"`
	LayoutDir *hcl.Attribute `hcl:"layout_dir,optional"`
	Grid      *hcl.Attribute `hcl:"grid,optional"`
	Font      *hcl.Attribute `hcl:"font,optional"`
	Range     hcl.Range      `hcl:",def_range"`
}

// Load reads the project in dir. Problems in file content are diagnostics, whose file names are
// relative to dir; a project that cannot be read is an error. Symbolic links that lead out of dir
// are not followed.
func Load(dir string) (*Project, diag.Diagnostics, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory", dir)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	return load(root.FS(), dir)
}

type loader struct {
	fsys  fs.FS
	p     *Project
	diags hcl.Diagnostics
}

type decodeFunc func(hcl.Body, *hcl.EvalContext) hcl.Diagnostics

// parse parses and decodes file rel of fsys, named name in diagnostics; it returns
// fs.ErrNotExist if there is no such file.
func (l *loader) parse(fsys fs.FS, rel, name string, decode decodeFunc) error {
	src, err := fs.ReadFile(fsys, rel)
	if err != nil {
		return err
	}
	f, d := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
	l.diags = append(l.diags, d...)
	if !d.HasErrors() {
		l.diags = append(l.diags, decode(f.Body, evalContext(fsys, path.Dir(rel)))...)
	}
	return nil
}

// parseProject is parse for a file of the project, recorded in Files when it exists. A
// directory where a file is expected is a diagnostic.
func (l *loader) parseProject(rel, role string, decode decodeFunc) error {
	if info, err := fs.Stat(l.fsys, rel); err == nil && info.IsDir() {
		rng := hcl.Range{Filename: rel, Start: hcl.InitialPos, End: hcl.InitialPos}
		l.diags = append(l.diags, spec.Errorf(rng, "Not a file", "%s is a directory.", rel))
		return nil
	}
	err := l.parse(l.fsys, rel, rel, decode)
	if err == nil {
		l.p.Files = append(l.p.Files, File{Path: rel, Role: role})
	}
	return err
}

func load(fsys fs.FS, dir string) (*Project, diag.Diagnostics, error) {
	p := &Project{Dir: dir, ViewsFile: "views.hcl", LayoutDir: "views", Sources: view.Sources{
		Spec: spec.New(), Styles: style.New(), Model: model.New(), Views: view.Views{}, Layouts: map[string]*view.Layout{}, Grid: 10,
	}}
	l := &loader{fsys: fsys, p: p}

	var cfg *projectBlock
	var cfgCtx *hcl.EvalContext
	err := l.parseProject(ProjectFile, RoleProject, func(body hcl.Body, ctx *hcl.EvalContext) hcl.Diagnostics {
		var f struct {
			Projects []projectBlock `hcl:"project,block"`
		}
		diags := gohcl.DecodeBody(body, ctx, &f)
		if len(f.Projects) > 1 {
			diags = append(diags, spec.Errorf(f.Projects[1].Range, "Duplicate project block", "dg.hcl holds at most one project block."))
		}
		if len(f.Projects) > 0 {
			cfg, cfgCtx = &f.Projects[0], ctx
		}
		return diags
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}

	var packs, models []string
	if cfg != nil {
		var err error
		if packs, models, err = l.config(cfg, cfgCtx); err != nil {
			return nil, nil, err
		}
	}
	p.ConfigInvalid = slices.ContainsFunc(l.diags, func(d *hcl.Diagnostic) bool {
		return d.Severity == hcl.DiagError && d.Subject != nil && d.Subject.Filename == ProjectFile
	})

	for _, step := range []struct {
		file   string
		role   string
		decode decodeFunc
	}{{"spec.hcl", RoleSpec, p.Spec.Decode}, {"style.hcl", RoleStyle, p.Styles.Decode}} {
		if err := l.parse(generic.Files, step.file, genericDir+"/"+step.file, step.decode); err != nil {
			return nil, nil, err
		}
		for _, dir := range append(packs, ".") {
			err := l.parseProject(path.Join(dir, step.file), step.role, step.decode)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, nil, err
			}
		}
	}

	if models == nil {
		if models, err = l.defaultModels(); err != nil {
			return nil, nil, err
		}
	}
	for _, rel := range models {
		if err := l.parseProject(rel, RoleModel, p.Model.Decode); err != nil {
			return nil, nil, err
		}
	}
	if len(models) == 0 {
		l.diags = append(l.diags, &hcl.Diagnostic{Severity: hcl.DiagWarning, Summary: "No model files", Detail: "The project has no model files."})
	}

	err = l.parseProject(p.ViewsFile, RoleViews, p.Views.Decode)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}
	if len(p.Views) == 0 {
		p.Views[view.DefaultView] = &view.View{ID: view.DefaultView, Include: []view.Selector{{Form: "all"}}}
	}
	if err := l.layouts(); err != nil {
		return nil, nil, err
	}

	l.diags = append(l.diags, p.Spec.Validate()...)
	l.diags = append(l.diags, p.Styles.Validate(p.Spec)...)
	l.diags = append(l.diags, p.Model.Validate(p.Spec)...)
	l.diags = append(l.diags, p.Views.Validate(p.Spec, p.Styles, p.Model)...)
	l.diags = append(l.diags, p.ValidateLayouts()...)
	diags := diag.FromHCL(l.diags)
	for _, o := range append(slices.Clone(p.Spec.Overrides), p.Styles.Overrides...) {
		verb := "replaces the one"
		if o.What == "style" || o.What == "edge_style" {
			verb = "overrides the properties it sets of the one"
		}
		detail := fmt.Sprintf("The %s %q %s defined at %s:%d:%d.", o.What, o.Name, verb, o.Prev.Filename, o.Prev.Start.Line, o.Prev.Start.Column)
		diags = append(diags, diag.At(diag.Info, o.Range, "Overridden definition", detail))
	}
	diags.Sort()
	return p, diags, nil
}

// config applies the project block, returning its pack directories and model files (nil if the
// block sets no models).
func (l *loader) config(cfg *projectBlock, ctx *hcl.EvalContext) (packs, models []string, err error) {
	p := l.p
	p.Name = cfg.Name
	list := func(attr *hcl.Attribute) []spec.Name {
		if attr == nil {
			return nil
		}
		names, d := spec.DecodeStrings(attr.Expr, ctx)
		l.diags = append(l.diags, d...)
		return names
	}
	relPath := func(n spec.Name) (string, bool) {
		clean := path.Clean(n.Name)
		if n.Name == "" || !fs.ValidPath(clean) {
			l.diags = append(l.diags, spec.Errorf(n.Range, "Invalid path", "%q is not a slash-separated path inside the project.", n.Name))
			return "", false
		}
		return clean, true
	}
	str := func(attr *hcl.Attribute, dst *string) {
		if attr == nil {
			return
		}
		var s string
		if d := spec.DecodeExpr(attr.Expr, ctx, &s); d.HasErrors() {
			l.diags = append(l.diags, d...)
			return
		}
		if clean, ok := relPath(spec.Name{Name: s, Range: attr.Expr.Range()}); ok {
			*dst = clean
		}
	}

	for _, n := range list(cfg.Packs) {
		if n.Name == "generic" {
			continue
		}
		dir, ok := relPath(n)
		switch {
		case !ok || slices.Contains(packs, dir):
		case dir == ".":
			l.diags = append(l.diags, spec.Errorf(n.Range, "Invalid pack", "The project root is not a pack; its spec.hcl and style.hcl load after every pack."))
		default:
			if info, err := fs.Stat(l.fsys, dir); err != nil || !info.IsDir() {
				l.diags = append(l.diags, spec.Errorf(n.Range, "Pack not found", "The pack directory %q does not exist.", n.Name))
				continue
			}
			packs = append(packs, dir)
		}
	}
	str(cfg.Views, &p.ViewsFile)
	str(cfg.LayoutDir, &p.LayoutDir)
	if cfg.Font != nil {
		str(cfg.Font, &p.Font)
		p.FontRange = cfg.Font.Expr.Range()
		if _, err := fs.Stat(l.fsys, p.Font); p.Font != "" && err != nil {
			l.diags = append(l.diags, spec.Errorf(p.FontRange, "File not found", "The font file %q does not exist.", p.Font))
		}
	}
	if cfg.Grid != nil {
		var grid int
		d := spec.DecodeExpr(cfg.Grid.Expr, ctx, &grid)
		l.diags = append(l.diags, d...)
		switch {
		case d.HasErrors():
		case grid < 1:
			l.diags = append(l.diags, spec.Errorf(cfg.Grid.Expr.Range(), "Invalid grid", "The grid must be a whole number of at least 1."))
		default:
			p.Grid = grid
		}
	}
	for _, n := range list(cfg.ReadOnly) {
		if _, err := path.Match(n.Name, ""); err != nil {
			l.diags = append(l.diags, spec.Errorf(n.Range, "Invalid pattern", "%q: %v.", n.Name, err))
			continue
		}
		p.ReadOnly = append(p.ReadOnly, n.Name)
	}
	if cfg.Models == nil {
		return packs, nil, nil
	}
	notModel := l.notModel(packs)
	models = []string{}
	for _, n := range list(cfg.Models) {
		matches, err := fs.Glob(l.fsys, n.Name)
		if err != nil {
			if errors.Is(err, path.ErrBadPattern) {
				l.diags = append(l.diags, spec.Errorf(n.Range, "Invalid pattern", "%q: %v.", n.Name, err))
				continue
			}
			return nil, nil, err
		}
		matches = slices.DeleteFunc(matches, notModel)
		if len(matches) == 0 {
			l.diags = append(l.diags, spec.Warnf(n.Range, "No matching files", "The pattern %q matches no model files.", n.Name))
		}
		for _, m := range matches {
			if !slices.Contains(models, m) {
				models = append(models, m)
			}
		}
	}
	return packs, models, nil
}

// notModel returns whether a project file is one that is never a model file: dg.hcl, the views
// file, and the spec.hcl and style.hcl of the project and its packs.
func (l *loader) notModel(packs []string) func(string) bool {
	return func(rel string) bool {
		if rel == ProjectFile || rel == l.p.ViewsFile {
			return true
		}
		dir, base := path.Split(rel)
		return (base == "spec.hcl" || base == "style.hcl") && (dir == "" || slices.Contains(packs, strings.TrimSuffix(dir, "/")))
	}
}

func (l *loader) defaultModels() ([]string, error) {
	names, err := fs.Glob(l.fsys, "*.hcl")
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(names, l.notModel(nil)), nil
}

func (l *loader) layouts() error {
	entries, err := fs.ReadDir(l.fsys, l.p.LayoutDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".layout.yaml")
		if !ok || e.IsDir() {
			continue
		}
		rel := path.Join(l.p.LayoutDir, e.Name())
		src, err := fs.ReadFile(l.fsys, rel)
		if err != nil {
			return err
		}
		layout, d := view.DecodeLayout(src, rel)
		l.diags = append(l.diags, d...)
		l.p.Layouts[id] = layout
		l.p.Files = append(l.p.Files, File{Path: rel, Role: RoleLayout})
	}
	return nil
}

// IsReadOnly reports whether the project file rel matches a read_only pattern.
func (p *Project) IsReadOnly(rel string) bool {
	return slices.ContainsFunc(p.ReadOnly, func(pattern string) bool {
		ok, _ := path.Match(pattern, rel)
		return ok
	})
}

// Resolve resolves view id; see view.Sources.Resolve.
func (p *Project) Resolve(id string) (*view.Resolved, diag.Diagnostics) {
	r, hdiags := p.Sources.Resolve(id)
	if p.Font != "" {
		hdiags = append(hdiags, spec.Errorf(p.FontRange, "Not supported yet", "A project font is drawn from M3."))
	}
	diags := diag.FromHCL(hdiags)
	diags.Sort()
	return r, diags
}
