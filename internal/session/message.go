package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/model"
	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/style"
	"github.com/nvcnvn/dg/internal/view"
)

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

// messages returns the model and pictures messages of p, encoded once per load.
func (s *Session) messages(p *project.Project) (model, pictures []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.modelOf != p {
		info, pics := modelOf(p)
		var err error
		if s.modelData, err = json.Marshal(modelMessage{Type: "model", Model: info}); err != nil {
			panic(fmt.Sprintf("encoding the model message: %v", err))
		}
		if s.picturesData, err = json.Marshal(pics); err != nil {
			panic(fmt.Sprintf("encoding the pictures message: %v", err))
		}
		s.modelOf = p
	}
	return s.modelData, s.picturesData
}

// modelMessage tells a client the model it edits; it is sent before the snapshot that needs it,
// when it differs from the last one the client got.
type modelMessage struct {
	Type  string    `json:"type"`
	Model modelInfo `json:"model"`
}

// modelInfo is what the editor needs of the whole project to edit the model: the vocabulary
// for its palette and inspector, and every node and edge, drawn in the view or not.
type modelInfo struct {
	Packs     []string   `json:"packs"`
	Kinds     []kindInfo `json:"kinds"`
	EdgeKinds []string   `json:"edge_kinds"`
	Tags      []string   `json:"tags"`
	Nodes     []nodeInfo `json:"nodes"`
	Edges     []edgeInfo `json:"edges"`
}

// kindInfo is a kind with the style a node of it has by default, without its pictures, which the
// palette draws.
type kindInfo struct {
	project.KindInfo
	Style style.Node `json:"style"`
}

// picturesMessage tells a client what the palette draws that model leaves out for its size (spec
// 15): each kind's default icon and image, and the custom shapes kinds use by default.
type picturesMessage struct {
	Type   string                  `json:"type"`
	Kinds  map[string]kindPictures `json:"kinds"`
	Shapes map[string]*style.Shape `json:"shapes"`
}

type kindPictures struct {
	Icon  string `json:"icon,omitempty"`
	Image string `json:"image,omitempty"`
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

// modelOf returns the model info of p, nodes and edges in ID order, and the pictures it leaves
// out. A node's ports are its instance ports.
func modelOf(p *project.Project) (modelInfo, picturesMessage) {
	v := p.Vocabulary()
	out := modelInfo{Packs: v.Packs, Kinds: []kindInfo{}, EdgeKinds: v.EdgeKinds, Tags: v.Tags, Nodes: []nodeInfo{}, Edges: []edgeInfo{}}
	pics := picturesMessage{Type: "pictures", Kinds: map[string]kindPictures{}, Shapes: map[string]*style.Shape{}}
	for _, k := range v.Kinds {
		st := style.DrawnNode(p.Styles.ForNode(p.Spec, k.Name, nil))
		if st.Icon != "" || st.Image != "" {
			pics.Kinds[k.Name] = kindPictures{Icon: st.Icon, Image: st.Image}
			st.Icon, st.Image = "", ""
		}
		if sh := p.Styles.Shapes[st.Shape]; sh != nil {
			pics.Shapes[sh.Name] = sh
		}
		out.Kinds = append(out.Kinds, kindInfo{KindInfo: k, Style: st})
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
	return out, pics
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
