// closeAllInPage is `harness.mjs`'s `closeAll`, the half that runs INSIDE the page — read the
// paragraph above `closeAll` there for why it closes what the server holds rather than what is on
// screen. It is its own module, with no imports, for two reasons: `page.evaluate` serializes the
// function's source, so it may name only the page's globals (`nibFetch`, `fetch`,
// `sessionStorage`); and `tierthreecloseall_test.go` runs it under Node with those globals stubbed,
// because tier 3 is too slow to be where this function's refusals are proved.
//
// **It refuses what it cannot see, and a 401 is the only "nothing to close" (/pending 731).** It
// returned 0 on ANY non-OK `/api/docs`, so a 403 — a page whose token is not this process's —
// read as a clean server, and the /pending 474 leak this door exists to catch passed as nothing
// left open. A locked server answers 401 and holds no document, so that one IS zero. And each
// `/api/close` is status-checked: a refused close used to fall through to the recount, which
// then reported whatever it reported with the reason gone.
export async function closeAllInPage() {
  const count = async () => {
    const r = await nibFetch('/api/docs');
    if (r.status === 401) return 0; // locked: a locked Nib holds no document
    if (!r.ok) throw new Error(`closeAll: /api/docs answered ${r.status}, so what this file left open cannot be counted`);
    return ((await r.json()).docs || []).length;
  };
  if ((await count()) === 0) return 0;
  const token = sessionStorage.getItem('nib-token') || '';
  for (let i = 0; i < 5 && (await count()) > 0; i++) {
    const r = await fetch('/api/close', { method: 'POST', headers: { 'X-CSRF-Token': token } });
    if (!r.ok) throw new Error(`closeAll: /api/close answered ${r.status}, so the server still holds what this file opened`);
  }
  return count();
}
