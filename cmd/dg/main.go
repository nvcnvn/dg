// Command dg is the diagram-as-code tool.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/export"
	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

const usage = `usage: dg <command> [flags]

commands:
  check    validate the project
  fmt      format the project's files
  export   draw a view as SVG or JSON geometry
`

// errDiagnostics marks a failure already reported as diagnostics.
var errDiagnostics = errors.New("project has errors")

// internalError is a failure of dg itself rather than of its input; it exits with code 2.
type internalError struct{ error }

func (e internalError) Unwrap() error { return e.error }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	var err error
	switch args[0] {
	case "check":
		err = runCheck(args[1:], stdout, stderr)
	case "fmt":
		err = runFmt(args[1:], stdout, stderr)
	case "export":
		err = runExport(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "dg: unknown command %q\n%s", args[0], usage)
		return 1
	}
	var internal internalError
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errDiagnostics):
		return 1
	case errors.As(err, &internal):
		fmt.Fprintf(stderr, "dg: %v\n", err)
		return 2
	default:
		fmt.Fprintf(stderr, "dg: %v\n", err)
		return 1
	}
}

func flags(name string, args []string, stderr io.Writer, define func(*flag.FlagSet)) (dir string, err error) {
	fl := flag.NewFlagSet(name, flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.StringVar(&dir, "C", ".", "project directory")
	define(fl)
	if err := fl.Parse(args); err != nil {
		return "", err
	}
	if fl.NArg() > 0 {
		return "", fmt.Errorf("%s: unexpected argument %q", name, fl.Arg(0))
	}
	return dir, nil
}

func load(dir string) (*project.Project, diag.Diagnostics, error) {
	p, diags, err := project.Load(dir)
	if err != nil {
		return nil, nil, internalError{err}
	}
	return p, diags, nil
}

func runCheck(args []string, stdout, stderr io.Writer) error {
	var strict, asJSON bool
	dir, err := flags("check", args, stderr, func(fl *flag.FlagSet) {
		fl.BoolVar(&strict, "strict", false, "treat warnings as errors")
		fl.BoolVar(&asJSON, "json", false, "print diagnostics as JSON")
	})
	if err != nil {
		return err
	}
	_, diags, err := load(dir)
	if err != nil {
		return err
	}
	if asJSON {
		if diags == nil {
			diags = diag.Diagnostics{}
		}
		var data []byte
		if data, err = json.MarshalIndent(diags, "", "  "); err == nil {
			_, err = stdout.Write(append(data, '\n'))
		}
	} else {
		for _, d := range diags {
			if _, err = fmt.Fprintln(stdout, d); err != nil {
				break
			}
		}
	}
	if err != nil {
		return internalError{err}
	}
	if diags.HasErrors() || strict && diags.Has(diag.Warning) {
		return errDiagnostics
	}
	return nil
}

func runFmt(args []string, stdout, stderr io.Writer) error {
	var check bool
	dir, err := flags("fmt", args, stderr, func(fl *flag.FlagSet) {
		fl.BoolVar(&check, "check", false, "list the files that would change and write nothing")
	})
	if err != nil {
		return err
	}
	p, loadDiags, err := load(dir)
	if err != nil {
		return err
	}
	changed, diags, err := ops.Format(p, check)
	if errors.Is(err, ops.ErrConfigInvalid) {
		report(stderr, loadDiags)
		return err
	}
	if err != nil {
		return internalError{err}
	}
	for _, d := range diags {
		fmt.Fprintln(stderr, d)
	}
	for _, path := range changed {
		fmt.Fprintln(stdout, path)
	}
	if diags.HasErrors() || check && len(changed) > 0 {
		return errDiagnostics
	}
	return nil
}

func runExport(args []string, stdout, stderr io.Writer) error {
	var viewID, format, out string
	dir, err := flags("export", args, stderr, func(fl *flag.FlagSet) {
		fl.StringVar(&viewID, "view", view.DefaultView, "view to export")
		fl.StringVar(&format, "format", "svg", "output format: svg or json")
		fl.StringVar(&out, "out", "", "output file (default standard output)")
	})
	if err != nil {
		return err
	}
	if format != "svg" && format != "json" {
		return fmt.Errorf("export: unknown format %q (want svg or json)", format)
	}

	p, diags, err := load(dir)
	if err != nil {
		return err
	}
	if err := report(stderr, diags); err != nil {
		return err
	}
	if p.Views[viewID] == nil {
		return fmt.Errorf("export: view %q is not defined (views: %s)", viewID, strings.Join(slices.Sorted(maps.Keys(p.Views)), ", "))
	}
	v, diags := p.Resolve(viewID)
	if err := report(stderr, diags); err != nil {
		return err
	}
	res, err := export.Render(context.Background(), v)
	if err != nil {
		return internalError{err}
	}
	data := res.SVG
	if format == "json" {
		data = res.JSON
	}
	if out == "" {
		_, err = stdout.Write(data)
		return err
	}
	return ops.WriteFile(out, data)
}

// report prints errors and warnings, and returns errDiagnostics if there are errors.
func report(stderr io.Writer, diags diag.Diagnostics) error {
	for _, d := range diags {
		if d.Severity != diag.Info {
			fmt.Fprintln(stderr, d)
		}
	}
	if diags.HasErrors() {
		return errDiagnostics
	}
	return nil
}
