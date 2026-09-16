package pdfops

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// stampTextWatermarks is the one read-change-write every TEXT stamp runs — `PLAN-ua-coverage.md`
// P01.S03. `add` places the watermarks, drawing embedded faces when `embedded` says it may.
//
// # One rewrite, not three
//
// A stamp's three changes — the watermarks, the used-glyph `/CIDSet`s of the faces it embedded
// (`dropCIDSetsOf`), and the optional-content configuration (`correctOptionalContent`) — ride one
// read and one write. As three `api`-and-tail passes they were three full parses of a document that
// may be hundreds of megabytes, on the route every save runs.
//
// # The document may already carry the face
//
// pdfcpu matches a watermark's font to an existing subset font BY NAME (`stamp.go`
// `createFontResForWM`) and then rebuilds that font's program from nib's TTF, taking glyph ids from
// the document's own ToUnicode (`font/fontDict.go` `UpdateUserfont`). Found by the slice review, and
// reproduced: a LibreOffice document's `BAAAAA+LiberationSans` is a simple TrueType font, pdfcpu
// refuses it as `corrupt fontDict`, and the bake answers 500. Worse, a subset whose glyph ids differ
// from nib's copy — a producer that renumbers — would be rebuilt with nib's glyphs at the document's
// ids, changing the document's own text without an error.
//
// So a face is drawn embedded only when every same-named font in the document maps each glyph to
// the character nib's face maps it to (`unreusableFace`) — which is true of the fonts nib itself
// wrote, so a second bake keeps embedding — and in Base-14 faces otherwise, said in the log. A
// failure on the embedded path that the check did not foresee is retried in Base-14 faces, because a
// stamp that costs the user the save is the trade P01 refused.
func stampTextWatermarks(pdf []byte, embedded bool, faces []string, add func(ctx *model.Context, embedded bool) error) ([]byte, error) {
	run := func(emb bool) ([]byte, error) {
		conf := model.NewDefaultConfiguration()
		conf.Cmd = model.ADDWATERMARKS
		// `api.AddWatermarks` sets this: identical content streams merged on read would share a stamp
		// meant for one page. Set for every stamp rather than only the all-pages one.
		conf.OptimizeDuplicateContentStreams = false
		return rewriteWithConf(pdf, conf, func(ctx *model.Context) error {
			if emb {
				if face, why := unreusableFace(ctx, faces); face != "" {
					log.Printf("stamped text: the document already carries its own %s, which pdfcpu would "+
						"rewrite (%s), so this stamp is set in Base-14 core fonts and will not embed them", face, why)
					emb = false
				}
			}
			if err := func() (err error) {
				defer fault.Catch(&err)
				return add(ctx, emb)
			}(); err != nil {
				// Text no face can bake (`stampText`) is the caller's error, not the face's: retrying it in
				// Base-14 reads the document twice and logs a face failure that did not happen.
				if emb && !errors.Is(err, ErrStampTextUnrepresentable) {
					return embeddedStampError{err}
				}
				return err
			}
			if emb {
				dropCIDSetsOf(ctx, faces)
				repairToUnicodeOf(ctx, faces)
			}
			// A stamp whose configuration could not be corrected is still a stamp.
			_ = correctOptionalContent(ctx)
			return nil
		})
	}
	out, err := run(embedded)
	var ee embeddedStampError
	if errors.As(err, &ee) {
		log.Printf("stamped text: stamping in the embedded faces failed (%v), so this stamp is set in "+
			"Base-14 core fonts and will not embed them", ee.err)
		return run(false)
	}
	return out, err
}

// embeddedStampError marks a failure of the stamp itself on the embedded path, so a document that
// cannot be read is not read twice.
type embeddedStampError struct{ err error }

func (e embeddedStampError) Error() string { return e.err.Error() }
func (e embeddedStampError) Unwrap() error { return e.err }

// unreusableFace names the first face in faces that the document already carries in a form pdfcpu
// would rewrite wrongly, and why; "" when every same-named font is one nib's face describes exactly.
func unreusableFace(ctx *model.Context, faces []string) (string, string) {
	want := map[string]bool{}
	for _, f := range faces {
		want[f] = true
	}
	for _, fo := range ctx.Optimize.FontObjects {
		if fo == nil || fo.Prefix == "" || !want[fo.FontName] {
			continue // pdfcpu reuses only a subset font of the same name
		}
		if why := fontNotNibs(ctx, fo.FontName, fo.FontDict); why != "" {
			return fo.FontName, why
		}
	}
	return "", ""
}

// fontNotNibs says why a document's font named like nib's face is not one pdfcpu can rebuild from
// nib's face without changing it, or "" when it is. It asks exactly what `UpdateUserfont` assumes:
// an Identity-H font whose ToUnicode is in the form pdfcpu parses, each glyph id in it naming the
// character nib's face gives that glyph.
func fontNotNibs(ctx *model.Context, name string, d types.Dict) string {
	if enc := d.NameEntry("Encoding"); enc == nil || *enc != "Identity-H" {
		return "it is not an Identity-H font"
	}
	sd, _, err := ctx.DereferenceStreamDict(d["ToUnicode"])
	if err != nil || sd == nil {
		return "it has no ToUnicode map"
	}
	if err := sd.Decode(); err != nil {
		return "its ToUnicode map does not decode"
	}
	pairs, err := pdfcpuToUnicode(string(sd.Content))
	if err != nil {
		return "its ToUnicode map is not in the form pdfcpu reads"
	}
	font.UserFontMetricsLock.RLock()
	ttf, ok := font.UserFontMetrics[name]
	font.UserFontMetricsLock.RUnlock()
	if !ok {
		return "nib's face is not loaded"
	}
	for gid, r := range pairs {
		if got := rune(ttf.ToUnicode[gid]); got != r {
			return fmt.Sprintf("its glyph %d is %U, and nib's face has %U there", gid, r, got)
		}
	}
	return ""
}

var errNotPdfcpuCMap = errors.New("pdfops: not a ToUnicode map in the form pdfcpu writes and reads")

// pdfcpuToUnicode reads a ToUnicode map the way pdfcpu's `usedGIDsFromCMap` does (`font/fontDict.go`)
// — blocks of `N beginbfchar`, each line `<GGGG> <UTF-16BE>`, a block under 100 entries last — and
// returns glyph id → character. As strict as pdfcpu, so a map it would refuse is refused here first,
// and unlike pdfcpu it never indexes a line it has not measured.
func pdfcpuToUnicode(cmap string) (map[uint16]rune, error) {
	i := strings.Index(cmap, "endcodespacerange")
	if i < 0 || i+len("endcodespacerange")+1 > len(cmap) {
		return nil, errNotPdfcpuCMap
	}
	sc := bufio.NewScanner(strings.NewReader(cmap[i+len("endcodespacerange")+1:]))
	next := func() string {
		if sc.Scan() {
			return sc.Text()
		}
		return ""
	}
	out := map[uint16]rune{}
	line := next()
	for {
		n, err := strconv.Atoi(strings.Split(line, " ")[0])
		if err != nil || n < 0 {
			return nil, errNotPdfcpuCMap
		}
		last := n < 100
		for j := 0; j < n; j++ {
			l := next()
			// "<GGGG> <" then hex then ">"
			if len(l) < 10 || l[0] != '<' || l[5:8] != "> <" || l[len(l)-1] != '>' {
				return nil, errNotPdfcpuCMap
			}
			g, gerr := hex.DecodeString(l[1:5])
			u, uerr := hex.DecodeString(l[8 : len(l)-1])
			if gerr != nil || uerr != nil || len(u) == 0 || len(u)%2 != 0 {
				return nil, errNotPdfcpuCMap
			}
			units := make([]uint16, len(u)/2)
			for k := range units {
				units[k] = binary.BigEndian.Uint16(u[2*k:])
			}
			rs := utf16.Decode(units)
			if len(rs) != 1 {
				return nil, errNotPdfcpuCMap // pdfcpu writes one character per glyph
			}
			out[binary.BigEndian.Uint16(g)] = rs[0]
		}
		if next() != "endbfchar" {
			return nil, errNotPdfcpuCMap
		}
		line = next()
		if line == "endcmap" {
			return out, nil
		}
		if last {
			return nil, errNotPdfcpuCMap
		}
	}
}

// repairToUnicodeOf rewrites the ToUnicode map of each subset font of faces that pdfcpu wrote in a form
// its own reader refuses — found by the P01 phase-close review, and reproduced.
//
// pdfcpu's writer (`font/fontDict.go` `bf`) declares the block after each hundredth entry one entry too
// long, so a face with more than 100 used glyphs ends in a block that promises an entry it does not
// hold — at exactly 200 glyphs, `1 beginbfchar` straight into `endbfchar`. Its reader
// (`usedGIDsFromCMap`) refuses that, and so does `fontNotNibs`, which asks what pdfcpu asks. So the
// second bake of any edit long enough to use a hundred glyphs refused nib's own font and drew in
// Base-14, failing 7.21.4.1 again. Re-counting the blocks changes no mapping, only the counts.
func repairToUnicodeOf(ctx *model.Context, faces []string) {
	want := map[string]bool{}
	for _, f := range faces {
		want[f] = true
	}
	for _, e := range ctx.XRefTable.Table {
		if e == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok || d.NameEntry("Type") == nil || *d.NameEntry("Type") != "Font" {
			continue
		}
		if st := d.NameEntry("Subtype"); st == nil || *st != "Type0" {
			continue
		}
		bf := d.NameEntry("BaseFont")
		if bf == nil {
			continue
		}
		base := *bf
		if i := strings.IndexByte(base, '+'); i == 6 {
			base = base[i+1:]
		}
		if !want[base] {
			continue
		}
		ref, isRef := d["ToUnicode"].(types.IndirectRef)
		if !isRef {
			continue
		}
		entry, found := ctx.FindTableEntryForIndRef(&ref)
		if !found {
			continue
		}
		sd, isStream := entry.Object.(types.StreamDict)
		if !isStream || sd.Decode() != nil {
			continue
		}
		if _, err := pdfcpuToUnicode(string(sd.Content)); err == nil {
			continue // already in the form pdfcpu reads
		}
		fixed, ok := recountBFChar(string(sd.Content))
		if !ok {
			continue
		}
		sd.Content = []byte(fixed)
		if sd.Encode() != nil {
			continue
		}
		entry.Object = sd
	}
}

// recountBFChar re-emits a ToUnicode map's `bfchar` entries in blocks of at most 100 with true counts,
// keeping everything before `endcodespacerange` and from `endcmap` on. It refuses (false) a map with
// anything but `bfchar` blocks between the two, since that is not pdfcpu's shape to repair.
func recountBFChar(cmap string) (string, bool) {
	const head = "endcodespacerange\n"
	i := strings.Index(cmap, head)
	j := strings.Index(cmap, "endcmap")
	if i < 0 || j < i {
		return "", false
	}
	var entries []string
	for _, l := range strings.Split(cmap[i+len(head):j], "\n") {
		switch {
		case l == "" || l == "endbfchar" || strings.HasSuffix(l, " beginbfchar"):
		case strings.HasPrefix(l, "<"):
			entries = append(entries, l)
		default:
			return "", false
		}
	}
	var b strings.Builder
	b.WriteString(cmap[:i+len(head)])
	for k := 0; k == 0 || k < len(entries); k += 100 {
		end := k + 100
		if end > len(entries) {
			end = len(entries)
		}
		fmt.Fprintf(&b, "%d beginbfchar\n", end-k)
		for _, l := range entries[k:end] {
			b.WriteString(l + "\n")
		}
		b.WriteString("endbfchar\n")
	}
	b.WriteString(cmap[j:])
	return b.String(), true
}
