// Package server serves the editor (FR-10): the embedded app over HTTP, and over a WebSocket the
// resolved view each client selects and the project's diagnostics, sent again whenever a
// project file changes (FR-13). The editor is read-only: clients cannot send operations yet.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
	"github.com/nvcnvn/dg/web"
)

const writeTimeout = 10 * time.Second

// Server holds the project as last loaded and the connected clients.
type Server struct {
	dir string

	mu      sync.Mutex
	cur     *project.Project // the latest load
	diags   diag.Diagnostics // of the latest load
	good    *project.Project // the latest load without errors
	loadErr error            // set when the latest load failed outright
	clients map[*client]bool
}

type client struct {
	conn *websocket.Conn
	mu   sync.Mutex // serializes view and sending
	view string
	last []byte // the last snapshot sent
}

// New loads the project in dir.
func New(dir string) *Server {
	s := &Server{dir: dir, clients: map[*client]bool{}}
	s.load()
	return s
}

// Handler serves the app at / and the WebSocket at /ws, only to requests whose Host is a
// loopback address, so a page on another site cannot reach it by rebinding its name to
// 127.0.0.1.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", web.Handler())
	mux.HandleFunc("/ws", s.serveWS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsLoopback(r.Host) {
			http.Error(w, "dg serve answers only on a loopback address", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// IsLoopback reports whether hostport, with or without a port, names a loopback address.
func IsLoopback(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = strings.Trim(hostport, "[]")
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) load() {
	p, diags, err := project.Load(s.dir)
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
func (s *Server) Reload(ctx context.Context) {
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

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn}
	s.mu.Lock()
	s.clients[c] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		_ = conn.CloseNow()
	}()
	ctx := r.Context()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		m, err := DecodeMessage(data)
		if err != nil {
			c.send(ctx, errorMessage{Type: "error", Message: err.Error()})
			continue
		}
		if m.View != "" && !s.defines(m.View) {
			c.send(ctx, errorMessage{Type: "error", Message: fmt.Sprintf("view %q is not defined", m.View)})
			continue
		}
		c.mu.Lock()
		c.view = m.View
		c.mu.Unlock()
		s.update(ctx, c)
	}
}

// update sends c its diagnostics and, when its view can be drawn, its snapshot. The view is
// drawn from the latest load, or from the latest load without errors when that has errors, so
// a client keeps seeing the last good drawing while the project is broken (FR-10).
func (s *Server) update(ctx context.Context, c *client) {
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
func (s *Server) defines(id string) bool {
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

func (c *client) sendSnapshot(ctx context.Context, m snapshotMessage) {
	data, err := json.Marshal(m)
	if err != nil || bytes.Equal(data, c.last) {
		return
	}
	c.last = data
	c.write(ctx, data)
}

func (c *client) send(ctx context.Context, m any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sendLocked(ctx, m)
}

func (c *client) sendLocked(ctx context.Context, m any) {
	data, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("encoding a server message: %v", err))
	}
	c.write(ctx, data)
}

func (c *client) write(ctx context.Context, data []byte) {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, data); err != nil {
		_ = c.conn.CloseNow()
	}
}

// Message is a message from a client. Only select-view is accepted; View "" selects the
// default view.
type Message struct {
	Type string `json:"type"`
	View string `json:"view"`
}

// DecodeMessage decodes and checks a message from a client.
func DecodeMessage(data []byte) (Message, error) {
	var m Message
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Message{}, fmt.Errorf("invalid message: %v", err)
	}
	if dec.More() {
		return Message{}, errors.New("invalid message: more than one JSON value")
	}
	switch m.Type {
	case "select-view":
		return m, nil
	case "op", "undo", "redo":
		return Message{}, fmt.Errorf("%s is not supported: the editor is read-only", m.Type)
	}
	return Message{}, fmt.Errorf("unknown message type %q", strings.ToValidUTF8(m.Type, "�"))
}

type viewInfo struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

type snapshotMessage struct {
	Type     string         `json:"type"`
	Views    []viewInfo     `json:"views"`
	Resolved *view.Resolved `json:"resolved"`
}

type diagnosticsMessage struct {
	Type        string           `json:"type"`
	Diagnostics diag.Diagnostics `json:"diagnostics"`
}

type errorMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
