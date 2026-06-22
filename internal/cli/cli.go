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
	"os"
	"strings"
	"syscall"
	"time"
)

// Run executes a headless subcommand. It reports handled=false (so main
// continues to the desktop boot) when args is empty or its first element is not
// a known verb; otherwise it runs the command and returns its exit code.
func Run(args []string, version string) (handled bool, code int) {
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
		return false, 0 // not a subcommand — let main open the desktop app
	}
	return true, run(args[1:])
}

// commands maps each verb to its handler. version/help are handled in Run.
var commands = map[string]func([]string) int{
	"timestamp": cmdTimestamp,
	"verify":    cmdVerify,
	"optimize":  cmdOptimize,
	"merge":     cmdMerge,
	"sanitize":  cmdSanitize,
	"sign":      cmdSign,
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `nib — local PDF tool

Run "nib" with no command (optionally a PDF path) to open the desktop app.
These subcommands run headlessly, without a browser:

  nib timestamp FILE...           write an OpenTimestamps proof (FILE.ots) per file
  nib timestamp --verify FILE...  check each FILE against its FILE.ots proof
  nib verify [--json] FILE...     report each file's signature integrity
  nib optimize IN -o OUT          losslessly shrink a PDF
  nib merge IN... -o OUT          concatenate PDFs into one
  nib sanitize IN -o OUT          strip metadata and active content (JS, actions)
  nib sign IN -o OUT --cert C.p12 certify a PDF with an imported .p12 identity
  nib version                     print the version

Commands that produce a PDF require -o/--out FILE. Exit status is non-zero on
failure (verify: 2 when a signature is invalid or absent).

sign reads the .p12 passphrase from --password-file FILE or the NIB_P12_PASSWORD
environment variable — never from the command line. Run "nib <command> -h" for
a command's own flags.
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

// usageFunc returns a FlagSet usage that prints a one-line synopsis and help,
// then the command's own flag defaults.
func usageFunc(fs *flag.FlagSet, synopsis, help string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "usage: %s\n\n%s\n", synopsis, help)
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

func blockPrivateIP(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("could not parse dial address %q", address)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("refusing to connect to non-public address %s", ip)
	}
	return nil
}
