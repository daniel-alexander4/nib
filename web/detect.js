// Nib field-detection toolkit. Pure functions over a rendered page canvas and
// pdf.js text items (in canvas device pixels): they propose fillable fields —
// rules, boxes, checkboxes, table cells, and circle-the-answer choice groups.
// No app state, no pdf.js import; only the DOM canvas API (measureText / pixels).
// Extracted from app.js so the detection logic can be exercised in isolation.

// buildTextRows groups text items into rows; per row it gives a reconstructed
// string `s` with per-char x-centre `cx` and height `ch`, plus a `words` list
// with pixel x-spans. Shared by the circle-a-choice detectors below.
//
// A row is NOT one line (ADR-095, ADR-099): an item joins a row when its baseline is within 6 (or 0.6 of its height)
// of the row's FIRST item's, and `row.y` is that first item's — good for telling rows apart and wrong for placing
// anything in one. So each word carries `y`, the baseline of the item that drew it, and a character's is
// rowBaselines(row)[i]. Nothing is drawn at `row.y`.
export function buildTextRows(items) {
  // One-entry memo keyed on the ARRAY IDENTITY. Five detectors (findYesNo,
  // findCircleOne, findPipeChoices, findSlashTemplates, findRunChoices) each call this
  // with the same `items` array on every Detect click, and the row grouping below is
  // O(n²) — so the same quadratic pass ran five times over the same input.
  //
  // Identity, not contents: a different array recomputes, so a caller that builds a new
  // item list gets new rows. The returned rows are SHARED rather than fresh, which is
  // safe only because no caller mutates them — verified across all five, and the reason
  // this is a memo rather than a cache with a copy.
  if (buildTextRows._for === items && items.length) return buildTextRows._rows;
  const rows = [];
  for (const it of items.slice().sort((a, b) => a.y - b.y)) {
    const row = rows.find((r) => Math.abs(r.y - it.y) <= Math.max(6, it.h * 0.6));
    if (row) row.items.push(it); else rows.push({ y: it.y, h: it.h, items: [it] });
  }
  buildTextRows._for = items;
  buildTextRows._rows = rows;
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
      // edge[c] = x at start of char c; edge[len] = the far edge. An item pdf.js read right to left (`dir`) has its
      // first character at its RIGHT: laid out from the left, part of a Hebrew or Arabic word was boxed at the
      // mirrored end, and the letters searched for stayed on the page (ADR-097).
      const rtl = it.dir === 'rtl';
      const edge = [rtl ? it.x + it.w : it.x];
      for (let c = 0; c < it.str.length; c++) edge.push(edge[c] + (rtl ? -1 : 1) * adv[c] * sc);
      let i = 0;
      while (i < it.str.length) { // words within the item, with pixel spans
        if (/\s/.test(it.str[i])) { i++; continue; }
        let j = i; while (j < it.str.length && !/\s/.test(it.str[j])) j++;
        words.push({ text: it.str.slice(i, j), x0: Math.min(edge[i], edge[j]), x1: Math.max(edge[i], edge[j]), h: it.h, y: it.y });
        i = j;
      }
      for (let c = 0; c < it.str.length; c++) { s += it.str[c]; cx.push((edge[c] + edge[c + 1]) / 2); ch.push(it.h); }
    }
    words.sort((a, b) => a.x0 - b.x0);
    row.s = s; row.cx = cx; row.ch = ch; row.words = words;
  }
  return rows;
}

export function pixelsOf(canvas) {
  const W = canvas.width, H = canvas.height;
  return { data: canvas.getContext('2d').getImageData(0, 0, W, H).data, W, H };
}

// snapChoices refines choice boxes (canvas px) to the actual rendered glyphs.
// Sub-item text position is estimated from font metrics and drifts after a long
// underscore run, so when the real ink in the choice band resolves into exactly
// one word-cluster per choice, snap each box's x-extent to its cluster. If the
// count doesn't match (multi-word choices, touching glyphs), keep the estimate.
// `pixels` is optional and is the whole performance point: the caller loops over choice
// groups, and reading the full canvas per group costs a ~13 MB getImageData each time for
// a picture that has not changed. pixelsOf() above reads it once; passing nothing keeps
// the old self-contained behaviour for any other caller.
export function snapChoices(canvas, choices, marker, pixels) {
  if (choices.length < 2) return choices;
  const px = pixels || pixelsOf(canvas);
  const W = px.W, H = px.H;
  const data = px.data;
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
    const cy = rowBaselines(row);
    let m;
    while ((m = re.exec(row.s))) {
      const yi = m.index, ni = m.index + m[0].length - 1;
      const yx = row.cx[yi], nx = row.cx[ni];
      if (isNaN(yx) || isNaN(nx)) continue;
      const hh = row.ch[yi] || row.ch[ni] || 14;
      out.push({ choices: [choiceBox(yx, yx, cy[yi], hh), choiceBox(nx, nx, cy[ni], hh)] });
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
      // How far the other row is, is measured from the MARKER's own baseline to the nearest thing in that row: either
      // row may hold two baselines, and the rows' first ones can be 6 further apart, or nearer, than the words are.
      const my = rowBaselines(row)[row.s.search(/circle\s+one/i)];
      for (const dr of [rows[ri + 1], rows[ri - 1]]) {
        if (!dr || Math.min(...dr.items.map((it) => Math.abs(it.y - my))) > row.h * 2.2) continue;
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
        words.push({ text: parts[pi], x0: w.x0 + cw * off, x1: w.x0 + cw * (off + parts[pi].length), h: w.h, y: w.y });
        off += parts[pi].length;
        if (pi < parts.length - 1) { words.push({ text: '/', x0: w.x0 + cw * off, x1: w.x0 + cw * (off + 1), h: w.h, y: w.y, delim: true }); off++; }
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
  return best.map((seg) => choiceBox(Math.min(...seg.map((w) => w.x0)), Math.max(...seg.map((w) => w.x1)), seg[0].y, seg[0].h));
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
  for (const row of rows) for (const w of row.words) if (templates.has(w.text)) out.push({ choices: splitSlash(w, w.y), marker: null });
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
        const ry = run[0].y;
        const inCell = cells.some((c) => run[0].x0 < c.x1 && run[run.length - 1].x1 > c.x0 && ry > c.y0 - 2 && ry < c.y1 + 2);
        // A real choice set has DISTINCT options; a run with a repeated word is a
        // field-caption row ("Signature Date Signature Date" — two signature
        // blocks side by side), not choices. Reject duplicates (case-insensitive).
        const distinct = new Set(run.map((w) => w.text.toLowerCase())).size === run.length;
        if (distinct && gmin > h * 0.8 && gmax < gmin * 2.6 && !inCell && labeled(row, ri, run)) {
          out.push({ choices: run.map((w) => choiceBox(w.x0, w.x1, w.y, w.h)), marker: null });
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
    // The bounds check the other three pixel predicates in this file already have.
    // Without it `dark(x - 1, y)` at x = 0 indexes (y*w - 1)*4 — the PREVIOUS ROW'S LAST
    // PIXEL — and six call sites do exactly that on the left edge, so a rule touching the
    // margin was tested against unrelated ink. (A y underflow read undefined and answered
    // false, which is wrong but harmless; the x one wraps to real data.)
    if (x < 0 || y < 0 || x >= w || y >= h) return false;
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

// refineFields corrects proposed fields against the page map (ADR-088): where the page's text, ruled lines and
// existing form fields really are, as fractions of the displayed page.
//
// Detection proposes from a picture of the page, and a picture cannot tell a label from a blank: a cell with a small
// label in its corner is "blank" by ink, and an underlined sentence is an underline. The map can. So, for each
// proposal:
//
//   - one that sits where the document already has a real field is dropped — it is not a blank, it is a field;
//   - a text field is cut at any vertical rule running through it, because a field does not span two cells;
//   - it is moved clear of a label printed above the blank, and narrowed clear of text beside it;
//   - and what is left is kept only if it is still big enough to type in.
//
// `cands` are {kind, rect: [x0, y0, x1, y1], …}; anything else on a candidate is carried through. Returns the
// refined list and how many were dropped, so the caller can say so.
export function refineFields(cands, map) {
  const ptX = 1 / map.width, ptY = 1 / map.height; // one point, as a fraction of each axis
  const text = map.text.filter((t) => !t.hidden);   // an OCR layer is not print: the blank under it is still blank
  const vRules = map.shapes.filter((s) => s.kind === 'v');
  const mid = (r) => [(r[0] + r[2]) / 2, (r[1] + r[3]) / 2];
  const holds = (r, p) => p[0] >= r[0] && p[0] <= r[2] && p[1] >= r[1] && p[1] <= r[3];
  const MIN_W = 14 * ptX, MIN_H = 7 * ptY;
  const out = [];
  let dropped = 0;

  for (const c of cands) {
    const r = c.rect;
    if (map.widgets.some((w) => holds(w.rect, mid(r)) || holds(r, mid(w.rect)))) { dropped++; continue; }
    if (c.kind !== 'text') { out.push(c); continue; }

    // Cut at the vertical rules that run through it.
    const xs = vRules
      .filter((s) => {
        const x = (s.rect[0] + s.rect[2]) / 2;
        return x > r[0] + 3 * ptX && x < r[2] - 3 * ptX
          && Math.min(r[3], s.rect[3]) - Math.max(r[1], s.rect[1]) > 0.6 * (r[3] - r[1]);
      })
      .map((s) => [s.rect[0], s.rect[2]])
      .sort((a, b) => a[0] - b[0]);
    const parts = [];
    let left = r[0];
    for (const [a, b] of xs) { parts.push([left, r[1], a - ptX, r[3]]); left = b + ptX; }
    parts.push([left, r[1], r[2], r[3]]);

    let kept = 0;
    for (const p of parts) {
      const f = clearOfText(p, text, ptX, ptY);
      if (f && f[2] - f[0] >= MIN_W && f[3] - f[1] >= MIN_H) { out.push({ ...c, rect: f }); kept++; }
    }
    if (!kept) dropped++;
  }
  return { fields: out, dropped };
}

// clearOfText moves box clear of the printed text inside it, or returns null when no room is left.
function clearOfText(box, text, ptX, ptY) {
  let [x0, y0, x1, y1] = box;
  const over = (t) => t.rect[2] > x0 && t.rect[0] < x1 && t.rect[3] > y0 && t.rect[1] < y1;
  // A label printed in the top of the blank — "Last", above where the name goes — pushes the field's top below it,
  // when there is still a line's height left underneath.
  const upper = text.filter((t) => over(t) && (t.rect[1] + t.rect[3]) / 2 < y0 + 0.6 * (y1 - y0));
  if (upper.length) {
    const below = Math.max(...upper.map((t) => t.rect[3])) + ptY;
    if (y1 - below >= 8 * ptY) y0 = below;
  }
  // Whatever text still shares the field's band blocks the part of its width it stands in. Text that only grazes
  // the band — the descenders of the line above — does not.
  const blocks = text
    .filter((t) => over(t) && Math.min(t.rect[3], y1) - Math.max(t.rect[1], y0) > 0.3 * (t.rect[3] - t.rect[1]))
    .map((t) => [t.rect[0] - 1.5 * ptX, t.rect[2] + 1.5 * ptX])
    .sort((a, b) => a[0] - b[0]);
  let best = null, at = x0;
  const offer = (a, b) => { if (b - a > 0 && (!best || b - a > best[1] - best[0])) best = [a, b]; };
  for (const [a, b] of blocks) { offer(at, Math.min(a, x1)); at = Math.max(at, b); }
  offer(at, x1);
  return best ? [best[0], y0, best[1], y1] : null;
}

// ── fields proposed FROM the page map (ADR-089) ──────────────────────────────
//
// refineFields corrects what a picture of the page proposed. On a page whose lines and text are drawn — a form made
// by a program, not scanned — the map can propose the fields itself, because a form says where its blanks are in what
// it draws: a table's cell is the room between two ruled lines and the uprights that join them, with its label in
// the top; a blank to write on is a line with nothing above it, or a run of underscores; a checkbox is a small empty
// square, or a box drawn as a character.
//
// Measured on two real grid forms before this existed: the picture found 18 of 70 and 16 of 78 real fields, because
// a cell with a label in it is not blank by ink. Read from the map: 70 of 70 and 78 of 78.
//
// A blank a form backs with a white rectangle is read too (ADR-096) — an IRS form made in a designer draws its amount
// boxes that way, over a tint, and the tint used to refuse every one of them as "inside a shaded panel".
//
// What it does NOT read, and the picture still does (the caller adds those): a blank bounded only by shading, with no
// ground of its own, and anything on a page whose rules are an image.

// A run that is only separators — the "-" between the parts of a phone number, the "/" in a date — stands INSIDE a
// blank. It is print, and it is not a label.
const SEPARATORS = /^[\s\-–—\/\\().,:;|_]*$/;
// A box drawn as a character.
const BOX_GLYPHS = new Set(['☐', '□', '❏', '❐', '❑', '❒', '▢', '◻', '◽', '▫']);
const FIELD_MIN_W = 14, FIELD_MIN_H = 7; // points: the smallest thing to type in — refineFields' figures
const GAP_MIN_W = 24;                    // a cell with two blanks in it: each must be at least this wide
const ROW_MAX_H = 45;                    // two lines of equal width this close, with no uprights, are still a row
const ABOVE_LINE = 15;                   // a field on a bare line stands one line's height above it
const SQUARE_MAX = 20, SQUARE_MIN = 5;   // CHECKBOX_MAX_PT's figure: a checkbox is a size on the page
const HEAVY = 1.6;                       // a bar thicker than this divides sections; nobody writes on it
const HINT_PAD = 1.5;                    // a blank marked by a hint reaches this far past the hint's letters

const overlaps = (a, b) => a[2] > b[0] && a[0] < b[2] && a[3] > b[1] && a[1] < b[3];

// ruledLines reads the map's shapes as the lines of a form: every rule, a thin filled bar (how a heavy rule is often
// drawn), and the four sides of a large open box. Pieces that continue one another are one line — a row's top edge
// is commonly drawn as one piece per cell, and an upright two rows tall as two.
function ruledLines(map, ptX, ptY) {
  const tolX = 2.5 * ptX, tolY = 2.5 * ptY;
  const hs = [], vs = [];
  for (const s of map.shapes) {
    const [x0, y0, x1, y1] = s.rect;
    const w = (x1 - x0) / ptX, h = (y1 - y0) / ptY;
    if (s.kind === 'h' || (s.kind === 'box' && s.filled && h <= 4 && w > 8)) hs.push({ y: (y0 + y1) / 2, x0, x1, heavy: h > HEAVY });
    else if (s.kind === 'v' || (s.kind === 'box' && s.filled && w <= 4 && h > 8)) vs.push({ x: (x0 + x1) / 2, y0, y1 });
    else if (s.kind === 'box' && !s.filled && (w > SQUARE_MAX || h > SQUARE_MAX)) {
      hs.push({ y: y0, x0, x1 }, { y: y1, x0, x1 });
      vs.push({ x: x0, y0, y1 }, { x: x1, y0, y1 });
    }
  }
  const H = [], V = [];
  for (const s of hs.sort((a, b) => a.y - b.y || a.x0 - b.x0)) {
    const l = H.find((q) => Math.abs(q.y - s.y) <= tolY / 2 && s.x0 <= q.x1 + tolX && s.x1 >= q.x0 - tolX);
    if (l) { l.x0 = Math.min(l.x0, s.x0); l.x1 = Math.max(l.x1, s.x1); l.heavy = l.heavy || s.heavy; } else H.push({ ...s });
  }
  for (const s of vs.sort((a, b) => a.x - b.x || a.y0 - b.y0)) {
    const l = V.find((q) => Math.abs(q.x - s.x) <= tolX / 2 && s.y0 <= q.y1 + tolY && s.y1 >= q.y0 - tolY);
    if (l) { l.y0 = Math.min(l.y0, s.y0); l.y1 = Math.max(l.y1, s.y1); } else V.push({ ...s });
  }
  H.sort((a, b) => a.y - b.y);
  return { H, V, tolX, tolY };
}

// blanksIn returns the parts of box a person could type in: below a label printed in its top (unless `bare` — the
// room above a bare line has no label of its own), and beside whatever shares its band. `solid` is what stands in
// a blank's way: print and tick boxes, as {rect}. `hints` is the page's faint print, which stands in nobody's way and
// says how a blank is divided (byHints).
function blanksIn(box, solid, ptX, ptY, bare, hints) {
  const [x0, , x1, y1] = box;
  let y0 = box[1];
  const inBox = (t) => t.rect[2] > x0 && t.rect[0] < x1 && t.rect[3] > y0 && t.rect[1] < y1;
  if (!bare) {
    const upper = solid.filter((t) => inBox(t) && (t.rect[1] + t.rect[3]) / 2 < y0 + 0.6 * (y1 - y0));
    if (upper.length) {
      // A text box is taller than its ink (it runs to the font's descent), so the blank starts a point inside it.
      const below = Math.max(...upper.map((t) => t.rect[3])) - ptY;
      if (y1 - below >= 8 * ptY) y0 = below;
    }
  }
  if (y1 - y0 < FIELD_MIN_H * ptY) return [];
  const blocks = solid
    .filter((t) => inBox(t) && Math.min(t.rect[3], y1) - Math.max(t.rect[1], y0) > 0.3 * (t.rect[3] - t.rect[1]))
    .map((t) => [t.rect[0] - 1.5 * ptX, t.rect[2] + 1.5 * ptX])
    .sort((a, b) => a[0] - b[0]);
  const gaps = [];
  let at = x0;
  for (const [a, b] of blocks) { if (Math.min(a, x1) > at) gaps.push([at, Math.min(a, x1)]); at = Math.max(at, b); }
  if (x1 > at) gaps.push([at, x1]);
  // Every gap wide enough to be a blank of its own ("( ___ ) ___ ext ___"); failing that, the widest, if it is a field.
  const wide = gaps.filter((g) => g[1] - g[0] >= GAP_MIN_W * ptX);
  const keep = wide.length ? wide
    : gaps.filter((g) => g[1] - g[0] >= FIELD_MIN_W * ptX).sort((a, b) => (b[1] - b[0]) - (a[1] - a[0])).slice(0, 1);
  return keep.flatMap((g) => byHints([g[0], y0, g[1], y1], hints || [], ptX, ptY));
}

// byHints divides a blank by the hints printed in it (ADR-099). A hint is FAINT print (`faint` on the map: painted
// light, where a label is in ink) — what a form prints inside a blank to say what goes there. One hint, however many
// words, leaves the blank whole: "First name" in grey is one entry. Two or more set apart along it are one entry
// each — the IRS 1040's dates are a single white ground holding "MM", "DD" and "YYYY", and three fields. Each entry
// is as wide as its hint's letters and HINT_PAD more (the form sized the hint to the entry), and the outer two reach
// the blank's own edges.
function byHints(blank, hints, ptX, ptY) {
  const [x0, y0, x1, y1] = blank;
  const ink = [];
  for (const t of hints) {
    if (!(t.rect[2] > x0 && t.rect[0] < x1 && t.rect[3] > y0 && t.rect[1] < y1)) continue;
    const em = (t.size / ptY) * ptX; // the hint's size, as a fraction of the page's width
    // The letters themselves, where their boundaries are known: a run is often set with the spaces that carry it to
    // the next entry ("MM     ").
    if (!(t.chars && t.cuts && t.cuts.length === t.chars.length + 1)) { ink.push({ a: t.rect[0], z: t.rect[2], em }); continue; }
    t.chars.forEach((c, i) => { if (!SEPARATORS.test(c)) ink.push({ a: t.cuts[i], z: t.cuts[i + 1], em }); });
  }
  ink.sort((p, q) => p.a - q.a);
  const marks = [];
  for (const k of ink) {
    const last = marks[marks.length - 1];
    if (last && k.a - last.z < Math.max(k.em, last.em)) last.z = Math.max(last.z, k.z); // nearer than its own size: one hint
    else marks.push({ ...k });
  }
  if (marks.length < 2) return [blank];
  return marks.map((m, i) => [
    i ? Math.max(x0, m.a - HINT_PAD * ptX) : x0, y0,
    i < marks.length - 1 ? Math.min(x1, m.z + HINT_PAD * ptX) : x1, y1,
  ]);
}

// proposeFields reads the page map for the fields the page itself draws. Returns {kind: 'text' | 'check', rect, from}
// in the map's own fractions; `from` says what was read ('cell', 'line', 'ground', 'underscores', 'square', 'glyph').
// Nothing is proposed where the document already has a field, or inside a shaded panel unless on a white ground.
export function proposeFields(map) {
  if (!map || map.noText) return [];
  const ptX = 1 / map.width, ptY = 1 / map.height;
  const { H, V, tolX, tolY } = ruledLines(map, ptX, ptY);
  // Faint print is a hint, not a label (ADR-099): it stands in no blank's way, and it says how one is divided.
  const print = map.text.filter((t) => !t.hidden && !t.faint && !SEPARATORS.test(t.text));
  const hints = map.text.filter((t) => t.faint && !SEPARATORS.test(t.text)); // (the map marks no hidden text faint)
  const shaded = map.shapes.filter((s) => s.kind === 'box' && s.filled
    && (s.rect[3] - s.rect[1]) / ptY > 4 && (s.rect[2] - s.rect[0]) / ptX > 4);
  const mid = (r) => [(r[0] + r[2]) / 2, (r[1] + r[3]) / 2];
  const holds = (r, p) => p[0] >= r[0] && p[0] <= r[2] && p[1] >= r[1] && p[1] <= r[3];
  const fields = [];
  const push = (kind, rect, from, overShade) => {
    if (!overShade && shaded.some((s) => holds(s.rect, mid(rect)))) return;
    if ((map.widgets || []).some((w) => holds(w.rect, mid(rect)) || holds(rect, mid(w.rect)))) return;
    fields.push({ kind, rect, from });
  };

  // Small empty squares are checkboxes — and they stand in a row the way a word does.
  const squares = map.shapes.filter((s) => {
    if (s.kind !== 'box' || s.filled) return false;
    const w = (s.rect[2] - s.rect[0]) / ptX, h = (s.rect[3] - s.rect[1]) / ptY;
    return w >= SQUARE_MIN && h >= SQUARE_MIN && w <= SQUARE_MAX && h <= SQUARE_MAX && Math.abs(w - h) < 4
      && !print.some((t) => overlaps(t.rect, s.rect));
  });
  for (const s of squares) push('check', s.rect, 'square');
  const solid = [...print, ...squares];

  // Rows. Each stretch of a line is closed by the nearest line below it; a cell two rows deep, by the second.
  // `closes` holds every line that is an edge of a row, top or bottom: a table's top edge is not a line to write on.
  const closes = new Set();
  const row = (T, B, x0, x1, sameWidth) => {
    const ups = V.filter((v) => v.x > x0 - tolX && v.x < x1 + tolX && v.y0 <= T.y + tolY && v.y1 >= B.y - tolY)
      .map((v) => v.x).sort((a, b) => a - b);
    // No upright joins them: a row only if the two lines are one width and close. Otherwise B is a line to write on.
    if (!ups.length && !(sameWidth && (B.y - T.y) / ptY <= ROW_MAX_H)) return;
    closes.add(B); closes.add(T);
    const xs = [x0, ...ups.filter((x) => x > x0 + tolX && x < x1 - tolX), x1];
    for (let i = 0; i + 1 < xs.length; i++) {
      const cell = [xs[i] + ptX, T.y + ptY, xs[i + 1] - ptX, B.y - ptY];
      const w = (cell[2] - cell[0]) / ptX, h = (cell[3] - cell[1]) / ptY;
      if (w < SQUARE_MIN || h < SQUARE_MIN) continue;
      const inside = solid.filter((t) => overlaps(t.rect, cell));
      if (w <= SQUARE_MAX && h <= SQUARE_MAX) { if (!inside.length) push('check', cell, 'cell'); continue; }
      // A cell of tick boxes: what is left of it is their labels' room.
      if (inside.some((t) => squares.includes(t))) continue;
      if (!ups.length && !inside.length) {
        push('text', [cell[0], Math.max(cell[1], cell[3] - ABOVE_LINE * ptY), cell[2], cell[3]], 'line');
        continue;
      }
      for (const f of blanksIn(cell, solid, ptX, ptY, false, hints)) push('text', f, 'cell');
    }
  };
  for (const T of H) {
    let open = [[T.x0, T.x1]];
    for (const B of H) {
      if (!open.length) break;
      if (B.y <= T.y + FIELD_MIN_H * ptY) {
        // A line just under this one closes what it covers, with no room between them — the gap between two boxes
        // stacked in a column, a double rule. The lower line opens its own rows; this one's do not reach past it.
        if (B.y > T.y + tolY / 2) {
          open = open.flatMap(([a, z]) => {
            const x0 = Math.max(a, B.x0), x1 = Math.min(z, B.x1);
            if (x1 - x0 <= FIELD_MIN_W * ptX) return [[a, z]];
            return [[a, x0], [x1, z]].filter(([p, q]) => q - p > FIELD_MIN_W * ptX);
          });
        }
        continue;
      }
      const whole = open.length === 1 && Math.abs(B.x0 - T.x0) <= tolX && Math.abs(B.x1 - T.x1) <= tolX;
      const next = [];
      for (const [a, z] of open) {
        const x0 = Math.max(a, B.x0), x1 = Math.min(z, B.x1);
        if (x1 - x0 <= FIELD_MIN_W * ptX) { next.push([a, z]); continue; }
        if (x0 - a > FIELD_MIN_W * ptX) next.push([a, x0]);
        if (z - x1 > FIELD_MIN_W * ptX) next.push([x1, z]);
        row(T, B, x0, x1, whole);
      }
      open = next;
    }
  }

  // A line that is no row's edge is a line to write on, where there is room above it.
  for (const L of H) {
    if (closes.has(L) || L.heavy) continue;
    const box = [L.x0, L.y - ABOVE_LINE * ptY, L.x1, L.y - 0.5 * ptY];
    if (fields.some((f) => overlaps(f.rect, box))) continue;
    for (const f of blanksIn(box, solid, ptX, ptY, true, hints)) push('text', f, 'line');
  }

  // A white ground is a blank the form laid out for itself (ADR-096): a rectangle painted white and nothing else, which
  // is how a form made in a designer backs each field — and on a tinted page it is the only edge the blank has. Read
  // last among the shapes, and only where the lines proposed nothing: where both see a blank, the lines' reading has
  // the label's room taken out of it already. A ground is the one thing proposed INSIDE a shaded panel, because it is
  // painted over the shade (the map drops a ground that something was painted over).
  const grounds = map.shapes.filter((s) => s.kind === 'white')
    .map((s) => ({ rect: s.rect, w: (s.rect[2] - s.rect[0]) / ptX, h: (s.rect[3] - s.rect[1]) / ptY }))
    .filter((g) => g.w >= SQUARE_MIN && g.h >= SQUARE_MIN && g.h <= ROW_MAX_H)
    .sort((a, b) => a.w * a.h - b.w * b.h); // the smallest first: a tick box before the cell it stands in
  const tick = (g) => g.w <= SQUARE_MAX && g.h <= SQUARE_MAX && Math.abs(g.w - g.h) < 4;
  // Two grounds side by side share an edge, and print set beside a tick box touches it: what counts is a point inside.
  const inner = (r) => [r[0] + ptX, r[1] + ptY, r[2] - ptX, r[3] - ptY];
  const ticks = grounds.filter((g) => tick(g) && !print.some((t) => overlaps(t.rect, inner(g.rect))));
  const taken = (r) => fields.some((f) => overlaps(f.rect, inner(r)));
  for (const g of grounds) {
    if (taken(g.rect)) continue;
    if (tick(g)) { if (ticks.includes(g)) push('check', g.rect, 'ground', true); continue; }
    // (A ground holding a tick box is taken by it: what is left of that cell is the tick box's label's room.)
    // An upright through a ground does NOT cut it. Measured on the 1040: nine grounds are crossed by one, every one is
    // a single real field (a comb's dividers), and cutting there lost seven well-placed fields and gained none.
    for (const f of blanksIn(g.rect, solid, ptX, ptY, false, hints)) push('text', f, 'ground', true);
  }

  // A run of underscores is a blank drawn with the keyboard; a box drawn as a character is a checkbox.
  for (const t of map.text) {
    if (t.hidden) continue;
    const chars = t.chars && t.cuts && t.cuts.length === t.chars.length + 1 ? t.chars : null;
    if (!chars) { if (/^_{4,}$/.test(t.text.trim())) push('text', t.rect, 'underscores'); continue; }
    for (let i = 0; i < chars.length; i++) {
      if (BOX_GLYPHS.has(chars[i])) { push('check', [t.cuts[i], t.rect[1], t.cuts[i + 1], t.rect[3]], 'glyph'); continue; }
      if (chars[i] !== '_') continue;
      let j = i;
      while (j + 1 < chars.length && chars[j + 1] === '_') j++;
      if (j - i + 1 >= 4) push('text', [t.cuts[i], t.rect[1], t.cuts[j + 1], t.rect[3]], 'underscores');
      i = j;
    }
  }
  return fields;
}

// mergeProposals puts the two readings of one page together: what the map proposed stands, and a proposal from the
// picture is added only where the map proposed nothing — the picture sees a blank bounded by shading, the map sees
// one bounded by lines or laid on a white ground, and where both see it the map's edges are the page's own.
export function mergeProposals(fromMap, fromPicture) {
  const area = (r) => Math.max(0, r[2] - r[0]) * Math.max(0, r[3] - r[1]);
  const shared = (a, b) => area([Math.max(a[0], b[0]), Math.max(a[1], b[1]), Math.min(a[2], b[2]), Math.min(a[3], b[3])]);
  return [...fromMap, ...fromPicture.filter((p) => !fromMap.some((m) => shared(m.rect, p.rect) > 0))];
}

// ── search matches placed from the page map (ADR-090) ────────────────────────
//
// A search-redaction box was placed from an ESTIMATE: the characters of a text item measured in a generic sans-serif
// and rescaled to the item's width (buildTextRows), then padded by 0.8 of a line height each side because the
// estimate could not be trusted. Measured: a mean 2.6 glyph-widths past the word, neighbouring glyphs blacked out in
// 72% of cases. The page map has each glyph's true boundary, from the font's own widths.

const MATCH_PAD_X = 0.75, MATCH_PAD_Y = 0.5; // points past the glyphs' own boxes: ink overhangs an advance, slightly
// Print drawn to its font's own ink (ADR-098) has no slack but the pad, where the reach had a quarter of a size:
// and the ink is the glyph's OUTLINE, which a renderer sets on its own pixel grid — measured at 300 dpi, ink stood up
// to 0.36pt past the outline, and a pixel on a screen is 0.75pt. So such a box gets a whole point. Measured cost over
// the corpus's 6,490 words against half a point: two more boxes touch the line above, none the line below.
const INK_PAD_Y = 1.0;
const WORD_GAP = 0.15;                       // of the font size: a wider gap between two runs is a space

// A SHORT stamp (`short` on the map: an OCR word as Nib wrote it before ADR-092, or one it could not fit) is narrower
// than the scanned word it stands for. Measured on two scans (3,855 words): its width
// is a median 0.72 of the word's, so the ink runs a median 3-4.5pt past its last glyph, 9-12pt at the ninetieth
// percentile — and a redaction drawn to the glyphs left the end of the word showing in 30 of 40 searches. What IS
// true of a stamped word is where it starts, so a hidden run is read as stretching toward the next hidden run on its
// line: never past it, and never more than HIDDEN_STRETCH_MAX of its own width (the ninety-ninth percentile needed
// 2.14). Any other hidden run — a word fitted to its box, another tool's layer — is read as it is written: its size is
// the line's, and a taller box would reach the lines either side (measured: 17 of 40 boxes on a real scan).
const HIDDEN_STRETCH_MAX = 2.5;
const HIDDEN_PAD_Y = 0.25; // of the font size: the stamped box sat a point above the bottom of the ink

// A hidden word in a right-to-left script was written in READING order, its first letter at the LEFT — the stamp is
// set glyph after glyph and unshaped (`ocrFontFor`) — while the scanned word's first letter is at its right. So the
// glyph boundaries say where a letter is in the stamp and not where it is on the paper: part of such a word was boxed
// at the mirrored end, the letters asked for left showing (/pending 851 part 3). Where the letters of a scanned
// word are is not known, so a match that takes any of the word takes all of it.
//
// Since ADR-097 Nib sets such a word last letter first and says so (`reversed` on the map, with its cuts running
// from the right), so its letters ARE where the scan has them — in a script whose letters stand apart. A Hebrew word
// written that way and fitted to its box is boxed by its glyphs. An Arabic-script word is still taken whole: its
// letters are stamped in their separate forms and printed joined, and the boundaries between them are not the
// page's (measured on the face it is stamped in: off by more than half a letter in 23 of 30 words). So is a word
// stamped before ADR-097, a short stamp, and another tool's layer.
const RTL = /[\u0590-\u08FF\uFB1D-\uFDFF\uFE70-\uFEFF]/;
const JOINED = /[\u0600-\u08FF\uFB50-\uFDFF\uFE70-\uFEFF]/; // Arabic, Syriac, Thaana, N'Ko …: everything in RTL but Hebrew
const lettersWhereScanned = (t) => !!t.reversed && !t.short && !JOINED.test(t.text);

// matchesInMap runs `patterns` (global RegExps) over the page's text as the map has it, and returns one box per
// match, [x0, y0, x1, y1] in the map's fractions, from the first matched glyph's left cut to the last one's right
// (the other way about in a run set in reverse — the box is over the matched glyphs' boundaries either way).
//
// Up and down a box is its runs' reach — a full size above the baseline — except over a run whose own ink the map
// carries (`ink`): a fitted OCR word (ADR-093), where the reach stood 1.65 times as tall as the word and took the
// line above with it, and print whose font's own glyphs say how far it reaches (ADR-098), where the reach took the
// tails of the line above for one word in eight and stopped short of a deep descender. Such a box also carries
// `hold`, the box it would have had from the reach.
// placeMatches matches an estimate against THAT. An estimate's centre is 0.35 of the size above its baseline, which
// is inside the reach always and inside the ink only where the match has ink there: not for a match that is all
// apostrophes or all full stops. (ADR-093 measured 6 of 39 searches keeping a second box and blamed pdf.js's span;
// the estimate was being drawn on its row's first baseline, which ADR-095 fixed — re-measured with no `hold`,
// 0 of 79 whole words keep one, ADR-097. `hold` stays for the matches whole words are not.)
//
// Only runs whose glyph boundaries are known take part (an upright run on an unturned page). Visible text and hidden
// text — an OCR layer — are searched as two layers, never joined into one line; a short stamp's boundaries are
// stretched as above. What this cannot place, the estimate still does — see placeMatches.
export function matchesInMap(map, patterns) {
  if (!map || !map.text) return [];
  const known = map.text.filter((t) => t.cuts && t.chars && t.cuts.length === t.chars.length + 1);
  const print = layerMatches(map, patterns, known.filter((t) => !t.hidden), false);
  const under = layerMatches(map, patterns, known.filter((t) => t.hidden), true);
  const out = [...print, ...under];
  if (print.skipped || under.skipped) out.skipped = true; // not every line end was searched across: see WRAP_BUDGET
  return out;
}

function layerMatches(map, patterns, runs, hidden) {
  const rows = [];
  for (const t of runs.slice().sort((a, b) => a.rect[1] - b.rect[1])) {
    const mid = (t.rect[1] + t.rect[3]) / 2, tall = t.rect[3] - t.rect[1];
    const row = rows.find((r) => Math.abs(r.mid - mid) <= 0.5 * Math.min(tall, r.tall));
    if (row) row.runs.push(t); else rows.push({ mid, tall, runs: [t] });
  }
  const out = [];
  const pieces = []; // each row's stretches of text, for the matches that wrap a line (wrappedMatches)
  for (const [line, row] of rows.entries()) {
    row.runs.sort((a, b) => a.rect[0] - b.rect[0]);
    // How far each run's glyph boundaries are stretched from its start: 1 for print.
    const stretch = row.runs.map((t, k) => {
      if (!t.short) return 1; // print, and a hidden word that already spans its scanned word (ADR-092)
      const wide = t.rect[2] - t.rect[0];
      const next = row.runs.slice(k + 1).find((u) => u.rect[0] > t.rect[0] + 1 / map.width);
      const room = next ? (next.rect[0] - 0.5 / map.width - t.rect[0]) / wide : HIDDEN_STRETCH_MAX;
      return Math.max(1, Math.min(HIDDEN_STRETCH_MAX, room));
    });
    const cut = (g, i) => g.t.rect[0] + (g.t.cuts[i] - g.t.rect[0]) * stretch[g.k];
    const whole = row.runs.map((t) => hidden && RTL.test(t.text) && !lettersWhereScanned(t)); // see RTL
    // The row as a string, each character knowing which glyph of which run drew it (null: a space between runs).
    let s = '';
    const at = [];
    const starts = [0]; // where in s a stretch begins: the row's start, and after each gap a column would leave
    row.runs.forEach((t, k) => {
      if (k > 0) {
        const prev = row.runs[k - 1];
        const gapPt = (t.rect[0] - prev.rect[2]) * map.width, sizePt = Math.min(t.size, prev.size) * map.height;
        if (gapPt > WORD_GAP * sizePt && !/\s$/.test(s)) { s += ' '; at.push(null); }
        if (gapPt > WRAP_GAP * sizePt) starts.push(s.length);
      }
      t.chars.forEach((c, i) => { for (const ch of c) { s += ch; at.push({ t, i, k }); } });
    });
    // The box over s[from, to): null when it holds no ink.
    const boxOf = (from, to) => {
      let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity, r0 = Infinity, r1 = -Infinity, inked = false, print = false;
      for (let k = from; k < to; k++) {
        const g = at[k];
        if (!g || !s[k].trim()) continue; // a space takes no ink
        const padY = g.t.short ? HIDDEN_PAD_Y * g.t.size : 0;
        const ink = !g.t.short && g.t.ink && g.t.ink.length === 2 ? g.t.ink : null;
        // Both boundaries of each glyph: a reversed run's cuts run from the right.
        const a = cut(g, whole[g.k] ? 0 : g.i), b = cut(g, whole[g.k] ? g.t.chars.length : g.i + 1);
        x0 = Math.min(x0, a, b); x1 = Math.max(x1, a, b);
        r0 = Math.min(r0, g.t.rect[1] - padY); r1 = Math.max(r1, g.t.rect[3] + padY);
        y0 = Math.min(y0, ink ? ink[0] : g.t.rect[1] - padY); y1 = Math.max(y1, ink ? ink[1] : g.t.rect[3] + padY);
        inked = inked || !!ink;
        print = print || (!!ink && !hidden);
      }
      if (!(x1 > x0)) return null;
      const px = MATCH_PAD_X / map.width, py = MATCH_PAD_Y / map.height, iy = (print ? INK_PAD_Y : MATCH_PAD_Y) / map.height;
      const box = [x0 - px, y0 - iy, x1 + px, y1 + iy];
      // Ink inside the reach never draws a box past the one the reach drew: the wider pad is room for the ink, and
      // where the font's glyphs stop only a hair short of the reach it would otherwise make the box LARGER.
      if (print && y0 >= r0) box[1] = Math.max(box[1], r0 - py);
      if (print && y1 <= r1) box[3] = Math.min(box[3], r1 + py);
      if (inked) box.hold = [x0 - px, r0 - py, x1 + px, r1 + py];
      return box;
    };
    for (const re of patterns) {
      re.lastIndex = 0;
      let m;
      while ((m = re.exec(s))) {
        if (!m[0].length) { re.lastIndex++; continue; }
        const box = boxOf(m.index, m.index + m[0].length);
        if (box) out.push(box);
      }
    }
    starts.forEach((from, n) => {
      const to = n + 1 < starts.length ? starts[n + 1] : s.length;
      const drawn = at.slice(from, to).filter(Boolean);
      if (!drawn.length) return;
      pieces.push({ row: line, x0: drawn[0].t.rect[0], x1: drawn[drawn.length - 1].t.rect[2], s: s.slice(from, to), boxOf: (a, b) => boxOf(from + a, from + b) });
    });
  }
  // A match that wraps a line: one box on each line it is on.
  const wrapped = wrappedMatches(pieces, patterns);
  for (const parts of wrapped) {
    for (const p of parts) { const box = p.piece.boxOf(p.from, p.to); if (box) out.push(box); }
  }
  if (wrapped.skipped) out.skipped = true;
  return out;
}

// ── A match that wraps a line ────────────────────────────────────────────────────────────────────────────────────────
// Both readings of a page search it a row at a time, so a phrase whose first word ends one line and whose second
// begins the next was found by neither: no box, no count, and a search that said "No matches" over a name in plain
// sight (measured through scanTextMatches, test/jsdom/wrappedmatch.test.mjs). For a redaction that is the dangerous
// direction, so both readings now also search ACROSS the line end, through this one door.
//
// `pieces` are a page's stretches of text: {row, x0, x1, s} — `row` counts lines down the page, x0..x1 is the
// stretch's extent across it, in any one unit. A row is one piece unless a gap wider than WRAP_GAP of the font size
// splits it: that is a column's gutter or a table's cell, and text wraps from the end of ITS column, not from the end
// of the row. Text carries on from a piece in the next row down that has a piece beginning left of this one's end —
// in every such piece of that row, because which of them is the same column is not known; a wrong guess costs a
// search over two strings and can only add a box.
//
// A line end is read three ways: as a space; and where the line ends in a hyphen, as nothing ("123-45-" + "6789") and
// as nothing with the hyphen dropped ("confi-" + "dential"). A match counts when it has ink on every line of the
// chain, the first and the last included — so a match inside one piece is left to the row's own search and is not
// found twice. Returns one entry per match: its parts, [{piece, from, to}], indices into each piece's own `s`.
//
// Not found: a match that wraps a PAGE, and one that runs over more than WRAP_LINES lines.
//
// WRAP_BUDGET (ADR-100) is the most chains one call will search. Past it the chains of three lines are left out, and
// if those of two lines alone are still past it, all of them — and the result says so (`skipped`), because a search
// that looked in fewer places and said nothing would be a redaction that reads as complete. The caller tells the
// user which page. The densest page of the accuracy corpus makes 9,386 chains (a tax form with 25 stretches on one
// line; 27 ms); the budget is 21 times that, and about a second of searching for five patterns (4–5 µs a chain).
const WRAP_GAP = 1.0, WRAP_LINES = 3, WRAP_BUDGET = 200000;
const LINE_HYPHEN = /[-\u00AD\u2010\u2011]$/; // a hyphen, a soft hyphen, and the two Unicode hyphens

//
// Cost. A piece is joined to every piece of the next row that begins left of its end, so a table of S cells a line
// makes about S²/2 chains of two lines and S³/6 of three for each line, and each chain is one run of each pattern
// over its text — that count IS the rule, and no chain can be dropped without knowing what a pattern could match.
// What is paid per chain is kept to that run: which row a piece carries into is looked up by row, not found by
// reading every piece on the page; a chain's text is its pieces' own trimmed strings put end to end, and where a
// match's characters are in each piece is worked out from the offsets, only for a match that reaches the chain's
// first and last lines. (As first written each chain read the whole page and built its text a character at a time:
// 200 lines of 20 cells took 2.2–3.2 s for five patterns; times beside the differential test in
// test/jsdom/wrappedmatch.test.mjs, which holds that version as the oracle.)
export function wrappedMatches(pieces, patterns) {
  pieces = pieces.filter((p) => p.s.trim());
  const out = [], seen = new Set();
  // Each piece once: its place in `pieces`, and its text as each position in a chain reads it.
  const info = new Map();
  pieces.forEach((p, idx) => {
    const lead = p.s.length - p.s.trimStart().length, end = p.s.trimEnd().length;
    info.set(p, { idx, lead, end, hyphen: LINE_HYPHEN.test(p.s.slice(0, end)), on: null });
  });
  // The rows, down the page: the pieces of each (in the order given) and the leftmost start among them.
  const byRow = new Map();
  for (const p of pieces) {
    if (!(p.row === p.row)) continue;
    let r = byRow.get(p.row);
    if (!r) byRow.set(p.row, r = { row: p.row, min: Infinity, pieces: [] });
    r.pieces.push(p);
    if (p.x0 < r.min) r.min = p.x0;
  }
  const rows = [...byRow.values()].sort((a, b) => a.row - b.row);
  rows.forEach((r, n) => { r.n = n; });
  // The pieces text carries on into from `a`: those of the first row below it with a piece beginning left of a's end.
  const carriesOn = (a) => {
    const ia = info.get(a);
    if (ia.on) return ia.on;
    ia.on = [];
    const at = byRow.get(a.row);
    if (!at) return ia.on;
    for (let n = at.n + 1; n < rows.length; n++) {
      if (!(rows[n].min < a.x1)) continue;
      ia.on = rows[n].pieces.filter((b) => b.x0 < a.x1);
      break;
    }
    return ia.on;
  };
  const search = (chain) => {
    const infos = chain.map((p) => info.get(p));
    const hyphened = infos.some((f, n) => n < chain.length - 1 && f.hyphen);
    for (const mode of ['space', 'joined', 'dehyphenated']) {
      if (mode !== 'space' && !hyphened) continue; // no line of this chain ends in a hyphen: the same string again
      // The chain's text, and for each piece: where its share starts in the text (at), and which of its own
      // characters that is (from) up to which (to).
      let s = '';
      const at = [], from = [], to = [];
      chain.forEach((p, n) => {
        const f = infos[n], last = n === chain.length - 1;
        const hyphen = !last && mode !== 'space' && f.hyphen;
        from.push(n ? f.lead : 0);
        to.push((last ? p.s.length : f.end) - (hyphen && mode === 'dehyphenated' ? 1 : 0));
        at.push(s.length);
        s += p.s.slice(from[n], to[n]);
        if (!hyphen && !last) s += ' ';
      });
      const lastAt = at[at.length - 1], firstEnd = at[0] + to[0] - from[0];
      for (const re of patterns) {
        re.lastIndex = 0;
        let m;
        while ((m = re.exec(s))) {
          if (!m[0].length) { re.lastIndex++; continue; }
          const m0 = m.index, m1 = m.index + m[0].length;
          if (m0 >= firstEnd || m1 <= lastAt) continue; // not on the first line, or not on the last
          // The ink of the match in each piece: its first and last characters there that are not spaces.
          const parts = [];
          for (let n = 0; n < chain.length; n++) {
            let lo = Math.max(m0, at[n]), hi = Math.min(m1, at[n] + to[n] - from[n]);
            while (lo < hi && !s[lo].trim()) lo++;
            while (hi > lo && !s[hi - 1].trim()) hi--;
            if (!(hi > lo)) break; // not on every line of this chain: another chain's, or a row's own
            parts.push({ piece: chain[n], from: from[n] + lo - at[n], to: from[n] + hi - at[n] });
          }
          if (parts.length < chain.length) continue;
          const key = parts.map((p, n) => infos[n].idx + ':' + p.from + ':' + p.to).join('|');
          if (seen.has(key)) continue;
          seen.add(key);
          out.push(parts);
        }
      }
    }
  };
  // How many chains there are of two lines, of three, …: counted before any is searched, which costs a sum and no
  // pattern. `lines` is how many lines a chain may run over with the count still inside WRAP_BUDGET.
  let lines = 1, total = 0;
  let ends = new Map(); // how many chains of the length being counted end at each piece
  for (const a of pieces) ends.set(a, (ends.get(a) || 0) + 1);
  while (lines < WRAP_LINES) {
    const next = new Map();
    let n = 0;
    for (const [p, c] of ends) for (const b of carriesOn(p)) { next.set(b, (next.get(b) || 0) + c); n += c; }
    if (total + n > WRAP_BUDGET) { out.skipped = true; break; }
    total += n; ends = next; lines++;
  }
  const walk = (chain) => {
    if (chain.length > 1) search(chain);
    if (chain.length < lines) for (const b of carriesOn(chain[chain.length - 1])) walk([...chain, b]);
  };
  for (const a of pieces) walk([a]);
  return out;
}

// rowBaselines says, for each character of a buildTextRows row's string, the baseline of the item that drew it (NaN
// for the space between two items). A row is NOT one line: buildTextRows puts an item in a row when its baseline is
// within 6 points (or 0.6 of its height) of the row's FIRST item's, and `row.y` is that first item's. Small type set
// in two columns a few points out of step, or closely leaded, is therefore one row on two baselines — and the
// estimate drew every match in it at row.y. Measured on five words of the accuracy corpus (5–8pt type): the word's
// own baseline was 4.5–6pt below row.y, so the box was drawn that much too high. Its centre fell above the exact
// box, so the estimate was KEPT beside it and blacked out 4–10 glyphs of the line above; and where there is no map
// to place the word, the box stopped 2–3 points above the word's baseline and left the feet of its letters showing.
export function rowBaselines(row) {
  const ys = [];
  row.items.forEach((it, n) => {
    if (n) ys.push(NaN);
    for (let c = 0; c < it.str.length; c++) ys.push(it.y);
  });
  return ys;
}
// rowPieces turns buildTextRows' rows — the ESTIMATE's reading of a page — into wrappedMatches' pieces. A piece's
// text is its items' text with nothing between (as scanTextMatches' own per-row search has it), and `k` says where in
// the row's string each of its characters is, so a part [from, to) of a piece is row.s[k[from]] .. row.s[k[to - 1]].
export function rowPieces(rows) {
  const out = [];
  rows.slice().sort((a, b) => a.y - b.y).forEach((row, line) => {
    let piece = null, at = 0;
    row.items.forEach((it, n) => {
      const prev = row.items[n - 1];
      if (n) at++; // the space buildTextRows put between two items
      if (!piece || it.x - (prev.x + prev.w) > WRAP_GAP * Math.min(it.h, prev.h)) out.push(piece = { row: line, ref: row, x0: it.x, x1: it.x, s: '', k: [] });
      for (let c = 0; c < it.str.length; c++) { piece.s += it.str[c]; piece.k.push(at + c); }
      at += it.str.length;
      piece.x1 = it.x + it.w;
    });
  });
  return out;
}

// placeMatches puts one page's two readings together. `estimated` are the boxes the estimate drew, `exact` the map's.
// An estimated box is REPLACED only by an exact box that holds its centre — the estimate over-covers evenly about the
// match, so its centre is the match's — and is otherwise KEPT: a match the map did not find is never dropped because a
// neighbouring match was found. An exact box that replaced nothing is a match the estimate missed, and is added.
// Returns {boxes, exact, kept}: how many boxes came from the page's own glyphs, and how many are estimates.
export function placeMatches(estimated, exact) {
  const holds = (r, x, y) => x >= r[0] && x <= r[2] && y >= r[1] && y <= r[3];
  const kept = estimated.filter((e) => !exact.some((m) => holds(m.hold || m, (e[0] + e[2]) / 2, (e[1] + e[3]) / 2)));
  return { boxes: [...exact, ...kept], exact: exact.length, kept: kept.length };
}
