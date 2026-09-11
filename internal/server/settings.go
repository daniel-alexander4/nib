package server

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"nib/internal/vault"
)

// settingsRequest is a partial update — only the non-nil fields are applied, so
// the UI can save one toggle without resending the others.
type settingsRequest struct {
	Appearance            *string   `json:"appearance"`
	CardHue               *string   `json:"cardHue"`
	CheckUpdatesOnStartup *bool     `json:"checkUpdatesOnStartup"`
	RecentHighlightColors *[]string `json:"recentHighlightColors"` // whole-list replace, newest first
	// ViewLayout is "pages" or "continuous"; anything else is refused rather than stored, so a
	// future build cannot be handed a layout name this one invented.
	ViewLayout *string `json:"viewLayout"`
	// Advanced is the four subsystem switches, sent whole.
	//
	// **A nested object rather than four top-level pointers**, because the partial-update shape
	// this struct uses everywhere else cannot express "configured, and all four off": four absent
	// pointers is what a client sends when it is changing something else entirely. One object
	// arriving means the user was on the Advanced card, and every field in it is their answer.
	Advanced *advancedRequest `json:"advanced"`
}

// advancedRequest mirrors vault.Advanced. Separate from it so the wire shape and the stored shape
// can move independently — the same reason every other request struct here is its own type.
type advancedRequest struct {
	Ceremony   bool `json:"ceremony"`
	Discovery  bool `json:"discovery"`
	Rendezvous bool `json:"rendezvous"`
	Timestamp  bool `json:"timestamp"`
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
	if req.ViewLayout != nil {
		// **"pages" is stored as EMPTY, which is the whole reason this is a switch and not an
		// assignment.** The default and the absence have to be one state on disk: every field
		// here is `omitempty`, so a build that does not know `viewLayout` drops it on re-save,
		// and a user whose stored value was the literal "pages" would come back on a layout they
		// never chose rather than on the default. Refused rather than coerced for anything else,
		// so this build cannot be handed a layout name it does not implement.
		switch *req.ViewLayout {
		case "continuous":
			cur.ViewLayout = "continuous"
		case "pages", "":
			cur.ViewLayout = ""
		default:
			httpError(w, http.StatusBadRequest, "invalid viewLayout")
			return
		}
	}
	if req.Advanced != nil {
		// **Refused while a proceeding is live**, and this is the whole of what `/pending 451`
		// called its hardest question. `Termination` carries two attested end states and `Receipt`
		// two derived ones; *"the user switched the feature off"* is none of them, so ending a
		// running ceremony this way would have to be recorded as `abandoned` or `stopped` — both
		// false statements to every other party, which is the class `/pending 428` closed on. The
		// only outcome the record can express is to refuse the change and say what is running.
		if cur.Advanced != nil && cur.Advanced.Ceremony && !req.Advanced.Ceremony && hasLiveCeremony() {
			httpError(w, http.StatusConflict,
				"a signing ceremony on this machine has not finished, so switching ceremonies off "+
					"now would leave it unreachable with no way to say what happened. End or leave "+
					"it first, then switch this off")
			return
		}
		cur.Advanced = &vault.Advanced{
			Ceremony:   req.Advanced.Ceremony,
			Discovery:  req.Advanced.Discovery,
			Rendezvous: req.Advanced.Rendezvous,
			Timestamp:  req.Advanced.Timestamp,
		}
	}
	if err := v.SetSettings(cur); err != nil {
		httpError(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
