package session

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nvcnvn/dg/internal/ops"
)

func TestDecodeMessage(t *testing.T) {
	x := 40
	for _, tc := range []struct {
		in   string
		want Message
		err  string
	}{
		{`{"type":"select-view","view":"main"}`, Message{Type: "select-view", View: "main"}, ""},
		{`{"type":"select-view"}`, Message{Type: "select-view"}, ""},
		{`{"type":"op","seq":3,"op":{"op":"move","view":"main","id":"a","x":40,"y":40}}`, Message{Type: "op", Seq: 3, Op: &ops.Op{Op: "move", View: "main", ID: "a", X: &x, Y: &x}}, ""},
		{`{"type":"undo","seq":4}`, Message{Type: "undo", Seq: 4}, ""},
		{`{"type":"redo"}`, Message{Type: "redo"}, ""},
		{`{"type":"op"}`, Message{}, "op takes an op"},
		{`{"type":"op","seq":7,"op":{"op":"move","view":"main","id":"a","x":10.5,"y":0}}`, Message{Seq: 7}, "whole number"},
		{`{"type":"redo","seq":8,"view":"main"}`, Message{Seq: 8}, "takes no view"},
		{`{"type":"op","op":{"op":"move","view":"main"}}`, Message{}, "missing field"},
		{`{"type":"op","op":{"op":"fly"}}`, Message{}, "unknown operation"},
		{`{"type":"op","view":"main","op":{"op":"unpin","view":"main","id":"a"}}`, Message{}, "no view"},
		{`{"type":"undo","view":"main"}`, Message{}, "takes no view"},
		{`{"type":"select-view","op":{}}`, Message{}, "takes no op"},
		{`{"type":"nope"}`, Message{}, "unknown message type"},
		{`{"type":"select-view","extra":1}`, Message{}, "unknown field"},
		{`{"type":"select-view","view":3}`, Message{}, "invalid message"},
		{`{"type":"select-view"} {}`, Message{}, "more than one"},
		{`[`, Message{}, "invalid message"},
	} {
		got, err := DecodeMessage([]byte(tc.in))
		if tc.err == "" && err != nil || tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)) {
			t.Errorf("DecodeMessage(%s) error %v, want %q", tc.in, err, tc.err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("DecodeMessage(%s) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func FuzzDecodeMessage(f *testing.F) {
	for _, s := range []string{
		`{"type":"select-view","view":"main"}`, `{"type":"op"}`, `{}`, `[]`, `{"type":"select-view","view":"\u0000"}`,
		`{"type":"op","seq":1,"op":{"op":"move","view":"main","id":"a","x":40,"y":40}}`,
		`{"type":"op","op":{"op":"batch","ops":[{"op":"resize","view":"v","id":"a","w":1,"h":2},{"op":"set_edge_hint","view":"v","id":"e","hint":{"via":[[1,2]]}}]}}`,
		`{"type":"undo","seq":2}`, `{"type":"redo"}`,
		`{"type":"op","seq":3,"op":{"op":"create_node","kind":"service","view":"main","parent":"lane","before":"s2"}}`,
		`{"type":"op","op":{"op":"batch","ops":[{"op":"show_node","view":"main","id":"a"},{"op":"hide_node","view":"main","id":"b"},{"op":"set_parent","id":"a","parent":"g"},{"op":"reorder","view":"main","container":"g","order":["a"]}]}}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := DecodeMessage(data)
		if err != nil {
			return
		}
		switch {
		case !slices.Contains([]string{"select-view", "op", "undo", "redo"}, m.Type):
			t.Fatalf("accepted message of type %q", m.Type)
		case (m.Type == "op") != (m.Op != nil):
			t.Fatalf("message %q with op %v", m.Type, m.Op)
		}
	})
}
