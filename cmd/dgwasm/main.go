//go:build js

// Command dgwasm is the playground's Go half, dg.wasm (FR-19): it carries internal/playground's
// messages through two JavaScript functions, dgToGo, which it defines, and dgToPage, which the page
// defines before starting it.
package main

import (
	"context"
	"syscall/js"

	"github.com/nvcnvn/dg/internal/playground"
)

// backlog is how many messages can wait; a page sends far fewer between two it has answered.
const backlog = 1024

func main() {
	toPage := js.Global().Get("dgToPage")
	p := playground.New(func(data []byte) { toPage.Invoke(string(data)) })
	in := make(chan string, backlog)
	js.Global().Set("dgToGo", js.FuncOf(func(_ js.Value, args []js.Value) any {
		in <- args[0].String()
		return nil
	}))
	p.Ready()
	ctx := context.Background()
	for m := range in {
		p.Receive(ctx, []byte(m))
	}
}
