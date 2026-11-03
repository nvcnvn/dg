// Package project discovers and loads a project's files.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/nvcnvn/dg/internal/model"
	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/style"
	"github.com/nvcnvn/dg/packs/generic"
)

// Grid is the project grid in pixels.
const Grid = 10

// Project is a loaded project.
type Project struct {
	Dir    string
	Spec   *spec.Spec
	Styles *style.Styles
	Model  *model.Model
}

var notModel = map[string]bool{"dg.hcl": true, "spec.hcl": true, "style.hcl": true, "views.hcl": true}

var unsupported = []string{"dg.hcl", "views.hcl"}

// Load reads the project in dir; problems in file content are diagnostics, unreadable files errors.
func Load(dir string) (*Project, hcl.Diagnostics, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory", dir)
	}
	return load(os.DirFS(dir), dir)
}

func load(fsys fs.FS, dir string) (*Project, hcl.Diagnostics, error) {
	p := &Project{Dir: dir, Spec: spec.New(), Styles: &style.Styles{}, Model: model.New()}
	var diags hcl.Diagnostics

	parse := func(fsys fs.FS, name, display string, decode func(hcl.Body) hcl.Diagnostics) error {
		src, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		f, d := hclsyntax.ParseConfig(src, display, hcl.InitialPos)
		diags = append(diags, d...)
		if !d.HasErrors() {
			diags = append(diags, decode(f.Body)...)
		}
		return nil
	}

	for _, step := range []struct {
		name   string
		decode func(hcl.Body) hcl.Diagnostics
	}{{"spec.hcl", p.Spec.Decode}, {"style.hcl", p.Styles.Decode}} {
		if err := parse(generic.Files, step.name, "generic/"+step.name, step.decode); err != nil {
			return nil, nil, err
		}
		err := parse(fsys, step.name, filepath.Join(dir, step.name), step.decode)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
	}

	names, err := fs.Glob(fsys, "*.hcl")
	if err != nil {
		return nil, nil, err
	}
	models := 0
	for _, base := range names {
		path := filepath.Join(dir, base)
		if slices.Contains(unsupported, base) {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unsupported file",
				Detail:   fmt.Sprintf("%s is not supported yet.", base),
				Subject:  &hcl.Range{Filename: path, Start: hcl.InitialPos, End: hcl.InitialPos},
			})
		}
		if notModel[base] {
			continue
		}
		if err := parse(fsys, base, path, p.Model.Decode); err != nil {
			return nil, nil, err
		}
		models++
	}
	if models == 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "No model files",
			Detail:   fmt.Sprintf("%s has no model files (*.hcl other than dg.hcl, spec.hcl, style.hcl and views.hcl).", dir),
		})
	}

	diags = append(diags, p.Spec.Validate()...)
	diags = append(diags, p.Styles.Validate(p.Spec)...)
	diags = append(diags, p.Model.Validate(p.Spec)...)
	return p, diags, nil
}

// FormatDiagnostic formats d as "file:line:column: severity: summary: detail".
func FormatDiagnostic(d *hcl.Diagnostic) string {
	severity := "error"
	if d.Severity == hcl.DiagWarning {
		severity = "warning"
	}
	msg := d.Summary
	if d.Detail != "" {
		msg += ": " + d.Detail
	}
	if d.Subject == nil {
		return severity + ": " + msg
	}
	return fmt.Sprintf("%s:%d:%d: %s: %s", d.Subject.Filename, d.Subject.Start.Line, d.Subject.Start.Column, severity, msg)
}
