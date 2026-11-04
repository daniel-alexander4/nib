package pdfops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"nib/internal/pdfread"
	"nib/internal/tempdir"
)

// The general, vector-preserving PDF/A path: shell out to an installed Ghostscript.
// It handles what the pure-Go PreparePDFA refuses — re-embedding referenced-but-
// unembedded fonts and converting colour — at the cost of an optional external
// runtime dependency (detected at runtime, never bundled; the binary stays
// cgo-free). Opt-in by design.
//
// SECURITY: Ghostscript *interprets* the input PDF (a historically CVE-prone
// interpreter). This is opt-in (the user installed gs and chose this path), runs
// under SAFER (gs ≥9.50 default; file access limited to the one ICC we permit),
// with a timeout and no shell (args as a slice). Nib is loopback/single-user, so
// the residual risk is the same class as opening a malicious PDF in any viewer.
// Like the pure-Go path the result is a *candidate*; verify with veraPDF.

// ErrGhostscriptMissing is returned when no Ghostscript binary can be found, so the caller
// can degrade gracefully rather than fail obscurely. Classified through `MissingToolFor` and
// then worded by each surface: the CLI names `--gs`, which only the CLI knows about.
//
// The "hidden GUI button" this comment used to promise is real but inverted — `#pdfaGsGo` is
// revealed when Ghostscript IS present, so with gs absent the modal said nothing at all and
// the 400 below was unreachable from the GUI. That branch now exists.
var ErrGhostscriptMissing = errors.New("Ghostscript (gs) is not installed")

const gsConvertTimeout = 2 * time.Minute

// convertWaitDelay is how long a converter's I/O may outlive its killed process tree before `runConvert`
// returns anyway — see `runConvert`.
const convertWaitDelay = 2 * time.Second

var gsCache toolCache

// ghostscriptPath returns the Ghostscript executable path, or "" if none is found.
//
// PATH first — "gs" on Unix, "gswin64c"/"gswin32c" on Windows — then the stock install
// locations. Windows is why the candidates are globs rather than literals: gs installs into a
// VERSION-numbered directory (`gs\gs10.03.1\bin`), which no fixed string can name. See
// `toolpath.go`.
func ghostscriptPath() string {
	return gsCache.find([]string{"gs", "gswin64c", "gswin32c"}, ghostscriptCandidates())
}

// GhostscriptAvailable reports whether Ghostscript can be found, so a caller can offer the
// general converter (and say so when it cannot).
func GhostscriptAvailable() bool { return ghostscriptPath() != "" }

// ConvertPDFAGhostscript converts pdf to a PDF/A-2b candidate via an installed
// Ghostscript. It writes the document, the vendored sRGB profile, and a PDF/A
// definition (which attaches the sRGB OutputIntent — bare -dPDFA=2 omits it) into
// a temp dir, then runs gs under SAFER. Returns ErrGhostscriptMissing when gs is
// absent.
//
// **It runs an outside tool, so it takes a slot at ImportGate** (ADR-185, ADR-009): two conversions at a
// time whichever door asked, the import path's or this one. It waits however long the queue is — a person
// pressed the button and nothing here is a hand-off backlog — and the two-minute budget starts once the slot
// is held. ctx is the caller's: a closed request or a Ctrl-C ends the wait and kills the group.
func ConvertPDFAGhostscript(ctx context.Context, pdf []byte) (out []byte, err error) {
	gs := ghostscriptPath()
	if gs == "" {
		return nil, ErrGhostscriptMissing
	}
	// Ghostscript reads the file itself and would convert a dynamic XFA form's placeholder page into a PDF/A
	// copy of "Please wait…" — the write door never sees it, so its refusal is asked here (ADR-145; the census
	// caught this door). One unvalidated parse, beside a conversion measured in seconds.
	if xfa, err := pdfread.ReadXFA(pdf); err == nil && xfa == pdfread.XFADynamic {
		return nil, pdfread.ErrDynamicXFA
	}
	err = ImportGate.RunWait(ctx, gsConvertTimeout, nil, func(ctx context.Context) error {
		out, err = convertPDFAGhostscript(ctx, gs, pdf)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func convertPDFAGhostscript(ctx context.Context, gs string, pdf []byte) ([]byte, error) {
	dir, err := tempdir.New("pdfa")
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	inPath := dir.Join("in.pdf")
	outPath := dir.Join("out.pdf")
	iccPath := dir.Join("srgb.icc")
	defPath := dir.Join("pdfa_def.ps")
	for _, w := range []struct {
		path string
		data []byte
	}{{inPath, pdf}, {iccPath, srgbICC}, {defPath, []byte(pdfaDefPS(iccPath))}} {
		if err := os.WriteFile(w.path, w.data, 0o600); err != nil {
			return nil, err
		}
	}

	cmd := exec.CommandContext(ctx, gs,
		"-dSAFER", // the ≥9.50 default, but explicit so an older gs is sandboxed too
		"-dPDFA=2", "-dPDFACompatibilityPolicy=1",
		"-dBATCH", "-dNOPAUSE", "-dQUIET",
		"-sColorConversionStrategy=RGB", "-sDEVICE=pdfwrite",
		"--permit-file-read="+iccPath,
		"-sOutputFile="+outPath,
		defPath, inPath,
	)
	cmd.Env = toolEnv(os.Environ(), dir.Path())
	return runConvert(ctx, cmd, outPath, "Ghostscript", dir)
}

// runConvert runs a prepared external-converter command and reads the file it
// was told to produce — the run/timeout/stderr-tail plumbing Ghostscript and
// LibreOffice share, kept in one place so fixes land in both.
func runConvert(ctx context.Context, cmd *exec.Cmd, outPath, tool string, dir *tempdir.Dir) ([]byte, error) {
	return runConvertPick(ctx, cmd, func() string { return outPath }, tool, dir)
}

// runConvertPick is runConvert for a tool that names its own output: pick is asked AFTER the process has
// ended, so a decoder that writes `out-1.png` where it was asked for `out.png` (libheif does, for a file
// with several pictures) is read from where it wrote.
func runConvertPick(ctx context.Context, cmd *exec.Cmd, pick func() string, tool string, dir *tempdir.Dir) ([]byte, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// **The timeout has to end the conversion, not only the process nib started** (`/pending 503`).
	// `soffice` is a launcher: it starts `soffice.bin`, and `CommandContext` kills only the launcher. A hung
	// `soffice.bin` kept converting past the timeout, and because it inherited the stderr pipe, `Run` waited
	// on it — the timeout returned nothing until the grandchild chose to exit. So the whole tree is killed
	// on cancel (`killTreeOnCancel`, per platform), and `WaitDelay` bounds the wait for any descendant that
	// escaped the tree and still holds the pipe.
	killTreeOnCancel(cmd)
	cmd.WaitDelay = convertWaitDelay
	// **Started, registered, then waited on — not `Run`** (ADR-185). A cancel is not the only way a
	// converter's owner ends: Quit, the idle exit and SIGTERM do not cancel this context, so the group is
	// registered for `KillConverters`, and its id is written beside the input so a sweep after a crash can
	// find what the crash orphaned.
	if err := beginStart(); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		endStart()
		return nil, fmt.Errorf("%s could not convert this document: %s", tool, execErrTail(stderr.String(), err))
	}
	forget, rerr := registerConverter(cmd.Process.Pid)
	endStart()
	if rerr != nil {
		killGroup(cmd.Process.Pid)
		_ = cmd.Wait()
		return nil, rerr
	}
	defer forget()
	if dir != nil {
		_ = dir.RecordGroup(cmd.Process.Pid) // best-effort: it only helps a sweep after a crash
	}
	// The memory ceiling (R7, Y5): a watchdog over the group, never an address-space limit. A kill reads as a
	// refusal that names the ceiling, not as a converter that "could not convert".
	stopWatch := watchMemory(cmd.Process.Pid, converterMemoryCeiling, memoryTick)
	defer func() { stopWatch() }()
	waitErr := cmd.Wait()
	if killed, _ := stopWatch(); killed {
		return nil, ErrConverterOverMemory
	}
	if err := waitErr; err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("%s timed out converting this document", tool)
		}
		return nil, fmt.Errorf("%s could not convert this document: %s", tool, execErrTail(stderr.String(), err))
	}
	// **Sized by `stat` before it is read** (D31(3)): a converter's output is its own, and reading a
	// file of whatever size it chose into memory is how a 5 GiB result becomes a 5 GiB heap.
	outPath := pick()
	fi, serr := os.Stat(outPath)
	if serr == nil && fi.Size() > maxConvertedBytes {
		return nil, ErrConvertedTooLarge
	}
	out, err := os.ReadFile(outPath)
	if err != nil || len(out) == 0 {
		return nil, fmt.Errorf("%s produced no output: %s", tool, execErrTail(stderr.String(), err))
	}
	return out, nil
}

// converterMemoryCeiling is the resident-set ceiling for one converter's process group. PROVISIONAL, set
// from the measurement P01.S02a records (importrss_test.go) with stated headroom.
const converterMemoryCeiling = 3 << 30

// memoryTick is how often the watchdog samples. A sample reads /proc, so it is not made faster than a
// converter can allocate a gigabyte.
const memoryTick = 250 * time.Millisecond

// ErrConverterOverMemory is a converter ended for exceeding its memory ceiling.
var ErrConverterOverMemory = errors.New("the converter used more memory than Nib allows, so it was stopped")

// maxConvertedBytes bounds what a converter may hand back. The server's open cap (`maxPDFBytes`, ADR-008)
// is the same figure and is still applied where the document is installed; this one stops the read.
const maxConvertedBytes = 200 << 20

// ErrConvertedTooLarge is a converter whose output is past maxConvertedBytes.
var ErrConvertedTooLarge = errors.New("the converted document is too large to open")

// pdfaDefPS builds the PDF/A definition Ghostscript needs to attach the sRGB
// OutputIntent. It references the vendored ICC at iccPath, which the caller
// permits via --permit-file-read under SAFER.
func pdfaDefPS(iccPath string) string {
	return "%!\n" +
		"[/_objdef {icc_PDFA} /type /stream /OBJ pdfmark\n" +
		"[{icc_PDFA} <</N 3>> /PUT pdfmark\n" +
		"[{icc_PDFA} (" + psEscape(iccPath) + ") (r) file /PUT pdfmark\n" +
		"[/_objdef {OutputIntent_PDFA} /type /dict /OBJ pdfmark\n" +
		"[{OutputIntent_PDFA} <<\n" +
		"  /Type /OutputIntent /S /GTS_PDFA1\n" +
		"  /DestOutputProfile {icc_PDFA}\n" +
		"  /OutputConditionIdentifier (sRGB)\n" +
		"  /Info (sRGB IEC61966-2.1)\n" +
		">> /PUT pdfmark\n" +
		"[{Catalog} <</OutputIntents [ {OutputIntent_PDFA} ]>> /PUT pdfmark\n"
}

// psEscape escapes a path for a PostScript ( ) literal string.
func psEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s)
}

// execErrTail returns a short, useful tail of a converter's stderr for an error.
func execErrTail(stderr string, err error) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		if err == nil { // empty output file with a clean exit: no detail to add
			return "(no error output)"
		}
		return err.Error()
	}
	lines := strings.Split(stderr, "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.Join(lines, "; ")
}
