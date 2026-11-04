package pdfops

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"nib/internal/ziputil"
)

// ErrPackageNamesOutside: a document package that points at a file or an address outside itself — a `file:` link, a
// UNC or absolute path, a `..` that leaves the package. Nib's conversion reads what the package holds and nothing it
// names (PLAN-import-export X11(a), ADR-197).
var ErrPackageNamesOutside = errors.New("that document points at a file or address outside itself, which Nib will not follow")

// packagePartCap is the most one scanned part may expand to. A part past it cannot be scanned, so the package is
// refused: "not scanned" is never "passed".
const packagePartCap = 16 << 20

// scanPackageReferences reads a zip document package in memory and refuses one that names a location outside itself,
// before any converter sees it. It is called from `ConvertOfficeToPDF` — the one function every LibreOffice route
// reaches — so `Import`, `Combine` and a direct caller all pass it. A file that is not a zip (an OLE file, RTF, an
// iWork bundle's IWA protobuf) has nothing here to scan: that is the declared gap (ADR-197).
//
// What is allowed: a link a reader follows only when the user clicks (`http`, `https`, `mailto`, `tel`), a fragment
// (`#…`), and a path inside the package. LibreOffice's network fetches are closed separately by the dead proxy of
// officeprofile.go; this scan is the local-file half.
func scanPackageReferences(data []byte) error {
	a, err := ziputil.Open(bytes.NewReader(data), int64(len(data)), packageLimits)
	if err != nil {
		if !bytes.Contains(data[:min(len(data), 4096)], []byte("PK\x03\x04")) {
			return nil // not a zip at all (an OLE or flat-XML document): nothing to scan, the declared gap
		}
		return fmt.Errorf("%w: %v", ErrArchiveRefused, err)
	}
	names := a.Names()
	isODF := names["mimetype"] || (names["content.xml"] && names["META-INF/manifest.xml"])
	for _, m := range a.Files() {
		var visit func(*xml.Decoder) error
		switch {
		case strings.HasSuffix(strings.ToLower(m.Name), ".rels"):
			name := m.Name
			visit = func(d *xml.Decoder) error { return scanRels(d, name, names) }
		case isODF && odfScanned(m.Name):
			visit = func(d *xml.Decoder) error { return scanODF(d, names) }
		default:
			continue
		}
		b, err := a.Read(m, packagePartCap)
		if err != nil {
			return fmt.Errorf("%w: a part of the document could not be scanned", ErrArchiveRefused)
		}
		d := xml.NewDecoder(bytes.NewReader(b))
		d.Strict = true
		d.Entity = nil
		if err := visit(d); err != nil {
			return err
		}
	}
	return nil
}

// odfScanned is an OpenDocument part that can hold a link: the four document parts, at the root or in an embedded object.
func odfScanned(name string) bool {
	switch path.Base(name) {
	case "content.xml", "styles.xml", "settings.xml", "meta.xml":
		return true
	}
	return false
}

func walk(d *xml.Decoder, f func(xml.StartElement) error) error {
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: a part of the document is not well-formed", ErrCorruptDocument)
		}
		if e, ok := tok.(xml.StartElement); ok {
			if err := f(e); err != nil {
				return err
			}
		}
	}
}

func attr(e xml.StartElement, local string) (string, bool) {
	for _, a := range e.Attr {
		if a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// scanRels checks every OOXML relationship target. A target is read the way a package reader reads it: an External one
// is an address (anything but a web or mail link is a place outside the package), an internal one is a path from the
// directory of the part that owns this .rels file — `word/_rels/document.xml.rels` owns `word/`, so `../customXml/x.xml`
// from it is inside the package, as every Word file with custom XML writes it.
func scanRels(d *xml.Decoder, relsName string, names map[string]bool) error {
	owner := path.Dir(path.Dir(relsName)) // "word/_rels/x.rels" -> "word"; "_rels/.rels" -> "."
	return walk(d, func(e xml.StartElement) error {
		if e.Name.Local != "Relationship" {
			return nil
		}
		target, ok := attr(e, "Target")
		if !ok {
			return nil
		}
		mode, _ := attr(e, "TargetMode")
		return forEachForm(target, func(t string) error {
			if err := addressRef(t, target); err != nil || isAddress(t) {
				return err
			}
			if strings.EqualFold(mode, "External") {
				return refusal(target) // a relative or absolute path outside the package is a file on this machine
			}
			if strings.HasPrefix(t, "/") { // the package root: must name a member
				if c := strings.TrimPrefix(path.Clean(t), "/"); names[c] && !strings.Contains(t, "..") {
					return nil
				}
				return refusal(target)
			}
			if c := path.Join(owner, t); c == ".." || strings.HasPrefix(c, "../") {
				return refusal(target)
			}
			return nil
		})
	})
}

// scanODF checks every `xlink:href` in an OpenDocument part: a picture, an object, a section source, a table source.
// A relative reference must name a member (or a directory of members) of the package; a dangling one is read against
// the file's own location, which is a place outside it.
func scanODF(d *xml.Decoder, names map[string]bool) error {
	return walk(d, func(e xml.StartElement) error {
		for _, a := range e.Attr {
			if a.Name.Local != "href" || !strings.Contains(a.Name.Space, "xlink") {
				continue
			}
			ref := a.Value
			if err := forEachForm(ref, func(t string) error {
				if err := addressRef(t, ref); err != nil || isAddress(t) {
					return err
				}
				if strings.HasPrefix(t, "/") {
					return refusal(ref)
				}
				c := path.Clean(t)
				if c == ".." || strings.HasPrefix(c, "../") || !inPackage(names, c) {
					return refusal(ref)
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// inPackage says a cleaned relative path names a member or a directory of members.
func inPackage(names map[string]bool, c string) bool {
	if c == "." || names[c] {
		return true
	}
	for n := range names {
		if strings.HasPrefix(n, c+"/") {
			return true
		}
	}
	return false
}

// forEachForm runs check on the reference as written and, when it differs, on its percent-decoded form: a reader that
// decodes must not be handed what the scan never saw. An undecodable reference is refused.
func forEachForm(ref string, check func(string) error) error {
	t := strings.TrimSpace(ref)
	if err := check(t); err != nil {
		return err
	}
	dec, err := url.PathUnescape(t)
	if err != nil {
		return refusal(ref)
	}
	if dec != t {
		return check(dec)
	}
	return nil
}

func refusal(ref string) error {
	return fmt.Errorf("%w: it names %q", ErrPackageNamesOutside, shorten(ref))
}

// addressRef refuses the forms that are never a path inside a package: a Windows path, a UNC share, or a scheme that is
// not a web or mail link.
func addressRef(t, shown string) error {
	switch {
	case strings.ContainsAny(t, "\\\x00") || strings.HasPrefix(t, "//"):
		return refusal(shown)
	}
	if scheme, ok := schemeOf(t); ok {
		switch strings.ToLower(scheme) {
		case "http", "https", "mailto", "tel":
			return nil
		}
		return refusal(shown) // file:, smb:, ftp:, jar:, a drive letter, anything else
	}
	return nil
}

// isAddress says the reference is an allowed address or empty or a fragment: nothing further to resolve.
func isAddress(t string) bool {
	if t == "" || strings.HasPrefix(t, "#") {
		return true
	}
	_, ok := schemeOf(t)
	return ok
}

func schemeOf(t string) (string, bool) {
	if i := strings.IndexByte(t, ':'); i > 0 && !strings.ContainsAny(t[:i], "/?#") {
		return t[:i], true
	}
	return "", false
}

func shorten(s string) string {
	if r := []rune(s); len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}
