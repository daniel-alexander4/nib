package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"nib/internal/uacheck"
)

// The accessibility report route — `PLAN-accessibility.md` P07.S06.

// TestTheUAReportRouteReachesTheDoorAndPublishesEveryVerdictAsAWord.
func TestTheUAReportRouteReachesTheDoorAndPublishesEveryVerdictAsAWord(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	resp, err := c.Get(ts.URL + "/api/uacheck")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var rep uaReportResponse
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		t.Fatal(err)
	}
	// Every registered clause is in the report, or a clause could go silently unreported.
	if len(rep.Results) != len(uacheck.Clauses()) {
		t.Fatalf("the report carries %d result(s) for %d registered clause(s)", len(rep.Results), len(uacheck.Clauses()))
	}
	words := map[string]bool{"pass": true, "fail": true, "cannot check": true, "not applicable": true, "not run": true}
	for _, r := range rep.Results {
		if !words[r.Verdict] {
			t.Errorf("clause %s travels with verdict %q, which is not one of law 4's words", r.Clause, r.Verdict)
		}
		if r.Summary == "" {
			t.Errorf("clause %s has no summary on the wire", r.Clause)
		}
	}
	// The fixture fails clauses nib checks (it has no structure tree), so the door must refuse, and say why.
	if rep.Conformant {
		t.Error("the report says a fixture with no structure tree is conformant")
	}
	if len(rep.Refusals) == 0 {
		t.Error("a non-conformant report carries no refusal, so a person is told no and not told why")
	}
	if !strings.Contains(strings.Join(rep.Refusals, "\n"), " fails: ") {
		t.Errorf("no refusal names a failing clause: %q", rep.Refusals)
	}
	// D4: the user is told where the structure came from — and a fixture nib did not tag records nothing.
	if !strings.Contains(rep.Structure, "no record") {
		t.Errorf("the report's structure line for an untagged fixture reads %q", rep.Structure)
	}
}

// TestTheUAReportRouteIsGatedLikeEveryDocumentRoute — an unauthenticated client gets no report.
func TestTheUAReportRouteIsGatedLikeEveryDocumentRoute(t *testing.T) {
	ts, _ := startServer(t)
	resp, err := http.Get(ts.URL + "/api/uacheck")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("the accessibility report answers an unauthenticated request")
	}
}
