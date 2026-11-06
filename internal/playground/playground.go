// Package playground is the Go half of the playground (FR-19): a project in memory, its session,
// and the messages only the page has.
package playground

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"

	"github.com/nvcnvn/dg/internal/fsys"
	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/session"
)

// The files of a playground project; the page shows the first two. Hiding a node or making a view
// adds ViewsFile and more layout files.
const (
	ModelFile  = "model.hcl"
	StyleFile  = "style.hcl"
	LayoutFile = "views/main.layout.yaml"
	ViewsFile  = "views.hcl"
)

// projectFile reports whether name is a file a playground project can hold.
func projectFile(name string) bool {
	layout, _ := path.Match("views/*.layout.yaml", name)
	return layout || name == ModelFile || name == StyleFile || name == ViewsFile
}

// Playground is the project of one page. Messages reach it one at a time.
type Playground struct {
	send    func([]byte)
	files   *fsys.Memory
	session *session.Session
	client  *session.Client
	shown   map[string]string // the files as the page last got them
}

// New returns a playground with no project yet, which sends its messages through send.
func New(send func([]byte)) *Playground {
	return &Playground{send: send}
}

// Ready tells the page that messages can come.
func (p *Playground) Ready() {
	p.reply(message{Type: "ready"})
}

// message is a message only the playground has, in either direction: from the page load, with
// the files of a project, write-file, or zip; to the page ready, loaded, files, or zip with data.
type message struct {
	Type    string            `json:"type"`
	Seq     int64             `json:"seq,omitempty"`
	Path    string            `json:"path,omitempty"`
	Text    *string           `json:"text,omitempty"`
	Files   map[string]string `json:"files,omitempty"`
	Data    string            `json:"data,omitempty"`
	Message string            `json:"message,omitempty"`
}

// Receive handles a message from the page.
func (p *Playground) Receive(ctx context.Context, data []byte) {
	var m message
	if err := json.Unmarshal(data, &m); err == nil {
		switch m.Type {
		case "load":
			p.fail(m, p.load(m.Files))
			return
		case "write-file":
			p.fail(m, p.writeFile(ctx, m))
			return
		case "zip":
			p.fail(m, p.zip(m))
			return
		}
	}
	if p.session == nil {
		p.fail(m, errors.New("no project is loaded"))
		return
	}
	p.session.Receive(ctx, p.client, data)
	p.showFiles()
}

func (p *Playground) fail(m message, err error) {
	if err != nil {
		p.reply(message{Type: "error", Seq: m.Seq, Message: err.Error()})
	}
}

func (p *Playground) reply(m message) {
	data, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("encoding a playground message: %v", err))
	}
	p.send(data)
}

// load replaces the project, and with it the undo history.
func (p *Playground) load(project map[string]string) error {
	if len(project) == 0 {
		return errors.New("a project needs a file")
	}
	out := map[string][]byte{}
	for name, text := range project {
		if !projectFile(name) {
			return fmt.Errorf("a playground project has no file %q", name)
		}
		out[name] = []byte(text)
	}
	files, err := fsys.NewMemory(out)
	if err != nil {
		return err
	}
	if p.session != nil {
		p.session.Disconnect(p.client)
	}
	p.files, p.session = files, session.New(files)
	p.client = p.session.Connect(func(_ context.Context, data []byte) { p.send(data) })
	p.shown = nil
	p.showFiles()
	p.reply(message{Type: "loaded"})
	return nil
}

// writeFile saves what the page's text editor holds, as an outside save (FR-13). The page is not
// sent the file back: its visitor may have typed on since.
func (p *Playground) writeFile(ctx context.Context, m message) error {
	if p.session == nil {
		return errors.New("no project is loaded")
	}
	if m.Path != ModelFile && m.Path != StyleFile {
		return fmt.Errorf("the page cannot write %q", m.Path)
	}
	if m.Text == nil {
		return errors.New("write-file takes a text")
	}
	if err := ops.Save(p.files, m.Path, []byte(*m.Text)); err != nil {
		return err
	}
	p.shown[m.Path] = *m.Text
	p.session.Reload(ctx)
	return nil
}

// showFiles sends the page the project's files if they changed since it last got them.
func (p *Playground) showFiles() {
	now := read(p.files)
	if p.shown == nil || !maps.Equal(now, p.shown) {
		p.shown = now
		p.reply(message{Type: "files", Files: now})
	}
}

// read returns the project's files; a file in memory can always be read.
func read(files fs.FS) map[string]string {
	out := map[string]string{}
	err := fs.WalkDir(files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(files, name)
		out[name] = string(data)
		return err
	})
	if err != nil {
		panic(fmt.Sprintf("reading the files in memory: %v", err))
	}
	return out
}

// zip sends the project as a .zip, in base64. It holds no times, so the same files make the same
// archive.
func (p *Playground) zip(m message) error {
	if p.session == nil {
		return errors.New("no project is loaded")
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	files := read(p.files)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		f, err := w.Create(name)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte(files[name])); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	p.reply(message{Type: "zip", Seq: m.Seq, Data: base64.StdEncoding.EncodeToString(b.Bytes())})
	return nil
}
