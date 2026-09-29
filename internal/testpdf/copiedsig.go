package testpdf

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// CopyVariant is how CopiedSignatureDictionary rewrites the copy's `/ByteRange`.
type CopyVariant int

const (
	// CopyExact keeps the victim's four numbers: the copy differs from the victim only in its object
	// number, so its gap surrounds ANOTHER object's `/Contents` (sign's conjunct 11,
	// `contents-elsewhere`). The library verifies it as a second signature by the victim.
	CopyExact CopyVariant = iota
	// CopyPastEOF appends `999999999 0` — a pair starting past the end of the file, which a last-pair
	// rule alone would let end coverage anywhere (conjunct 3, `malformed-byterange`). The library
	// verifies it too.
	CopyPastEOF
	// CopyLibraryFails shortens the first length by one, so the library's hash over the copy's ranges
	// FAILS (conjunct 7, `malformed-byterange`) — the shape that, counted as a signer, turned a
	// document whose every real signature verifies `Invalid`.
	CopyLibraryFails
)

var (
	byteRangeRe = regexp.MustCompile(`/ByteRange\s*\[([^\]]*)\]`)
	sizeRe      = regexp.MustCompile(`/Size\s+(\d+)`)
	rootRe      = regexp.MustCompile(`/Root\s+(\d+)\s+0\s+R`)
	startxrefRe = regexp.MustCompile(`startxref\s+(\d+)`)
)

// CopiedSignatureDictionary is /pending 687: signed with an incremental update appended that holds a
// NEW object carrying the first `/Adobe.PPKLite` signature dictionary's text — the victim's
// `/Contents` and `/ByteRange` — rewritten per v. It returns the document and the copy's object
// number.
//
// **Byte surgery only, and on purpose** (P01.S03): `internal/sign`'s own tests import this package,
// so it cannot import `sign`, and the fixture must reach `internal/p2p` and `internal/server`, which
// cannot import `sign`'s test helpers. The update is an uncompressed xref stream — pdfsign writes
// streams, and digitorus cannot follow a classic table whose `/Prev` names one — and the previous
// revision's `/Size`, `/Root` and `startxref` are read as the LAST occurrence of each in the file,
// which is where an incremental writer puts them.
func CopiedSignatureDictionary(signed []byte, v CopyVariant) (doc []byte, obj int, err error) {
	vi := bytes.Index(signed, []byte("/Adobe.PPKLite"))
	if vi < 0 {
		return nil, 0, errors.New("testpdf: no /Adobe.PPKLite signature dictionary")
	}
	hs := bytes.LastIndex(signed[:vi], []byte(" 0 obj"))
	if hs < 0 {
		return nil, 0, errors.New("testpdf: the signature dictionary has no object header")
	}
	oe := bytes.Index(signed[hs:], []byte("endobj"))
	if oe < 0 {
		return nil, 0, errors.New("testpdf: the signature dictionary has no endobj")
	}
	body := strings.TrimSpace(string(signed[hs+len(" 0 obj") : hs+oe]))
	m := byteRangeRe.FindStringSubmatch(body)
	if m == nil {
		return nil, 0, errors.New("testpdf: the signature dictionary has no /ByteRange")
	}
	switch v {
	case CopyExact:
	case CopyPastEOF:
		body = byteRangeRe.ReplaceAllString(body, "/ByteRange ["+m[1]+" 999999999 0]")
	case CopyLibraryFails:
		f := strings.Fields(m[1])
		if len(f) != 4 {
			return nil, 0, fmt.Errorf("testpdf: the victim's /ByteRange has %d elements, want 4", len(f))
		}
		n, err := strconv.Atoi(f[1])
		if err != nil {
			return nil, 0, fmt.Errorf("testpdf: the victim's first length: %w", err)
		}
		body = byteRangeRe.ReplaceAllString(body, fmt.Sprintf("/ByteRange [%s %d %s %s]", f[0], n-1, f[2], f[3]))
	default:
		return nil, 0, fmt.Errorf("testpdf: unknown copy variant %d", v)
	}
	size, err := lastInt(sizeRe, signed)
	if err != nil {
		return nil, 0, err
	}
	root, err := lastInt(rootRe, signed)
	if err != nil {
		return nil, 0, err
	}
	prevX, err := lastInt(startxrefRe, signed)
	if err != nil {
		return nil, 0, err
	}
	obj = size
	x := obj + 1
	var b bytes.Buffer
	b.Write(signed)
	b.WriteString("\n")
	objOff := b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", obj, body)
	xOff := b.Len()
	var data bytes.Buffer
	for _, o := range []int{objOff, xOff} {
		data.Write([]byte{1, byte(o >> 24), byte(o >> 16), byte(o >> 8), byte(o), 0, 0})
	}
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Index[%d 2]/Root %d 0 R/Prev %d/Length %d>>\nstream\n",
		x, x+1, obj, root, prevX, data.Len())
	b.Write(data.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", xOff)
	return b.Bytes(), obj, nil
}

// lastInt is re's first group at its last match in b.
func lastInt(re *regexp.Regexp, b []byte) (int, error) {
	all := re.FindAllSubmatch(b, -1)
	if len(all) == 0 {
		return 0, fmt.Errorf("testpdf: no match for %s", re)
	}
	return strconv.Atoi(string(all[len(all)-1][1]))
}
