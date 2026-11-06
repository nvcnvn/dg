// Package session is the editor of one project (FR-10), whatever carries its messages: the resolved
// view each client selects and the project's diagnostics, sent again on every reload, and the
// operations, undo and redo its clients send, with one undo history.
package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/fsys"
	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

// Session holds the project as last loaded and the connected clients.
type Session struct {
	files fsys.FS

	mu      sync.Mutex
	cur     *project.Project // the latest load
	diags   diag.Diagnostics // of the latest load
	good    *project.Project // the latest load without errors
	loadErr error            // set when the latest load failed outright
	clients map[*Client]bool
	// modelData is the model message of the project modelOf, kept so it is encoded once.
	modelOf   *project.Project
	modelData []byte

	// edit serializes the operations, undos and redos of every client with the messages they
	// send; it guards the history and the session they share (FR-10).
	edit sync.Mutex
	hist ops.History
	sess *ops.Session
}

// Client is a connected client of a session.
type Client struct {
	write func(context.Context, []byte)
	mu    sync.Mutex // serializes view and sending
	view  string
	last  []byte // the last snapshot sent
	model []byte // the last model message sent
}

// New loads the project in files.
func New(files fsys.FS) *Session {
	s := &Session{files: files, clients: map[*Client]bool{}, sess: ops.NewSession()}
	s.load()
	return s
}

// Connect adds a client that gets its messages from write, one at a time, until Disconnect.
func (s *Session) Connect(write func(context.Context, []byte)) *Client {
	c := &Client{write: write}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[c] = true
	return c
}

// Disconnect removes client c.
func (s *Session) Disconnect(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, c)
}

// Receive handles message data from client c.
func (s *Session) Receive(ctx context.Context, c *Client, data []byte) {
	m, err := DecodeMessage(data)
	if err != nil {
		c.send(ctx, errorMessage{Type: "error", Seq: m.Seq, Message: err.Error()})
		return
	}
	switch m.Type {
	case "select-view":
		if m.View != "" && !s.defines(m.View) {
			c.send(ctx, errorMessage{Type: "error", Seq: m.Seq, Message: fmt.Sprintf("view %q is not defined", m.View)})
			return
		}
		c.mu.Lock()
		c.view = m.View
		c.mu.Unlock()
		s.update(ctx, c)
		c.send(ctx, s.history())
	default:
		s.change(ctx, c, m)
	}
}

func (s *Session) load() {
	p, diags, err := project.LoadFS(s.files)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadErr = err
	if err != nil {
		return
	}
	s.cur, s.diags = p, diags
	if !diags.HasErrors() {
		s.good = p
	}
}

// Reload loads the project again and updates every client, all at once so a slow client does
// not delay the others.
func (s *Session) Reload(ctx context.Context) {
	s.load()
	s.mu.Lock()
	clients := slices.Collect(maps.Keys(s.clients))
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Go(func() { s.update(ctx, c) })
	}
	wg.Wait()
}

// change applies the operation, undo or redo m from client c, sends every client its new
// snapshot and the history, and then answers c. It holds s.edit throughout, so the clients get
// the histories in the order of the changes.
func (s *Session) change(ctx context.Context, c *Client, m Message) {
	s.edit.Lock()
	defer s.edit.Unlock()
	var ch *ops.Change
	var err error
	switch m.Type {
	case "op":
		if ch, err = ops.Apply(s.files, *m.Op, s.sess); err == nil {
			s.hist.Record(ch)
		}
	case "undo":
		ch, err = s.hist.Undo(s.files, s.sess)
	case "redo":
		ch, err = s.hist.Redo(s.files, s.sess)
	}
	if err == nil && len(ch.Files) > 0 {
		s.Reload(ctx)
	}
	hist := s.historyLocked()
	s.mu.Lock()
	clients := slices.Collect(maps.Keys(s.clients))
	s.mu.Unlock()
	for _, o := range clients {
		o.send(ctx, hist)
	}
	if err != nil {
		e := errorMessage{Type: "error", Seq: m.Seq, Message: err.Error()}
		if r, ok := errors.AsType[*ops.RefusedError](err); ok {
			e.Diagnostics = r.Diagnostics
		}
		c.send(ctx, e)
		return
	}
	c.send(ctx, ackMessage{Type: "ack", Seq: m.Seq, Results: ch.Results, Files: ch.Paths(), Warnings: ch.Warnings})
}

func (s *Session) history() historyMessage {
	s.edit.Lock()
	defer s.edit.Unlock()
	return s.historyLocked()
}

func (s *Session) historyLocked() historyMessage {
	return historyMessage{Type: "history", Undo: s.hist.CanUndo(), Redo: s.hist.CanRedo()}
}

// update sends c its diagnostics and, when its view can be drawn, its snapshot. The view is
// drawn from the latest load, or from the latest load without errors when that has errors, so
// a client keeps seeing the last good drawing while the project is broken (FR-10).
func (s *Session) update(ctx context.Context, c *Client) {
	s.mu.Lock()
	cur, diags, good, loadErr := s.cur, s.diags, s.good, s.loadErr
	s.mu.Unlock()
	if loadErr != nil {
		c.send(ctx, errorMessage{Type: "error", Message: fmt.Sprintf("cannot load the project: %v", loadErr)})
		return
	}
	p := cur
	if diags.HasErrors() {
		p = good
	}
	all := slices.Clone(diags)
	c.mu.Lock()
	defer c.mu.Unlock()
	if p != nil {
		id := defaultView(p, c.view)
		r, rdiags := p.Resolve(id)
		if p == cur {
			all = append(all, rdiags...)
			all.Sort()
		}
		if !rdiags.HasErrors() {
			if m := s.modelMessage(p); !bytes.Equal(m, c.model) {
				c.model = m
				c.write(ctx, m)
			}
			c.sendSnapshot(ctx, snapshotMessage{Type: "snapshot", Views: views(p), Resolved: r})
		}
	}
	if all == nil {
		all = diag.Diagnostics{}
	}
	c.sendLocked(ctx, diagnosticsMessage{Type: "diagnostics", Diagnostics: all})
}

// defines reports whether the project as last loaded, or as last loaded without errors, defines
// view id.
func (s *Session) defines(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur != nil && s.cur.Views[id] != nil || s.good != nil && s.good.Views[id] != nil
}

// defaultView returns id if p defines it, else "main" if p defines that, else p's first view.
func defaultView(p *project.Project, id string) string {
	if p.Views[id] != nil {
		return id
	}
	if p.Views[view.DefaultView] != nil {
		return view.DefaultView
	}
	return slices.Sorted(maps.Keys(p.Views))[0]
}

func views(p *project.Project) []viewInfo {
	var out []viewInfo
	for _, id := range slices.Sorted(maps.Keys(p.Views)) {
		out = append(out, viewInfo{ID: id, Title: p.Views[id].Title})
	}
	return out
}

func (c *Client) sendSnapshot(ctx context.Context, m snapshotMessage) {
	data, err := json.Marshal(m)
	if err != nil || bytes.Equal(data, c.last) {
		return
	}
	c.last = data
	c.write(ctx, data)
}

func (c *Client) send(ctx context.Context, m any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sendLocked(ctx, m)
}

func (c *Client) sendLocked(ctx context.Context, m any) {
	data, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("encoding a server message: %v", err))
	}
	c.write(ctx, data)
}
