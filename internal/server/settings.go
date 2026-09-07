package server

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// settingsRequest is a partial update — only the non-nil fields are applied, so
// the UI can save one toggle without resending the others.
type settingsRequest struct {
	Appearance            *string   `json:"appearance"`
	CardHue               *string   `json:"cardHue"`
	CheckUpdatesOnStartup *bool     `json:"checkUpdatesOnStartup"`
	RecentHighlightColors *[]string `json:"recentHighlightColors"` // whole-list replace, newest first
}

// maxRecentHighlightColors caps the stored most-recently-used highlight palette.
const maxRecentHighlightColors = 5

var hexColorRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// sanitizeHighlightColors normalizes to lowercase #rrggbb, drops anything that
// isn't a valid hex color, dedupes (keeping first/newest), and caps the list. The
// client owns the MRU ordering; this is the server-side guard on what gets stored.
func sanitizeHighlightColors(in []string) []string {
	out := make([]string, 0, maxRecentHighlightColors)
	seen := map[string]bool{}
	for _, c := range in {
		c = strings.ToLower(strings.TrimSpace(c))
		if !hexColorRe.MatchString(c) || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
		if len(out) == maxRecentHighlightColors {
			break
		}
	}
	return out
}

// handleSettings persists the user's UI preferences (appearance, auto-update check,
// recent highlight colours) into the vault. The current values are read back via
// /api/status.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	v := vaultFrom(r)
	cur := v.Settings()
	if req.Appearance != nil {
		switch *req.Appearance {
		// Two Catppuccin flavours: "dark" is Mocha and "light" is Latte. Frappé and Macchiato
		// were accepted here between v1.123.0 and v1.123.4 and are not any more — a vault may
		// still HOLD one, and the client normalises it to dark on the way in (applyAppearance),
		// so what is rejected here is only a new save of a flavour that no longer has a palette.
		case "dark", "light":
			cur.Appearance = *req.Appearance
		default:
			httpError(w, http.StatusBadRequest, "invalid appearance")
			return
		}
	}
	if req.CardHue != nil {
		switch *req.CardHue {
		// The six accents the stylesheet defines, plus "all" for the rotation. Kept in step with
		// web/style.css's `:root[data-cardhue=…]` blocks and with CARD_HUES in web/app.js —
		// test/jsdom/theme.test.mjs compares the three lists, for the same reason it compares the
		// theme ones: each disagreement fails silently and differently.
		case "all", "blue", "mauve", "green", "peach", "red", "yellow":
			cur.CardHue = *req.CardHue
		default:
			httpError(w, http.StatusBadRequest, "invalid cardHue")
			return
		}
	}
	if req.CheckUpdatesOnStartup != nil {
		cur.DisableAutoUpdate = !*req.CheckUpdatesOnStartup
	}
	if req.RecentHighlightColors != nil {
		cur.RecentHighlightColors = sanitizeHighlightColors(*req.RecentHighlightColors)
	}
	if err := v.SetSettings(cur); err != nil {
		httpError(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
