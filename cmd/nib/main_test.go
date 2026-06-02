package main

import "testing"

func TestLoopbackBind(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:0", true},
		{"127.0.0.1:8791", true},
		{"localhost:8791", true},
		{"[::1]:8791", true},
		{"0.0.0.0:8791", false},
		{":8791", false},
		{"192.168.1.10:8791", false},
		{"example.com:8791", false},
		{"127.0.0.1", false}, // missing port
		{"", false},
	}
	for _, c := range cases {
		if got := loopbackBind(c.addr); got != c.want {
			t.Errorf("loopbackBind(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}
