package server

import (
	"encoding/json"
	"io"
	"net/http"
)

// settingsRequest is a partial update — only the non-nil fields are applied, so
// the UI can save one toggle without resending the others.
type settingsRequest struct {
	ToolbarStyle          *string `json:"toolbarStyle"`
	CheckUpdatesOnStartup *bool   `json:"checkUpdatesOnStartup"`
}

// handleSettings persists the user's UI preferences (toolbar layout, auto-update
// check) into the vault. The current values are read back via /api/status.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	v := vaultFrom(r)
	cur := v.Settings()
	if req.ToolbarStyle != nil {
		switch *req.ToolbarStyle {
		case "menus", "toolbar", "both":
			cur.ToolbarStyle = *req.ToolbarStyle
		default:
			httpError(w, http.StatusBadRequest, "invalid toolbar style")
			return
		}
	}
	if req.CheckUpdatesOnStartup != nil {
		cur.DisableAutoUpdate = !*req.CheckUpdatesOnStartup
	}
	if err := v.SetSettings(cur); err != nil {
		httpError(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
