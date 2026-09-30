package testpdf

import "testing"

// TestPacketClaimsUAFailsClosed — `/pending 709` R2-11. The oracle for "nothing carries an
// identification it did not verify" read any packet its decoder rejected as "no claim", so a packet
// broken BEFORE the identification — an undeclared entity — passed every census that asked it.
func TestPacketClaimsUAFailsClosed(t *testing.T) {
	const claim = `<rdf:Description rdf:about="" xmlns:pdfuaid="` + uaNS + `"><pdfuaid:part>1</pdfuaid:part></rdf:Description>`
	const open = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`
	const shut = `</rdf:RDF></x:xmpmeta>`
	for _, c := range []struct {
		name, packet string
		want         bool
	}{
		{"a well-formed claim", open + claim + shut, true},
		{"a well-formed packet with no claim", open + shut, false},
		{"a claim after an undeclared entity", open + `<rdf:Description>&bogus;</rdf:Description>` + claim + shut, true},
		{"a broken packet naming nothing", open + `<rdf:Description>&bogus;</rdf:Description>` + shut, false},
	} {
		if got := PacketClaimsUA(c.packet); got != c.want {
			t.Errorf("%s: PacketClaimsUA = %v, want %v", c.name, got, c.want)
		}
	}
}
