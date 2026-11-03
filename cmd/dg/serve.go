package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/nvcnvn/dg/internal/server"
)

func runServe(args []string, stdout, stderr io.Writer) error {
	var addr string
	var open bool
	dir, err := flags("serve", args, stderr, func(fl *flag.FlagSet) {
		fl.StringVar(&addr, "addr", "127.0.0.1:0", "address to listen on")
		fl.BoolVar(&open, "open", true, "open the editor in the browser")
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		return internalError{err}
	}
	if !server.IsLoopback(addr) {
		return fmt.Errorf("serve: -addr %q is not a loopback address; dg serve has no authentication", addr)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return internalError{err}
	}
	url := "http://" + ln.Addr().String() + "/"
	fmt.Fprintf(stdout, "dg serve: %s\n", url)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s := server.New(dir)
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- s.Watch(ctx, func(err error) { fmt.Fprintf(stderr, "dg serve: watching files: %v\n", err) })
	}()
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := <-watchErr; err != nil {
			fmt.Fprintf(stderr, "dg serve: cannot watch the project, so changes are not shown: %v\n", err)
		}
		<-ctx.Done()
		_ = srv.Close()
	}()
	if open {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(stderr, "dg serve: cannot open the browser: %v\n", err)
		}
	}
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return internalError{err}
	}
	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
