package server

import (
	"os"
	"strings"
	"testing"
)

// Shutdown ends the converters first and the directories after: removing a directory under a converter that is
// still writing would turn a clean teardown into a half-written one. KillConverters refuses every later
// converter in the process, so the behaviour is held in tempdir's own test and the order here, by source.
func TestShutdownKillsConvertersThenClosesEveryTemporaryDirectory(t *testing.T) {
	b, err := os.ReadFile("shutdown.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	kill, closeAll := strings.Index(s, "pdfops.KillConverters()"), strings.Index(s, "tempdir.CloseAll()")
	if kill < 0 || closeAll < 0 || kill > closeAll {
		t.Fatalf("Shutdown must call KillConverters (%d) before tempdir.CloseAll (%d)", kill, closeAll)
	}
}
