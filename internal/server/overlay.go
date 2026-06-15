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

	// Covers are opaque fills baked UNDERNEATH the text — the cover half of a
	// cover-and-replace edit. They must go on before StampFields so the
	// replacement text lands on top of them, not behind.
	if raw := r.FormValue("covers"); raw != "" {
		var reqs []stampReq
		if err := json.Unmarshal([]byte(raw), &reqs); err != nil {
			httpError(w, http.StatusBadRequest, "invalid covers")
			return
		}
		covers := make([]pdfops.Stamp, 0, len(reqs))
		for _, q := range reqs {
			if q.PNG == "" {
				continue
			}
			b, err := base64.StdEncoding.DecodeString(q.PNG)
			if err != nil {
				httpError(w, http.StatusBadRequest, "invalid cover png")
				return
			}
			covers = append(covers, pdfops.Stamp{Page: q.Page, Rect: q.Rect, PNG: b})
		}
		var err error
		if pdfBytes, err = pdfops.StampImages(pdfBytes, covers); err != nil {
			httpError(w, http.StatusInternalServerError, "could not stamp covers: "+err.Error())
			return
		}
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

// handleFlags embeds (or strips) the document's signing placeholders. A non-empty
// "flags" form value is stored as the NibFlags property so the saved-for-signing
// PDF carries its sign/date/initial markers to the recipient in a single file; an
// absent/empty value strips the property, which the recipient's final save does
// once the markers are filled (the bake preserves it, so it must go explicitly).
func (s *Server) handleFlags(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}

	var out []byte
	var err error
	if raw := r.FormValue("flags"); raw != "" {
		if !json.Valid([]byte(raw)) {
			httpError(w, http.StatusBadRequest, "invalid flags")
			return
		}
		out, err = pdfops.SetFlags(pdfBytes, []byte(raw))
	} else {
		out, err = pdfops.ClearFlags(pdfBytes)
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not update flags: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(out)
}
