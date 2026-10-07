// The accuracy harness's `scan` column: the scoring, with no browser in it (test/accuracy/accuracy.mjs drives the
// searches; test/jsdom/placematches.test.mjs holds these rules to their words).
//
// A scan that has been OCR'd has a hidden text layer over a picture. The page map knows where the LAYER's words are
// and nothing about the ink under them, so the print scoring has no truth for one. The OCR engine has it: the word
// boxes the layer was stamped from. A scan is scored when the corpus keeps those beside it —
//
//     <name>.pdf          the scan, already OCR'd (in the corpus list like any document)
//     <name>.words.json   the `words` of the /api/ocr request that made its layer:
//                         [{page, text, rect: [x0, y0, x1, y1]}, …], points from the page's bottom-left
//
// — and a document with no such file, or a page with no hidden text, gets no `scan` entry at all. Nothing runs OCR
// (minutes a page in a browser): the layer is read as it is.
//
// For words the engine saw once on the page, the search is run as a user runs it and each box is compared with the
// engine's box for that word: is any of the scanned word's ink OUTSIDE the box (must be none — that ink stays
// readable after the redaction), and does the box reach the ink of a word on ANOTHER line.
//
// "Another line" is asked both ways — neither word's middle is within the other's height — and the other word must
// be at least SPECK across and high. The probe this column replaces asked one way only, and counted three words
// whose "other line" was a speck of OCR noise inside their own box ('.' 1.0 × 0.3pt, '/' 1.1 × 0.3pt, 'za'
// 2.5 × 10pt). A speck on another line that a box reaches is still counted, apart (`specks`).
export const EDGE = 0.3;  // points: less than this is the same edge
export const SPECK = 1.5; // points: an engine "word" smaller than this either way is noise, not a word

const quant = (v, p) => { if (!v.length) return null; const s = [...v].sort((a, b) => a - b); return s[Math.floor(p * (s.length - 1))]; };
const r2 = (v) => (v == null ? null : Math.round(v * 100) / 100);

// scanWords are the engine's words on page n, as boxes in points from the page's TOP-left — the map's own direction.
export function scanWords(truth, n, map) {
  return truth.filter((w) => w.page === n && Array.isArray(w.rect) && w.rect[2] > w.rect[0] && w.rect[3] > w.rect[1])
    .map((w) => ({ text: w.text, box: [w.rect[0], map.height - w.rect[3], w.rect[2], map.height - w.rect[1]] }));
}

// scanLayer sets the page's hidden runs against the engine's boxes: how many words have a run that begins where the
// engine saw the word and reads the same, and how wide that run is beside the ink (0.72 at the median for a stamp
// from before ADR-092, 1.00 for a fitted one). `ok` is false when fewer than half line up: a turned page, a page box
// that does not start at the origin, or another document's words — and such a page is not scored.
export function scanLayer(words, map) {
  const hidden = map.text.filter((t) => t.hidden), widths = [];
  for (const w of words) {
    const cy = (w.box[1] + w.box[3]) / 2 / map.height;
    const t = hidden.find((r) => Math.abs(r.rect[0] * map.width - w.box[0]) < 1.2 && cy >= r.rect[1] - 2 / map.height && cy <= r.rect[3] + 2 / map.height && r.text.trim() === w.text);
    if (t) widths.push((t.rect[2] - t.rect[0]) * map.width / (w.box[2] - w.box[0]));
  }
  return { engineWords: words.length, lineUp: widths.length, widthOverInk: [quant(widths, 0.1), quant(widths, 0.5), quant(widths, 0.9)].map(r2),
    ok: words.length > 0 && widths.length >= 0.5 * words.length };
}

// scanPicks are the words to search for: whole words of letters and digits that are on the page ONCE — as the engine
// read it and as the layer has it, folding case and counting a word inside a longer one, as the search does — spread
// down the page, `limit` of them.
export function scanPicks(words, map, limit) {
  const lc = words.map((w) => w.text.toLowerCase()), layerText = map.text.filter((t) => t.hidden).map((t) => t.text).join(' ').toLowerCase();
  const picks = words.filter((w) => /^[A-Za-z0-9]{4,}$/.test(w.text) && lc.filter((t) => t.includes(w.text.toLowerCase())).length === 1
    && layerText.split(w.text.toLowerCase()).length === 2);
  const step = Math.max(1, Math.floor(picks.length / limit));
  return picks.filter((_, i) => i % step === 0).slice(0, limit);
}

// scoreScanWord compares the boxes one search drew (fractions of the page) with the engine's box for the word.
export function scoreScanWord(w, boxes, map, words) {
  const e = w.box, mid = (e[1] + e[3]) / 2;
  const pts = boxes.map((b) => [b[0] * map.width, b[1] * map.height, b[2] * map.width, b[3] * map.height]);
  const on = pts.filter((b) => Math.min(b[2], e[2]) > Math.max(b[0], e[0]) && Math.min(b[3], e[3]) > Math.max(b[1], e[1]));
  if (!on.length) return { word: w.text, missed: true, boxes: boxes.length };
  const u = [Math.min(...on.map((b) => b[0])), Math.min(...on.map((b) => b[1])), Math.max(...on.map((b) => b[2])), Math.max(...on.map((b) => b[3]))];
  // Ink the box does not cover, by side: positive is ink showing.
  const out = { left: u[0] - e[0], right: e[2] - u[2], above: u[1] - e[1], below: e[3] - u[3] };
  let reaches = 0, specks = 0;
  for (const o of words) {
    if (o === w) continue;
    const ob = o.box, omid = (ob[1] + ob[3]) / 2;
    if ((mid >= ob[1] && mid <= ob[3]) || (omid >= e[1] && omid <= e[3])) continue; // the word's own line
    if (!on.some((b) => Math.min(b[2], ob[2]) - Math.max(b[0], ob[0]) > EDGE && Math.min(b[3], ob[3]) - Math.max(b[1], ob[1]) > EDGE)) continue;
    if (ob[2] - ob[0] < SPECK || ob[3] - ob[1] < SPECK) specks++; else reaches++;
  }
  return {
    word: w.text, boxes: boxes.length, inkOutside: r2(Math.max(0, out.left, out.right, out.above, out.below)),
    left: r2(out.left), right: r2(out.right), above: r2(out.above), below: r2(out.below),
    pastRight: r2(u[2] - e[2]), tall: r2((u[3] - u[1]) / (e[3] - e[1])), reaches, specks,
  };
}

// scanSummary is the column's share of the summary: null when the corpus has no scan to score.
export function scanSummary(rows, seconds) {
  const pages = rows.filter((r) => r.scan);
  if (!pages.length) return null;
  const all = pages.flatMap((r) => r.scan.words || []), hit = all.filter((w) => !w.missed);
  return {
    pages: pages.filter((r) => r.scan.words).length,
    words: all.length,
    missed: all.length - hit.length,
    notOneBox: hit.filter((w) => w.boxes !== 1).length,
    inkOutside: hit.filter((w) => w.inkOutside > EDGE).length, // must be 0: ink of the word left readable
    maxInkOutsidePt: hit.length ? Math.max(...hit.map((w) => w.inkOutside)) : null,
    reachesAnotherLine: hit.filter((w) => w.reaches > 0).length,
    reachesOnlySpecks: hit.filter((w) => !w.reaches && w.specks > 0).length,
    boxPastInkRightMedPt: quant(hit.map((w) => w.pastRight), 0.5),
    boxOverInkHeightMed: quant(hit.map((w) => w.tall), 0.5),
    layerWidthOverInkMed: quant(pages.map((r) => r.scan.layer.widthOverInk[1]).filter((v) => v != null), 0.5),
    notScored: pages.filter((r) => r.scan.error).map((r) => `${r.doc} p${r.page}: ${r.scan.error}`),
    seconds: Math.round(seconds),
  };
}

// scanReport is the column's lines of the per-page report.
export function scanReport(rows) {
  const pages = rows.filter((r) => r.scan);
  if (!pages.length) return [];
  const lines = ['', 'scan redaction (doc · page | words / missed / not one box / ink outside / reaches another line / only specks | box past ink right, median pt | layer width ÷ ink, median)'];
  for (const r of pages) {
    const head = `  ${r.doc.slice(0, 38).padEnd(38)} p${String(r.page).padEnd(3)} | `;
    if (r.scan.error) { lines.push(head + `not scored: ${r.scan.error}`); continue; }
    const ws = r.scan.words, hit = ws.filter((w) => !w.missed);
    lines.push(head + `${ws.length}/${ws.length - hit.length}/${hit.filter((w) => w.boxes !== 1).length}/${hit.filter((w) => w.inkOutside > EDGE).length}`
      + `/${hit.filter((w) => w.reaches > 0).length}/${hit.filter((w) => !w.reaches && w.specks > 0).length} | ${quant(hit.map((w) => w.pastRight), 0.5) ?? '—'} | ${r.scan.layer.widthOverInk[1] ?? '—'}`);
  }
  return lines;
}
