// The role map in the tree panel — ADR-127.
//
// ── What only this tier can see ──────────────────────────────────────────────
// The Go tests prove a `rolemap` edit sets, replaces and removes one mapping and refuses what it must. This
// proves what a PERSON is shown and what their press sends: that the section is a closed native details
// whose summary counts the mappings, that each mapping is a row with a picker preset to its target, that
// Change, Remove and Add each send exactly one edit — and only on a button, never on the picker's change —
// that Remove is disabled with its reason while tags use the name, that after the re-read focus is back on
// the row that was used, and that a refusal keeps what was typed and shows the server's sentence.
//
// What it cannot see: a real browser's Tab order through the rows and a details opened by the keyboard —
// tier 3's tagrolemap.test.mjs.
//
// One boot per file — see boot.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { boot, REPO } from './boot.mjs';
import { setNextDocument } from './stub-pdfjs.mjs';

const OPEN = {
  id: 'roles:1', name: 'doc.pdf', path: '/tmp/nib-harness/doc.pdf',
  canSave: true, canUndo: false, canRedo: false, signature: { state: 'unsigned' },
};
const BOX = [0, 0, 612, 792];
const el = (id, parent, kind, standard, text, kids) => ({
  id, parent, kind, standard, page: 1, text, alt: '', hasAlt: false, scope: '', kids,
  colSpan: 1, rowSpan: 1, headers: [], rect: [72, 600, 300, 720], pageBox: BOX,
});
const baseTree = () => ({
  tagged: true, unaddressable: 0,
  elements: [
    el(4, -1, 'Document', 'Document', 'TitleBody', [1, 2]),
    el(5, 0, 'Heading 1', 'H1', 'Title', []),
    el(6, 0, 'Body Text', 'P', 'Body', []),
  ],
  roleMap: [
    { name: 'Body Text', to: 'P', standard: 'P', elements: 1 },
    { name: 'Chained', to: 'Heading 1', standard: 'H1', elements: 0 },
    { name: 'Heading 1', to: 'H1', standard: 'H1', elements: 2 },
    { name: 'Unused', to: 'Note', standard: 'Note', elements: 0 },
  ],
});
let tree = baseTree();
const edits = [];
let editReply = null;

const { document: doc, window: win, settle } = await boot({
  routes: {
    '/api/docs': () => ({ docs: [OPEN], activeId: OPEN.id }),
    '/api/open': OPEN,
    '/api/scan': { hidden: [] },
    '/api/tags/tree': () => tree,
    '/api/tags/edit': (opts) => {
      edits.push(JSON.parse(opts.body));
      return editReply ? editReply() : { ...OPEN, canUndo: true };
    },
  },
});

const $ = (id) => doc.getElementById(id);
const rows = () => [...$('tagRoleList').children];
const row = (name) => rows().find((li) => li.dataset.role === name);
const items = () => [...doc.querySelectorAll('#tagTreeList [role="treeitem"]')];

// reopenPanel closes and opens the panel: a read of the tree nobody's edit caused.
async function reopenPanel() {
  const head = doc.querySelector('.tab[data-panel="tagtree"]');
  if (head.classList.contains('active')) head.click();
  head.click();
  await settle();
  await settle();
}

async function act(control) {
  setNextDocument({ numPages: 2 });
  control.focus();
  control.click();
  await settle();
  await settle();
  await settle();
}

test('the section is a closed details whose summary counts the mappings, with a row for each, sorted as the server sends them', async () => {
  setNextDocument({ numPages: 2 });
  $('pathInput').value = OPEN.path;
  $('openGo').click();
  await settle();
  doc.querySelector('.modetab[data-tab="accessibility"]').click(); // ADR-035
  await settle();
  doc.querySelector('.tab[data-panel="tagtree"]').click();
  await settle();
  await settle();
  const box = $('tagRoleMap');
  assert.equal(box.tagName, 'DETAILS', 'the role map is not a native details');
  assert.equal(box.hidden, false, 'a tagged document is not offered its role map');
  assert.equal(box.open, false, 'the role map is open before anyone asked for it');
  assert.equal($('tagRoleSummary').tagName, 'SUMMARY', 'the line that opens the section is not its summary');
  assert.equal($('tagRoleSummary').textContent, 'Role map — 4 custom types mapped', 'the summary does not say how many custom types the document maps');
  assert.deepEqual(rows().map((li) => li.dataset.role), ['Body Text', 'Chained', 'Heading 1', 'Unused'], 'there is not one row per mapping, in the order sent');

  const heading = row('Heading 1');
  assert.equal(heading.querySelector('.tagrolename').textContent, 'Heading 1', 'the row does not show the custom name');
  assert.equal(heading.querySelector('select').value, 'H1', 'the picker is not preset to what the name is mapped to');
  assert.equal(heading.querySelector('select').getAttribute('aria-label'), 'Heading 1 means', 'the picker has no name of its own for a screen reader');
  assert.equal(heading.querySelector('.tagRoleRemove').disabled, true, 'a mapping two tags use can be removed');
  const why = doc.getElementById(heading.querySelector('.tagRoleRemove').getAttribute('aria-describedby'));
  assert.match(why.textContent, /2 tags are of this type.*cannot be removed.*change their type first/, 'the row does not say why its mapping cannot be removed');
  assert.match(row('Body Text').querySelector('p').textContent, /^1 tag is of this type/, 'one tag is not counted in the singular');
  assert.equal(row('Unused').querySelector('.tagRoleRemove').disabled, false, 'a mapping no tag uses cannot be removed');
  assert.match(row('Unused').querySelector('p').textContent, /No tag is of this type/, 'an unused mapping does not say so');
  // A name mapped onto another custom name: the picker shows the target as written, never a standard type
  // the document does not name, and the row says what it is read as.
  assert.equal(row('Chained').querySelector('select').value, 'Heading 1', 'a chained mapping\'s picker claims a standard target');
  assert.match(row('Chained').querySelector('p').textContent, /read as H1/, 'a chained mapping does not say what it is read as');
  for (const li of rows()) {
    for (const b of li.querySelectorAll('button')) {
      assert.ok(b.getAttribute('aria-label').startsWith(b.textContent), `the button "${b.textContent}" has an accessible name that does not begin with the word it shows`);
      assert.ok(b.getAttribute('aria-label').includes(li.dataset.role), `the button "${b.textContent}" does not name the type it acts on`);
    }
  }
});

test('the pickers offer exactly the types the server accepts, and choosing one sends nothing', async () => {
  const src = fs.readFileSync(path.join(REPO, 'internal/pdfops/structedit.go'), 'utf8');
  const block = src.slice(src.indexOf('var standardStructTypes = map[string]bool{'), src.indexOf('// rootParent'));
  const server = [...block.matchAll(/"([A-Za-z0-9]+)": true/g)].map((m) => m[1]).sort();
  assert.ok(server.length > 40, `read only ${server.length} standard types from structedit.go — the parse has drifted`);
  assert.deepEqual([...$('tagRoleNewType').options].map((o) => o.value).sort(), server, 'the new mapping\'s picker does not offer exactly the standard types');
  assert.deepEqual([...row('Unused').querySelector('select').options].map((o) => o.value).sort(), server, 'a row\'s picker does not offer exactly the standard types');

  edits.length = 0;
  const select = row('Unused').querySelector('select');
  select.value = 'Div';
  select.dispatchEvent(new win.Event('change', { bubbles: true }));
  select.dispatchEvent(new win.Event('input', { bubbles: true }));
  $('tagRoleNewType').value = 'Sect';
  $('tagRoleNewType').dispatchEvent(new win.Event('change', { bubbles: true }));
  await settle();
  assert.deepEqual(edits, [], 'choosing in a picker sent an edit — only a button applies');
});

test('Change sends the one rolemap edit, the selected tag stays selected, and focus returns to the row\'s button', async () => {
  items()[1].focus();
  await settle();
  edits.length = 0;
  const select = row('Heading 1').querySelector('select');
  select.value = 'H2';
  const after = baseTree();
  after.elements[1].standard = 'H2';
  after.roleMap[1].standard = 'H2';
  after.roleMap[2] = { name: 'Heading 1', to: 'H2', standard: 'H2', elements: 2 };
  tree = after;
  await act(row('Heading 1').querySelector('.tagRoleChange'));
  assert.deepEqual(edits, [{ edits: [{ kind: 'rolemap', role: 'Heading 1', value: 'H2' }] }], 'Change did not send exactly one rolemap edit naming the type and its new meaning');
  assert.equal(row('Heading 1').querySelector('select').value, 'H2', 'the re-read row does not show the new mapping');
  assert.equal(doc.activeElement, row('Heading 1').querySelector('.tagRoleChange'), 'focus did not return to the row\'s Change button');
  assert.match(items()[1].textContent, /^H2 \(Heading 1\)/, 'the tag\'s line in the tree does not read its new resolved type');
  assert.equal(items()[1].getAttribute('aria-selected'), 'true', 'the tag that was selected is not selected after the re-read');
  assert.match($('tagRoleStatus').textContent, /Heading 1 now means H2 — Ctrl\+Z takes it back/, 'the status does not say what changed and that it can be undone');
  assert.equal($('tagRoleMap').open, false, 'setup: the details was opened by a re-render'); // its state is the person's, never the render's
});

test('Remove sends an empty value, and with the row gone focus lands on the name field', async () => {
  edits.length = 0;
  const after = { ...tree, roleMap: tree.roleMap.filter((m) => m.name !== 'Unused') };
  tree = after;
  await act(row('Unused').querySelector('.tagRoleRemove'));
  assert.deepEqual(edits, [{ edits: [{ kind: 'rolemap', role: 'Unused', value: '' }] }], 'Remove did not send the rolemap edit with an empty value');
  assert.equal(row('Unused'), undefined, 'the removed mapping still has a row');
  assert.equal($('tagRoleSummary').textContent, 'Role map — 3 custom types mapped', 'the summary does not count the mappings left');
  assert.equal(doc.activeElement, $('tagRoleNewName'), 'with its row gone, focus is not on the name field');
  assert.match($('tagRoleStatus').textContent, /mapping of Unused is removed — Ctrl\+Z/, 'the status does not say what was removed');
});

test('Add maps a new name, by its button or Enter in the name field, and focus lands on the new row', async () => {
  edits.length = 0;
  $('tagRoleNewName').value = '  Side Bar  ';
  $('tagRoleNewType').value = 'Sect';
  tree = { ...tree, roleMap: [...tree.roleMap, { name: 'Side Bar', to: 'Sect', standard: 'Sect', elements: 0 }] };
  await act($('tagRoleAdd'));
  assert.deepEqual(edits, [{ edits: [{ kind: 'rolemap', role: 'Side Bar', value: 'Sect' }] }], 'Add did not send the trimmed name and the chosen type');
  assert.equal(doc.activeElement, row('Side Bar').querySelector('.tagRoleChange'), 'focus is not on the new row\'s control');
  assert.equal($('tagRoleNewName').value, '', 'the name field still holds a name that now has its row');

  edits.length = 0;
  $('tagRoleNewName').value = 'Callout';
  $('tagRoleNewType').value = 'Note';
  tree = { ...tree, roleMap: [...tree.roleMap, { name: 'Callout', to: 'Note', standard: 'Note', elements: 0 }] };
  setNextDocument({ numPages: 2 });
  $('tagRoleNewName').focus();
  $('tagRoleNewName').dispatchEvent(new win.KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }));
  await settle();
  await settle();
  await settle();
  assert.deepEqual(edits, [{ edits: [{ kind: 'rolemap', role: 'Callout', value: 'Note' }] }], 'Enter in the name field did not add the mapping');

  edits.length = 0;
  $('tagRoleNewName').value = '   ';
  await act($('tagRoleAdd'));
  assert.deepEqual(edits, [], 'an empty name was sent');
  assert.match($('tagRoleStatus').textContent, /Type the name/, 'an empty name is not asked for');
  assert.equal(doc.activeElement, $('tagRoleNewName'), 'an empty name does not put focus in the field that needs it');
});

test('a refusal keeps what was typed, shows the server\'s sentence, and leaves nothing for the next read to act on', async () => {
  edits.length = 0;
  $('tagRoleNewName').value = 'H1';
  editReply = () => new Response(JSON.stringify({ error: 'H1 is a standard structure type, and a standard type cannot be mapped to another' }),
    { status: 400, headers: { 'Content-Type': 'application/json' } });
  await act($('tagRoleAdd'));
  editReply = null;
  assert.equal(edits.length, 1, 'the refused edit was not sent');
  assert.match($('tagRoleStatus').textContent, /H1 is a standard structure type/, 'the server\'s sentence is not shown');
  assert.equal($('tagRoleNewName').value, 'H1', 'a refusal emptied the name field');
  // The next read of the tree, for any reason, does not act on the refused edit's restore.
  $('tagRoleNewName').blur();
  await reopenPanel();
  assert.equal($('tagRoleStatus').textContent, '', 'a later read of the tree announced an edit that was refused');
  assert.notEqual(doc.activeElement, $('tagRoleNewName'), 'a later read of the tree moved focus for an edit that was refused');
});

test('a document with no mappings is still offered the section, and an untagged one is not', async () => {
  tree = { tagged: true, unaddressable: 0, elements: baseTree().elements, roleMap: [] };
  await reopenPanel();
  assert.equal($('tagRoleMap').hidden, false, 'a tagged document with no role map cannot be given one');
  assert.equal($('tagRoleSummary').textContent, 'Role map — no custom types mapped', 'the summary does not say there are none');
  assert.equal(rows().length, 0, 'rows are left from the tree before');

  tree = { tagged: false, unaddressable: 0, elements: [], roleMap: [] };
  await reopenPanel();
  assert.equal($('tagRoleMap').hidden, true, 'an untagged document is offered a role map');
});
