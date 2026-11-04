package pdfops

import (
	"strings"
	"testing"
)

func TestAConvertersStderrIsKeptToItsTail(t *testing.T) {
	tb := &tailBuffer{max: 100}
	for i := 0; i < 10000; i++ {
		tb.Write([]byte(strings.Repeat("x", 37) + "\n"))
	}
	tb.Write([]byte("the last line\n"))
	if cap(tb.b) > 1000 {
		t.Errorf("buffer grew to %d", cap(tb.b))
	}
	got := tb.String()
	if len(got) > 100 || !strings.HasSuffix(got, "the last line\n") {
		t.Errorf("tail = %q", got)
	}
	tb.Write([]byte(strings.Repeat("y", 5000)))
	if len(tb.String()) != 100 {
		t.Errorf("one big write left %d bytes", len(tb.String()))
	}
}
