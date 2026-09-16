package server

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode"

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
	// ReadAloudVoice is a voice NAME from the browser's list, "" for the default; ReadAloudRate is
	// the speed, 1 for the default (/pending 482). Both are stored as absence when they are the default.
	ReadAloudVoice *string  `json:"readAloudVoice"`
	ReadAloudRate  *float64 `json:"readAloudRate"`
	// HiddenModes is the whole set of switched-off main-menu tabs, sent as one list (ADR-036).
	//
	// A list rather than one id and a bool, for `Advanced`'s reason one field down: a partial
	// update cannot express "configured, and nothing is hidden". An empty array arriving means the
	// user unticked the last box; an absent field means they were changing something else.
	HiddenModes *[]string `json:"hiddenModes"`
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

// The read-aloud bounds (/pending 482): a voice name is a label, and the speeds are the ones the
// Settings card offers.
const (
	maxVoiceNameBytes = 200
	minReadAloudRate  = 0.5
	maxReadAloudRate  = 2
)

// hideableModes are the main-menu tabs a user may switch off (ADR-036).
//
// **`settings` is deliberately absent, and that is a safety property rather than a preference.**
// The control that un-hides a mode is a card inside Settings, so a hidden Settings could only be
// undone by editing the vault — a state the product cannot talk its way out of.
//
// **A second source of truth for the mode set, and the repo's own answer to that is a test rather
// than cleverness.** `web/index.html` declares the modes and this list must agree with it;
// `TestEveryHideableModeIsARealTab` compares the two, the same shape `theme.test.mjs` uses to hold
// the stylesheet, the Go whitelist and the picker together. A mode added to the markup and not to
// this list is refused by the route with no symptom anywhere else.
var hideableModes = map[string]bool{
	"file": true, "markup": true, "edit": true, "accessibility": true, "secure": true, "collaborate": true,
}

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
		// **"presentation" is refused rather than stored.** It is something you are doing for the
		// next ten minutes, not how you like to read, and an app that reopened full screen because
		// of a meeting last Tuesday would be wrong in a way the user cannot diagnose. The client
		// does not send it; this is what makes that a property of the product rather than a habit.
		default:
			httpError(w, http.StatusBadRequest, "invalid viewLayout")
			return
		}
	}
	if req.ReadAloudVoice != nil {
		// A voice name is the browser's label, stored and handed back; it is bounded and refused if it
		// carries control characters, so the vault cannot be made to hold something that is not a label.
		name := *req.ReadAloudVoice
		if len(name) > maxVoiceNameBytes || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			httpError(w, http.StatusBadRequest, "invalid readAloudVoice")
			return
		}
		cur.ReadAloudVoice = name
	}
	if req.ReadAloudRate != nil {
		// 1 is the default and is stored as absence, for ViewLayout's reason. The range is the one the
		// client offers; a rate outside it is refused rather than clamped into a speed nobody chose.
		switch rate := *req.ReadAloudRate; {
		case rate == 1:
			cur.ReadAloudRate = 0
		case rate >= minReadAloudRate && rate <= maxReadAloudRate:
			cur.ReadAloudRate = rate
		default:
			httpError(w, http.StatusBadRequest, "invalid readAloudRate")
			return
		}
	}
	if req.HiddenModes != nil {
		// Refused rather than filtered, for the reason `viewLayout` above is: a request naming a
		// mode this build does not have is a client and a server that disagree about the menu, and
		// silently dropping the unknown id stores a set the user did not choose while answering
		// "ok". `settings` is refused by the same branch — it is not in `hideableModes`.
		seen := map[string]bool{}
		out := make([]string, 0, len(*req.HiddenModes))
		for _, m := range *req.HiddenModes {
			if !hideableModes[m] {
				httpError(w, http.StatusBadRequest, "invalid hiddenModes: "+m)
				return
			}
			if seen[m] {
				continue
			}
			seen[m] = true
			out = append(out, m)
		}
		// Empty is stored as absence, per `HiddenModes`' own rule: nothing hidden and never asked
		// are one state, and both mean every mode shows.
		if len(out) == 0 {
			cur.HiddenModes = nil
		} else {
			cur.HiddenModes = out
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
	// **Applied under ONE hold of the lock (`/pending 519`).** Everything above is validation and
	// refusal — five branches answer 4xx and return, and the advanced branch reads the filesystem
	// through `hasLiveCeremony` — so it all happens BEFORE the door, and what goes inside is only
	// the settled values. `UpdateSettings` runs this function while holding the vault's mutex, which
	// is not reentrant: nothing in here may call back into `v`.
	//
	// `cur` was read at the top of the handler and the fields the request did not name still carry
	// what was stored then, so this assigns the whole struct rather than the changed fields — which
	// is what `SetSettings` did. The difference the door makes is that a concurrent writer can no
	// longer land between that read and this write; the seed, the other writer, now no-ops under the
	// same lock when the user has already answered.
	if err := v.UpdateSettings(func(s *vault.Settings) { *s = cur }); err != nil {
		httpError(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
