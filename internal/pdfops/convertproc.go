package pdfops

import (
	"path/filepath"
	"strconv"
	"strings"
)

// taskkillCommand is the command that ends a converter's process tree on Windows: `taskkill /T`, which
// walks child processes, resolved from `%SystemRoot%` rather than from PATH.
//
// **It sits in an UNTAGGED file so a machine that is not Windows can still test it** (`/pending 516`). The
// call that uses it is Windows-only and nothing on this repo's machines can run it — `convertproc_windows.go`
// declares that gap and it stands — but the argv is ordinary string handling, and the rule it carries is not
// about Windows at all: a program that ends other processes must not be whatever the user's PATH answers to
// `taskkill`. `fileURL` in `office.go` is the same shape for the same reason, out of the same pending item.
//
// The separator is the host's, because `filepath.Join` is: on Windows this yields `C:\Windows\System32\…`
// and the identical call on Linux yields forward slashes. What a test off Windows can pin is therefore the
// RULE — that the program comes from `%SystemRoot%` when the environment names one — and not the spelling.
func taskkillCommand(systemRoot string, pid int) (string, []string) {
	// No %SystemRoot% leaves PATH as the only thing there is, and a tree kill that never runs is worse
	// than one that goes through PATH: a hung `soffice.bin` holds the pipe the conversion waits on.
	name := "taskkill"
	if systemRoot != "" {
		name = filepath.Join(systemRoot, "System32", "taskkill.exe")
	}
	return name, []string{"/T", "/F", "/PID", strconv.Itoa(pid)}
}

// toolEnvKeys are the environment variables a converter is handed (ADR-185, D33, X11(d)). Everything else
// is dropped: before this, no site set `cmd.Env`, so LibreOffice and Ghostscript inherited the whole of
// Nib's — proxy settings (`HTTP(S)_PROXY`), `SSH_AUTH_SOCK`, `NIB_*`, tokens in the launching shell — for
// a program that interprets a stranger's file. A prefix ending in `*` matches the family.
//
// HOME stays the real one on purpose: LibreOffice finds the person's installed fonts through it, and a
// document laid out in a font the machine has should not change because it was imported. The throwaway
// LibreOffice PROFILE (officeprofile.go) is what isolates its configuration. Declared: an isolated HOME is
// the stricter answer and costs that fidelity; it is revisited when a converter that needs it arrives.
var toolEnvKeys = []string{
	"PATH", "HOME", "USER", "LOGNAME", "LANG", "LANGUAGE", "LC_*", "TZ", "XDG_*", "FONTCONFIG_*",
	// Windows: the interpreter and the profile roots LibreOffice and Ghostscript resolve from.
	"SystemRoot", "SYSTEMDRIVE", "windir", "ComSpec", "PATHEXT", "USERPROFILE", "APPDATA", "LOCALAPPDATA",
	"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "ProgramData",
}

// toolEnv builds the environment a converter runs in: the allowlist above from the given environment (in
// `KEY=value` form), plus TMPDIR / TEMP / TMP pointing into the job's own directory so the tool's scratch
// files land where the sweep owns them (Y10). tmp may be empty for a tool run with no directory.
func toolEnv(environ []string, tmp string) []string {
	var out []string
	for _, kv := range environ {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		k := kv[:i]
		for _, allow := range toolEnvKeys {
			if strings.HasSuffix(allow, "*") && strings.HasPrefix(k, strings.TrimSuffix(allow, "*")) || strings.EqualFold(allow, k) {
				out = append(out, kv)
				break
			}
		}
	}
	if tmp != "" {
		out = append(out, "TMPDIR="+tmp, "TEMP="+tmp, "TMP="+tmp)
	}
	return out
}
