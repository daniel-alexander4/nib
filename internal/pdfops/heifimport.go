package pdfops

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"nib/internal/tempdir"
)

// HEIC and HEIF in (PLAN-import-export P04.S02b, D2, W2, ADR-199).
//
// A phone's picture is an HEVC (or AV1) bitstream in an ISO base-media container. Go has no decoder and Nib
// carries none, so the picture is turned into a PNG by a tool the user has and the PNG goes through the same
// door as any other picture (ImageToDocumentCtx), which holds it to the pixel caps. Three layers:
//
//  1. **The tool is asked, once per binary, whether it can read HEIC at all** (`heifProbe`). Many
//     ImageMagick builds cannot, and `magick` on PATH is no evidence either way. The probe converts a 1 KB
//     HEIC Nib carries (heifprobe.heic) and requires a 64×48 PNG back. A tool that fails it is skipped for
//     the next one; none that passes is a named refusal, not "could not convert".
//  2. **The file's own declared sizes are read first** (`heifDeclaredSizes`): the container says how big each
//     picture is before any decoder runs, and a picture over the pixel cap is refused without starting one.
//  3. **The tool runs with nothing to reach.** libheif's tools take one input and write one output; ImageMagick
//     is given the format by prefix (`heic:`) so a file's name or contents never choose another coder, and a
//     policy.xml of Nib's own (MAGICK_CONFIGURE_PATH) allows the HEIC and PNG coders and nothing else —
//     no delegates, no `@` paths, no MVG, SVG or URL coder.
//
// The order is heif-dec, heif-convert, magick (the plan's D2): libheif's own tools do exactly one job.

// Refusals named by the HEIC door.
var (
	// ErrHEIFMissing: no HEIF decoder was found. Classified by MissingToolFor.
	ErrHEIFMissing = errors.New("no HEIF decoder (heif-dec, heif-convert or magick) is installed")
	// ErrHEIFCannotRead: a decoder is installed and cannot read a HEIC picture.
	ErrHEIFCannotRead = errors.New("the HEIF tool on this computer cannot read HEIC pictures; install libheif's heif-dec, or an ImageMagick built with HEIC support")
	// ErrHEIFUnreadable: the decoder ran and produced nothing it could call a picture.
	ErrHEIFUnreadable = errors.New("that HEIC picture could not be read")
)

//go:embed heifprobe.heic
var heifProbeImage []byte

const (
	heifProbeW, heifProbeH = 64, 48
	// heifConvertTimeout is the decoder's wall clock (the plan's starting value, D2).
	heifConvertTimeout = 60 * time.Second
	heifProbeTimeout   = 20 * time.Second
)

// heifNames is the decoder order. The index is the cache's.
var heifNames = [...]string{"heif-dec", "heif-convert", "magick"}

var heifCaches [len(heifNames)]toolCache

// heifTool is one decoder found on this machine.
type heifTool struct {
	name string // one of heifNames: it decides the arguments
	path string
}

// heifCandidates are the stock install places that are not on PATH. Elsewhere these tools are packaged onto
// PATH (Debian's libheif-examples, Homebrew's libheif and imagemagick).
func heifCandidates(name string) []string {
	if runtime.GOOS != "windows" || name != "magick" {
		return nil
	}
	var out []string
	for _, root := range programFileRoots() {
		out = append(out, filepath.Join(root, "ImageMagick*", "magick.exe"))
	}
	return out
}

// heifTools lists every decoder found, in the order they are tried.
func heifTools() []heifTool {
	var out []heifTool
	for i, n := range heifNames {
		names := []string{n}
		if runtime.GOOS == "windows" {
			names = append(names, n+".exe")
		}
		if p := heifCaches[i].find(names, heifCandidates(n)); p != "" {
			out = append(out, heifTool{name: n, path: p})
		}
	}
	return out
}

// HEIFAvailable reports whether any decoder is FOUND. It starts no process (/api/status runs no probe, Y2):
// whether a found tool can read HEIC is asked at the first conversion and named then.
func HEIFAvailable() bool { return len(heifTools()) > 0 }

// ---- the container's own sizes ----------------------------------------------------------------------

// heifBrands are the `ftyp` brands that say HEIC or HEIF; an AV1 picture (AVIF) is not claimed.
var heifBrands = map[string]bool{"heic": true, "heix": true, "hevc": true, "hevx": true, "heim": true, "heis": true, "mif1": true, "msf1": true}

// isHEIF reads a file's head: an `ftyp` box whose major or compatible brand names HEIF and none names AVIF.
func isHEIF(head []byte) bool {
	if len(head) < 16 || string(head[4:8]) != "ftyp" {
		return false
	}
	size := int(binary.BigEndian.Uint32(head[0:4]))
	if size < 16 || size > len(head) {
		size = len(head)
	}
	found := false
	for off := 8; off+4 <= size; off += 4 {
		if off == 12 {
			continue // the minor version, not a brand
		}
		b := string(head[off : off+4])
		if b == "avif" || b == "avis" {
			return false
		}
		if heifBrands[b] {
			found = true
		}
	}
	return found
}

// heifRow is the format table's row for HEIC; classify returns it for any HEIF family file.
func heifRow() Format {
	f, _ := FormatForExt("heic")
	return f
}

const (
	heifMaxBoxes = 20000 // boxes read in the whole walk
	heifMaxDepth = 5
)

// heifDeclaredSizes returns the width and height of every `ispe` (image spatial extents) property in the
// file's metadata, which is what the container says each picture measures before any decoder runs. A file
// whose metadata cannot be walked returns what it found; the decoder judges the rest.
func heifDeclaredSizes(data []byte) (sizes [][2]uint32) {
	budget := heifMaxBoxes
	var walk func(b []byte, depth int)
	walk = func(b []byte, depth int) {
		for len(b) >= 8 && budget > 0 {
			budget--
			size := uint64(binary.BigEndian.Uint32(b[0:4]))
			typ := string(b[4:8])
			hdr := uint64(8)
			switch size {
			case 0:
				size = uint64(len(b))
			case 1:
				if len(b) < 16 {
					return
				}
				size = binary.BigEndian.Uint64(b[8:16])
				hdr = 16
			}
			if size < hdr || size > uint64(len(b)) {
				return
			}
			body := b[hdr:size]
			switch typ {
			case "meta":
				if depth < heifMaxDepth && len(body) >= 4 {
					walk(body[4:], depth+1) // a full box: version and flags first
				}
			case "iprp", "ipco":
				if depth < heifMaxDepth {
					walk(body, depth+1)
				}
			case "ispe":
				if len(body) >= 12 {
					sizes = append(sizes, [2]uint32{binary.BigEndian.Uint32(body[4:8]), binary.BigEndian.Uint32(body[8:12])})
				}
			}
			b = b[size:]
		}
	}
	walk(data, 0)
	return sizes
}

// heifWithinCaps refuses a picture whose container declares more than the pixel cap, before a decoder runs.
func heifWithinCaps(data []byte) error {
	for _, s := range heifDeclaredSizes(data) {
		if s[0] == 0 || s[1] == 0 {
			continue
		}
		if s[0] > 1<<31-1 || s[1] > 1<<31-1 {
			return fmt.Errorf("%w: it declares %d×%d", ErrImagePixels, s[0], s[1])
		}
		if err := (&rasterJob{}).charge(int(s[0]), int(s[1])); err != nil {
			return err
		}
	}
	return nil
}

// ---- running a decoder ------------------------------------------------------------------------------

// magickPolicy is the whole ImageMagick policy for a HEIC conversion: nothing is allowed, then the two
// coders this job uses. Later rules win, so the order is the point.
const magickPolicy = `<?xml version="1.0" encoding="UTF-8"?>
<policymap>
  <policy domain="delegate" rights="none" pattern="*" />
  <policy domain="coder" rights="none" pattern="*" />
  <policy domain="coder" rights="read" pattern="{HEIC,HEIF}" />
  <policy domain="coder" rights="write" pattern="{PNG}" />
  <policy domain="filter" rights="none" pattern="*" />
  <policy domain="module" rights="none" pattern="*" />
  <policy domain="module" rights="read|write" pattern="{HEIC,HEIF,PNG}" />
  <policy domain="path" rights="none" pattern="@*" />
  <policy domain="path" rights="none" pattern="|*" />
  <policy domain="resource" name="time" value="60" />
  <policy domain="resource" name="memory" value="1GiB" />
  <policy domain="resource" name="map" value="1GiB" />
  <policy domain="resource" name="disk" value="1GiB" />
</policymap>
`

// heifArgs is the ONE spelling of a decoder's arguments (the probe and the conversion both use it).
func heifArgs(t heifTool, in, out string) []string {
	if t.name == "magick" {
		// The coder is named by prefix on both sides, so neither the name nor the bytes choose one.
		return []string{"-quiet", "heic:" + in, "png:" + out}
	}
	return []string{in, out}
}

// runHEIF decodes data to a PNG with t, in a directory of its own. A file with several pictures is read at
// its first (libheif writes out-1.png, out-2.png …; the rest are ignored, declared in ADR-199).
func runHEIF(ctx context.Context, t heifTool, data []byte, timeout time.Duration) ([]byte, error) {
	dir, err := tempdir.New("heif")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	inDir, outDir := dir.Join("in"), dir.Join("out")
	for _, d := range []string{inDir, outDir} {
		if err := os.Mkdir(d, 0o700); err != nil {
			return nil, err
		}
	}
	in := filepath.Join(inDir, "in.heic")
	out := filepath.Join(outDir, "out.png")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return nil, err
	}
	env := toolEnv(os.Environ(), dir.Path())
	if t.name == "magick" {
		cfg := dir.Join("cfg")
		if err := os.Mkdir(cfg, 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(cfg, "policy.xml"), []byte(magickPolicy), 0o600); err != nil {
			return nil, err
		}
		env = append(env, "MAGICK_CONFIGURE_PATH="+cfg)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.path, heifArgs(t, in, out)...)
	cmd.Env = env
	// cmd.Stdin stays nil: the null device.
	pick := func() string {
		for _, n := range []string{"out.png", "out-1.png"} {
			if _, err := os.Stat(filepath.Join(outDir, n)); err == nil {
				return filepath.Join(outDir, n)
			}
		}
		return out
	}
	return runConvertPick(ctx, cmd, pick, t.name, dir)
}

// heifProbeCache is one answer per binary (path, size, modification time: an upgrade is asked again),
// refusals included, because a probe costs a decode and does not change until the file does.
var heifProbeCache struct {
	mu sync.Mutex
	m  map[string]heifProbeState
}

type heifProbeState struct {
	size int64
	mod  time.Time
	err  error
}

// heifProbe asks whether t reads HEIC: it converts the picture Nib carries and requires a PNG of the size
// that picture has. A cancelled probe says nothing and is not cached.
func heifProbe(ctx context.Context, t heifTool) error {
	fi, serr := os.Stat(t.path)
	heifProbeCache.mu.Lock()
	defer heifProbeCache.mu.Unlock()
	if serr == nil {
		if c, ok := heifProbeCache.m[t.path]; ok && c.size == fi.Size() && c.mod.Equal(fi.ModTime()) {
			return c.err
		}
	}
	err := probeHEIF(ctx, t)
	if ctx.Err() != nil || (err != nil && !errors.Is(err, ErrHEIFCannotRead)) {
		return err // not a verdict: cancelled, timed out, out of memory or shutting down
	}
	if serr == nil {
		if heifProbeCache.m == nil {
			heifProbeCache.m = map[string]heifProbeState{}
		}
		heifProbeCache.m[t.path] = heifProbeState{size: fi.Size(), mod: fi.ModTime(), err: err}
	}
	return err
}

func probeHEIF(ctx context.Context, t heifTool) error {
	out, err := runHEIF(ctx, t, heifProbeImage, heifProbeTimeout)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if probeUnfinished(err) {
			return err
		}
		return fmt.Errorf("%w (%s: %v)", ErrHEIFCannotRead, t.name, err)
	}
	cfg, perr := png.DecodeConfig(bytes.NewReader(out))
	if perr != nil || cfg.Width != heifProbeW || cfg.Height != heifProbeH {
		return fmt.Errorf("%w (%s did not return the test picture)", ErrHEIFCannotRead, t.name)
	}
	return nil
}

// ConvertHEICToPNG decodes a HEIC or HEIF picture through the first installed decoder that can read one.
func ConvertHEICToPNG(ctx context.Context, data []byte) ([]byte, error) {
	return convertHEICWith(ctx, heifTools(), data)
}

// convertHEICWith is ConvertHEICToPNG for a named list of decoders (a test names stand-ins).
func convertHEICWith(ctx context.Context, tools []heifTool, data []byte) ([]byte, error) {
	if len(tools) == 0 {
		return nil, ErrHEIFMissing
	}
	if err := heifWithinCaps(data); err != nil {
		return nil, err
	}
	var skipped error
	for _, t := range tools {
		if err := heifProbe(ctx, t); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if skipped == nil {
				skipped = err
			}
			continue
		}
		out, err := runHEIF(ctx, t, data, heifConvertTimeout)
		if err != nil {
			if errors.Is(err, ErrConverterOverMemory) || errors.Is(err, ErrConvertedTooLarge) || errors.Is(err, ErrShuttingDown) || ctx.Err() != nil {
				return nil, err
			}
			return nil, fmt.Errorf("%w: %s said: %s", ErrHEIFUnreadable, t.name, strings.TrimPrefix(err.Error(), t.name+" could not convert this document: "))
		}
		return out, nil
	}
	return nil, skipped
}

// heicToDocument is the HEIC row's producer: decode, then the picture door.
func heicToDocument(ctx context.Context, data []byte, name string) ([]byte, error) {
	out, err := ConvertHEICToPNG(ctx, data)
	if err != nil {
		return nil, err
	}
	return ImageToDocumentCtx(ctx, out, name)
}
