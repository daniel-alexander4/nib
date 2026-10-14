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

package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/log"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ExtractImages dumps embedded image resources from inFile into outDir for selected pages.
func ExtractImages(cmd *Command) ([]string, error) {
	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		return nil, api.ExtractImages(rs, cmd.PageSelection, api.WriteImageToDisk(*cmd.OutDir, "stdin"), cmd.Conf)
	}
	return nil, api.ExtractImagesFile(*cmd.InFile, *cmd.OutDir, cmd.PageSelection, cmd.Conf)
}

// ExtractFonts dumps embedded fontfiles from inFile into outDir for selected pages.
func ExtractFonts(cmd *Command) ([]string, error) {
	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		return nil, api.ExtractFonts(rs, cmd.PageSelection, api.WriteFontToDisk(*cmd.OutDir, "stdin"), cmd.Conf)
	}
	return nil, api.ExtractFontsFile(*cmd.InFile, *cmd.OutDir, cmd.PageSelection, cmd.Conf)
}

// ExtractPages generates single page PDF files from inFile in outDir for selected pages.
func ExtractPages(cmd *Command) ([]string, error) {
	if *cmd.OutDir == "-" {
		log.SetCLILogger(nil)

		rs, _, cleanup, err := streamInOut(*cmd.InFile, "-")
		if err != nil {
			return nil, err
		}
		if cleanup != nil {
			defer cleanup()
		}

		conf := cmd.Conf
		if conf == nil {
			conf = model.NewDefaultConfiguration()
		}
		conf.Cmd = model.EXTRACTPAGES

		ctx, err := api.ReadValidateAndOptimize(rs, conf)
		if err != nil {
			return nil, err
		}

		pages, err := api.PagesForPageSelection(ctx.PageCount, cmd.PageSelection, true, true)
		if err != nil {
			return nil, err
		}

		pageNr, count := 0, 0
		for i, v := range pages {
			if v {
				pageNr = i
				count++
			}
		}
		if count != 1 {
			return nil, fmt.Errorf("pdfcpu: extract page to stdout requires exactly one selected page")
		}

		r, err := api.ExtractPage(ctx, pageNr)
		if err != nil {
			return nil, err
		}

		_, err = io.Copy(os.Stdout, r)
		return nil, err
	}

	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		return nil, api.ExtractPages(rs, cmd.PageSelection, api.WritePageToDisk(*cmd.OutDir, "stdin"), cmd.Conf)
	}

	return nil, api.ExtractPagesFile(*cmd.InFile, *cmd.OutDir, cmd.PageSelection, cmd.Conf)
}

// ExtractContent dumps "PDF source" files from inFile into outDir for selected pages.
func ExtractContent(cmd *Command) ([]string, error) {
	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		return nil, api.ExtractContent(rs, cmd.PageSelection, api.WriteContentToDisk(*cmd.OutDir, "stdin"), cmd.Conf)
	}
	return nil, api.ExtractContentFile(*cmd.InFile, *cmd.OutDir, cmd.PageSelection, cmd.Conf)
}

// ExtractMetadata dumps all metadata dict entries for inFile into outDir.
func ExtractMetadata(cmd *Command) ([]string, error) {
	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		return nil, api.ExtractMetadata(rs, api.WriteMetadataToDisk(*cmd.OutDir, "stdin"), cmd.Conf)
	}

	return nil, api.ExtractMetadataFile(*cmd.InFile, *cmd.OutDir, cmd.Conf)
}
