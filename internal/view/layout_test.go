package view

import "testing"

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
