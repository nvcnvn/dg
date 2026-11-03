// Package lsp is dg's language server (spec FR-17): the Language Server Protocol over a stream,
// for the HCL files of one project, working on the editor's unsaved buffers.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/project"
)

// Server serves one project. It handles one message at a time.
type Server struct {
	dir  string // absolute
	out  io.Writer
	docs map[string]*document // open buffers by project-relative path

	loaded    *loaded // nil when a buffer or file changed since the last load
	lastGood  *project.Project
	published map[string]string // the diagnostics last published for each URI, as JSON
	dirty     bool              // diagnostics are to be published again
	state     int
}

// Lifecycle states of a server.
const (
	starting = iota
	running
	shutDown
)

type document struct {
	text    string
	version int
}

// loaded is the project as last loaded from the buffers over the files, with its diagnostics.
type loaded struct {
	p     *project.Project
	diags diag.Diagnostics
	texts map[string]string
	lines map[string]*lines
	ix    *index
}

// New returns a server for the project in dir.
func New(dir string) (*Server, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &Server{dir: abs, docs: map[string]*document{}, published: map[string]string{}}, nil
}

// errExitWithoutShutdown is returned by Serve when the client sends exit before shutdown.
var errExitWithoutShutdown = errors.New("exit before shutdown")

// Serve reads messages from r and writes messages to w until the client sends exit or r ends.
// Diagnostics are published after a change once no more input is waiting or a request has been
// answered, so a burst of changes, such as fast typing, loads the project once.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	s.out = w
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		body, err := readMessage(br)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		exit, answered, err := s.handle(body)
		if exit || err != nil {
			return err
		}
		if s.dirty && (answered || br.Buffered() == 0) {
			if err := s.publish(); err != nil {
				return err
			}
		}
	}
}

// handle handles one message, reporting whether it is exit and whether it was a request.
func (s *Server) handle(body []byte) (exit, request bool, err error) {
	var m inMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return false, false, s.send(errorResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParseError, err.Error()}})
	}
	switch {
	case m.Method == "":
		return false, false, nil // a response to a request the server never sends
	case m.Method == "exit":
		if s.state != shutDown {
			return true, false, errExitWithoutShutdown
		}
		return true, false, nil
	case len(m.ID) == 0 || string(m.ID) == "null":
		if s.state == running {
			s.notify(m.Method, m.Params)
		}
		return false, false, nil
	}
	var result any
	switch {
	case s.state == starting && m.Method != "initialize":
		err = &rpcError{codeServerNotInitialized, "the server is not initialized"}
	case s.state == shutDown:
		err = &rpcError{codeInvalidRequest, "the server is shut down"}
	default:
		result, err = s.request(m.Method, m.Params)
	}
	if err != nil {
		var re *rpcError
		if !errors.As(err, &re) {
			re = &rpcError{codeRequestFailed, err.Error()}
		}
		return false, true, s.send(errorResponse{JSONRPC: "2.0", ID: m.ID, Error: re})
	}
	return false, true, s.send(response{JSONRPC: "2.0", ID: m.ID, Result: result})
}

func (s *Server) send(m any) error {
	return writeMessage(s.out, m)
}

func decode(params json.RawMessage, v any) error {
	if err := json.Unmarshal(params, v); err != nil {
		return &rpcError{codeInvalidParams, "invalid params: " + err.Error()}
	}
	return nil
}

// changed marks the project as changed: it is loaded again when next needed, and its
// diagnostics published.
func (s *Server) changed() {
	s.loaded = nil
	s.dirty = true
}

func (s *Server) notify(method string, params json.RawMessage) {
	switch method {
	case "initialized":
		s.changed()
	case "textDocument/didOpen":
		var p didOpenParams
		if decode(params, &p) != nil {
			return
		}
		if rel, ok := s.rel(p.TextDocument.URI); ok {
			s.docs[rel] = &document{text: p.TextDocument.Text, version: p.TextDocument.Version}
			s.changed()
		}
	case "textDocument/didChange":
		var p didChangeParams
		if decode(params, &p) != nil {
			return
		}
		rel, ok := s.rel(p.TextDocument.URI)
		if d := s.docs[rel]; ok && d != nil {
			for _, c := range p.ContentChanges {
				if c.Range == nil {
					d.text = c.Text
					continue
				}
				ls := newLines(d.text)
				start, end := ls.offset(c.Range.Start), ls.offset(c.Range.End)
				d.text = d.text[:start] + c.Text + d.text[max(start, end):]
			}
			d.version = p.TextDocument.Version
			s.changed()
		}
	case "textDocument/didClose":
		var p documentParams
		if decode(params, &p) != nil {
			return
		}
		if rel, ok := s.rel(p.TextDocument.URI); ok {
			delete(s.docs, rel)
			s.changed()
		}
	case "textDocument/didSave", "workspace/didChangeWatchedFiles":
		s.changed()
	}
}

func (s *Server) request(method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		s.state = running
		return map[string]any{
			"capabilities": map[string]any{
				"textDocumentSync":           map[string]any{"openClose": true, "change": 2, "save": true},
				"completionProvider":         map[string]any{"triggerCharacters": []string{".", `"`}},
				"hoverProvider":              true,
				"definitionProvider":         true,
				"referencesProvider":         true,
				"renameProvider":             map[string]any{"prepareProvider": true},
				"documentFormattingProvider": true,
			},
			"serverInfo": map[string]any{"name": "dg"},
		}, nil
	case "shutdown":
		s.state = shutDown
		return nil, nil
	case "textDocument/completion":
		return s.positionRequest(params, s.completion)
	case "textDocument/hover":
		return s.positionRequest(params, s.hover)
	case "textDocument/definition":
		return s.positionRequest(params, s.definition)
	case "textDocument/references":
		var p referenceParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return s.positionRequest(params, func(l *loaded, rel string, pos Position) (any, error) {
			return s.references(l, rel, pos, p.Context.IncludeDeclaration)
		})
	case "textDocument/prepareRename":
		return s.positionRequest(params, s.prepareRename)
	case "textDocument/rename":
		var p renameParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return s.positionRequest(params, func(l *loaded, rel string, pos Position) (any, error) {
			return s.rename(l, rel, pos, p.NewName)
		})
	case "textDocument/formatting":
		var p documentParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		rel, ok := s.rel(p.TextDocument.URI)
		if !ok {
			return nil, nil
		}
		l, err := s.load()
		if err != nil {
			return nil, err
		}
		return s.format(l, rel)
	}
	return nil, &rpcError{codeMethodNotFound, fmt.Sprintf("method %q is not supported", method)}
}

// positionRequest decodes the params of a request at a position in a document and calls f with
// the project and the document's project-relative path; a document outside the project gets
// a null result.
func (s *Server) positionRequest(params json.RawMessage, f func(*loaded, string, Position) (any, error)) (any, error) {
	var p positionParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	l, err := s.load()
	if err != nil {
		return nil, err
	}
	return f(l, rel, p.Position)
}

// fsys returns the project's files with the open buffers over them, and a function that closes
// it.
func (s *Server) fsys() (*overlay, func(), error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, nil, err
	}
	return &overlay{base: root.FS(), docs: s.docs, read: map[string]string{}}, func() { root.Close() }, nil
}

// overlay is a project's files with the open buffers in place of the files they edit. It
// remembers the content of every file opened, so positions are found in the text that was read.
type overlay struct {
	base fs.FS
	docs map[string]*document
	read map[string]string
}

func (o *overlay) Open(name string) (fs.File, error) {
	if d := o.docs[name]; d != nil {
		o.read[name] = d.text
		return fstest.MapFS{name: {Data: []byte(d.text), Mode: 0o644}}.Open(name)
	}
	f, err := o.base.Open(name)
	if err != nil {
		return nil, err
	}
	if info, err := f.Stat(); err != nil || info.IsDir() {
		return f, nil
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	o.read[name] = string(data)
	return fstest.MapFS{name: {Data: data, Mode: 0o644}}.Open(name)
}

// load returns the project as the buffers and files now are, loading it again after a change.
func (s *Server) load() (*loaded, error) {
	if s.loaded != nil {
		return s.loaded, nil
	}
	fsys, done, err := s.fsys()
	if err != nil {
		return nil, err
	}
	defer done()
	p, diags, err := project.LoadFS(fsys, s.dir)
	if err != nil {
		return nil, err
	}
	l := &loaded{p: p, diags: diags, texts: fsys.read, lines: map[string]*lines{}}
	for rel, d := range s.docs {
		if _, ok := l.texts[rel]; !ok {
			l.texts[rel] = d.text
		}
	}
	s.loaded = l
	if len(p.Unparsed) == 0 {
		s.lastGood = p
	}
	return l, nil
}

// linesOf returns the text of project file rel indexed by line.
func (l *loaded) linesOf(rel string) *lines {
	if l.lines[rel] == nil {
		l.lines[rel] = newLines(l.texts[rel])
	}
	return l.lines[rel]
}

// index returns the index of IDs in the project's files, built when first needed.
func (l *loaded) index() *index {
	if l.ix == nil {
		l.ix = buildIndex(l)
	}
	return l.ix
}

// parsed returns the project of l, or, while one of its files does not parse, the project as
// last loaded with every file parsed, whose model knows what the file being typed defined.
func (s *Server) parsed(l *loaded) *project.Project {
	if len(l.p.Unparsed) > 0 && s.lastGood != nil {
		return s.lastGood
	}
	return l.p
}

// publish sends the diagnostics of every file whose diagnostics changed since they were last
// published, an empty list for a file that has none any more. Diagnostics without a file, or
// about the built-in pack, which the editor cannot open, are put at the start of dg.hcl.
func (s *Server) publish() error {
	s.dirty = false
	l, err := s.load()
	if err != nil {
		return s.send(notification{JSONRPC: "2.0", Method: "window/showMessage", Params: map[string]any{
			"type": 1, "message": "dg: cannot load the project: " + err.Error(),
		}})
	}
	byURI := map[string][]diagnostic{}
	for _, d := range l.diags {
		file, rng := d.File, Range{}
		if file == "" || strings.HasPrefix(file, "<") {
			file = project.ProjectFile
		} else {
			rng = l.linesOf(file).rng(d.Byte, d.EndByte)
		}
		msg := d.Summary
		if d.Detail != "" {
			msg += ": " + d.Detail
		}
		sev := map[diag.Severity]int{diag.Error: severityError, diag.Warning: severityWarning, diag.Info: severityInformation}[d.Severity]
		uri := s.uri(file)
		byURI[uri] = append(byURI[uri], diagnostic{Range: rng, Severity: sev, Source: "dg", Message: msg})
	}
	for uri := range s.published {
		if _, ok := byURI[uri]; !ok {
			byURI[uri] = []diagnostic{}
		}
	}
	for _, uri := range slices.Sorted(maps.Keys(byURI)) {
		data, err := json.Marshal(byURI[uri])
		if err != nil {
			return err
		}
		if s.published[uri] == string(data) {
			continue
		}
		if len(byURI[uri]) == 0 {
			delete(s.published, uri)
		} else {
			s.published[uri] = string(data)
		}
		err = s.send(notification{JSONRPC: "2.0", Method: "textDocument/publishDiagnostics", Params: publishDiagnosticsParams{URI: uri, Diagnostics: byURI[uri]}})
		if err != nil {
			return err
		}
	}
	return nil
}

// rel returns the project-relative slash path of a file URI inside the project.
func (s *Server) rel(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return "", false
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
	}
	rel, err := filepath.Rel(s.dir, filepath.FromSlash(p))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if !fs.ValidPath(rel) || rel == "." {
		return "", false
	}
	return rel, true
}

// uri returns the file URI of project-relative path rel.
func (s *Server) uri(rel string) string {
	p := filepath.ToSlash(filepath.Join(s.dir, filepath.FromSlash(rel)))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
