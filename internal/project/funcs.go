package project

import (
	"fmt"
	"io/fs"
	"path"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/gocty"
)

// maxPortRange bounds port_range so a typo cannot allocate millions of ports; maxPortNumber
// bounds from and to so that to-from cannot overflow.
const (
	maxPortRange  = 10000
	maxPortNumber = 1_000_000_000
)

// evalContext returns the functions available to a file in directory dir of fsys, which scope
// names.
func evalContext(fsys fs.FS, dir, scope string) *hcl.EvalContext {
	return &hcl.EvalContext{Functions: map[string]function.Function{
		"port_range": portRange,
		"file":       fileFunc(fsys, dir, scope),
	}}
}

var portRange = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "prefix", Type: cty.String},
		{Name: "from", Type: cty.Number},
		{Name: "to", Type: cty.Number},
	},
	Type: function.StaticReturnType(cty.List(cty.String)),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		var from, to int64
		for i, dst := range []*int64{&from, &to} {
			if err := gocty.FromCtyValue(args[i+1], dst); err != nil || *dst < -maxPortNumber || *dst > maxPortNumber {
				return cty.NilVal, function.NewArgErrorf(i+1, "must be a whole number from %d to %d", -maxPortNumber, maxPortNumber)
			}
		}
		if to < from || to-from >= maxPortRange {
			return cty.NilVal, fmt.Errorf("to must be at least from and at most %d more", maxPortRange-1)
		}
		prefix := args[0].AsString()
		ports := make([]cty.Value, 0, to-from+1)
		for i := from; i <= to; i++ {
			ports = append(ports, cty.StringVal(fmt.Sprintf("%s%d", prefix, i)))
		}
		return cty.ListVal(ports), nil
	},
})

func fileFunc(fsys fs.FS, dir, scope string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "path", Type: cty.String}},
		Type:   function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			rel := args[0].AsString()
			full := path.Join(dir, rel)
			if path.IsAbs(rel) || !fs.ValidPath(full) {
				return cty.NilVal, fmt.Errorf("%q is not a relative path inside the %s", rel, scope)
			}
			data, err := fs.ReadFile(fsys, full)
			if err != nil {
				return cty.NilVal, fmt.Errorf("cannot read %q", rel)
			}
			if !utf8.Valid(data) {
				return cty.NilVal, fmt.Errorf("%q is not UTF-8 text", rel)
			}
			return cty.StringVal(string(data)), nil
		},
	})
}
