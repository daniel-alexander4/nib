package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"nib/internal/pdfops"
)

// stampReq is an image stamp posted alongside text fields: either a library
// image (Image = its id, bytes resolved server-side) or an inline base64 PNG
// (PNG, used for the drawn Y/N circle).
type stampReq struct {
	Page  int        `json:"page"`
	Rect  [4]float64 `json:"rect"`
	Image string     `json:"image"`
	PNG   string     `json:"png"`
}

// handleBake stamps overlay-field values (from auto-detected lines/boxes/
// checkboxes) onto the posted PDF and returns the result. It's the canonical
// "current filled document" the client uses before saving, flattening, or
// exporting — overlay fields are HTML widgets, so they must be baked here.
func (s *Server) handleBake(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	var fields []pdfops.Field
	if raw := r.FormValue("fields"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			httpError(w, http.StatusBadRequest, "invalid fields")
			return
		}
	}
	out, err := pdfops.StampFields(pdfBytes, fields)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not stamp fields: "+err.Error())
		return
	}

	if raw := r.FormValue("stamps"); raw != "" {
		var reqs []stampReq
		if err := json.Unmarshal([]byte(raw), &reqs); err != nil {
			httpError(w, http.StatusBadRequest, "invalid stamps")
			return
		}
		v := vaultFrom(r)
		stamps := make([]pdfops.Stamp, 0, len(reqs))
		for _, q := range reqs {
			var png []byte
			switch {
			case q.Image != "":
				img, ok := v.Image(q.Image)
				if !ok {
					continue
				}
				png = img.Data
			case q.PNG != "":
				b, err := base64.StdEncoding.DecodeString(q.PNG)
				if err != nil {
					httpError(w, http.StatusBadRequest, "invalid stamp png")
					return
				}
				png = b
			default:
				continue
			}
			stamps = append(stamps, pdfops.Stamp{Page: q.Page, Rect: q.Rect, PNG: png})
		}
		out, err = pdfops.StampImages(out, stamps)
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not stamp images: "+err.Error())
			return
		}
	}

	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(out)
}
