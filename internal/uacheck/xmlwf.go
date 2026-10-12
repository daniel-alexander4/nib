package uacheck

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// What Go's `encoding/xml` leaves unchecked and veraPDF's XML parser refuses — the helpers `dynamicRenderIn`
// (ua1 7.15 t1) calls beside the decoder (`/pending 673`).
//
// Nothing here was read from a parser's source. Each rule is the XML 1.0 or Namespaces constraint Go's decoder
// omits, and each was put to veraPDF 1.30.2 as a config packet first: the rows are
// `TestTheXFAReaderRefusesWhatVeraPDFsParserRefuses`, with veraPDF's answer beside every one.

// xmlPacketEncoding is how a packet's bytes become the UTF-8 text the decoder reads.
type xmlPacketEncoding int

const (
	// xmlReadable: the text is UTF-8 and may be decoded.
	xmlReadable xmlPacketEncoding = iota
	// xmlNotADocument: veraPDF's parser refuses the packet — measured — so no value is read from it.
	xmlNotADocument
	// xmlUnknown: an encoding or a shape nib has not measured; the caller refuses.
	xmlUnknown
)

// xmlDeclaration is the XML declaration exactly as XML 1.0 production 23 writes it: version, then encoding,
// then standalone, whitespace between them, nothing else. Go's decoder reads it as a processing instruction
// and looks pseudo-attributes up by substring, so it accepts one with no version, in any order, with
// `standalone="maybe"` — each measured as refused by veraPDF.
var xmlDeclaration = regexp.MustCompile(`^<\?xml[ \t\r\n]+version[ \t\r\n]*=[ \t\r\n]*(?:"([^"]*)"|'([^']*)')` +
	`(?:[ \t\r\n]+encoding[ \t\r\n]*=[ \t\r\n]*(?:"([^"]*)"|'([^']*)'))?` +
	`(?:[ \t\r\n]+standalone[ \t\r\n]*=[ \t\r\n]*(?:"(?:yes|no)"|'(?:yes|no)'))?[ \t\r\n]*\?>`)

// xmlEncName is production 81: a declared encoding's name.
var xmlEncName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]*$`)

// xmlPacketText returns the packet as the UTF-8 text veraPDF's parser reads, the encoding its declaration
// names in lower case, and whether that is a document at all.
//
// Measured: a UTF-8 byte-order mark is skipped; UTF-16 is read with a byte-order mark, or without one when the
// packet opens with `<?` in UTF-16 — and then only under no declared encoding, `UTF-16`, or the name of the
// byte order the bytes are in. An odd byte, an unpaired surrogate, or a declared `UTF-8` or `ISO-8859-1`
// over UTF-16 bytes is refused. ISO-8859-1 and US-ASCII are read under the labels listed, and a byte above
// 0x7F in a packet declared US-ASCII is refused.
func xmlPacketText(packet []byte) ([]byte, string, xmlPacketEncoding) {
	wide, bigEndian, bom := false, false, false
	switch {
	case bytes.HasPrefix(packet, []byte("\xef\xbb\xbf")):
		packet, bom = packet[3:], true
	case bytes.HasPrefix(packet, []byte("\xff\xfe")):
		packet, wide, bom = packet[2:], true, true
	case bytes.HasPrefix(packet, []byte("\xfe\xff")):
		packet, wide, bigEndian, bom = packet[2:], true, true, true
	case bytes.HasPrefix(packet, []byte("<\x00?\x00")):
		wide = true
	case bytes.HasPrefix(packet, []byte("\x00<\x00?")):
		wide, bigEndian = true, true
	}
	if wide {
		if len(packet)%2 != 0 {
			return nil, "", xmlNotADocument
		}
		units := make([]uint16, len(packet)/2)
		for i := range units {
			if bigEndian {
				units[i] = uint16(packet[2*i])<<8 | uint16(packet[2*i+1])
			} else {
				units[i] = uint16(packet[2*i+1])<<8 | uint16(packet[2*i])
			}
		}
		text := make([]byte, 0, len(packet))
		for i := 0; i < len(units); i++ {
			r := rune(units[i])
			if utf16.IsSurrogate(r) {
				if i+1 >= len(units) {
					return nil, "", xmlNotADocument
				}
				if r = utf16.DecodeRune(r, rune(units[i+1])); r == utf8.RuneError {
					return nil, "", xmlNotADocument // an unpaired surrogate
				}
				i++
			}
			text = utf8.AppendRune(text, r)
		}
		packet = text
	}
	declared := ""
	if bytes.HasPrefix(packet, []byte("<?xml")) && len(packet) > 5 && (isXMLSpace(packet[5]) || packet[5] == '?') {
		m := xmlDeclaration.FindSubmatch(packet)
		if m == nil {
			return nil, "", xmlNotADocument
		}
		// "1.1" is left for the decoder, which refuses it by name; any other version veraPDF's parser refuses.
		if v := string(m[1]) + string(m[2]); v != "1.0" && v != "1.1" {
			return nil, "", xmlNotADocument
		}
		if enc := string(m[3]) + string(m[4]); enc != "" || bytes.Contains(m[0], []byte("encoding")) {
			if !xmlEncName.MatchString(enc) {
				return nil, "", xmlNotADocument
			}
			declared = strings.ToLower(enc)
		}
	}
	family := ""
	switch declared {
	case "", "utf-8", "utf8":
		family = "utf-8"
	case "utf-16":
		family = "utf-16"
	case "utf-16le":
		family = "utf-16le"
	case "utf-16be":
		family = "utf-16be"
	case "iso-8859-1", "iso_8859-1", "iso8859_1", "latin1", "l1", "cp819":
		family = "latin1"
	case "us-ascii", "ascii":
		family = "ascii"
	default:
		return nil, declared, xmlUnknown
	}
	if wide {
		own := "utf-16le"
		if bigEndian {
			own = "utf-16be"
		}
		if declared == "" || family == "utf-16" || family == own {
			return packet, declared, xmlReadable
		}
		return nil, declared, xmlNotADocument
	}
	high := false
	for _, c := range packet {
		if c >= 0x80 {
			high = true
			break
		}
	}
	switch family {
	case "utf-8":
		return packet, declared, xmlReadable
	case "latin1", "ascii":
		if !high {
			return packet, declared, xmlReadable
		}
		if bom {
			return nil, declared, xmlUnknown // a UTF-8 byte-order mark over another encoding's bytes: unmeasured
		}
		if family == "ascii" {
			return nil, declared, xmlNotADocument
		}
		text := make([]byte, 0, len(packet)+len(packet)/8)
		for _, c := range packet {
			text = utf8.AppendRune(text, rune(c))
		}
		return text, declared, xmlReadable
	}
	// UTF-16 declared over bytes that are not UTF-16: veraPDF refuses it, and nib's reader has always said it
	// cannot read the packet — a refusal, which is never the wrong verdict.
	return nil, declared, xmlUnknown
}

func isXMLSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

// isXMLChar is XML 1.0 production 2.
func isXMLChar(r rune) bool {
	return r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) || (r >= 0x10000 && r <= 0x10FFFF)
}

// xmlCharsOnly reports whether b is valid UTF-8 holding only XML characters. Go's decoder checks this of
// character data and attribute values and NOT of a comment or a processing instruction.
func xmlCharsOnly(b []byte) bool {
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if (r == utf8.RuneError && size <= 1) || !isXMLChar(r) {
			return false
		}
		b = b[size:]
	}
	return true
}

// xmlCharRefsLegal reports whether every character reference in raw — markup as written, not a CDATA section
// — names an XML character. Go's decoder turns `&#xD800;` into U+FFFD and reads on.
func xmlCharRefsLegal(raw []byte) bool {
	for {
		i := bytes.Index(raw, []byte("&#"))
		if i < 0 {
			return true
		}
		raw = raw[i+2:]
		end := bytes.IndexByte(raw, ';')
		if end < 0 {
			return true
		}
		digits, base := string(raw[:end]), 10
		if strings.HasPrefix(digits, "x") {
			digits, base = digits[1:], 16
		}
		if n, err := strconv.ParseUint(digits, base, 32); err == nil && !isXMLChar(rune(n)) {
			return false
		}
		raw = raw[end:]
	}
}

// xmlAttributesSpaced reports whether a start tag, as written, has white space before every attribute
// (production 40). Go's decoder reads `<p a="1"b="2">` as two attributes.
func xmlAttributesSpaced(tag []byte) bool {
	i := 1
	for i < len(tag) && !isXMLSpace(tag[i]) && tag[i] != '/' && tag[i] != '>' {
		i++
	}
	for {
		spaced := false
		for i < len(tag) && isXMLSpace(tag[i]) {
			i, spaced = i+1, true
		}
		if i >= len(tag) || tag[i] == '/' || tag[i] == '>' {
			return true
		}
		if !spaced {
			return false
		}
		for i < len(tag) && tag[i] != '=' {
			i++
		}
		i++
		for i < len(tag) && isXMLSpace(tag[i]) {
			i++
		}
		if i >= len(tag) {
			return true
		}
		closing := bytes.IndexByte(tag[i+1:], tag[i])
		if closing < 0 {
			return true
		}
		i += closing + 2
	}
}

// xmlQNameRefused reports a name veraPDF's parser refuses for its colons: one that ENDS in a colon. A name
// that is only a colon, or begins with one, it reads — measured — and one with two colons Go already refuses.
func xmlQNameRefused(raw string) bool {
	return len(raw) > 1 && strings.HasSuffix(raw, ":")
}

const (
	xmlNamespaceURI   = "http://www.w3.org/XML/1998/namespace"
	xmlnsNamespaceURI = "http://www.w3.org/2000/xmlns/"
)

// xmlBindingRefused reports a namespace declaration Namespaces in XML forbids (its "Reserved Prefixes and
// Namespace Names" constraint): the prefix `xmlns` declared at all, `xml` bound to anything but its own name,
// and any other prefix — or the default, prefix "" — bound to either reserved name.
func xmlBindingRefused(prefix, uri string) bool {
	switch prefix {
	case "xmlns":
		return true
	case "xml":
		return uri != xmlNamespaceURI
	}
	return uri == xmlNamespaceURI || uri == xmlnsNamespaceURI
}
