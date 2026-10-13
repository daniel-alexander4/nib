// Prints build/formrt.sh's results as the few lines a person reads. Counts and field types only.
import fs from 'node:fs';

const { tally, rows } = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const pct = (a, b) => (b ? `${(100 * a / b).toFixed(1)}%` : 'n/a');
const sum = (dir, key) => rows.reduce((n, r) => n + ((r[dir] && r[dir][key]) || 0), 0);
const types = (dir) => {
  const out = {};
  for (const r of rows) for (const [t, n] of Object.entries((r[dir] && r[dir].wrongTypes) || {})) out[t] = (out[t] || 0) + n;
  return out;
};
const docsWith = (dir, pred) => rows.filter((r) => r[dir] && pred(r[dir])).length;

console.log(`documents read: ${rows.length} · with a form both sides read: ${tally.files}`);
for (const [why, n] of Object.entries(tally.skipped)) console.log(`  set aside — ${why}: ${n}`);
for (const [dir, name] of [['w2p', 'filled in the window (pdf.js), read by pdfcpu'], ['p2w', 'filled by pdfcpu (nib fill), read in the window']]) {
  const t = tally[dir];
  console.log(`${name}:`);
  console.log(`  ${t.fields} values written · ${t.ok} read back as written (${pct(t.ok, t.fields)}) · ${t.wrong} read back as something else · ${t.missing} not found by name`);
  console.log(`  documents with any value wrong or missing: ${docsWith(dir, (r) => r.wrong || r.missing)} · documents the step failed on outright: ${docsWith(dir, (r) => r.error)}`);
  const wt = types(dir);
  if (Object.keys(wt).length) console.log(`  wrong, by field type: ${Object.entries(wt).map(([k, v]) => `${k} ${v}`).join(', ')}`);
  const errs = {};
  for (const r of rows) if (r[dir] && r[dir].error) errs[r[dir].error.replace(/[0-9]+/g, 'N').slice(0, 70)] = (errs[r[dir].error.replace(/[0-9]+/g, 'N').slice(0, 70)] || 0) + 1;
  for (const [e, n] of Object.entries(errs)) console.log(`  failed outright — ${e}: ${n}`);
}
console.log(`untouched fields whose value changed or vanished when the window saved: ${tally.w2p.lost} (in ${docsWith('w2p', (r) => r.lost)} documents)`);
console.log(`fields on no page (in the form's tree, on none of the document's pages): ${rows.reduce((n, r) => n + (r.offPage || 0), 0)} in ${rows.filter((r) => r.offPage).length} documents · of those holding a value: ${rows.reduce((n, r) => n + (r.offPageWithValue || 0), 0)}`);
console.log(`fields only pdf.js lists: ${rows.reduce((n, r) => n + (r.onlyInPdfjs || 0), 0)} · only pdfcpu lists: ${rows.reduce((n, r) => n + (r.onlyInPdfcpu || 0), 0)}`);
const other = {};
for (const r of rows) for (const [k, n] of Object.entries(r.pdfcpuOther || {})) other[k] = (other[k] || 0) + n;
console.log(`not judged (pdfcpu's count): ${Object.entries(other).map(([k, v]) => `${k} ${v}`).join(', ') || 'none'}`);
