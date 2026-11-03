package main

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestServeStopsOnStdinEOF checks that dg serve -stdin stops when its standard input closes, so
// an editor that dies does not leave it running.
func TestServeStopsOnStdinEOF(t *testing.T) {
	in, closeIn := io.Pipe()
	stdin = in
	t.Cleanup(func() { stdin = os.Stdin })
	out, w := io.Pipe()
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"serve", "-C", "../../examples/hello", "-open=false", "-stdin"}, w, io.Discard)
		w.Close()
	}()
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "dg serve: http://127.0.0.1:") {
		t.Fatalf("dg serve printed %q, %v", line, err)
	}
	go func() { _, _ = io.Copy(io.Discard, out) }()
	closeIn.Close()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("dg serve kept running after standard input closed")
	}
}
