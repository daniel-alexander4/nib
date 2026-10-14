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
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/log"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// AddWatermarks adds watermarks or stamps to selected pages of inFile and writes the result to outFile.
func AddWatermarks(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		return nil, api.AddWatermarksFile(*cmd.InFile, *cmd.OutFile, cmd.PageSelection, cmd.Watermark, cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return nil, api.AddWatermarks(rs, w, cmd.PageSelection, cmd.Watermark, cmd.Conf)
}

// RemoveWatermarks remove watermarks or stamps from selected pages of inFile and writes the result to outFile.
func RemoveWatermarks(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		return nil, api.RemoveWatermarksFile(*cmd.InFile, *cmd.OutFile, cmd.PageSelection, cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return nil, api.RemoveWatermarks(rs, w, cmd.PageSelection, cmd.Conf)
}

func listAnnotations(rs io.ReadSeeker, selectedPages []string, json bool, conf *model.Configuration) (int, []string, error) {
	if json {
		log.SetCLILogger(nil)
	}
	annots, err := api.Annotations(rs, selectedPages, conf)
	if err != nil {
		return 0, nil, err
	}
	if json {
		return pdfcpu.ListAnnotationsJSON(annots)
	}

	return pdfcpu.ListAnnotations(annots)
}

func listAnnotationsFile(inFile string, selectedPages []string, json bool, conf *model.Configuration) (int, []string, error) {
	f, err := os.Open(inFile)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()

	return listAnnotations(f, selectedPages, json, conf)
}

// ListAnnotationsFile returns a list of page annotations of inFile.
func ListAnnotationsFile(inFile string, selectedPages []string, conf *model.Configuration) (int, []string, error) {
	return listAnnotationsFile(inFile, selectedPages, false, conf)
}

// ListAnnotationsJSONFile returns a JSON list of page annotations of inFile.
func ListAnnotationsJSONFile(inFile string, selectedPages []string, conf *model.Configuration) (int, []string, error) {
	return listAnnotationsFile(inFile, selectedPages, true, conf)
}

// ListAnnotations returns inFile's page annotations.
func ListAnnotations(cmd *Command) ([]string, error) {
	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		_, ss, err := listAnnotations(rs, cmd.PageSelection, cmd.BoolVal1, cmd.Conf)
		return ss, err
	}

	_, ss, err := listAnnotationsFile(*cmd.InFile, cmd.PageSelection, cmd.BoolVal1, cmd.Conf)
	return ss, err
}

// RemoveAnnotations deletes annotations from inFile's page tree and writes the result to outFile.
func RemoveAnnotations(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		incr := false // No incremental writing on cli.
		return nil, api.RemoveAnnotationsFile(*cmd.InFile, *cmd.OutFile, cmd.PageSelection, cmd.StringVals, cmd.IntVals, cmd.Conf, incr)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return nil, api.RemoveAnnotations(rs, w, cmd.PageSelection, cmd.StringVals, cmd.IntVals, cmd.Conf)
}

func listBookmarks(rs io.ReadSeeker, conf *model.Configuration) ([]string, error) {
	if rs == nil {
		return nil, errors.New("pdfcpu: listBookmarks: missing rs")
	}

	if conf == nil {
		conf = model.NewDefaultConfiguration()
	} else {
		conf.ValidationMode = model.ValidationRelaxed
	}
	conf.Cmd = model.LISTBOOKMARKS

	ctx, err := api.ReadAndValidate(rs, conf)
	if err != nil {
		return nil, err
	}

	return pdfcpu.BookmarkList(ctx)
}

// ListBookmarksFile returns the bookmarks of inFile.
func ListBookmarksFile(inFile string, conf *model.Configuration) ([]string, error) {
	f, err := os.Open(inFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return listBookmarks(f, conf)
}

// ListBookmarks returns inFile's outlines.
func ListBookmarks(cmd *Command) ([]string, error) {
	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		return listBookmarks(rs, cmd.Conf)
	}

	return ListBookmarksFile(*cmd.InFile, cmd.Conf)
}

// ExportBookmarks returns a representation of inFile's outlines as outFileJSON.
func ExportBookmarks(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" {
		return nil, api.ExportBookmarksFile(*cmd.InFile, *cmd.OutFileJSON, cmd.Conf)
	}

	rs, err := readSeekerFromStdin()
	if err != nil {
		return nil, err
	}

	f, err := os.Create(*cmd.OutFileJSON)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return nil, api.ExportBookmarksJSON(rs, f, "stdin", cmd.Conf)
}

// ImportBookmarks creates/replaces outlines of inFile corresponding to declarations found in inJSONFile and writes the result to outFile.
func ImportBookmarks(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		return nil, api.ImportBookmarksFile(*cmd.InFile, *cmd.InFileJSON, *cmd.OutFile, cmd.BoolVal1, cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	f, err := os.Open(*cmd.InFileJSON)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return nil, api.ImportBookmarks(rs, f, w, cmd.BoolVal1, cmd.Conf)
}

// RemoveBookmarks erases outlines of inFile.
func RemoveBookmarks(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		return nil, api.RemoveBookmarksFile(*cmd.InFile, *cmd.OutFile, cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return nil, api.RemoveBookmarks(rs, w, cmd.Conf)
}

// ListPageLayout returns inFile's page layout.
func ListPageLayout(cmd *Command) ([]string, error) {
	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		return api.ListPageLayout(rs, cmd.Conf)
	}

	return api.ListPageLayoutFile(*cmd.InFile, cmd.Conf)
}

// SetPageLayout sets inFile's page layout.
func SetPageLayout(cmd *Command) ([]string, error) {
	pageLayout := model.PageLayoutFor(cmd.StringVal)
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		return nil, api.SetPageLayoutFile(*cmd.InFile, *cmd.OutFile, *pageLayout, cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return nil, api.SetPageLayout(rs, w, *pageLayout, cmd.Conf)
}

// ResetPageLayout resets inFile's page layout.
func ResetPageLayout(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		return nil, api.ResetPageLayoutFile(*cmd.InFile, *cmd.OutFile, cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return nil, api.ResetPageLayout(rs, w, cmd.Conf)
}

// ListPageMode returns inFile's page mode.
func ListPageMode(cmd *Command) ([]string, error) {
	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		return api.ListPageMode(rs, cmd.Conf)
	}

	return api.ListPageModeFile(*cmd.InFile, cmd.Conf)
}

// SetPageMode sets inFile's page mode.
func SetPageMode(cmd *Command) ([]string, error) {
	pageMode := model.PageModeFor(cmd.StringVal)
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		return nil, api.SetPageModeFile(*cmd.InFile, *cmd.OutFile, *pageMode, cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return nil, api.SetPageMode(rs, w, *pageMode, cmd.Conf)
}

// ResetPageMode resets inFile's page mode.
func ResetPageMode(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		return nil, api.ResetPageModeFile(*cmd.InFile, *cmd.OutFile, cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return nil, api.ResetPageMode(rs, w, cmd.Conf)
}

// ListViewerPreferences returns inFile's viewer preferences.
func ListViewerPreferences(cmd *Command) ([]string, error) {
	if *cmd.InFile == "-" {
		rs, err := readSeekerFromStdin()
		if err != nil {
			return nil, err
		}
		if !cmd.BoolVal2 {
			return api.ListViewerPreferences(rs, cmd.BoolVal1, cmd.Conf)
		}

		vp, version, err := api.ViewerPreferences(rs, cmd.Conf)
		if err != nil {
			return nil, err
		}
		if !cmd.BoolVal1 {
			if vp == nil {
				return []string{"No viewer preferences available."}, nil
			}
		} else {
			vp, err = model.ViewerPreferencesWithDefaults(vp, *version)
			if err != nil {
				return nil, err
			}
		}

		s := struct {
			Header     pdfcpu.Header            `json:"header"`
			ViewerPref *model.ViewerPreferences `json:"viewerPreferences"`
		}{
			Header:     pdfcpu.Header{Version: "pdfcpu " + model.VersionStr, Creation: time.Now().Format("2006-01-02 15:04:05 MST")},
			ViewerPref: vp,
		}

		bb, err := json.MarshalIndent(s, "", "\t")
		if err != nil {
			return nil, err
		}
		return []string{string(bb)}, nil
	}

	return api.ListViewerPreferencesFile(*cmd.InFile, cmd.BoolVal1, cmd.BoolVal2, cmd.Conf)
}

// SetViewerPreferences sets inFile's viewer preferences.
func SetViewerPreferences(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		if *cmd.InFileJSON != "" {
			return nil, api.SetViewerPreferencesFileFromJSONFile(*cmd.InFile, *cmd.OutFile, *cmd.InFileJSON, cmd.Conf)
		}
		return nil, api.SetViewerPreferencesFileFromJSONBytes(*cmd.InFile, *cmd.OutFile, []byte(cmd.StringVal), cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	if *cmd.InFileJSON != "" {
		f, err := os.Open(*cmd.InFileJSON)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return nil, api.SetViewerPreferencesFromJSONReader(rs, w, f, cmd.Conf)
	}
	return nil, api.SetViewerPreferencesFromJSONBytes(rs, w, []byte(cmd.StringVal), cmd.Conf)
}

// ResetViewerPreferences resets inFile's viewer preferences.
func ResetViewerPreferences(cmd *Command) ([]string, error) {
	if *cmd.InFile != "-" && *cmd.OutFile != "-" {
		return nil, api.ResetViewerPreferencesFile(*cmd.InFile, *cmd.OutFile, cmd.Conf)
	}

	rs, w, cleanup, err := streamInOut(*cmd.InFile, *cmd.OutFile)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return nil, api.ResetViewerPreferences(rs, w, cmd.Conf)
}
