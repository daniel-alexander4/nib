// The form round-trip instrument (/pending 271) — driven by build/formrt.sh, over a LOCAL list of real forms.
//
// A form filled in the window is written by pdf.js (`saveDocument`); a form filled any other way — `nib fill`,
// the CSV and XFDF routes — is written by pdfcpu. Each then has to be read by the other: a form filled in the
// window is exported and mail-merged by pdfcpu, and one filled by `nib fill` is opened in the window. This walks
// both directions over documents this repo did not author, because a fixture only shows the two agree about a
// form Nib made.
//
//   window → pdfcpu: every fillable text field and check box that is ON A PAGE is given a new value through pdf.js's own
//                    annotation storage, the document is saved by pdf.js, and pdfcpu's export of the saved
//                    bytes is read for each value by field name.
//   pdfcpu → window: pdfcpu's export of the ORIGINAL is given new values, `nib fill` writes them, and pdf.js
//                    reads the result's fields by name.
//
// It prints counts and field TYPES. Never a field's name or value, and never a path: the forms are whatever is
// on the machine it runs on. Fields that are on no page are counted and left alone. What it does not judge: whether a value is DRAWN (the appearance stream), radio
// groups, list and combo boxes, date fields, and signatures.
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
// write by field name ({text: {name: value}, box: {name: bool}}); with it the document is saved by pdf.js and
// the bytes come back. Without it, the fields are only read.
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
      fields[name] = { onPage: onPage.has(name), type: f.type, value: f.value, editable: f.editable !== false, hidden: !!f.hidden, multiline: !!f.multiline, maxLen: f.maxLen || 0, comb: !!f.comb, password: !!f.password, exportValues: f.exportValues, ids: list.map((x) => x.id), kids: list.length };
    }
    if (!set) return { fields };
    for (const [name, value] of Object.entries(set.text)) for (const id of fields[name].ids) doc.annotationStorage.setValue(id, { value });
    for (const [name, value] of Object.entries(set.box)) for (const id of fields[name].ids) doc.annotationStorage.setValue(id, { value });
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

// byName flattens pdfcpu's export to {name: {type, value, locked}} for the two types this judges.
function byName(exported) {
  const out = {};
  const other = {};
  for (const form of (exported && exported.forms) || []) {
    for (const [kind, list] of Object.entries(form)) {
      if (!Array.isArray(list)) continue;
      for (const f of list) {
        if (kind === 'textfield') out[f.name] = { type: 'text', value: f.value ?? '', locked: !!f.locked, multiline: !!f.multiline };
        else if (kind === 'checkbox') out[f.name] = { type: 'checkbox', value: !!f.value, locked: !!f.locked };
        else other[kind] = (other[kind] || 0) + 1;
      }
    }
  }
  return { fields: out, other };
}

async function open(file) {
  const r = await api('/api/open', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: file }) });
  if (r.status !== 200) return { refused: (r.body && (r.body.cause || r.body.error)) || `HTTP ${r.status}` };
  return { id: r.body.id, xfa: r.body.xfa || '', signed: !!(r.body.signature && r.body.signature.state && r.body.signature.state !== 'unsigned') };
}
const exported = async (id) => {
  const r = await api('/api/form-data?doc=' + encodeURIComponent(id), { headers: { 'X-Nib-Doc': id } });
  return r.status === 200 && r.body ? byName(r.body) : null;
};

const value = (i) => `nib rt ${i}`;
const fit = (v, f) => (f.maxLen ? v.slice(0, f.maxLen) : v);
const same = (a, b) => String(a ?? '').replace(/\r\n?/g, '\n') === String(b ?? '').replace(/\r\n?/g, '\n');

const rows = [];
const tally = { files: 0, skipped: {}, w2p: { fields: 0, ok: 0, missing: 0, wrong: 0, lost: 0 }, p2w: { fields: 0, ok: 0, missing: 0, wrong: 0 } };
const skip = (why) => { tally.skipped[why] = (tally.skipped[why] || 0) + 1; };

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
    tally.files++;
    row.pdfcpuOther = before.other;
    row.onlyInPdfjs = names.filter((k) => ['text', 'checkbox'].includes(win.fields[k].type) && !before.fields[k]).length;
    row.onlyInPdfcpu = Object.keys(before.fields).filter((k) => !win.fields[k]).length;

    // ── window → pdfcpu ──
    const set = { text: {}, box: {} };
    let i = 0;
    for (const k of names) {
      const f = win.fields[k];
      if (!f.onPage) { row.offPage = (row.offPage || 0) + 1; if (f.value !== undefined && f.value !== null && f.value !== '' && f.value !== 'Off') row.offPageWithValue = (row.offPageWithValue || 0) + 1; continue; }
      if (!f.editable || f.hidden || f.password) continue;
      if (f.type === 'text' && !f.comb) set.text[k] = fit(value(++i), f);
      else if (f.type === 'checkbox' && f.kids === 1) set.box[k] = !(f.value && f.value !== 'Off');
    }
    const asked = Object.keys(set.text).length + Object.keys(set.box).length;
    row.w2p = { asked, ok: 0, missing: 0, wrong: 0, wrongTypes: {} };
    if (asked) {
      const saved = await inWindow(o.id, set);
      if (saved.error || !saved.saved) row.w2p.error = saved.error || 'no bytes';
      else {
        const p = path.join(WORK, `formrt-${n}-window.pdf`);
        fs.writeFileSync(p, Buffer.from(saved.saved, 'base64'));
        await closeAll();
        const o2 = await open(p);
        const got = o2.refused ? null : await exported(o2.id);
        if (!got) row.w2p.error = 'pdfcpu could not read what pdf.js saved: ' + String(o2.refused || 'no export').slice(0, 60);
        else {
          for (const [k, v] of [...Object.entries(set.text), ...Object.entries(set.box)]) {
            const g = got.fields[k];
            if (!g) row.w2p.missing++;
            else if (typeof v === 'boolean' ? g.value === v : same(g.value, v)) row.w2p.ok++;
            else { row.w2p.wrong++; const t = win.fields[k].type + (win.fields[k].multiline ? ' (multiline)' : ''); row.w2p.wrongTypes[t] = (row.w2p.wrongTypes[t] || 0) + 1; }
          }
          // And what was NOT touched is still there: a save that drops a neighbour's value is the worse loss.
          row.w2p.lost = Object.entries(before.fields).filter(([k, b]) => !(k in set.text) && !(k in set.box) && got.fields[k] && !(typeof b.value === 'boolean' ? got.fields[k].value === b.value : same(got.fields[k].value, b.value))).length
            + Object.keys(before.fields).filter((k) => !got.fields[k]).length;
        }
        fs.rmSync(p, { force: true });
      }
    }

    // ── pdfcpu → window ──
    const data = {};
    let j = 0;
    for (const [k, f] of Object.entries(before.fields)) {
      if (f.locked) continue;
      const w = win.fields[k];
      if (f.type === 'text') data[k] = fit(value(1000 + ++j), w || {});
      else data[k] = !f.value;
    }
    row.p2w = { asked: Object.keys(data).length, ok: 0, missing: 0, wrong: 0, wrongTypes: {} };
    if (row.p2w.asked) {
      await closeAll();
      const o3 = await open(file);
      const raw = o3.refused ? null : await api('/api/form-data?doc=' + encodeURIComponent(o3.id), { headers: { 'X-Nib-Doc': o3.id } });
      if (!raw || raw.status !== 200 || !raw.body) row.p2w.error = 'no export to fill from';
      else {
        for (const form of raw.body.forms || []) {
          for (const f of form.textfield || []) if (f.name in data) f.value = data[f.name];
          for (const f of form.checkbox || []) if (f.name in data) f.value = data[f.name];
        }
        const jp = path.join(WORK, `formrt-${n}.json`), op = path.join(WORK, `formrt-${n}-pdfcpu.pdf`);
        fs.writeFileSync(jp, JSON.stringify(raw.body));
        try {
          execFileSync(NIB, ['fill', file, '--data', jp, '-o', op], { stdio: ['ignore', 'ignore', 'pipe'], timeout: 120000 });
          await closeAll();
          const o4 = await open(op);
          const back = o4.refused ? { error: 'not opened' } : await inWindow(o4.id, null);
          if (back.error) row.p2w.error = 'the window could not read what pdfcpu filled: ' + back.error;
          else {
            for (const [k, v] of Object.entries(data)) {
              const g = back.fields[k];
              if (!g) row.p2w.missing++;
              else if (typeof v === 'boolean' ? (v ? !!g.value && g.value !== 'Off' : !g.value || g.value === 'Off') : same(g.value, v)) row.p2w.ok++;
              else { row.p2w.wrong++; const t = before.fields[k].type + (before.fields[k].multiline ? ' (multiline)' : ''); row.p2w.wrongTypes[t] = (row.p2w.wrongTypes[t] || 0) + 1; }
            }
          }
        } catch (e) {
          row.p2w.error = 'nib fill refused: ' + String((e.stderr || e.message || '')).replace(/\/\S+/g, '<path>').slice(0, 100);
        }
        fs.rmSync(jp, { force: true }); fs.rmSync(op, { force: true });
      }
    }
    for (const [dir, r] of [['w2p', row.w2p], ['p2w', row.p2w]]) {
      tally[dir].fields += r.asked; tally[dir].ok += r.ok; tally[dir].missing += r.missing; tally[dir].wrong += r.wrong;
      if (dir === 'w2p') tally.w2p.lost += r.lost || 0;
      if (r.error) tally[dir].errors = (tally[dir].errors || 0) + 1;
    }
  }
  await closeAll();
  fs.writeFileSync(OUT, JSON.stringify({ tally, rows }, null, 1));
  console.log('FORMRT ' + JSON.stringify(tally));
});
