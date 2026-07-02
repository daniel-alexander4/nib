package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"nib/internal/vault"
)

const maxImageBytes = 16 << 20 // 16 MiB per library image

// imageMeta is the non-sensitive description returned to the UI; the bytes are
// fetched separately from /api/images/{id}.
type imageMeta struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	MIME    string `json:"mime"`
	Builtin bool   `json:"builtin,omitempty"` // binary-shipped, read-only
}

func (s *Server) handleImagesList(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	out := make([]imageMeta, 0, len(v.Images())+len(v.BuiltinImages()))
	for _, img := range v.Images() {
		out = append(out, imageMeta{ID: img.ID, Name: img.Name, MIME: img.MIME})
	}
	for _, img := range v.BuiltinImages() {
		out = append(out, imageMeta{ID: img.ID, Name: img.Name, MIME: img.MIME, Builtin: true})
	}
	writeJSON(w, out)
}

func (s *Server) handleImageGet(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	img, ok := v.Image(r.PathValue("id"))
	if !ok {
		httpError(w, http.StatusNotFound, "no such image")
		return
	}
	w.Header().Set("Content-Type", img.MIME)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(img.Data)
}

func (s *Server) handleImageDelete(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	if err := v.DeleteImage(r.PathValue("id")); err != nil {
		if errors.Is(err, vault.ErrReadOnlyImage) {
			httpError(w, http.StatusForbidden, "built-in image can't be deleted")
			return
		}
		httpError(w, http.StatusInternalServerError, "could not delete image")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleImageAdd stores a new image, either uploaded as multipart form data or
// fetched from a URL (server-side, like /api/open). Only PNG and JPEG are
// accepted — a signature is ideally a transparent PNG.
func (s *Server) handleImageAdd(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)

	var (
		name string
		data []byte
	)
	if r.Header.Get("Content-Type") == "application/json" {
		var req struct{ URL, Name string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		b, err := safeFetch(req.URL, maxImageBytes, 15*time.Second)
		if err != nil {
			httpError(w, http.StatusBadGateway, "could not fetch image")
			return
		}
		name, data = req.Name, b
	} else {
		cleanup, pok := parseMultipart(w, r, maxImageBytes)
		if !pok {
			return
		}
		defer cleanup()
		file, header, err := r.FormFile("file")
		if err != nil {
			httpError(w, http.StatusBadRequest, "no file uploaded")
			return
		}
		defer file.Close()
		data, err = io.ReadAll(file)
		if err != nil {
			httpError(w, http.StatusBadRequest, "could not read upload")
			return
		}
		name = header.Filename
		if formName := r.FormValue("name"); formName != "" {
			name = formName
		}
	}

	mime := http.DetectContentType(data)
	if mime != "image/png" && mime != "image/jpeg" {
		httpError(w, http.StatusUnsupportedMediaType, "only PNG and JPEG images are supported")
		return
	}
	if name == "" {
		name = "image"
	}
	img, err := v.AddImage(name, mime, data)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not store image")
		return
	}
	writeJSON(w, imageMeta{ID: img.ID, Name: img.Name, MIME: img.MIME})
}
