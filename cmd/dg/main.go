// Command dg is the diagram-as-code tool.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/nvcnvn/dg/internal/export"
	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

const usage = `usage: dg <command> [flags]

commands:
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

func runExport(args []string, stdout, stderr io.Writer) error {
	fl := flag.NewFlagSet("export", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "project directory")
	viewID := fl.String("view", view.DefaultView, "view to export")
	format := fl.String("format", "svg", "output format: svg or json")
	out := fl.String("out", "", "output file (default standard output)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if fl.NArg() > 0 {
		return fmt.Errorf("export: unexpected argument %q", fl.Arg(0))
	}
	if *format != "svg" && *format != "json" {
		return fmt.Errorf("export: unknown format %q (want svg or json)", *format)
	}

	p, err := load(*dir, stderr)
	if err != nil {
		return err
	}
	v, err := view.Resolve(p, *viewID)
	if err != nil {
		return err
	}
	res, err := export.Render(context.Background(), v)
	if err != nil {
		return internalError{err}
	}
	data := res.SVG
	if *format == "json" {
		data = res.JSON
	}
	if *out == "" {
		_, err = stdout.Write(data)
		return err
	}
	return ops.WriteFile(*out, data)
}

func load(dir string, stderr io.Writer) (*project.Project, error) {
	p, diags, err := project.Load(dir)
	if err != nil {
		return nil, err
	}
	for _, d := range diags {
		fmt.Fprintln(stderr, project.FormatDiagnostic(d))
	}
	if diags.HasErrors() {
		return nil, errDiagnostics
	}
	return p, nil
}
