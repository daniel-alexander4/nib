package pdfops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nib/internal/tempdir"
)

// PostScript and EPS in (PLAN-import-export P04.S02a, X11(b), ADR-198).
//
// PostScript is a PROGRAM, not a picture: Ghostscript runs it, and a program can read files
// (`(/etc/hostname) (r) file`), start commands (`(%pipe%…) (r) file`) and loop forever. Nib's answer is
// three layers, none of which trusts the others:
//
//  1. **The flags allow nothing.** `-dSAFER`, the one device `pdfwrite`, no `--permit-file-*`, the input
//     alone in a directory of its own, stdin the null device, a lowered environment (toolEnv). One
//     function builds them (`psArgs`) for the conversion AND the probe, so the probe cannot test flags the
//     conversion does not use.
//  2. **The build is asked, once, whether it holds** (`confineProbe`). A version number cannot say: a
//     distribution backports a security fix and keeps the old number (Ubuntu 24.04 reports 10.02.1), and an
//     upstream release can carry a bypass the number does not show. The probe runs a PostScript program that
//     tries to read a file and to start a command, and requires both to be refused and the command not to
//     have run. Measured on 10.02.1: with `-dSAFER` both refused; with `-dNOSAFER` both succeed.
//  3. **A floor for what predates the confinement** (`gsFloor`): `-dSAFER` became the default, and
//     `--permit-file-*` its vocabulary, in 9.50.
//
// The page count, output size, time and memory are the door's (importPages, maxConvertedBytes, the gate,
// the watchdog in runConvert).

// gsFloor is the oldest Ghostscript the door will run a program on. 9.50 is where SAFER became the default
// and `--permit-file-read` / `--permit-file-write` took their present meaning. It is a FLOOR and not a
// verdict: CVE-2024-29510 (a SAFER bypass through the `uniprint` device) is fixed upstream in 10.03.1 and a
// version below that may still be patched by its distributor, so the number cannot decide. The probe does NOT
// exercise that bypass (ADR-203, /pending 943): it tries a file read and a pipe, which plain SAFER already refuses.
var gsFloor = gsVersion{9, 50}

// Refusals named by the PostScript door.
var (
	// ErrGhostscriptTooOld: a Ghostscript older than gsFloor.
	ErrGhostscriptTooOld = errors.New("the Ghostscript on this computer is too old to open PostScript safely; install Ghostscript 9.50 or newer")
	// ErrGhostscriptNotConfined: this Ghostscript let a PostScript program read a file or start a command.
	ErrGhostscriptNotConfined = errors.New("this computer's Ghostscript did not keep a PostScript program from reading files or starting programs, so Nib will not run one on it")
	// ErrPostScriptUnreadable: Ghostscript ran and produced nothing it could call a document.
	ErrPostScriptUnreadable = errors.New("Ghostscript could not read that PostScript file")
)

type gsVersion struct{ major, minor int }

func (v gsVersion) less(o gsVersion) bool {
	return v.major < o.major || v.major == o.major && v.minor < o.minor
}

func (v gsVersion) String() string { return fmt.Sprintf("%d.%02d", v.major, v.minor) }

// parseGSVersion reads `gs --version` ("10.02.1", "9.56.1", "9.27"). Anything else is false.
func parseGSVersion(s string) (gsVersion, bool) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) < 2 {
		return gsVersion{}, false
	}
	maj, e1 := strconv.Atoi(parts[0])
	min, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || maj < 0 || min < 0 {
		return gsVersion{}, false
	}
	return gsVersion{maj, min}, true
}

// psArgs is the ONE spelling of Ghostscript's arguments for interpreting a stranger's PostScript. eps adds
// the crop to the file's own bounding box. -dLastPage bounds the work: a program that draws a million pages
// stops after one more than Nib opens, and the door refuses on the count.
func psArgs(in, out string, eps bool) []string {
	args := []string{
		"-dSAFER", // explicit, so an old gs is confined too; never -dNOSAFER, never --permit-file-*
		"-dBATCH", "-dNOPAUSE", "-dQUIET",
		"-sDEVICE=pdfwrite",
		"-dLastPage=" + strconv.Itoa(importPages+1),
		"-sOutputFile=" + out,
	}
	if eps {
		args = append(args, "-dEPSCrop")
	}
	// "--" ends option parsing: the input is a path in a directory Nib made, but a name that began with "-"
	// would otherwise be an option.
	return append(args, "--", in)
}

// confineState is what the probe learned about one Ghostscript binary.
type confineState struct {
	path    string
	size    int64
	modTime time.Time
	err     error
}

var confineCache struct {
	mu sync.Mutex
	st confineState
}

// confineProbe answers once per binary (path, size and modification time, so an upgrade is asked again)
// whether gs meets gsFloor and keeps a PostScript program inside the sandbox. The answer — including a
// refusal — is cached, because it costs a Ghostscript start (~0.17 s) and does not change until the file does.
func confineProbe(ctx context.Context, gs string) error {
	fi, serr := os.Stat(gs)
	confineCache.mu.Lock()
	defer confineCache.mu.Unlock()
	if serr == nil {
		c := confineCache.st
		if c.path == gs && c.size == fi.Size() && c.modTime.Equal(fi.ModTime()) {
			return c.err
		}
	}
	err := probeGhostscript(ctx, gs)
	if ctx.Err() != nil {
		return err // a cancelled probe says nothing about the binary
	}
	if err != nil && !errors.Is(err, ErrGhostscriptNotConfined) && !errors.Is(err, ErrGhostscriptTooOld) {
		return err // a probe that did not finish (timeout, no scratch space) is not a verdict
	}
	if serr == nil {
		confineCache.st = confineState{path: gs, size: fi.Size(), modTime: fi.ModTime(), err: err}
	}
	return err
}

// confineProbePS tries the two things the plan names (X11(b)): read a file and start a command. Each is
// wrapped in `stopped` and prints HELD when the interpreter refused it, LEAK when it did not.
func confineProbePS(marker string) string {
	return "%!PS\n" +
		"{ (" + psEscape(marker+".read") + ") (r) file closefile (LEAK) print } stopped { (HELD) print } if\n" +
		"{ (%pipe%touch " + psEscape(marker+".ran") + ") (r) file closefile (LEAK) print } stopped { (HELD) print } if\n" +
		"showpage\n"
}

func probeGhostscript(ctx context.Context, gs string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dir, err := tempdir.New("psprobe")
	if err != nil {
		return err
	}
	defer dir.Close()

	vcmd := exec.CommandContext(ctx, gs, "--version")
	vcmd.Env = toolEnv(os.Environ(), dir.Path())
	vout, verr := vcmd.Output()
	if verr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %v", ErrGhostscriptNotConfined, verr)
	}
	v, ok := parseGSVersion(string(vout))
	if !ok {
		return fmt.Errorf("%w: it answered %q to --version", ErrGhostscriptNotConfined, strings.TrimSpace(string(vout)))
	}
	if v.less(gsFloor) {
		return fmt.Errorf("%w (found %s)", ErrGhostscriptTooOld, v)
	}

	// The target lives in a directory OF ITS OWN, not in the job's: Ghostscript lets a program use its scratch
	// directory (TMPDIR), which is the job's, so a "secret" there is readable under SAFER by design (measured:
	// the first version of this probe put it there and read it back).
	outside, err := tempdir.New("psprobe-outside")
	if err != nil {
		return err
	}
	defer outside.Close()
	marker := outside.Join("marker")
	// The file the first attempt asks to read must EXIST, or a refusal and a missing file read alike.
	if err := outside.WriteFile("marker.read", []byte("secret")); err != nil {
		return err
	}
	if err := dir.WriteFile("probe.ps", []byte(confineProbePS(marker))); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, gs, psArgs(dir.Join("probe.ps"), dir.Join("probe.pdf"), false)...)
	cmd.Env = toolEnv(os.Environ(), dir.Path())
	killTreeOnCancel(cmd)
	cmd.WaitDelay = convertWaitDelay
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: the probe did not run (%v)", ErrGhostscriptNotConfined, err)
	}
	if _, err := os.Stat(marker + ".ran"); err == nil {
		return ErrGhostscriptNotConfined
	}
	if got := stdout.String(); got != "HELDHELD" {
		return fmt.Errorf("%w: the probe printed %q", ErrGhostscriptNotConfined, got)
	}
	return nil
}

// ConvertPostScriptToPDF turns PostScript or EPS into a PDF through an installed Ghostscript. The input is
// written alone into a directory Nib makes, under a fixed name; the output goes to a second one. The caller
// holds the gate (Import does) and its context ends the conversion, killing the process tree.
func ConvertPostScriptToPDF(ctx context.Context, data []byte, eps bool) ([]byte, error) {
	gs := ghostscriptPath()
	if gs == "" {
		return nil, ErrGhostscriptMissing
	}
	return convertPostScriptWith(ctx, gs, data, eps)
}

// convertPostScriptWith is ConvertPostScriptToPDF for a named Ghostscript binary (a test names a stand-in).
func convertPostScriptWith(ctx context.Context, gs string, data []byte, eps bool) ([]byte, error) {
	if err := confineProbe(ctx, gs); err != nil {
		return nil, err
	}
	dir, err := tempdir.New("ps")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	inDir := dir.Join("in")
	outDir := dir.Join("out")
	for _, d := range []string{inDir, outDir} {
		if err := os.Mkdir(d, 0o700); err != nil {
			return nil, err
		}
	}
	inName := "in.ps"
	if eps {
		inName = "in.eps"
	}
	inPath := filepath.Join(inDir, inName)
	outPath := filepath.Join(outDir, "out.pdf")
	if err := os.WriteFile(inPath, data, 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, gsConvertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gs, psArgs(inPath, outPath, eps)...)
	cmd.Env = toolEnv(os.Environ(), dir.Path())
	// cmd.Stdin stays nil: Go then reads the null device, so a program that reads standard input gets end-of-file.
	pdf, err := runConvert(ctx, cmd, outPath, "Ghostscript", dir)
	if err != nil {
		return nil, err
	}
	n, err := PageCount(pdf)
	if err != nil || n == 0 {
		return nil, ErrPostScriptUnreadable
	}
	if n > importPages {
		return nil, fmt.Errorf("%w: Nib opens up to %d", ErrTooManyPages, importPages)
	}
	return pdf, nil
}

// probeUnfinished: the error says the probe did not complete, which is a fact about this moment and not
// about the tool, so it is never cached as a verdict.
func probeUnfinished(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		errors.Is(err, ErrConverterOverMemory) || errors.Is(err, ErrShuttingDown) || errors.Is(err, ErrConvertedTooLarge)
}
