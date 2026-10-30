package fontcode

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

// Adobe's Glyph List, and Adobe's rule for what a glyph NAME says (ADR-176, /pending 918). A simple font's
// `/Differences` renames the glyphs a code draws, and the only text such a font gives is the name — so a reader that
// wants the text asks here, and so does the checker, which reads the same list as veraPDF does. One list, two readers
// (ADR-009): `internal/uacheck` takes `GlyphList` as published and adds the one entry veraPDF adds; a text reader takes
// `GlyphText`.
//
// The list is Adobe's AGL 2.0 (`agl/glyphlist.txt`, BSD-3-Clause, vendored unmodified; `agl/VERSION` pins it).

//go:embed agl/glyphlist.txt
var glyphListText string

var (
	glyphListOnce sync.Once
	glyphList     map[string]string
)

// GlyphList is Adobe's list as published: glyph name → the text it stands for. Parsed once. It must not be changed;
// a caller that wants an entry of its own copies it (`uacheck` does, for `.notdef`).
func GlyphList() map[string]string {
	glyphListOnce.Do(func() {
		glyphList = map[string]string{}
		for _, line := range strings.Split(glyphListText, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line[0] == '#' {
				continue
			}
			name, codes, ok := strings.Cut(line, ";")
			if !ok {
				continue
			}
			var v []rune
			for _, h := range strings.Fields(codes) {
				n, err := strconv.ParseUint(h, 16, 32)
				if err != nil {
					v = nil
					break
				}
				v = append(v, rune(n))
			}
			if v != nil {
				glyphList[name] = string(v)
			}
		}
	})
	return glyphList
}

// maxGlyphNameParts bounds the components one name is split into: a name is read from a document.
const maxGlyphNameParts = 64

// GlyphText is the text a glyph name stands for, by Adobe's rule ("Unicode and Glyph Names", the AGL specification):
// drop everything from the first ".", split the rest at "_", and read each component from the list, as `uniXXXX…`
// (groups of four hex digits) or as `uXXXX`–`uXXXXXX`. A component that is none of these — `g123`, `cid00017`,
// a name the list does not carry — makes the whole name unreadable: false, never a guess. `.notdef` and an empty name
// are false.
//
// A presentation ligature stays what the list says (`fi` → U+FB01): turning it into letters is a text reader's choice,
// not this rule's, and the checker must see the character Adobe names.
func GlyphText(name string) (string, bool) {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		name = name[:i]
	}
	if name == "" {
		return "", false
	}
	parts := strings.Split(name, "_")
	if len(parts) > maxGlyphNameParts {
		return "", false
	}
	var sb strings.Builder
	for _, part := range parts {
		if s, ok := GlyphList()[part]; ok {
			sb.WriteString(s)
			continue
		}
		switch {
		case strings.HasPrefix(part, "uni") && len(part) > 3 && (len(part)-3)%4 == 0:
			for i := 3; i < len(part); i += 4 {
				if !writeHexRune(&sb, part[i:i+4]) {
					return "", false
				}
			}
		case strings.HasPrefix(part, "u") && len(part) >= 5 && len(part) <= 7:
			if !writeHexRune(&sb, part[1:]) {
				return "", false
			}
		default:
			return "", false
		}
	}
	return sb.String(), sb.Len() > 0
}

// writeHexRune writes the code point hex names, or says it names none. Adobe's rule takes upper-case hex only and
// refuses a surrogate half; a value past U+10FFFF names nothing.
func writeHexRune(sb *strings.Builder, hex string) bool {
	for i := 0; i < len(hex); i++ {
		if c := hex[i]; !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || n > 0x10FFFF || (n >= 0xD800 && n <= 0xDFFF) {
		return false
	}
	sb.WriteRune(rune(n))
	return true
}
