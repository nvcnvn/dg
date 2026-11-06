// Package server serves the editor (FR-10): the embedded app over HTTP, and over a WebSocket the
// resolved view each client selects and the project's diagnostics, sent again whenever a
// project file changes (FR-13), and the operations, undo and redo its clients send, with one
// undo history for the session.
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
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/fsys"
	"github.com/nvcnvn/dg/internal/model"
	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/style"
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
	// modelData is the model message of the project modelOf, kept so it is encoded once.
	modelOf   *project.Project
	modelData []byte

	// edit serializes the operations, undos and redos of every client with the messages they
	// send; it guards the history and the session they share (FR-10).
	edit sync.Mutex
	hist ops.History
	sess *ops.Session
}

type client struct {
	conn  *websocket.Conn
	mu    sync.Mutex // serializes view and sending
	view  string
	last  []byte // the last snapshot sent
	model []byte // the last model message sent
}

// New loads the project in dir.
func New(dir string) *Server {
	s := &Server{dir: dir, clients: map[*client]bool{}, sess: ops.NewSession()}
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
			c.send(ctx, errorMessage{Type: "error", Seq: m.Seq, Message: err.Error()})
			continue
		}
		switch m.Type {
		case "select-view":
			if m.View != "" && !s.defines(m.View) {
				c.send(ctx, errorMessage{Type: "error", Seq: m.Seq, Message: fmt.Sprintf("view %q is not defined", m.View)})
				continue
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
}

// change applies the operation, undo or redo m from client c, sends every client its new
// snapshot and the history, and then answers c. It holds s.edit throughout, so the clients get
// the histories in the order of the changes.
func (s *Server) change(ctx context.Context, c *client, m Message) {
	s.edit.Lock()
	defer s.edit.Unlock()
	var ch *ops.Change
	var err error
	switch m.Type {
	case "op":
		if ch, err = ops.Apply(fsys.Disk(s.dir), *m.Op, s.sess); err == nil {
			s.hist.Record(ch)
		}
	case "undo":
		ch, err = s.hist.Undo(fsys.Disk(s.dir), s.sess)
	case "redo":
		ch, err = s.hist.Redo(fsys.Disk(s.dir), s.sess)
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

func (s *Server) history() historyMessage {
	s.edit.Lock()
	defer s.edit.Unlock()
	return s.historyLocked()
}

func (s *Server) historyLocked() historyMessage {
	return historyMessage{Type: "history", Undo: s.hist.CanUndo(), Redo: s.hist.CanRedo()}
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

// Message is a message from a client: select-view (View "" selects the default view), op with
// the operation (FR-12), undo or redo. Seq, when set, is repeated in the ack or error that
// answers an op, undo or redo.
type Message struct {
	Type string
	Seq  int64
	View string
	Op   *ops.Op
}

// DecodeMessage decodes and checks a message from a client. A message that is not valid still
// has its Seq when it has a whole number seq, so the error answering it can repeat it.
func DecodeMessage(data []byte) (Message, error) {
	var head struct {
		Seq int64 `json:"seq"`
	}
	_ = json.Unmarshal(data, &head)
	m, err := decodeMessage(data)
	if err != nil {
		return Message{Seq: head.Seq}, err
	}
	return m, nil
}

func decodeMessage(data []byte) (Message, error) {
	var raw struct {
		Type string          `json:"type"`
		Seq  int64           `json:"seq"`
		View *string         `json:"view"`
		Op   json.RawMessage `json:"op"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return Message{}, fmt.Errorf("invalid message: %v", err)
	}
	if dec.More() {
		return Message{}, errors.New("invalid message: more than one JSON value")
	}
	m := Message{Type: raw.Type, Seq: raw.Seq}
	switch m.Type {
	case "select-view":
		if raw.Op != nil {
			return Message{}, errors.New("invalid message: select-view takes no op")
		}
		if raw.View != nil {
			m.View = *raw.View
		}
		return m, nil
	case "op":
		if raw.View != nil || raw.Op == nil {
			return Message{}, errors.New("invalid message: op takes an op and no view")
		}
		op, err := ops.DecodeOp(raw.Op)
		if err != nil {
			return Message{}, err
		}
		m.Op = &op
		return m, nil
	case "undo", "redo":
		if raw.View != nil || raw.Op != nil {
			return Message{}, fmt.Errorf("invalid message: %s takes no view or op", m.Type)
		}
		return m, nil
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

// modelMessage returns the model message of p, encoded once per load.
func (s *Server) modelMessage(p *project.Project) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.modelOf != p {
		data, err := json.Marshal(modelMessage{Type: "model", Model: modelOf(p)})
		if err != nil {
			panic(fmt.Sprintf("encoding the model message: %v", err))
		}
		s.modelOf, s.modelData = p, data
	}
	return s.modelData
}

// modelMessage tells a client the model it edits; it is sent before the snapshot that needs it,
// when it differs from the last one the client got.
type modelMessage struct {
	Type  string    `json:"type"`
	Model modelInfo `json:"model"`
}

// modelInfo is what the editor needs of the whole project to edit the model: the vocabulary
// for its palette and inspector, the custom shapes kinds use by default, and every node and
// edge, drawn in the view or not.
type modelInfo struct {
	Packs     []string                `json:"packs"`
	Kinds     []kindInfo              `json:"kinds"`
	EdgeKinds []string                `json:"edge_kinds"`
	Tags      []string                `json:"tags"`
	Shapes    map[string]*style.Shape `json:"shapes,omitempty"`
	Nodes     []nodeInfo              `json:"nodes"`
	Edges     []edgeInfo              `json:"edges"`
}

// kindInfo is a kind with the style a node of it has by default, which the palette draws.
type kindInfo struct {
	project.KindInfo
	Style style.Node `json:"style"`
}

type nodeInfo struct {
	ID           string                     `json:"id"`
	Kind         string                     `json:"kind"`
	Label        string                     `json:"label"`
	Parent       string                     `json:"parent,omitempty"`
	Tags         []string                   `json:"tags,omitempty"`
	Props        map[string]json.RawMessage `json:"props,omitempty"`
	Ports        []view.Port                `json:"ports,omitempty"`
	Compartments []view.Compartment         `json:"compartments,omitempty"`
	ReadOnly     bool                       `json:"read_only,omitempty"`
}

type edgeInfo struct {
	ID       string       `json:"id"`
	Kind     string       `json:"kind"`
	From     view.End     `json:"from"`
	To       view.End     `json:"to"`
	Labels   model.Labels `json:"labels"`
	Tags     []string     `json:"tags,omitempty"`
	ReadOnly bool         `json:"read_only,omitempty"`
}

// modelOf returns the model info of p, nodes and edges in ID order. A node's ports are its
// instance ports.
func modelOf(p *project.Project) modelInfo {
	v := p.Vocabulary()
	out := modelInfo{Packs: v.Packs, Kinds: []kindInfo{}, EdgeKinds: v.EdgeKinds, Tags: v.Tags, Nodes: []nodeInfo{}, Edges: []edgeInfo{}}
	for _, k := range v.Kinds {
		info := kindInfo{KindInfo: k, Style: style.DrawnNode(p.Styles.ForNode(p.Spec, k.Name, nil))}
		if sh := p.Styles.Shapes[info.Style.Shape]; sh != nil {
			if out.Shapes == nil {
				out.Shapes = map[string]*style.Shape{}
			}
			out.Shapes[sh.Name] = sh
		}
		out.Kinds = append(out.Kinds, info)
	}
	for _, id := range slices.Sorted(maps.Keys(p.Model.Nodes)) {
		n := p.Model.Nodes[id]
		info := nodeInfo{ID: id, Kind: n.Kind, Label: n.Label, Tags: n.Tags, ReadOnly: p.IsReadOnly(n.Range.Filename)}
		if n.Parent != nil {
			info.Parent = n.Parent.Node
		}
		for _, prop := range n.Props {
			if data, err := ctyjson.Marshal(prop.Value, prop.Value.Type()); err == nil {
				if info.Props == nil {
					info.Props = map[string]json.RawMessage{}
				}
				info.Props[prop.Name] = data
			}
		}
		for _, port := range n.Ports {
			info.Ports = append(info.Ports, view.Port{Name: port.Name, Side: port.Side})
		}
		for _, c := range n.Compartments {
			rc := view.Compartment{Name: c.Name, Rows: []view.Row{}}
			for _, r := range c.Rows {
				rc.Rows = append(rc.Rows, view.Row{Name: r.Name, Type: r.Type, Key: r.Key})
			}
			info.Compartments = append(info.Compartments, rc)
		}
		out.Nodes = append(out.Nodes, info)
	}
	for _, id := range slices.Sorted(maps.Keys(p.Model.Edges)) {
		e := p.Model.Edges[id]
		out.Edges = append(out.Edges, edgeInfo{
			ID: id, Kind: e.Kind, Labels: e.Labels, Tags: e.Tags, ReadOnly: p.IsReadOnly(e.Range.Filename),
			From: view.End{Node: e.From.Node, Port: e.From.Port, Row: e.From.Row},
			To:   view.End{Node: e.To.Node, Port: e.To.Port, Row: e.To.Row},
		})
	}
	return out
}

type diagnosticsMessage struct {
	Type        string           `json:"type"`
	Diagnostics diag.Diagnostics `json:"diagnostics"`
}

type errorMessage struct {
	Type        string           `json:"type"`
	Seq         int64            `json:"seq,omitempty"`
	Message     string           `json:"message"`
	Diagnostics diag.Diagnostics `json:"diagnostics,omitempty"`
}

type ackMessage struct {
	Type     string           `json:"type"`
	Seq      int64            `json:"seq,omitempty"`
	Results  []ops.Result     `json:"results"`
	Files    []string         `json:"files"`
	Warnings diag.Diagnostics `json:"warnings,omitempty"`
}

// historyMessage tells every client whether the session has a change to undo and to redo.
type historyMessage struct {
	Type string `json:"type"`
	Undo bool   `json:"undo"`
	Redo bool   `json:"redo"`
}
