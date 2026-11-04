# ADR-198 — PostScript is run only by a Ghostscript that has been asked to keep it in, and the answer is not its version number

Status: accepted, 2026-11-03 (PLAN-import-export P04.S02a, v2.40.0). Extends ADR-185, ADR-197; X11(b), X11(d).

**The slice was cut once.** P04.S02 was PostScript/EPS and HEIC/HEIF. This machine has Ghostscript and no HEIF
decoder (`heif-dec`, `heif-convert`, `magick` and `convert` are all absent; `ffmpeg` is present and is not in the plan's
tool order), so a HEIC row built here would be a row whose present path no one ran. S02a is PostScript and EPS, wired
end to end; S02b is HEIC, with the tool order and the capability probe of D2.

**The door.** A table row per extension (`ps`, `eps`; Tool Ghostscript; `FamilyPostScript`, sniffed from `%!` or the
DOS-EPS header `C5 D0 D3 C6`). `classify` sends PostScript under a `.txt`, `.md` or `.csv` name to the text row and never
runs it, PostScript under any other name or none to `ErrExtensionDisagrees`, and a `.ps` that is not PostScript likewise.
`convertImport` calls `ConvertPostScriptToPDF`, which resolves `gs`, asks `confineProbe`, writes the input alone into
`<job>/in/in.ps` (`in.eps`), and runs `psArgs` with the output in `<job>/out/`, stdin the null device, `toolEnv`, the
gate's two minutes and the watchdog. The output is sized by `stat` before it is read and its page count is held to
`importPages` (a program is stopped at one page past it by `-dLastPage`). A document with no page is
`ErrPostScriptUnreadable`.

**One spelling of the flags.** `psArgs` builds the arguments for the conversion and for the probe, so the probe cannot
test flags the conversion does not use. `-dSAFER`, `-sDEVICE=pdfwrite`, `-dBATCH -dNOPAUSE -dQUIET`, `-dLastPage`, the
output, `-dEPSCrop` for EPS, and `--` before the input. Never `-dNOSAFER`, `-dDELAYSAFER` or `--permit-file-*`
(`TestThePostScriptArgumentsAllowNothing`).

**The floor is a number and the confinement is a measurement.** X11(b) asks for a named version floor "recorded from
NVD". A version cannot carry that: Ubuntu 24.04 reports 10.02.1 with its security fixes backported, and an upstream
number does not show a bypass in a device the program can select. So the floor (`gsFloor`, 9.50, where SAFER became the
default and `--permit-file-*` took its present meaning) refuses only what predates the sandbox, and `probeGhostscript`
runs a program that tries `(<file>) (r) file` and `(%pipe%touch <file>) (r) file`, each inside `stopped`, and requires
`HELDHELD` on standard output and the marker file absent. The answer, a refusal included, is cached per binary (path,
size, modification time) so an upgrade is asked again. **Measured on this machine (gs 10.02.1):** with `-dSAFER` both are
refused and no command runs; with `-dNOSAFER` the same program prints `LEAKLEAK` and the command runs
(`TestTheLeakFixtureLeaksWithoutTheConfinement` is that control and fails when the fixture stops leaking). A probe cost is
one Ghostscript start (~0.17 s), once per binary. A wrapper that rewrites `-dSAFER` to `-dNOSAFER` fails the probe
(`...RefusesAnUnconfinedGhostscript`).

**Refusals, each named.** `ErrGhostscriptTooOld` and `ErrGhostscriptNotConfined` (422, counter cause `tool-unsafe`),
`ErrPostScriptUnreadable` (422), `ErrTooManyPages` (413). A missing Ghostscript is the table's missing-tool refusal and the
window's `#psMissing` notice (static link, hidden while `gsAvailable`) with its own *Check again*.

**Declared:** the probe tests the two confinements the plan names, not every Ghostscript bypass (CVE-2024-29510 is a
`uniprint` device bypass fixed upstream in 10.03.1; a program that selects a device is not in the probe, and `pdfwrite`
is the only device the arguments name but a program can `setdevice`). A distribution build that is vulnerable and reports
a number at or above 9.50 passes the version check and is caught only if it fails the probe. The EPS preview is ignored
(the PostScript is what runs). Windows and macOS candidate lists for `gs` already exist (`ghostscriptCandidates`); the
probe is run on neither. Non-Linux has no memory watchdog (Y5, declared there).
