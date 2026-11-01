package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"nib/internal/pdfops"
)

// imageRow is one line of `nib images --json`. Its keys are part of the machine-readable contract
// (docs/cli-machine-readable.md, ADR-183): page and index are from 1, index counting the page's
// pictures by ascending object number (the one order pdfcpu does not shuffle between runs); filter is the stream's own filter pipeline; bytes is the
// written file's length.
type imageRow struct {
	Page   int    `json:"page"`
	Index  int    `json:"index"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Filter string `json:"filter"`
	Bytes  int    `json:"bytes"`
	File   string `json:"file"`
}

// cmdImages writes a PDF's embedded pictures into a folder, each in the format its stream holds where
// that can be done (a JPEG stays the same JPEG) and otherwise re-encoded by pdfcpu. It exists so a
// program can read a scan's pages, each one picture, without rendering them.
func cmdImages(args []string) int {
	fs := flag.NewFlagSet("nib images", flag.ContinueOnError)
	var outDir string
	var asJSON bool
	fs.StringVar(&outDir, "out-dir", "", "the folder to write each picture into (created if missing; an existing file of the same name is never overwritten) (required)")
	fs.BoolVar(&asJSON, "json", false, "print each picture's page, index, width, height, filter, bytes and file as JSON")
	fs.Usage = usageFunc(fs, "nib images IN --out-dir DIR [--json]", "Extract a PDF's embedded pictures into DIR as page-NNN-img-NN.EXT. A JPEG is written as the same JPEG; other pictures are re-encoded as PNG or TIFF. IN may be - (stdin). A document with none writes nothing and lists nothing.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if outDir == "" {
		errf("missing --out-dir (where to write the pictures)")
		return 1
	}
	if fs.NArg() != 1 {
		errf("expected one input PDF, got %d", fs.NArg())
		return 1
	}
	pdf, err := readInput(fs.Arg(0))
	if err != nil {
		errf("%v", err)
		return 1
	}
	files, err := pdfops.ExtractImageFiles(pdf)
	if err != nil {
		errf("%s: %v", inputName(fs.Arg(0)), err)
		return 1
	}
	rows := make([]imageRow, 0, len(files))
	if len(files) > 0 {
		if err := os.MkdirAll(outDir, 0o700); err != nil {
			errf("%v", err)
			return 1
		}
		// SELF-OVERWRITE EXEMPT: this writer replaces NOTHING. Every file is opened O_EXCL, and the
		// loop below refuses the whole extraction when ANY name already exists, so a document sitting
		// in DIR (the case /pending 569 is about) can never be written over; the names end in an image
		// extension and are never the input's anyway.
		//
		// Refuse before writing anything: a rerun that overwrote an earlier extraction would leave a
		// folder that is half one run and half the other.
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = fmt.Sprintf("page-%03d-img-%02d.%s", f.Page, f.Index, f.Ext)
			if _, err := os.Lstat(filepath.Join(outDir, names[i])); err == nil {
				errf("%s already exists in %s: nothing was written", names[i], outDir)
				return 1
			}
		}
		for i, f := range files {
			// 0600: a picture out of a return is as private as the return.
			w, err := os.OpenFile(filepath.Join(outDir, names[i]), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err == nil {
				_, err = w.Write(f.Data)
				if cerr := w.Close(); err == nil {
					err = cerr
				}
			}
			if err != nil {
				errf("%v", err)
				return 1
			}
			rows = append(rows, imageRow{f.Page, f.Index, f.Width, f.Height, f.Filter, len(f.Data), names[i]})
		}
	}
	if asJSON {
		b, _ := json.Marshal(rows)
		fmt.Println(string(b))
		return 0
	}
	for _, r := range rows {
		fmt.Printf("page %d  #%d  %dx%d  %s  %d bytes  %s\n", r.Page, r.Index, r.Width, r.Height, r.Filter, r.Bytes, r.File)
	}
	if len(rows) == 0 {
		fmt.Println("(no embedded pictures)")
	}
	return 0
}
