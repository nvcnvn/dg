package main

import (
	"context"
	"flag"
	"io"
	"os"
	"os/signal"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nvcnvn/dg/internal/mcp"
)

func runMCP(args []string, _, stderr io.Writer) error {
	dir, err := flags("mcp", args, stderr, func(*flag.FlagSet) {})
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		return internalError{err}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := mcp.New(dir).Run(ctx, &sdk.StdioTransport{}); err != nil && ctx.Err() == nil {
		return internalError{err}
	}
	return nil
}
