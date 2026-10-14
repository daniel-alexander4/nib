// The form round-trip instrument (/pending 271) — driven by build/formrt.sh, over a LOCAL list of real forms.
//
// A form filled in the window is written by pdf.js (`saveDocument`); a form filled any other way — `nib fill`,
// the CSV and XFDF routes — is written by pdfcpu. Each then has to be read by the other: a form filled in the
// window is exported and mail-merged by pdfcpu, and one filled by `nib fill` is opened in the window. This walks
// both directions over documents this repo did not author, because a fixture only shows the two agree about a
// form Nib made.
//
//   window → pdfcpu: every fillable field that is ON A PAGE — text, date, check box, radio group, combo and list
//                    box — is given a new value through pdf.js's own annotation storage, the document is saved
//                    by pdf.js, and pdfcpu's export of the saved bytes is read for each value by field name.
//   pdfcpu → window: pdfcpu's export of the ORIGINAL is given new values, `nib fill` writes them, and pdf.js
//                    reads the result's fields by name.
//
// And in both directions, whether each value written is DRAWN: every page holding a written field is rendered by
// pdf.js from the document's appearance streams (`AnnotationMode.ENABLE`, the print path — not the HTML form layer
// the window types into), before and after, and a field counts as drawn when some widget's rectangle changed. It
// says a reader shows SOMETHING new there, not that it shows the right glyphs; and pdf.js builds an appearance
// itself where a document asks readers to (`/NeedAppearances`), as Acrobat does and some readers do not.
//
// It prints counts and field TYPES. Never a field's name or value, and never a path: the forms are whatever is
// on the machine it runs on. Fields that are on no page are counted and left alone. What it does not judge:
// signatures, push buttons, a check box with several widgets, a comb field, and a date in a format with a month
// name or a time in it (each counted as not written, by reason).
import test, { after } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { launch, shutdown, WORK } from '../ui/harness.mjs';

const LIST = process.env.NIB_FORMRT_LIST;
const NIB = process.env.NIB_FORMRT_BIN;
const OUT = process.env.NIB_FORMRT_RESULTS;
const MAX_BYTES = 20 << 20;
const files = fs.readFileSync(LIST, 'utf8').split('\n').map((l) => l.trim()).filter((l) => l && !l.startsWith('#'));

const h = await launch();
const page = h.page;
after(async () => { await shutdown(h); });

const api = (p, opt) => page.evaluate(async ({ p, opt }) => {
  const r = await nibFetch(p, opt);
  const text = await r.text();
  let body = null;
  try { body = JSON.parse(text); } catch { /* not JSON */ }
  return { status: r.status, body, text: body ? '' : text.slice(0, 200) };
}, { p, opt });
const closeAll = async () => { for (let i = 0; i < 4; i++) await api('/api/close', { method: 'POST' }); };

// inWindow runs pdf.js — the build the window itself loads — over the open document. `set` is the values to
// write by field name ({name: {storage: {widgetId: value}}}); with it the document is saved by pdf.js and the
// bytes come back. Without it, the fields are only read.
const inWindow = (docId, set) => page.evaluate(async ({ docId, set }) => {
  const lib = await import('/vendor/pdfjs/pdf.min.mjs');
  const res = await nibFetch('/api/pdf?doc=' + encodeURIComponent(docId));
  if (!res.ok) return { error: 'pdf ' + res.status };
  const data = new Uint8Array(await res.arrayBuffer());
  let doc;
  const task = lib.getDocument({ data, cMapUrl: '/vendor/pdfjs/cmaps/', cMapPacked: true });
  try { doc = await task.promise; } catch (e) { return { error: 'pdf.js could not open it: ' + (e && e.name) }; }
  try {
    const objs = (await doc.getFieldObjects()) || {};
    // A field ON A PAGE: one some page's widget names. `getFieldObjects` lists the whole form tree, and a file
    // cut from a larger form package keeps the other pages' fields in it — nobody can type in those, pdfcpu
    // rightly does not list them, and writing to them is how this instrument's first run reported 5,177 values
    // "not found" that no user could have entered (/pending 888).
    const onPage = new Set();
    for (let p = 1; p <= doc.numPages; p++) {
      const pg = await doc.getPage(p);
      for (const a of await pg.getAnnotations({ intent: 'display' })) if (a.subtype === 'Widget' && a.fieldName) onPage.add(a.fieldName);
    }
    const fields = {};
    for (const [name, list] of Object.entries(objs)) {
      const f = list.find((x) => x.type) || list[0];
      if (!f) continue;
      const widgets = list.filter((x) => x.type).map((x) => ({ id: x.id, state: x.exportValues, value: x.value, page: x.page, rect: x.rect, hidden: !!x.hidden }));
      fields[name] = { onPage: onPage.has(name), type: f.type, value: f.value, editable: f.editable !== false, hidden: !!f.hidden, multiline: !!f.multiline, maxLen: f.maxLen || 0, comb: !!f.comb, password: !!f.password, exportValues: f.exportValues, ids: list.map((x) => x.id), kids: list.length, widgets, items: f.items || null, datetimeFormat: f.datetimeFormat || '' };
    }
    if (!set) return { fields };
    for (const e of Object.values(set)) for (const [id, value] of Object.entries(e.storage)) doc.annotationStorage.setValue(id, { value });
    const bytes = await doc.saveDocument();
    let bin = '';
    for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode.apply(null, bytes.subarray(i, i + 0x8000));
    return { fields, saved: btoa(bin) };
  } catch (e) {
    return { error: 'pdf.js: ' + (e && (e.message || e.name)) };
  } finally {
    await task.destroy();
  }
}, { docId, set });

// byName flattens pdfcpu's export to {name: {type, value, locked, …}}. A list box's value is its array of
// selections; a radio group's is the option pdfcpu names (its state name, or the `/Opt` entry that name indexes).
const KINDS = { textfield: 'text', datefield: 'date', checkbox: 'checkbox', radiobuttongroup: 'radio', combobox: 'combo', listbox: 'list' };
function byName(exported) {
  const out = {};
  const other = {};
  for (const form of (exported && exported.forms) || []) {
    for (const [kind, list] of Object.entries(form)) {
      if (!Array.isArray(list)) continue;
      for (const f of list) {
        const type = KINDS[kind];
        if (!type) { other[kind] = (other[kind] || 0) + 1; continue; }
        const value = type === 'checkbox' ? !!f.value : type === 'list' ? (f.values || []) : (f.value ?? '');
        out[f.name] = { type, value, locked: !!f.locked, multiline: !!f.multiline, format: f.format || '', options: f.options || [] };
      }
    }
  }
  return { fields: out, other };
}

// drawnIn renders, with pdf.js, every page on which `checks` names a widget — once from the document `before` and
// once from `after`, both from their appearance streams — and answers, per field, whether any of its widgets'
// rectangles changed. A pixel has changed when a channel moved by more than 48; a widget, when at least 6 pixels
// or 0.3% of it did (a check mark covers far more; one glyph of a typed value at this scale, about that).
const drawnIn = (before, after, checks) => page.evaluate(async ({ before, after, checks }) => {
  const lib = await import('/vendor/pdfjs/pdf.min.mjs');
  const load = async (id) => {
    const r = await nibFetch('/api/pdf?doc=' + encodeURIComponent(id));
    return lib.getDocument({ data: new Uint8Array(await r.arrayBuffer()), cMapUrl: '/vendor/pdfjs/cmaps/', cMapPacked: true });
  };
  const ta = await load(before), tb = await load(after);
  try {
    const a = await ta.promise, b = await tb.promise;
    const paint = async (doc, p) => {
      const pg = await doc.getPage(p + 1);
      const vp = pg.getViewport({ scale: 2 });
      const c = new OffscreenCanvas(Math.ceil(vp.width), Math.ceil(vp.height));
      const ctx = c.getContext('2d', { willReadFrequently: true });
      await pg.render({ canvasContext: ctx, viewport: vp, annotationMode: lib.AnnotationMode.ENABLE }).promise;
      return { vp, w: c.width, h: c.height, px: ctx.getImageData(0, 0, c.width, c.height).data };
    };
    const out = {};
    const pages = [...new Set(checks.flatMap((c) => c.rects.map((r) => r.page)))];
    for (const p of pages) {
      if (p < 0 || p >= a.numPages || p >= b.numPages) continue;
      const x = await paint(a, p), y = await paint(b, p);
      if (x.w !== y.w || x.h !== y.h) continue;
      for (const c of checks) {
        for (const r of c.rects) {
          if (r.page !== p || out[c.name]) continue;
          // The viewport's own transform (pdf.js 6 has no rectangle helper): user space → canvas pixels.
          const [ta0, tb0, tc0, td0, te0, tf0] = x.vp.transform;
          const at = (u, v) => [ta0 * u + tc0 * v + te0, tb0 * u + td0 * v + tf0];
          const [[x1, y1], [x2, y2]] = [at(r.rect[0], r.rect[1]), at(r.rect[2], r.rect[3])];
          const l = Math.max(0, Math.floor(Math.min(x1, x2))), t = Math.max(0, Math.floor(Math.min(y1, y2)));
          const rr = Math.min(x.w, Math.ceil(Math.max(x1, x2))), bb = Math.min(x.h, Math.ceil(Math.max(y1, y2)));
          let moved = 0;
          for (let j = t; j < bb; j++) for (let i = l; i < rr; i++) {
            const o = (j * x.w + i) * 4;
            if (Math.abs(x.px[o] - y.px[o]) > 48 || Math.abs(x.px[o + 1] - y.px[o + 1]) > 48 || Math.abs(x.px[o + 2] - y.px[o + 2]) > 48) moved++;
          }
          if (moved >= Math.max(6, 0.003 * (rr - l) * (bb - t))) out[c.name] = true;
        }
      }
    }
    return { drawn: out };
  } catch (e) {
    return { error: 'pdf.js could not draw it: ' + (e && (e.message || e.name)) };
  } finally {
    await ta.destroy(); await tb.destroy();
  }
}, { before, after, checks });

async function open(file) {
  const r = await api('/api/open', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: file }) });
  if (r.status !== 200) return { refused: (r.body && (r.body.cause || r.body.error)) || `HTTP ${r.status}` };
  return { id: r.body.id, xfa: r.body.xfa || '', signed: !!(r.body.signature && r.body.signature.state && r.body.signature.state !== 'unsigned') };
}
// exported is pdfcpu's export of the open document, flattened; `none` where Nib says the document has no form
// fields to export (a 422 with that cause — a signature only, or nothing on a page); null where the export failed.
const exported = async (id) => {
  const r = await api('/api/form-data?doc=' + encodeURIComponent(id), { headers: { 'X-Nib-Doc': id } });
  if (r.status === 422 && r.body && r.body.cause === 'no-form-fields') return { none: true, fields: {}, other: {} };
  return r.status === 200 && r.body ? byName(r.body) : null;
};

const value = (i) => `nib rt ${i}`;
const fit = (v, f) => (f.maxLen ? v.slice(0, f.maxLen) : v);
const same = (a, b) => String(a ?? '').replace(/\r\n?/g, '\n') === String(b ?? '').replace(/\r\n?/g, '\n');
const sameValue = (a, b) => (Array.isArray(a) || Array.isArray(b)
  ? JSON.stringify([].concat(a ?? []).map(String)) === JSON.stringify([].concat(b ?? []).map(String))
  : typeof a === 'boolean' || typeof b === 'boolean' ? !!a === !!b : same(a, b));

// dateIn writes 23 July 2026 in a field's own format — the tokens pdfcpu's date formats are made of (yyyy, yy, mm,
// m, dd, d) and any separator. A format naming the month or the weekday, or holding a time, is not written: null.
const dateIn = (fmt) => {
  if (!fmt) return null;
  const tokens = [['yyyy', '2026'], ['yy', '26'], ['mm', '07'], ['m', '7'], ['dd', '23'], ['d', '23']];
  let out = '';
  for (let i = 0; i < fmt.length;) {
    if (/^(mmm|ddd)/.test(fmt.slice(i))) return null;
    const t = tokens.find(([k]) => fmt.startsWith(k, i));
    if (t) { out += t[1]; i += t[0].length; continue; }
    if (/[A-Za-z]/.test(fmt[i])) return null;
    out += fmt[i++];
  }
  return out;
};
// The window's own name for a pdf.js field type, so both directions tally under one set of kinds.
const PDFJS_KIND = { text: 'text', checkbox: 'checkbox', radiobutton: 'radio', combobox: 'combo', listbox: 'list' };

const rows = [];
const blank = () => ({ fields: 0, ok: 0, missing: 0, wrong: 0, drawn: 0, notDrawn: 0, byKind: {} });
const tally = { files: 0, skipped: {}, w2p: { ...blank(), lost: 0 }, p2w: blank() };
const skip = (why) => { tally.skipped[why] = (tally.skipped[why] || 0) + 1; };
const kindRow = (r, kind) => (r.byKind[kind] ||= { asked: 0, ok: 0, missing: 0, wrong: 0, drawn: 0, notDrawn: 0, how: {} });
const notWritten = (row, why) => { row.notWritten ||= {}; row.notWritten[why] = (row.notWritten[why] || 0) + 1; };

// judge records one written value against what the other side read back: `how` names a wrong answer's shape.
const judge = (r, kind, ok, how) => {
  const k = kindRow(r, kind);
  k.asked++; r.asked++;
  if (ok === null) { k.missing++; r.missing++; return; }
  if (ok) { k.ok++; r.ok++; return; }
  k.wrong++; r.wrong++; k.how[how] = (k.how[how] || 0) + 1;
};
// checksFor names the widget rectangles of the fields written, from the ORIGINAL's field objects.
const checksFor = (win, names) => names.map((name) => ({
  name, rects: win.fields[name].widgets.filter((w) => w.rect && Number.isInteger(w.page) && !w.hidden).map((w) => ({ page: w.page, rect: w.rect })),
}));
const tallyDrawn = (r, kinds, drawn) => {
  for (const [name, kind] of Object.entries(kinds)) {
    const k = kindRow(r, kind);
    if (drawn[name]) { k.drawn++; r.drawn++; } else { k.notDrawn++; r.notDrawn++; }
  }
};

test(`form round-trip over ${files.length} local documents`, { timeout: 4 * 3600 * 1000 }, async () => {
  let n = 0;
  for (const file of files) {
    n++;
    const row = { n };
    rows.push(row);
    let size = 0;
    try { size = fs.statSync(file).size; } catch { row.skip = 'not readable'; skip(row.skip); continue; }
    if (size > MAX_BYTES) { row.skip = 'over 20 MB'; skip(row.skip); continue; }
    await closeAll();
    const o = await open(file);
    if (o.refused) { row.skip = 'not opened: ' + String(o.refused).slice(0, 60); skip('not opened'); continue; }
    if (o.xfa === 'dynamic') { row.skip = 'dynamic XFA (Nib refuses to change it)'; skip(row.skip); continue; }
    const before = await exported(o.id);
    const win = await inWindow(o.id, null);
    if (win.error) { row.skip = win.error; skip('pdf.js could not read it'); continue; }
    const names = Object.keys(win.fields);
    row.pdfjsFields = names.length;
    row.pdfcpuFields = before ? Object.keys(before.fields).length : null;
    row.xfa = o.xfa; row.signed = o.signed;
    if (!names.length && !(before && Object.keys(before.fields).length)) { row.skip = 'no fields'; skip(row.skip); continue; }
    if (!before) { row.skip = 'pdfcpu could not export the form'; skip(row.skip); continue; }
    if (before.none) { row.skip = 'no form fields to export (a signature only, or none on a page)'; skip(row.skip); continue; }
    tally.files++;
    row.pdfcpuOther = before.other;
    row.onlyInPdfjs = names.filter((k) => win.fields[k].onPage && PDFJS_KIND[win.fields[k].type] && !before.fields[k]).length;
    row.onlyInPdfcpu = Object.keys(before.fields).filter((k) => !win.fields[k]).length;

    // ── window → pdfcpu ──
    // A field's kind is pdfcpu's where pdfcpu lists it (only pdfcpu tells a date from text), else the window's.
    const set = {};
    const kindOf = {};
    let i = 0;
    for (const k of names) {
      const f = win.fields[k];
      if (!f.onPage) { row.offPage = (row.offPage || 0) + 1; if (f.value !== undefined && f.value !== null && f.value !== '' && f.value !== 'Off') row.offPageWithValue = (row.offPageWithValue || 0) + 1; continue; }
      if (!f.editable || f.hidden || f.password || !PDFJS_KIND[f.type]) continue;
      const kind = (before.fields[k] && before.fields[k].type) || PDFJS_KIND[f.type];
      const all = (v) => Object.fromEntries(f.ids.map((id) => [id, v]));
      if (kind === 'text') {
        if (f.comb) { notWritten(row, 'text: comb'); continue; }
        const v = fit(value(++i), f);
        set[k] = { storage: all(v), expect: v };
      } else if (kind === 'date') {
        const v = dateIn(before.fields[k].format || f.datetimeFormat);
        if (v === null) { notWritten(row, 'date: a format this does not write'); continue; }
        set[k] = { storage: all(v), expect: v };
      } else if (kind === 'checkbox') {
        if (f.kids !== 1) { notWritten(row, 'checkbox: several widgets'); continue; }
        const v = !(f.value && f.value !== 'Off');
        set[k] = { storage: all(v), expect: v };
      } else if (kind === 'radio') {
        const to = f.widgets.find((w) => w.state && w.state !== 'Off' && w.state !== f.value);
        if (!to) { notWritten(row, 'radio: no other button'); continue; }
        set[k] = { storage: Object.fromEntries(f.widgets.map((w) => [w.id, w.state === to.state])), expect: to.state };
      } else {
        const to = (f.items || []).find((it) => it.exportValue !== f.value);
        if (!to) { notWritten(row, `${kind}: no other option`); continue; }
        set[k] = { storage: all(to.exportValue), expect: to.exportValue, display: to.displayValue };
      }
      kindOf[k] = kind;
    }
    const asked = Object.keys(set).length;
    row.w2p = { ...blank(), asked: 0 };
    if (asked) {
      const saved = await inWindow(o.id, set);
      if (saved.error || !saved.saved) row.w2p.error = saved.error || 'no bytes';
      else {
        const p = path.join(WORK, `formrt-${n}-window.pdf`);
        fs.writeFileSync(p, Buffer.from(saved.saved, 'base64'));
        const o2 = await open(p);
        const got = o2.refused ? null : await exported(o2.id);
        if (!got) row.w2p.error = 'pdfcpu could not read what pdf.js saved: ' + String(o2.refused || 'no export').slice(0, 60);
        else {
          for (const [k, s] of Object.entries(set)) {
            const kind = kindOf[k];
            const g = got.fields[k];
            if (!g) { judge(row.w2p, kind, null); continue; }
            if (kind === 'radio') {
              // pdfcpu names a group whose kids carry `/Opt` by the option the state name indexes.
              judge(row.w2p, kind, g.value === s.expect || (/^\d+$/.test(s.expect) && g.options[Number(s.expect)] === g.value), g.value === '' ? 'no option' : 'another option');
            } else if (kind === 'combo' || kind === 'list') {
              const v = kind === 'list' ? g.value[0] : g.value;
              judge(row.w2p, kind, v === s.expect && (kind !== 'list' || g.value.length === 1), v === s.display ? 'its display text' : 'another value');
            } else {
              judge(row.w2p, kind, sameValue(g.value, s.expect), kind + (win.fields[k].multiline ? ' (multiline)' : ''));
            }
          }
          // And what was NOT touched is still there: a save that drops a neighbour's value is the worse loss.
          row.w2p.lost = Object.entries(before.fields).filter(([k, b]) => !(k in set) && got.fields[k] && !sameValue(got.fields[k].value, b.value)).length
            + Object.keys(before.fields).filter((k) => !got.fields[k]).length;
          const d = await drawnIn(o.id, o2.id, checksFor(win, Object.keys(set)));
          if (d.error) row.w2p.drawError = d.error; else tallyDrawn(row.w2p, kindOf, d.drawn);
        }
        fs.rmSync(p, { force: true });
      }
    }

    // ── pdfcpu → window ──
    const data = {};
    const kindTo = {};
    let j = 0;
    for (const [k, f] of Object.entries(before.fields)) {
      if (f.locked) continue;
      const w = win.fields[k];
      if (w && !w.onPage) continue;
      if (f.type === 'text') data[k] = fit(value(1000 + ++j), w || {});
      else if (f.type === 'date') {
        const v = dateIn(f.format);
        if (v === null) { notWritten(row, 'date: a format this does not write'); continue; }
        data[k] = v;
      } else if (f.type === 'checkbox') data[k] = !f.value;
      else {
        const have = f.type === 'list' ? f.value : [f.value];
        const to = f.options.find((x) => !have.includes(x));
        if (to === undefined) { notWritten(row, `${f.type}: no other option`); continue; }
        data[k] = f.type === 'list' ? [to] : to;
      }
      kindTo[k] = f.type;
    }
    row.p2w = { ...blank(), asked: 0 };
    if (Object.keys(data).length) {
      await closeAll();
      const o3 = await open(file);
      const raw = o3.refused ? null : await api('/api/form-data?doc=' + encodeURIComponent(o3.id), { headers: { 'X-Nib-Doc': o3.id } });
      if (!raw || raw.status !== 200 || !raw.body) row.p2w.error = 'no export to fill from';
      else {
        for (const form of raw.body.forms || []) {
          for (const [kind, list] of Object.entries(form)) {
            if (!KINDS[kind] || !Array.isArray(list)) continue;
            for (const f of list) {
              if (!(f.name in data)) continue;
              if (kind === 'listbox') f.values = data[f.name]; else f.value = data[f.name];
            }
          }
        }
        const jp = path.join(WORK, `formrt-${n}.json`), op = path.join(WORK, `formrt-${n}-pdfcpu.pdf`);
        fs.writeFileSync(jp, JSON.stringify(raw.body));
        try {
          execFileSync(NIB, ['fill', file, '--data', jp, '-o', op], { stdio: ['ignore', 'ignore', 'pipe'], timeout: 120000 });
          const o4 = await open(op);
          const back = o4.refused ? { error: 'not opened' } : await inWindow(o4.id, null);
          if (back.error) row.p2w.error = 'the window could not read what pdfcpu filled: ' + back.error;
          else {
            for (const [k, v] of Object.entries(data)) {
              const kind = kindTo[k];
              const g = back.fields[k];
              if (!g) { judge(row.p2w, kind, null); continue; }
              if (kind === 'checkbox') judge(row.p2w, kind, v ? !!g.value && g.value !== 'Off' : !g.value || g.value === 'Off', kind);
              else if (kind === 'radio') {
                // The window's value is the group's state name; pdfcpu, given an `/Opt` text, writes the index. A
                // check box whose widgets carry different on-states (pdfcpu reads it as a group) is answered per
                // widget by pdf.js — each from its own `/AS` — so it is judged by which widget is on.
                const state = [v, String(before.fields[k].options.indexOf(v))];
                const perWidget = g.widgets.length > 1 && g.widgets.every((w) => (state.includes(w.state) ? w.value === w.state : w.value !== w.state));
                judge(row.p2w, kind, g.value === v || g.value === state[1] || perWidget, 'another option');
              } else if (kind === 'list') judge(row.p2w, kind, g.value === v[0], 'another value');
              else judge(row.p2w, kind, same(g.value, v), kind + (before.fields[k].multiline ? ' (multiline)' : ''));
            }
            if (win.fields && Object.keys(data).some((k) => win.fields[k])) {
              const drawable = Object.keys(data).filter((k) => win.fields[k]);
              const d = await drawnIn(o3.id, o4.id, checksFor(win, drawable));
              if (d.error) row.p2w.drawError = d.error;
              else tallyDrawn(row.p2w, Object.fromEntries(drawable.map((k) => [k, kindTo[k]])), d.drawn);
            }
          }
        } catch (e) {
          // pdfcpu's own words only: the rest of the line is the file's name, which is not this report's to print.
          const said = String(e.stderr || e.message || '');
          const at = said.lastIndexOf('pdfcpu:');
          row.p2w.error = 'nib fill refused: ' + (at >= 0 ? said.slice(at).replace(/["'(].*$/s, '').trim().slice(0, 80) : 'no reason pdfcpu gave');
        }
        fs.rmSync(jp, { force: true }); fs.rmSync(op, { force: true });
      }
    }
    for (const dir of ['w2p', 'p2w']) {
      const r = row[dir], t = tally[dir];
      t.fields += r.asked; t.ok += r.ok; t.missing += r.missing; t.wrong += r.wrong; t.drawn += r.drawn; t.notDrawn += r.notDrawn;
      for (const [kind, k] of Object.entries(r.byKind)) {
        const tk = kindRow(t, kind);
        for (const key of ['asked', 'ok', 'missing', 'wrong', 'drawn', 'notDrawn']) tk[key] += k[key];
        for (const [h, c] of Object.entries(k.how)) tk.how[h] = (tk.how[h] || 0) + c;
      }
      if (dir === 'w2p') t.lost += r.lost || 0;
      if (r.error) t.errors = (t.errors || 0) + 1;
      if (r.drawError) t.drawErrors = (t.drawErrors || 0) + 1;
    }
  }
  await closeAll();
  fs.writeFileSync(OUT, JSON.stringify({ tally, rows }, null, 1));
  console.log('FORMRT ' + JSON.stringify(tally));
});
