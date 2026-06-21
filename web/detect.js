// Nib field-detection toolkit. Pure functions over a rendered page canvas and
// pdf.js text items (in canvas device pixels): they propose fillable fields —
// rules, boxes, checkboxes, table cells, and circle-the-answer choice groups.
// No app state, no pdf.js import; only the DOM canvas API (measureText / pixels).
// Extracted from app.js so the detection logic can be exercised in isolation.

// buildTextRows groups text items into rows; per row it gives a reconstructed
// string `s` with per-char x-centre `cx` and height `ch`, plus a `words` list
// with pixel x-spans. Shared by the circle-a-choice detectors below.
export function buildTextRows(items) {
  const rows = [];
  for (const it of items.slice().sort((a, b) => a.y - b.y)) {
    const row = rows.find((r) => Math.abs(r.y - it.y) <= Math.max(6, it.h * 0.6));
    if (row) row.items.push(it); else rows.push({ y: it.y, h: it.h, items: [it] });
  }
  const mctx = (buildTextRows._ctx ||= document.createElement('canvas').getContext('2d'));
  mctx.font = '16px sans-serif';
  for (const row of rows) {
    row.items.sort((a, b) => a.x - b.x);
    let s = '';
    const cx = [], ch = [], words = [];
    for (let k = 0; k < row.items.length; k++) {
      const it = row.items[k];
      if (k > 0) { s += ' '; cx.push(NaN); ch.push(it.h); }
      // Per-char left edge by proportional glyph advance (measureText), scaled
      // to the item's true width — uniform width mis-places text after a long
      // underscore run or proportional letters.
      const adv = [];
      let sum = 0;
      for (const cc of it.str) { const a = mctx.measureText(cc).width || 1; adv.push(a); sum += a; }
      const sc = it.w / (sum || 1);
      const edge = [it.x]; // edge[c] = x at start of char c; edge[len] = right edge
      for (let c = 0; c < it.str.length; c++) edge.push(edge[c] + adv[c] * sc);
      let i = 0;
      while (i < it.str.length) { // words within the item, with pixel spans
        if (/\s/.test(it.str[i])) { i++; continue; }
        let j = i; while (j < it.str.length && !/\s/.test(it.str[j])) j++;
        words.push({ text: it.str.slice(i, j), x0: edge[i], x1: edge[j], h: it.h });
        i = j;
      }
      for (let c = 0; c < it.str.length; c++) { s += it.str[c]; cx.push((edge[c] + edge[c + 1]) / 2); ch.push(it.h); }
    }
    words.sort((a, b) => a.x0 - b.x0);
    row.s = s; row.cx = cx; row.ch = ch; row.words = words;
  }
  return rows;
}

// snapChoices refines choice boxes (canvas px) to the actual rendered glyphs.
// Sub-item text position is estimated from font metrics and drifts after a long
// underscore run, so when the real ink in the choice band resolves into exactly
// one word-cluster per choice, snap each box's x-extent to its cluster. If the
// count doesn't match (multi-word choices, touching glyphs), keep the estimate.
export function snapChoices(canvas, choices, marker) {
  if (choices.length < 2) return choices;
  const ctx = canvas.getContext('2d'), W = canvas.width, H = canvas.height;
  const data = ctx.getImageData(0, 0, W, H).data;
  const dark = (x, y) => { if (x < 0 || y < 0 || x >= W || y >= H) return false; const i = (y * W + x) * 4; return data[i + 3] >= 40 && Math.max(data[i], data[i + 1], data[i + 2]) < 160; };
  const h = choices[0].y1 - choices[0].y0;
  const baseY = Math.round(Math.max(...choices.map((c) => c.y1)) - h * 0.28);
  const yb0 = Math.round(Math.min(...choices.map((c) => c.y0))), yb1 = baseY - 1; // glyph band, above the baseline (skip any underline)
  const lo = Math.min(...choices.map((c) => c.x0)), hi = Math.max(...choices.map((c) => c.x1));
  // The estimate can drift by a word-width, so search wide; but clamp at the
  // marker so "(circle one)" isn't mistaken for a choice.
  let xa = Math.round(lo - 3 * h), xb = Math.round(hi + 3 * h);
  if (marker) {
    if (marker[0] >= hi - h) xb = Math.min(xb, Math.round(marker[0] - 2));      // marker to the right
    else if (marker[1] <= lo + h) xa = Math.max(xa, Math.round(marker[1] + 2)); // marker to the left
  }
  const inked = (x) => { for (let y = yb0; y <= yb1; y++) if (dark(x, y)) return true; return false; };
  const gT = Math.max(2, Math.round(h * 0.12)); // gaps within a word are smaller than the spaces around delimiters
  const clusters = [];
  let s = -1, gap = 0;
  for (let x = xa; x <= xb; x++) {
    if (inked(x)) { if (s < 0) s = x; gap = 0; }
    else if (s >= 0 && ++gap > gT) { clusters.push([s, x - gap]); s = -1; }
  }
  if (s >= 0) clusters.push([s, xb - gap]);
  const wide = clusters.filter((c) => c[1] - c[0] >= Math.max(4, h * 0.28)); // drop thin "|" marks and specks
  if (wide.length !== choices.length) return choices;
  return choices.map((c, i) => { const pad = h * (c.word ? 0.3 : 0.6); return { ...c, x0: wide[i][0] - pad, x1: wide[i][1] + pad }; });
}

// glyphCircleBox builds a circle/pill box (canvas px) around a choice spanning
// [x0,x1] on a baseline. `word` (a multi-char choice) gets a wider, pill box.
export function choiceBox(x0, x1, baseY, hh) {
  const word = (x1 - x0) > hh * 1.3;
  const pad = hh * (word ? 0.35 : 0.7);
  return { x0: x0 - pad, y0: baseY - hh * 1.1, x1: x1 + pad, y1: baseY + hh * 0.28, word };
}

// findYesNo locates "Y/N" choices, robust to case, spacing, and the glyphs being
// split across text-layer fragments. Returns circle-one groups (Y and N as two
// single-letter choices).
export function findYesNo(items) {
  const out = [];
  for (const row of buildTextRows(items)) {
    const re = /\bY\s*\/\s*N\b/gi;
    let m;
    while ((m = re.exec(row.s))) {
      const yi = m.index, ni = m.index + m[0].length - 1;
      const yx = row.cx[yi], nx = row.cx[ni];
      if (isNaN(yx) || isNaN(nx)) continue;
      const hh = row.ch[yi] || row.ch[ni] || 14;
      out.push({ choices: [choiceBox(yx, yx, row.y, hh), choiceBox(nx, nx, row.y, hh)] });
    }
  }
  return out;
}

// findCircleOne locates a "(circle one)" instruction and the choices it governs
// — a delimiter-separated list (| or /) on the same row, or, if the marker
// stands alone, on the adjacent row (e.g. "Male/Female" below "(circle one)").
// Each choice becomes a circleable box (pill around a word, circle around one
// letter). Returns the same group shape as findYesNo.
export function findCircleOne(items) {
  const rows = buildTextRows(items);
  const out = [];
  for (let ri = 0; ri < rows.length; ri++) {
    const row = rows[ri];
    if (!/circle\s+one/i.test(row.s)) continue;
    const mm = /\(?\s*circle\s+one\s*\)?/i.exec(row.s);
    let mx0 = Infinity, mx1 = -Infinity;
    if (mm) for (let c = mm.index; c < mm.index + mm[0].length; c++) { if (!isNaN(row.cx[c])) { mx0 = Math.min(mx0, row.cx[c]); mx1 = Math.max(mx1, row.cx[c]); } }
    let choices = extractChoices(row, mx0, mx1);
    let marker = [mx0, mx1]; // same row → exclude marker when snapping to ink
    if (choices.length < 2) { // marker alone — look at the adjacent rows
      for (const dr of [rows[ri + 1], rows[ri - 1]]) {
        if (!dr || Math.abs(dr.y - row.y) > row.h * 2.2) continue;
        const c2 = extractChoices(dr, mx0, mx1);
        if (c2.length >= 2) { choices = c2; marker = null; break; } // marker is on another row
      }
    }
    if (choices.length >= 2) out.push({ choices, marker });
  }
  return out;
}

// extractChoices pulls the delimiter-separated option list nearest the marker
// x-range from a row. Choices are bounded by "junk" (underscores, colons,
// parentheses — labels and fill-blanks) and by large horizontal gaps, so
// "Bank Name: ___ Checking | Savings | Visa | MC" yields the four options only.
// `markerless` trims carrier prose off the first option ("Increase my monthly
// dues by $5 | …" → "$5"): with no marker or punctuation to bound it, the lead-in
// would otherwise ride into choice one (see trimLeadIn).
export function extractChoices(row, mx0, mx1, markerless) {
  const words = [];
  for (const w of row.words) {
    if (/^[A-Za-z]{2,}(\/[A-Za-z]{2,})+$/.test(w.text)) { // "Male/Female" → split on /
      const parts = w.text.split('/'), cw = (w.x1 - w.x0) / w.text.length;
      let off = 0;
      for (let pi = 0; pi < parts.length; pi++) {
        words.push({ text: parts[pi], x0: w.x0 + cw * off, x1: w.x0 + cw * (off + parts[pi].length), h: w.h });
        off += parts[pi].length;
        if (pi < parts.length - 1) { words.push({ text: '/', x0: w.x0 + cw * off, x1: w.x0 + cw * (off + 1), h: w.h, delim: true }); off++; }
      }
    } else words.push({ ...w, delim: w.text === '|' || w.text === '/' });
  }
  const isJunk = (w) => /[_:()]/.test(w.text);
  const lists = [];
  let segs = [[]], hadDelim = false, prevX1 = null, prevDelim = false;
  const flush = () => { if (hadDelim) { const cs = segs.filter((seg) => seg.length); if (cs.length >= 2) lists.push(cs); } segs = [[]]; hadDelim = false; };
  for (const w of words) {
    if (isJunk(w)) { flush(); prevX1 = w.x1; prevDelim = false; continue; }
    // A big horizontal gap ends a list — but the wide spacing around a "|"
    // delimiter is expected, so don't break a list across a delimiter.
    const bigGap = prevX1 != null && (w.x0 - prevX1) > w.h * 1.6;
    if (bigGap && !w.delim && !prevDelim) flush();
    if (w.delim) { hadDelim = true; segs.push([]); }
    else segs[segs.length - 1].push(w);
    prevX1 = w.x1; prevDelim = !!w.delim;
  }
  flush();
  if (!lists.length) return [];
  const span = (cs) => { const xs = cs.flat(); return { x0: Math.min(...xs.map((w) => w.x0)), x1: Math.max(...xs.map((w) => w.x1)) }; };
  let best = lists[0], bestD = Infinity;
  for (const cs of lists) {
    const lb = span(cs);
    const d = mx0 === Infinity ? 0 : Math.max(0, lb.x0 - mx1, mx0 - lb.x1);
    if (d < bestD) { bestD = d; best = cs; }
  }
  if (markerless && best.length) best[0] = trimLeadIn(best[0]);
  return best.map((seg) => choiceBox(Math.min(...seg.map((w) => w.x0)), Math.max(...seg.map((w) => w.x1)), row.y, seg[0].h));
}

// trimLeadIn drops carrier words preceding the first option in a marker-free
// list. Keep the trailing run of the first segment whose tokens share the class
// (number/currency vs word) of the token touching the first delimiter: in
// "Increase my monthly dues by $5" the "$5" is number-class and "by" is word-
// class, so the run stops at "$5"; a multi-word option like "New York" (both
// word-class) is kept whole.
export function trimLeadIn(seg) {
  if (seg.length < 2) return seg;
  const cls = (t) => /^[$\d]/.test(t.text) ? 'n' : /^[A-Za-z]/.test(t.text) ? 'w' : 'o';
  const c = cls(seg[seg.length - 1]);
  let i = seg.length - 1;
  while (i > 0 && cls(seg[i - 1]) === c) i--;
  return seg.slice(i);
}

// findPipeChoices locates a pipe-separated list that carries its own delimiter,
// so no "(circle one)" cue is needed ("$5 | $10 | $25"). "|" is unambiguous — it
// only ever separates options — so unlike a slash it is trusted on its own. Rows
// a marker already governs are left to findCircleOne; trimLeadIn strips carrier
// prose off the first option. Returns the same group shape as findYesNo.
export function findPipeChoices(items) {
  const out = [];
  for (const row of buildTextRows(items)) {
    if (/circle\s+one/i.test(row.s) || !row.words.some((w) => w.text === '|')) continue;
    const choices = extractChoices(row, Infinity, -Infinity, true);
    if (choices.length >= 2) out.push({ choices, marker: null });
  }
  return out;
}

// splitSlash turns an "X/Y" token into its choice boxes (one per part).
export function splitSlash(w, baseY) {
  const parts = w.text.split('/'), cw = (w.x1 - w.x0) / w.text.length;
  const out = []; let off = 0;
  for (const p of parts) { out.push(choiceBox(w.x0 + cw * off, w.x0 + cw * (off + p.length), baseY, w.h)); off += p.length + 1; }
  return out;
}

// findSlashTemplates propagates a "(circle one)" decision to identical options
// elsewhere on the page. A form writes "(circle one)" once over the first
// "Male/Female" and means it for every later one too — so each slash-pair token a
// marker governs becomes a template, and every matching token on the page is
// emitted as a choice group. A slash is too ambiguous to trust alone (a bare
// "Home/Work Phone:" is a phrase, not a choice), but an exact match to a token
// the user already circled is safe: "Home/Work" never equals "Male/Female", so it
// is left untouched. Returns the same group shape as findYesNo.
export function findSlashTemplates(items) {
  const rows = buildTextRows(items);
  const slashTok = (r) => (r && r.words.find((w) => /^[A-Za-z]{2,}(\/[A-Za-z]{2,})+$/.test(w.text))) || null;
  const templates = new Set();
  for (let ri = 0; ri < rows.length; ri++) {
    if (!/circle\s+one/i.test(rows[ri].s)) continue;
    for (const r of [rows[ri], rows[ri + 1], rows[ri - 1]]) { const w = slashTok(r); if (w) { templates.add(w.text); break; } }
  }
  const out = [];
  if (!templates.size) return out;
  for (const row of rows) for (const w of row.words) if (templates.has(w.text)) out.push({ choices: splitSlash(w, row.y), marker: null });
  return out;
}

// findRunChoices locates a space-separated choice run with no delimiter and no
// "(circle one)" cue — "Type of Membership: Youth Teen Adult Senior Family". The
// signal is weak, so it is deliberately conservative: a run of >=3 short
// capitalised words at uniform wide spacing, introduced by a colon-label (same
// row to the left, or the row above in the same column), and clear of the
// detected table grid so office-use column headers don't match. Returns the same
// group shape as findYesNo.
export function findRunChoices(items, cells) {
  const rows = buildTextRows(items);
  const stop = /^(and|or|the|of|to|by|for|in|on|a|an|is|are)$/i;
  const ok = (t) => /^[A-Z][A-Za-z]{1,13}$/.test(t.text) && !stop.test(t.text);
  const labeled = (row, ri, run) => {
    const x = run[0].x0, h = run[0].h, above = rows[ri - 1];
    if (above && /:/.test(above.s) && above.words.some((w) => Math.abs(w.x0 - x) < h * 1.5)) return true;
    return row.words.some((w) => w.x1 < x && /:$/.test(w.text)); // colon label left of the run
  };
  const out = [];
  for (let ri = 0; ri < rows.length; ri++) {
    const row = rows[ri];
    if (/circle\s+one/i.test(row.s)) continue;
    const ws = row.words;
    for (let i = 0; i < ws.length; i++) {
      if (!ok(ws[i])) continue;
      let j = i; const gaps = [];
      while (j + 1 < ws.length && ok(ws[j + 1])) { gaps.push(ws[j + 1].x0 - ws[j].x1); j++; }
      const run = ws.slice(i, j + 1);
      if (run.length >= 3) {
        const gmin = Math.min(...gaps), gmax = Math.max(...gaps), h = run[0].h;
        const inCell = cells.some((c) => run[0].x0 < c.x1 && run[run.length - 1].x1 > c.x0 && row.y > c.y0 - 2 && row.y < c.y1 + 2);
        // A real choice set has DISTINCT options; a run with a repeated word is a
        // field-caption row ("Signature Date Signature Date" — two signature
        // blocks side by side), not choices. Reject duplicates (case-insensitive).
        const distinct = new Set(run.map((w) => w.text.toLowerCase())).size === run.length;
        if (distinct && gmin > h * 0.8 && gmax < gmin * 2.6 && !inCell && labeled(row, ri, run)) {
          out.push({ choices: run.map((w) => choiceBox(w.x0, w.x1, row.y, w.h)), marker: null });
        }
      }
      i = j; // don't rescan inside the run
    }
  }
  return out;
}

// dedupeGroups drops a later choice group whose box overlaps an earlier one, so
// a row a marked and a marker-free pass both catch yields a single widget. The
// marked / Y-N passes are listed first, so they win.
export function dedupeGroups(groups) {
  const bbox = (g) => ({ x0: Math.min(...g.choices.map((c) => c.x0)), y0: Math.min(...g.choices.map((c) => c.y0)), x1: Math.max(...g.choices.map((c) => c.x1)), y1: Math.max(...g.choices.map((c) => c.y1)) });
  const area = (b) => (b.x1 - b.x0) * (b.y1 - b.y0);
  const kept = [];
  for (const g of groups) {
    const b = bbox(g);
    const dup = kept.some((k) => {
      const a = bbox(k), ix = Math.min(a.x1, b.x1) - Math.max(a.x0, b.x0), iy = Math.min(a.y1, b.y1) - Math.max(a.y0, b.y0);
      return ix > 0 && iy > 0 && ix * iy > 0.5 * Math.min(area(a), area(b));
    });
    if (!dup) kept.push(g);
  }
  return kept;
}

// detectRegions scans a rendered page canvas for horizontal rule lines and for
// rectangles (boxes / checkboxes), via dark horizontal and vertical runs. It is
// a best-effort heuristic — it proposes regions; the user adjusts.
export function detectRegions(canvas) {
  const ctx = canvas.getContext('2d');
  const w = canvas.width, h = canvas.height;
  const data = ctx.getImageData(0, 0, w, h).data;
  // A "dark" pixel is reasonably dark AND roughly neutral (gray/black), so
  // anti-aliased thin black rules count but coloured section-dividers/callouts do
  // not — those aren't fillable fields.
  const dark = (x, y) => {
    const i = (y * w + x) * 4;
    if (data[i + 3] < 40) return false;
    const r = data[i], g = data[i + 1], b = data[i + 2];
    const mx = Math.max(r, g, b), mn = Math.min(r, g, b);
    return mx < 205 && mx - mn < 52; // dark-ish (incl. faint anti-aliased rules) and neutral
  };
  const maxThick = Math.max(3, Math.round(w / 280)); // a rule is thin at this resolution

  const minH = 11; // short enough to catch a checkbox edge, long enough to skip glyphs
  const minV = Math.max(8, Math.floor(h * 0.008));
  const gapTol = 2; // tolerate tiny anti-alias gaps without bridging text letters
  let hsegs = []; // {y, x0, x1}
  for (let y = 1; y < h - 1; y++) {
    let x0 = -1, lastDark = -1;
    for (let x = 0; x < w; x++) {
      if (dark(x, y)) { if (x0 < 0) x0 = x; lastDark = x; }
      else if (x0 >= 0 && x - lastDark > gapTol) {
        if (lastDark - x0 >= minH) hsegs.push({ y, x0, x1: lastDark });
        x0 = -1;
      }
    }
    if (x0 >= 0 && lastDark - x0 >= minH) hsegs.push({ y, x0, x1: lastDark });
  }
  // Keep only THIN segments first. A real rule is dark at its row but mostly
  // white just above/below; a text run is part of a tall glyph cluster. Filtering
  // here (before pairing/capping) drops the thousands of text fragments that
  // otherwise fill the cap and pair into false boxes.
  const isThinLine = (a) => {
    let thickness = 0, samples = 0;
    const step = Math.max(1, Math.floor((a.x1 - a.x0) / 20));
    for (let xx = a.x0; xx <= a.x1; xx += step) {
      if (!dark(xx, a.y)) continue;
      let up = 0, down = 0;
      for (let k = 1; k <= 12 && dark(xx, a.y - k); k++) up++;
      for (let k = 1; k <= 12 && dark(xx, a.y + k); k++) down++;
      thickness += up + down;
      samples++;
    }
    return samples > 0 && thickness / samples <= maxThick;
  };
  hsegs = hsegs.filter(isThinLine);
  // Median vertical ink thickness of a segment, robust to text glyphs that
  // cross the row (those spike a few samples but not the median). A field
  // underline is a hairline (~2px at this resolution); a section-divider bar,
  // box border, or underlined heading is heavier (>=3px).
  const lineThickness = (a) => {
    const ths = [];
    const step = Math.max(1, Math.floor((a.x1 - a.x0) / 30));
    for (let xx = a.x0; xx <= a.x1; xx += step) {
      let cy = -1;
      for (let k = -3; k <= 3; k++) if (dark(xx, a.y + k)) { cy = a.y + k; break; }
      if (cy < 0) continue;
      let up = 0, down = 0;
      for (let k = 1; k <= 40 && dark(xx, cy - k); k++) up++;
      for (let k = 1; k <= 40 && dark(xx, cy + k); k++) down++;
      ths.push(up + down + 1);
    }
    if (!ths.length) return 0;
    ths.sort((p, q) => p - q);
    return ths[ths.length >> 1];
  };
  if (hsegs.length > 800) { // safety cap; rarely reached once text is gone
    hsegs.sort((a, b) => (b.x1 - b.x0) - (a.x1 - a.x0));
    hsegs = hsegs.slice(0, 800).sort((a, b) => a.y - b.y);
  }

  const vAt = (x, y0, y1) => { // a near-continuous vertical edge at column x?
    let hits = 0;
    for (let y = y0; y <= y1; y++) if (dark(x, y) || dark(x - 1, y) || dark(x + 1, y)) hits++;
    return hits >= (y1 - y0) * 0.7; // a real box side is solid
  };
  const maxBox = Math.round(w * 0.04); // checkbox size cap (~0.04 of page width)

  const regions = [];
  const used = new Array(hsegs.length).fill(false);
  // Pair small square edges into checkboxes ONLY — wide underlines are never
  // candidates, so they can't be consumed as box edges.
  for (let i = 0; i < hsegs.length; i++) {
    if (used[i]) continue;
    const a = hsegs[i];
    const aw = a.x1 - a.x0;
    if (aw > maxBox) continue;
    for (let j = i + 1; j < hsegs.length; j++) {
      if (used[j]) continue;
      const b = hsegs[j];
      const dy = b.y - a.y;
      if (dy < minV || dy > maxBox) continue;
      if (Math.abs(b.x0 - a.x0) > 8 || Math.abs(b.x1 - a.x1) > 8) continue;
      if (Math.abs(aw - dy) > Math.max(aw, dy) * 0.7) continue; // roughly square
      if (vAt(a.x0, a.y, b.y) && vAt(a.x1, a.y, b.y)) {
        // A real checkbox is mostly empty inside; a "box" traced over glyph
        // strokes (e.g. letters in a title) is not — reject those.
        let dn = 0, tot = 0;
        for (let yy = a.y + 3; yy < b.y - 2; yy += 2) for (let xx = a.x0 + 3; xx < a.x1 - 2; xx += 2) { tot++; if (dark(xx, yy)) dn++; }
        if (tot > 0 && dn / tot > 0.20) continue;
        regions.push({ x: a.x0, y: a.y, w: aw, h: dy, box: true });
        used[i] = used[j] = true;
        break;
      }
    }
  }
  // Remaining thin segments of field width are underlines (not full-width rules).
  for (let i = 0; i < hsegs.length; i++) {
    if (used[i]) continue;
    const a = hsegs[i];
    const aw = a.x1 - a.x0;
    if (aw < w * 0.022 || aw > w * 0.92) continue; // not too short, not a divider
    // A sheet-wide line that is thicker than a hairline is a section-divider
    // bar / box border / underlined heading, never a fill field — drop it.
    if (aw >= w * 0.6 && lineThickness(a) >= 3) continue;
    regions.push({ x: a.x0, y: a.y - 2, w: aw, h: 4, box: false });
  }
  // Drop near-duplicate regions (a thick edge spans a couple of rows).
  const out = [];
  for (const r of regions) {
    if (out.some((o) => Math.abs(o.x - r.x) < 8 && Math.abs(o.y - r.y) < 8 && Math.abs(o.w - r.w) < 14)) continue;
    out.push(r);
  }
  return out.slice(0, 120);
}

// detectFilledBoxes finds standalone bounded rectangles (a wide top rule and a
// parallel bottom rule joined by solid left/right sides) whose interior is
// text-dense — a callout/instruction box like "MINIMUM TIME COMMITMENT…". The
// caller suppresses detected fields inside these, since a box already full of
// printed text with no blank line is not something to fill. Wide boxes can span
// many grid columns, so they are found by rule-pairing, not the cell grid.
export function detectFilledBoxes(canvas) {
  const ctx = canvas.getContext('2d');
  const w = canvas.width, h = canvas.height;
  const data = ctx.getImageData(0, 0, w, h).data;
  const dark = (x, y) => {
    if (x < 0 || y < 0 || x >= w || y >= h) return false;
    const i = (y * w + x) * 4;
    if (data[i + 3] < 40) return false;
    const r = data[i], g = data[i + 1], b = data[i + 2];
    return Math.max(r, g, b) < 205 && Math.max(r, g, b) - Math.min(r, g, b) < 52;
  };
  // Wide horizontal rules (candidate box tops/bottoms), one per y-cluster.
  const minW = w * 0.30;
  const rules = [];
  for (let y = 1; y < h - 1; y++) {
    let x0 = -1, last = -1;
    for (let x = 0; x < w; x++) {
      if (dark(x, y) || dark(x, y - 1) || dark(x, y + 1)) { if (x0 < 0) x0 = x; last = x; }
      else if (x0 >= 0 && x - last > 2) { if (last - x0 >= minW) rules.push({ y, x0, x1: last }); x0 = -1; }
    }
    if (x0 >= 0 && last - x0 >= minW) rules.push({ y, x0, x1: last });
  }
  rules.sort((a, b) => a.y - b.y);
  const dedup = [];
  for (const r of rules) { const p = dedup[dedup.length - 1]; if (p && r.y - p.y <= 4 && Math.abs(r.x0 - p.x0) < 12) continue; dedup.push(r); }

  const vSide = (x, ya, yb) => { let hit = 0, n = 0; for (let y = ya; y <= yb; y += 2) { n++; if (dark(x, y) || dark(x - 1, y) || dark(x + 1, y)) hit++; } return n > 0 && hit / n > 0.7; };
  const boxes = [];
  // Pair each wide rule with the very next one only: a clean callout box has
  // nothing wide between its top and bottom. Tables (row rules between) and
  // stacked underlines don't qualify, which keeps this to genuine boxes.
  for (let i = 0; i < dedup.length - 1; i++) {
    const top = dedup[i], bot = dedup[i + 1];
    const dy = bot.y - top.y;
    if (dy < 25 || dy > h * 0.30) continue;
    if (Math.abs(bot.x0 - top.x0) > 12 || Math.abs(bot.x1 - top.x1) > 12) continue;
    if (!vSide(top.x0, top.y, bot.y) || !vSide(top.x1, top.y, bot.y)) continue;
    let dn = 0, tot = 0; // interior text density (exclude the border band)
    for (let yy = top.y + 5; yy < bot.y - 5; yy += 2) for (let xx = top.x0 + 5; xx < top.x1 - 5; xx += 2) { tot++; if (dark(xx, yy)) dn++; }
    if (tot > 0 && dn / tot > 0.02) boxes.push({ x0: top.x0, y0: top.y, x1: top.x1, y1: bot.y });
  }
  return boxes;
}

// detectFaintRules finds very light-gray field underlines that the dark() test
// (tuned for printed rules) misses — common on clean, modern forms. It keys on
// LOCAL vertical contrast (a thin run darker than the pixels just above and
// below) rather than an absolute darkness, so it works even on a shaded box and
// without flooding on near-white anti-aliasing. Returns underline regions in
// the same shape as detectRegions; the caller dedupes against those.
export function detectFaintRules(canvas) {
  const ctx = canvas.getContext('2d');
  const w = canvas.width, h = canvas.height;
  const data = ctx.getImageData(0, 0, w, h).data;
  const lum = (x, y) => { const i = (y * w + x) * 4; return data[i + 3] < 40 ? 255 : Math.max(data[i], data[i + 1], data[i + 2]); };
  const neutral = (x, y) => { const i = (y * w + x) * 4; if (data[i + 3] < 40) return false; const r = data[i], g = data[i + 1], b = data[i + 2]; return Math.max(r, g, b) - Math.min(r, g, b) < 52; };
  const ink = (x, y) => neutral(x, y) && lum(x, y) < 250 && Math.min(lum(x, y - 4), lum(x, y + 4)) - lum(x, y) >= 10;
  const minW = Math.round(w * 0.05);
  const out = [];
  for (let y = 4; y < h - 4; y++) {
    let x0 = -1, last = -1;
    const flush = (a, b) => {
      const aw = b - a;
      if (aw < minW || aw > w * 0.92) return;
      // Reject thick lines: a field rule is a hairline, a section-divider bar is
      // several px. Measure the vertical run of similar darkness at the rule.
      const ths = []; const st = Math.max(1, Math.floor(aw / 12));
      for (let x = a; x <= b; x += st) {
        const base = lum(x, y);
        let up = 0, down = 0;
        for (let k = 1; k <= 12 && lum(x, y - k) <= base + 12; k++) up++;
        for (let k = 1; k <= 12 && lum(x, y + k) <= base + 12; k++) down++;
        ths.push(up + down + 1);
      }
      ths.sort((p, q) => p - q);
      if (ths[ths.length >> 1] > 3) return; // thick → divider/bar, not a field rule
      out.push({ x: a, y: y - 2, w: aw, h: 4, box: false });
    };
    for (let x = 0; x < w; x++) {
      if (ink(x, y)) { if (x0 < 0) x0 = x; last = x; }
      else if (x0 >= 0 && x - last > 2) { flush(x0, last); x0 = -1; }
    }
    if (x0 >= 0) flush(x0, last);
  }
  out.sort((a, b) => a.y - b.y);
  const ded = [];
  for (const r of out) { if (ded.some((o) => Math.abs(o.y - r.y) < 8 && Math.abs(o.x - r.x) < 20)) continue; ded.push(r); }
  return ded.slice(0, 120);
}

// detectTableCells finds grid cells in a rendered page: rectangles bounded on
// all four sides by rules. Each cell is flagged `filled` if its interior holds
// printed text (so the caller can skip it) — blank cells become fillable.
export function detectTableCells(canvas) {
  const ctx = canvas.getContext('2d');
  const w = canvas.width, h = canvas.height;
  const data = ctx.getImageData(0, 0, w, h).data;
  const dark = (x, y) => {
    if (x < 0 || y < 0 || x >= w || y >= h) return false;
    const i = (y * w + x) * 4;
    if (data[i + 3] < 40) return false;
    const r = data[i], g = data[i + 1], b = data[i + 2];
    return Math.max(r, g, b) < 205 && Math.max(r, g, b) - Math.min(r, g, b) < 52;
  };
  // Column x-positions of long vertical rules, clustered; likewise row y's.
  const minV = Math.round(h * 0.02), minHrun = Math.round(w * 0.10);
  const vXs = [], hYs = [];
  for (let x = 1; x < w - 1; x++) {
    let y0 = -1, last = -1;
    for (let y = 0; y < h; y++) {
      if (dark(x, y) || dark(x - 1, y) || dark(x + 1, y)) { if (y0 < 0) y0 = y; last = y; }
      else if (y0 >= 0 && y - last > 2) { if (last - y0 >= minV) vXs.push(x); y0 = -1; }
    }
    if (y0 >= 0 && last - y0 >= minV) vXs.push(x);
  }
  for (let y = 1; y < h - 1; y++) {
    let x0 = -1, last = -1;
    for (let x = 0; x < w; x++) {
      if (dark(x, y) || dark(x, y - 1) || dark(x, y + 1)) { if (x0 < 0) x0 = x; last = x; }
      else if (x0 >= 0 && x - last > 2) { if (last - x0 >= minHrun) hYs.push(y); x0 = -1; }
    }
    if (x0 >= 0 && last - x0 >= minHrun) hYs.push(y);
  }
  const cluster = (vals, tol) => {
    vals.sort((a, b) => a - b);
    const cl = [];
    for (const v of vals) {
      const last = cl[cl.length - 1];
      if (last && v - last.sum / last.n <= tol) { last.sum += v; last.n++; }
      else cl.push({ sum: v, n: 1 });
    }
    return cl.map((c) => Math.round(c.sum / c.n));
  };
  const vx = cluster(vXs, 6), hy = cluster(hYs, 5);
  if (vx.length < 2 || hy.length < 2) return [];

  const cover = (pts, n) => { let hit = 0; for (let i = 0; i < n; i++) if (pts(i)) hit++; return hit / n > 0.6; };
  const hasH = (y, xa, xb) => cover((i) => { const x = xa + i * 2; return dark(x, y) || dark(x, y - 1) || dark(x, y + 1); }, Math.max(1, (xb - xa) >> 1));
  const hasV = (x, ya, yb) => cover((i) => { const y = ya + i * 2; return dark(x, y) || dark(x - 1, y) || dark(x + 1, y); }, Math.max(1, (yb - ya) >> 1));

  const cells = [];
  for (let i = 0; i < vx.length - 1; i++) {
    for (let j = 0; j < hy.length - 1; j++) {
      const x0 = vx[i], x1 = vx[i + 1], y0 = hy[j], y1 = hy[j + 1];
      if (x1 - x0 < 14 || y1 - y0 < 14) continue;
      if (!(hasH(y0, x0 + 2, x1 - 2) && hasH(y1, x0 + 2, x1 - 2) && hasV(x0, y0 + 2, y1 - 2) && hasV(x1, y0 + 2, y1 - 2))) continue;
      let dn = 0, tot = 0;
      for (let yy = y0 + 4; yy < y1 - 4; yy += 2) for (let xx = x0 + 4; xx < x1 - 4; xx += 2) { tot++; if (dark(xx, yy)) dn++; }
      cells.push({ x0, y0, x1, y1, filled: tot > 0 && dn / tot > 0.02 });
    }
  }
  return cells;
}

// detectGrid makes a BEST-EFFORT guess at how many columns and rows an imposed
// page is laid out in, by looking for sustained whitespace "gutters" between
// blocks of content. It only ever seeds the editable Columns/Rows inputs — it
// never splits on its own — because gutter detection on a real scan (speckle,
// off-white background, content touching the trim) is unreliable.
//
// The signal is RELATIVE, not an absolute white threshold: per column/row we sum
// darkness (255 − luminance) and normalize to the page's peak, so an off-white
// scan with no true white still resolves as long as the gutters are lighter than
// the content around them.
export function detectGrid(canvas) {
  const w = canvas.width, h = canvas.height;
  const data = canvas.getContext('2d').getImageData(0, 0, w, h).data;
  return {
    cols: countBands(inkProfile(data, w, h, true)),
    rows: countBands(inkProfile(data, w, h, false)),
  };
}

// inkProfile returns per-column (vertical) or per-row darkness, each normalized
// to the page's peak so the empty/content cut is relative to this page's range.
function inkProfile(data, w, h, vertical) {
  const len = vertical ? w : h, span = vertical ? h : w;
  const ink = new Float64Array(len);
  let peak = 0;
  for (let a = 0; a < len; a++) {
    let sum = 0;
    for (let b = 0; b < span; b++) {
      const x = vertical ? a : b, y = vertical ? b : a;
      const i = (y * w + x) * 4;
      sum += 255 - (0.299 * data[i] + 0.587 * data[i + 1] + 0.114 * data[i + 2]);
    }
    ink[a] = sum / span;
    if (ink[a] > peak) peak = ink[a];
  }
  if (peak > 0) for (let i = 0; i < len; i++) ink[i] /= peak;
  return ink;
}

// countBands trims the page margins to the content extent, then counts interior
// gutters — runs of near-empty lines at least 4% of the content extent wide
// (wider than an inter-letter gap) — and returns bands = gutters + 1, capped.
function countBands(ink) {
  const len = ink.length, empty = 0.06;
  let c0 = 0, c1 = len - 1;
  while (c0 < len && ink[c0] <= empty) c0++;
  while (c1 > c0 && ink[c1] <= empty) c1--;
  if (c1 <= c0) return 1;
  const minGap = Math.max(3, (c1 - c0) * 0.04);
  let gutters = 0, run = 0;
  for (let i = c0; i <= c1; i++) {
    if (ink[i] <= empty) {
      run++;
    } else {
      if (run >= minGap) gutters++;
      run = 0;
    }
  }
  return Math.max(1, Math.min(8, gutters + 1));
}
