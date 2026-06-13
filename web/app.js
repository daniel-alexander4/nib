// Nib front-end. pdf.js renders the page and its interactive AcroForm
// fields; the user fills them in place; saveDocument() writes the edits back
// into the PDF and we hand the bytes to the server to persist. The server only
// does I/O and signature verification — the fill lives entirely here, in the
// same engine Firefox uses.

import * as pdfjsLib from './vendor/pdfjs/pdf.min.mjs';
import {
  PDFViewer,
  EventBus,
  PDFLinkService,
  PDFFindController,
  GenericL10n,
} from './vendor/pdfjs/pdf_viewer.mjs';

pdfjsLib.GlobalWorkerOptions.workerSrc = './vendor/pdfjs/pdf.worker.min.mjs';

// --- element handles ---------------------------------------------------------
const $ = (id) => document.getElementById(id);
const els = {
  menubar: $('menubar'), toolbar: $('toolbar'), openMenuItem: $('openMenuItem'),
  pathInput: $('pathInput'), openGo: $('openGo'),
  textToolBtn: $('textToolBtn'), detectBtn: $('detectBtn'),
  prevBtn: $('prevBtn'), nextBtn: $('nextBtn'),
  zoomInBtn: $('zoomInBtn'), zoomOutBtn: $('zoomOutBtn'), fitBtn: $('fitBtn'),
  sigBadge: $('sigBadge'), saveBtn: $('saveBtn'), statusCluster: $('statusCluster'),
  viewerWrap: $('viewerWrap'), viewerContainer: $('viewerContainer'),
  thumbs: $('thumbs'), thumbGrid: $('thumbGrid'), outline: $('outline'),
  appendBtn: $('appendBtn'), appendInput: $('appendInput'),
  redactBtn: $('redactBtn'), applyRedactBtn: $('applyRedactBtn'),
  editTextBtn: $('editTextBtn'), removeOriginalsBtn: $('removeOriginalsBtn'),
  scanBtn: $('scanBtn'), scanModal: $('scanModal'), scanBody: $('scanBody'),
  scanStripBtn: $('scanStripBtn'), scanSafeBtn: $('scanSafeBtn'),
  scanFlattenBtn: $('scanFlattenBtn'), scanClose: $('scanClose'),
  backupBtn: $('backupBtn'), restoreInput: $('restoreInput'), checkUpdatesBtn: $('checkUpdatesBtn'),
  updatePill: $('updatePill'), updateGet: $('updateGet'), updateDismiss: $('updateDismiss'),
  manageKeysBtn: $('manageKeysBtn'), keysModal: $('keysModal'), keysList: $('keysList'),
  keyCandidates: $('keyCandidates'), keyPaste: $('keyPaste'), keyAddPath: $('keyAddPath'),
  keyAddBtn: $('keyAddBtn'), keyCreateBtn: $('keyCreateBtn'), keysClose: $('keysClose'),
  managePeersBtn: $('managePeersBtn'), peersModal: $('peersModal'), peersList: $('peersList'),
  peerSelfFp: $('peerSelfFp'), peerPaste: $('peerPaste'), peerLabel: $('peerLabel'),
  peerPinBtn: $('peerPinBtn'), peersClose: $('peersClose'),
  cosignBtn: $('cosignBtn'), cosignModal: $('cosignModal'), cosignPeer: $('cosignPeer'),
  cosignIntent: $('cosignIntent'), cosignNoPeers: $('cosignNoPeers'),
  cosignCancel: $('cosignCancel'), cosignGo: $('cosignGo'),
  peerSelfCopy: $('peerSelfCopy'),
  sessionInitBtn: $('sessionInitBtn'), sessionInitModal: $('sessionInitModal'),
  sinPeer: $('sinPeer'), sinNoPeers: $('sinNoPeers'), sinAddr: $('sinAddr'),
  sinIntent: $('sinIntent'), sinProgress: $('sinProgress'),
  sinCancel: $('sinCancel'), sinGo: $('sinGo'),
  sessionRecvBtn: $('sessionRecvBtn'), sessionRecvModal: $('sessionRecvModal'),
  srvArm: $('srvArm'), srvWait: $('srvWait'), srvConsent: $('srvConsent'),
  srvPeer: $('srvPeer'), srvNoPeers: $('srvNoPeers'), srvBind: $('srvBind'),
  srvSelfFp: $('srvSelfFp'), srvSelfCopy: $('srvSelfCopy'),
  srvCancel: $('srvCancel'), srvArmGo: $('srvArmGo'),
  srvWaitAddr: $('srvWaitAddr'), srvWaitPeer: $('srvWaitPeer'), srvDisarm: $('srvDisarm'),
  srvPeerLabel: $('srvPeerLabel'), srvPeerFp: $('srvPeerFp'), srvPeerCopy: $('srvPeerCopy'),
  srvPeerReason: $('srvPeerReason'), srvPreview: $('srvPreview'), srvIntent: $('srvIntent'),
  srvDecline: $('srvDecline'), srvAccept: $('srvAccept'),
  authOverlay: $('authOverlay'), authForm: $('authForm'), authTitle: $('authTitle'),
  authHint: $('authHint'), authPw: $('authPw'), migrateRow: $('migrateRow'),
  keyChoice: $('keyChoice'), keySelect: $('keySelect'), keyPath: $('keyPath'),
  createPath: $('createPath'), authWarn: $('authWarn'),
  introOverlay: $('introOverlay'),
  authSubmit: $('authSubmit'), authError: $('authError'),
  addImageBtn: $('addImageBtn'), drawSigBtn: $('drawSigBtn'), addImageInput: $('addImageInput'),
  imageGrid: $('imageGrid'),
  sigModal: $('sigModal'), sigCanvas: $('sigCanvas'),
  sigClear: $('sigClear'), sigCancel: $('sigCancel'), sigSave: $('sigSave'),
  sigDetailsBtn: $('sigDetailsBtn'), sigDetailsModal: $('sigDetailsModal'),
  sigDetailsBody: $('sigDetailsBody'), sigDetailsClose: $('sigDetailsClose'),
  bgModal: $('bgModal'), bgCanvas: $('bgCanvas'), bgRemove: $('bgRemove'),
  bgThresh: $('bgThresh'), bgThreshRow: $('bgThreshRow'),
  bgCancel: $('bgCancel'), bgSave: $('bgSave'),
  autofillBtn: $('autofillBtn'), editProfileBtn: $('editProfileBtn'),
  saveFlatBtn: $('saveFlatBtn'), saveEditableBtn: $('saveEditableBtn'), finalizeBtn: $('finalizeBtn'),
  exportZipBtn: $('exportZipBtn'), exportPngBtn: $('exportPngBtn'),
  exportFormJsonBtn: $('exportFormJsonBtn'), exportFormCsvBtn: $('exportFormCsvBtn'),
  exportCertBtn: $('exportCertBtn'),
  finalizeModal: $('finalizeModal'), fzText: $('fzText'), fzDate: $('fzDate'),
  fzTsa: $('fzTsa'), fzTsaOn: $('fzTsaOn'), fzCancel: $('fzCancel'), fzGo: $('fzGo'),
  fzOpacity: $('fzOpacity'), fzSize: $('fzSize'), fzAngle: $('fzAngle'), fzColor: $('fzColor'),
  fzPreviewMark: $('fzPreviewMark'),
  profileModal: $('profileModal'), profileText: $('profileText'),
  profileCancel: $('profileCancel'), profileSave: $('profileSave'),
  saveAsModal: $('saveAsModal'), saveAsTitle: $('saveAsTitle'), saveAsName: $('saveAsName'),
  saveAsDir: $('saveAsDir'), saveAsHere: $('saveAsHere'), saveAsUp: $('saveAsUp'),
  saveAsList: $('saveAsList'), saveAsCancel: $('saveAsCancel'), saveAsGo: $('saveAsGo'),
  openModal: $('openModal'), openDir: $('openDir'), openHere: $('openHere'),
  openUp: $('openUp'), openList: $('openList'), openCancel: $('openCancel'),
  autoUpdateChk: $('autoUpdateChk'),
  aboutBtn: $('aboutBtn'), aboutModal: $('aboutModal'), aboutTitle: $('aboutTitle'),
  aboutMain: $('aboutMain'), aboutDocText: $('aboutDocText'), aboutVersion: $('aboutVersion'),
  aboutLicenseBtn: $('aboutLicenseBtn'), aboutNoticesBtn: $('aboutNoticesBtn'),
  aboutBackBtn: $('aboutBackBtn'), aboutClose: $('aboutClose'),
};

// Controls duplicated across the menubar and toolbar are addressed by class.
const all = (sel) => document.querySelectorAll(sel);

// --- unlock: SSH key + CSRF --------------------------------------------------
// Nib unlocks at startup from the user's SSH key. The first-run wizard
// enrolls a key (or migrates an old password vault); after that the vault opens
// with no prompt. csrf is the per-process token issued when the vault unlocks.
let csrf = null;
let authState = 'setup'; // setup | migrate | key-missing | ready

// apiFetch wraps fetch with the CSRF header on writes; a 401 reopens the wizard.
async function apiFetch(url, opts = {}) {
  opts.headers = { ...(opts.headers || {}) };
  if (opts.method && opts.method !== 'GET') opts.headers['X-CSRF-Token'] = csrf;
  const res = await fetch(url, opts);
  if (res.status === 401) { refreshStatus(); throw new Error('locked'); }
  return res;
}

function selectedKeyMode() {
  return els.authForm.querySelector('input[name="keymode"]:checked')?.value || 'use';
}

// syncKeyMode shows only the control that belongs to the selected key mode, so
// each option (including "create") reads as a distinct, actionable choice.
function syncKeyMode() {
  const mode = selectedKeyMode();
  els.keySelect.hidden = mode !== 'use';
  els.keyPath.hidden = mode !== 'path';
  els.createPath.hidden = mode !== 'create';
}
els.keyChoice.addEventListener('change', syncKeyMode);

// The first-run intro popup explains the SSH key before the wizard; it stays up
// until the user clicks the backdrop (off the card).
let introSeen = false;
els.introOverlay.addEventListener('click', (e) => {
  if (e.target === els.introOverlay) { els.introOverlay.hidden = true; introSeen = true; }
});

// applyStatus drives the UI from /api/status.
function applyStatus(st) {
  authState = st.state;
  els.aboutVersion.textContent = st.version || 'dev';
  if (st.state === 'ready') {
    csrf = st.csrf;
    els.authOverlay.hidden = true;
    loadImages();
    // Apply saved preferences: the toolbar layout and the auto-update toggle.
    applyStyle(st.toolbarStyle || 'menus');
    els.autoUpdateChk.checked = st.autoUpdate;
    els.autoUpdateChk.disabled = st.updateCheckLocked;
    els.autoUpdateChk.parentElement.title = st.updateCheckLocked ? 'Forced off by NIB_NO_UPDATE_CHECK' : '';
    // Automatic update check, once per session, at the first usable moment.
    if (st.autoUpdate && !updateChecked) { updateChecked = true; runUpdateCheck(true); }
    const initial = new URLSearchParams(location.search).get('open');
    if (initial && !pdfDocument) openPath(initial).catch((e) => toast('could not open: ' + e.message));
    return;
  }

  csrf = null;
  els.authError.textContent = '';
  els.authOverlay.hidden = false;
  els.migrateRow.hidden = st.state !== 'migrate';

  // Populate the existing-key dropdown from detected ~/.ssh keys.
  els.keySelect.innerHTML = '';
  for (const path of st.candidates || []) {
    const o = document.createElement('option');
    o.value = path; o.textContent = path;
    els.keySelect.appendChild(o);
  }
  const haveCandidates = (st.candidates || []).length > 0;

  if (st.state === 'key-missing') {
    els.authTitle.textContent = 'Unlock key not found';
    els.authHint.textContent = `Nib can't read the SSH key it was set up with (${st.keyPath || 'unknown path'}). Restore that key file, then retry.`;
    els.authWarn.hidden = true;
    els.keyChoice.hidden = true;
    els.authSubmit.textContent = 'Retry';
    return;
  }
  els.keyChoice.hidden = false;
  els.authWarn.hidden = false;
  els.authTitle.textContent = st.state === 'migrate' ? 'Migrate to SSH-key unlock' : 'Set up Nib';
  els.authHint.textContent = st.state === 'migrate'
    ? 'Enter your old vault password once; Nib will re-key the vault to your SSH key.'
    : 'Choose the SSH key that unlocks Nib. No password is used.';
  els.createPath.value = st.defaultKeyPath || '~/.ssh/id_ed25519';
  els.authForm.querySelector('input[value="use"]').checked = haveCandidates;
  els.authForm.querySelector('input[value="create"]').checked = !haveCandidates;
  els.authSubmit.textContent = st.state === 'migrate' ? 'Migrate' : 'Enable';
  syncKeyMode();
  // First run only: introduce the SSH key before the user picks one.
  if (st.state === 'setup' && !introSeen) els.introOverlay.hidden = false;
}

async function refreshStatus() {
  try {
    applyStatus(await (await fetch('/api/status')).json());
  } catch {
    els.authOverlay.hidden = false;
    els.authError.textContent = 'Could not reach Nib.';
  }
}

els.authForm.addEventListener('submit', async (e) => {
  e.preventDefault();
  els.authError.textContent = '';
  const mode = selectedKeyMode();
  const body = { password: els.authPw.value };
  if (mode === 'create') {
    body.mode = 'create';
    body.keyPath = els.createPath.value.trim();
  } else if (mode === 'path') {
    body.mode = 'use';
    body.keyPath = els.keyPath.value.trim();
    if (!body.keyPath) return (els.authError.textContent = 'Enter a key path.');
  } else {
    body.mode = 'use';
    body.keyPath = els.keySelect.value;
    if (!body.keyPath) return (els.authError.textContent = 'No key selected.');
  }
  // Recovery: once the key is back at its path, retrying unlock is just a status
  // re-check (ensureUnlocked runs server-side).
  if (authState === 'key-missing') { await refreshStatus(); return; }

  const url = authState === 'migrate' ? '/api/ssh/migrate' : '/api/ssh/enroll';
  const res = await fetch(url, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const st = await res.json();
  if (!res.ok) { els.authError.textContent = st.error || 'failed'; return; }
  applyStatus(st);
});

// --- vault backup / restore --------------------------------------------------
els.backupBtn.onclick = () => { window.location = '/api/vault/export'; };
els.restoreInput.onchange = async () => {
  const file = els.restoreInput.files[0]; if (!file) return;
  if (!confirm('Replace your current vault with this backup? It will only open if this machine’s SSH key is enrolled in it.')) return;
  const res = await apiFetch('/api/vault/import', { method: 'POST', body: await file.arrayBuffer() });
  if (res.ok) applyStatus(await res.json()); else toast((await res.json()).error || 'restore failed');
};

// --- update check ------------------------------------------------------------
// Runs once at startup (when autoUpdate is set) and from "Check for updates…".
// Notify-only: a newer release raises the menubar pill, which downloads the
// asset for this OS/arch on click. Nib installs nothing.
let updateChecked = false;
let updateInfo = null; // last check result, for the pill's download action

// runUpdateCheck queries the server. auto=true is the silent startup check (only
// surfaces the pill); auto=false is the manual menu item (also toasts the result).
async function runUpdateCheck(auto) {
  let d;
  try {
    const res = await fetch('/api/update/check');
    if (!res.ok) throw new Error();
    d = await res.json();
  } catch {
    if (!auto) toast('Could not check for updates.');
    return;
  }
  if (!d.updateAvailable) {
    // No upgrade needed: show the current version as a static badge (no
    // download action, no dismiss) rather than hiding the pill.
    updateInfo = null;
    els.updatePill.classList.add('current');
    els.updateGet.textContent = `v${d.current}`;
    els.updateGet.title = d.latest ? `You’re on the latest version (v${d.current})` : `No published releases yet (you have v${d.current})`;
    els.updatePill.hidden = false;
    if (!auto) toast(d.latest ? `You’re on the latest version (v${d.current}).` : `No published releases yet (you have v${d.current}).`);
    return;
  }
  updateInfo = d;
  els.updatePill.classList.remove('current');
  els.updateGet.title = 'Download the latest version for your system';
  els.updateGet.textContent = `Update to v${d.latest} ↓`;
  els.updatePill.hidden = false;
  if (!auto) toast(`Nib v${d.latest} is available — click the pill to download.`);
}

els.checkUpdatesBtn.onclick = () => runUpdateCheck(false);
els.updateDismiss.onclick = () => { els.updatePill.hidden = true; };
els.updateGet.onclick = () => {
  if (!updateInfo) return;
  // A release asset serves as an attachment, so this downloads without leaving
  // the app; the release page (fallback) opens in a new tab.
  window.open(updateInfo.downloadUrl || updateInfo.url, '_blank', 'noopener');
};

// --- authorized keys ---------------------------------------------------------
// The vault's content key is sealed to one or more SSH public keys; this dialog
// lists them, lets the user authorize another (paste a line or pick a local
// key), and revoke old ones. The server blocks removing the only key or the one
// in use this session.

// keyLabel derives a friendly label for an authorized_keys line: its comment if
// present, otherwise a truncated key blob.
function keyLabel(line) {
  const f = line.trim().split(/\s+/);
  const comment = f.slice(2).join(' ');
  const blob = f[1] || line;
  return comment || (blob.length > 22 ? blob.slice(0, 11) + '…' + blob.slice(-8) : blob);
}

function renderKeys(data) {
  els.keysList.innerHTML = '';
  for (const k of data.keys) {
    const algo = k.pubKey.trim().split(/\s+/)[0] || '';
    const row = document.createElement('div');
    row.className = 'keyrow';
    const meta = document.createElement('div');
    meta.className = 'keymeta';
    const fp = document.createElement('div');
    fp.className = 'keyfp';
    fp.textContent = `${algo} ${keyLabel(k.pubKey)}`;
    fp.title = k.pubKey;
    const sub = document.createElement('div');
    sub.className = 'keysub';
    sub.textContent = k.keyPath;
    meta.append(fp, sub);
    row.append(meta);
    if (k.current) {
      const cur = document.createElement('span');
      cur.className = 'keycur';
      cur.textContent = 'in use';
      row.append(cur);
    } else {
      const del = document.createElement('button');
      del.className = 'keydel';
      del.textContent = 'Remove';
      del.onclick = () => removeKey(k.pubKey, keyLabel(k.pubKey));
      row.append(del);
    }
    els.keysList.append(row);
  }
  els.keyCandidates.innerHTML = '';
  for (const path of data.candidates || []) {
    const b = document.createElement('button');
    b.textContent = `+ ${path}`;
    b.title = `Authorize the key at ${path}`;
    b.onclick = () => addKey({ mode: 'use', keyPath: path });
    els.keyCandidates.append(b);
  }
}

async function loadKeys() {
  const res = await apiFetch('/api/ssh/keys');
  if (res.ok) renderKeys(await res.json());
}

async function addKey(body) {
  const res = await apiFetch('/api/ssh/keys', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (res.ok) {
    renderKeys(await res.json());
    els.keyPaste.value = '';
    els.keyAddPath.value = '';
    toast('Key authorized');
  } else {
    toast((await res.json()).error || 'could not add key');
  }
}

async function removeKey(pubKey, label) {
  if (!confirm(`Remove authorized key “${label}”? It will no longer be able to unlock this vault.`)) return;
  const res = await apiFetch('/api/ssh/keys/remove', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ pubKey }),
  });
  if (res.ok) { renderKeys(await res.json()); toast('Key removed'); }
  else toast((await res.json()).error || 'could not remove key');
}

els.manageKeysBtn.onclick = () => { els.keysModal.hidden = false; loadKeys(); };
els.keysClose.onclick = () => { els.keysModal.hidden = true; };

// --- identity & pinned peers (co-signing) ------------------------------------
// groupFingerprint renders a 64-hex fingerprint in spaced quads so two people can
// compare it out-of-band character by character.
function groupFingerprint(hex) {
  return (hex.match(/.{1,4}/g) || []).join(' ');
}

// selfFingerprint is this identity's full hex SPKI, cached from /api/peers so the
// Copy buttons hand the peer the exact value to pin (the grouped display has spaces).
let selfFingerprint = '';

// copyFp puts a full ungrouped fingerprint on the clipboard for out-of-band
// comparison — the safety-number check that anchors who a live session is with.
async function copyFp(hex) {
  if (!hex) return;
  try { await navigator.clipboard.writeText(hex); toast('Fingerprint copied'); }
  catch { toast('could not copy'); }
}

function renderPeers(data) {
  selfFingerprint = data.fingerprint || '';
  els.peerSelfFp.textContent = groupFingerprint(selfFingerprint);
  els.peersList.innerHTML = '';
  for (const p of data.peers || []) {
    const row = document.createElement('div');
    row.className = 'keyrow';
    const meta = document.createElement('div');
    meta.className = 'keymeta';
    const name = document.createElement('div');
    name.className = 'keyfp';
    name.textContent = p.label || 'Unlabelled peer';
    const fp = document.createElement('div');
    fp.className = 'keysub';
    fp.textContent = groupFingerprint(p.fingerprint);
    meta.append(name, fp);
    const copy = document.createElement('button');
    copy.className = 'copyfp';
    copy.textContent = 'Copy';
    copy.title = 'Copy fingerprint';
    copy.onclick = () => copyFp(p.fingerprint);
    const del = document.createElement('button');
    del.className = 'keydel';
    del.textContent = 'Unpin';
    del.onclick = () => unpinPeer(p.fingerprint, p.label || p.fingerprint);
    row.append(meta, copy, del);
    els.peersList.append(row);
  }
}

async function loadPeers() {
  const res = await apiFetch('/api/peers');
  if (res.ok) renderPeers(await res.json());
}

async function pinPeer() {
  const res = await apiFetch('/api/peers/pin', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ fingerprint: els.peerPaste.value, label: els.peerLabel.value }),
  });
  if (res.ok) {
    renderPeers(await res.json());
    els.peerPaste.value = '';
    els.peerLabel.value = '';
    toast('Peer pinned');
  } else {
    toast((await res.json()).error || 'could not pin peer');
  }
}

async function unpinPeer(fingerprint, label) {
  if (!confirm(`Unpin “${label}”? You'll need to compare and pin their fingerprint again to co-sign with them.`)) return;
  const res = await apiFetch('/api/peers/remove', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ fingerprint }),
  });
  if (res.ok) { renderPeers(await res.json()); toast('Peer unpinned'); }
  else toast((await res.json()).error || 'could not unpin peer');
}

els.managePeersBtn.onclick = () => { els.peersModal.hidden = false; loadPeers(); };
els.peersClose.onclick = () => { els.peersModal.hidden = true; };
els.peerPinBtn.onclick = pinPeer;
els.peerSelfCopy.onclick = () => copyFp(selfFingerprint);

// --- co-sign with a peer -----------------------------------------------------
async function openCosign() {
  if (!pdfDocument) return;
  const res = await apiFetch('/api/peers');
  if (!res.ok) { toast('could not load peers'); return; }
  const peers = (await res.json()).peers || [];
  els.cosignPeer.innerHTML = '';
  for (const p of peers) {
    const o = document.createElement('option');
    o.value = p.fingerprint;
    o.textContent = (p.label || 'Unlabelled peer') + ' — ' + groupFingerprint(p.fingerprint.slice(0, 8)) + '…';
    els.cosignPeer.append(o);
  }
  const none = peers.length === 0;
  els.cosignNoPeers.hidden = !none;
  els.cosignPeer.hidden = none;
  els.cosignGo.disabled = none;
  els.cosignModal.hidden = false;
}

// renderAttestation rasterizes the server-provided lines into a white block sized
// to the rect's aspect — the signing library stretches the image to fill the
// rect, so the PNG must match its width:height. The lines come from the server
// (Go AppearanceLines), never rebuilt here, so the block can't drift from /Reason.
async function renderAttestation(lines, rect) {
  const w = rect[2] - rect[0], h = rect[3] - rect[1], scale = 3;
  const cv = document.createElement('canvas');
  cv.width = Math.round(w * scale);
  cv.height = Math.round(h * scale);
  const ctx = cv.getContext('2d');
  ctx.fillStyle = '#fff'; ctx.fillRect(0, 0, cv.width, cv.height);
  ctx.strokeStyle = '#000'; ctx.lineWidth = scale; ctx.strokeRect(0, 0, cv.width, cv.height);
  ctx.fillStyle = '#000'; ctx.textBaseline = 'top';
  const pad = 4 * scale;
  const lineH = (cv.height - 2 * pad) / lines.length;
  ctx.font = Math.min(lineH * 0.7, 9 * scale) + 'px sans-serif';
  lines.forEach((ln, i) => ctx.fillText(ln, pad, pad + i * lineH));
  return await new Promise((r) => cv.toBlob(r, 'image/png'));
}

async function cosign() {
  const fingerprint = els.cosignPeer.value;
  if (!fingerprint) return;
  const intent = els.cosignIntent.value;
  els.cosignModal.hidden = true;
  const qr = await apiFetch('/api/cosign/quote', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ fingerprint, intent }),
  });
  if (!qr.ok) { toast((await qr.json()).error || 'could not start co-signing'); return; }
  const q = await qr.json();
  const png = await renderAttestation(q.lines, q.rect);

  const form = new FormData();
  form.append('pdf', new Blob([await bakedBytes()], { type: 'application/pdf' }), 'doc.pdf');
  form.append('params', JSON.stringify({ fingerprint, intent, when: q.when }));
  form.append('appearance', png, 'attestation.png');
  const sr = await apiFetch('/api/cosign/sign', { method: 'POST', body: form });
  if (!sr.ok) { toast((await sr.json()).error || 'could not co-sign'); return; }
  openSaveAs(await sr.blob(), exportBase() + '-cosigned.pdf', 'Save co-signed PDF');
}

els.cosignBtn.onclick = openCosign;
els.cosignCancel.onclick = () => { els.cosignModal.hidden = true; };
els.cosignGo.onclick = cosign;

// --- co-sign live (dial side) ------------------------------------------------
// Send the open document to a pinned peer who is armed to receive, co-sign it in
// real time, and adopt the doubly-signed result as the open document. The signing
// path is shared with Track A co-sign (/api/cosign/quote on the open document, then
// the rasterized appearance); only the transport differs.
async function openSessionInit() {
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  const res = await apiFetch('/api/peers');
  if (!res.ok) { toast('could not load peers'); return; }
  const peers = (await res.json()).peers || [];
  els.sinPeer.innerHTML = '';
  for (const p of peers) {
    const o = document.createElement('option');
    o.value = p.fingerprint;
    o.textContent = (p.label || 'Unlabelled peer') + ' — ' + groupFingerprint(p.fingerprint.slice(0, 8)) + '…';
    els.sinPeer.append(o);
  }
  const none = peers.length === 0;
  els.sinNoPeers.hidden = !none;
  els.sinPeer.hidden = none;
  els.sinProgress.hidden = true;
  els.sinGo.disabled = none;
  els.sessionInitModal.hidden = false;
}

async function sessionInit() {
  const fingerprint = els.sinPeer.value;
  if (!fingerprint) return;
  const address = els.sinAddr.value.trim();
  if (!address) { toast('Enter the peer’s address'); return; }
  const intent = els.sinIntent.value;
  // Quote against the open document — correct here, since the open document is what
  // we sign and send (unlike the receive flow, which signs the peer's document).
  const qr = await apiFetch('/api/cosign/quote', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ fingerprint, intent }),
  });
  if (!qr.ok) { toast((await qr.json()).error || 'could not start co-signing'); return; }
  const q = await qr.json();
  const png = await renderAttestation(q.lines, q.rect);

  els.sinGo.disabled = true; els.sinCancel.disabled = true; els.sinProgress.hidden = false;
  const form = new FormData();
  form.append('pdf', new Blob([await bakedBytes()], { type: 'application/pdf' }), 'doc.pdf');
  form.append('params', JSON.stringify({ fingerprint, intent, when: q.when }));
  form.append('appearance', png, 'attestation.png');
  form.append('address', address);
  const res = await apiFetch('/api/session/initiate', { method: 'POST', body: form });
  els.sinGo.disabled = false; els.sinCancel.disabled = false; els.sinProgress.hidden = true;
  if (!res.ok) { toast((await res.json()).error || 'co-signing did not complete'); return; }
  els.sessionInitModal.hidden = true;
  await setDocumentFromServer(await res.json());
  toast('Co-signed live — document updated');
}

els.sessionInitBtn.onclick = openSessionInit;
els.sinCancel.onclick = () => { els.sessionInitModal.hidden = true; };
els.sinGo.onclick = sessionInit;

// --- receive a live co-signature ---------------------------------------------
// Arm a pinned-peer-only listener, review the document a peer sends over the live
// channel in an isolated preview (never the main viewer — that would discard the
// open document's unsaved edits), and co-sign it with explicit consent. The session
// is one-shot: it tears down after a single accept, decline, or timeout.
let recvPoll = 0; // token; bump to invalidate any in-flight poll or preview render
let recvStage = 'arm'; // arm | wait | consent | applying | declining
let recvArmedLabel = ''; // the pinned label of the peer we armed for
let recvPeerFp = ''; // the connecting peer's verified fingerprint, for the Copy button

function showRecvView(which) {
  els.srvArm.hidden = which !== 'srvArm';
  els.srvWait.hidden = which !== 'srvWait';
  els.srvConsent.hidden = which !== 'srvConsent';
}

async function openSessionRecv() {
  const res = await apiFetch('/api/peers');
  if (!res.ok) { toast('could not load peers'); return; }
  const data = await res.json();
  selfFingerprint = data.fingerprint || '';
  els.srvSelfFp.textContent = groupFingerprint(selfFingerprint);
  const peers = data.peers || [];
  els.srvPeer.innerHTML = '';
  for (const p of peers) {
    const o = document.createElement('option');
    o.value = p.fingerprint;
    o.dataset.label = p.label || 'Unlabelled peer';
    o.textContent = (p.label || 'Unlabelled peer') + ' — ' + groupFingerprint(p.fingerprint.slice(0, 8)) + '…';
    els.srvPeer.append(o);
  }
  const none = peers.length === 0;
  els.srvNoPeers.hidden = !none;
  els.srvPeer.hidden = none;
  els.srvArmGo.disabled = none;
  els.srvIntent.value = 'I agree to sign this document.';
  recvStage = 'arm';
  showRecvView('srvArm');
  els.sessionRecvModal.hidden = false;
}

async function armRecv() {
  const opt = els.srvPeer.selectedOptions[0];
  if (!opt) return;
  const bind = els.srvBind.value.trim();
  if (!bind) { toast('Enter a listen address'); return; }
  recvArmedLabel = opt.dataset.label;
  els.srvArmGo.disabled = true;
  const res = await apiFetch('/api/session/arm', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ fingerprint: opt.value, bind }),
  });
  els.srvArmGo.disabled = false;
  if (!res.ok) { toast((await res.json()).error || 'could not arm'); return; }
  const st = await res.json();
  els.srvWaitAddr.textContent = st.address || bind;
  els.srvWaitPeer.textContent = recvArmedLabel;
  recvStage = 'wait';
  showRecvView('srvWait');
  const token = ++recvPoll;
  setTimeout(() => pollRecv(token), 1200);
}

// pollRecv drives the receive state machine off /api/session/status: a pending
// request promotes the wait view to consent; the session disarming ends the flow
// (reloading the open document if our accept was applied).
async function pollRecv(token) {
  if (token !== recvPoll) return;
  let st;
  try { st = await (await apiFetch('/api/session/status')).json(); }
  catch { return; } // 401 is handled by apiFetch; anything else stops this poll
  if (token !== recvPoll) return;
  if (st.pending && recvStage === 'wait') {
    recvStage = 'consent';
    showConsent(st.pending);
  } else if (!st.armed) {
    if (recvStage === 'applying') { await reloadOpenDoc(); toast('Co-signed — document updated'); }
    else if (recvStage === 'wait') toast('Session ended — no peer connected');
    else if (recvStage === 'consent') toast('Session timed out');
    endRecv();
    return;
  }
  setTimeout(() => pollRecv(token), 1500);
}

function showConsent(pending) {
  recvPeerFp = pending.fingerprint || '';
  els.srvPeerLabel.textContent = recvArmedLabel || 'your pinned peer';
  els.srvPeerFp.textContent = groupFingerprint(recvPeerFp);
  els.srvPeerReason.textContent = pending.reason || '(none given)';
  showRecvView('srvConsent');
  loadPendingPreview(recvPoll);
}

// loadPendingPreview renders the received document in its own pdf.js instance,
// entirely apart from the main viewer, so reviewing (and declining) a peer's
// document never disturbs the open document or its unsaved edits.
async function loadPendingPreview(token) {
  els.srvPreview.innerHTML = '';
  let doc;
  try { doc = await pdfjsLib.getDocument({ url: '/api/session/pending-pdf?t=' + Date.now() }).promise; }
  catch { if (token === recvPoll) els.srvPreview.textContent = 'could not render the document'; return; }
  for (let i = 1; i <= doc.numPages; i++) {
    if (token !== recvPoll) return; // modal closed mid-render
    const page = await doc.getPage(i);
    const base = page.getViewport({ scale: 1 });
    const vp = page.getViewport({ scale: Math.min(1.2, 380 / base.width) });
    const canvas = document.createElement('canvas');
    canvas.width = Math.ceil(vp.width);
    canvas.height = Math.ceil(vp.height);
    els.srvPreview.append(canvas);
    await page.render({ canvasContext: canvas.getContext('2d'), viewport: vp }).promise;
  }
}

async function acceptRecv() {
  els.srvAccept.disabled = true; els.srvDecline.disabled = true;
  const intent = els.srvIntent.value;
  // The responder's visible block is placed server-side; the quote gives only the
  // canonical lines, so the rendered image can't drift from the signed /Reason.
  let appearance = '';
  const qr = await apiFetch('/api/session/quote', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ intent }),
  });
  if (qr.ok) {
    const q = await qr.json();
    appearance = await blobToBase64(await renderAttestation(q.lines, q.rect));
  }
  const res = await apiFetch('/api/session/respond', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ accept: true, intent, appearance }),
  });
  if (!res.ok) {
    els.srvAccept.disabled = false; els.srvDecline.disabled = false;
    toast((await res.json()).error || 'could not co-sign');
    return;
  }
  recvStage = 'applying';
  toast('Signing…');
  // pollRecv detects the session disarming, then reloads the co-signed document.
}

async function declineRecv() {
  els.srvAccept.disabled = true; els.srvDecline.disabled = true;
  recvStage = 'declining';
  try {
    await apiFetch('/api/session/respond', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ accept: false }),
    });
  } catch { /* the session tears down regardless */ }
  toast('Declined — your document is unchanged');
  endRecv();
}

// reloadOpenDoc refreshes the viewer after runSession applies a received co-signature
// out of band (no response carries the new metadata, so the UI fetches it).
async function reloadOpenDoc() {
  try { await setDocumentFromServer(await (await apiFetch('/api/doc')).json()); }
  catch { toast('co-signed, but could not refresh the view'); }
}

// endRecv stops polling, discards the isolated preview, and closes the modal.
function endRecv() {
  recvPoll++; // invalidate any in-flight poll / preview render
  recvStage = 'arm';
  els.srvPreview.innerHTML = '';
  els.srvAccept.disabled = false; els.srvDecline.disabled = false;
  els.sessionRecvModal.hidden = true;
}

// cancelRecv closes the modal, disarming a live listener first if one is up.
async function cancelRecv() {
  const armed = recvStage === 'wait' || recvStage === 'consent';
  endRecv();
  if (armed) { try { await apiFetch('/api/session/disarm', { method: 'POST' }); } catch { /* shutting down anyway */ } }
}

// blobToBase64 returns the raw base64 of a Blob (no data: prefix) for JSON upload.
function blobToBase64(blob) {
  return new Promise((resolve, reject) => {
    const fr = new FileReader();
    fr.onload = () => resolve(String(fr.result).split(',')[1] || '');
    fr.onerror = reject;
    fr.readAsDataURL(blob);
  });
}

els.sessionRecvBtn.onclick = openSessionRecv;
els.srvCancel.onclick = cancelRecv;
els.srvDisarm.onclick = cancelRecv;
els.srvArmGo.onclick = armRecv;
els.srvAccept.onclick = acceptRecv;
els.srvDecline.onclick = declineRecv;
els.srvSelfCopy.onclick = () => copyFp(selfFingerprint);
els.srvPeerCopy.onclick = () => copyFp(recvPeerFp);

// About dialog: explainer by default; the licence/notices buttons swap the body
// for the embedded /legal/ document, and Back returns to the explainer.
function showAboutMain() {
  els.aboutTitle.textContent = 'About Nib';
  els.aboutMain.hidden = false;
  els.aboutDocText.hidden = true;
  els.aboutLicenseBtn.hidden = els.aboutNoticesBtn.hidden = false;
  els.aboutBackBtn.hidden = true;
}
async function showAboutDoc(title, path) {
  els.aboutDocText.textContent = 'Loading…';
  els.aboutTitle.textContent = title;
  els.aboutMain.hidden = true;
  els.aboutDocText.hidden = false;
  els.aboutLicenseBtn.hidden = els.aboutNoticesBtn.hidden = true;
  els.aboutBackBtn.hidden = false;
  try {
    const res = await fetch(path);
    els.aboutDocText.textContent = res.ok ? await res.text() : 'Could not load document.';
  } catch { els.aboutDocText.textContent = 'Could not load document.'; }
}
els.aboutBtn.onclick = () => { showAboutMain(); els.aboutModal.hidden = false; };
els.aboutClose.onclick = () => { els.aboutModal.hidden = true; };
els.aboutBackBtn.onclick = showAboutMain;
els.aboutLicenseBtn.onclick = () => showAboutDoc('Licence (GPLv3)', '/legal/LICENSE');
els.aboutNoticesBtn.onclick = () => showAboutDoc('Third-party notices', '/legal/THIRD-PARTY-NOTICES.md');
els.keyAddBtn.onclick = () => {
  const pubKey = els.keyPaste.value.trim();
  if (!pubKey) { toast('Paste a public key first'); return; }
  addKey({ mode: 'paste', pubKey, keyPath: els.keyAddPath.value.trim() });
};
els.keyCreateBtn.onclick = () => addKey({ mode: 'create', keyPath: els.keyAddPath.value.trim() });

// --- viewer wiring -----------------------------------------------------------
const eventBus = new EventBus();
const linkService = new PDFLinkService({ eventBus });
const findController = new PDFFindController({ eventBus, linkService });
const viewer = new PDFViewer({
  container: els.viewerContainer,
  eventBus,
  linkService,
  findController,
  l10n: new GenericL10n('en-US'),
  annotationMode: pdfjsLib.AnnotationMode.ENABLE_FORMS, // render fillable fields
});
linkService.setViewer(viewer);

let pdfDocument = null;
let docMeta = { canSave: false, path: '' };
let lastSig = null; // last verification result, for the signature-details modal

// Full-rewrite edits (sanitise, flatten, redact, remove-originals, page ops) drop
// any signature the document carries. These guards keep one from being destroyed
// unawares: signatureWarning is a caveat appended to an action's existing confirm,
// and confirmSignatureLoss is the standalone prompt for actions that don't already
// confirm. Both are silent on an unsigned document, so ordinary edits stay
// frictionless; after the first such edit the result is unsigned, so they don't
// nag on a doc that no longer has a signature to lose.
function isSigned() {
  return !!(lastSig && lastSig.state && lastSig.state !== 'unsigned');
}
function signatureWarning() {
  return isSigned() ? '\n\nThis will also invalidate the document’s existing signature.' : '';
}
function confirmSignatureLoss() {
  return !isSigned() || confirm('This document is signed. Editing it will invalidate the existing signature. Continue?');
}
let originalName = ''; // basename of the opened file, for default export names
let docGen = 0; // bumps on each load so a stale async render/build can bail

let fitPageWidth = 0; // intrinsic width (pts) of the page last fit-to-width
eventBus.on('pagesinit', () => { fitPageWidth = 0; viewer.currentScaleValue = 'page-width'; });
eventBus.on('pagechanging', (e) => {
  all('.pageNum').forEach((i) => { i.value = e.pageNumber; });
  markCurrentThumb(e.pageNumber);
  // In fit-width mode, re-fit to the page now in view so the pages of a mixed-
  // size PDF each fill the width. No-op on a normal PDF (every page shares a
  // width, so the scale never changes) and when the user has manually zoomed.
  if (viewer.currentScaleValue === 'page-width') {
    const vp = viewer.getPageView(e.pageNumber - 1)?.viewport;
    const w = vp ? vp.width / vp.scale : 0;
    if (w && Math.abs(w - fitPageWidth) > 0.5) {
      fitPageWidth = w;
      viewer.currentScaleValue = 'page-width';
    }
  }
});

// --- open / load -------------------------------------------------------------
async function setDocumentFromServer(meta) {
  const gen = ++docGen;
  docMeta = meta;
  if (meta.name && meta.name !== '.') originalName = meta.name;
  clearOverlays();
  let doc;
  try {
    doc = await pdfjsLib.getDocument({ url: '/api/pdf?t=' + Date.now() }).promise;
  } catch (e) {
    toast('could not render the document');
    console.error('pdf load failed', e);
    return;
  }
  if (gen !== docGen) return; // a newer load superseded this one

  pdfDocument = doc;
  viewer.setDocument(pdfDocument);
  linkService.setDocument(pdfDocument, null);

  els.viewerWrap.classList.add('has-doc');
  all('.pageCount').forEach((s) => { s.textContent = '/ ' + pdfDocument.numPages; });
  els.saveBtn.disabled = false;
  els.saveBtn.title = meta.canSave ? 'Save (overwrites ' + meta.path + ')' : 'Save a copy (downloads — opened without a local path)';
  updateBadge(meta.signature);
  // Sidebars are non-essential; a build failure must not break the load.
  buildThumbnails(gen).catch((e) => console.error('thumbnails failed', e));
  buildOutline(gen).catch((e) => console.error('outline failed', e));
}

async function openPath(path) {
  if (!path) return;
  const res = await apiFetch('/api/open', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  });
  if (!res.ok) return toast((await res.json()).error || 'could not open file');
  await setDocumentFromServer(await res.json());
}

async function uploadFile(file) {
  const form = new FormData();
  form.append('file', file);
  const res = await apiFetch('/api/upload', { method: 'POST', body: form });
  if (!res.ok) return toast((await res.json()).error || 'could not open file');
  await setDocumentFromServer(await res.json());
}

async function openURL(url) {
  const res = await apiFetch('/api/open-url', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
  });
  if (!res.ok) return toast((await res.json()).error || 'could not fetch URL');
  originalName = (url.split('/').pop() || '').split('?')[0] || 'document.pdf';
  await setDocumentFromServer(await res.json());
}

// openSmart routes the Open box to a URL fetch or a local path open.
function openSmart(value) {
  if (!value) return;
  if (/^https?:\/\//i.test(value)) openURL(value);
  else openPath(value);
}

// --- save --------------------------------------------------------------------
async function save() {
  if (!pdfDocument) return;
  els.saveBtn.disabled = true;
  try {
    const bytes = await bakedBytes();
    // No local path to overwrite (drag-dropped or opened by URL) — save a copy
    // by downloading it instead.
    if (!docMeta.canSave) {
      downloadBlob(new Blob([bytes], { type: 'application/pdf' }), 'document.pdf');
      return;
    }
    const res = await apiFetch('/api/save', {
      method: 'POST',
      headers: { 'Content-Type': 'application/pdf' },
      body: bytes,
    });
    if (!res.ok) { toast((await res.json()).error || 'save failed'); return; }
    const meta = await res.json();
    updateBadge(meta.signature);
    toast('Saved');
    // If detected fields were baked in, reload so the page shows the stamped
    // text and the transient input widgets are cleared.
    if (overlayFields.length) await setDocumentFromServer(meta);
  } catch (err) {
    toast('save failed: ' + err.message);
  } finally {
    els.saveBtn.disabled = !pdfDocument;
  }
}

// --- signature badge ---------------------------------------------------------
function updateBadge(sig) {
  lastSig = sig;
  const b = els.sigBadge;
  const signers = sig?.signers || [];
  const map = {
    valid:    ['badge-valid', '✓ Untampered'],
    invalid:  ['badge-invalid', '⚠ Modified since signing'],
    unsigned: ['badge-unsigned', 'Unsigned'],
  };
  let [cls, label] = map[sig?.state] || ['badge-none', 'no document'];
  // Deliberately no inline signing time: it may be self-asserted, and the
  // badge can't qualify it. Time + its trust level live in the details modal.
  if (sig?.state === 'valid' && signers.length > 1) label += ' · ' + signers.length + ' signers';
  // Valid signatures, but content rides after the last one, uncovered: the bare
  // "Untampered" overstates it, so tone the badge to caution. (Invalid already
  // dominates; the full explanation is in the details modal.)
  if (sig?.state === 'valid' && sig?.addedAfter) { cls = 'badge-warn'; label += ' · content added after signing'; }
  b.className = 'badge ' + cls;
  b.textContent = label;
  b.title = label;
  // Details only exist for a signed document (valid or modified).
  els.sigDetailsBtn.hidden = !signers.length;
}

// timeLabel turns a signer's time backing into honest plain English.
function timeLabel(s) {
  if (s.timeBacking === 'tsa') return 'Timestamped ' + s.when + ' by an independent timestamp authority';
  if (s.timeBacking === 'self-asserted') return 'Dated ' + s.when + ' by the signer\'s own device';
  return 'No signing time recorded';
}

async function openSigDetails() {
  const signers = lastSig?.signers || [];
  if (!signers.length) return;
  const body = els.sigDetailsBody;
  body.innerHTML = '';
  const rows = signers.map((s) => {
    const row = document.createElement('div');
    row.className = 'sigrow';

    const who = document.createElement('div');
    who.className = 'sigrow-who';
    who.textContent = s.name || 'Unnamed signer';
    row.appendChild(who);

    const status = document.createElement('div');
    status.className = s.valid ? 'sigrow-ok' : 'sigrow-bad';
    status.textContent = s.valid ? '✓ Untampered' : '⚠ Modified since signing';
    row.appendChild(status);

    const time = document.createElement('div');
    time.className = 'sigrow-time';
    time.textContent = timeLabel(s);
    row.appendChild(time);

    body.appendChild(row);
    return row;
  });
  // Document-level caution: content in a revision after the last signature is
  // covered by none. Shown whatever the per-signer verdicts (it's about the
  // whole file, not one signer); the signatures themselves stay valid.
  if (lastSig?.addedAfter) {
    const note = document.createElement('div');
    note.className = 'signote';
    note.textContent = '⚠ Content was added after the last signature — it is not covered by any signature.';
    body.appendChild(note);
  }
  els.sigDetailsModal.hidden = false;
  augmentSigDetails(rows);
}

// augmentSigDetails fetches the co-signing attestations and adds, per signer that
// carries one, what they accept + whether it cross-binds to a real co-signer's
// key + whether the viewer has pinned them. The parse + cross-binding are done in
// Go (p2p); this only renders. Wording stays key-level — it confirms each party
// attests to the other's fingerprint, not a CA-vouched identity.
async function augmentSigDetails(rows) {
  let atts;
  try {
    const res = await apiFetch('/api/attestations');
    if (!res.ok) return;
    atts = (await res.json()).attestations || [];
  } catch { return; }
  const attested = [];
  atts.forEach((a, i) => {
    if (!a.acceptedPeer || !rows[i]) return;
    attested.push(a);
    const box = document.createElement('div');
    box.className = 'sigatt';
    const what = document.createElement('div');
    what.textContent = a.reason.replace(/\[SPKI:([0-9a-fA-F]{64})\]/, (_, h) => '[' + groupFingerprint(h.slice(0, 8)) + '…]');
    box.appendChild(what);
    const verdict = document.createElement('div');
    if (a.matched) {
      verdict.className = 'sigatt-ok';
      verdict.textContent = '✓ Accepts a co-signer of this document' + (a.pinned ? ', whom you have pinned' : '');
    } else {
      // Not matched covers both an absent peer and one whose signature is invalid,
      // so the wording must be true in either case (the broken signer, if any, is
      // already flagged red on its own row).
      verdict.textContent = 'The accepted peer is not a confirmed co-signer of this document';
    }
    box.appendChild(verdict);
    rows[i].appendChild(box);
  });
  if (attested.length >= 2 && attested.every((a) => a.matched)) {
    const m = document.createElement('div');
    m.className = 'sigmutual';
    m.textContent = '✓ Mutually co-signed — each party’s signature attests to the other’s key.';
    els.sigDetailsBody.appendChild(m);
  }
  if (attested.length) {
    const note = document.createElement('div');
    note.className = 'sigatt';
    note.textContent = 'These attestations are read from the signatures themselves — the authoritative source. The block printed on the page is a human-readable convenience and could be altered by another tool.';
    els.sigDetailsBody.appendChild(note);
  }
}
els.sigDetailsBtn.onclick = openSigDetails;
els.sigDetailsClose.onclick = () => { els.sigDetailsModal.hidden = true; };

// --- hidden-content scan -----------------------------------------------------
// Scan the open document for active/hidden content (auto-run hooks, scripts,
// risky links, attachments, layers) and offer three removal methods, strongest
// fidelity-preserving first with a guaranteed-inert flatten as the floor.
const SEV_LABEL = { high: 'High', medium: 'Medium', low: 'Low' };
const SEV_ORDER = { high: 0, medium: 1, low: 2 };

function renderScanReport(rep) {
  const body = els.scanBody;
  body.innerHTML = '';
  const findings = (rep.findings || []).slice().sort(
    (a, b) => (SEV_ORDER[a.severity] ?? 3) - (SEV_ORDER[b.severity] ?? 3));
  if (!findings.length) {
    const p = document.createElement('p');
    p.className = 'scan-empty';
    p.textContent = '✓ No active or hidden content found — but scanning can’t catch deliberately hidden data; only Flatten is certain to remove it.';
    body.appendChild(p);
    return;
  }
  for (const f of findings) {
    const row = document.createElement('div');
    row.className = 'sigrow';
    const sev = document.createElement('div');
    sev.className = 'scan-sev ' + f.severity;
    sev.textContent = SEV_LABEL[f.severity] || f.severity;
    const detail = document.createElement('div');
    detail.className = 'scan-detail';
    detail.textContent = f.detail;
    const where = document.createElement('div');
    where.className = 'scan-where';
    where.textContent = f.page ? 'Page ' + f.page : 'Whole document';
    row.append(sev, detail, where);
    body.appendChild(row);
  }
}

async function openScan() {
  if (!pdfDocument) return toast('Open a PDF first');
  els.scanBody.innerHTML = '<p class="scan-where">Scanning…</p>';
  els.scanModal.hidden = false;
  try {
    const res = await apiFetch('/api/scan');
    if (!res.ok) throw new Error('scan');
    renderScanReport(await res.json());
  } catch { els.scanModal.hidden = true; toast('scan failed'); }
}
els.scanBtn.onclick = openScan;
els.scanClose.onclick = () => { els.scanModal.hidden = true; };

// runSanitize applies a server-side removal (strip/safe). On success it reloads
// the cleaned document and shows what remains; on failure it leaves the document
// untouched and points to the next, more thorough method.
async function runSanitize(method, stepDown) {
  if (!pdfDocument) return;
  if (!confirmSignatureLoss()) return;
  const res = await apiFetch('/api/sanitize?method=' + method, { method: 'POST' });
  if (!res.ok) return toast('removal failed');
  const out = await res.json();
  if (!out.ok) return toast('Could not cleanly remove it — try ' + stepDown + '.');
  await setDocumentFromServer(out);
  renderScanReport(out.residual);
  const left = (out.residual.findings || []).length;
  toast(left ? 'Cleaned — ' + left + ' item(s) remain; Flatten removes the rest'
             : 'Cleaned — nothing hidden remains');
}
els.scanStripBtn.onclick = () => runSanitize('strip', 'Remove files & media, or Flatten');
els.scanSafeBtn.onclick = () => runSanitize('safe', 'Flatten');

// Flatten is the guaranteed-inert floor: rasterise every page and load the
// flattened result back as the open document.
els.scanFlattenBtn.onclick = async () => {
  if (!pdfDocument) return;
  if (!confirmSignatureLoss()) return;
  const pages = await renderFilledPages(2);
  const form = new FormData();
  pages.forEach((p, i) => {
    form.append('image', p.blob, `page-${i + 1}.png`);
    form.append('pageW', String(p.w));
    form.append('pageH', String(p.h));
  });
  form.append('format', 'pdf');
  form.append('reload', '1');
  const res = await apiFetch('/api/assemble', { method: 'POST', body: form });
  if (!res.ok) return toast('flatten failed');
  await setDocumentFromServer(await res.json());
  renderScanReport({ findings: [] });
  toast('Flattened — document is now inert images');
};

// --- thumbnails sidebar ------------------------------------------------------
async function buildThumbnails(gen = docGen) {
  els.thumbGrid.innerHTML = '';
  for (let n = 1; n <= pdfDocument.numPages; n++) {
    if (gen !== docGen) return; // a newer document loaded — stop rendering stale thumbs
    const page = await pdfDocument.getPage(n);
    const base = page.getViewport({ scale: 1 });
    const viewport = page.getViewport({ scale: 150 / base.width });

    const wrap = document.createElement('div');
    wrap.className = 'thumbwrap';
    wrap.dataset.page = n;
    wrap.draggable = true;
    const canvas = document.createElement('canvas');
    canvas.className = 'thumb';
    canvas.width = viewport.width;
    canvas.height = viewport.height;
    canvas.onclick = () => { viewer.currentPageNumber = n; };

    const acts = document.createElement('div');
    acts.className = 'thumbacts';
    const rot = document.createElement('button'); rot.textContent = '↻'; rot.title = 'Rotate';
    rot.onclick = (e) => { e.stopPropagation(); pageOp('rotate', { pages: String(n), deg: 90 }); };
    const del = document.createElement('button'); del.textContent = '×'; del.title = 'Delete page';
    del.onclick = (e) => { e.stopPropagation(); if (pdfDocument.numPages > 1) pageOp('delete', { pages: String(n) }); };
    acts.append(rot, del);

    const label = document.createElement('div');
    label.className = 'thumb-label';
    label.textContent = n;

    wrap.append(canvas, acts, label);
    els.thumbGrid.appendChild(wrap);
    await page.render({ canvasContext: canvas.getContext('2d'), viewport }).promise;
  }
  markCurrentThumb(viewer.currentPageNumber || 1);
}

function markCurrentThumb(n) {
  els.thumbGrid.querySelectorAll('.thumbwrap').forEach((c) => {
    c.classList.toggle('current', Number(c.dataset.page) === n);
  });
}

// --- thumbnail drag-to-reorder ----------------------------------------------
// Dragging a thumbnail live-moves it among its siblings; on drop, the new
// thumbnail order (each .thumbwrap's original page number) is sent as a reorder.
// A cancelled drag restores the original DOM order so a thumbnail's click target
// never diverges from its position.
let dragSrc = null;      // the .thumbwrap being dragged
let dragOrig = null;     // snapshot of the grid's child order, for cancel revert
let dragDropped = false; // a real drop committed this drag

function onThumbDragStart(e) {
  const wrap = e.target.closest('.thumbwrap');
  if (!wrap) return;
  dragSrc = wrap;
  dragOrig = [...els.thumbGrid.children];
  dragDropped = false;
  wrap.classList.add('dragging');
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/plain', wrap.dataset.page); // payload required to drag in Firefox
}

function onThumbDragOver(e) {
  if (!dragSrc) return;
  e.preventDefault(); // allow drop
  e.dataTransfer.dropEffect = 'move';
  const after = thumbBelow(e.clientY);
  if (after) els.thumbGrid.insertBefore(dragSrc, after);
  else els.thumbGrid.appendChild(dragSrc);
}

// thumbBelow returns the first thumbnail whose vertical midpoint is below the
// cursor (where the dragged thumbnail should be inserted before), or null for end.
function thumbBelow(y) {
  return [...els.thumbGrid.querySelectorAll('.thumbwrap:not(.dragging)')].find((w) => {
    const r = w.getBoundingClientRect();
    return y < r.top + r.height / 2;
  }) || null;
}

function onThumbDrop(e) {
  if (!dragSrc) return;
  e.preventDefault();
  dragDropped = true;
  const order = [...els.thumbGrid.querySelectorAll('.thumbwrap')].map((w) => w.dataset.page);
  if (order.join(',') !== dragOrig.map((w) => w.dataset.page).join(',')) {
    pageOp('reorder', { pages: order.join(',') });
  }
}

function onThumbDragEnd() {
  if (dragSrc) dragSrc.classList.remove('dragging');
  if (!dragDropped && dragOrig) dragOrig.forEach((w) => els.thumbGrid.appendChild(w)); // cancelled — restore order
  dragSrc = null;
  dragOrig = null;
  dragDropped = false;
}

els.thumbGrid.addEventListener('dragstart', onThumbDragStart);
els.thumbGrid.addEventListener('dragover', onThumbDragOver);
els.thumbGrid.addEventListener('drop', onThumbDrop);
els.thumbGrid.addEventListener('dragend', onThumbDragEnd);

// --- page operations (M7): bake edits, apply server-side, reload -------------
async function pageOp(op, extra = {}) {
  if (!pdfDocument) return;
  if (!confirmSignatureLoss()) return;
  const bytes = await bakedBytes();
  const form = new FormData();
  form.append('pdf', new Blob([bytes], { type: 'application/pdf' }), 'doc.pdf');
  form.append('op', op);
  if (extra.pages) form.append('pages', extra.pages);
  if (extra.deg != null) form.append('deg', String(extra.deg));
  if (extra.file) form.append('append', extra.file, 'append.pdf');
  const res = await apiFetch('/api/pages', { method: 'POST', body: form });
  if (!res.ok) return toast('page operation failed');
  await setDocumentFromServer(await res.json());
}

els.appendBtn.onclick = () => els.appendInput.click();
els.appendInput.onchange = () => {
  if (els.appendInput.files[0]) pageOp('append', { file: els.appendInput.files[0] });
  els.appendInput.value = '';
};

// --- outline sidebar ---------------------------------------------------------
async function buildOutline(gen = docGen) {
  els.outline.innerHTML = '';
  const outline = await pdfDocument.getOutline();
  if (gen !== docGen) return; // a newer document loaded — drop this stale outline
  if (!outline || !outline.length) {
    els.outline.innerHTML = '<div class="thumb-label">No outline</div>';
    return;
  }
  const render = (items, depth) => {
    for (const it of items) {
      const a = document.createElement('a');
      a.textContent = it.title;
      a.style.paddingLeft = 4 + depth * 12 + 'px';
      a.onclick = () => linkService.goToDestination(it.dest);
      els.outline.appendChild(a);
      if (it.items?.length) render(it.items, depth + 1);
    }
  };
  render(outline, 0);
}

// --- image library + stamping (M3) -------------------------------------------
// A placed image/quick-stamp becomes a Nib overlay widget — draggable and
// resizable — baked server-side by pdfops (StampImages) at save/flatten, the
// same coordinate-accurate pipeline as the auto-detected fields. We do NOT use
// pdf.js's STAMP editor: its saveDocument() baking placed stamps a few points
// too high (the line/letter no longer matched). bitmapUrl is a library image
// (/api/images/{id}) or a data: URL for a generated stamp.
async function placeStamp(bitmapUrl) {
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  const n = viewer.currentPageNumber;
  const pv = viewer.getPageView(n - 1);
  if (!pv?.div || !pv.viewport) { toast('Scroll the page into view, then try again'); return; }
  const base = (await pdfDocument.getPage(n)).getViewport({ scale: 1 }); // PDF points
  const img = new Image();
  img.onload = () => {
    const W = pv.div.clientWidth, H = pv.div.clientHeight;
    const aspect = (img.naturalWidth / img.naturalHeight) || 1;
    const dispW = Math.min(W * 0.3, img.naturalWidth || W * 0.3);
    const dispH = dispW / aspect;
    const x = (W - dispW) / 2, y = (H - dispH) / 2;
    const frac = [x / W, y / H, (x + dispW) / W, (y + dispH) / H];
    makeStamp(bitmapUrl, aspect, frac, { page: n, pageW: base.width, pageH: base.height }, pv);
  };
  img.onerror = () => toast('could not load image');
  img.src = bitmapUrl;
}

// makeStamp registers a draggable/resizable image overlay (kind 'stamp'). The
// baking source is the library id (server resolves bytes) or an inline base64
// PNG for a generated stamp.
function makeStamp(src, aspect, frac, opts, pv) {
  const f = { page: opts.page, frac, pageW: opts.pageW, pageH: opts.pageH, kind: 'stamp', aspect };
  if (src.startsWith('data:')) f.png = src.slice(src.indexOf(',') + 1);
  else { const m = src.match(/\/api\/images\/([^/?#]+)/); if (m) f.imageId = m[1]; }

  const el = document.createElement('div');
  el.className = 'ovl ovl-stamp';
  el.tabIndex = 0;
  const img = document.createElement('img');
  img.src = src; img.alt = '';
  const handle = document.createElement('span');
  handle.className = 'stamp-resize';
  const del = document.createElement('button');
  del.className = 'stamp-del'; del.textContent = '×'; del.title = 'Remove stamp';
  el.append(img, handle, del);
  f.el = el;

  const remove = () => { el.remove(); overlayFields = overlayFields.filter((o) => o !== f); };
  del.onclick = (e) => { e.stopPropagation(); remove(); };
  el.addEventListener('keydown', (e) => { if (e.key === 'Delete' || e.key === 'Backspace') remove(); });
  enableStampGestures(f, el, handle);

  overlayFields.push(f);
  pv.div.appendChild(el);
  layoutField(f, pv);
}

// enableStampGestures wires move (anywhere on the stamp) and resize (the corner
// handle), updating the field's page-fraction rect. Pointer math is in page-div
// pixels; resize preserves the image aspect and everything clamps to the page.
function enableStampGestures(f, el, handle) {
  let mode = null, sx = 0, sy = 0, start = null;
  const begin = (m) => (e) => {
    if (m === 'drag' && e.target.closest('.stamp-resize, .stamp-del')) return;
    e.preventDefault(); e.stopPropagation();
    mode = m; sx = e.clientX; sy = e.clientY; start = f.frac.slice();
    el.setPointerCapture(e.pointerId);
  };
  el.addEventListener('pointerdown', begin('drag'));
  handle.addEventListener('pointerdown', begin('resize'));
  el.addEventListener('pointermove', (e) => {
    if (!mode) return;
    const pv = viewer.getPageView(f.page - 1);
    if (!pv?.div) return;
    const W = pv.div.clientWidth, H = pv.div.clientHeight;
    const dx = (e.clientX - sx) / W, dy = (e.clientY - sy) / H;
    let [x0, y0, x1, y1] = start;
    if (mode === 'drag') {
      const w = x1 - x0, h = y1 - y0;
      x0 = Math.min(Math.max(x0 + dx, 0), 1 - w);
      y0 = Math.min(Math.max(y0 + dy, 0), 1 - h);
      f.frac = [x0, y0, x0 + w, y0 + h];
    } else {
      let nw = Math.max((x1 - x0) + dx, 12 / W);
      let nh = nw * W / (f.aspect * H); // keep image aspect (page-pixel terms)
      const k = Math.min(1, (1 - x0) / nw, (1 - y0) / nh); // clamp, preserve aspect
      f.frac = [x0, y0, x0 + nw * k, y0 + nh * k];
    }
    layoutField(f, pv);
  });
  el.addEventListener('pointerup', (e) => { mode = null; try { el.releasePointerCapture(e.pointerId); } catch { /* already released */ } });
}

async function loadImages() {
  const res = await apiFetch('/api/images');
  if (!res.ok) return;
  libraryImages = await res.json();
  els.imageGrid.innerHTML = '';
  for (const m of libraryImages) {
    const card = document.createElement('div');
    card.className = 'libimg';
    card.title = 'Place ' + m.name;
    const img = document.createElement('img');
    img.src = '/api/images/' + m.id;
    const name = document.createElement('div');
    name.className = 'name';
    name.textContent = m.name;
    card.append(img, name);
    card.onclick = (e) => { if (!e.target.closest('.del')) placeStamp('/api/images/' + m.id); };
    // Built-in (binary-shipped) signatures are read-only — no delete control.
    if (!m.builtin) {
      const del = document.createElement('button');
      del.className = 'del';
      del.textContent = '×';
      del.title = 'Delete';
      del.onclick = async (e) => {
        e.stopPropagation();
        const r = await apiFetch('/api/images/' + m.id, { method: 'DELETE' });
        if (r.ok || r.status === 204) loadImages();
      };
      card.append(del);
    }
    els.imageGrid.appendChild(card);
  }
}

els.addImageBtn.onclick = () => els.addImageInput.click();
els.addImageInput.onchange = async () => {
  const file = els.addImageInput.files[0];
  els.addImageInput.value = '';
  if (file) openBgModal(file);
};

// Uploaded-image background removal: a scanned/photographed signature sits on
// opaque paper, which stamps as an ugly white box. Knock the near-white
// background out to transparency client-side, preview it, then upload the
// result through the same /api/images path the draw pad uses.
let bgSrc = null; // { bitmap, file, name, w, h }
async function openBgModal(file) {
  const bitmap = await createImageBitmap(file);
  bgSrc = { bitmap, file, name: file.name, w: bitmap.width, h: bitmap.height };
  els.bgRemove.checked = !sourceHasAlpha(); // don't re-knock-out an already-transparent PNG
  els.bgThresh.value = 200;
  els.bgModal.hidden = false;
  renderBgPreview();
}
function sourceHasAlpha() {
  const { bitmap, w, h } = bgSrc;
  const cv = document.createElement('canvas');
  cv.width = w; cv.height = h;
  const ctx = cv.getContext('2d');
  ctx.drawImage(bitmap, 0, 0);
  const d = ctx.getImageData(0, 0, w, h).data;
  let transparent = 0;
  for (let i = 3; i < d.length; i += 4) if (d[i] < 250) transparent++;
  return transparent > (d.length / 4) * 0.05; // >5% already-transparent pixels
}
function renderBgPreview() {
  const { bitmap, w, h } = bgSrc;
  const cv = els.bgCanvas;
  cv.width = w; cv.height = h;
  const ctx = cv.getContext('2d');
  ctx.clearRect(0, 0, w, h);
  ctx.drawImage(bitmap, 0, 0);
  els.bgThreshRow.style.display = els.bgRemove.checked ? '' : 'none';
  if (els.bgRemove.checked) knockoutBackground(ctx, w, h, Number(els.bgThresh.value));
}
// Map luminance to alpha: pixels brighter than `threshold` go fully transparent,
// a soft band below it ramps alpha so antialiased edges stay smooth; original RGB
// (and any existing alpha) is preserved so colored ink survives.
//
// TRIPWIRE: this heuristic has no automated guard (no JS test harness in this repo;
// the risky part is the modal/upload wiring, which is integration, not pure math).
// It's verified by the live preview at use time. After changing this function,
// renderBgPreview, or the bgModal upload path, re-check by hand: upload a dark-ink-
// on-white-paper image and confirm the preview shows a transparent background with
// the ink intact, then Save and confirm the library thumbnail is still transparent.
function knockoutBackground(ctx, w, h, threshold) {
  const img = ctx.getImageData(0, 0, w, h);
  const d = img.data;
  const band = 40;
  const lo = threshold - band;
  for (let i = 0; i < d.length; i += 4) {
    const lum = 0.299 * d[i] + 0.587 * d[i + 1] + 0.114 * d[i + 2];
    let a;
    if (lum >= threshold) a = 0;
    else if (lum <= lo) a = 255;
    else a = Math.round(255 * (threshold - lum) / band);
    d[i + 3] = Math.round(d[i + 3] * a / 255);
  }
  ctx.putImageData(img, 0, 0);
}
els.bgRemove.onchange = renderBgPreview;
els.bgThresh.oninput = renderBgPreview;
els.bgCancel.onclick = () => { els.bgModal.hidden = true; bgSrc = null; };
els.bgSave.onclick = async () => {
  const removing = els.bgRemove.checked;
  const blob = removing
    ? await new Promise((r) => els.bgCanvas.toBlob(r, 'image/png'))
    : bgSrc.file; // keep the original (and its format) untouched
  const form = new FormData();
  form.append('file', blob, removing ? 'signature.png' : bgSrc.name);
  form.append('name', bgSrc.name);
  const res = await apiFetch('/api/images', { method: 'POST', body: form });
  if (res.ok) { els.bgModal.hidden = true; bgSrc = null; loadImages(); }
  else toast((await res.json()).error || 'could not add image');
};

// Quick-stamps: render a small bitmap and place it via the same stamp path.
document.querySelectorAll('.quickstamps button').forEach((b) => {
  b.onclick = () => placeStamp(quickStampURL(b.dataset.stamp));
});
function quickStampURL(kind) {
  const cv = document.createElement('canvas');
  const ctx = cv.getContext('2d');
  if (kind === 'check') {
    const s = 3; // supersample for a crisp stroke, matching shapePNG's pen
    cv.width = cv.height = 64 * s;
    ctx.strokeStyle = INK;
    ctx.lineWidth = inkWidth(s); ctx.lineCap = 'round'; ctx.lineJoin = 'round';
    ctx.beginPath(); ctx.moveTo(12 * s, 34 * s); ctx.lineTo(26 * s, 50 * s); ctx.lineTo(54 * s, 12 * s); ctx.stroke();
  } else {
    const text = kind === 'date' ? new Date().toLocaleDateString() : 'APPROVED';
    ctx.font = 'bold 36px sans-serif';
    cv.width = ctx.measureText(text).width + 24;
    cv.height = 56;
    ctx.font = 'bold 36px sans-serif'; // resizing the canvas resets the context
    ctx.fillStyle = '#c1121f'; ctx.textBaseline = 'middle';
    ctx.fillText(text, 12, 30);
  }
  return cv.toDataURL('image/png');
}

// Signature draw pad → transparent PNG saved to the library.
let sigCtx = null;
let sigDrawing = false;
function sigPoint(e) {
  const r = els.sigCanvas.getBoundingClientRect();
  return {
    x: (e.clientX - r.left) * els.sigCanvas.width / r.width,
    y: (e.clientY - r.top) * els.sigCanvas.height / r.height,
  };
}
els.drawSigBtn.onclick = () => {
  els.sigModal.hidden = false;
  sigCtx = els.sigCanvas.getContext('2d');
  sigCtx.clearRect(0, 0, els.sigCanvas.width, els.sigCanvas.height);
  sigCtx.strokeStyle = '#111'; sigCtx.lineWidth = 3; sigCtx.lineCap = 'round'; sigCtx.lineJoin = 'round';
};
els.sigCanvas.addEventListener('pointerdown', (e) => {
  sigDrawing = true;
  const p = sigPoint(e);
  sigCtx.beginPath(); sigCtx.moveTo(p.x, p.y);
  els.sigCanvas.setPointerCapture(e.pointerId);
});
els.sigCanvas.addEventListener('pointermove', (e) => {
  if (!sigDrawing) return;
  const p = sigPoint(e);
  sigCtx.lineTo(p.x, p.y); sigCtx.stroke();
});
els.sigCanvas.addEventListener('pointerup', () => { sigDrawing = false; });
els.sigClear.onclick = () => sigCtx.clearRect(0, 0, els.sigCanvas.width, els.sigCanvas.height);
els.sigCancel.onclick = () => { els.sigModal.hidden = true; };
els.sigSave.onclick = () => {
  els.sigCanvas.toBlob(async (blob) => {
    const form = new FormData();
    form.append('file', blob, 'signature.png');
    form.append('name', 'Signature');
    const res = await apiFetch('/api/images', { method: 'POST', body: form });
    els.sigModal.hidden = true;
    if (res.ok) { loadImages(); toast('Signature saved'); } else toast('could not save signature');
  }, 'image/png');
};

// --- flatten / export / finalize / autofill (M5) -----------------------------
function downloadBlob(blob, name) {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = name;
  // The anchor must be in the document for the click to trigger a download in
  // some browsers and in Chromium's --app window (which has no download shelf).
  a.style.display = 'none';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  toast('Downloaded ' + name + ' (check your Downloads folder)');
}

// --- save / export destination dialog ----------------------------------------
// Flatten, editable save, finalize, and the ZIP/PNG exports all route their
// output through openSaveAs: the user picks a name and folder (default ~/nib)
// and the server writes the bytes there. The browser can't target an arbitrary
// folder on its own, so this replaces the old ~/Downloads browser-download.

// exportBase is the opened file's name without its directory or .pdf suffix,
// used to build defaults like "<base>-flattened.pdf".
function exportBase() {
  const b = (originalName || docMeta.name || 'document').replace(/\.[Pp][Dd][Ff]$/, '');
  return b || 'document';
}

// joinPath appends a name to a directory, collapsing any trailing slash.
const joinPath = (dir, name) => dir.replace(/\/+$/, '') + '/' + name;

let saveAsBlob = null; // the bytes the dialog will write on confirm

async function browseDir(path) {
  const res = await apiFetch('/api/listdir' + (path ? '?path=' + encodeURIComponent(path) : ''));
  if (!res.ok) return;
  const info = await res.json();
  els.saveAsDir.value = info.path;
  els.saveAsHere.textContent = info.path;
  els.saveAsUp.disabled = !info.parent;
  els.saveAsUp.dataset.parent = info.parent || '';
  els.saveAsList.innerHTML = '';
  for (const d of info.dirs) {
    const li = document.createElement('li');
    li.textContent = d;
    li.onclick = () => browseDir(joinPath(info.path, d));
    els.saveAsList.appendChild(li);
  }
}

function openSaveAs(blob, defaultName, title) {
  saveAsBlob = blob;
  els.saveAsTitle.textContent = title || 'Save';
  els.saveAsName.value = defaultName;
  els.saveAsModal.hidden = false;
  els.saveAsName.focus();
  els.saveAsName.select();
  browseDir(''); // server resolves the empty path to the ~/nib default
}

els.saveAsCancel.onclick = () => { els.saveAsModal.hidden = true; saveAsBlob = null; };
els.saveAsUp.onclick = () => { const p = els.saveAsUp.dataset.parent; if (p) browseDir(p); };
els.saveAsDir.onchange = () => browseDir(els.saveAsDir.value.trim());
els.saveAsGo.onclick = async () => {
  if (!saveAsBlob) return;
  const name = els.saveAsName.value.trim();
  const dir = els.saveAsDir.value.trim().replace(/\/+$/, '');
  if (!name) return toast('Enter a file name');
  if (!dir) return toast('Choose a folder');
  const form = new FormData();
  form.append('path', dir + '/' + name);
  form.append('data', saveAsBlob, name);
  const res = await apiFetch('/api/write', { method: 'POST', body: form });
  if (!res.ok) { toast((await res.json()).error || 'could not save'); return; }
  const meta = await res.json();
  els.saveAsModal.hidden = true;
  saveAsBlob = null;
  toast('Saved to ' + meta.path);
};

// renderPageBlob rasterises one page of an already-parsed doc to a PNG at the
// given scale, runs the optional paint hook over the canvas after rendering (used
// to burn redaction boxes in), and returns the blob plus the page's true point
// size. The shared atom behind renderFilledPages and flattenPages.
async function renderPageBlob(doc, n, scale, paint) {
  const page = await doc.getPage(n);
  const base = page.getViewport({ scale: 1 }); // points: the page's true physical size
  const vp = page.getViewport({ scale });
  const cv = document.createElement('canvas');
  cv.width = vp.width; cv.height = vp.height;
  const ctx = cv.getContext('2d');
  await page.render({ canvasContext: ctx, viewport: vp, annotationMode: pdfjsLib.AnnotationMode.ENABLE }).promise;
  if (paint) paint(ctx, cv, n);
  const blob = await new Promise((r) => cv.toBlob(r, 'image/png'));
  return { blob, w: base.width, h: base.height };
}

// renderFilledPages rasterises the saved (form-filled, stamped) document so the
// raster reflects every edit. Used for flatten and image export.
async function renderFilledPages(scale, onlyPage) {
  const bytes = await bakedBytes();
  const doc = await pdfjsLib.getDocument({ data: bytes }).promise;
  const pages = [];
  const from = onlyPage || 1;
  const to = onlyPage || doc.numPages;
  for (let n = from; n <= to; n++) pages.push(await renderPageBlob(doc, n, scale));
  return pages;
}

// flattenPages rasterises the given pages of the saved document and posts them to
// /api/redact, which swaps each for a flat image — destroying the vector content
// underneath. The shared engine behind "Apply redactions" and "Remove originals";
// the optional paint hook lets redaction burn its black boxes in before the snapshot.
async function flattenPages(pages, paint) {
  const bytes = await bakedBytes();
  // pdf.js detaches the buffer it parses, but `bytes` is also uploaded as the pdf
  // field, so render from a copy and keep the original intact for the upload.
  const doc = await pdfjsLib.getDocument({ data: bytes.slice() }).promise;
  const form = new FormData();
  form.append('pdf', new Blob([bytes], { type: 'application/pdf' }), 'doc.pdf');
  for (const n of pages) {
    const { blob, w, h } = await renderPageBlob(doc, n, 2, paint);
    form.append('page', blob, `page-${n}.png`);
    form.append('pageNum', String(n));
    form.append('pageW', String(w));
    form.append('pageH', String(h));
  }
  return apiFetch('/api/redact', { method: 'POST', body: form });
}

// assembleBlob rasterises every (filled, stamped) page and packages it server-
// side into a flattened image-PDF or a ZIP of PNGs. Returns the blob, or null on
// failure.
async function assembleBlob(format) {
  const pages = await renderFilledPages(2);
  const form = new FormData();
  pages.forEach((p, i) => {
    form.append('image', p.blob, `page-${i + 1}.png`);
    form.append('pageW', String(p.w));
    form.append('pageH', String(p.h));
  });
  form.append('format', format);
  const res = await apiFetch('/api/assemble', { method: 'POST', body: form });
  if (!res.ok) { toast('export failed'); return null; }
  return res.blob();
}

els.saveFlatBtn.onclick = async () => {
  if (!pdfDocument) return toast('Open a PDF first');
  const blob = await assembleBlob('pdf');
  if (blob) openSaveAs(blob, exportBase() + '-flattened.pdf', 'Save flattened PDF');
};
els.exportZipBtn.onclick = async () => {
  if (!pdfDocument) return toast('Open a PDF first');
  const blob = await assembleBlob('zip');
  if (blob) openSaveAs(blob, exportBase() + '-pages.zip', 'Export pages (ZIP)');
};

els.saveEditableBtn.onclick = async () => {
  if (!pdfDocument) return;
  const bytes = await bakedBytes();
  openSaveAs(new Blob([bytes], { type: 'application/pdf' }), exportBase() + '-editable.pdf', 'Save editable copy');
};

els.exportPngBtn.onclick = async () => {
  if (!pdfDocument) return;
  const [{ blob }] = await renderFilledPages(2, viewer.currentPageNumber);
  openSaveAs(blob, exportBase() + '-page' + viewer.currentPageNumber + '.png', 'Export page (PNG)');
};

els.exportFormJsonBtn.onclick = () => { window.location = '/api/form-data?format=json'; };
els.exportFormCsvBtn.onclick = () => { window.location = '/api/form-data?format=csv'; };
els.exportCertBtn.onclick = () => { window.location = '/api/identity'; };

// Finalize & sign.
els.finalizeBtn.onclick = () => { if (pdfDocument) els.finalizeModal.hidden = false; };
els.fzCancel.onclick = () => { els.finalizeModal.hidden = true; };
// The timestamp URL is opt-in: the field stays disabled until the box is ticked.
els.fzTsaOn.onchange = () => { els.fzTsa.disabled = !els.fzTsaOn.checked; if (els.fzTsaOn.checked) els.fzTsa.focus(); };
// Presets set the label text and its "ink" (colour / opacity); angle and size
// stay as independent geometry. Most labels are a faint mark; VOID is bold red,
// since it negates the document. The watermark always draws on top of the page.
const WM_PRESETS = {
  DRAFT: { color: '#8a8a8a', opacity: 10 },
  CONFIDENTIAL: { color: '#8a8a8a', opacity: 10 },
  FINALIZED: { color: '#8a8a8a', opacity: 10 },
  COPY: { color: '#8a8a8a', opacity: 10 },
  VOID: { color: '#cc0000', opacity: 65 },
};
// drawPreview reflects the current controls onto the in-dialog mock page. It is a
// faithful approximation of pdfcpu's render, not a pixel-exact copy.
function drawPreview() {
  const m = els.fzPreviewMark;
  m.textContent = els.fzText.value.trim() || 'WATERMARK';
  m.style.color = els.fzColor.value;
  m.style.opacity = els.fzOpacity.value / 100;
  m.style.fontSize = 8 + (els.fzSize.value / 100) * 32 + 'px';
  m.style.transform = `translate(-50%, -50%) rotate(${-els.fzAngle.value}deg)`;
}
const syncWmPresets = () => all('.wmpreset').forEach((b) => b.classList.toggle('active', b.dataset.wm === els.fzText.value));
all('.wmpreset').forEach((b) => {
  b.onclick = () => {
    els.fzText.value = b.dataset.wm;
    const s = WM_PRESETS[b.dataset.wm];
    if (s) { els.fzColor.value = s.color; els.fzOpacity.value = s.opacity; }
    syncWmPresets();
    drawPreview();
  };
});
[els.fzText, els.fzOpacity, els.fzSize, els.fzAngle, els.fzColor].forEach((el) => {
  el.addEventListener('input', () => { syncWmPresets(); drawPreview(); });
});
drawPreview();
els.fzGo.onclick = async () => {
  els.finalizeModal.hidden = true;
  let text = els.fzText.value.trim();
  if (text && els.fzDate.checked) text += ' ' + new Date().toLocaleDateString();

  const bytes = await bakedBytes();
  const form = new FormData();
  form.append('pdf', new Blob([bytes], { type: 'application/pdf' }), 'doc.pdf');
  form.append('params', JSON.stringify({
    reason: 'Finalized in Nib',
    watermark: {
      text,
      color: els.fzColor.value,
      opacity: els.fzOpacity.value / 100,
      scale: els.fzSize.value / 100,
      angle: Number(els.fzAngle.value),
    },
    tsaUrl: els.fzTsaOn.checked ? els.fzTsa.value.trim() : '',
  }));
  const res = await apiFetch('/api/finalize', { method: 'POST', body: form });
  if (!res.ok) { toast('Could not finalize'); return; }
  openSaveAs(await res.blob(), exportBase() + '-finalized.pdf', 'Save finalized PDF');
};

// Autofill: set matching form-field values from the saved profile.
els.autofillBtn.onclick = async () => {
  if (!pdfDocument) return toast('Open a PDF first');
  const res = await apiFetch('/api/profile');
  const profile = res.ok ? await res.json() : {};
  if (!Object.keys(profile).length) return toast('No profile yet — edit it first');
  const objs = await pdfDocument.getFieldObjects();
  if (!objs) return toast('This PDF has no form fields');
  let count = 0;
  for (const [name, arr] of Object.entries(objs)) {
    if (profile[name] === undefined) continue;
    for (const o of arr) { pdfDocument.annotationStorage.setValue(o.id, { value: profile[name] }); count++; }
  }
  viewer.refresh?.();
  toast(count ? `Filled ${count} field(s) — review and Save` : 'No matching field names');
};

// Autofill profile editor (one "name = value" per line).
els.editProfileBtn.onclick = async () => {
  const res = await apiFetch('/api/profile');
  const profile = res.ok ? await res.json() : {};
  els.profileText.value = Object.entries(profile).map(([k, v]) => `${k} = ${v}`).join('\n');
  els.profileModal.hidden = false;
};
els.profileCancel.onclick = () => { els.profileModal.hidden = true; };
els.profileSave.onclick = async () => {
  const profile = {};
  for (const line of els.profileText.value.split('\n')) {
    const i = line.indexOf('=');
    if (i < 0) continue;
    const k = line.slice(0, i).trim();
    if (k) profile[k] = line.slice(i + 1).trim();
  }
  const res = await apiFetch('/api/profile', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(profile),
  });
  els.profileModal.hidden = true;
  toast(res.ok ? 'Profile saved' : 'could not save profile');
};

// --- redaction (M9) ----------------------------------------------------------
// Draw boxes over pages; on Apply, the marked pages are re-rendered with the
// boxes painted in and replaced by those flat images server-side, so the content
// under a box is genuinely removed. Non-marked pages keep their vector text.
let redactMode = false;
let redactMarks = []; // {page, fx, fy, fw, fh} as fractions of the page
let redStart = null, redDiv = null, redHit = null;

els.redactBtn.onclick = () => {
  redactMode = !redactMode;
  reflectRedact();
  els.viewerContainer.style.cursor = redactMode ? 'crosshair' : '';
};
// Keep both the Edit-menu and toolbar redact buttons lit while redact mode is on.
function reflectRedact() {
  all('#redactBtn, [data-forward="redactBtn"]').forEach((b) => b.classList.toggle('active', redactMode));
}

// The pdf.js .page has a transparent border (9px), so its border-box rect is
// larger than the rendered content. Redaction must work in the CONTENT box: that
// is where an absolutely-positioned overlay's origin sits, and what the apply-
// time canvas (rendered from the PDF page) covers. Using the border-box instead
// offsets the live overlay by the border and shifts the baked redaction.
//
// TRIPWIRE: the "box actually covers the secret" offset property has no automated
// guard (it's rendered-coordinate math; a headless-browser pixel test is too
// flaky/heavyweight for this single-binary repo). Before committing any change to
// pageContentRect or the fraction capture/apply path, re-run the manual procedure
// in memory/reference_redaction_visual_check.md.
function pageContentRect(div) {
  const r = div.getBoundingClientRect();
  const cs = getComputedStyle(div);
  const bl = parseFloat(cs.borderLeftWidth) || 0, bt = parseFloat(cs.borderTopWidth) || 0;
  const br = parseFloat(cs.borderRightWidth) || 0, bb = parseFloat(cs.borderBottomWidth) || 0;
  return { left: r.left + bl, top: r.top + bt, width: r.width - bl - br, height: r.height - bt - bb };
}

function pageAt(x, y) {
  for (let i = 0; i < (pdfDocument?.numPages || 0); i++) {
    const pv = viewer.getPageView(i);
    const r = pv?.div?.getBoundingClientRect();
    if (r && x >= r.left && x <= r.right && y >= r.top && y <= r.bottom) {
      return { pv, n: i + 1, r: pageContentRect(pv.div) };
    }
  }
  return null;
}
function sizeMark(div, r, a, b) {
  div.style.left = (Math.min(a.x, b.x) - r.left) + 'px';
  div.style.top = (Math.min(a.y, b.y) - r.top) + 'px';
  div.style.width = Math.abs(b.x - a.x) + 'px';
  div.style.height = Math.abs(b.y - a.y) + 'px';
}
els.viewerContainer.addEventListener('pointerdown', (e) => {
  if (!redactMode) return;
  redHit = pageAt(e.clientX, e.clientY);
  if (!redHit) return;
  redStart = { x: e.clientX, y: e.clientY };
  redDiv = document.createElement('div');
  redDiv.className = 'redactmark';
  redHit.pv.div.appendChild(redDiv);
  sizeMark(redDiv, redHit.r, redStart, redStart);
  e.preventDefault();
});
els.viewerContainer.addEventListener('pointermove', (e) => {
  if (redStart) sizeMark(redDiv, redHit.r, redStart, { x: e.clientX, y: e.clientY });
});
els.viewerContainer.addEventListener('pointerup', (e) => {
  if (!redStart) return;
  const r = redHit.r;
  const x0 = Math.min(redStart.x, e.clientX), y0 = Math.min(redStart.y, e.clientY);
  const fw = Math.abs(e.clientX - redStart.x) / r.width;
  const fh = Math.abs(e.clientY - redStart.y) / r.height;
  if (fw > 0.005 && fh > 0.005) {
    redactMarks.push({ page: redHit.n, fx: (x0 - r.left) / r.width, fy: (y0 - r.top) / r.height, fw, fh });
  } else {
    redDiv.remove();
  }
  redStart = null; redDiv = null; redHit = null;
});

els.applyRedactBtn.onclick = async () => {
  if (!redactMarks.length) return toast('Draw redaction boxes first');
  if (!confirm('Permanently redact the marked pages? Those pages become flat images and the content under each box is removed. This cannot be undone.' + signatureWarning())) return;

  const byPage = {};
  for (const m of redactMarks) (byPage[m.page] ||= []).push(m);
  const res = await flattenPages(Object.keys(byPage).map(Number), (ctx, cv, n) => {
    ctx.fillStyle = '#000';
    for (const m of byPage[n]) ctx.fillRect(m.fx * cv.width, m.fy * cv.height, m.fw * cv.width, m.fh * cv.height);
  });
  if (!res.ok) return toast('redaction failed');
  redactMarks = [];
  redactMode = false;
  reflectRedact();
  els.viewerContainer.style.cursor = '';
  await setDocumentFromServer(await res.json());
  toast('Redacted — affected pages are now flattened images');
};

// --- cover-and-replace text editing ------------------------------------------
// Drag a box over baked-in text; Nib reads the text + its size/colour/font under
// the box, covers it with an opaque background-coloured fill, and drops an
// editable overlay prefilled in the recognized style. On save the cover is baked
// under the replacement text (bakedBytes -> /api/bake). The original text stays
// in the page (a visual edit) until "Remove originals" flattens the edited pages
// (reusing the redaction path), which removes it for good.
let editMode = false;
let edStart = null, edDiv = null, edHit = null;

els.editTextBtn.onclick = () => {
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  editMode = !editMode;
  if (editMode && redactMode) { redactMode = false; reflectRedact(); } // one box tool at a time
  reflectEdit();
  els.viewerContainer.style.cursor = editMode ? 'crosshair' : '';
};
function reflectEdit() {
  all('#editTextBtn, [data-forward="editTextBtn"]').forEach((b) => b.classList.toggle('active', editMode));
}

els.viewerContainer.addEventListener('pointerdown', (e) => {
  if (!editMode) return;
  edHit = pageAt(e.clientX, e.clientY);
  if (!edHit) return;
  edStart = { x: e.clientX, y: e.clientY };
  edDiv = document.createElement('div');
  edDiv.className = 'editmark';
  edHit.pv.div.appendChild(edDiv);
  sizeMark(edDiv, edHit.r, edStart, edStart);
  e.preventDefault();
});
els.viewerContainer.addEventListener('pointermove', (e) => {
  if (edStart) sizeMark(edDiv, edHit.r, edStart, { x: e.clientX, y: e.clientY });
});
els.viewerContainer.addEventListener('pointerup', async (e) => {
  if (!edStart) return;
  const r = edHit.r, hit = edHit;
  const x0 = Math.min(edStart.x, e.clientX), y0 = Math.min(edStart.y, e.clientY);
  const fw = Math.abs(e.clientX - edStart.x) / r.width;
  const fh = Math.abs(e.clientY - edStart.y) / r.height;
  const fx0 = (x0 - r.left) / r.width, fy0 = (y0 - r.top) / r.height;
  edDiv.remove();
  edStart = null; edDiv = null; edHit = null;
  if (fw > 0.004 && fh > 0.004) await addEdit(hit, [fx0, fy0, fx0 + fw, fy0 + fh]);
});

// addEdit reads the text run under the drawn box and its style, then builds the
// prefilled, covered edit overlay. frac is the box in page fractions (top-left).
async function addEdit(hit, frac) {
  const n = hit.n, pv = hit.pv;
  const page = await pdfDocument.getPage(n);
  const base = page.getViewport({ scale: 1 });
  const pageW = base.width, pageH = base.height; // PDF points
  // Box in PDF points (bottom-left origin) for the text-overlap test.
  const bx0 = frac[0] * pageW, bx1 = frac[2] * pageW;
  const byTop = (1 - frac[1]) * pageH, byBot = (1 - frac[3]) * pageH; // byBot < byTop

  let text = '', size = (byTop - byBot) * 0.8, font = 'Helvetica';
  try {
    const tc = await page.getTextContent();
    const picked = [];
    for (const it of tc.items) {
      if (!it.str) continue;
      const ix0 = it.transform[4], iy = it.transform[5];
      const ih = it.height || Math.hypot(it.transform[2], it.transform[3]);
      const ix1 = ix0 + it.width, iyTop = iy + ih * 0.8, iyBot = iy - ih * 0.2;
      if (ix1 < bx0 || ix0 > bx1 || iyTop < byBot || iyBot > byTop) continue; // no overlap
      picked.push({ str: it.str, x: ix0, y: iy, fontName: it.fontName, size: Math.hypot(it.transform[2], it.transform[3]) || ih });
    }
    if (picked.length) {
      picked.sort((a, b) => (Math.abs(a.y - b.y) > 1 ? b.y - a.y : a.x - b.x)); // top-to-bottom, left-to-right
      text = picked.map((p) => p.str).join('').replace(/\s+/g, ' ').trim();
      const dom = picked.slice().sort((a, b) => b.str.length - a.str.length)[0]; // the dominant run sets the style
      size = dom.size || size;
      let name = tc.styles?.[dom.fontName]?.fontFamily || '';
      try { if (page.commonObjs.has(dom.fontName)) name += ' ' + (page.commonObjs.get(dom.fontName)?.name || ''); } catch { /* font not resolved */ }
      font = classifyFont(name);
    }
  } catch { /* image-only page: no text layer */ }

  const { text: color, bg } = await samplePageColors(page, frac);
  // Widen the cover a touch so ascenders/descenders of the original are hidden.
  const my = 0.15 * (frac[3] - frac[1]), mx = 0.01 * (frac[2] - frac[0]);
  const coverFrac = [Math.max(0, frac[0] - mx), Math.max(0, frac[1] - my), Math.min(1, frac[2] + mx), Math.min(1, frac[3] + my)];
  const f = makeEditField(frac, { page: n, pageW, pageH, text, size, color, bg, font, coverFrac }, pv);
  f.el.focus();
  f.el.select();
}

function makeEditField(frac, opts, pv) {
  const f = {
    page: opts.page, frac, pageW: opts.pageW, pageH: opts.pageH, kind: 'edit',
    font: opts.font, size: opts.size, color: opts.color, bg: opts.bg, coverFrac: opts.coverFrac,
  };
  const el = document.createElement('input');
  el.type = 'text';
  el.className = 'ovl ovl-edit';
  el.value = opts.text || '';
  el.style.color = opts.color;
  el.style.background = opts.bg; // opaque: covers the original live, matching the baked cover
  el.style.fontFamily = cssFamily(opts.font);
  el.style.fontWeight = /Bold/.test(opts.font) ? '700' : '400';
  el.style.fontStyle = /(Italic|Oblique)/.test(opts.font) ? 'italic' : 'normal';
  f.el = el;
  overlayFields.push(f);
  layoutField(f, pv);
  pv.div.appendChild(el);
  return f;
}

// samplePageColors renders the page and reads, within the box, the darkest pixel
// (the ink → replacement text colour) and the lightest (the background → cover
// colour). White-filled first, so a transparent/white page reads white.
async function samplePageColors(page, frac) {
  const vp = page.getViewport({ scale: Math.min(2, 1400 / page.getViewport({ scale: 1 }).width) });
  const cv = document.createElement('canvas');
  cv.width = Math.ceil(vp.width); cv.height = Math.ceil(vp.height);
  const c = cv.getContext('2d');
  c.fillStyle = '#fff'; c.fillRect(0, 0, cv.width, cv.height);
  await page.render({ canvasContext: c, viewport: vp }).promise;
  const x0 = Math.max(0, Math.floor(frac[0] * cv.width)), y0 = Math.max(0, Math.floor(frac[1] * cv.height));
  const w = Math.max(1, Math.min(cv.width - x0, Math.ceil((frac[2] - frac[0]) * cv.width)));
  const h = Math.max(1, Math.min(cv.height - y0, Math.ceil((frac[3] - frac[1]) * cv.height)));
  const data = c.getImageData(x0, y0, w, h).data;
  let dl = 1e9, ll = -1, text = '#000000', bg = '#ffffff';
  for (let i = 0; i < data.length; i += 4) {
    const lum = 0.299 * data[i] + 0.587 * data[i + 1] + 0.114 * data[i + 2];
    if (lum < dl) { dl = lum; text = rgbHex(data[i], data[i + 1], data[i + 2]); }
    if (lum > ll) { ll = lum; bg = rgbHex(data[i], data[i + 1], data[i + 2]); }
  }
  return { text, bg };
}
const rgbHex = (r, g, b) => '#' + [r, g, b].map((v) => v.toString(16).padStart(2, '0')).join('');

// coverPNG renders a solid background-coloured rectangle at the rect's aspect, so
// the server's fit-to-rect image stamp fills it exactly.
function coverPNG(wPts, hPts, hex) {
  const cv = document.createElement('canvas');
  cv.width = Math.max(2, Math.round(wPts * 2));
  cv.height = Math.max(2, Math.round(hPts * 2));
  const c = cv.getContext('2d');
  c.fillStyle = hex || '#ffffff';
  c.fillRect(0, 0, cv.width, cv.height);
  return cv.toDataURL('image/png').split(',')[1];
}

// classifyFont maps a BaseFont/family string to the closest Base-14 core font
// (serif→Times, mono→Courier, else Helvetica) carrying bold/italic — the MVP of
// font recognition. Embedded-font reuse is a later, higher-fidelity tier.
function classifyFont(name) {
  const s = (name || '').toLowerCase();
  const bold = /bold|black|heavy|semibold/.test(s);
  const italic = /italic|oblique/.test(s);
  let fam = 'Helvetica';
  if (/courier|mono|consol/.test(s)) fam = 'Courier';
  else if (/times|serif|roman|georgia|garamond|minion|cambria|book antiqua/.test(s) && !/sans/.test(s)) fam = 'Times';
  if (fam === 'Times') return 'Times-' + (bold && italic ? 'BoldItalic' : bold ? 'Bold' : italic ? 'Italic' : 'Roman');
  const suffix = bold && italic ? '-BoldOblique' : bold ? '-Bold' : italic ? '-Oblique' : '';
  return fam + suffix; // Helvetica / Courier (+ -Bold / -Oblique / -BoldOblique)
}
function cssFamily(core) {
  if (core.startsWith('Times')) return 'serif';
  if (core.startsWith('Courier')) return 'monospace';
  return 'sans-serif';
}

// Remove originals: flatten every edited page so the covered text is gone for
// good. Reuses the redaction path — bake the covers + replacement text in, then
// rasterize just those pages (no black box painted).
els.removeOriginalsBtn.onclick = async () => {
  const pages = [...new Set(overlayFields.filter((f) => f.kind === 'edit').map((f) => f.page))];
  if (!pages.length) return toast('No text edits to flatten');
  if (!confirm('Make the text edits permanent? The edited page(s) become flat images and the original text underneath is removed. This cannot be undone.' + signatureWarning())) return;

  const res = await flattenPages(pages);
  if (!res.ok) return toast('could not flatten edits');
  overlayFields = overlayFields.filter((f) => { if (f.kind === 'edit' && pages.includes(f.page)) { f.el.remove(); return false; } return true; });
  editMode = false; reflectEdit();
  els.viewerContainer.style.cursor = '';
  await setDocumentFromServer(await res.json());
  toast('Edits flattened — original text removed on edited page(s)');
};

// --- open dialog -------------------------------------------------------------
// The Open… dialog is the single open surface: type a path or URL, or browse the
// filesystem. Browsing opens BY PATH (via openPath -> /api/open), so the file can
// be saved in place and is remembered in Recent — unlike a drag-drop upload.
let lastBrowseDir = ''; // remember where the user last browsed, across opens

async function openBrowse(path) {
  const res = await apiFetch('/api/listdir?path=' + encodeURIComponent(path || '~'));
  if (!res.ok) return toast('could not list folder');
  const info = await res.json();
  lastBrowseDir = info.path;
  els.openDir.value = info.path;
  els.openHere.textContent = info.path;
  els.openUp.disabled = !info.parent;
  els.openUp.dataset.parent = info.parent || '';
  els.openList.innerHTML = '';
  // PDFs first — they're what this dialog opens; folders follow for navigation.
  for (const f of (info.files || [])) {
    const li = document.createElement('li');
    li.className = 'file';
    li.textContent = f;
    li.onclick = () => { els.openModal.hidden = true; openPath(joinPath(info.path, f)); };
    els.openList.appendChild(li);
  }
  for (const d of info.dirs) {
    const li = document.createElement('li');
    li.textContent = d;
    li.onclick = () => openBrowse(joinPath(info.path, d));
    els.openList.appendChild(li);
  }
}

function openOpenDialog() {
  els.openModal.hidden = false;
  els.pathInput.value = '';
  openBrowse(lastBrowseDir);
  els.pathInput.focus();
}
function openTyped() {
  const v = els.pathInput.value.trim();
  if (!v) return;
  els.openModal.hidden = true;
  openSmart(v);
}
els.openMenuItem.onclick = openOpenDialog;
els.openGo.onclick = openTyped;
els.pathInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') openTyped(); });
els.openCancel.onclick = () => { els.openModal.hidden = true; };
els.openUp.onclick = () => { const p = els.openUp.dataset.parent; if (p) openBrowse(p); };
els.openDir.onchange = () => openBrowse(els.openDir.value.trim());

// --- menu bar ----------------------------------------------------------------
// One controller for both bars (the menubar's Edit/View and the toolbar's
// Recent/Save/Export/More): click a top label to open it, hover to switch while
// another is open, click a command (or click-outside / Escape) to close. Inputs
// inside a dropdown don't close it. The Recent menu refreshes its list on open.
let openMenu = null;
function closeMenu() {
  if (openMenu) { openMenu.classList.remove('open'); openMenu = null; }
}
function showMenu(menu) {
  if (openMenu === menu) return;
  closeMenu();
  menu.classList.add('open');
  openMenu = menu;
  if (menu.querySelector('.recentSlot')) refreshRecent();
}
function onBarClick(e) {
  const top = e.target.closest('.menutop');
  if (top) { const m = top.parentElement; openMenu === m ? closeMenu() : showMenu(m); return; }
  if (e.target.closest('.dropdown button')) closeMenu(); // a command was chosen
}
function onBarHover(e) {
  if (!openMenu) return;
  const top = e.target.closest('.menutop');
  if (top && top.parentElement !== openMenu) showMenu(top.parentElement);
}
for (const bar of [els.menubar, els.toolbar]) {
  bar.addEventListener('click', onBarClick);
  bar.addEventListener('mouseover', onBarHover);
}
// Close on any click outside an open menu — including the toolbar's edit icons.
document.addEventListener('click', (e) => { if (!e.target.closest('.menu')) closeMenu(); });
document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeMenu(); });

// Settings: the layout style (menus / toolbar / both) drives a body attribute the
// CSS keys off; the saved value is applied in applyStatus and persisted on change.
function applyStyle(style) {
  document.body.dataset.style = style;
  all('.styleOpt').forEach((b) => b.classList.toggle('active', b.dataset.styleVal === style));
  // Toolbar mode hides the menubar, so the always-on chrome rides on the toolbar.
  (style === 'toolbar' ? els.toolbar : els.menubar).appendChild(els.statusCluster);
}
async function saveSettings(body) {
  try {
    const res = await apiFetch('/api/settings', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    });
    if (!res.ok) toast('Could not save settings');
  } catch { toast('Could not save settings'); }
}
all('.styleOpt').forEach((b) => {
  b.onclick = () => { applyStyle(b.dataset.styleVal); saveSettings({ toolbarStyle: b.dataset.styleVal }); };
});
els.autoUpdateChk.onchange = () => saveSettings({ checkUpdatesOnStartup: els.autoUpdateChk.checked });

async function refreshRecent() {
  const res = await apiFetch('/api/recent');
  const recent = (res.ok ? await res.json() : []) || []; // tolerate a null body
  for (const slot of all('.recentSlot')) { // a slot in the File menu and one in the toolbar
    slot.innerHTML = '';
    if (!recent.length) {
      const empty = document.createElement('div');
      empty.className = 'menuitem idle'; empty.textContent = 'No recent files';
      slot.appendChild(empty);
      continue;
    }
    for (const p of recent) {
      const b = document.createElement('button');
      b.textContent = p.replace(/^.*\//, ''); b.title = p;
      b.onclick = () => openPath(p);
      slot.appendChild(b);
    }
  }
}

// Annotation tools (M4 Text + M8 Highlight/Draw): mutually exclusive toggles of
// the pdf.js editor mode. Each is baked into the PDF by saveDocument(). Modes
// come from the buttons' data-mode (FREETEXT, HIGHLIGHT, INK).
let activeTool = null;
function setTool(mode) {
  activeTool = activeTool === mode ? null : mode;
  viewer.annotationEditorMode = {
    mode: activeTool ? pdfjsLib.AnnotationEditorType[activeTool] : pdfjsLib.AnnotationEditorType.NONE,
  };
  // Mirror the active mode onto every control bound to it (Edit menu + toolbar).
  document.querySelectorAll('[data-mode]').forEach((b) => b.classList.toggle('active', b.dataset.mode === activeTool));
}
document.querySelectorAll('[data-mode]').forEach((b) => {
  b.onclick = () => setTool(b.dataset.mode);
});

// The toolbar mirrors the menus. Mode tools wire themselves via [data-mode]
// above; every other toolbar control forwards to its menu twin by id.
all('#toolbar [data-forward]').forEach((b) => { b.onclick = () => $(b.dataset.forward).click(); });

// Drag-and-drop a PDF onto the window to open it (upload origin -> Save As).
['dragover', 'drop'].forEach((ev) => window.addEventListener(ev, (e) => e.preventDefault()));
window.addEventListener('drop', (e) => {
  const file = [...(e.dataTransfer?.files || [])].find((f) => f.type === 'application/pdf');
  if (file) uploadFile(file);
});

// Detect fillable regions (underlines, boxes, checkboxes) and place an editable
// overlay widget on each — a text box for lines/boxes, a checkbox for squares.
// Each field stores its rectangle in PDF points; on save the values are stamped
// into the PDF (see bakedBytes). Widgets reposition with zoom/scroll.
// Each field stores its rectangle as a FRACTION of the page (top-left origin),
// so display position is just frac * actual page-div size (no scale drift) and
// the PDF rect for stamping is frac * page dimensions.
let overlayFields = []; // {page, frac:[fx0,fy0,fx1,fy1], pageW, pageH, kind, el}
let libraryImages = []; // cached /api/images list (the image-library panel)
function clearOverlays() { overlayFields.forEach((f) => f.el.remove()); overlayFields = []; }
// clearDetected drops only auto-detected fields (text/check/circleone), keeping
// user-placed stamps and text edits so re-running Detect doesn't wipe a
// signature/quick-stamp or a cover-and-replace edit.
function clearDetected() {
  overlayFields = overlayFields.filter((f) => {
    if (f.kind === 'stamp' || f.kind === 'edit') return true;
    f.el.remove();
    return false;
  });
}

function layoutField(f, pv) {
  const W = pv.div.clientWidth, H = pv.div.clientHeight;
  const h = (f.frac[3] - f.frac[1]) * H;
  f.el.style.left = (f.frac[0] * W) + 'px';
  f.el.style.top = (f.frac[1] * H) + 'px';
  f.el.style.width = ((f.frac[2] - f.frac[0]) * W) + 'px';
  f.el.style.height = h + 'px';
  if (f.kind === 'text') f.el.style.fontSize = Math.max(7, h * 0.72) + 'px';
  // Edit fields carry a recognized point size; scale it to the rendered page
  // (css px per point = rendered width / page points) so the live overlay matches
  // the baked text instead of being sized from the box height.
  else if (f.kind === 'edit') f.el.style.fontSize = (f.size * W / f.pageW) + 'px';
}
function relayoutOverlays() {
  for (const f of overlayFields) {
    const pv = viewer.getPageView(f.page - 1);
    if (pv?.div && pv.viewport) {
      if (f.el.parentElement !== pv.div) pv.div.appendChild(f.el);
      layoutField(f, pv);
    }
  }
}
eventBus.on('scalechanging', relayoutOverlays);
eventBus.on('pagerendered', relayoutOverlays);

// page-fraction (top-left origin) -> PDF points (bottom-left origin)
function rectPoints(f, frac) {
  const [fx0, fy0, fx1, fy1] = frac;
  return [fx0 * f.pageW, (1 - fy1) * f.pageH, fx1 * f.pageW, (1 - fy0) * f.pageH];
}

function collectFields() {
  const out = [];
  for (const f of overlayFields) {
    if (f.kind === 'text' && f.el.value.trim() !== '') {
      out.push({ page: f.page, rect: rectPoints(f, f.frac), text: f.el.value });
    } else if (f.kind === 'edit' && f.el.value.trim() !== '') {
      // Cover-and-replace: carry the recognized font/size/colour so the bake
      // matches the original run. (An emptied edit is an erase — cover only.)
      out.push({ page: f.page, rect: rectPoints(f, f.frac), text: f.el.value, font: f.font, size: f.size, color: f.color });
    }
  }
  return out;
}

// collectCovers gathers the opaque fills baked under each text edit: a solid
// background-coloured PNG over the covered run, sent so the server stamps it
// before the replacement text (see /api/bake).
function collectCovers() {
  const out = [];
  for (const f of overlayFields) {
    if (f.kind !== 'edit') continue;
    const rect = rectPoints(f, f.coverFrac || f.frac);
    out.push({ page: f.page, rect, png: coverPNG(rect[2] - rect[0], rect[3] - rect[1], f.bg) });
  }
  return out;
}

// collectStamps gathers image stamps: placed images/quick-stamps (library id or
// inline PNG), circled choices (a pill/ellipse PNG over the picked option), and
// checkbox X's.
function collectStamps() {
  const out = [];
  for (const f of overlayFields) {
    if (f.kind === 'stamp') {
      const rect = rectPoints(f, f.frac);
      if (f.imageId) out.push({ page: f.page, rect, image: f.imageId });
      else if (f.png) out.push({ page: f.page, rect, png: f.png });
    } else if (f.kind === 'circleone' && f.choice != null) {
      const ch = f.choices[f.choice];
      out.push({ page: f.page, rect: rectPoints(f, ch.rect), png: shapePNG((ch.rect[2] - ch.rect[0]) * f.pageW, (ch.rect[3] - ch.rect[1]) * f.pageH, ch.word) });
    } else if (f.kind === 'check' && f.el.checked) {
      const [x0, y0, x1, y1] = rectPoints(f, f.frac);
      out.push({ page: f.page, rect: [x0, y0, x1, y1], png: xPNG(x1 - x0, y1 - y0) });
    }
  }
  return out;
}

// bakedBytes is the canonical current document: pdf.js form/annotation edits via
// saveDocument(), plus the auto-detected overlay fields stamped in server-side.
async function bakedBytes() {
  // saveDocument() bakes pdf.js annotation-storage edits (form fills, FREETEXT/
  // INK/HIGHLIGHT/STAMP) and warns + does a needless rewrite when storage is
  // empty. Our overlay edits (covers/fields/stamps) bake server-side via
  // /api/bake, so when there are no pdf.js edits, getData() is the library's
  // prescribed lighter equivalent (raw bytes, no bake).
  const saved = pdfDocument.annotationStorage.size > 0
    ? await pdfDocument.saveDocument()
    : await pdfDocument.getData();
  const fields = collectFields();
  const stamps = collectStamps();
  const covers = collectCovers();
  if (!fields.length && !stamps.length && !covers.length) return saved;
  const form = new FormData();
  form.append('pdf', new Blob([saved], { type: 'application/pdf' }), 'doc.pdf');
  if (covers.length) form.append('covers', JSON.stringify(covers));
  if (fields.length) form.append('fields', JSON.stringify(fields));
  if (stamps.length) form.append('stamps', JSON.stringify(stamps));
  const res = await apiFetch('/api/bake', { method: 'POST', body: form });
  if (!res.ok) { toast('could not apply edits'); return saved; }
  return new Uint8Array(await res.arrayBuffer());
}

els.detectBtn.onclick = async () => {
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  clearDetected();
  const n = viewer.currentPageNumber;
  const pv = viewer.getPageView(n - 1);
  if (!pv?.div || !pv.viewport) { toast('Scroll the page into view, then try again'); return; }

  // Render the page to an offscreen canvas at a consistent resolution, so
  // detection doesn't depend on the current zoom (faint thin rules need it).
  const page = await pdfDocument.getPage(n);
  const base = page.getViewport({ scale: 1 });
  const pageW = base.width, pageH = base.height; // PDF points
  const dvp = page.getViewport({ scale: Math.min(3, 1600 / pageW) });
  const canvas = document.createElement('canvas');
  canvas.width = dvp.width; canvas.height = dvp.height;
  const cx = canvas.getContext('2d');
  cx.fillStyle = '#fff';
  cx.fillRect(0, 0, canvas.width, canvas.height);
  await page.render({ canvasContext: cx, viewport: dvp }).promise;

  const regions = detectRegions(canvas);
  // Add faint light-gray underlines the main pass misses, deduped against it.
  for (const fr of detectFaintRules(canvas)) {
    if (!regions.some((r) => Math.abs(r.y - fr.y) < 8 && fr.x < r.x + r.w && fr.x + fr.w > r.x)) regions.push(fr);
  }
  const cells = detectTableCells(canvas);
  // Text-filled callout boxes, minus any that overlap the table grid (those are
  // the table itself — its cells, not a box, decide what's fillable there).
  const filledBoxes = detectFilledBoxes(canvas).filter((b) =>
    !cells.some((c) => c.x0 < b.x1 && c.x1 > b.x0 && c.y0 < b.y1 && c.y1 > b.y0));

  // Text layer: locate signature labels and Y/N choices by their words, in the
  // same device pixels as the detection canvas.
  const W = canvas.width, H = canvas.height;
  let textItems = [];
  try {
    const tc = await page.getTextContent();
    textItems = tc.items.filter((it) => it.str && it.str.trim()).map((it) => {
      const t = pdfjsLib.Util.transform(dvp.transform, it.transform);
      return { str: it.str, x: t[4], y: t[5], w: it.width * dvp.scale, h: Math.abs(it.height * dvp.scale) };
    });
  } catch { /* image-only PDF: no text layer, skip word matching */ }
  const ynItems = findYesNo(textItems);

  const inCell = (x, y) => cells.some((c) => x >= c.x0 - 2 && x <= c.x1 + 2 && y >= c.y0 - 2 && y <= c.y1 + 2);
  // A text-filled callout box swallows any field proposed inside it (it has no
  // blank to fill). Only line/box regions are suppressed — table-cell inputs
  // come from `cells`, so a box overlapping a table never hides its cells.
  const inFilledBox = (x, y) => filledBoxes.some((b) => x >= b.x0 && x <= b.x1 && y >= b.y0 && y <= b.y1);
  const aboveLine = 15 / pageH; // underline field extends one line-height above the rule
  let added = 0;

  // 1. Blank table cells each become their own text input.
  for (const c of cells) {
    if (c.filled) continue; // a cell with printed text / no blank space — skip
    makeField('text', [c.x0 / W, c.y0 / H, c.x1 / W, c.y1 / H], { page: n, pageW, pageH }, pv);
    added++;
  }

  // 2. Underlines, boxes, checkboxes — but not inside the table (cells own those).
  for (let i = 0; i < regions.length; i++) {
    const r = regions[i];
    const mx = r.x + r.w / 2, my = r.y + r.h / 2;
    if (inCell(mx, my) || inFilledBox(mx, my)) continue;
    let fx0 = r.x / W, fx1 = (r.x + r.w) / W;
    let fy0 = r.y / H, fy1 = (r.y + r.h) / H;
    const cssW = (fx1 - fx0) * pv.div.clientWidth, cssH = (fy1 - fy0) * pv.div.clientHeight;

    let kind = 'text';
    if (r.box && cssW <= 28 && cssH <= 28) kind = 'check';
    else if (!r.box) fy0 = fy1 - aboveLine;
    makeField(kind, [fx0, fy0, fx1, fy1], { page: n, pageW, pageH }, pv);
    added++;
  }

  // 3. "Circle one" choices (incl. Y/N) become a circle-my-answer widget: a
  //    radio set of choices, each circled (pill around a word) when picked.
  for (const grp of [...ynItems, ...findCircleOne(textItems)]) {
    const choices = snapChoices(canvas, grp.choices, grp.marker);
    const cf = choices.map((c) => ({ rect: [c.x0 / W, c.y0 / H, c.x1 / W, c.y1 / H], word: !!c.word }));
    const x0 = Math.min(...cf.map((c) => c.rect[0])), y0 = Math.min(...cf.map((c) => c.rect[1]));
    const x1 = Math.max(...cf.map((c) => c.rect[2])), y1 = Math.max(...cf.map((c) => c.rect[3]));
    makeField('circleone', [x0, y0, x1, y1], { page: n, pageW, pageH, choices: cf }, pv);
    added++;
  }

  toast(added ? `Added ${added} fillable field(s) — fill, then Save` : 'Nothing detected on this page');
};

// buildTextRows groups text items into rows; per row it gives a reconstructed
// string `s` with per-char x-centre `cx` and height `ch`, plus a `words` list
// with pixel x-spans. Shared by the circle-a-choice detectors below.
function buildTextRows(items) {
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
function snapChoices(canvas, choices, marker) {
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
function choiceBox(x0, x1, baseY, hh) {
  const word = (x1 - x0) > hh * 1.3;
  const pad = hh * (word ? 0.35 : 0.7);
  return { x0: x0 - pad, y0: baseY - hh * 1.1, x1: x1 + pad, y1: baseY + hh * 0.28, word };
}

// findYesNo locates "Y/N" choices, robust to case, spacing, and the glyphs being
// split across text-layer fragments. Returns circle-one groups (Y and N as two
// single-letter choices).
function findYesNo(items) {
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
function findCircleOne(items) {
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
function extractChoices(row, mx0, mx1) {
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
  return best.map((seg) => choiceBox(Math.min(...seg.map((w) => w.x0)), Math.max(...seg.map((w) => w.x1)), row.y, seg[0].h));
}

// detectRegions scans a rendered page canvas for horizontal rule lines and for
// rectangles (boxes / checkboxes), via dark horizontal and vertical runs. It is
// a best-effort heuristic — it proposes regions; the user adjusts.
function detectRegions(canvas) {
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
function detectFilledBoxes(canvas) {
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
function detectFaintRules(canvas) {
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
function detectTableCells(canvas) {
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

// makeField creates an auto-detected overlay widget of the given kind and rect
// (page fractions, top-left origin) and registers it. kinds: text, check,
// circleone (circle one of N choices). Signatures aren't a field kind — place a
// signature image from the library instead.
function makeField(kind, frac, opts, pv) {
  const f = { page: opts.page, frac, pageW: opts.pageW, pageH: opts.pageH, kind };
  if (kind === 'check') {
    f.el = document.createElement('input');
    f.el.type = 'checkbox';
    f.el.className = 'ovl ovl-check';
  } else if (kind === 'circleone') {
    f.choices = opts.choices; // [{rect:[fracs], word}]
    f.el = document.createElement('div');
    f.el.className = 'ovl ovl-circleone';
    const [fx0, fy0, fx1, fy1] = frac;
    const pct = (r) => ({ left: 100 * (r[0] - fx0) / (fx1 - fx0), top: 100 * (r[1] - fy0) / (fy1 - fy0), w: 100 * (r[2] - r[0]) / (fx1 - fx0), h: 100 * (r[3] - r[1]) / (fy1 - fy0) });
    const mark = document.createElement('span');
    mark.className = 'circ-mark';
    mark.hidden = true;
    f.el.appendChild(mark);
    f.choices.forEach((ch, idx) => {
      const p = pct(ch.rect);
      const b = document.createElement('button');
      b.type = 'button'; b.className = 'circ-hit';
      b.style.left = p.left + '%'; b.style.top = p.top + '%'; b.style.width = p.w + '%'; b.style.height = p.h + '%';
      b.onclick = () => {
        f.choice = f.choice === idx ? null : idx; // radio: pick one (or clear)
        if (f.choice === idx) {
          mark.style.left = p.left + '%'; mark.style.top = p.top + '%'; mark.style.width = p.w + '%'; mark.style.height = p.h + '%';
          mark.style.borderRadius = ch.word ? '999px' : '50%';
          mark.hidden = false;
        } else mark.hidden = true;
      };
      f.el.appendChild(b);
    });
  } else {
    f.el = document.createElement('input');
    f.el.type = 'text';
    f.el.className = 'ovl ovl-text';
  }
  overlayFields.push(f);
  layoutField(f, pv);
  pv.div.appendChild(f.el);
  return f;
}

// INK is the blue pen used for every hand-drawn mark — circles, pills, the
// checkbox X, and the quick-stamp checkmark — so they all read as one pen.
const INK = '#1d4ed8';
const inkWidth = (s) => Math.max(2, Math.round(1.6 * s)); // pen weight at supersample s

// xPNG renders a transparent PNG with a stroked "X" in the same ink and pen
// weight as shapePNG's circles, sized to fill a checkbox. The server scales it
// to the rect.
function xPNG(wPts, hPts) {
  const s = 3;
  const cv = document.createElement('canvas');
  cv.width = Math.max(8, Math.round(wPts * s));
  cv.height = Math.max(8, Math.round(hPts * s));
  const c = cv.getContext('2d');
  c.strokeStyle = INK;
  c.lineWidth = inkWidth(s);
  c.lineCap = 'round';
  const m = c.lineWidth, W = cv.width, H = cv.height; // inset so the stroke stays in the box
  c.beginPath();
  c.moveTo(m, m); c.lineTo(W - m, H - m);
  c.moveTo(W - m, m); c.lineTo(m, H - m);
  c.stroke();
  return cv.toDataURL('image/png').split(',')[1];
}

// shapePNG renders a transparent PNG with a stroked hand-circle: an ellipse for
// a single letter, or a pill (stadium) around a word. Returns base64 (no data:
// prefix). The server scales it to the rect.
function shapePNG(wPts, hPts, pill) {
  const s = 3; // supersample for a crisp stroke
  const cv = document.createElement('canvas');
  cv.width = Math.max(8, Math.round(wPts * s));
  cv.height = Math.max(8, Math.round(hPts * s));
  const c = cv.getContext('2d');
  c.strokeStyle = INK;
  c.lineWidth = inkWidth(s);
  const lw = c.lineWidth, W = cv.width, H = cv.height;
  c.beginPath();
  if (pill) {
    const r = (H - lw) / 2; // fully rounded ends → stadium
    const x0 = lw / 2, x1 = W - lw / 2, y0 = lw / 2, y1 = H - lw / 2;
    c.moveTo(x0 + r, y0);
    c.lineTo(x1 - r, y0);
    c.arc(x1 - r, y0 + r, r, -Math.PI / 2, Math.PI / 2);
    c.lineTo(x0 + r, y1);
    c.arc(x0 + r, y0 + r, r, Math.PI / 2, -Math.PI / 2);
    c.closePath();
  } else {
    c.ellipse(W / 2, H / 2, W / 2 - lw, H / 2 - lw, 0, 0, Math.PI * 2);
  }
  c.stroke();
  return cv.toDataURL('image/png').split(',')[1];
}
els.saveBtn.onclick = save;

els.prevBtn.onclick = () => { if (viewer.currentPageNumber > 1) viewer.currentPageNumber--; };
els.nextBtn.onclick = () => { if (viewer.currentPageNumber < pdfDocument.numPages) viewer.currentPageNumber++; };
all('.pageNum').forEach((input) => input.addEventListener('change', () => {
  const n = Number(input.value);
  if (pdfDocument && n >= 1 && n <= pdfDocument.numPages) viewer.currentPageNumber = n;
}));
els.zoomInBtn.onclick = () => { viewer.currentScale = viewer.currentScale * 1.15; };
els.zoomOutBtn.onclick = () => { viewer.currentScale = viewer.currentScale / 1.15; };
els.fitBtn.onclick = () => { viewer.currentScaleValue = 'page-width'; };

let searchTimer;
all('.searchInput').forEach((input) => input.addEventListener('input', () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    eventBus.dispatch('find', {
      type: '', query: input.value, caseSensitive: false,
      highlightAll: true, findPrevious: false,
    });
  }, 200);
}));

// sidebar tabs
document.querySelectorAll('.tab').forEach((tab) => {
  tab.onclick = () => {
    document.querySelectorAll('.tab').forEach((t) => t.classList.remove('active'));
    document.querySelectorAll('.panel').forEach((p) => p.classList.remove('active'));
    tab.classList.add('active');
    $(tab.dataset.panel).classList.add('active');
    if (tab.dataset.panel === 'library') loadImages();
  };
});

// sidebar collapse
function toggleSidebar() {
  const hidden = $('sidebar').classList.toggle('collapsed');
  $('toggleSidebarBtn').textContent = hidden ? 'Show sidebar' : 'Hide sidebar';
}
$('toggleSidebarBtn').onclick = toggleSidebar;

// keyboard: Ctrl/Cmd+S saves, Ctrl/Cmd+B toggles the sidebar
window.addEventListener('keydown', (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') { e.preventDefault(); save(); }
  if ((e.ctrlKey || e.metaKey) && e.key === 'b') { e.preventDefault(); toggleSidebar(); }
});

// --- toast -------------------------------------------------------------------
let toastEl;
function toast(msg) {
  if (!toastEl) {
    toastEl = document.createElement('div');
    toastEl.id = 'toast';
    document.body.appendChild(toastEl);
  }
  toastEl.textContent = msg;
  toastEl.classList.add('show');
  setTimeout(() => toastEl.classList.remove('show'), 2500);
}

// --- launch: check unlock state, then show the app or the first-run wizard ----
refreshStatus();
