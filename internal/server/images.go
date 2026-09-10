package server

import (
	"errors"
	"io"
	"net/http"

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
	// **The remote-image-by-URL branch was deleted here** (`/pending 449`). It branched on
	// `Content-Type: application/json` into `safeFetch`, and nothing in the tree posted JSON to
	// this route: both client sites post multipart `{file, name}`, and `build/`, `test/` and
	// `internal/cli/` have no sender either.
	//
	// **It was not a hole and it is not deleted as one.** `safeFetch` is the same primitive
	// `handleOpenURL` exposes and the client DOES reach, and this route sits behind
	// `requireUnlocked`, CSRF and loopback. What it was is an unreachable fetch of an arbitrary
	// URL on a request path — exercised by no test, considered by no reviewer, and one JSON
	// client away from going live without either.
	//
	// "Add an image from a URL" may well be worth building. An implementation with no surface
	// is not that feature; it is a claim the feature exists. If it is wanted it needs a control,
	// a decision and a test, and none of those is made cheaper by this branch having survived.
	{
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
