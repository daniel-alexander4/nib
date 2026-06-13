// Package ots produces OpenTimestamps proofs. It submits a document's SHA-256
// digest to public OpenTimestamps calendar servers and assembles their returned
// commitments into a standard .ots proof file, which anchors the digest to the
// Bitcoin blockchain and is independently verifiable by any OpenTimestamps tool —
// with no certificate, no account, and no trust in Nib.
//
// Only the 32-byte digest ever leaves the machine, never the document.
//
// Nib produces proofs but does not verify them: verifying an .ots requires a
// source of Bitcoin block headers (a node or a block explorer), which is out of
// scope and a dependency Nib deliberately does not take. A freshly stamped proof
// is "pending" and becomes verifiable a few hours after the next Bitcoin block
// confirms it; anyone can then check it with the reference client or web tools.
//
// The .ots wire format (header, varuint version, file-hash op, digest, then a
// sequence of operations ending in an attestation) follows the OpenTimestamps
// spec; see github.com/opentimestamps. A single calendar's /digest response is
// exactly one such operation sequence, so a proof is the file preamble followed
// by each calendar's response, separated by a 0xff checkpoint byte (which resets
// the running state so the next sequence starts from the digest again).
package ots

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultCalendars are the standard public OpenTimestamps calendar servers (the
// same set the reference client uses). A stamp submits to all of them so the
// proof survives any single calendar disappearing before it is upgraded.
var DefaultCalendars = []string{
	"https://alice.btc.calendar.opentimestamps.org",
	"https://bob.btc.calendar.opentimestamps.org",
	"https://finney.btc.calendar.opentimestamps.org",
	"https://btc.calendar.catallaxy.com",
}

// .ots format constants, from the OpenTimestamps spec.
var (
	headerMagic  = []byte{0x00, 0x4f, 0x70, 0x65, 0x6e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x73, 0x00, 0x00, 0x50, 0x72, 0x6f, 0x6f, 0x66, 0x00, 0xbf, 0x89, 0xe2, 0xe8, 0x84, 0xe8, 0x92, 0x94}
	pendingMagic = []byte{0x83, 0xdf, 0xe3, 0x0d, 0x2e, 0xf9, 0x0c, 0x8e}
)

const (
	opSHA256       = 0x08 // also the file-digest hash op (digests are sha256)
	tagAttestation = 0x00
	tagCheckpoint  = 0xff

	perCalendarTimeout = 20 * time.Second
	maxResponseBytes   = 1 << 16 // a calendar commitment is small; cap defensively
)

// Stamp submits digest to each calendar concurrently and assembles the validated
// responses into an .ots proof. It needs at least one calendar to accept the
// digest; calendars that error or return a malformed commitment are skipped.
func Stamp(ctx context.Context, client *http.Client, digest [32]byte, calendars []string) ([]byte, error) {
	type result struct {
		i   int
		seq []byte
	}
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		got []result
	)
	for i, cal := range calendars {
		wg.Add(1)
		go func(i int, cal string) {
			defer wg.Done()
			seq, err := submit(ctx, client, cal, digest)
			if err != nil {
				return
			}
			mu.Lock()
			got = append(got, result{i, seq})
			mu.Unlock()
		}(i, cal)
	}
	wg.Wait()

	if len(got) == 0 {
		return nil, errors.New("no calendar server accepted the timestamp")
	}
	sort.Slice(got, func(a, b int) bool { return got[a].i < got[b].i })
	seqs := make([][]byte, len(got))
	for i, r := range got {
		seqs[i] = r.seq
	}
	return buildOTS(digest, seqs), nil
}

// submit POSTs the digest to one calendar's /digest endpoint and returns its
// commitment sequence, after confirming the response is well-formed.
func submit(ctx context.Context, client *http.Client, calendar string, digest [32]byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, perCalendarTimeout)
	defer cancel()

	url := strings.TrimRight(calendar, "/") + "/digest"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(digest[:]))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.opentimestamps.v1")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "nib")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar %s returned %d", calendar, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if err := validatePendingSequence(body); err != nil {
		return nil, fmt.Errorf("calendar %s: %w", calendar, err)
	}
	return body, nil
}

// buildOTS assembles the .ots proof: the file preamble (magic, version, sha256
// file-hash op, digest) followed by each calendar sequence. Sequences are joined
// with a 0xff checkpoint byte that resets the running state to the digest, so
// each one is evaluated independently — the same structure the reference
// serializer emits for multiple calendar attestations.
func buildOTS(digest [32]byte, sequences [][]byte) []byte {
	out := make([]byte, 0, len(headerMagic)+34+256*len(sequences))
	out = append(out, headerMagic...)
	out = append(out, 0x01) // version
	out = append(out, opSHA256)
	out = append(out, digest[:]...)
	for i, seq := range sequences {
		if i < len(sequences)-1 {
			out = append(out, tagCheckpoint)
		}
		out = append(out, seq...)
	}
	return out
}

// validatePendingSequence confirms a calendar response is exactly one well-formed
// sequence of operations terminating in a single pending (calendar) attestation,
// with no trailing bytes — so a buggy or hostile calendar cannot make Nib emit a
// corrupt or surprising proof.
func validatePendingSequence(b []byte) error {
	c := &cursor{b: b}
	for {
		tag, err := c.byte()
		if err != nil {
			return errors.New("commitment ended before an attestation")
		}
		if tag == tagAttestation {
			magic, err := c.read(len(pendingMagic))
			if err != nil || !bytes.Equal(magic, pendingMagic) {
				return errors.New("not a pending calendar attestation")
			}
			payload, err := c.varbytes()
			if err != nil {
				return err
			}
			if _, err := (&cursor{b: payload}).varbytes(); err != nil {
				return errors.New("malformed calendar reference")
			}
			if !c.atEnd() {
				return errors.New("unexpected trailing data after attestation")
			}
			return nil
		}
		switch tag {
		case 0xf0, 0xf1: // append, prepend — each take a byte-string argument
			if _, err := c.varbytes(); err != nil {
				return err
			}
		case 0x02, 0x03, 0x08, 0x67, 0xf2, 0xf3: // sha1, ripemd160, sha256, keccak256, reverse, hexlify — no argument
		default:
			return fmt.Errorf("unknown operation tag 0x%02x", tag)
		}
	}
}

// cursor reads the .ots byte encoding: bytes and LEB128-style varuints/varbytes.
type cursor struct {
	b   []byte
	pos int
}

func (c *cursor) atEnd() bool { return c.pos >= len(c.b) }

func (c *cursor) byte() (byte, error) {
	if c.pos >= len(c.b) {
		return 0, io.EOF
	}
	v := c.b[c.pos]
	c.pos++
	return v, nil
}

func (c *cursor) read(n int) ([]byte, error) {
	if n < 0 || c.pos+n > len(c.b) {
		return nil, io.ErrUnexpectedEOF
	}
	v := c.b[c.pos : c.pos+n]
	c.pos += n
	return v, nil
}

func (c *cursor) varuint() (uint64, error) {
	var value, shift uint64
	for {
		b, err := c.byte()
		if err != nil {
			return 0, err
		}
		value |= (uint64(b) & 0x7f) << shift
		if b&0x80 == 0 {
			return value, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, errors.New("varuint overflow")
		}
	}
}

func (c *cursor) varbytes() ([]byte, error) {
	n, err := c.varuint()
	if err != nil {
		return nil, err
	}
	if n > maxResponseBytes {
		return nil, errors.New("varbytes length too large")
	}
	return c.read(int(n))
}
