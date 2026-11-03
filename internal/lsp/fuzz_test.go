package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fuzzProject = repo + "/testdata/lsp/buffers"

// fuzzFiles are the files a fuzzed buffer edits: model, views, spec, style, layout and
// project files.
var fuzzFiles = []string{"model.hcl", "views.hcl", "spec.hcl", "style.hcl", "views/main.layout.yaml", "dg.hcl"}

// fuzzServer returns a server for the fuzz project, initialized, with file open as text and its
// diagnostics published.
func fuzzServer(t testing.TB, file, text string) *Server {
	s, err := New(fuzzProject)
	if err != nil {
		t.Fatal(err)
	}
	s.out = io.Discard
	open, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{
		"textDocument": map[string]any{"uri": s.uri(file), "languageId": "hcl", "version": 1, "text": text},
	}})
	for _, m := range [][]byte{[]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`), []byte(`{"jsonrpc":"2.0","method":"initialized"}`), open} {
		if _, _, err := s.handle(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.publish(); err != nil {
		t.Fatal(err)
	}
	return s
}

// FuzzMessage sends the server one message of any content, and reads messages from it as a
// stream.
func FuzzMessage(f *testing.F) {
	files, err := filepath.Glob(filepath.Join(repo, "testdata/lsp/*.jsonl"))
	if err != nil {
		f.Fatal(err)
	}
	s, err := New(fuzzProject)
	if err != nil {
		f.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			f.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			if line != "" && !strings.HasPrefix(line, "//") {
				f.Add([]byte(strings.ReplaceAll(strings.ReplaceAll(line, "$ROOT", s.uri(".")), `"$FILE"`, `"node \"x\" {}"`)))
			}
		}
	}
	f.Add([]byte("Content-Length: 2\r\n\r\n{}"))
	model, err := os.ReadFile(filepath.Join(fuzzProject, "model.hcl"))
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		s := fuzzServer(t, "model.hcl", string(model))
		_, _, _ = s.handle(body)
		r := bufio.NewReader(bytes.NewReader(body))
		for {
			if _, err := readMessage(r); err != nil {
				break
			}
		}
	})
}

// FuzzBuffer opens a buffer of any content and asks for every feature at a position in it.
func FuzzBuffer(f *testing.F) {
	model, err := os.ReadFile(filepath.Join(fuzzProject, "model.hcl"))
	if err != nil {
		f.Fatal(err)
	}
	views, err := os.ReadFile(filepath.Join(fuzzProject, "views.hcl"))
	if err != nil {
		f.Fatal(err)
	}
	layout, err := os.ReadFile(filepath.Join(fuzzProject, "views/main.layout.yaml"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(uint8(0), string(model), uint16(9), uint16(14))
	f.Add(uint8(0), string(model), uint16(1), uint16(10))
	f.Add(uint8(0), "node \"a\" {\n  parent = node.\n}\nedge \"e\" { from = node[\"a\"].port.", uint16(3), uint16(32))
	f.Add(uint8(1), string(views), uint16(2), uint16(18))
	f.Add(uint8(1), "view \"v\" {\n  style \"node.", uint16(1), uint16(14))
	f.Add(uint8(2), "kind \"k\" {\n  extends = \"", uint16(1), uint16(13))
	f.Add(uint8(3), "style \"service\" {\n  shape = \"", uint16(1), uint16(11))
	f.Add(uint8(4), string(layout), uint16(2), uint16(3))
	f.Add(uint8(5), "project \"p\" {\n  read_only = [\"*.hcl\"]\n}\n", uint16(1), uint16(4))
	f.Fuzz(func(t *testing.T, which uint8, text string, line, char uint16) {
		file := fuzzFiles[int(which)%len(fuzzFiles)]
		s := fuzzServer(t, file, text)
		uri := s.uri(file)
		pos := fmt.Sprintf(`"textDocument": {"uri": %q}, "position": {"line": %d, "character": %d}`, uri, line, char)
		for _, m := range []string{
			`"method": "textDocument/completion", "params": {` + pos + `}`,
			`"method": "textDocument/hover", "params": {` + pos + `}`,
			`"method": "textDocument/definition", "params": {` + pos + `}`,
			`"method": "textDocument/references", "params": {` + pos + `, "context": {"includeDeclaration": true}}`,
			`"method": "textDocument/prepareRename", "params": {` + pos + `}`,
			`"method": "textDocument/rename", "params": {` + pos + `, "newName": "renamed"}`,
			`"method": "textDocument/formatting", "params": {"textDocument": {"uri": "` + uri + `"}}`,
		} {
			if _, _, err := s.handle([]byte(`{"jsonrpc": "2.0", "id": 2, ` + m + `}`)); err != nil {
				t.Fatal(err)
			}
		}
	})
}
