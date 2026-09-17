package pdfops

import "errors"

// The one door onto "this failed because an optional converter is not there" (ADR-009).
//
// # Why a KIND and not a sentence
//
// Four sites classified this themselves — `internal/server/office.go`,
// `internal/server/pdfa.go`, and two in `internal/cli/commands.go` — each with its own
// `errors.Is` branch and its own literal. That produced six strings and three different
// wordings for two tools, and the sentinel's own text (the one no user ever sees, because
// every door substitutes) made a seventh and eighth.
//
// But the fix is NOT one shared sentence, and that distinction is the repo's own:
// `internal/server/handoff.go` states it where `readInstallablePDF` is consumed —
// *"ADR-009 unifies the CHECKS; it explicitly does not require every site to print the same
// sentence."* A CLI user who typed `--gs` needs to hear about `--gs`; a GUI user needs a
// link they can click; a hand-off needs to sound like an answer rather than a bug report.
// So this door returns a typed KIND and the facts that genuinely are one fact, and each site
// words it for its own audience — exactly the shape `pathRefusal{kind, status, msg}` already
// uses next door, where `openHandedOff` switches on `kind` and says it in its own voice.
//
// # The URL is here for the CLI, and never for the wire
//
// `Vendor` exists so the CLI can print a link. It must NOT be serialised into an HTTP
// response: ADR-039 refused a route that accepts a URL because it "would be a general 'fetch
// this and write it to my disk' primitive, with the request body choosing both host and
// path", and the same reasoning forbids the server choosing a NAVIGATION target. The web
// client hardcodes its own link in `web/index.html`, where CSP and the innerHTML census can
// both see it.

// MissingTool identifies an optional external converter that could not be found.
type MissingTool int

const (
	// ToolNone means the error was not a missing-converter error at all.
	ToolNone MissingTool = iota
	ToolLibreOffice
	ToolGhostscript
)

// MissingToolFor classifies err. The bool is false for every other error, so a caller reads
// it as `if tool, ok := MissingToolFor(err); ok { … }` and keeps its existing fallback for
// everything else.
//
// This is the routing ADR-009 asks for: adding a third optional tool means adding a sentinel
// and an arm HERE, and `TestEveryMissingToolRefusalGoesThroughOneDoor` fails any site that
// classifies a sentinel itself instead.
func MissingToolFor(err error) (MissingTool, bool) {
	switch {
	case errors.Is(err, ErrLibreOfficeMissing):
		return ToolLibreOffice, true
	case errors.Is(err, ErrGhostscriptMissing):
		return ToolGhostscript, true
	}
	return ToolNone, false
}

// Name is the tool's own name, as its vendor spells it.
func (t MissingTool) Name() string {
	switch t {
	case ToolLibreOffice:
		return "LibreOffice"
	case ToolGhostscript:
		return "Ghostscript"
	}
	return ""
}

// Vendor is the vendor's landing page — never a direct installer URL and never a mirror, so
// that what nib points at is the page a user would have found themselves. README already
// carries both links in prose; this is the same fact, reachable from code.
func (t MissingTool) Vendor() string {
	switch t {
	case ToolLibreOffice:
		return "https://www.libreoffice.org/"
	case ToolGhostscript:
		return "https://www.ghostscript.com/"
	}
	return ""
}

// NotFound is the shared half of what every surface says: nib looked for the tool and did not
// find it, stated as a fact about the SEARCH rather than as a verdict on the user's machine.
//
// The wording matters and is the reason this change exists. `exec.LookPath` answers "is this
// name on PATH"; the candidate lists in `toolpath.go` add the stock install locations. Both
// together still cannot see a tool installed somewhere neither knows, so "is not installed"
// is a claim nib cannot support, while "could not find" is one it can. Every site appends its
// own consequence and its own remedy to this.
func (t MissingTool) NotFound() string {
	if t == ToolNone {
		return ""
	}
	return "could not find " + t.Name()
}
