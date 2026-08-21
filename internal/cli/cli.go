// Package cli implements nib's headless subcommands — the scriptable surface
// that runs PDF operations without the loopback server or a browser. main
// dispatches to Run before binding a port: a recognized verb runs and exits,
// while any other first argument (a PDF path, --replace, or nothing) falls
// through to the normal desktop boot.
//
// Conventions, kept uniform across every command:
//   - commands that produce a PDF take their inputs as positional arguments and
//     write the result to the -o/--out FILE flag (required);
//   - timestamp writes a FILE.ots proof beside each input; verify prints a
//     report — neither uses -o;
//   - errors go to stderr; the exit status is 0 on success, 1 on a usage or I/O
//     error, and 2 when a verification fails.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"nib/internal/addrscope"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Run executes a headless subcommand. It reports handled=false (so main
// continues to the desktop boot) when args is empty or its first element is not
// a known verb; otherwise it runs the command and returns its exit code.
// buildVersion is the version Run was given, kept so the DIAGNOSTICS can name the build.
//
// `nib rendezvous` and `nib discover` exist to be pasted to somebody else, and a report
// that does not say which build produced it costs a round trip every time.
var buildVersion = "dev"

func Run(args []string, version string) (handled bool, code int) {
	if version != "" {
		buildVersion = version
	}
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "version", "--version":
		fmt.Println("nib " + version)
		return true, 0
	case "help", "--help", "-h":
		printUsage(os.Stdout)
		return true, 0
	}
	run, ok := commands[args[0]]
	if !ok {
		// Not a known verb. Normally the first arg is a PDF path and main opens the
		// desktop app — but a transform-only flag (-o/--out/-w/--in-place) among the
		// args means the user meant a headless command and mistyped the verb; booting
		// the GUI would silently discard their -o. Only that unambiguous signal is
		// treated as a typo; a bare non-verb still falls through to open a file.
		if hasTransformFlag(args[1:]) {
			fmt.Fprintf(os.Stderr, "nib: unknown command %q (run \"nib help\" for the command list)\n", args[0])
			return true, 1
		}
		return false, 0 // not a subcommand — let main open the desktop app
	}
	return true, run(args[1:])
}

// hasTransformFlag reports whether args carry an output flag that only the
// headless transform commands accept, so a non-verb first argument alongside one
// is a mistyped command rather than a file to open in the desktop app.
//
// **`--out-dir` and `--data` are in the list, and their absence was the defect.** The list
// knew only `-o/--out/-w/--in-place`, while `split`, `fill` (CSV mail-merge) and
// `pagenum --continuous` write through `--out-dir` and `fill` requires `--data`. So
// `nib splt big.pdf --out-dir out --every 2` was not recognised as a mistyped verb: it fell
// through to the desktop boot, `flag.Parse` stopped at the first non-flag, and
// `initialFile` returned a path to a file named "splt". The batch was silently discarded —
// exactly the outcome this guard's own comment says it exists to prevent, for the three
// subcommands that produce the MOST files.
func hasTransformFlag(args []string) bool {
	for _, a := range args {
		switch a {
		case "-o", "--out", "-w", "--in-place", "--out-dir", "--data":
			return true
		}
		if strings.HasPrefix(a, "-o=") || strings.HasPrefix(a, "--out=") ||
			strings.HasPrefix(a, "--out-dir=") || strings.HasPrefix(a, "--data=") {
			return true
		}
	}
	return false
}

// commands maps each verb to its handler. version/help are handled in Run.
var commands = map[string]func([]string) int{
	"timestamp":   cmdTimestamp,
	"verify":      cmdVerify,
	"optimize":    cmdOptimize,
	"merge":       cmdMerge,
	"sanitize":    cmdSanitize,
	"sign":        cmdSign,
	"watch":       cmdWatch,
	"rotate":      cmdRotate,
	"pages":       cmdPages,
	"split":       cmdSplit,
	"encrypt":     cmdEncrypt,
	"decrypt":     cmdDecrypt,
	"nup":         cmdNup,
	"normalize":   cmdNormalize,
	"pagenum":     cmdPagenum,
	"pagelabels":  cmdPagelabels,
	"fill":        cmdFill,
	"export-xfdf": cmdExportXFDF,
	"pdfa":        cmdPDFA,
	"office":      cmdOffice,
	"attachments": cmdAttachments,
	"outline":     cmdOutline,
	"register":    cmdRegister,
	"unregister":  cmdUnregister,
	"discover":    cmdDiscover,
	"rendezvous":  cmdRendezvous,
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `nib — local PDF tool

Run "nib" with no command (optionally a PDF path) to open the desktop app.
These subcommands run headlessly, without a browser:

  nib register                    (Windows) offer Nib for PDFs in "Open with"
  nib unregister                  (Windows) undo register
  nib timestamp FILE...           write an OpenTimestamps proof (FILE.ots) per file
  nib timestamp --verify FILE...  check each FILE against its FILE.ots proof
  nib verify [--json] FILE...     report signature integrity (exit 2 if not wholly signed)
  nib optimize IN -o OUT          losslessly shrink a PDF
  nib merge IN... -o OUT          concatenate PDFs into one
  nib sanitize IN -o OUT          strip metadata and active content (JS, actions)
  nib sign IN -o OUT --cert C.p12 certify a PDF with an imported .p12 identity
  nib rotate IN -o OUT --deg N    rotate pages (90/180/270; --pages to limit)
  nib pages IN -o OUT --keep SEL  keep/reorder (or --remove) pages, e.g. 1-3,5
  nib split IN --out-dir DIR …    burst into files by --every N, --ranges, or --bookmarks
  nib encrypt IN -o OUT           add AES-256 password protection (--password-file)
  nib decrypt IN -o OUT           remove password protection / owner restrictions
  nib nup IN -o OUT --n N         place N pages per sheet (2/4/6/9/16…)
  nib normalize IN -o OUT         resize every page to the doc's most common size
  nib pdfa IN -o OUT               convert to a PDF/A-2b archival candidate (--gs: via Ghostscript)
  nib office IN -o OUT             convert a document (.md/.docx/.xlsx/.odt/…) to PDF (office formats need LibreOffice)
  nib pagenum IN -o OUT           stamp running page numbers / Bates numbering
                                  (--continuous threads one counter across FILE…)
  nib pagelabels IN -o OUT        set logical page labels (--range PAGE:STYLE[:START[:PREFIX]])
  nib fill IN --data D            fill a form from JSON/XFDF (-o OUT), or mail-merge a CSV (--out-dir DIR)
  nib export-xfdf IN -o OUT        export a form's field data as XFDF (Acrobat/Foxit interchange)
  nib attachments IN [--json]     list embedded files (--extract NAME / --add FILE)
  nib outline IN [--json]         list the document's bookmark outline
  nib discover [--seconds N]      report what link-local peer discovery can see here
  nib rendezvous [--seconds N]    report whether the DHT remote co-signing uses is
                                  reachable (contacts the public internet)
  nib watch DIR --do OP           run timestamp/optimize/sanitize on each new PDF
  nib version                     print the version

Commands that produce a PDF require -o/--out FILE. For optimize, merge, sanitize,
sign, rotate, pages, encrypt, decrypt, nup, normalize, pagenum, pagelabels, fill (JSON/XFDF), and export-xfdf, "-" reads a PDF from stdin or (as
-o -) writes it to stdout, so they compose in a pipeline. Exit status is non-zero
on failure (verify: 2 when a signature is invalid or absent).

sign reads the .p12 passphrase from --password-file FILE, the NIB_P12_PASSWORD
environment variable, or a no-echo terminal prompt — never from the command
line. Run "nib <command> -h" for a command's own flags.
`)
}

// parse runs fs.Parse and maps the outcome to a CLI result: ok is false when
// the caller should return code (a usage error → 1, or -h → 0 after fs printed
// its usage). Flags may appear before or after the positional file arguments —
// stdlib flag stops at the first non-flag, so reorder lifts the flags first.
func parse(fs *flag.FlagSet, args []string) (code int, ok bool) {
	fs.SetOutput(os.Stderr)
	switch err := fs.Parse(reorder(fs, args)); {
	case err == nil:
		return 0, true
	case errors.Is(err, flag.ErrHelp):
		return 0, false
	default:
		return 1, false
	}
}

// reorder lifts flag tokens (and the value of any non-boolean flag) ahead of the
// positional arguments, so flags work in any position — stdlib flag otherwise
// stops at the first non-flag. A "--" terminator ends flag scanning; everything
// after it is positional. Unknown flags are left in place for fs.Parse to report.
func reorder(fs *flag.FlagSet, args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			pos = append(pos, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.ContainsRune(name, '=') {
			continue // --flag=value carries its own value
		}
		// A non-boolean flag consumes the next token as its value.
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, pos...)
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

// outFlag registers the shared -o/--out output-file flag (both spellings set p).
func outFlag(fs *flag.FlagSet, p *string) {
	fs.StringVar(p, "o", "", "write the output PDF to `FILE` (required)")
	fs.StringVar(p, "out", "", "write the output PDF to `FILE` (required)")
}

// inPlaceFlag registers the shared -w/--in-place flag (both spellings set p),
// which rewrites each input file in place instead of writing to -o.
func inPlaceFlag(fs *flag.FlagSet, p *bool) {
	fs.BoolVar(p, "w", false, "rewrite each input file in place (batch); excludes -o")
	fs.BoolVar(p, "in-place", false, "rewrite each input file in place (batch); excludes -o")
}

// writeAtomic replaces path with data without risking a half-written file: it
// writes a temp file in the same directory, copies the original's permission
// mode onto it, then renames over path (an atomic replace on every target OS).
// A failed write or transform therefore never corrupts or relaxes the original.
func writeAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	// Follow a symlink to its target before writing. os.Stat already follows one to
	// read the mode, but CreateTemp+Rename replaces the LINK — so an in-place
	// rewrite of a symlink silently destroyed the link, left the real file
	// untouched, and dropped the output in the link's directory instead. The same
	// rename breaks a hard link. README promises the rewrite "preserves the file's
	// permissions"; converting a link into a regular file is not that.
	if resolved, rerr := filepath.EvalSymlinks(path); rerr == nil {
		path = resolved
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".nib-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // harmless no-op once the rename has consumed it
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// os.CreateTemp makes the temp 0600; restore the original file's mode so an
	// in-place rewrite doesn't silently tighten permissions.
	if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// usageFunc returns a FlagSet usage that prints a one-line synopsis and help,
// then the command's own flag defaults.
func usageFunc(fs *flag.FlagSet, synopsis, help string) func() {
	return func() {
		// fs.Output(), not os.Stderr. A FlagSet's output is settable and PrintDefaults on
		// the next line already honours it, so writing the synopsis to os.Stderr split one
		// message across two destinations the moment anything set one — which is what a
		// test capturing this text has to do. It defaults to os.Stderr, so the shipped
		// behaviour is unchanged.
		fmt.Fprintf(fs.Output(), "usage: %s\n\n%s\n", synopsis, help)
		fs.PrintDefaults()
	}
}

// errf prints a CLI error to stderr with the nib: prefix.
func errf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "nib: "+format+"\n", a...)
}

// safeClient is the HTTP client for the timestamp paths. It blocks connections
// to loopback/private/link-local addresses on every dial (including redirect
// hops), so a hostile calendar URL embedded in an untrusted .ots proof can't be
// used to probe the user's own network — mirroring the server's
// untrustedFetchClient. The public calendars and explorers it normally reaches
// are unaffected.
func safeClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				Control:   blockPrivateIP,
			}).DialContext,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return requireHTTPScheme(req.URL)
		},
	}
}

func requireHTTPScheme(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	return nil
}

// blockPrivateIP refuses any address `addrscope.Routable` refuses, at connect time on the
// resolved IP. **Shared with `internal/server`'s copy since P04's phase close** — the two
// were byte-identical, both weaker than the repo's own table, and duplicating a security
// predicate is how one of them gets fixed.
func blockPrivateIP(_, address string, _ syscall.RawConn) error {
	return addrscope.RefuseNonPublicDialAddress(address)
}
