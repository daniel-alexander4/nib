package ceremony

import "encoding/binary"

// The one length-prefix encoder every signed preimage in this package uses.
//
// # Why this is shared rather than written per preimage
//
// It was written three times — RosterHash's `writeLP`, the candidate record's, and the
// AEAD's associated data — and the whole premise of the domain tags is that these are two
// preimage *languages* built from the same chunks. Three copies make that a claim rather
// than a fact, and they can drift apart silently: nothing would fail if one of them
// changed its count width or its byte order.
//
// The drift was not hypothetical. The roster tag added at P04.S03 was covered by NOTHING —
// deleting it left the entire repo green — precisely because `RosterHash` computes its
// preimage inline and returns only a digest, so no test could see the bytes. Factoring the
// builder out is what makes both sides inspectable: the tests assert against the BYTES this
// builder produces, not against a digest. (This used to cite a `preimageOf` helper "below",
// which has never existed anywhere in the tree — a citation nobody could follow, describing a
// mechanism that was never built.)
//
// Every chunk is prefixed with a fixed 8-byte big-endian count so no byte can migrate
// across a boundary. That matters most where the axes are variable-length and
// attacker-influenceable, which is the candidate record exactly.
type preimageBuilder struct {
	b []byte
}

// add appends one length-prefixed chunk.
func (p *preimageBuilder) add(v []byte) *preimageBuilder {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(v)))
	p.b = append(p.b, n[:]...)
	p.b = append(p.b, v...)
	return p
}

// addString is add for text axes.
func (p *preimageBuilder) addString(s string) *preimageBuilder { return p.add([]byte(s)) }

// addUint is add for a numeric axis, always 8 bytes big-endian.
//
// Fixed width on purpose: a decimal rendering would make "10" and "1"+"0" distinguishable
// only by the length prefix, and a one-byte rendering (which the AEAD's associated data
// used before this was shared) wraps silently at 256.
func (p *preimageBuilder) addUint(n uint64) *preimageBuilder {
	var v [8]byte
	binary.BigEndian.PutUint64(v[:], n)
	return p.add(v[:])
}

// bytes returns the accumulated preimage.
func (p *preimageBuilder) bytes() []byte { return p.b }
