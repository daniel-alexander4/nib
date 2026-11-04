package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// listenSites counts the calls in one file that open a listener or serve one: net.Listen*, a ListenConfig's
// Listen*, http.Serve*, (*http.Server).Serve* and ListenAndServe* (PLAN-import-export R3, X4(d), ADR-200).
// It matches the selector's NAME, not its receiver, because a listener is reached through a variable as often
// as through a package: `lc.Listen(...)`, `srv.Serve(ln)`.
func listenSites(t *testing.T, name, src string) int {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	n := 0
	ast.Inspect(f, func(x ast.Node) bool {
		if c, ok := x.(*ast.CallExpr); ok {
			if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
				s := sel.Sel.Name
				if strings.HasPrefix(s, "Listen") || strings.HasPrefix(s, "ListenAndServe") || s == "Serve" || strings.HasPrefix(s, "ServeTLS") || s == "ListenPacket" {
					n++
				}
			}
		}
		return true
	})
	return n
}

func TestTheListenerCensusSeesEverySpelling(t *testing.T) {
	for name, src := range map[string]string{
		"net.Listen":          `package x; import "net"; func f() { net.Listen("tcp", ":0") }`,
		"net.ListenPacket":    `package x; import "net"; func f() { net.ListenPacket("udp", ":0") }`,
		"ListenConfig.Listen": `package x; import "net"; func f() { var lc net.ListenConfig; lc.Listen(nil, "tcp", ":0") }`,
		"ListenConfig packet": `package x; import "net"; func f() { var lc net.ListenConfig; lc.ListenPacket(nil, "udp", ":0") }`,
		"http.Serve":          `package x; import "net/http"; func f() { http.Serve(nil, nil) }`,
		"Server.Serve":        `package x; import "net/http"; func f() { s := &http.Server{}; s.Serve(nil) }`,
		"ListenAndServe":      `package x; import "net/http"; func f() { http.ListenAndServe(":0", nil) }`,
		"ListenAndServeTLS":   `package x; import "net/http"; func f() { http.ListenAndServeTLS(":0", "", "", nil) }`,
		"Server.ServeTLS":     `package x; import "net/http"; func f() { s := &http.Server{}; s.ServeTLS(nil, "", "") }`,
	} {
		if listenSites(t, "x.go", src) != 1 {
			t.Errorf("the census misses the %s spelling", name)
		}
	}
	if listenSites(t, "y.go", `package y; func f() { run(); Listener() }`) != 0 {
		t.Error("the census sees a listener in code that has none")
	}
}

// TestEveryListenerNibOpensIsDeclared — R3's census. A new file that opens or serves a listener fails until it
// is named here with why. Where a file's count is pinned a second listener in it is caught too; the p2p and session files are many-named and pinned by name only.
func TestEveryListenerNibOpensIsDeclared(t *testing.T) {
	declared := map[string]struct {
		why   string
		sites int
	}{
		"cmd/nib/main.go":                  {"the app's own server on 127.0.0.1 (Listen, the http.Server and its Serve)", 2},
		"internal/p2p/endpoint.go":         {"the signing ceremony's shared UDP endpoint (count not pinned: the p2p names are many)", 0},
		"internal/p2p/quic.go":             {"the ceremony's QUIC transport (count not pinned)", 0},
		"internal/p2p/transport.go":        {"the ceremony's TLS listener (count not pinned)", 0},
		"internal/server/session.go":       {"arms a ceremony listener through p2p (its call, not a socket of its own)", 0},
		"internal/discovery/mcast.go":      {"link-local discovery (multicast)", 1},
		"internal/portmap/upnp.go":         {"the UPnP/NAT-PMP reply socket", 1},
		"internal/cli/rendezvous.go":       {"`nib rendezvous`, a DHT node the user runs on purpose", 1},
		"internal/pdfops/htmlimport.go":    {"calls browserrender.Listen, the render origin's only caller (ADR-200)", 1},
		"internal/browserrender/origin.go": {"the render origin: 127.0.0.1, one document, one request, 256-bit path (ADR-200)", 2},
	}
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "web", "docs", "test", "third_party", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 60 {
		t.Fatalf("setup: only %d source files found — this guard is not walking the repo", len(files))
	}
	got := map[string]int{}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if n := listenSites(t, path, string(b)); n > 0 {
			got[filepath.ToSlash(path)] = n
		}
	}
	var keys []string
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d, ok := declared[k]
		if !ok {
			t.Errorf("%s opens or serves a listener (%d site(s)) and is not declared in this census — name it and say why", k, got[k])
			continue
		}
		if d.sites != 0 && got[k] != d.sites {
			t.Errorf("%s has %d listener site(s), the census pins %d (%s)", k, got[k], d.sites, d.why)
		}
	}
	for k := range declared {
		if _, ok := got[k]; !ok {
			t.Errorf("%s is declared as opening a listener and opens none; remove the entry", k)
		}
	}
}
