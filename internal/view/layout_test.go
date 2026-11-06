package view

import (
	"strings"
	"testing"
)

func TestDecodeID(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"api", "api", true},
		{"system%3Acontroller", "system:controller", true},
		{"a%3ab%23c%25d", "a:b#c%d", true},
		{"kube-system/coredns", "kube-system/coredns", true},
		{"a:b", "", false},
		{"a#2", "", false},
		{"50%", "", false},
		{"%41", "", false},
	}
	for _, tt := range tests {
		got, err := DecodeID(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("DecodeID(%q) = %q, %v; want %q, ok %v", tt.in, got, err, tt.want, tt.ok)
		}
		if tt.ok && EncodeID(got) != EncodeID(tt.want) {
			t.Errorf("EncodeID(%q) = %q", got, EncodeID(got))
		}
	}
}

func TestDecodeLayoutTooDeep(t *testing.T) {
	tests := []struct {
		name, src string
		want      bool
	}{
		{"brackets", "version: 1\nplace: " + strings.Repeat("[", 101), true},
		{"indentation", "version: 1\n" + strings.Repeat(" ", 201) + "a: 1\n", true},
		{"within the limit", "version: 1\nplace:\n  a: {x: 0, y: 0}\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, d := DecodeLayout([]byte(tt.src), "main.layout.yaml")
			if got := strings.Contains(d.Error(), "nests past"); got != tt.want {
				t.Errorf("diagnostics: %v", d)
			}
		})
	}
}
