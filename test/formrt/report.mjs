// Prints build/formrt.sh's results as the few lines a person reads. Counts and field types only.
import fs from 'node:fs';

const { tally, rows } = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const pct = (a, b) => (b ? `${(100 * a / b).toFixed(1)}%` : 'n/a');
const sum = (dir, key) => rows.reduce((n, r) => n + ((r[dir] && r[dir][key]) || 0), 0);
const docsWith = (dir, pred) => rows.filter((r) => r[dir] && pred(r[dir])).length;

console.log(`documents read: ${rows.length} · with a form both sides read: ${tally.files}`);
for (const [why, n] of Object.entries(tally.skipped)) console.log(`  set aside — ${why}: ${n}`);
for (const [dir, name] of [['w2p', 'filled in the window (pdf.js), read by pdfcpu'], ['p2w', 'filled by pdfcpu (nib fill), read in the window']]) {
  const t = tally[dir];
  console.log(`${name}:`);
  console.log(`  ${t.fields} values written · ${t.ok} read back as written (${pct(t.ok, t.fields)}) · ${t.wrong} read back as something else · ${t.missing} not found by name`);
  console.log(`  documents with any value wrong or missing: ${docsWith(dir, (r) => r.wrong || r.missing)} · documents the step failed on outright: ${docsWith(dir, (r) => r.error)}`);
  console.log(`  drawn: ${t.drawn} of ${t.drawn + t.notDrawn} values the page shows changed (${pct(t.drawn, t.drawn + t.notDrawn)})${t.tooSmall ? ` · in a field too small to draw (under 4 pt): ${t.tooSmall}` : ''}${t.drawErrors ? ` · documents pdf.js could not draw: ${t.drawErrors}` : ''}`);
  for (const [kind, k] of Object.entries(t.byKind || {})) {
    const how = Object.entries(k.how).map(([h, n]) => `${h} ${n}`).join(', ');
    console.log(`    ${kind}: ${k.asked} written · ${k.ok} as written · ${k.wrong} something else${how ? ` (${how})` : ''} · ${k.missing} not found · drawn ${k.drawn} of ${k.drawn + k.notDrawn}`);
  }
  const errs = {};
  for (const r of rows) if (r[dir] && r[dir].error) errs[r[dir].error.replace(/[0-9]+/g, 'N').slice(0, 70)] = (errs[r[dir].error.replace(/[0-9]+/g, 'N').slice(0, 70)] || 0) + 1;
  for (const [e, n] of Object.entries(errs)) console.log(`  failed outright — ${e}: ${n}`);
}
console.log(`untouched fields whose value changed or vanished when the window saved: ${tally.w2p.lost} (in ${docsWith('w2p', (r) => r.lost)} documents)`);
console.log(`fields on no page (in the form's tree, on none of the document's pages): ${rows.reduce((n, r) => n + (r.offPage || 0), 0)} in ${rows.filter((r) => r.offPage).length} documents · of those holding a value: ${rows.reduce((n, r) => n + (r.offPageWithValue || 0), 0)}`);
console.log(`fields only pdf.js lists: ${rows.reduce((n, r) => n + (r.onlyInPdfjs || 0), 0)} · only pdfcpu lists: ${rows.reduce((n, r) => n + (r.onlyInPdfcpu || 0), 0)}`);
const unwritten = {};
for (const r of rows) for (const [k, n] of Object.entries(r.notWritten || {})) unwritten[k] = (unwritten[k] || 0) + n;
console.log(`not written (either direction): ${Object.entries(unwritten).map(([k, v]) => `${k} ${v}`).join(', ') || 'none'}`);
const other = {};
for (const r of rows) for (const [k, n] of Object.entries(r.pdfcpuOther || {})) other[k] = (other[k] || 0) + n;
console.log(`not judged (pdfcpu's count): ${Object.entries(other).map(([k, v]) => `${k} ${v}`).join(', ') || 'none'}`);
