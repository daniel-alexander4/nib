// filecount.mjs — a node:test reporter that prints, per test FILE, how many tests ran and how many
// skipped, as `# nib-file <ran> <skipped> <path>` after the run. `build/tiergate.sh`'s
// `nib_population` reads it; see that file for why a total cannot answer the question.
//
// What counts: every test and subtest node:test reports, except suites (a `describe` is a container,
// not a test) and the synthetic entry node:test reports for a file itself — a file that registered
// nothing, or failed to load, is reported as one test NAMED BY ITS PATH, which a count would credit.
// `skip` and `todo` are counted as skipped: neither ran its body.
import path from 'node:path';

export default async function* filecount(source) {
  const per = new Map();
  for await (const ev of source) {
    if (ev.type !== 'test:pass' && ev.type !== 'test:fail') continue;
    const d = ev.data;
    if (!d.file) continue;
    const rel = path.relative(process.cwd(), d.file);
    if (!per.has(rel)) per.set(rel, { ran: 0, skipped: 0 });
    if (d.details?.type === 'suite' || d.name === d.file) continue;
    const c = per.get(rel);
    if (d.skip !== undefined || d.todo !== undefined) c.skipped++;
    else c.ran++;
  }
  for (const [file, c] of [...per].sort()) yield `# nib-file ${c.ran} ${c.skipped} ${file}\n`;
}
