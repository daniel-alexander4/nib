// The accuracy harness's VERTICAL-REACH column (ADR-098, /pending 851 parts 1 and 2): up and down the page, does a
// search-redaction's box cover the word's own ink, and does it reach the ink of the line above or below?
//
// The other redaction columns score a box ACROSS the page against the map's glyph cuts, which come from the font's
// own widths. Up and down the map has no such fact — a run's box is a full size above its baseline and a quarter
// below, whatever the letters are — so scoring a box's height against the map would be scoring the constant against
// itself. The truth here is the page's PIXELS, drawn by a renderer that is neither Nib nor the browser Nib runs in
// (poppler's `pdftoppm`, the crop box at 300 dpi, grey). Without it the column says so and scores nothing.
//
// What is read off the pixels, per word, within the word's own columns (its glyph cuts):
//
//   its OWN ink — the rows of ink that touch the stretch from its baseline to half a size above it (every letter and
//   digit has ink there), carried up and down across gaps of at most GAP of a size: the dot of an i, an accent, the
//   dots under an Arabic letter. Never further than OWN_UP above the baseline or OWN_DOWN below it, and never into
//   another line: where the map has a run on another baseline over the same columns, the word's ink stops where that
//   line's letters are likely to begin (LINE_UP of its size above its baseline, LINE_DOWN below). Closely leaded
//   type leaves less white between two lines than between an i and its dot (measured: 11pt Georgia on 12pt), so
//   the gap alone cannot tell them apart. This is the one place the column guesses, and it guesses about the OTHER
//   line: a word under a closely set line has its ink read at most that far up.
//
//   the nearest OTHER ink above and below — the first row of ink past that. A ruled line the map knows is not ink.
//
// Two populations, scored the same way: the words the harness SEARCHED in the app (the boxes the app drew), and
// every word of the page the map can box (`matchesInMap` over the run — the function the app runs, and the redaction
// column shows the app drawing exactly its boxes). The second is the large one, and it is the one a change to the
// box is judged on: `inkOut` must be 0 before and after, and no word's may grow.
//
// One number is taken from the map: where the baseline is. A run's rect is `runBox`'s — 1.0 of the size above the
// baseline, 0.25 below — and this reads the baseline back out of it. If those two constants change, so must BOX_UP
// and BOX_DOWN, and `TestTheReachColumnReadsTheBaselineRunBoxWrote` (internal/pdfops) fails until they do.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { matchesInMap } from '../../web/detect.js';

export const BOX_UP = 1.0, BOX_DOWN = 0.25; // runBox's ascentEm and descentEm
const DPI = 300;
const DARK = 110;          // a grey below this is ink: black type, and not the grey watermark some returns set behind it
const GAP = 0.17;          // of the size: the widest gap inside one word's ink
const OWN_UP = 1.25, OWN_DOWN = 0.5; // of the size: the furthest a word's own ink is followed
const LINE_UP = 0.8, LINE_DOWN = 0.3; // of ITS size: where another line's letters are likely to be
const SAME_LINE = 0.5;     // of the size: a run whose baseline is nearer than this is on the word's own line
const LOOK_UP = 1.8, LOOK_DOWN = 1.2; // of the size: how far other ink is looked for
const TOUCH = 0.25;        // points: less than this is the renderer's antialiasing, about a pixel
const PER_PAGE = 400;      // words of the page scored, spread evenly

let scratch = null, poppler = null;

// renderPage draws page n of the PDF at `file` and returns {w, h, px} — one grey byte a pixel — or null when there
// is no renderer or the drawing is not the page the map describes (a crop box or a turn the two read differently).
export function renderPage(file, n, map) {
  if (poppler === null) {
    try { execFileSync('pdftoppm', ['-v'], { stdio: 'ignore' }); poppler = true; } catch { poppler = false; }
  }
  if (!poppler) return null;
  if (!scratch) scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'nib-reach-'));
  const out = path.join(scratch, 'page');
  try {
    execFileSync('pdftoppm', ['-gray', '-r', String(DPI), '-cropbox', '-f', String(n), '-l', String(n), '-singlefile', file, out], { stdio: 'ignore' });
    const b = fs.readFileSync(out + '.pgm');
    // "P5\n<w> <h>\n<max>\n" and then the bytes.
    const head = /^P5\s+(\d+)\s+(\d+)\s+(\d+)\s/.exec(b.subarray(0, 40).toString('latin1'));
    if (!head) return null;
    const w = +head[1], h = +head[2];
    if (Math.abs(w / h - map.width / map.height) > 0.005 * (map.width / map.height)) return null;
    return { w, h, px: b.subarray(head[0].length) };
  } catch { return null; }
}

// wordsOf is every word of the page the map can box: letters and digits inside one visible run with glyph cuts.
function wordsOf(map) {
  const out = [];
  for (const t of map.text) {
    if (t.hidden || !t.cuts || !t.chars || t.cuts.length !== t.chars.length + 1) continue;
    let s = ''; const at = [];
    t.chars.forEach((c, i) => { for (const ch of c) { s += ch; at.push(i); } });
    const one = { width: map.width, height: map.height, text: [t] };
    for (const m of s.matchAll(/[\p{L}\p{N}]{3,}/gu)) {
      const g0 = at[m.index], g1 = at[m.index + m[0].length - 1];
      const rect = [t.cuts[g0], t.rect[1], t.cuts[g1 + 1], t.rect[3]];
      // The box the search draws for THIS occurrence: the run alone, searched for the word, the match that starts here.
      const boxes = matchesInMap(one, [new RegExp(m[0].replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gu')]);
      const box = boxes.find((b) => Math.abs((b[0] + b[2]) / 2 - (rect[0] + rect[2]) / 2) < 0.5 / map.width);
      if (box) out.push({ word: m[0], rect, box: [...box] });
    }
  }
  const step = Math.max(1, Math.ceil(out.length / PER_PAGE));
  return out.filter((_, i) => i % step === 0);
}

// reachOf reads one word's ink off the page and lays its box against it. Everything it returns is in points or in
// sizes ("em") of the word's own type, measured from its baseline: up is positive for `own.top` and `other.above`,
// down is positive for `own.bottom` and `other.below`.
export function reachOf(img, map, word) {
  const { w: W, h: H, px } = img;
  const tall = word.rect[3] - word.rect[1];
  const size = tall / (BOX_UP + BOX_DOWN);
  const em = size * H;                                // pixels in one size
  const base = (word.rect[3] - BOX_DOWN * size) * H;  // the baseline's row
  const c0 = Math.max(0, Math.floor(word.rect[0] * W)), c1 = Math.min(W, Math.ceil(word.rect[2] * W));
  if (!(em >= 8) || c1 - c0 < 3) return { word: word.word, unscored: 'too small to read' };
  const pt = H / map.height;                          // pixels in a point
  // Ruled lines and box edges the map knows, as pixel boxes grown a little: they are not a line of text.
  const rules = [];
  for (const s of map.shapes || []) {
    const r = [s.rect[0] * W, s.rect[1] * H, s.rect[2] * W, s.rect[3] * H];
    const grow = 0.6 * pt;
    const thin = Math.min(r[2] - r[0], r[3] - r[1]) <= 6 * pt; // a bar: a rule too thick for the map to call one
    if (s.kind === 'h' || s.kind === 'v' || (s.kind === 'box' && s.filled && thin)) rules.push([r[0] - grow, r[1] - grow, r[2] + grow, r[3] + grow]);
    else if (s.kind === 'box' && !s.filled) {
      rules.push([r[0] - grow, r[1] - grow, r[2] + grow, r[1] + grow], [r[0] - grow, r[3] - grow, r[2] + grow, r[3] + grow],
        [r[0] - grow, r[1] - grow, r[0] + grow, r[3] + grow], [r[2] - grow, r[1] - grow, r[2] + grow, r[3] + grow]);
    }
  }
  const y0 = Math.max(0, Math.floor(base - LOOK_UP * em)), y1 = Math.min(H - 1, Math.ceil(base + LOOK_DOWN * em));
  const near = rules.filter((r) => r[2] > c0 && r[0] < c1 && r[3] > y0 && r[1] < y1);
  const inked = (y) => {
    if (y < 0 || y >= H) return false;
    let n = 0, all = 0;
    for (let x = c0; x < c1; x++) {
      if (px[y * W + x] >= DARK) continue;
      all++;
      if (!near.some((r) => x >= r[0] && x <= r[2] && y >= r[1] && y <= r[3])) n++;
    }
    // Ink across the whole of a word of three letters or more is no letter: a bar the map does not carry (a thick
    // stroked line is neither its rule nor its box).
    return n >= 2 && all < 0.98 * (c1 - c0);
  };
  // Type set over other type — a watermark across a page of print — has no ink of its own to read.
  const over = map.text.some((t) => {
    if (t.hidden || t.rect[2] <= word.rect[0] || t.rect[0] >= word.rect[2]) return false;
    const tb = t.rect[3] - BOX_DOWN * (t.rect[3] - t.rect[1]) / (BOX_UP + BOX_DOWN);
    return Math.abs(tb * H - base) >= SAME_LINE * em && tb * H < base + 0.2 * em && tb * H > base - 0.9 * em;
  });
  if (over) return { word: word.word, unscored: 'set over another line' };
  // Where another line's letters begin, above and below: the furthest the word's own ink is followed.
  let upTo = base - OWN_UP * em, downTo = base + OWN_DOWN * em;
  for (const t of map.text) {
    if (t.hidden || t.rect[2] * W <= c0 || t.rect[0] * W >= c1) continue;
    const ts = (t.rect[3] - t.rect[1]) / (BOX_UP + BOX_DOWN), tb = (t.rect[3] - BOX_DOWN * ts) * H;
    if (Math.abs(tb - base) < SAME_LINE * em) continue;
    if (tb < base) upTo = Math.max(upTo, tb + LINE_DOWN * ts * H); else downTo = Math.min(downTo, tb - LINE_UP * ts * H);
  }
  const lineUp = upTo > base - OWN_UP * em, lineDown = downTo < base + OWN_DOWN * em;
  // The word's own ink: seeded where every letter has some, then carried across small gaps.
  let top = -1, bottom = -1;
  for (let y = Math.floor(base - 0.5 * em); y <= Math.ceil(base); y++) if (inked(y)) { if (top < 0) top = y; bottom = y; }
  if (top < 0) return { word: word.word, unscored: 'no ink where the word is' };
  const gap = GAP * em;
  let hitUp = false, hitDown = false;
  for (let y = top - 1, miss = 0; miss <= gap; y--) {
    if (y < base - OWN_UP * em) { hitUp = miss === 0; break; }
    if (y < upTo && (miss > 0 || !inked(y))) break; // past where the next line begins: only ink that has not stopped
    if (inked(y)) { top = y; miss = 0; } else miss++;
  }
  for (let y = bottom + 1, miss = 0; miss <= gap; y++) {
    if (y > base + OWN_DOWN * em) { hitDown = miss === 0; break; }
    if (y > downTo && (miss > 0 || !inked(y))) break;
    if (inked(y)) { bottom = y; miss = 0; } else miss++;
  }
  // Ink that does not stop is not a word alone on white: a dark ground, a picture behind it, the side of a box or a
  // rule the map does not carry running through it. Where the word ends cannot be read, so it is not scored.
  if (hitUp || hitDown) return { word: word.word, unscored: 'its ink does not stop (a ground, a picture or a line through it)' };
  let above = null, below = null; // the nearest other ink: its lowest row above, its highest row below
  for (let y = lineUp ? Math.min(top - 1, Math.floor(upTo)) : top - Math.floor(gap) - 2; y >= y0; y--) if (inked(y)) { above = y; break; }
  for (let y = lineDown ? Math.max(bottom + 1, Math.ceil(downTo)) : bottom + Math.floor(gap) + 2; y <= y1; y++) if (inked(y)) { below = y; break; }
  const r2 = (v) => Math.round(v * 100) / 100;
  const bTop = word.box[1] * H, bBot = word.box[3] * H;
  const out = {
    word: word.word, at: word.rect.map((v) => Math.round(v * 1e5) / 1e5), sizePt: r2(size * map.height),
    own: { top: r2((base - top) / em), bottom: r2((bottom + 1 - base) / em) },
    other: { above: above == null ? null : r2((base - (above + 1)) / em), below: below == null ? null : r2((below - base) / em) },
    box: { top: r2((base - bTop) / em), bottom: r2((bBot - base) / em) },
    // The word's own ink left OUTSIDE the box: the unsafe direction.
    inkOutAbove: r2(Math.max(0, bTop - top) / pt), inkOutBelow: r2(Math.max(0, bottom + 1 - bBot) / pt),
    // How far the box runs into the other line's ink.
    reachAbove: above == null ? 0 : r2(Math.max(0, above + 1 - bTop) / pt), reachBelow: below == null ? 0 : r2(Math.max(0, bBot - below) / pt),
  };
  return out;
}

// scorePage is the column for one page: `searched` are the redaction column's rows for it (the boxes the app drew).
export function scorePage(doc, n, map, searched) {
  const img = renderPage(doc.stripped, n, map);
  if (!img) return { unscored: poppler ? 'the page could not be drawn as the map has it' : 'pdftoppm (poppler) is not installed' };
  const drawn = [];
  for (const r of searched || []) {
    const row = (r.drawn || []).filter((b) => b.onRow).map((b) => b.rect);
    if (r.missed || !row.length || !r.pick) continue;
    const box = [Math.min(...row.map((b) => b[0])), Math.min(...row.map((b) => b[1])), Math.max(...row.map((b) => b[2])), Math.max(...row.map((b) => b[3]))];
    drawn.push(reachOf(img, map, { word: r.word, rect: r.pick, box }));
  }
  return { searched: drawn, page: wordsOf(map).map((w) => reachOf(img, map, w)) };
}

// summarise adds a population up. A box "reaches" or "leaves ink out" past TOUCH, which is a pixel of antialiasing.
export function summarise(words) {
  const ok = words.filter((w) => !w.unscored);
  const med = (xs) => { const s = xs.slice().sort((a, b) => a - b); return s.length ? s[Math.floor(s.length / 2)] : null; };
  const q = (xs, f) => { const s = xs.slice().sort((a, b) => a - b); return s.length ? s[Math.min(s.length - 1, Math.floor(f * s.length))] : null; };
  return {
    words: ok.length, unscored: words.length - ok.length,
    reachesAbove: ok.filter((w) => w.reachAbove > TOUCH).length, reachesBelow: ok.filter((w) => w.reachBelow > TOUCH).length,
    inkOutAbove: ok.filter((w) => w.inkOutAbove > TOUCH).length, inkOutBelow: ok.filter((w) => w.inkOutBelow > TOUCH).length,
    maxInkOutPt: ok.reduce((a, w) => Math.max(a, w.inkOutAbove, w.inkOutBelow), 0),
    // The ink itself, in sizes from the baseline: what a tighter box would have to hold.
    inkTopEm: { median: med(ok.map((w) => w.own.top)), p99: q(ok.map((w) => w.own.top), 0.99), max: ok.reduce((a, w) => Math.max(a, w.own.top), 0) },
    inkBottomEm: { median: med(ok.map((w) => w.own.bottom)), p99: q(ok.map((w) => w.own.bottom), 0.99), max: ok.reduce((a, w) => Math.max(a, w.own.bottom), 0) },
    boxTallOverInk: med(ok.map((w) => (w.box.top + w.box.bottom) / Math.max(0.01, w.own.top + w.own.bottom))),
  };
}

// report is the column's summary over every page, and its lines for the console.
export function report(rows) {
  const scored = rows.filter((r) => r.reach && !r.reach.unscored);
  const all = (k) => scored.flatMap((r) => r.reach[k]);
  const out = { pages: scored.length, unscoredPages: rows.length - scored.length, searched: summarise(all('searched')), page: summarise(all('page')) };
  const why = rows.find((r) => r.reach && r.reach.unscored);
  if (why) out.unscoredWhy = why.reach.unscored;
  const lines = ['\nvertical reach, per page (doc · page | every word: scored / reaches above / reaches below / ink out above / ink out below / unscored | searched: scored / above / below / ink out)'];
  for (const r of scored) {
    const p = summarise(r.reach.page), s = summarise(r.reach.searched);
    lines.push(`  ${r.doc.slice(0, 38).padEnd(38)} p${String(r.page).padEnd(3)} | ${p.words}/${p.reachesAbove}/${p.reachesBelow}/${p.inkOutAbove}/${p.inkOutBelow}/${p.unscored}`
      + ` | ${s.words}/${s.reachesAbove}/${s.reachesBelow}/${s.inkOutAbove + s.inkOutBelow}`);
  }
  return { summary: out, lines };
}
