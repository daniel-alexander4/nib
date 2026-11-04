# ADR-200 — A web page is printed only from an origin that serves it once and reaches nothing else

Status: accepted, 2026-11-03 (PLAN-import-export P04.S04a, v2.42.0). Implements D29, R3, X4, X5, Y8, Y10; extends ADR-157, ADR-185.

**The door.** One table row, `html` (extensions `html`, `htm`, `xhtml`; Tool `ToolBrowser`; `FamilyHTML`), no `Door` flag.
The family is sniffed (`isHTML`): after a byte-order mark and white space the file opens `<!doctype html` or `<html`, or opens
with a tag and an `<html`, `<head` or `<body` follows in the head. A name that says text keeps markup as text (a Markdown file
may open with a tag); a name that says nothing is not guessed; a fragment under `.html` is a page by its name. MHTML and EPUB
are S04b.

**Three layers, none trusting the others.**

1. *The page is rebuilt, not filtered* (`sanitizeHTML`, x/net/html). A stranger's markup is parsed and a new document is
   built from an allowlist of tags and attributes; what is not named is not copied. Script, frames, objects, SVG, forms,
   `<link>`, `<meta>`, `<base>` and `<title>` go whole. A style sheet or attribute is kept only if it holds no `@import`,
   `url(`, `image-set(`, `src:`, backslash or `@font-face`. A link keeps only `http`, `https` and `mailto`. A picture is kept
   only as a `data:` URL of four image types, 8 MiB each and 64 MiB together; anything else becomes its `alt` text. No file
   path is ever derived from the page.
2. *The browser gets no file and no network* (`internal/browserrender`). The sanitised page is served from a listener on
   `127.0.0.1` that lives for one request: a path of 256 random bits, a `Host` of `nib-render.test:<port>`, the peer's
   user checked per request through `internal/peercred` (the ADR-157 lookup, moved to a leaf so both doors can import it),
   `Connection: close`, a 30 s lifetime, `Close` on every path. A wrong host, user, path or method is a 404 that does not
   spend the use. The browser's resolver is told one name and that every other name does not exist
   (`MAP nib-render.test 127.0.0.1:<port>, MAP * ~NOTFOUND`), `--no-proxy-server`, a fresh 0700 profile, no `--no-sandbox`,
   and the Content-Security-Policy is a response header, so no markup can drop it.
3. *The render is proved to have been the page* (X4 c). The origin records that the document was written whole to the one
   request; a browser that wrote a PDF without asking for it (a retry after the use was spent gets a 404 and may print a blank
   page) is `ErrNotServed`. The PDF must have pages and is held to the 1,000-page cap.

**The browser is asked, once per binary, whether it can print at all** (`renderProbe`, cached by path, size and time,
refusals included): the whole door renders a one-line page and one page must come back. That is what refuses a snap-confined
Chromium that cannot use Nib's profile folder, a root-run one that refuses its sandbox, and a stub that exits 1
(`ErrBrowserCannotRender`, 422; the format reads as unavailable). `/api/status` carries `browser` from the same answer.

**Where the exec lives.** `browserrender` builds the origin and `Args`, the ONE spelling of the flags
(`TestTheBrowserIsLaunchedWithTheDoorsFlags`); it does not start a process. `pdfops/htmlimport.go` starts the browser through
`runConvert`, so it is in the converter's process group, memory watchdog and Quit registry, and sets `cmd.Env` from `toolEnv`
(which drops the proxy variables). The plan put the launch inside `browserrender`; it cannot import `pdfops`, and a second
process runner would be a second door. Declared deviation.

**Censuses.** `TestEveryListenerNibOpensIsDeclared` (root) is new: every call named `Listen*`, `Serve*` or `ListenAndServe*`
in non-test code, one planted fixture per spelling, the render origin pinned at two sites and its only caller at one.
`TestEveryProcessNibStartsIsDeclared` gains `htmlimport.go` as a converter. The census matches the name, not the receiver,
so the p2p files are many-named and pinned by file only.

**Measured (google-chrome 157, chromium 156, sandbox on, this machine).** A page prints in 0.9 s through the door. Eleven
vectors, each with a positive control that fires from `file:` without the door: meta refresh and `<iframe>`, `<object>` and
`<embed>` to a `file:` URL (the secret's words reach the PDF), and link prefetch, stylesheet, `@import`, `url()`, `<img>`,
meta refresh, SVG `<image>` and `<base>` to a loopback canary (hits). Through the door, unsanitised, the secret is absent, the
canary count is 0 and a benign sentinel is present. An anchor `ping` fired in neither run (printing follows no link), so it is
not a vector.

**Declared.** Off Linux the peer check answers Unknown and the origin serves (ADR-157's gap). Not run: Windows, macOS, a
snap-packaged browser, Firefox (no headless PDF; a Firefox-only machine gets the missing-tool sentence). The profile folder
and a confidential page can remain in the temp directory until the next start of Nib if the process is killed. Fonts and
layout are the browser's: a page is printed as the browser prints it, at its default page size.
