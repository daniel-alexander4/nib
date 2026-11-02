// Package ziputil is the ONE door Nib reads and writes a zip archive through (PLAN-import-export R8, D14, D32,
// ADR-187; ADR-009: a rule that holds at more than one call site is written once).
//
// Reading: an archive is held to its limits BEFORE any member is decompressed — entry count, declared and
// summed size, the expansion ratio, duplicate names and unsafe names — and a member is then read through an
// io.LimitReader over the REAL stream, because a central directory is attacker-written text and its declared
// sizes are a claim and not a fact. Writing: members go out in the order given, and a stored member (an ODF or
// EPUB `mimetype`, which a reader sniffs at byte 38) is written with CreateRaw so its size and CRC are in the
// local header.
//
// A leaf package: it imports the standard library only, so no format package reaches the archive code through
// anything that reaches a PDF.
package ziputil

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
)

// Refusals. Each is a sentence a person can act on; a caller maps them with errors.Is.
var (
	// ErrUnreadable: the central directory cannot be read (or the reader reported it insecure).
	ErrUnreadable = errors.New("the archive could not be read")
	// ErrTooManyEntries: more members than the caller's cap.
	ErrTooManyEntries = errors.New("the archive declares too many parts")
	// ErrTooLarge: members sum past the caller's decoded-bytes cap, or one member reads past its own.
	ErrTooLarge = errors.New("the archive expands too far")
	// ErrRatio: a large expansion at a ratio no honest archive has.
	ErrRatio = errors.New("the archive expands at a ratio no real document has")
	// ErrDuplicate: two members with one name — which one a reader takes depends on the reader.
	ErrDuplicate = errors.New("the archive names one part twice")
	// ErrUnsafeName: a member name that is absolute, climbs out with `..`, uses a backslash, or carries a control
	// character. Nothing is written under an archive's names, but a name a path would misread is refused whole.
	ErrUnsafeName = errors.New("the archive has a part with an unsafe name")
)

// Limits are the caps an archive is held to. A zero field is a caller's mistake, not "unlimited": Open refuses
// a Limits with any cap unset, so no door can forget one.
type Limits struct {
	// Entries is the most members an archive may declare.
	Entries int
	// DecodedBytes is the most the members may sum to, by their declared sizes, and the most any one member (or all
	// members read through Read) may produce.
	DecodedBytes uint64
	// RatioFloor and RatioMax: an archive that expands past RatioFloor bytes is refused when it expands by more
	// than RatioMax to one. A small archive cannot be a bomb whatever its ratio.
	RatioFloor uint64
	RatioMax   uint64
	// TolerateUnsafeNames lets a package through whose member names a path could misread (an old Windows tool
	// writes `word\document.xml`). It is for a package an outside converter reads by itself and Nib never
	// extracts; a caller that reads members by name leaves it false.
	TolerateUnsafeNames bool
}

func (l Limits) valid() bool {
	return l.Entries > 0 && l.DecodedBytes > 0 && l.RatioFloor > 0 && l.RatioMax > 0
}

// Archive is an opened archive that passed Limits. Reading members is through Read.
type Archive struct {
	zr    *zip.Reader
	lim   Limits
	spent uint64 // bytes produced by Read so far, against lim.DecodedBytes
}

// Open reads r's central directory and holds the archive to lim. No member is decompressed.
//
// An error from zip.NewReader is a refusal EVEN WITH a non-nil reader: since Go 1.20 an insecure member name
// returns the reader and zip.ErrInsecurePath together, and a caller that tests only for a nil reader walks an
// archive the standard library has already called unsafe.
func Open(r io.ReaderAt, size int64, lim Limits) (*Archive, error) {
	if !lim.valid() {
		return nil, errors.New("ziputil: Open needs every limit set")
	}
	zr, err := zip.NewReader(r, size)
	if errors.Is(err, zip.ErrInsecurePath) && zr != nil && lim.TolerateUnsafeNames {
		err = nil
	}
	if err != nil || zr == nil {
		if errors.Is(err, zip.ErrInsecurePath) {
			return nil, fmt.Errorf("%w: %v", ErrUnsafeName, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	if len(zr.File) > lim.Entries {
		return nil, fmt.Errorf("%w: %d parts, and the cap is %d", ErrTooManyEntries, len(zr.File), lim.Entries)
	}
	var total, packed uint64
	seen := make(map[string]struct{}, len(zr.File))
	for _, m := range zr.File {
		if !lim.TolerateUnsafeNames && !SafeName(m.Name) {
			return nil, fmt.Errorf("%w: %q", ErrUnsafeName, printable(m.Name))
		}
		if _, dup := seen[m.Name]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicate, printable(m.Name))
		}
		seen[m.Name] = struct{}{}
		total += m.UncompressedSize64
		packed += m.CompressedSize64
		if total > lim.DecodedBytes || total < m.UncompressedSize64 {
			return nil, fmt.Errorf("%w: more than %d MiB", ErrTooLarge, lim.DecodedBytes>>20)
		}
	}
	if total > lim.RatioFloor && (packed == 0 || total/packed > lim.RatioMax) {
		return nil, fmt.Errorf("%w: %d to one", ErrRatio, total/max(packed, 1))
	}
	return &Archive{zr: zr, lim: lim}, nil
}

// Files are the members, in directory order. Directories are included; IsDir says which.
func (a *Archive) Files() []*zip.File { return a.zr.File }

// Names is the set of member names.
func (a *Archive) Names() map[string]bool {
	m := make(map[string]bool, len(a.zr.File))
	for _, f := range a.zr.File {
		m[f.Name] = true
	}
	return m
}

// Read returns a member's bytes, at most max of them. It reads through an io.LimitReader over the real stream:
// a member whose directory entry says 10 bytes and whose stream is a gigabyte is refused at max+1, not trusted.
// The bytes read over every call count against Limits.DecodedBytes, so a thousand members each under max cannot
// add up to a bomb.
func (a *Archive) Read(f *zip.File, max int64) ([]byte, error) {
	if max <= 0 {
		return nil, errors.New("ziputil: Read needs a positive max")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%w: %q holds more than %d bytes", ErrTooLarge, printable(f.Name), max)
	}
	a.spent += uint64(len(b))
	if a.spent > a.lim.DecodedBytes {
		return nil, fmt.Errorf("%w: more than %d MiB read", ErrTooLarge, a.lim.DecodedBytes>>20)
	}
	return b, nil
}

// SafeName reports whether a member name is one a path could not misread: not empty, not absolute, no `..`
// element, no backslash, no NUL or other control character, no drive letter.
func SafeName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsRune(name, '\\') {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	if len(name) >= 2 && name[1] == ':' {
		return false
	}
	for _, el := range strings.Split(name, "/") {
		if el == ".." {
			return false
		}
	}
	return true
}

// printable makes a member name safe to put in a sentence a window will show.
func printable(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			b.WriteRune('?')
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() > 80 {
		return b.String()[:80] + "…"
	}
	return b.String()
}

// Entry is one member to write. Store writes it uncompressed with its size and CRC in the local header, which an
// ODF or EPUB `mimetype` needs; every other member is deflated.
type Entry struct {
	Name  string
	Data  []byte
	Store bool
}

// Write builds an archive from entries, in the order given (a stored `mimetype` first is the caller's order, and
// the position of byte 38 follows from it). Names must be safe and unique.
func Write(entries []Entry) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !SafeName(e.Name) {
			return nil, fmt.Errorf("%w: %q", ErrUnsafeName, printable(e.Name))
		}
		if seen[e.Name] {
			return nil, fmt.Errorf("%w: %q", ErrDuplicate, printable(e.Name))
		}
		seen[e.Name] = true
		var w io.Writer
		var err error
		if e.Store {
			// CreateRaw, not Create: Create streams with a data-descriptor flag that zeroes the local-header size,
			// which strict readers (LibreOffice) reject for `mimetype`.
			w, err = zw.CreateRaw(&zip.FileHeader{
				Name:               e.Name,
				Method:             zip.Store,
				CRC32:              crc32.ChecksumIEEE(e.Data),
				CompressedSize64:   uint64(len(e.Data)),
				UncompressedSize64: uint64(len(e.Data)),
			})
		} else {
			w, err = zw.Create(e.Name)
		}
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(e.Data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
