package verify

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"io"
	"math"

	"github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/revocation"
	"github.com/digitorus/pkcs7"
	"github.com/digitorus/timestamp"
)

// processSignature processes a single digital signature found in the PDF.
func processSignature(v pdf.Value, file io.ReaderAt, size int64, options *VerifyOptions) (Signer, string, error) {
	signer := Signer{
		Name:        v.Key("Name").Text(),
		Reason:      v.Key("Reason").Text(),
		Location:    v.Key("Location").Text(),
		ContactInfo: v.Key("ContactInfo").Text(),
	}

	// Parse signature time if available from the signature object
	sigTime := v.Key("M")
	if !sigTime.IsNull() {
		if t, err := parseDate(sigTime.Text()); err == nil {
			signer.SignatureTime = &t
		}
	}

	// Parse PKCS#7 signature
	p7, err := pkcs7.Parse([]byte(v.Key("Contents").RawString()))
	if err != nil {
		return signer, "", fmt.Errorf("failed to parse PKCS#7: %v", err)
	}

	// Process byte range for signature verification
	err = processByteRange(v, file, size, p7)
	if err != nil {
		return signer, fmt.Sprintf("Failed to process ByteRange: %v", err), nil
	}

	// Process timestamp if present
	err = processTimestamp(p7, &signer)
	if err != nil {
		return signer, fmt.Sprintf("Failed to process timestamp: %v", err), nil
	}

	// Verify the digital signature
	err = verifySignature(p7, &signer)
	if err != nil {
		return signer, fmt.Sprintf("Failed to verify signature: %v", err), nil
	}

	// Process certificate chains and revocation
	var revInfo revocation.InfoArchival
	_ = p7.UnmarshalSignedAttribute(asn1.ObjectIdentifier{1, 2, 840, 113583, 1, 1, 8}, &revInfo)

	certError, err := buildCertificateChainsWithOptions(p7, &signer, revInfo, options)
	if err != nil {
		return signer, fmt.Sprintf("Failed to build certificate chains: %v", err), nil
	}

	return signer, certError, nil
}

// processByteRange processes the byte range for signature verification.
//
// MODIFIED BY NIB (NOTICE.nib): upstream read every pair with io.ReadAll into a buffer of its own
// and then appended that buffer to p7.Content, so one signature allocated about six times the bytes
// it covers. The pairs are now read straight into p7.Content, grown ONCE to the bytes the pairs can
// yield in a file of `size` bytes. What is read, in what order, and every error are upstream's: each
// pair still goes through io.NewSectionReader and is read to its end, and the size is only a
// capacity — a wrong one costs an allocation and changes no byte.
func processByteRange(v pdf.Value, file io.ReaderAt, size int64, p7 *pkcs7.PKCS7) error {
	br := v.Key("ByteRange")
	n := br.Len()

	// What the pairs can yield: a pair starting inside the file reads to its length or to the end
	// of the file, whichever comes first (a negative length reads to the end, as the section reader
	// has it), and any other pair reads nothing.
	var want int64
	for i := 1; i <= n; i += 2 {
		off, l := br.Index(i-1).Int64(), br.Index(i).Int64()
		if off < 0 || off >= size {
			continue
		}
		rest := size - off
		if l >= 0 && l < rest {
			rest = l
		}
		if want += rest; want < 0 || want > math.MaxInt-int64(len(p7.Content)) {
			want = 0
			break
		}
	}
	buf := p7.Content
	if want > int64(cap(buf)-len(buf)) {
		buf = make([]byte, len(p7.Content), int64(len(p7.Content))+want)
		copy(buf, p7.Content)
	}

	for i := 0; i < n; i++ {
		// As the byte range comes in pairs, we increment one extra
		i++

		// Read the byte range from the raw file and add it to the contents.
		// This content will be hashed with the corresponding algorithm to
		// verify the signature.
		sec := io.NewSectionReader(file, br.Index(i-1).Int64(), br.Index(i).Int64())
		for {
			if len(buf) == cap(buf) {
				// Full. A section that has ended says so to a read of no bytes, and the last
				// pair always ends here when the capacity was right; only where it was short
				// (it never is for a file of `size` bytes) does the buffer grow, as io.ReadAll
				// grows its own.
				if _, err := sec.Read(nil); err == io.EOF {
					break
				}
				buf = append(buf, 0)[:len(buf)]
			}
			m, err := sec.Read(buf[len(buf):cap(buf)])
			buf = buf[:len(buf)+m]
			if err == io.EOF {
				break
			}
			if err != nil {
				p7.Content = buf
				return fmt.Errorf("failed to read byte range %d: %v", i, err)
			}
		}
		p7.Content = buf
	}
	return nil
}

// processTimestamp processes timestamp information from the signature.
func processTimestamp(p7 *pkcs7.PKCS7, signer *Signer) error {
	for _, s := range p7.Signers {
		// Timestamp - RFC 3161 id-aa-timeStampToken
		for _, attr := range s.UnauthenticatedAttributes {
			if attr.Type.Equal(asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}) {
				ts, err := timestamp.Parse(attr.Value.Bytes)
				if err != nil {
					return fmt.Errorf("failed to parse timestamp: %v", err)
				}

				signer.TimeStamp = ts

				// Verify timestamp hash
				r := bytes.NewReader(s.EncryptedDigest)
				h := signer.TimeStamp.HashAlgorithm.New()
				b := make([]byte, h.Size())
				for {
					n, err := r.Read(b)
					if err == io.EOF {
						break
					}
					h.Write(b[:n])
				}

				if !bytes.Equal(h.Sum(nil), signer.TimeStamp.HashedMessage) {
					return fmt.Errorf("timestamp hash does not match")
				}
				break
			}
		}
	}
	return nil
}

// verifySignature verifies the digital signature.
func verifySignature(p7 *pkcs7.PKCS7, signer *Signer) error {
	// Directory of certificates, including OCSP
	certPool := x509.NewCertPool()
	for _, cert := range p7.Certificates {
		certPool.AddCert(cert)
	}

	// Verify the digital signature of the pdf file.
	err := p7.VerifyWithChain(certPool)
	if err != nil {
		err = p7.Verify()
		if err == nil {
			signer.ValidSignature = true
			signer.TrustedIssuer = false
		} else {
			return fmt.Errorf("signature verification failed: %v", err)
		}
	} else {
		signer.ValidSignature = true
		signer.TrustedIssuer = true
	}

	return nil
}
