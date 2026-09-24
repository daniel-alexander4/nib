package uacheck

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The checker against veraPDF's OWN PDF/UA-1 corpus — `/pending 489`.
//
// Law 5's oracle test (`oracle_test.go`) runs over documents nib and LibreOffice produced, and both write
// the easy encodings: a direct string `/Lang`, a direct boolean `/Marked`. veraPDF's corpus uses every
// legal encoding, and a first run over it found 34 false `Fail` verdicts on files veraPDF passes — all of
// them readers breaking on a hex string, an indirect object, or a rule stricter than veraPDF's. This test
// is that run, standing.
//
// **What it asserts, and against what.** For every corpus file and every clause nib implements:
//   - a FALSE PASS is nib `Pass` OR `NotApplicable` where veraPDF failed the clause — both let a document be
//     called conformant (`Verdict.conformant`), so a rule that stops seeing its subject is the same defect as
//     one that passes it (`/pending 496`: this counted `Pass` only, and a walk that dropped the offending
//     element answered `NotApplicable` past it);
//   - a FALSE FAIL is nib `Fail` where veraPDF did not fail it.
// Both are errors unless the (file, clause) pair is in `corpusAllow` with its reason. `CannotCheck` is a
// declared limit and never either. A file nib cannot open is reported, not scored — nib's `open` error is
// outside every rule.
//
// **And a reach table, because `CannotCheck` is never scored.** A rule that answered `CannotCheck` on every
// file would pass both assertions above having checked nothing. `corpusReach` holds, per clause, how many
// files veraPDF evaluated it on AND nib settled it (`Pass` or `Fail`) — exact, so a reader that silently
// stops reaching is red, and a rule that newly reaches changes the number in the same edit.
//
// **Where the corpus comes from.** `$NIB_UA_CORPUS`, else `~/nib/verapdfs/PDF_UA-1` — a sparse clone of
// `github.com/veraPDF/veraPDF-corpus`. Absent corpus or absent veraPDF is a SKIP that says so, the way
// law 5's own test skips: a green run over an absent oracle verifies nothing.

// corpusAllow is every (file, clause) pair this test excuses, with why. Keyed "<dir>/<file> / <clause>".
var corpusAllow = map[string]string{}

// corpusReach is, per implemented clause, the number of corpus files on which veraPDF evaluated the clause and
// nib settled it — measured 2026-09-15 over the 297-file set. Every implemented clause has a row.
//
// `7.1 t6` joins at 294 with `/pending 548`, and no other row moved: the walk gained a third result and
// changed neither of the two the other rules read. Scored pairs 5,557 → 5,852.
//
// `7.1 t5` joins at **5** and `7.1 t7` at 294 with P03.S01, and no other row moved — though the typing walk
// changed underneath every rule (it now stops at the first standard type it reaches by mapping). t5's
// subject is an element nib cannot type as standard, and five corpus files hold one. Scored pairs are 6,442
// with both registered.
//
// The seventeen `7.2` containment rows join with P03.S02, measured over the same set; no other row moved.
// Scored pairs 6,442 → 11,457. Each clause's reach is the number of corpus files holding its subject type.
// The eight cardinality and placement rows join with P03.S03, and no row moved: pairs 11,457 → 13,817.
// P03.S04 ports veraPDF's table layout (`rules_table.go`): 7.2 t15 and t41-t43 and 7.5 t2 join at 36, and
// **7.5 t1 moves 27 → 36** — the nine files it answered `CannotCheck` on before, a grid it did not build, it
// now settles, and agrees with veraPDF on every one. Pairs 13,817 → 15,302.
// P03.S05's three heading rows join and 7.4.2 t1 stays at 134 — the plan's acceptance, measured. 7.4.4 t3's 134 is
// the same population (every file with a numbered heading); pairs 15,302 → 16,187.
// P03.S06: 7.9 t1/t2 and 7.18.4 t2 join (9, 9, 18); pairs 16,187 → 17,072. 7.1 t12 is NOT a row — veraPDF 1.30.2
// cannot fail it for an element reached through the tree (`rules_notesform.go`), so it is not registered.
// P04.S01: 7.2 t2 joins at 276 (every file veraPDF evaluates it on, outlines or none) and 7.2 t29 at 291; pairs
// 17,072 → 17,662; no other row moved.
// P04.S02: 7.2 t21, t22 and t23 join at 294 each — every file holding a structure element, since veraPDF runs one
// check per element and passes it where the key is absent; pairs 17,662 → 18,547. The same slice closed `d.text`'s
// dangling-reference hole (a reference to a free object read as an empty string that is present), which no corpus
// row moved: measured before and after, every reach figure here is unchanged.
// P04.S03: 7.2 t24 joins at 71 and 7.2 t25 at 19; pairs 18,547 → 19,137. **The two figures differ from t21-t23's
// 294 because the population is different in kind**: veraPDF runs t24 once per annotation and t25 once per form
// field, so a file with neither is not evaluated at all — where t21-t23 run once per structure element and reach
// almost every file. 71 and 19 are what "every file holding an annotation" and "every file holding a form field"
// come to on this corpus, and a figure near 294 here would mean the population had been widened by mistake.
// P04.S04: `7.1 t1` and `7.1 t2` join at 293 each — every corpus file holding a marked-content sequence — and
// `7.2 t30`, `t31` and `t32` at **295**, two ABOVE the 293 that hold a marked-content sequence: the P04 close
// moved `catalogDeclaresLang` to the front of those three, where every other language clause already asked it,
// so two files whose catalog declares a `/Lang` are now settled instead of refused. They read 293 with the
// question asked after the walk — the SAME population, because the subject is the sequence and veraPDF
// passes a check whose tag is not Span or whose key is absent. They read 259 first, which is the subset whose
// catalog `/Lang` settles the clause, and the oracle caught the difference by name on fifteen documents:
// "veraPDF passed, nib not applicable". **The same slice changed two SHIPPED rules and no corpus verdict moved**: 7.1 t3 stopped
// reading a bare `/MCID` as tagged content (it must resolve through the parent tree to an element whose `/P` chain
// reaches the root) and 7.2 t34 stopped exempting text inside an `/Artifact`. Measured before and after over the
// 297-file set: 0 false pass, 0 false fail either way, and every reach figure above unchanged — the corpus holds
// no document with a dangling MCID and none whose only unlanguaged text is an artifact's, which is why four
// measured false passes had survived it.
// P05.S01: `7.18.1 t1` and `t2` join at **71** each — the same figure as 7.2 t24 and for the same reason,
// since all three are one check per annotation and 71 is what "every corpus file holding an annotation" comes
// to on this set. Pairs 20,612 → 21,202 (run, not computed). **No other row moved, and three shipped readers were re-expressed over
// the new door in the same slice** (`checkWidgetsInFormElements`, `scanAnnotsAndFields`, `walkAppearances`), so
// the unchanged figures are the evidence that absorbing them changed no population: 7.18.4 t1 stays at 19 with
// its exemption now applied, 7.2 t24 at 71 and t25 at 19, measured before and after.
// P05.S02: `7.18.5 t1` and `t2` join at **31** (every corpus file holding a link annotation), `7.18.8 t1` at
// **1**, and **`7.18.2 t1` at ZERO**. Pairs 21,202 → 22,382.
//
// **A zero row is recorded rather than omitted, and it is the honest figure.** The corpus holds exactly one
// TrapNet document, `7.18 Annotations/7.18.2 Annotation types/7.18.2-t01-fail-a.pdf`, and it is on
// `corpusUnreadable` below — pdfcpu refuses a TrapNet annotation without its required `/F` before any rule
// runs. So this clause has NO corpus evidence at all and rests entirely on the oracle's two mutations (a
// visible TrapNet and a hidden one) plus its own fixtures. `7.18.8 t1`'s single file is the same weakness one
// step less severe. Both are named in the slice's inventory as declared gaps; a row silently left out would
// have read as coverage.
// P05.S03: `7.18.1 t3` joins at **19** — the same figure as `7.18.4 t1` and `7.2 t25`, because all three have
// the form population and 19 is what "every corpus file holding a widget or a field" comes to here — and
// `7.18.3 t1` at **295**, the highest row in this table, because its subject is a PAGE and veraPDF runs one
// check per page whether or not the page carries an annotation. Pairs 22,382 → 22,972.
// P05.S04: both media-clip clauses join at **5** — every corpus file under `7.18.6 Media`, and unlike P05.S02's
// TrapNet clause this family has real corpus evidence on BOTH halves (two `t01` files and three `t02`). Pairs
// 22,972 → 23,562, and the phase closes the checker at 80 of the 106.
var corpusReach = map[string]int{
	"5 t1": 294, "5 t2": 293, "6.2 t1": 295,
	// P06.S01. The three prefix clauses share `5 t2`'s subject gate — the identification's presence — so
	// they share its reach. `6.1 t1` reads the file's own leading bytes, which every readable file has,
	// so its reach is every file nib opens: it is the widest row in this table by construction.
	"5 t3": 293, "5 t4": 293, "5 t5": 293, "6.1 t1": 295,
	// P06.S02. `7.1 t4` is asked of every document, like `6.1 t1`. `7.11 t1`'s and `7.15 t1`'s are the
	// documents that carry a file specification or an AcroForm at all — and `7.15 t1`'s rose from 19 to
	// 25 when the rule learned to re-read the file unvalidated, because pdfcpu's validator DELETES an
	// `/AcroForm` it refuses and six corpus documents had lost theirs that way.
	"7.1 t4": 295, "7.11 t1": 16, "7.15 t1": 25,
	// P06.S03. `7.20 t1`'s subject is a form XObject and 42 corpus files hold one. **`7.16 t1` is ONE**,
	// and that one is the whole corpus evidence for the clause: the set holds exactly one encrypted
	// document, `7.16-t01-fail-a.pdf`, which veraPDF fails and nib fails. Its sibling
	// `7.16-t01-pass-a.pdf` carries NO encryption dictionary at all — measured, veraPDF reports
	// 0 passed / 0 failed on it — so no corpus document passes this clause WITH a subject, and the
	// oracle has to supply that half.
	"7.20 t1": 42, "7.16 t1": 1,
	// P06.S04. `7.7 t1` joins at 5 — the corpus files holding a Formula element. It shares `7.3 t1`'s
	// predicate through one door, and `7.3 t1`'s 18 did not move: the two clauses differ only in the
	// structure type they look for.
	"7.7 t1": 5,
	// P06.S05. `7.20 t2` joins at 42, `7.20 t1`'s figure, because its subject is the same drawn form. It
	// was 37 on the first run: five annotation files hold identical appearance streams, which pdfcpu's
	// predicate calls twins, and the rule refused all five until it learned that a twin carrying no key and
	// drawing no form cannot move the count. The corpus's own pair (`7.20-t02-fail-a`, a keyed form drawn
	// three times, and `-pass-a`) is settled on both halves.
	"7.20 t2": 42,
	// P07.S01. The CMap clauses' subject is a Type 0 font the content USES, so 7.21.3.1 t1, 7.21.3.2 t1 and 7.21.3.3
	// t1 reach every readable file that shows text (62 — most NotApplicable, the Type 0 files agreeing both ways);
	// t2 needs an EMBEDDED CMap (12) and t3 one reached through /UseCMap (2). `7.21.3.2-t01-fail-a` and `-c` are
	// refused, not scored: pdfcpu's validator drops the very Type 0 font the clause fails, so nib has nothing to read.
	// P07.S03: 255 readable files draw a simple TrueType font, and nib settles all four clauses on every one; the
	// corpus holds t2's four fail files and no fail file for t1 or t4 (their fail halves are `ttFixtures`), and t3's
	// one fail file is unreadable to pdfcpu (below). No other row moved.
	"7.21.6 t1": 255, "7.21.6 t2": 255, "7.21.6 t3": 255, "7.21.6 t4": 255,
	"7.21.3.1 t1": 62, "7.21.3.2 t1": 62, "7.21.3.3 t1": 62, "7.21.3.3 t2": 12, "7.21.3.3 t3": 2,
	"7.1 t1": 293, "7.1 t2": 293, "7.1 t3": 292, "7.1 t5": 5, "7.1 t6": 294, "7.1 t7": 294, "7.1 t8": 295, "7.1 t9": 294, "7.1 t10": 295, "7.1 t11": 295,
	"7.2 t3": 36, "7.2 t4": 36, "7.2 t5": 21, "7.2 t6": 19, "7.2 t7": 17, "7.2 t8": 36, "7.2 t9": 36,
	"7.2 t10": 36, "7.2 t17": 44, "7.2 t18": 24, "7.2 t19": 43, "7.2 t20": 44, "7.2 t26": 6, "7.2 t27": 6,
	"7.2 t36": 21, "7.2 t37": 19, "7.2 t38": 17,
	"7.2 t11": 36, "7.2 t12": 36, "7.2 t13": 36, "7.2 t14": 36, "7.2 t16": 36, "7.2 t39": 36,
	"7.2 t28": 6, "7.2 t40": 43,
	"7.2 t15": 36, "7.2 t41": 36, "7.2 t42": 36, "7.2 t43": 36, "7.5 t2": 36,
	"7.4.4 t1": 294, "7.4.4 t2": 7, "7.4.4 t3": 134,
	"7.9 t1": 9, "7.9 t2": 9, "7.18.4 t2": 18,
	"7.2 t2": 276, "7.2 t29": 291,
	"7.2 t21": 294, "7.2 t22": 294, "7.2 t23": 294,
	"7.2 t24": 71, "7.2 t25": 19,
	"7.2 t30": 295, "7.2 t31": 295, "7.2 t32": 295,
	"7.18.1 t1": 71, "7.18.1 t2": 71,
	"7.18.5 t1": 31, "7.18.5 t2": 31, "7.18.8 t1": 1, "7.18.2 t1": 0,
	"7.18.1 t3": 19, "7.18.3 t1": 295,
	"7.18.6.2 t1": 5, "7.18.6.2 t2": 5,
	"7.2 t33": 293, "7.2 t34": 295, "7.3 t1": 18, "7.4.2 t1": 134, "7.5 t1": 36,
	"7.10 t1": 6, "7.10 t2": 6, "7.18.4 t1": 19,
	"7.21.4.1 t1": 287, "7.21.4.2 t2": 43, "7.21.7 t1": 282, "7.21.7 t2": 282,
}

// corpusUnreadable is every corpus file nib's reader cannot open, with why. Reported, not scored. Both
// are pdfcpu's validator refusing the file before any rule runs, which is outside every clause nib checks.
var corpusUnreadable = map[string]string{
	"7.18 Annotations/7.18.2 Annotation types/7.18.2-t01-fail-a.pdf": "pdfcpu: a TrapNet annotation without its required /F",
	"7.21 Fonts/7.21.6 Character encodings/7.21.6-t03-fail-a.pdf":    "pdfcpu: an /Encoding named Custom, which it refuses",
}

func corpusDir() string {
	if d := os.Getenv("NIB_UA_CORPUS"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "nib", "verapdfs", "PDF_UA-1")
}

func TestTheCheckerAgreesWithVeraPDFsOwnCorpus(t *testing.T) {
	dir := corpusDir()
	var files []string
	if dir != "" {
		filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.HasSuffix(strings.ToLower(p), ".pdf") {
				files = append(files, p)
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Skip("SKIP (not a pass): veraPDF's PDF/UA-1 corpus is absent, so /pending 489's standing run is " +
			"UNCHECKED. Set NIB_UA_CORPUS, or sparse-clone veraPDF-corpus's PDF_UA-1 to ~/nib/verapdfs.")
	}
	vp := veraPDFPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the corpus run is UNCHECKED. Set NIB_VERAPDF, put " +
			"verapdf on PATH, or install to ~/verapdf.")
	}
	sort.Strings(files)
	// A floor on the population: a corpus that shrank to a handful would pass every assertion below.
	if len(files) < 250 {
		t.Fatalf("the corpus holds %d PDF(s) — too few to be veraPDF's PDF/UA-1 set (297 when this was written)", len(files))
	}

	// veraPDF names each job by base name; the corpus reuses names across directories, so run it over
	// uniquely-named links in a temp dir.
	tmp := t.TempDir()
	links := make([]string, len(files))
	names := make([]string, len(files))
	for i, f := range files {
		rel, _ := filepath.Rel(dir, f)
		names[i] = filepath.ToSlash(rel)
		links[i] = filepath.Join(tmp, strings.NewReplacer("/", "__", " ", "_").Replace(names[i]))
		if err := os.Symlink(f, links[i]); err != nil {
			t.Fatal(err)
		}
	}
	out, _ := exec.Command(vp, append([]string{"--flavour", "ua1", "--passed"}, links...)...).Output()
	var rep veraReport
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the veraPDF report did not parse: %v\n%.500s", err, out)
	}
	vera := veraStates(rep, links)

	implemented := map[string]bool{}
	for _, c := range Clauses() {
		implemented[c] = true
	}
	var falsePass, falseFail []string
	reach := map[string]int{}
	scored, unreadable := 0, 0
	seenUnreadable := map[string]bool{}
	for i, f := range files {
		pdf, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if vera[i] == nil {
			t.Errorf("%s: veraPDF returned no job for it — a lost file reads exactly like a clean one", names[i])
			continue
		}
		nr, err := Check(pdf)
		if err != nil {
			unreadable++
			seenUnreadable[names[i]] = true
			if _, known := corpusUnreadable[names[i]]; !known {
				t.Errorf("%s: nib cannot open a corpus file and it is not recorded in corpusUnreadable: %v", names[i], err)
			}
			continue
		}
		for _, r := range nr.Results {
			if !implemented[r.Clause] {
				continue
			}
			v, listed := vera[i][r.Clause]
			if !listed {
				continue
			}
			if r.Verdict == Pass || r.Verdict == Fail {
				reach[r.Clause]++
			}
			if r.Verdict == CannotCheck {
				continue
			}
			scored++
			key := names[i] + " / " + r.Clause
			if _, allowed := corpusAllow[key]; allowed {
				continue
			}
			switch {
			case r.Verdict.conformant() && v == veraFailed:
				falsePass = append(falsePass, key+" — veraPDF failed it; nib: "+r.Verdict.String()+" "+r.Why)
			case r.Verdict == Fail && v != veraFailed:
				falseFail = append(falseFail, key+" — veraPDF "+string(v)+"; nib: "+r.Why+" ("+r.Where+")")
			}
		}
	}
	for k := range corpusUnreadable {
		if !seenUnreadable[k] {
			t.Errorf("corpusUnreadable has %q, which nib now opens — remove the row", k)
		}
	}
	t.Logf("veraPDF corpus: %d file(s), %d (file, clause) pair(s) scored, %d unreadable, %d false pass, %d false fail",
		len(files), scored, unreadable, len(falsePass), len(falseFail))
	for _, e := range falsePass {
		t.Errorf("FALSE PASS: %s", e)
	}
	for _, e := range falseFail {
		t.Errorf("FALSE FAIL: %s", e)
	}
	for c := range implemented {
		want, has := corpusReach[c]
		switch {
		case !has:
			t.Errorf("REACH: %s has no row in corpusReach — nib settles it on %d corpus file(s); record that number", c, reach[c])
		case reach[c] != want:
			t.Errorf("REACH: nib settles %s on %d corpus file(s), corpusReach says %d — a drop is a reader that stopped "+
				"reaching; a rise is a rule that newly reaches and changes the row in the same edit", c, reach[c], want)
		}
	}
	for c := range corpusReach {
		if !implemented[c] {
			t.Errorf("REACH: corpusReach has %q, which nib does not implement — remove the row", c)
		}
	}
}
