package cli

import (
	"strings"

	"nib/internal/pdfops"
)

// Windows file association. Nib ships as one portable .exe with no installer, so
// nothing ever told Windows the program exists — a PDF in Explorer had no way to
// reach it except dragging the file onto the binary. `nib register` writes the
// user-scoped registry entries that put Nib in the "Open with" list for PDFs and every format it converts; `nib
// unregister` takes them out again.
//
// It cannot make Nib the DEFAULT handler, and no program can: since Windows 8
// the default for an extension lives in a UserChoice key stamped with a per-user
// hash, and writing it from outside the Settings UI is ignored. Registering is
// the supported half — after it, the user picks Nib once from "Open with" and
// ticks "Always use this app" if they want it to stick.
//
// Everything goes under HKCU, so no administrator rights are needed and an
// unregister is a clean removal of exactly what was added.

// winBase is filepath.Base for a Windows path, spelled out rather than borrowed.
// These functions compose Windows registry values but are compiled and tested on
// every platform, and filepath.Base does not treat "\" as a separator off
// Windows — so borrowing it would return the entire path as the "base name" in
// exactly the tests written to check the base name.
func winBase(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	if i := strings.IndexByte(p, ':'); i >= 0 { // drive-relative, e.g. C:nib.exe
		return p[i+1:]
	}
	return p
}

// regEntry is one value under HKCU\Software\Classes. An empty Name means the
// key's default value; an empty Value means the name is a marker whose presence
// is the whole signal (that is how OpenWithProgids and SupportedTypes work).
type regEntry struct {
	Path  string
	Name  string
	Value string
}

// progID is one file type Nib registers: a ProgId, the name Explorer shows for the type when Nib is its
// default, and the extensions it covers (with their dot).
type progID struct {
	ID    string
	Label string
	Exts  []string
}

// progIDs is PDF plus one ProgId per row of the format table (ADR-215), so every type Nib opens is in
// Explorer's "Open with" list. One per row, never the PDF one shared: a ProgId names the type, and a
// .docx a user then sets to open with Nib would be shown as "PDF Document".
func progIDs() []progID {
	out := []progID{{ID: `Nib.Document`, Label: "PDF Document", Exts: []string{".pdf"}}}
	for _, f := range pdfops.Formats() {
		p := progID{ID: `Nib.` + f.Key, Label: f.Label}
		for _, e := range f.Exts {
			p.Exts = append(p.Exts, "."+e)
		}
		out = append(out, p)
	}
	return out
}

// associationEntries returns the registry values that register exe for PDFs and every format Nib
// converts. It is kept separate from the code that writes them so the layout can
// be table-tested from any platform — the same reason assetURL takes goos as a
// parameter rather than reading runtime.GOOS.
//
// exe must be an absolute path. Its base name matters: the published binaries
// are named nib-<version>-windows-amd64.exe, and the Applications key has to
// match whatever the user actually runs, not a hardcoded "nib.exe".
func associationEntries(exe string) []regEntry {
	// The %1 is quoted separately from the program path so a document whose name
	// contains spaces arrives as one argument.
	command := `"` + exe + `" "%1"`
	app := `Applications\` + winBase(exe)
	out := []regEntry{{app + `\shell\open\command`, "", command}}
	for _, p := range progIDs() {
		out = append(out, regEntry{p.ID, "", p.Label}, regEntry{p.ID + `\shell\open\command`, "", command})
		for _, e := range p.Exts {
			// Presence of the extension here is what puts the app in the Open-with
			// list for that type; the value itself is unused.
			out = append(out, regEntry{app + `\SupportedTypes`, e, ""})
		}
	}
	for _, v := range sharedValues() {
		out = append(out, regEntry{v.Path, v.Name, ""})
	}
	return out
}

// sharedValues are Nib's values inside keys it does not own: `<ext>\OpenWithProgids` belongs to the
// extension and carries every other application's registration, so unregister deletes these VALUES and
// never the keys.
func sharedValues() []regEntry {
	var out []regEntry
	for _, p := range progIDs() {
		for _, e := range p.Exts {
			out = append(out, regEntry{e + `\OpenWithProgids`, p.ID, ""})
		}
	}
	return out
}

// ownedKeys returns the keys `nib unregister` may delete outright, deepest
// first so each is empty by the time it is removed.
//
// <ext>\OpenWithProgids is deliberately NOT in this list: that key is shared with
// every other application for the type, so unregistering removes only our
// value from it (sharedValues). Deleting the key would silently deregister the user's other
// readers.
func ownedKeys(exe string) []string {
	app := `Applications\` + winBase(exe)
	var out []string
	for _, p := range progIDs() {
		out = append(out, p.ID+`\shell\open\command`, p.ID+`\shell\open`, p.ID+`\shell`, p.ID)
	}
	return append(out,
		app+`\shell\open\command`,
		app+`\shell\open`,
		app+`\shell`,
		app+`\SupportedTypes`,
		app,
	)
}
