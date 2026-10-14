/*
Copyright 2026 The pdfcpu Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package cli provides pdfcpu command line processing.
package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/pdfcpu/pdfcpu/pkg/log"
)

func readSeekerFromStdin() (io.ReadSeeker, error) {
	bb, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	if len(bb) == 0 {
		return nil, fmt.Errorf("pdfcpu: stdin is empty")
	}
	return bytes.NewReader(bb), nil
}

func streamInOut(inFile, outFile string) (io.ReadSeeker, io.Writer, func(), error) {
	var cleanup func()
	if inFile == "-" && outFile == "" {
		outFile = "-"
	}

	var rs io.ReadSeeker
	if inFile == "-" {
		var err error
		rs, err = readSeekerFromStdin()
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		f, err := os.Open(inFile)
		if err != nil {
			return nil, nil, nil, err
		}
		rs = f
		cleanup = func() {
			_ = f.Close()
		}
	}

	w := io.Writer(os.Stdout)
	if outFile == "-" {
		log.SetCLILogger(nil)
		return rs, w, cleanup, nil
	}

	f, err := os.Create(outFile)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, nil, err
	}
	prevCleanup := cleanup
	cleanup = func() {
		_ = f.Close()
		if prevCleanup != nil {
			prevCleanup()
		}
	}
	w = f

	return rs, w, cleanup, nil
}
