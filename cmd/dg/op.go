package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/nvcnvn/dg/internal/ops"
)

const opUsage = `usage: dg op <operation> [-C dir] [flags] [--json]
       dg op batch [-C dir] [--file path] [--json]

Applies one operation to the project's files (spec FR-12) and writes every file it changes
together. An operation that would add a validation error is refused with that error and writes
nothing; so is one that would change a read_only file. dg op has no geometry: a move pins only
the node it names; read drawn positions with dg export --format json.

batch reads operations as JSON from --file or standard input: a list of operation objects such
as [{"op": "create_node", "kind": "service", "label": "API"}], applied in order as one change.
The fields of each object are the flags of its operation without the dashes; see
dg op <operation> --help.

--json prints {"results": [...], "files": [...], "warnings": [...]}, or {"error": ...,
"diagnostics": [...]} when the operation is refused. Exit code 0: applied; 1: refused or
invalid; 2: dg failed.

operations:
`

// opFlags are the flags of an operation's fields that are not named after the field.
var opFlags = map[string][]string{
	"from":   {"from", "from-port", "from-row"},
	"to":     {"to", "to-port", "to-row"},
	"labels": {"start-label", "label", "end-label"},
	"hint":   {"from-side", "to-side", "via", "label-pos"},
}

var opFlagHelp = map[string]string{
	"from":        "node the edge starts at",
	"from-port":   "port of the start node",
	"from-row":    "row of the start node",
	"to":          "node the edge ends at",
	"to-port":     "port of the end node",
	"to-row":      "row of the end node",
	"start-label": "label at the edge's start",
	"label":       "label at the edge's middle",
	"end-label":   "label at the edge's end",
	"from-side":   "side the edge leaves its start node by: top, right, bottom or left",
	"to-side":     "side the edge enters its end node by",
	"via":         "waypoint x,y in canvas coordinates (repeatable)",
	"label-pos":   "position of the middle label along the route, 0 to 1",
}

// applyOp applies an operation; tests replace it to record operations instead.
var applyOp = ops.Apply

var listFields = []string{"tags", "include", "exclude", "collapse", "edges", "unset", "order"}

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func opUsageText() string {
	var b strings.Builder
	b.WriteString(opUsage)
	for _, d := range ops.Defs {
		fmt.Fprintf(&b, "  %-16s %s\n", d.Name, d.Summary)
	}
	return b.String()
}

func runOp(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" {
		fmt.Fprint(stderr, opUsageText())
		if len(args) == 0 {
			return errors.New("op: name an operation")
		}
		return flag.ErrHelp
	}
	name := args[0]
	def, ok := ops.LookupDef(name)
	if !ok {
		return fmt.Errorf("op: unknown operation %q; operations are %s", name, strings.Join(ops.OpNames(), ", "))
	}
	fl := newOpFlagSet(def)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "project directory")
	asJSON := fl.Bool("json", false, "print the result as JSON")
	var file string
	values := map[string]any{}
	if name == "batch" {
		fl.StringVar(&file, "file", "", "file holding the operations as JSON (default standard input)")
	} else {
		defineOpFlags(fl, def, values)
	}
	fl.Usage = func() { opHelp(fl, def) }
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	if fl.NArg() > 0 {
		return fmt.Errorf("op %s: unexpected argument %q", name, fl.Arg(0))
	}
	var op ops.Op
	var err error
	if name == "batch" {
		op, err = readBatch(file)
	} else {
		op, err = opFromFlags(fl, def, values)
	}
	if err != nil {
		return reportOp(stdout, stderr, *asJSON, nil, err)
	}
	if _, err := os.Stat(*dir); err != nil {
		return internalError{err}
	}
	c, err := applyOp(*dir, op, ops.NewSession())
	return reportOp(stdout, stderr, *asJSON, c, err)
}

func newOpFlagSet(def ops.Def) *flag.FlagSet {
	fl := flag.NewFlagSet("op "+def.Name, flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	return fl
}

func readBatch(file string) (ops.Op, error) {
	var data []byte
	var err error
	if file == "" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		return ops.Op{}, internalError{err}
	}
	return ops.DecodeOps(data)
}

// defineOpFlags defines the flags of def's fields; values receives a getter per flag.
func defineOpFlags(fl *flag.FlagSet, def ops.Def, values map[string]any) {
	for _, f := range def.Fields {
		help := f.Help
		if f.Required {
			help = "(required) " + help
		}
		switch {
		case opFlags[f.Name] != nil:
			for _, name := range opFlags[f.Name] {
				h := opFlagHelp[name]
				if name == f.Name && f.Required {
					h = "(required) " + h
				}
				if name == "via" {
					values[name] = &listFlag{}
					fl.Var(values[name].(*listFlag), name, h)
				} else {
					values[name] = fl.String(name, "", h)
				}
			}
		case slices.Contains(listFields, f.Name):
			values[f.Name] = &listFlag{}
			fl.Var(values[f.Name].(*listFlag), f.Name, help+" (repeatable)")
		case f.Name == "recursive":
			values[f.Name] = fl.Bool(f.Name, false, help)
		case slices.Contains(jsonFields, f.Name):
			values[f.Name] = fl.String(f.Name, "", help+", as `JSON`")
		case f.Name == "offset":
			values[f.Name] = fl.String(f.Name, "", help+" (a `number`)")
		case slices.Contains(numberFields, f.Name):
			values[f.Name] = fl.String(f.Name, "", help+" (an `integer`)")
		default:
			values[f.Name] = fl.String(f.Name, "", help)
		}
	}
}

var (
	jsonFields   = []string{"props", "compartments", "overlays", "styles", "decoration"}
	numberFields = []string{"x", "y", "w", "h", "offset"}
)

// opFromFlags builds the operation's JSON object from the flags set and decodes it as dg op
// batch and the MCP op tool do.
func opFromFlags(fl *flag.FlagSet, def ops.Def, values map[string]any) (ops.Op, error) {
	set := map[string]bool{}
	fl.Visit(func(f *flag.Flag) { set[f.Name] = true })
	obj := map[string]any{"op": def.Name}
	str := func(f string) string { return *values[f].(*string) }
	for _, field := range def.Fields {
		name := field.Name
		switch {
		case name == "from" || name == "to":
			if !set[name] && !set[name+"-port"] && !set[name+"-row"] {
				continue
			}
			end := map[string]string{"node": str(name)}
			for _, part := range []string{"port", "row"} {
				if set[name+"-"+part] {
					end[part] = str(name + "-" + part)
				}
			}
			obj[name] = end
		case name == "labels":
			labels := map[string]string{}
			for f, key := range map[string]string{"start-label": "start", "label": "mid", "end-label": "end"} {
				if set[f] {
					labels[key] = str(f)
				}
			}
			if len(labels) > 0 {
				obj[name] = labels
			}
		case name == "hint":
			hint := map[string]any{}
			for _, f := range []string{"from-side", "to-side"} {
				if set[f] {
					hint[strings.ReplaceAll(f, "-", "_")] = str(f)
				}
			}
			if set["label-pos"] {
				v, err := strconv.ParseFloat(str("label-pos"), 64)
				if err != nil {
					return ops.Op{}, fmt.Errorf("--label-pos: %q is not a number", str("label-pos"))
				}
				hint["label_pos"] = v
			}
			var via [][2]int
			for _, pt := range *values["via"].(*listFlag) {
				xs, ys, ok := strings.Cut(pt, ",")
				x, err1 := strconv.Atoi(strings.TrimSpace(xs))
				y, err2 := strconv.Atoi(strings.TrimSpace(ys))
				if !ok || err1 != nil || err2 != nil {
					return ops.Op{}, fmt.Errorf("--via: %q is not x,y", pt)
				}
				via = append(via, [2]int{x, y})
			}
			if via != nil {
				hint["via"] = via
			}
			obj[name] = hint
		case slices.Contains(listFields, name):
			if set[name] || field.Required {
				obj[name] = []string(*values[name].(*listFlag))
			}
		case name == "recursive":
			if set[name] {
				obj[name] = *values[name].(*bool)
			}
		case !set[name]:
		case slices.Contains(numberFields, name):
			if _, err := strconv.ParseFloat(str(name), 64); err != nil {
				return ops.Op{}, fmt.Errorf("--%s: %q is not a number", name, str(name))
			}
			obj[name] = json.RawMessage(str(name))
		case slices.Contains(jsonFields, name):
			if !json.Valid([]byte(str(name))) {
				return ops.Op{}, fmt.Errorf("--%s: %q is not valid JSON", name, str(name))
			}
			obj[name] = json.RawMessage(str(name))
		default:
			obj[name] = str(name)
		}
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return ops.Op{}, fmt.Errorf("op %s: %w", def.Name, err)
	}
	return ops.DecodeOp(data)
}

func opHelp(fl *flag.FlagSet, def ops.Def) {
	w := fl.Output()
	fmt.Fprintf(w, "usage: dg op %s [-C dir] [--json]", def.Name)
	if def.Name == "batch" {
		fmt.Fprint(w, " [--file path]")
	}
	fmt.Fprintf(w, "\n\n%s\n\nflags:\n", def.Summary)
	fl.PrintDefaults()
	fmt.Fprintf(w, "\nas JSON (dg op batch, the MCP op tool), with the required fields:\n  %s\n", def.Example())
	if def.Name != "batch" {
		fmt.Fprintf(w, "fields: %s\n", def.Signature())
	}
}

// reportOp prints the result of an operation, or why it failed.
func reportOp(stdout, stderr io.Writer, asJSON bool, c *ops.Change, err error) error {
	var internal internalError
	if errors.As(err, &internal) {
		return err
	}
	if asJSON {
		data, merr := json.MarshalIndent(ops.NewOutcome(c, err), "", "  ")
		if merr != nil {
			return internalError{merr}
		}
		if _, werr := stdout.Write(append(data, '\n')); werr != nil {
			return internalError{werr}
		}
		if err != nil {
			return errDiagnostics
		}
		return nil
	}
	if err != nil {
		return err
	}
	for _, r := range c.Results {
		line := r.Op
		if r.ID != "" {
			line += " " + r.ID
		}
		for _, v := range []struct {
			name string
			v    *int
		}{{"x", r.X}, {"y", r.Y}, {"w", r.W}, {"h", r.H}} {
			if v.v != nil {
				line += fmt.Sprintf(" %s=%d", v.name, *v.v)
			}
		}
		fmt.Fprintln(stdout, line)
	}
	for _, d := range c.Warnings {
		fmt.Fprintln(stderr, d)
	}
	for _, p := range c.Paths() {
		fmt.Fprintln(stdout, "wrote "+p)
	}
	return nil
}
