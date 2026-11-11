// Page Functions → Edit Objects, the real click (PLAN-image-edit P01.S04, plan-review pin "tier 3 click"): a page
// turned by /Rotate 90 draws two pictures at known places; the test clicks where the SECOND is displayed, removes
// it, and reads back from the server that exactly that picture's address went — then Undo, and the document is
// byte for byte the one opened. A box placed from user space instead of the displayed page would put the click
// on the wrong picture, or on none, on a turned page.
import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { launch, shutdown } from './harness.mjs';
import { writeRawFixture } from './fixtures.mjs';

// twoPicturesTurned is one 612×792 page with /Rotate 90 drawing two different 4×4 grey pictures.
function twoPicturesTurned() {
  const content = 'q 150 0 0 120 60 80 cm /Im0 Do Q\nq 140 0 0 100 360 560 cm /Im1 Do Q\nBT /F1 18 Tf 72 400 Td (Two pictures) Tj ET';
  const image = (bytes) => `<< /Type /XObject /Subtype /Image /Width 4 /Height 4 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 16 >>\nstream\n${bytes}\nendstream`;
  const objs = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Rotate 90 /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> /XObject << /Im0 6 0 R /Im1 7 0 R >> >> >>',
    `<< /Length ${content.length} >>\nstream\n${content}\nendstream`,
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
    image('AAAAAAAAAAAAAAAA'),
    image('zzzzzzzzzzzzzzzz'),
  ];
  let out = '%PDF-1.7\n';
  const offsets = [];
  objs.forEach((o, i) => {
    offsets.push(out.length);
    out += `${i + 1} 0 obj\n${o}\nendobj\n`;
  });
  const xref = out.length;
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n`;
  for (const o of offsets) out += `${String(o).padStart(10, '0')} 00000 n \n`;
  out += `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(out, 'latin1');
}

const DOC = writeRawFixture('editobjects.pdf', twoPicturesTurned());
const h = await launch();
const { page } = h;
after(() => shutdown(h));

const listing = () => page.evaluate(async () => (await (await nibFetch('/api/page/objects?page=1')).json()).objects);
const served = () => page.evaluate(async () => {
  const b = new Uint8Array(await (await nibFetch('/api/pdf')).arrayBuffer());
  const d = new Uint8Array(await crypto.subtle.digest('SHA-256', b));
  return Array.from(d, (x) => x.toString(16).padStart(2, '0')).join('');
});

test('a click on the second of two pictures on a turned page removes that picture, and Undo puts the bytes back', async () => {
  await h.openDocument(DOC, 1);
  const opened = await served();
  const before = await listing();
  assert.equal(before.length, 2, `setup: the page lists ${before.length} pictures, want 2`);
  assert.ok(before.every((o) => o.editable), 'setup: a fixture picture is not removable');

  await h.mode('edit');
  await h.card('Edit Objects');
  await page.click('#objectsBtn');
  await page.waitForFunction(() => document.querySelectorAll('.viewerContainer:not([hidden]) .obj-outline').length === 2, null, { timeout: 15000 });

  // Where the SECOND picture is displayed, worked out from the fixture and not from the server: its `cm` puts it at
  // x 360-500, y 560-660 of a 612×792 page, and /Rotate 90 shows user y across and user x down a 792-wide, 612-high
  // page — [560/792, 360/612, 660/792, 500/612]. The server's box must agree; the click goes to the fixture's point.
  const want = [560 / 792, 360 / 612, 660 / 792, 500 / 612];
  before[1].box.forEach((v, i) => assert.ok(Math.abs(v - want[i]) < 0.002,
    `the server places the second picture at ${before[1].box}, not where /Rotate 90 displays it (${want})`));
  const [l, t, r, b] = want;
  const pt = await page.evaluate(([fx, fy]) => {
    const pageEl = document.querySelector('.viewerContainer:not([hidden]) .page');
    pageEl.scrollIntoView({ block: 'center' });
    const rect = pageEl.getBoundingClientRect();
    const cs = getComputedStyle(pageEl);
    const bl = parseFloat(cs.borderLeftWidth) || 0, bt = parseFloat(cs.borderTopWidth) || 0;
    const w = rect.width - bl - (parseFloat(cs.borderRightWidth) || 0);
    const hgt = rect.height - bt - (parseFloat(cs.borderBottomWidth) || 0);
    return { x: Math.round(rect.x + bl + w * fx), y: Math.round(rect.y + bt + hgt * fy) };
  }, [(l + r) / 2, (t + b) / 2]);
  // And what the page DRAWS there is the second picture's grey ('z', 0x7a), not the first's ('A', 0x41): the point is
  // on the picture as pdf.js rendered it, whatever the arithmetic above got wrong.
  const grey = await page.evaluate(({ x, y }) => {
    const c = document.querySelector('.viewerContainer:not([hidden]) .page canvas');
    const rc = c.getBoundingClientRect();
    const px = Math.floor((x - rc.x) * c.width / rc.width), py = Math.floor((y - rc.y) * c.height / rc.height);
    return c.getContext('2d').getImageData(px, py, 1, 1).data[0];
  }, pt);
  assert.ok(Math.abs(grey - 0x7a) <= 6, `the click point draws grey ${grey}, not the second picture's 122`);
  await page.mouse.click(pt.x, pt.y);
  await page.waitForTimeout(300);
  const notice = await page.evaluate(() => document.getElementById('objectNoticeText').textContent);
  assert.match(notice, /^Picture 2 of 2, page 1, selected/, `the click selected nothing, or the wrong picture: the region says "${notice}"`);

  await page.click('#objectsRemoveBtn');
  await page.waitForFunction(() => /^Removed the picture/.test(document.getElementById('objectNoticeText').textContent), null, { timeout: 15000 });
  const afterRemove = await listing();
  assert.deepEqual(afterRemove.map((o) => o.digest), [before[0].digest],
    'the picture that went is not the one under the click: the page lists the wrong survivor');

  const removed = await served();
  await page.click('#undoBtn');
  // Waited on the bytes changing, not on a clock: Undo is a request and a reload.
  for (let i = 0; i < 60 && (await served()) === removed; i++) await page.waitForTimeout(250);
  assert.equal(await served(), opened, 'Undo did not bring back the document byte for byte');
});

// PLAN-image-edit P02.S03: a drag on the turned page moves the second picture by what the pointer travelled, in the
// page's displayed fractions; the server reads it back there, the page draws it there, and Undo puts the bytes back.
test('a drag moves the second picture on the turned page where the pointer went, and Undo puts the bytes back', async () => {
  const opened = await served();
  const before = await listing();
  assert.equal(before.length, 2, `setup: the page lists ${before.length} pictures, want 2`);
  assert.ok(before[1].movable, `setup: the second picture is not movable (${before[1].moveWhy})`);
  if (await page.evaluate(() => document.getElementById('objectsBtn').getAttribute('aria-pressed') !== 'true')) await page.click('#objectsBtn');
  await page.waitForFunction(() => document.querySelectorAll('.viewerContainer:not([hidden]) .obj-outline').length === 2, null, { timeout: 15000 });

  const geo = await page.evaluate((box) => {
    const pageEl = document.querySelector('.viewerContainer:not([hidden]) .page');
    pageEl.scrollIntoView({ block: 'center' });
    const rect = pageEl.getBoundingClientRect();
    const cs = getComputedStyle(pageEl);
    const bl = parseFloat(cs.borderLeftWidth) || 0, bt = parseFloat(cs.borderTopWidth) || 0;
    const w = rect.width - bl - (parseFloat(cs.borderRightWidth) || 0);
    const hgt = rect.height - bt - (parseFloat(cs.borderBottomWidth) || 0);
    const [l, t, r, b] = box;
    return { x0: rect.x + bl, y0: rect.y + bt, w, h: hgt, cx: rect.x + bl + w * (l + r) / 2, cy: rect.y + bt + hgt * (t + b) / 2 };
  }, before[1].box);
  const dx = -Math.round(geo.w * 0.3), dy = -Math.round(geo.h * 0.2);
  await page.mouse.move(Math.round(geo.cx), Math.round(geo.cy));
  await page.mouse.down();
  await page.mouse.move(Math.round(geo.cx) + dx / 2, Math.round(geo.cy) + dy / 2, { steps: 4 });
  await page.mouse.move(Math.round(geo.cx) + dx, Math.round(geo.cy) + dy, { steps: 4 });
  await page.mouse.up();
  await page.waitForFunction(() => /^Moved the picture/.test(document.getElementById('objectNoticeText').textContent), null, { timeout: 15000 });

  const after = await listing();
  const want = before[1].box.map((v, i) => v + (i % 2 === 0 ? dx / geo.w : dy / geo.h));
  after[1].box.forEach((v, i) => assert.ok(Math.abs(v - want[i]) < 0.004,
    `the server reads the moved picture at ${after[1].box}, not where it was dragged (${want})`));
  assert.equal(after[0].digest, before[0].digest, 'the other picture changed');
  // What the page draws at the new centre is the second picture's grey.
  await page.waitForTimeout(500);
  const grey = await page.evaluate(({ fx, fy }) => {
    const c = document.querySelector('.viewerContainer:not([hidden]) .page canvas');
    return c.getContext('2d').getImageData(Math.floor(c.width * fx), Math.floor(c.height * fy), 1, 1).data[0];
  }, { fx: (want[0] + want[2]) / 2, fy: (want[1] + want[3]) / 2 });
  assert.ok(Math.abs(grey - 0x7a) <= 6, `the page draws grey ${grey} where the picture was dragged, not the picture's 122`);

  const moved = await served();
  await page.click('#undoBtn');
  for (let i = 0; i < 60 && (await served()) === moved; i++) await page.waitForTimeout(250);
  assert.equal(await served(), opened, 'Undo did not bring back the document byte for byte');
});
