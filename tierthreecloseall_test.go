package nib

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTierThreeCloseAllRefusesWhatItCannotSee — /pending 731 (4).
//
// `harness.mjs`'s `closeAll` is how every tier-3 file leaves the SHARED nib process (/pending 474,
// `TestEveryTierThreeFileEndsThroughTheOneDoor`). Its in-page half returned 0 on ANY non-OK
// `/api/docs`, so a 403 — a page whose token is not this process's — read as "nothing to close"
// and the leak the door exists to catch passed as a clean server; and a refused `/api/close` was
// never looked at. Tier 3 is minutes long and needs a browser, so the function lives in
// `test/ui/closeall.mjs` with no imports and is run here under Node with the page's three globals
// stubbed. Skips without `node`, like `textrun_pdfjs_test.go`.
func TestTierThreeCloseAllRefusesWhatItCannotSee(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	mod, err := filepath.Abs(filepath.Join("test", "ui", "closeall.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	// Each case is the server's answers: `docs` is the /api/docs sequence (a number is a 200 with
	// that many documents, anything else a status), `close` the /api/close status. The script prints
	// one line per case: `<name> ok <count>` or `<name> threw <message>`.
	script := `
import { pathToFileURL } from 'node:url';
const { closeAllInPage } = await import(pathToFileURL(process.argv[1]).href);
globalThis.sessionStorage = { getItem: () => 'tok' };
const res = (status, body) => new Response(body === undefined ? '' : JSON.stringify(body), { status });
const cases = {
  forbidden:   { docs: [403], close: 200 },
  locked:      { docs: [401], close: 200 },
  refused:     { docs: [1, 1], close: 500 },
  clean:       { docs: [1, 1, 0, 0], close: 200 },
  empty:       { docs: [0], close: 200 },
};
for (const [name, c] of Object.entries(cases)) {
  let i = 0;
  globalThis.nibFetch = async () => {
    const d = c.docs[Math.min(i++, c.docs.length - 1)];
    return d >= 100 ? res(d) : res(200, { docs: Array(d).fill({}) });
  };
  globalThis.fetch = async () => res(c.close);
  try { console.log(name, 'ok', await closeAllInPage()); }
  catch (e) { console.log(name, 'threw', e.message); }
}
`
	out, err := exec.Command(node, "--input-type=module", "-e", script, mod).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name, rest, ok := strings.Cut(line, " "); ok {
			got[name] = rest
		}
	}
	want := map[string]string{
		// The two refusals: each must THROW, so `shutdown` fails the file loudly.
		"forbidden": "threw",
		"refused":   "threw",
		// The controls: without them a function that throws on everything passes the two above.
		"locked": "ok 0",
		"clean":  "ok 0",
		"empty":  "ok 0",
	}
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("case %s printed nothing; the script output was:\n%s", name, out)
			continue
		}
		if !strings.HasPrefix(g, w) {
			t.Errorf("case %s: closeAll %s, want %s. A refused /api/docs or /api/close read as "+
				"\"nothing left open\" hides the /pending 474 leak from the one door that exists to "+
				"catch it", name, g, w)
		}
	}
}
