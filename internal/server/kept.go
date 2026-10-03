package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"nib/internal/atomicfile"
)

// A copy kept when you signed — PLAN-returned-document P04, ADR-073.
//
// Finalize & sign can write the signed output it returns to `~/nib/signed/` as well, when the user ticks "Keep a copy
// for my records" (D13: opt-in, off by default). The folder already holds two other writers' files — every signed
// arrival (`saveReceived`) and every delivered ceremony copy (`saveDelivered`), whose existence re-arms a ceremony
// (`alreadyDelivered`) — so a kept copy carries a name neither of them can produce, and **this file is the one door**
// through which any route names a kept copy (ADR-009): `keptPathFor` accepts only the grammar below and rebuilds the
// path from its parsed parts, so no client string ever reaches a path join.

// keptSubdir is the folder kept copies share with the received and delivered documents.
const keptSubdir = "signed"

// keptGrammar is version 1 of a kept copy's name: `kept_<slug>_<YYYYmmdd-HHMMSS>-<first 8 hex of sha256>.pdf`. The slug
// is `labelSlug`'s alphabet, which never holds `_` (it maps `_` to `-`), so the name parses one way and no peer label or
// ceremony intent can produce it. A later grammar takes a NEW prefix; a reader ignores names it does not parse.
var keptGrammar = regexp.MustCompile(`^kept_([a-z0-9-]{1,48})_([0-9]{8}-[0-9]{6})-([0-9a-f]{8})\.pdf$`)

// errNotAKeptName is the refusal of a name the grammar does not accept.
var errNotAKeptName = errors.New("that is not the name of a copy kept when you signed")

// keptName is the name for a copy of signed, kept at now, of the document the client called docName. docName is the
// client's text and untrusted: it only ever narrows to `labelSlug`'s alphabet, capped at 48 like a delivered name.
func keptName(docName string, signed []byte, now time.Time) string {
	// Only a `.pdf` extension is dropped: the client sends a name without one, and stripping any extension cut
	// "Contract v1.2" to "contract-v1" (the slice review).
	if strings.EqualFold(filepath.Ext(docName), ".pdf") {
		docName = strings.TrimSuffix(docName, filepath.Ext(docName))
	}
	slug := labelSlug(docName)
	if len(slug) > 48 {
		slug = strings.TrimRight(slug[:48], "-")
	}
	if slug == "" {
		slug = "document"
	}
	sum := sha256.Sum256(signed)
	return "kept_" + slug + "_" + now.Format("20060102-150405") + "-" + hex.EncodeToString(sum[:4]) + ".pdf"
}

// keptPathFor is the path of the kept copy called name, or errNotAKeptName. The path is REBUILT from the parsed parts,
// never joined from name, so nothing the grammar did not match reaches the filesystem.
func keptPathFor(name string) (string, error) {
	m := keptGrammar.FindStringSubmatch(name)
	if m == nil {
		return "", fmt.Errorf("%w: %q", errNotAKeptName, name)
	}
	return filepath.Join(defaultOutputDir(), keptSubdir, "kept_"+m[1]+"_"+m[2]+"-"+m[3]+".pdf"), nil
}

// saveKept writes signed as a kept copy and returns its name. Durable, 0600, through the same pair the folder's other
// two writers use: the copy may be the only evidence its owner has of what they signed (D14), so a crash after the
// signing answered must not leave a truncated copy the user was told exists.
func saveKept(docName string, signed []byte, now time.Time) (string, error) {
	name := keptName(docName, signed, now)
	path, err := keptPathFor(name)
	if err != nil {
		return "", err // a name this file built that its own grammar refuses — a defect, never user input
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := atomicfile.WriteDurable(path, signed, 0o600); err != nil {
		return "", err
	}
	return name, nil
}

// keptRefusal is the body of a Finalize that signed but could not keep its copy: the signing is refused (D14 — a user
// who ticked the box must never hold a signature they believe is kept and is not), and `cause` lets the modal say so
// rather than a generic failure. 500, never 422: the modal reads 422 as a wrong certificate passphrase.
type keptRefusal struct {
	Error  string `json:"error"`
	Cause  string `json:"cause"`
	Reason string `json:"reason"`
	// Full is true when the disk is full — the one cause whose advice is "free some space"; every other cause (a file
	// where the folder should be, no permission) is told to check the folder instead (the slice review).
	Full bool `json:"full,omitempty"`
}

// refuseUnkept answers a Finalize whose kept copy could not be written.
func refuseUnkept(w http.ResponseWriter, err error) {
	writeJSONStatus(w, http.StatusInternalServerError, keptRefusal{
		Error:  "the document was not signed: Nib could not keep the copy you asked for",
		Cause:  "copy-not-kept",
		Reason: err.Error(),
		Full:   errors.Is(err, syscall.ENOSPC),
	})
}

// keptEntry is one copy kept when you signed, as the list shows it: read from the NAME and a stat, never by opening the
// file — a list that parsed each copy would cost the total bytes kept every time it is shown (the plan-review's perf seat).
type keptEntry struct {
	Name     string `json:"name"`     // the file name, which is also what the remove route takes
	Document string `json:"document"` // the document's slug, as the name carries it
	KeptAt   string `json:"keptAt"`   // when the copy was written (local time, from the name), "YYYY-mm-dd HH:MM:SS"
	Size     int64  `json:"size"`
}

// keptListResponse is GET /api/kept's answer: every kept copy, newest first, and what they occupy together.
type keptListResponse struct {
	Kept       []keptEntry `json:"kept"`
	TotalBytes int64       `json:"totalBytes"`
}

// listKept reads ~/nib/signed for the names keptGrammar accepts that are regular files — a symlink, a directory, a
// staging file, a peer's received document and a delivered ceremony copy are all left out — newest first.
func listKept() ([]keptEntry, error) {
	dir := filepath.Join(defaultOutputDir(), keptSubdir)
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []keptEntry
	for _, e := range ents {
		m := keptGrammar.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		fi, err := os.Lstat(filepath.Join(dir, e.Name()))
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		at := m[2]
		out = append(out, keptEntry{Name: e.Name(), Document: m[1], Size: fi.Size(),
			KeptAt: at[0:4] + "-" + at[4:6] + "-" + at[6:8] + " " + at[9:11] + ":" + at[11:13] + ":" + at[13:15]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].KeptAt != out[j].KeptAt {
			return out[i].KeptAt > out[j].KeptAt
		}
		return out[i].Name > out[j].Name
	})
	return out, nil
}

// removeKept deletes the kept copy called name. Only through the door, only a regular file, never RemoveAll; a copy
// already gone is not a failure (a retried remove converges). It reports whether it removed anything.
func removeKept(name string) (bool, error) {
	path, err := keptPathFor(name)
	if err != nil {
		return false, err
	}
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !fi.Mode().IsRegular() {
		return false, fmt.Errorf("%s is not a kept copy Nib wrote (not a regular file), so Nib will not remove it", name)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return true, nil
}

// keptCopyFor finds the copy kept when you signed that data IS or BEGINS WITH — the one match door (ADR-009), read by the
// returned-document sheet's first link (and, from P04.S03, by the Simple Sign row). The LONGEST match wins: a copy kept
// at a later signing is the tighter baseline, and "everything after it was appended" must not include your own later
// signing (the slice review). ends are the coverage ends of data's own signatures:
// a kept copy is Finalize's output, which its last signature covers to its last byte, so it can only match at one of
// them. A copy is read only when its size equals an end AND the name's 8-hex digest equals that prefix's — and the match
// itself is byte equality, because the name's digest is an index and never evidence (ADR-073).
func keptCopyFor(data []byte, ends []int64) (keptEntry, []byte, error) {
	kept, err := listKept()
	if err != nil {
		return keptEntry{}, nil, err
	}
	bySize := map[int64][]keptEntry{}
	for _, k := range kept {
		bySize[k.Size] = append(bySize[k.Size], k)
	}
	sorted := append([]int64(nil), ends...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })
	seen := map[int64]bool{}
	for _, end := range sorted {
		if end <= 0 || end > int64(len(data)) || seen[end] || len(bySize[end]) == 0 {
			continue
		}
		seen[end] = true
		sum := sha256.Sum256(data[:end])
		digest := hex.EncodeToString(sum[:4])
		for _, k := range bySize[end] {
			if !strings.HasSuffix(k.Name, "-"+digest+".pdf") {
				continue
			}
			path, err := keptPathFor(k.Name)
			if err != nil {
				continue
			}
			b, err := os.ReadFile(path)
			if errors.Is(err, fs.ErrNotExist) {
				continue // removed since it was listed (another window's Remove): not a reason to fail the search
			}
			if err != nil {
				return keptEntry{}, nil, err
			}
			if bytes.Equal(b, data[:end]) {
				return k, b, nil
			}
		}
	}
	return keptEntry{}, nil, nil
}
