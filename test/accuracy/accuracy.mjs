// The accuracy harness's second half (ADR-088, build/accuracy.sh): the real app, a real browser, real documents.
//
// For every prepared document it opens the copy with the form fields removed, and on each mapped page:
//
//   DETECTION — presses Detect fields and reads back where every proposed field was put. Scored against the
//   document's own fields where it had them (the answer key), and against the page map everywhere: does a proposed
//   field sit on printed text, does it run through a ruled line.
//
//   REDACTION — searches for words whose exact extent the map knows (its glyph cuts come from the font's own
//   widths) and reads back the boxes. Scored on how much of the word a box covers and how far past it the box runs.
//
// Everything is in fractions of the displayed page, which is what both the map and the overlays use.
import fs from 'node:fs';
import path from 'node:path';
import { launch, shutdown } from '../ui/harness.mjs';
import { matchesInMap } from '../../web/detect.js';
import { scanWords, scanLayer, scanPicks, scoreScanWord, scanSummary, scanReport } from './scanscore.mjs';
import * as reach from './reach.mjs';

const [manifestPath, resultsPath] = process.argv.slice(2);
const docs = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));

// ── geometry ────────────────────────────────────────────────────────────────
const area = (r) => Math.max(0, r[2] - r[0]) * Math.max(0, r[3] - r[1]);
const inter = (a, b) => [Math.max(a[0], b[0]), Math.max(a[1], b[1]), Math.min(a[2], b[2]), Math.min(a[3], b[3])];
const iou = (a, b) => { const i = area(inter(a, b)); return i ? i / (area(a) + area(b) - i) : 0; };
const centre = (r) => [(r[0] + r[2]) / 2, (r[1] + r[3]) / 2];
const holds = (r, p) => p[0] >= r[0] && p[0] <= r[2] && p[1] >= r[1] && p[1] <= r[3];
const pct = (n, d) => (d ? Math.round(1000 * n / d) / 10 : null);
const mean = (xs) => (xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : null);

// A proposed field and a real one are the same KIND of thing when both take text or both are ticked.
const ticked = (kind) => kind === 'check' || kind === 'radio';

// labelsOf is the page's print that a field must not sit on: each run's letters and digits. The "-" between the parts
// of a phone number and the underscores of a blank drawn with the keyboard are print too, and a field belongs exactly
// there — the real fields of the corpus's own forms cover them — so they are not labels. Where a run's glyph cuts are
// known, its label is the stretch from its first letter to its last.
function labelsOf(map) {
  const out = [];
  for (const t of map.text) {
    if (t.hidden || !/[\p{L}\p{N}]/u.test(t.text)) continue;
    if (!t.cuts || !t.chars || t.cuts.length !== t.chars.length + 1) { out.push(t); continue; }
    // Split at a run of four or more underscores: "Name ______ Date ______" is two labels and two blanks.
    let from = -1, to = -1, blank = 0;
    const flush = () => { if (from >= 0) out.push({ ...t, rect: [t.cuts[from], t.rect[1], t.cuts[to + 1], t.rect[3]] }); from = -1; };
    t.chars.forEach((c, i) => {
      if (c === '_') { if (++blank === 4) flush(); return; }
      blank = 0;
      if (/[\p{L}\p{N}]/u.test(c)) { if (from < 0) from = i; to = i; }
    });
    flush();
  }
  return out;
}

// scoreDetection compares what Detect proposed on one page with the page's map.
function scoreDetection(found, map) {
  const truth = map.widgets.filter((w) => w.kind !== 'button' && w.kind !== 'signature');
  const text = labelsOf(map);
  const out = { proposed: found.length, truth: truth.length };

  // Against the answer key: a real field is FOUND when a proposed field of its kind holds its centre or it holds
  // the proposed one's; a proposed field is RIGHT when it found one.
  const used = new Set();
  let hit = 0, kindWrong = 0; const ious = [], missed = [];
  for (const w of truth) {
    let best = -1, bestIou = 0;
    found.forEach((f, i) => {
      if (used.has(i)) return;
      if (!holds(f.rect, centre(w.rect)) && !holds(w.rect, centre(f.rect))) return;
      const v = iou(f.rect, w.rect);
      if (v >= bestIou) { best = i; bestIou = v; }
    });
    if (best < 0) { if (w.name) missed.push(w.name); continue; }
    used.add(best);
    if (ticked(found[best].kind) !== ticked(w.kind)) { kindWrong++; continue; }
    hit++; ious.push(bestIou);
  }
  out.found = hit; out.kindWrong = kindWrong; out.right = used.size - kindWrong;
  out.meanIoU = mean(ious);
  // FOUND is generous: a 15pt sliver whose centre falls inside a 200pt field finds it. foundWell asks that the two
  // mostly coincide — the count a person would call "the field is where it belongs". (Measured: on two grid forms
  // the generous count fell 23 -> 18 and 24 -> 16 across a change that lost no field with half its area in common.)
  out.foundWell = ious.filter((v) => v >= 0.5).length;
  out.missedNames = missed.slice(0, 12); // which real fields nothing was proposed for, by the form's own names
  // Shaded boxes are where a form prints a heading or a "for office use" panel: context for a page that scores badly.
  out.shadedBoxes = map.shapes.filter((s) => s.kind === 'box' && s.filled).length;

  // Against the map: a field that takes text must not sit on printed text, and must not run through a rule.
  const pt = 1 / map.height; // one point, as a fraction of the page's height
  const textFields = found.filter((f) => !ticked(f.kind));
  out.textFields = textFields.length;
  const on = (f, t) => area(inter(f.rect, t.rect)) > 0.25 * area(t.rect) || holds(f.rect, centre(t.rect));
  // Faint print is a hint, not a label (ADR-099): "MM" in grey where a month goes, which the form's own field
  // covers. A field on one is counted beside the labels and not among them.
  out.onLabel = textFields.filter((f) => text.some((t) => !t.faint && on(f, t))).length;
  out.onHint = textFields.filter((f) => text.some((t) => t.faint && on(f, t))).length;
  out.throughRule = textFields.filter((f) => map.shapes.some((s) => {
    if (s.kind !== 'v') return false;
    const x = (s.rect[0] + s.rect[2]) / 2;
    // a vertical rule well inside the field, crossing most of its height
    return x > f.rect[0] + 3 * pt * (map.height / map.width) && x < f.rect[2] - 3 * pt * (map.height / map.width)
      && Math.min(f.rect[3], s.rect[3]) - Math.max(f.rect[1], s.rect[1]) > 0.6 * (f.rect[3] - f.rect[1]);
  })).length;
  return out;
}

// wordsToFind picks words whose extent the map knows exactly: inside one run, with glyph cuts, and appearing once
// on the page — so whatever box the search draws for it is a box for THIS occurrence.
function wordsToFind(map, limit) {
  const pageText = map.text.map((t) => t.text).join(' ');
  const picks = [];
  for (const t of map.text) {
    if (t.hidden || !t.cuts || !t.chars || t.cuts.length !== t.chars.length + 1) continue;
    // Each char entry may be more than one character (a ligature) or none; index characters back to glyphs.
    let s = ''; const at = [];
    t.chars.forEach((c, i) => { for (const ch of c) { s += ch; at.push(i); } });
    for (const m of s.matchAll(/[A-Za-z]{6,}/g)) {
      const w = m[0];
      // Once on the page — as the SEARCH counts, which folds case and matches inside longer words: "Speech" is not
      // alone on a page that also prints "ashlandspeech". (It was counted case-sensitively, and a second hit on the
      // same line was then scored as one box 62 glyph-widths too wide.)
      if (pageText.toLowerCase().split(w.toLowerCase()).length !== 2) continue;
      const before = s[m.index - 1], after = s[m.index + w.length];
      if ((before && /[A-Za-z0-9]/.test(before)) || (after && /[A-Za-z0-9]/.test(after))) continue;
      const g0 = at[m.index], g1 = at[m.index + w.length - 1];
      picks.push({ word: w, rect: [t.cuts[g0], t.rect[1], t.cuts[g1 + 1], t.rect[3]], size: t.size, glyph: (t.cuts[g1 + 1] - t.cuts[g0]) / w.length });
    }
  }
  // Spread across the page rather than the first few lines.
  const step = Math.max(1, Math.floor(picks.length / limit));
  return picks.filter((_, i) => i % step === 0).slice(0, limit);
}

// sourceOf says where a box the app drew came from: 'exact' when it is one of the boxes the page map gives for this
// word (the app runs the same matchesInMap over the same map), 'estimate' when it is not — a box pdf.js's text layer
// placed and placeMatches kept. Read off the page rather than reported by the app, so the product carries nothing for
// the instrument. The tolerance is the overlay's own rounding: its edges are whole CSS pixels of a page some 800 wide,
// and an estimate differs from its exact box by most of a line height each side.
const SAME = 0.004;
const sourceOf = (box, exact) => (exact.some((e) => e.every((v, i) => Math.abs(v - box[i]) <= SAME)) ? 'exact' : 'estimate');
const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const r5 = (r) => r.map((v) => Math.round(v * 1e5) / 1e5);

// scoreRedaction compares the boxes a search drew for one word with the word's true extent.
function scoreRedaction(pick, boxes, map) {
  const exact = matchesInMap(map, [new RegExp(escapeRe(pick.word), 'gi')]);
  // A box read off the page has whole-pixel edges — up to a pixel, some 0.8pt, wider than the mark the app holds and
  // will apply. Where a drawn box IS one of the map's, it is scored as the map has it. Of the three words that "took
  // one neighbouring glyph" at v1.189.0, two were a glyph whose centre lay between the mark's edge and the pixel's;
  // the third is real (a 1.39pt-wide glyph in 5pt type, its centre inside the mark's 0.75pt pad). ADR-095.
  const seen = boxes; // as drawn, for extraSeen below
  boxes = boxes.map((b) => exact.find((e) => e.every((v, i) => Math.abs(v - b[i]) <= SAME)) || b);
  const tall = pick.rect[3] - pick.rect[1], midY = (pick.rect[1] + pick.rect[3]) / 2;
  const row = boxes.filter((b) => Math.min(b[3], pick.rect[3]) - Math.max(b[1], pick.rect[1]) > 0.3 * tall);
  // Every box the search drew anywhere on the page, by source: a box off the word's row is a second place the word
  // was "found" — the estimate of a match the map boxed elsewhere, or a match the harness did not expect.
  const drawn = boxes.map((b) => ({ src: sourceOf(b, exact), onRow: row.includes(b), rect: r5(b) }));
  if (!row.length) return { word: pick.word, missed: true, mapPlaced: exact.length, drawn };
  const x0 = Math.min(...row.map((b) => b[0])), x1 = Math.max(...row.map((b) => b[2]));
  const covered = Math.max(0, Math.min(x1, pick.rect[2]) - Math.max(x0, pick.rect[0])) / (pick.rect[2] - pick.rect[0]);
  const box = [x0, Math.min(...row.map((b) => b[1])), x1, Math.max(...row.map((b) => b[3]))];
  // Other glyphs whose centre the box blacks out: what the redaction takes that nobody asked it to. Counted against
  // the UNION of the row's boxes, as before, and each one also laid to the box that holds it: which kind of box took
  // it, on the word's own line or another, and to which side.
  // extraSeen is the same count against the boxes as the page DREW them (the figure before the boxes were scored as
  // the app holds them): the difference between the two is what the pixel grid adds, and no redaction takes it.
  const seenRow = seen.filter((_, i) => row.includes(boxes[i]));
  const seenBox = [Math.min(...seenRow.map((b) => b[0])), Math.min(...seenRow.map((b) => b[1])), Math.max(...seenRow.map((b) => b[2])), Math.max(...seenRow.map((b) => b[3]))];
  let extra = 0, extraSeen = 0; const taken = [];
  for (const t of map.text) {
    if (t.hidden || !t.cuts) continue;
    for (let i = 0; i + 1 < t.cuts.length; i++) {
      if (!(t.chars[i] || '').trim()) continue;
      const c = [(t.cuts[i] + t.cuts[i + 1]) / 2, (t.rect[1] + t.rect[3]) / 2];
      if (holds(seenBox, c) && !holds(pick.rect, c)) extraSeen++;
      if (!holds(box, c) || holds(pick.rect, c)) continue;
      extra++;
      const by = row.filter((b) => holds(b, c)).map((b) => sourceOf(b, exact));
      taken.push({
        by: by.length ? [...new Set(by)].sort().join('+') : 'union-only', // inside the union, in no one box
        lines: Math.round(10 * (c[1] - midY) / tall) / 10, // 0: the word's own line
        side: c[0] < pick.rect[0] ? 'left' : c[0] > pick.rect[2] ? 'right' : 'over',
        glyphsOff: Math.round(10 * (c[0] < pick.rect[0] ? pick.rect[0] - c[0] : c[0] - pick.rect[2]) / pick.glyph) / 10,
        widthPt: Math.round(100 * (t.cuts[i + 1] - t.cuts[i]) * map.width) / 100, sizePt: Math.round(10 * t.size * map.height) / 10,
      });
    }
  }
  return {
    word: pick.word, covered,
    // how far the box runs past the word on each side, in the word's own average glyph widths
    overLeft: (pick.rect[0] - x0) / pick.glyph, overRight: (x1 - pick.rect[2]) / pick.glyph, extra, extraSeen,
    mapPlaced: exact.length, sources: [...new Set(row.map((b) => sourceOf(b, exact)))].sort().join('+'),
    drawn, taken, pick: r5(pick.rect), exact: exact.map(r5),
  };
}

// ── the browser ─────────────────────────────────────────────────────────────
const h = await launch();
const page = h.page;

const pageSel = (n) => `.viewerContainer:not([hidden]) .page[data-page-number="${n}"]`;

async function goto(n) {
  await page.fill('#sbPages .pageNum', String(n)).catch(async () => { await h.panel('thumbs'); await page.fill('#sbPages .pageNum', String(n)); });
  await page.press('#sbPages .pageNum', 'Enter');
  await page.waitForSelector(`${pageSel(n)} canvas`, { timeout: 30000 });
  await page.waitForTimeout(300);
}

// overlaysOn reads the boxes of the elements matching `sel` on page n, as fractions of the page.
const overlaysOn = (n, sel) => page.evaluate(([ps, s]) => {
  const div = document.querySelector(ps);
  if (!div) return [];
  const W = div.clientWidth, H = div.clientHeight;
  return [...div.querySelectorAll(s)].map((el) => ({
    kind: el.classList.contains('ovl-check') ? 'check' : el.classList.contains('ovl-circleone') ? 'choice' : 'text',
    rect: [el.offsetLeft / W, el.offsetTop / H, (el.offsetLeft + el.offsetWidth) / W, (el.offsetTop + el.offsetHeight) / H],
  }));
}, [pageSel(n), sel]);

async function detect(n) {
  // Detect says when it is done — "Added N fillable field(s)…" or "Nothing detected…" — and that is waited for first.
  // The count settling was the only wait, and a count that has not started moving is settled at 0: on a loaded
  // machine a dense page (the 1040, an outlines-only form) read 0 proposals, three times in one day, and read its
  // real figure when run alone. The old toast is cleared so the last press's sentence cannot answer for this one.
  await page.evaluate(() => { const t = document.getElementById('toast'); if (t) t.textContent = ''; });
  await page.$eval('#detectBtn', (b) => b.click());
  await page.waitForFunction(() => /^(Added \d+ fillable|Nothing detected)/.test((document.getElementById('toast') || {}).textContent || ''), null, { timeout: 180000 });
  let last = -1, still = 0;
  for (let i = 0; i < 80 && still < 3; i++) {
    await page.waitForTimeout(250);
    const c = (await overlaysOn(n, '.ovl-text, .ovl-check, .ovl-circleone')).length;
    still = c === last ? still + 1 : 0; last = c;
  }
  return overlaysOn(n, '.ovl-text, .ovl-check, .ovl-circleone');
}

async function search(n, word) {
  // Marks stay on the page from one search to the next, so the new ones are the difference — counted, not compared:
  // a word that is in the same place on two pages draws the same box twice once boxes are exact, and a box equal to
  // an earlier one is still a new box. (Comparing by value scored every such word "not marked".)
  const key = (m) => m.rect.map((v) => v.toFixed(4)).join();
  const before = new Map();
  for (const m of await overlaysOn(n, '.redactmark')) before.set(key(m), (before.get(key(m)) || 0) + 1);
  await page.$eval('#redactTextBtn', (b) => b.click());
  await page.waitForSelector('#redactTextModal:not([hidden])');
  await page.fill('#rtTerm', word);
  await page.click('#rtFind');
  // "Searching…" is a status too: wait for the dialog to close (marks were made) or to say there were none.
  await page.waitForFunction(() => document.getElementById('redactTextModal').hidden || /^No matches/.test(document.getElementById('rtStatus').textContent), null, { timeout: 120000 });
  if (!(await page.$eval('#redactTextModal', (m) => m.hidden))) { await page.click('#rtCancel'); return []; }
  await page.waitForTimeout(150);
  return (await overlaysOn(n, '.redactmark')).filter((m) => {
    const left = before.get(key(m)) || 0;
    if (left) before.set(key(m), left - 1);
    return !left;
  }).map((m) => m.rect);
}

// ── scan redaction: the `scan` column (scanscore.mjs) ──────────────────────
// Bounded, because every search maps every page of the document (a 14-page scan: some seconds a search):
// SCAN_PAGES pages a scan, SCAN_WORDS words a page.
const SCAN_PAGES = 2, SCAN_WORDS = Number(process.env.NIB_ACCURACY_SCAN_WORDS || 10);
const scanTruth = new Map(); // a words file -> its words
let scanSeconds = 0;

// scanColumn scores page n of an OCR'd scan — the page is on screen — or returns nothing for a page that is not one:
// a document with no saved word boxes, a page with no hidden text, a scan that has had its SCAN_PAGES.
async function scanColumn(doc, n, map, done) {
  if (!doc.words || done >= SCAN_PAGES || !map.text.some((t) => t.hidden)) return undefined;
  const started = Date.now();
  if (!scanTruth.has(doc.words)) scanTruth.set(doc.words, JSON.parse(fs.readFileSync(doc.words, 'utf8')));
  const words = scanWords(scanTruth.get(doc.words), n, map), layer = scanLayer(words, map);
  if (!layer.ok) return { layer, error: 'the saved word boxes do not line up with this page\'s text layer' };
  const rows = [];
  for (const w of scanPicks(words, map, SCAN_WORDS)) {
    // Scored as the app holds each box, not as the pixel grid drew it — as the print column does.
    const exact = matchesInMap(map, [new RegExp(escapeRe(w.text), 'gi')]);
    const boxes = (await search(n, w.text)).map((b) => exact.find((x) => x.every((v, i) => Math.abs(v - b[i]) <= SAME)) || b);
    rows.push(scoreScanWord(w, boxes, map, words));
  }
  scanSeconds += (Date.now() - started) / 1000;
  return { layer, words: rows };
}

// open opens a document to work on — or says why it is not one. A document that carries a sign-here request opens
// LOCKED (app.js: `signLocked = docHadFlags`): editing is off, so Detect and the search cannot be driven. The prepared
// copy goes without the request (`TestAccuracyPrep`), so none should — and one that does is named, not left to fail
// at the first disabled button. (The helper could not open such a document at all until it asked whether the
// viewer's class HAS `has-doc`; that was this harness's thirty-second "openDocument: Timeout" on one corpus file.)
async function open(doc) {
  await h.openDocument(doc.stripped, doc.pages);
  await page.waitForTimeout(300); // the lock is applied after the first page is up
  if (await page.evaluate(() => document.getElementById('viewerWrap').classList.contains('signing-locked'))) {
    throw new Error('not scored: it opens in signing mode (it carries a sign-here request, and Nib locks editing on such a document)');
  }
}

const results = [];
for (const doc of docs) {
  const name = path.basename(doc.source);
  if (doc.error || !doc.maps || !doc.maps.length) { results.push({ doc: name, error: doc.error || 'no page could be mapped' }); continue; }
  try {
    await h.closeAll();
    await page.reload();
    await page.waitForSelector('#empty');
    await open(doc);
    for (const [i, prepared] of doc.maps.entries()) {
      const n = prepared.page;
      await goto(n);
      // The map the APP serves for the document it has open — the copy without fields — through the route the
      // product will use. The answer key is the one thing that copy cannot have: the original's own fields.
      const live = await page.evaluate((pn) => window.nibFetch('/api/pagemap?page=' + pn).then((r) => (r.ok ? r.json() : null)), n);
      if (!live) throw new Error(`the app could not map page ${n}`);
      const map = { ...live, widgets: prepared.widgets };
      const row = { doc: name, page: n, kind: map.noText ? (map.shapes.length ? 'outlines' : 'scan') : 'text', size: `${Math.round(map.width)}x${Math.round(map.height)}` };
      const found = await detect(n);
      row.detection = scoreDetection(found, map);
      // The same page, zoomed: what a field IS must not depend on how large the page is drawn.
      if (i === 0) {
        for (let z = 0; z < 4; z++) await page.click('#zoomInBtn');
        await page.waitForSelector(`${pageSel(n)} canvas`); await page.waitForTimeout(500);
        const zoomed = await detect(n);
        const count = (fs_, k) => fs_.filter((f) => f.kind === k).length;
        row.zoom = { text: [count(found, 'text'), count(zoomed, 'text')], check: [count(found, 'check'), count(zoomed, 'check')] };
        await page.click('#fitBtn'); await page.waitForTimeout(400);
      }
      row.redaction = [];
      for (const pick of wordsToFind(map, 6)) row.redaction.push(scoreRedaction(pick, await search(n, pick.word), map));
      row.scan = await scanColumn(doc, n, map, results.filter((r) => r.doc === name && r.scan && r.scan.words).length);
      row.reach = reach.scorePage(doc, n, map, row.redaction); // up and down the page, against its pixels (reach.mjs)
      results.push(row);
      process.stdout.write('.');
    }
  } catch (e) {
    results.push({ doc: name, error: String(e.message || e).split('\n')[0] });
    if (process.env.NIB_ACCURACY_DEBUG) {
      console.error(e.stack);
      await page.screenshot({ path: process.env.NIB_ACCURACY_DEBUG }).catch(() => {});
    }
    process.stdout.write('x');
  }
}
await shutdown(h);
process.stdout.write('\n');

// ── the report ──────────────────────────────────────────────────────────────
const rows = results.filter((r) => r.detection);
const sum = (f) => rows.reduce((a, r) => a + (f(r) || 0), 0);
const keyed = rows.filter((r) => r.detection.truth > 0);
const red = rows.flatMap((r) => r.redaction);
const hits = red.filter((r) => !r.missed);
const summary = {
  pages: rows.length,
  detection: {
    answerKeyPages: keyed.length,
    realFields: keyed.reduce((a, r) => a + r.detection.truth, 0),
    foundPct: pct(keyed.reduce((a, r) => a + r.detection.found, 0), keyed.reduce((a, r) => a + r.detection.truth, 0)),
    foundWellPct: pct(keyed.reduce((a, r) => a + r.detection.foundWell, 0), keyed.reduce((a, r) => a + r.detection.truth, 0)),
    proposedOnKeyedPages: keyed.reduce((a, r) => a + r.detection.proposed, 0),
    rightPct: pct(keyed.reduce((a, r) => a + r.detection.right, 0), keyed.reduce((a, r) => a + r.detection.proposed, 0)),
    wrongKind: keyed.reduce((a, r) => a + r.detection.kindWrong, 0),
    meanIoU: mean(keyed.filter((r) => r.detection.meanIoU != null).map((r) => r.detection.meanIoU)),
    textFields: sum((r) => r.detection.textFields),
    onLabelPct: pct(sum((r) => r.detection.onLabel), sum((r) => r.detection.textFields)),
    onHint: sum((r) => r.detection.onHint),
    throughRulePct: pct(sum((r) => r.detection.throughRule), sum((r) => r.detection.textFields)),
    zoomDisagrees: rows.filter((r) => r.zoom && (r.zoom.text[0] !== r.zoom.text[1] || r.zoom.check[0] !== r.zoom.check[1])).length,
    zoomTried: rows.filter((r) => r.zoom).length,
  },
  redaction: {
    words: red.length,
    missedPct: pct(red.length - hits.length, red.length),
    fullyCoveredPct: pct(hits.filter((r) => r.covered >= 0.98).length, hits.length),
    meanCovered: mean(hits.map((r) => r.covered)),
    meanOverreachGlyphs: mean(hits.map((r) => Math.max(0, r.overLeft) + Math.max(0, r.overRight))),
    takesOtherGlyphsPct: pct(hits.filter((r) => r.extra > 0).length, hits.length),
    meanOtherGlyphs: mean(hits.map((r) => r.extra)),
    takesOtherGlyphs: hits.filter((r) => r.extra > 0).length,
    takesOtherGlyphsAsDrawn: hits.filter((r) => r.extraSeen > 0).length, // against whole-pixel boxes: the figure up to v1.189.0
    // Where each word's boxes came from, and — for the words that take a neighbour — the same split.
    bySource: Object.fromEntries(['exact', 'estimate', 'estimate+exact'].map((k) => [k, hits.filter((r) => r.sources === k).length])),
    takesOtherBySource: Object.fromEntries(['exact', 'estimate', 'estimate+exact'].map((k) => [k, hits.filter((r) => r.extra > 0 && r.sources === k).length])),
    mapPlacedNone: red.filter((r) => r.mapPlaced === 0).length,
    boxesOffTheRow: red.reduce((a, r) => a + r.drawn.filter((b) => !b.onRow).length, 0),
  },
  scan: scanSummary(rows, scanSeconds),
  errors: results.filter((r) => r.error).map((r) => `${r.doc}: ${r.error}`),
};
fs.writeFileSync(resultsPath, JSON.stringify({ at: new Date().toISOString(), summary, results }, null, 1));

const f1 = (v) => (v == null ? '—' : (Math.round(v * 100) / 100).toString());
console.log(`\nper page (doc · page · kind | detection: proposed / real / found / found-well / right / on-label / through-rule | redaction: words / missed / mean cover / mean overreach)`);
for (const r of rows) {
  const d = r.detection, rr = r.redaction.filter((x) => !x.missed);
  console.log(`  ${r.doc.slice(0, 38).padEnd(38)} p${String(r.page).padEnd(3)} ${r.kind.padEnd(8)} | ${d.proposed}/${d.truth}/${d.found}/${d.foundWell}/${d.right}/${d.onLabel}/${d.throughRule}`
    + ` | ${r.redaction.length}/${r.redaction.length - rr.length}/${f1(mean(rr.map((x) => x.covered)))}/${f1(mean(rr.map((x) => Math.max(0, x.overLeft) + Math.max(0, x.overRight))))}`
    + (r.zoom ? ` | zoom text ${r.zoom.text.join('→')} check ${r.zoom.check.join('→')}` : ''));
}
for (const line of scanReport(rows)) console.log(line);
const reached = reach.report(rows); // the vertical-reach column: its own summary, written and printed
summary.reach = reached.summary;
fs.writeFileSync(resultsPath, JSON.stringify({ at: new Date().toISOString(), summary, results }, null, 1));
console.log(reached.lines.join('\n'));
console.log('\nsummary', JSON.stringify(summary, null, 1));
console.log(`\nresults written to ${resultsPath}`);
