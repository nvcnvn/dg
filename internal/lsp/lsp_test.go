package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/project"
)

var update = flag.Bool("update", false, "rewrite golden files")

const repo = "../.."

// session runs the client messages in lines against the project in dir and returns what the
// server sent. In the messages, $ROOT is the project's URI and a document text "$FILE" is the
// content of the document's file.
func session(t testing.TB, dir string, lines []string) []json.RawMessage {
	t.Helper()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := s.uri(".")
	var in bytes.Buffer
	for _, line := range lines {
		line = strings.ReplaceAll(line, "$ROOT", root)
		if strings.Contains(line, `"$FILE"`) {
			var m struct {
				Params didOpenParams `json:"params"`
			}
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatal(err)
			}
			rel, _ := s.rel(m.Params.TextDocument.URI)
			data, err := os.ReadFile(filepath.Join(dir, rel))
			if err != nil {
				t.Fatal(err)
			}
			text, _ := json.Marshal(string(data))
			line = strings.Replace(line, `"$FILE"`, string(text), 1)
		}
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(line), line)
	}
	var out bytes.Buffer
	if err := s.Serve(&in, &out); err != nil {
		t.Fatal(err)
	}
	var msgs []json.RawMessage
	r := bufio.NewReader(&out)
	for {
		body, err := readMessage(r)
		if errors.Is(err, io.EOF) {
			return msgs
		}
		if err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, bytes.ReplaceAll(body, []byte(root), []byte("$ROOT")))
	}
}

// TestSessions replays the recorded sessions in testdata/lsp: each .jsonl file names its
// project on its first line and holds client messages, one per line ("//" lines are comments).
// What the server sends is compared with the session's .golden file.
func TestSessions(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(repo, "testdata/lsp/*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			var head struct{ Project string }
			if err := json.Unmarshal([]byte(lines[0]), &head); err != nil || head.Project == "" {
				t.Fatalf("the first line names the project: %v", err)
			}
			var msgs []string
			for _, l := range lines[1:] {
				if l != "" && !strings.HasPrefix(l, "//") {
					msgs = append(msgs, l)
				}
			}
			var got bytes.Buffer
			for _, m := range session(t, filepath.Join(repo, head.Project), msgs) {
				if err := json.Indent(&got, m, "", "  "); err != nil {
					t.Fatal(err)
				}
				got.WriteString("\n")
			}
			golden(t, strings.TrimSuffix(file, ".jsonl")+".golden", got.Bytes())
		})
	}
}

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the output:\n%s", path, got)
	}
}

// TestDiagnosticsMatchCheck checks that the server publishes, for every project under
// testdata/check and for testdata/lsp/unicode, the diagnostics dg check reports, at the same
// positions, and those without a file at the start of dg.hcl. Positions are converted here
// with utf16.Encode; on a line that is ASCII up to a position, its column is dg check's too.
func TestDiagnosticsMatchCheck(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join(repo, "testdata/check/*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range append(dirs, filepath.Join(repo, "testdata/lsp/unicode")) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			_, diags, err := project.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			position := func(file string, line, column, off int) Position {
				data, _ := os.ReadFile(filepath.Join(dir, file)) // empty for a directory named as a file
				text := string(data[:off])
				start := strings.LastIndexByte(text, '\n') + 1
				p := Position{strings.Count(text, "\n"), len(utf16.Encode([]rune(text[start:])))}
				if p.Line != line-1 || isASCII(text[start:]) && p.Character != column-1 {
					t.Errorf("%s: byte %d is at %v, not at line %d, column %d", file, off, p, line, column)
				}
				return p
			}
			var want []string
			for _, d := range diags {
				file, rng := d.File, Range{}
				if file == "" || strings.HasPrefix(file, "<") {
					file = project.ProjectFile
				} else {
					rng = Range{position(file, d.Line, d.Column, d.Byte), position(file, d.EndLine, d.EndColumn, d.EndByte)}
				}
				msg := d.Summary
				if d.Detail != "" {
					msg += ": " + d.Detail
				}
				sev := map[diag.Severity]int{diag.Error: 1, diag.Warning: 2, diag.Info: 3}[d.Severity]
				want = append(want, fmt.Sprintf("%s %v %d %s", file, rng, sev, msg))
			}
			var got []string
			msgs := session(t, dir, []string{
				`{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}}`,
				`{"jsonrpc": "2.0", "method": "initialized", "params": {}}`,
			})
			for _, m := range msgs[1:] {
				var n struct {
					Method string
					Params publishDiagnosticsParams
				}
				if err := json.Unmarshal(m, &n); err != nil || n.Method != "textDocument/publishDiagnostics" {
					t.Fatalf("unexpected message %s", m)
				}
				for _, d := range n.Params.Diagnostics {
					got = append(got, fmt.Sprintf("%s %v %d %s", strings.TrimPrefix(n.Params.URI, "$ROOT/"), d.Range, d.Severity, d.Message))
				}
			}
			if strings.Join(got, "\n") != strings.Join(sortedByFile(want), "\n") {
				t.Errorf("published:\n%s\ndg check:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// sortedByFile orders diagnostics as the server publishes them, by file and then as dg check
// lists them.
func sortedByFile(ds []string) []string {
	byFile := map[string][]string{}
	var files []string
	for _, d := range ds {
		f, _, _ := strings.Cut(d, " ")
		if byFile[f] == nil {
			files = append(files, f)
		}
		byFile[f] = append(byFile[f], d)
	}
	var out []string
	for _, f := range slices.Sorted(slices.Values(files)) {
		out = append(out, byFile[f]...)
	}
	return out
}

func TestLines(t *testing.T) {
	text := "a = \"é😀x\"\r\nb\n"
	ls := newLines(text)
	tests := []struct {
		off  int
		want Position
	}{
		{0, Position{0, 0}},
		{5, Position{0, 5}},
		{7, Position{0, 6}},
		{11, Position{0, 8}},
		{13, Position{0, 10}},
		{15, Position{1, 0}},
		{16, Position{1, 1}},
		{17, Position{2, 0}},
		{99, Position{2, 0}},
	}
	for _, tt := range tests {
		got := ls.position(tt.off)
		if got != tt.want {
			t.Errorf("position(%d) = %v, want %v", tt.off, got, tt.want)
		}
		if off := ls.offset(got); off != min(tt.off, len(text)) {
			t.Errorf("offset(%v) = %d, want %d", got, off, tt.off)
		}
	}
}

// TestBurstPublishesOnce checks that changes waiting to be read are taken together: fifty
// keystrokes, each changing the diagnostics, then a completion, publish once after the answer.
func TestBurstPublishesOnce(t *testing.T) {
	dir := filepath.Join(repo, "testdata/lsp/buffers")
	lines := []string{
		`{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}}`,
		`{"jsonrpc": "2.0", "method": "initialized", "params": {}}`,
		`{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": {"textDocument": {"uri": "$ROOT/model.hcl", "version": 1, "text": "$FILE"}}}`,
	}
	for i := range 50 {
		lines = append(lines, fmt.Sprintf(`{"jsonrpc": "2.0", "method": "textDocument/didChange", "params": {"textDocument": {"uri": "$ROOT/model.hcl", "version": %d}, "contentChanges": [{"range": {"start": {"line": 10, "character": %d}, "end": {"line": 10, "character": %d}}, "text": "x"}]}}`, i+2, 16+i, 16+i))
	}
	lines = append(lines, `{"jsonrpc": "2.0", "id": 2, "method": "textDocument/completion", "params": {"textDocument": {"uri": "$ROOT/model.hcl"}, "position": {"line": 10, "character": 14}}}`)
	var got []string
	for _, m := range session(t, dir, lines) {
		var msg struct {
			ID     int
			Method string
			Params publishDiagnosticsParams
		}
		if err := json.Unmarshal(m, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Method != "" {
			got = append(got, fmt.Sprintf("%s %d", msg.Method, len(msg.Params.Diagnostics)))
		} else {
			got = append(got, fmt.Sprint("reply ", msg.ID))
		}
	}
	want := "reply 1, reply 2, textDocument/publishDiagnostics 1"
	if strings.Join(got, ", ") != want {
		t.Errorf("sent %s, want %s", strings.Join(got, ", "), want)
	}
}

// TestUnquotedLabel checks the range of an ID written as a block label without quotes, which
// HCL allows.
func TestUnquotedLabel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model.hcl"), []byte("node abcd {}\n\nnode \"b\" {\n  kind = \"group\"\n}\n\nedge e {\n  from = node.abcd\n  to   = node.b\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs := session(t, dir, []string{
		`{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}}`,
		`{"jsonrpc": "2.0", "id": 2, "method": "textDocument/definition", "params": {"textDocument": {"uri": "$ROOT/model.hcl"}, "position": {"line": 7, "character": 14}}}`,
	})
	var reply struct{ Result Location }
	if err := json.Unmarshal(msgs[1], &reply); err != nil {
		t.Fatal(err)
	}
	if want := (Range{Position{0, 5}, Position{0, 9}}); reply.Result.Range != want {
		t.Errorf("definition at %v, want %v", reply.Result.Range, want)
	}
}
