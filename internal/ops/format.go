package ops

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

// ErrConfigInvalid is returned by Format for a project whose dg.hcl has errors, since its
// read_only patterns may be incomplete.
var ErrConfigInvalid = errors.New("dg.hcl has errors; nothing is formatted, since read_only may be incomplete")

// Format puts p's writable files in canonical form (FR-3): HCL files with hclwrite.Format and
// layout files regenerated. Files that do not parse are left alone and reported. It returns the
// project-relative paths of the files that change; with check it writes nothing.
func Format(p *project.Project, check bool) ([]string, diag.Diagnostics, error) {
	if p.ConfigInvalid {
		return nil, nil, ErrConfigInvalid
	}
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	realRoot, err := filepath.EvalSymlinks(p.Dir)
	if err != nil {
		return nil, nil, err
	}
	var hdiags []*hcl.Diagnostic
	var changed []string
	out := map[string][]byte{}
	for _, f := range p.Files {
		if p.IsReadOnly(f.Path) {
			continue
		}
		src, err := root.ReadFile(f.Path)
		if err != nil {
			return nil, nil, err
		}
		var formatted []byte
		if f.Role == project.RoleLayout {
			l, d := view.DecodeLayout(src, f.Path)
			hdiags = append(hdiags, d...)
			if d.HasErrors() {
				continue
			}
			formatted = l.Encode()
		} else {
			_, d := hclsyntax.ParseConfig(src, f.Path, hcl.InitialPos)
			if d.HasErrors() {
				hdiags = append(hdiags, d...)
				continue
			}
			formatted = hclwrite.Format(src)
		}
		if bytes.Equal(formatted, src) {
			continue
		}
		path := filepath.Join(p.Dir, filepath.FromSlash(f.Path))
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, nil, err
		}
		if rel, err := filepath.Rel(realRoot, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, nil, fmt.Errorf("%s links outside the project", f.Path)
		}
		changed = append(changed, f.Path)
		out[path] = formatted
	}
	diags := diag.FromHCL(hdiags)
	diags.Sort()
	if check {
		return changed, diags, nil
	}
	return changed, diags, WriteFiles(out)
}
