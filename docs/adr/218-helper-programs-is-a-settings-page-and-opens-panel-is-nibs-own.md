# ADR-218 — Helper Programs is a Settings page, and Open…'s panel is Nib's own

Status: accepted, 2026-11-07 (Dan: "if helper is installed, make the parens a light green, if not, a light red", "put
my mouse on the tooltip to scroll", "click on the libreoffice not installed and have it take me to the setting page",
"create a settings page for the helper programs … a short summary of what they are, how nib uses them, an indicator
if they are installed or not and a link to their site"; placed after Toggle Features). Supersedes ADR-118's "Settings
is SIX entries" and ADR-107's order only by inserting this entry; extends ADR-215, ADR-217.

1. **Settings is seven entries**: Appearance, Read Aloud, Toggle Features, Helper Programs, Identity & Keys, Vault,
   About. `#settingsHelpersPage` lists six programs (LibreOffice, Ghostscript, a HEIF decoder, a Chromium-family
   browser, pdftocairo, scanimage): what it is, what Nib uses it for, an indicator written by `paintHelpers` from
   `/api/status` (green "Installed", red "Not found on this computer"), a link to its site, and one Check again.
   wmctrl and xdotool are not listed: Nib reports nothing about them and they change nothing about what opens.
2. **The links are written in the page** (ADR-039) and held equal to `MissingTool.Vendor` by
   `TestTheHelperPageLinksAreTheVendors`. SANE's link is its project site on GitLab Pages, because
   www.sane-project.org does not answer over HTTPS (measured 2026-11-07).
3. **Open… is described by `#openFormats`, a panel Nib draws, not a `title`.** A browser's tooltip cannot be coloured,
   cannot be scrolled and vanishes when the mouse leaves the button. The panel shows on hover or focus, stays while the
   mouse is on it, scrolls past 70% of the window, goes on Escape, and is built from text nodes only.
4. **Colours are tokens per theme** (`--found-text`, `--missing-text`). The dark theme uses the palette's own green and
   red (11.8:1, 7.6:1 on `--mantle`); the light palette's are below AA for small text (2.75:1, 4.46:1), so the light
   theme takes deeper shades of the same hues (5.3:1, 5.9:1). The words say the same as the colour.
5. **A line whose program was not found is a link** to that program's section (`goHelper`, through the status's
   `toolKey` = `MissingTool.Key`). A found or built-in line is not a link.
