package ops

import (
	"bytes"
	"errors"
	"io/fs"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/fsys"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

// ErrConfigInvalid is returned by Format for a project whose dg.hcl has errors, since its
// read_only patterns may be incomplete.
var ErrConfigInvalid = errors.New("dg.hcl has errors; nothing is formatted, since read_only may be incomplete")

// Format puts the writable files of p, loaded from files, in canonical form (FR-3): HCL files
// with hclwrite.Format and layout files regenerated. Files that do not parse are left alone and
// reported. It returns the project-relative paths of the files that change; with check it writes
// nothing.
func Format(files fsys.FS, p *project.Project, check bool) ([]string, diag.Diagnostics, error) {
	if p.ConfigInvalid {
		return nil, nil, ErrConfigInvalid
	}
	var hdiags []*hcl.Diagnostic
	var changed []string
	out := map[string][]byte{}
	for _, f := range p.Files {
		if p.IsReadOnly(f.Path) {
			continue
		}
		src, err := fs.ReadFile(files, f.Path)
		if err != nil {
			return nil, nil, err
		}
		formatted, d := FormatFile(f, src)
		hdiags = append(hdiags, d...)
		if formatted == nil || bytes.Equal(formatted, src) {
			continue
		}
		changed = append(changed, f.Path)
		out[f.Path] = formatted
	}
	diags := diag.FromHCL(hdiags)
	diags.Sort()
	if check {
		return changed, diags, nil
	}
	return changed, diags, files.Write(out)
}

// FormatFile returns src, the content of project file f, in canonical form (FR-3): an HCL file
// formatted with hclwrite.Format, a layout file regenerated. It returns nil, with the errors,
// for a file that does not parse, and warnings of a layout file that does.
func FormatFile(f project.File, src []byte) ([]byte, hcl.Diagnostics) {
	if f.Role == project.RoleLayout {
		l, d := view.DecodeLayout(src, f.Path)
		if d.HasErrors() {
			return nil, d
		}
		return l.Encode(), d
	}
	if _, d := hclsyntax.ParseConfig(src, f.Path, hcl.InitialPos); d.HasErrors() {
		return nil, d
	}
	return hclwrite.Format(src), nil
}
