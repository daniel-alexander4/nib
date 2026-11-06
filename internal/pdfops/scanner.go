package pdfops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nib/internal/tempdir"
)

// Scan straight from a scanner (/pending 903, ADR-213). SANE's `scanimage` is the only door: Nib speaks to no
// scanner itself, so the feature exists where that tool is installed (Linux, mostly) and is simply absent where it
// is not. A scanner is a device and not a file, so the person's scan is turned into PNG pages by the tool and each
// page goes through the same picture door as any other picture (ImageToDocumentCtx's pixel caps) before the pages
// are joined; Nib adds no scan-specific way to make a PDF.

// ErrScanNoFeeder: a document feeder was asked for and the scanner names no source that is one.
var ErrScanNoFeeder = errors.New("this scanner has no document feeder")

// ErrScanNoPages: the tool ran and wrote no page.
var ErrScanNoPages = errors.New("the scanner produced no page")

// ErrScanDevice: the device name is not one the scanner list gave.
var ErrScanDevice = errors.New("that scanner is not one Nib listed; choose it again from the list")

const (
	scanListTimeout    = 40 * time.Second // network scanners are found by discovery, which takes seconds
	scanHelpTimeout    = 30 * time.Second
	scanFlatbedTimeout = 5 * time.Minute  // a 600 dpi colour page on a slow flatbed is about a minute
	scanFeederTimeout  = 15 * time.Minute // the whole stack
	scanMaxPages       = 100
	scanMaxDeviceName  = 256
	scanOutOfPaper     = 7 // SANE_STATUS_NO_DOCS: how a batch ends when the feeder is empty
)

// ScanModes and ScanResolutions are what the window offers; the route holds a request to them.
var (
	ScanModes       = []string{"Color", "Gray", "Lineart"}
	ScanResolutions = []int{100, 150, 200, 300, 600}
)

var scanCache toolCache

// ScannerAvailable reports whether scanimage is FOUND. It starts no process (/api/status runs no probe).
func ScannerAvailable() bool {
	return scanCache.find([]string{"scanimage"}, nil) != ""
}

// ScanDevice is one scanner the tool listed.
type ScanDevice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ScanOptions is one scan. Feeder asks for the document feeder and every sheet in it; otherwise one page from
// the glass.
type ScanOptions struct {
	Device string
	DPI    int
	Mode   string
	Feeder bool
}

// validScanDeviceName holds a device name to the shape a name from the list has. It is passed as one argument
// (`--device-name=NAME`), never through a shell, and it may not begin like an option.
func validScanDeviceName(s string) bool {
	if s == "" || len(s) > scanMaxDeviceName || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// parseScanDevices reads `scanimage -f '%v|%m|%t|%d%n'`. The device name is last because it is the one field that
// may hold a `|`.
func parseScanDevices(out string) []ScanDevice {
	var devs []ScanDevice
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 4)
		if len(parts) != 4 || !validScanDeviceName(parts[3]) {
			continue
		}
		label := strings.TrimSpace(strings.TrimSpace(parts[0]) + " " + strings.TrimSpace(parts[1]))
		if label == "" {
			label = parts[3]
		}
		devs = append(devs, ScanDevice{ID: parts[3], Label: label})
	}
	return devs
}

// feederSource finds the source name a scanner calls its document feeder in `scanimage --help`'s
// `--source Flatbed|ADF|...` line. Backends name it differently ("ADF", "Automatic Document Feeder").
func feederSource(help string) (string, bool) {
	for _, line := range strings.Split(help, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "--source") {
			continue
		}
		open, end := strings.Index(line, " "), strings.Index(line, " [")
		if open < 0 {
			continue
		}
		list := strings.TrimSpace(line[open:])
		if end > open {
			list = strings.TrimSpace(line[open:end])
		}
		for _, name := range strings.Split(list, "|") {
			low := strings.ToLower(name)
			if strings.Contains(low, "adf") || strings.Contains(low, "feeder") {
				return strings.TrimSpace(name), true
			}
		}
	}
	return "", false
}

// scanWording says "scan" where the shared runner says "convert".
func scanWording(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.Replace(err.Error(), "could not convert this document", "could not scan", 1)
	msg = strings.Replace(msg, "timed out converting this document", "timed out scanning", 1)
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

func scanTool() (string, error) {
	p := scanCache.find([]string{"scanimage"}, nil)
	if p == "" {
		return "", errors.New("scanimage is not installed")
	}
	return p, nil
}

// ScanDevices lists the scanners scanimage can see. It can take seconds (a network scanner is found by
// discovery), so the window asks when the dialog opens and never at start-up.
func ScanDevices(ctx context.Context) ([]ScanDevice, error) {
	tool, err := scanTool()
	if err != nil {
		return nil, err
	}
	dir, err := tempdir.New("scan")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	ctx, cancel := context.WithTimeout(ctx, scanListTimeout)
	defer cancel()
	out := dir.Join("devices.txt")
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cmd := exec.CommandContext(ctx, tool, "-f", "%v|%m|%t|%d%n")
	cmd.Env = toolEnv(os.Environ(), dir.Path())
	cmd.Stdout = f
	// An empty list is an answer (no scanner), and the runner reads an empty file as "no output"; so the list is
	// read from the file here after the process, not through the runner's return.
	_, rerr := runConvertPickOK(ctx, cmd, func() string { return out }, "scanimage", dir, nil)
	data, _ := os.ReadFile(out)
	if rerr != nil && (len(data) > 0 || !strings.Contains(rerr.Error(), "produced no output")) {
		return nil, scanWording(rerr)
	}
	return parseScanDevices(string(data)), nil
}

// ScanPages scans and returns the pages as PNG files, in order. The caller has checked the device against the
// list it gave (the route does); the name is held to a shape here as well.
func ScanPages(ctx context.Context, o ScanOptions) ([][]byte, error) {
	if !validScanDeviceName(o.Device) {
		return nil, ErrScanDevice
	}
	tool, err := scanTool()
	if err != nil {
		return nil, err
	}
	budget := scanFlatbedTimeout
	if o.Feeder {
		budget = scanFeederTimeout
	}
	var pages [][]byte
	run := func(ctx context.Context) error {
		var rerr error
		pages, rerr = scanOnce(ctx, tool, o)
		return rerr
	}
	if err := ImportGate.Run(ctx, budget, nil, run); err != nil {
		return nil, scanWording(err)
	}
	return pages, nil
}

func scanOnce(ctx context.Context, tool string, o ScanOptions) ([][]byte, error) {
	dir, err := tempdir.New("scan")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	env := toolEnv(os.Environ(), dir.Path())
	args := []string{"--device-name=" + o.Device, fmt.Sprintf("--resolution=%d", o.DPI), "--mode=" + o.Mode, "--format=png"}
	if o.Feeder {
		hctx, hcancel := context.WithTimeout(ctx, scanHelpTimeout)
		defer hcancel()
		help := exec.CommandContext(hctx, tool, "--device-name="+o.Device, "--help")
		help.Env = env
		// `--help` writes to stdout, and a failure to open the device still writes the generic part; the
		// source line is what is wanted, so any output is read.
		txt, _ := help.Output()
		src, ok := feederSource(string(txt))
		if !ok {
			return nil, ErrScanNoFeeder
		}
		args = append(args, "--source="+src, "--batch="+dir.Join("page-%03d.png"), fmt.Sprintf("--batch-count=%d", scanMaxPages))
	} else {
		args = append(args, "--output-file="+dir.Join("page-001.png"))
	}
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Env = env
	first := dir.Join("page-001.png")
	_, rerr := runConvertPickOK(ctx, cmd, func() string { return first }, "scanimage", dir, func(code int) bool { return o.Feeder && code == scanOutOfPaper })
	files, _ := filepath.Glob(dir.Join("page-*.png"))
	sort.Strings(files)
	// Only the out-of-paper exit ends a feeder run well. Any other ending with pages already scanned (a jam, a
	// timeout) is refused whole: a document with the end of the stack missing, said nowhere, is the worse answer.
	if rerr != nil {
		return nil, rerr
	}
	var pages [][]byte
	for _, f := range files {
		if fi, serr := os.Stat(f); serr == nil && fi.Size() > maxConvertedBytes {
			return nil, ErrConvertedTooLarge
		}
		b, rerr := os.ReadFile(f)
		if rerr != nil || len(b) == 0 {
			continue
		}
		pages = append(pages, b)
	}
	if len(pages) == 0 {
		return nil, ErrScanNoPages
	}
	return pages, nil
}
