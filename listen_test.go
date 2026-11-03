package main

import "testing"

func TestTCPLocalPort(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantOK  bool
	}{
		{"0.0.0.0:8080", "8080", true},
		{"127.0.0.1:8080", "8080", true},
		{"[::]:8080", "8080", true},
		{"[::1]:8080", "8080", true},
		{"192.168.1.10:18080", "18080", true},
		{":8080", "8080", true},
		{"", "", false},
		{"not-an-addr", "", false},
	}
	for _, tt := range tests {
		got, ok := tcpLocalPort(tt.in)
		if ok != tt.wantOK || got != tt.want {
			t.Fatalf("tcpLocalPort(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}

	// Critical: :18080 must not be treated as :8080.
	p, ok := tcpLocalPort("10.0.0.1:18080")
	if !ok || p == "8080" {
		t.Fatalf("10.0.0.1:18080 incorrectly matched as 8080: port=%q ok=%v", p, ok)
	}
	if p != "18080" {
		t.Fatalf("got port %q, want 18080", p)
	}
}
