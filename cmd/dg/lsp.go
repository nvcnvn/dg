package main

import (
	"flag"
	"io"
	"os"

	"github.com/nvcnvn/dg/internal/lsp"
)

func runLSP(args []string, _, stderr io.Writer) error {
	dir, err := flags("lsp", args, stderr, func(*flag.FlagSet) {})
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		return internalError{err}
	}
	s, err := lsp.New(dir)
	if err != nil {
		return internalError{err}
	}
	return s.Serve(os.Stdin, os.Stdout)
}
