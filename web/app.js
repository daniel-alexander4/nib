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
import {
  detectRegions,
  detectFaintRules,
  detectTableCells,
  detectFilledBoxes,
  findYesNo,
  findCircleOne,
  findSlashTemplates,
  findPipeChoices,
  findRunChoices,
  snapChoices,
  dedupeGroups,
  buildTextRows,
} from './detect.js';
import { diffWords } from './vendor/diff/diff.min.mjs';
import pixelmatch from './vendor/pixelmatch/pixelmatch.mjs';

pdfjsLib.GlobalWorkerOptions.workerSrc = './vendor/pdfjs/pdf.worker.min.mjs';

// --- element handles ---------------------------------------------------------
const $ = (id) => document.getElementById(id);
const els = {
  menubar: $('menubar'), toolbar: $('toolbar'), openMenuItem: $('openMenuItem'),
  officeOpenBtn: $('officeOpenBtn'), officeInput: $('officeInput'),
  combineBtn: $('combineBtn'), combineModal: $('combineModal'), combineList: $('combineList'),
  combineAddBtn: $('combineAddBtn'), combineInput: $('combineInput'),
  combineCancel: $('combineCancel'), combineGo: $('combineGo'),
  compareBtn: $('compareBtn'), compareModal: $('compareModal'), compareBody: $('compareBody'),
  compareInput: $('compareInput'), comparePick: $('comparePick'), compareClose: $('compareClose'),
  compareTools: $('compareTools'), compareSummary: $('compareSummary'),
  comparePager: $('comparePager'), cmPrev: $('cmPrev'), cmNext: $('cmNext'),
  cmAPrev: $('cmAPrev'), cmANext: $('cmANext'), cmPageLabelA: $('cmPageLabelA'),
  cmBPrev: $('cmBPrev'), cmBNext: $('cmBNext'), cmPageLabelB: $('cmPageLabelB'),
  cmAuto: $('cmAuto'), cmAlignStat: $('cmAlignStat'),
  fillCsvBtn: $('fillCsvBtn'), fillCsvModal: $('fillCsvModal'), fillCsvPick: $('fillCsvPick'),
  fillCsvInput: $('fillCsvInput'), fillCsvStatus: $('fillCsvStatus'), fillCsvClose: $('fillCsvClose'),
  pathInput: $('pathInput'), openGo: $('openGo'),
  textToolBtn: $('textToolBtn'), detectBtn: $('detectBtn'),
  hlColors: $('hlColors'), hlSwatches: $('hlSwatches'), hlCustom: $('hlCustom'),
  borderBtn: $('borderBtn'), borderWidth: $('borderWidth'), borderWidthInput: $('borderWidthInput'),
  dropdownBtn: $('dropdownBtn'), radioBtn: $('radioBtn'),
  shapeBtn: $('shapeBtn'), shapeOpts: $('shapeOpts'), shapeFill: $('shapeFill'),
  noteBtn: $('noteBtn'),
  prevBtn: $('prevBtn'), nextBtn: $('nextBtn'),
  findPrevBtn: $('findPrevBtn'), findNextBtn: $('findNextBtn'), findCount: $('findCount'),
  zoomInBtn: $('zoomInBtn'), zoomOutBtn: $('zoomOutBtn'), fitBtn: $('fitBtn'),
  sigBadge: $('sigBadge'), saveBtn: $('saveBtn'), statusCluster: $('statusCluster'),
  themeToggle: $('themeToggle'),
  viewerWrap: $('viewerWrap'), viewerContainer: $('viewerContainer'),
  thumbs: $('thumbs'), thumbGrid: $('thumbGrid'), outline: $('outline'),
  outlineModal: $('outlineModal'), outlineEditList: $('outlineEditList'),
  outlineAddBtn: $('outlineAddBtn'), outlineCancel: $('outlineCancel'), outlineSave: $('outlineSave'),
  thumbSelBar: $('thumbSelBar'), thumbSelCount: $('thumbSelCount'),
  selRotateLeftBtn: $('selRotateLeftBtn'), selRotateRightBtn: $('selRotateRightBtn'),
  selDeleteBtn: $('selDeleteBtn'), selClearBtn: $('selClearBtn'),
  selMoveFrontBtn: $('selMoveFrontBtn'), selMoveBackBtn: $('selMoveBackBtn'),
  appendBtn: $('appendBtn'), appendInput: $('appendInput'),
  redactBtn: $('redactBtn'), applyRedactBtn: $('applyRedactBtn'),
  redactTextBtn: $('redactTextBtn'), redactTextModal: $('redactTextModal'),
  rtTerm: $('rtTerm'), rtSSN: $('rtSSN'), rtEmail: $('rtEmail'), rtPhone: $('rtPhone'),
  rtCard: $('rtCard'), rtFind: $('rtFind'), rtStatus: $('rtStatus'), rtCancel: $('rtCancel'),
  editTextBtn: $('editTextBtn'), removeOriginalsBtn: $('removeOriginalsBtn'), ocrBtn: $('ocrBtn'), ocrLang: $('ocrLang'), ocrQuality: $('ocrQuality'),
  scanBtn: $('scanBtn'), scanModal: $('scanModal'), scanBody: $('scanBody'),
  scanStripBtn: $('scanStripBtn'), scanMetaBtn: $('scanMetaBtn'), scanSafeBtn: $('scanSafeBtn'),
  scanFlattenBtn: $('scanFlattenBtn'), scanClose: $('scanClose'),
  attachBtn: $('attachBtn'), attachmentsModal: $('attachmentsModal'), attachBody: $('attachBody'),
  attachAddBtn: $('attachAddBtn'), attachInput: $('attachInput'), attachClose: $('attachClose'),
  decryptBtn: $('decryptBtn'), decryptModal: $('decryptModal'), decryptPw: $('decryptPw'),
  decryptGo: $('decryptGo'), decryptCancel: $('decryptCancel'), decryptError: $('decryptError'),
  encryptBtn: $('encryptBtn'), encryptModal: $('encryptModal'), encryptPw: $('encryptPw'),
  encryptPw2: $('encryptPw2'), encryptGo: $('encryptGo'), encryptCancel: $('encryptCancel'), encryptError: $('encryptError'),
  backupBtn: $('backupBtn'), restoreInput: $('restoreInput'),
  updatePill: $('updatePill'), updateGet: $('updateGet'), updateDismiss: $('updateDismiss'),
  manageKeysBtn: $('manageKeysBtn'), keysModal: $('keysModal'), keysList: $('keysList'),
  keyCandidates: $('keyCandidates'), keyPaste: $('keyPaste'), keyAddPath: $('keyAddPath'),
  keyAddBtn: $('keyAddBtn'), keyCreateBtn: $('keyCreateBtn'), keysClose: $('keysClose'),
  managePeersBtn: $('managePeersBtn'), peersModal: $('peersModal'), peersList: $('peersList'),
  peerSelfFp: $('peerSelfFp'), peerPaste: $('peerPaste'), peerLabel: $('peerLabel'),
  peerPinBtn: $('peerPinBtn'), peersClose: $('peersClose'),
  extSignerStatus: $('extSignerStatus'), extP12File: $('extP12File'), extP12Pass: $('extP12Pass'),
  extP12Import: $('extP12Import'), extP12Remove: $('extP12Remove'),
  cosignBtn: $('cosignBtn'), cosignModal: $('cosignModal'), cosignPeer: $('cosignPeer'),
  cosignIntent: $('cosignIntent'), cosignNoPeers: $('cosignNoPeers'),
  cosignCancel: $('cosignCancel'), cosignGo: $('cosignGo'),
  peerSelfCopy: $('peerSelfCopy'),
  sessionInitBtn: $('sessionInitBtn'), sessionInitModal: $('sessionInitModal'),
  sinPeer: $('sinPeer'), sinNoPeers: $('sinNoPeers'), sinAddr: $('sinAddr'),
  sinIntent: $('sinIntent'), sinProgress: $('sinProgress'),
  sinCancel: $('sinCancel'), sinGo: $('sinGo'),
  sessionSendBtn: $('sessionSendBtn'), sessionSendModal: $('sessionSendModal'),
  ssnPeer: $('ssnPeer'), ssnNoPeers: $('ssnNoPeers'), ssnAddr: $('ssnAddr'),
  ssnProgress: $('ssnProgress'), ssnCancel: $('ssnCancel'), ssnGo: $('ssnGo'),
  sessionRecvBtn: $('sessionRecvBtn'), sessionRecvDocBtn: $('sessionRecvDocBtn'), sessionRecvModal: $('sessionRecvModal'),
  srvTitle: $('srvTitle'), srvArmHint: $('srvArmHint'), srvConsentHint: $('srvConsentHint'),
  srvArm: $('srvArm'), srvWait: $('srvWait'), srvConsent: $('srvConsent'),
  srvPeer: $('srvPeer'), srvNoPeers: $('srvNoPeers'), srvBind: $('srvBind'),
  srvSelfFp: $('srvSelfFp'), srvSelfCopy: $('srvSelfCopy'),
  srvCancel: $('srvCancel'), srvArmGo: $('srvArmGo'),
  srvWaitAddr: $('srvWaitAddr'), srvWaitPeer: $('srvWaitPeer'), srvDisarm: $('srvDisarm'),
  srvPeerLabel: $('srvPeerLabel'), srvPeerFp: $('srvPeerFp'), srvPeerCopy: $('srvPeerCopy'),
  srvReasonCap: $('srvReasonCap'), srvPeerReason: $('srvPeerReason'), srvPreview: $('srvPreview'),
  srvIntentRow: $('srvIntentRow'), srvIntent: $('srvIntent'),
  srvDecline: $('srvDecline'), srvAccept: $('srvAccept'),
  authOverlay: $('authOverlay'), authForm: $('authForm'), authTitle: $('authTitle'),
  authHint: $('authHint'), authPw: $('authPw'), authPwLabel: $('authPwLabel'), migrateRow: $('migrateRow'),
  keyChoice: $('keyChoice'), keySelect: $('keySelect'), keyPath: $('keyPath'),
  createPath: $('createPath'), authWarn: $('authWarn'),
  introOverlay: $('introOverlay'),
  authSubmit: $('authSubmit'), authError: $('authError'),
  addImageBtn: $('addImageBtn'), drawSigBtn: $('drawSigBtn'), addImageInput: $('addImageInput'),
  imageGrid: $('imageGrid'), saveForSigningBtn: $('saveForSigningBtn'),
  signCompleteBtn: $('signCompleteBtn'),
  signBanner: $('signBanner'), signMsg: $('signMsg'), signAction: $('signAction'), signDone: $('signDone'),
  sigModal: $('sigModal'), sigCanvas: $('sigCanvas'),
  sigClear: $('sigClear'), sigCancel: $('sigCancel'), sigSave: $('sigSave'),
  sigDetailsBtn: $('sigDetailsBtn'), sigDetailsModal: $('sigDetailsModal'),
  sigDetailsBody: $('sigDetailsBody'), sigDetailsClose: $('sigDetailsClose'),
  bgModal: $('bgModal'), bgCanvas: $('bgCanvas'), bgRemove: $('bgRemove'),
  bgThresh: $('bgThresh'), bgThreshRow: $('bgThreshRow'),
  bgCancel: $('bgCancel'), bgSave: $('bgSave'),
  autofillBtn: $('autofillBtn'), editProfileBtn: $('editProfileBtn'),
  saveFlatBtn: $('saveFlatBtn'), saveEditableBtn: $('saveEditableBtn'), saveFillableBtn: $('saveFillableBtn'), finalizeBtn: $('finalizeBtn'),
  fieldNameModal: $('fieldNameModal'), fieldNameList: $('fieldNameList'), fieldNameGo: $('fieldNameGo'), fieldNameCancel: $('fieldNameCancel'),
  reduceBtn: $('reduceBtn'), reduceModal: $('reduceModal'), reduceQuality: $('reduceQuality'), reduceQ: $('reduceQ'),
  reduceResult: $('reduceResult'), reduceGo: $('reduceGo'), reduceSave: $('reduceSave'), reduceCancel: $('reduceCancel'),
  exportZipBtn: $('exportZipBtn'), exportPngBtn: $('exportPngBtn'),
  exportImagesBtn: $('exportImagesBtn'), exportTextBtn: $('exportTextBtn'),
  exportTableXlsxBtn: $('exportTableXlsxBtn'), exportTableCsvBtn: $('exportTableCsvBtn'),
  exportTableOdsBtn: $('exportTableOdsBtn'),
  exportFormJsonBtn: $('exportFormJsonBtn'), exportFormCsvBtn: $('exportFormCsvBtn'),
  exportFormXfdfBtn: $('exportFormXfdfBtn'),
  importXfdfBtn: $('importXfdfBtn'), importXfdfModal: $('importXfdfModal'), importXfdfPick: $('importXfdfPick'),
  importXfdfInput: $('importXfdfInput'), importXfdfStatus: $('importXfdfStatus'), importXfdfClose: $('importXfdfClose'),
  pdfaBtn: $('pdfaBtn'), pdfaModal: $('pdfaModal'), pdfaStatus: $('pdfaStatus'),
  pdfaGo: $('pdfaGo'), pdfaGsGo: $('pdfaGsGo'), pdfaClose: $('pdfaClose'),
  exportCertBtn: $('exportCertBtn'), printBtn: $('printBtn'),
  finalizeModal: $('finalizeModal'), fzText: $('fzText'), fzDate: $('fzDate'),
  fzTsa: $('fzTsa'), fzTsaOn: $('fzTsaOn'), fzCancel: $('fzCancel'), fzGo: $('fzGo'),
  fzOpacity: $('fzOpacity'), fzSize: $('fzSize'), fzAngle: $('fzAngle'), fzColor: $('fzColor'),
  fzSignAs: $('fzSignAs'), fzPassphrase: $('fzPassphrase'),
  timestampBtn: $('timestampBtn'), timestampModal: $('timestampModal'), tsCancel: $('tsCancel'), tsGo: $('tsGo'),
  timestampVerifyBtn: $('timestampVerifyBtn'), tsVerifyModal: $('tsVerifyModal'), tvExplorerOn: $('tvExplorerOn'),
  tvExplorer: $('tvExplorer'), tvFile: $('tvFile'), tvResult: $('tvResult'), tvCancel: $('tvCancel'), tvPick: $('tvPick'), tvSave: $('tvSave'),
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
  undoBtn: $('undoBtn'), redoBtn: $('redoBtn'),
  rotateLeftBtn: $('rotateLeftBtn'), rotateRightBtn: $('rotateRightBtn'),
  extractBtn: $('extractBtn'), insertBlankBtn: $('insertBlankBtn'),
  duplicatePageBtn: $('duplicatePageBtn'),
  insertPdfBtn: $('insertPdfBtn'), insertPdfInput: $('insertPdfInput'),
  extractModal: $('extractModal'), extractPages: $('extractPages'),
  extractHint: $('extractHint'), extractCancel: $('extractCancel'), extractGo: $('extractGo'),
  pageNumBtn: $('pageNumBtn'), pageNumModal: $('pageNumModal'),
  pnPosition: $('pnPosition'), pnStart: $('pnStart'), pnPad: $('pnPad'),
  pnPrefix: $('pnPrefix'), pnTotal: $('pnTotal'), pnPreview: $('pnPreview'),
  pnCancel: $('pnCancel'), pnGo: $('pnGo'),
  pageLabelsBtn: $('pageLabelsBtn'), pageLabelsModal: $('pageLabelsModal'),
  plList: $('plList'), plAdd: $('plAdd'), plPreview: $('plPreview'),
  plCancel: $('plCancel'), plGo: $('plGo'),
  nupBtn: $('nupBtn'), nupModal: $('nupModal'), nupN: $('nupN'), nupBorder: $('nupBorder'),
  normalizeBtn: $('normalizeBtn'),
  cropBtn: $('cropBtn'), cropModal: $('cropModal'), cropAllPages: $('cropAllPages'),
  cropCancel: $('cropCancel'), cropGo: $('cropGo'),
  nupCancel: $('nupCancel'), nupGo: $('nupGo'),
  exportBookmarkSplitBtn: $('exportBookmarkSplitBtn'), bookmarkSplitModal: $('bookmarkSplitModal'),
  bsPrefix: $('bsPrefix'), bsPreview: $('bsPreview'), bsDir: $('bsDir'), bsHere: $('bsHere'),
  bsUp: $('bsUp'), bsList: $('bsList'), bsCancel: $('bsCancel'), bsGo: $('bsGo'),
  exportPageSplitBtn: $('exportPageSplitBtn'), pageSplitModal: $('pageSplitModal'),
  psEvery: $('psEvery'), psRanges: $('psRanges'), psPrefix: $('psPrefix'), psPreview: $('psPreview'),
  psDir: $('psDir'), psHere: $('psHere'), psUp: $('psUp'), psList: $('psList'),
  psCancel: $('psCancel'), psGo: $('psGo'),
  splitBtn: $('splitBtn'), splitModal: $('splitModal'), splitPreview: $('splitPreview'),
  splitCols: $('splitCols'), splitRows: $('splitRows'),
  splitResize: $('splitResize'), splitCancel: $('splitCancel'), splitGo: $('splitGo'),
  splitBoxBtn: $('splitBoxBtn'), applyBoxSplitBtn: $('applyBoxSplitBtn'),
};

// Controls duplicated across the menubar and toolbar are addressed by class.
const all = (sel) => document.querySelectorAll(sel);

// --- unlock: SSH key + CSRF --------------------------------------------------
// Nib unlocks at startup from the user's SSH key. The first-run wizard
// enrolls a key (or migrates an old password vault); after that the vault opens
// with no prompt. csrf is the per-process token issued when the vault unlocks.
let csrf = null;
let authState = 'setup'; // setup | migrate | key-missing | ready
let gsAvailable = false; // Ghostscript installed (from /api/status) → offer the general PDF/A converter
let loAvailable = false; // LibreOffice installed (from /api/status) → offer office-document → PDF conversion

// apiFetch wraps fetch with the CSRF header on writes; a 401 reopens the wizard.
async function apiFetch(url, opts = {}) {
  opts.headers = { ...(opts.headers || {}) };
  if (opts.method && opts.method !== 'GET') opts.headers['X-CSRF-Token'] = csrf;
  const res = await fetch(url, opts);
  if (res.status === 401) { refreshStatus(); throw new Error('locked'); }
  return res;
}

// errText extracts the server's {error} message from a failed response, falling
// back when the body isn't JSON (proxy error, truncated) — so failure paths can
// always toast something instead of rejecting inside the error handler itself.
async function errText(res, fallback) {
  try { return (await res.json()).error || fallback; } catch { return fallback; }
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
  gsAvailable = !!st.ghostscript;
  loAvailable = !!st.libreoffice;
  // Markdown converts natively, so the button always shows; without LibreOffice
  // the picker is scoped to what the server can actually convert.
  els.officeInput.accept = loAvailable
    ? '.md,.markdown,.doc,.docx,.odt,.rtf,.txt,.xls,.xlsx,.ods,.csv,.ppt,.pptx,.odp'
    : '.md,.markdown';
  els.aboutVersion.textContent = st.version || 'dev';
  if (st.state === 'ready') {
    csrf = st.csrf;
    els.authOverlay.hidden = true;
    loadImages();
    // Apply saved preferences: theme and the auto-update toggle.
    applyAppearance(st.appearance || 'dark');
    // Saved highlight palette (most-recently-used colors); fall back to defaults.
    recentHlColors = (st.recentHighlightColors && st.recentHighlightColors.length)
      ? st.recentHighlightColors.slice(0, 5) : DEFAULT_HL_COLORS.slice();
    selectedHlColor = recentHlColors[0];
    renderHlSwatches();
    els.autoUpdateChk.checked = st.autoUpdate;
    els.autoUpdateChk.disabled = st.updateCheckLocked;
    els.autoUpdateChk.parentElement.title = st.updateCheckLocked ? 'Forced off by NIB_NO_UPDATE_CHECK' : '';
    // Always show the installed version, yellow (status unknown) until a check
    // runs; a check turns it green (up to date) or red (update available).
    showVersionBadge(st.version);
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
  els.authPwLabel.textContent = 'Current vault password';

  if (st.state === 'key-locked') {
    els.authTitle.textContent = 'Enter your key passphrase';
    els.authHint.textContent = `The SSH key that unlocks Nib (${st.keyPath || 'your key'}) is passphrase-protected. Enter its passphrase to unlock — the key stays encrypted on disk.`;
    els.authWarn.hidden = true;
    els.keyChoice.hidden = true;
    els.migrateRow.hidden = false;
    els.authPwLabel.textContent = 'SSH key passphrase';
    els.authPw.value = '';
    els.authSubmit.textContent = 'Unlock';
    els.authPw.focus();
    return;
  }

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
  // Passphrase unlock: the enrolled key is encrypted; send only the passphrase.
  if (authState === 'key-locked') {
    const res = await fetch('/api/ssh/unlock', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ passphrase: els.authPw.value }),
    });
    const st = await res.json();
    if (!res.ok) { els.authError.textContent = st.error || 'failed'; return; }
    applyStatus(st);
    return;
  }
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
  if (res.ok) applyStatus(await res.json()); else toast(await errText(res, 'restore failed'));
};

// --- update check ------------------------------------------------------------
// Runs once at startup (when autoUpdate is set) and from clicking the pill
// itself. The pill's color is its update status: yellow = unknown (no check has
// run, or it failed), green = on the latest release, red = a newer release
// exists. A pill click that finds an update offers the download in a confirm
// prompt. Nib installs nothing.
let updateChecked = false;
let updateChecking = false; // in-flight guard: a second click mustn't double-check

// showVersionBadge renders the always-on installed-version pill in its yellow
// "status unknown" state (no dismiss). This is the default: shown on load and
// after a failed check. A check that completes turns it green (up to date) or
// swaps in the red update notice (runUpdateCheck).
function showVersionBadge(version) {
  els.updatePill.classList.add('current', 'unknown');
  els.updatePill.classList.remove('latest');
  els.updateGet.textContent = `v${version || 'dev'}`;
  els.updateGet.title = `Installed version (v${version || 'dev'}) — update status unknown, click to check`;
  els.updatePill.hidden = false;
}

// startDownload fetches the release. The OS/arch asset serves as an attachment,
// so assigning location downloads in place without user activation; the release
// page (fallback when no asset matches) needs a tab — if that open is ever
// blocked, the pill stays red and a second click re-offers it.
function startDownload(d) {
  if (d.downloadUrl) {
    toast(`Downloading Nib v${d.latest}…`);
    location.assign(d.downloadUrl);
  } else {
    window.open(d.url, '_blank', 'noopener');
  }
}

// runUpdateCheck queries the server and repaints the pill. auto=true is the
// silent startup check (only colors the pill); auto=false is a pill click, which
// also toasts an up-to-date result and offers a found update for download.
async function runUpdateCheck(auto) {
  if (updateChecking) return;
  updateChecking = true;
  let d;
  try {
    const res = await fetch('/api/update/check');
    if (!res.ok) throw new Error();
    d = await res.json();
  } catch {
    if (!auto) toast('Could not check for updates.');
    return;
  } finally {
    updateChecking = false;
  }
  if (!d.updateAvailable) {
    // Up to date: the green badge with a confirmed title (and toast if manual).
    els.updatePill.classList.add('current', 'latest');
    els.updatePill.classList.remove('unknown');
    els.updateGet.textContent = `v${d.current}`;
    els.updateGet.title = d.latest ? `You’re on the latest version (v${d.current})` : `Up to date (v${d.current})`;
    els.updatePill.hidden = false;
    if (!auto) toast(d.latest ? `You’re on the latest version (v${d.current}).` : `Up to date (v${d.current}).`);
    return;
  }
  els.updatePill.classList.remove('current', 'latest', 'unknown');
  els.updateGet.title = 'A newer version is available — click to download';
  els.updateGet.textContent = `Update to v${d.latest} ↓`;
  els.updatePill.hidden = false;
  if (!auto && confirm(`Nib v${d.latest} is available (you have v${d.current}). Download it now?`)) {
    startDownload(d);
  }
}

els.updateDismiss.onclick = () => { els.updatePill.hidden = true; };
// The pill is the manual check: any click (yellow, green, or red) re-checks —
// even when the startup check is toggled off — and offers a found update.
els.updateGet.onclick = () => runUpdateCheck(false);

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
    toast(await errText(res, 'could not add key'));
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
  else toast(await errText(res, 'could not remove key'));
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
    toast(await errText(res, 'could not pin peer'));
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
  else toast(await errText(res, 'could not unpin peer'));
}

els.managePeersBtn.onclick = () => { els.peersModal.hidden = false; loadPeers(); loadExtSigner(); };

// loadExtSigner shows the imported signing certificate (if any) and toggles the
// Remove button. The status line reads the public cert — no passphrase needed.
async function loadExtSigner() {
  try {
    const info = await (await apiFetch('/api/identity/external')).json();
    if (info.present) {
      const exp = info.notAfter ? ' · expires ' + new Date(info.notAfter).toLocaleDateString() : '';
      els.extSignerStatus.textContent = 'Imported: ' + (info.subject || 'certificate') +
        (info.issuer ? ' (issued by ' + info.issuer + ')' : '') + exp;
      els.extP12Remove.hidden = false;
    } else {
      els.extSignerStatus.textContent = 'None imported — Finalize uses your Nib self-signed identity.';
      els.extP12Remove.hidden = true;
    }
  } catch { els.extSignerStatus.textContent = ''; }
}
els.extP12Import.onclick = async () => {
  const file = els.extP12File.files[0];
  if (!file) return toast('Choose a .p12 / .pfx file');
  const form = new FormData();
  form.append('p12', file, file.name);
  form.append('passphrase', els.extP12Pass.value);
  const res = await apiFetch('/api/identity/external', { method: 'POST', body: form });
  if (res.status === 401) { els.extP12Pass.focus(); return toast('Wrong passphrase, or not a PKCS#12 file'); }
  if (!res.ok) { toast(await errText(res, 'Could not import certificate')); return; }
  els.extP12Pass.value = ''; els.extP12File.value = '';
  loadExtSigner();
  toast('Signing certificate imported');
};
els.extP12Remove.onclick = async () => {
  if (!confirm('Remove the imported signing certificate? Finalize will go back to your Nib identity.')) return;
  const res = await apiFetch('/api/identity/external/remove', { method: 'POST' });
  if (!res.ok) { toast('Could not remove'); return; }
  loadExtSigner();
  toast('Imported certificate removed');
};
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
  if (!qr.ok) { toast(await errText(qr, 'could not start co-signing')); return; }
  const q = await qr.json();
  const png = await renderAttestation(q.lines, q.rect);

  const form = await bakedForm();
  form.append('params', JSON.stringify({ fingerprint, intent, when: q.when }));
  form.append('appearance', png, 'attestation.png');
  const sr = await apiFetch('/api/cosign/sign', { method: 'POST', body: form });
  if (!sr.ok) { toast(await errText(sr, 'could not co-sign')); return; }
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
  if (!qr.ok) { toast(await errText(qr, 'could not start co-signing')); return; }
  const q = await qr.json();
  const png = await renderAttestation(q.lines, q.rect);

  els.sinGo.disabled = true; els.sinCancel.disabled = true; els.sinProgress.hidden = false;
  try {
    const form = await bakedForm();
    form.append('params', JSON.stringify({ fingerprint, intent, when: q.when }));
    form.append('appearance', png, 'attestation.png');
    form.append('address', address);
    const res = await apiFetch('/api/session/initiate', { method: 'POST', body: form });
    if (!res.ok) { toast(await errText(res, 'co-signing did not complete')); return; }
    els.sessionInitModal.hidden = true;
    await setDocumentFromServer(await res.json());
    toast('Co-signed live — document updated');
  } catch (e) {
    toast('could not co-sign: ' + e.message);
  } finally {
    els.sinGo.disabled = false; els.sinCancel.disabled = false; els.sinProgress.hidden = true;
  }
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
let recvMode = 'cosign'; // cosign | receive — receive saves a one-way transfer, no signing
let recvArmedLabel = ''; // the pinned label of the peer we armed for
let recvPeerFp = ''; // the connecting peer's verified fingerprint, for the Copy button

function showRecvView(which) {
  els.srvArm.hidden = which !== 'srvArm';
  els.srvWait.hidden = which !== 'srvWait';
  els.srvConsent.hidden = which !== 'srvConsent';
}

// openSessionRecv arms the receive modal in one of two modes: 'cosign' (review and
// co-sign a peer's document) or 'receive' (accept a one-way transfer and save it to
// ~/nib). The two share the arm/wait/consent machinery; only the consent chrome and
// the accept action differ.
async function openSessionRecv(mode) {
  recvMode = mode === 'receive' ? 'receive' : 'cosign';
  const receive = recvMode === 'receive';
  els.srvTitle.textContent = receive ? 'Receive a document' : 'Receive a live co-signature';
  els.srvIntentRow.hidden = receive; // a plain transfer needs no agreement statement
  els.srvReasonCap.textContent = receive ? 'What they’re sending' : 'Their signed statement';
  els.srvAccept.textContent = receive ? 'Accept & save' : 'Accept & co-sign';
  els.srvArmHint.textContent = receive
    ? 'Wait for a pinned peer to connect and send you a document over a live, encrypted channel. You review it and decide before it’s saved into ~/nib.'
    : 'Wait for a pinned peer to connect and send you a document to co-sign over a live, encrypted channel. You review it and decide before anything is signed.';
  els.srvConsentHint.textContent = receive
    ? 'A peer connected and sent a document. Review exactly what you’ll keep, then accept to save it or decline.'
    : 'A peer connected and sent a document. Review exactly what you’ll co-sign, then accept or decline.';
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
    body: JSON.stringify({ fingerprint: opt.value, bind, mode: recvMode }),
  });
  els.srvArmGo.disabled = false;
  if (!res.ok) { toast(await errText(res, 'could not arm')); return; }
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
    if (recvStage === 'applying' && recvMode === 'receive') {
      toast(st.received ? 'Saved ' + st.received.path : 'Document received');
    } else if (recvStage === 'applying') { await reloadOpenDoc(); toast('Co-signed — document updated'); }
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
  // This function is the doc's sole holder, so it destroys it on every exit —
  // otherwise each consent preview leaks a worker-side document.
  try {
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
  } finally {
    doc.loadingTask.destroy().catch(() => {});
  }
}

async function acceptRecv() {
  els.srvAccept.disabled = true; els.srvDecline.disabled = true;
  // A one-way transfer is just consent to keep the file — no signing, no appearance.
  if (recvMode === 'receive') {
    const res = await apiFetch('/api/session/respond', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ accept: true }),
    });
    if (!res.ok) {
      els.srvAccept.disabled = false; els.srvDecline.disabled = false;
      toast(await errText(res, 'could not accept'));
      return;
    }
    recvStage = 'applying';
    toast('Saving…');
    return; // pollRecv detects the disarm and reports where it landed
  }
  const intent = els.srvIntent.value;
  // The responder's visible block is placed server-side; the quote gives only the
  // canonical lines, so the rendered image can't drift from the signed /Reason.
  let appearance = '';
  const qr = await apiFetch('/api/session/quote', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ intent }),
  });
  if (!qr.ok) {
    // Proceeding would co-sign without the visible attestation block; abort so
    // the user can retry rather than silently sign with no on-page trace.
    els.srvAccept.disabled = false; els.srvDecline.disabled = false;
    toast(await errText(qr, 'could not prepare the signature block'));
    return;
  }
  const q = await qr.json();
  appearance = await blobToBase64(await renderAttestation(q.lines, q.rect));
  const res = await apiFetch('/api/session/respond', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ accept: true, intent, appearance }),
  });
  if (!res.ok) {
    els.srvAccept.disabled = false; els.srvDecline.disabled = false;
    toast(await errText(res, 'could not co-sign'));
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

els.sessionRecvBtn.onclick = () => openSessionRecv('cosign');
els.sessionRecvDocBtn.onclick = () => openSessionRecv('receive');
els.srvCancel.onclick = cancelRecv;
els.srvDisarm.onclick = cancelRecv;
els.srvArmGo.onclick = armRecv;
els.srvAccept.onclick = acceptRecv;
els.srvDecline.onclick = declineRecv;
els.srvSelfCopy.onclick = () => copyFp(selfFingerprint);
els.srvPeerCopy.onclick = () => copyFp(recvPeerFp);

// --- send a document to a peer (one-way transfer) ----------------------------
// Hand the open document to a pinned, armed peer over the live channel — they
// review and keep it. Replaces emailing the for-signing or signed file. The
// document goes out as the same bytes a save would: baked, with the placed flags
// embedded when there are any (so a flagged file opens in signing mode for them).
async function openSessionSend() {
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  const res = await apiFetch('/api/peers');
  if (!res.ok) { toast('could not load peers'); return; }
  const data = await res.json();
  const peers = data.peers || [];
  els.ssnPeer.innerHTML = '';
  for (const p of peers) {
    const o = document.createElement('option');
    o.value = p.fingerprint;
    o.textContent = (p.label || 'Unlabelled peer') + ' — ' + groupFingerprint(p.fingerprint.slice(0, 8)) + '…';
    els.ssnPeer.append(o);
  }
  const none = peers.length === 0;
  els.ssnNoPeers.hidden = !none;
  els.ssnPeer.hidden = none;
  els.ssnGo.disabled = none;
  els.ssnProgress.hidden = true;
  els.sessionSendModal.hidden = false;
}

// sendableForm is the body /api/session/send expects: the baked document, with the
// currently-placed flags embedded when any markers exist — the same hand-off bytes
// "Save for signing…" produces, routed to a peer instead of a file.
async function sendableForm() {
  let bytes = await bakedBytes();
  const flags = collectFlags();
  if (flags.length) bytes = await embedFlags(bytes, flags);
  const form = new FormData();
  form.append('pdf', new Blob([bytes], { type: 'application/pdf' }), 'doc.pdf');
  return form;
}

async function sendToPeer() {
  const opt = els.ssnPeer.selectedOptions[0];
  if (!opt) return;
  const address = els.ssnAddr.value.trim();
  if (!address) { toast('Enter the peer address'); return; }
  els.ssnGo.disabled = true; els.ssnProgress.hidden = false;
  try {
    const form = await sendableForm();
    form.append('fingerprint', opt.value);
    form.append('address', address);
    const res = await apiFetch('/api/session/send', { method: 'POST', body: form });
    if (!res.ok) { toast(await errText(res, 'could not send')); return; }
    const r = await res.json();
    toast(r.declined ? 'The peer declined the document' : 'Sent — the peer has the document');
    els.sessionSendModal.hidden = true;
  } catch (e) {
    toast('could not send: ' + e.message);
  } finally {
    els.ssnGo.disabled = false; els.ssnProgress.hidden = true;
  }
}

els.sessionSendBtn.onclick = openSessionSend;
els.ssnCancel.onclick = () => { els.sessionSendModal.hidden = true; };
els.ssnGo.onclick = sendToPeer;

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
els.aboutLicenseBtn.onclick = () => showAboutDoc('Licence (AGPLv3)', '/legal/LICENSE');
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
  imageResourcesPath: './vendor/pdfjs/images/', // sticky-note icons resolve here (see annotation-note.svg)
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

eventBus.on('pagesinit', () => {
  viewer.currentScaleValue = 'page-width'; // immediate fit (page 1) so there's no 100%-then-fit flash…
});
// …then refine to the widest page once every page view is populated and the layout
// has settled (pagesinit is too early — see fitWidestWidth).
eventBus.on('pagesloaded', fitWidestWidth);
eventBus.on('pagechanging', (e) => {
  all('.pageNum').forEach((i) => { i.value = e.pageNumber; });
  markCurrentThumb(e.pageNumber);
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
    // An encrypted PDF needs its open password before pdf.js can render it; prompt
    // for it and unlock the working copy rather than dead-end on a generic error.
    if (e && e.name === 'PasswordException') { openDecryptPrompt(); return; }
    toast('could not render the document');
    console.error('pdf load failed', e);
    return;
  }
  if (gen !== docGen) return; // a newer load superseded this one

  const old = pdfDocument;
  pdfDocument = doc;
  viewer.setDocument(pdfDocument);
  linkService.setDocument(pdfDocument, null);
  // Free the superseded document's worker-side resources once the viewer and
  // link service have been repointed — without this, every edit op (each of
  // which reloads through here) orphans a document for the session's lifetime.
  if (old && old !== doc) old.loadingTask.destroy().catch(() => {});
  // A fresh document gets a new pdf.js editor manager; re-assert the chosen
  // highlight color so it sticks across reloads (page ops) until the user changes it.
  applyHighlightColor(selectedHlColor);

  els.viewerWrap.classList.add('has-doc');
  all('.pageCount').forEach((s) => { s.textContent = '/ ' + pdfDocument.numPages; });
  els.saveBtn.disabled = false;
  setDocControls(true);
  els.saveBtn.title = meta.canSave ? 'Save (overwrites ' + meta.path + ')' : 'Save a copy (downloads — opened without a local path)';
  updateBadge(meta.signature);
  // Rebuild any embedded signing flags as markers and offer the signing flow.
  docHadFlags = Array.isArray(meta.flags) && meta.flags.length > 0;
  signLocked = docHadFlags; // a received signing document opens locked, non-editable
  els.signBanner.hidden = true;
  reconstructFlags(meta.flags);
  applySignLock();
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
  if (!res.ok) return toast(await errText(res, 'could not open file'));
  await setDocumentFromServer(await res.json());
}

async function uploadFile(file) {
  const form = new FormData();
  form.append('file', file);
  const res = await apiFetch('/api/upload', { method: 'POST', body: form });
  if (!res.ok) return toast(await errText(res, 'could not open file'));
  await setDocumentFromServer(await res.json());
}

// Office → PDF: pick a Word/Excel/PowerPoint/OpenDocument file, convert it on the
// server via LibreOffice, and open the resulting PDF as the active document. Only
// offered when LibreOffice is installed (the button is hidden otherwise).
els.officeOpenBtn.onclick = () => els.officeInput.click();
els.officeInput.onchange = async () => {
  const file = els.officeInput.files[0];
  els.officeInput.value = '';
  if (!file) return;
  const form = new FormData();
  form.append('file', file);
  toast('Converting to PDF…');
  try {
    const res = await apiFetch('/api/office', { method: 'POST', body: form });
    if (!res.ok) return toast(await errText(res, 'could not convert file'));
    await setDocumentFromServer(await res.json());
  } catch {
    toast('could not convert file');
  }
};

async function openURL(url) {
  const res = await apiFetch('/api/open-url', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
  });
  if (!res.ok) return toast(await errText(res, 'could not fetch URL'));
  originalName = (url.split('/').pop() || '').split('?')[0] || 'document.pdf';
  await setDocumentFromServer(await res.json());
}

// --- combine several PDFs into one new document ------------------------------
// Pick files, order them with ↑/↓, and merge top-to-bottom into a fresh document
// (page-level reorder is then the existing thumbnail drag). Posts every file under
// the repeated "file" field in list order; the server preserves that order.
let combineFiles = [];
function renderCombineList() {
  els.combineList.innerHTML = '';
  if (!combineFiles.length) {
    const p = document.createElement('p');
    p.className = 'scan-empty';
    p.textContent = 'Add two or more PDFs to combine.';
    els.combineList.appendChild(p);
  }
  combineFiles.forEach((f, i) => {
    const row = document.createElement('div');
    row.className = 'combinerow';
    const meta = document.createElement('div');
    meta.className = 'keymeta';
    const name = document.createElement('div');
    name.className = 'keyfp';
    name.textContent = `${i + 1}. ${f.name}`;
    name.title = f.name;
    meta.appendChild(name);
    const ctrls = document.createElement('div');
    ctrls.className = 'combinectrls';
    const up = document.createElement('button');
    up.textContent = '↑'; up.title = 'Move up'; up.disabled = i === 0;
    up.onclick = () => { [combineFiles[i - 1], combineFiles[i]] = [combineFiles[i], combineFiles[i - 1]]; renderCombineList(); };
    const down = document.createElement('button');
    down.textContent = '↓'; down.title = 'Move down'; down.disabled = i === combineFiles.length - 1;
    down.onclick = () => { [combineFiles[i + 1], combineFiles[i]] = [combineFiles[i], combineFiles[i + 1]]; renderCombineList(); };
    const del = document.createElement('button');
    del.className = 'keydel'; del.textContent = 'Remove';
    del.onclick = () => { combineFiles.splice(i, 1); renderCombineList(); };
    ctrls.append(up, down, del);
    row.append(meta, ctrls);
    els.combineList.appendChild(row);
  });
  els.combineGo.disabled = combineFiles.length < 2;
}
els.combineBtn.onclick = () => {
  combineFiles = [];
  renderCombineList();
  els.combineModal.hidden = false;
};
els.combineCancel.onclick = () => { els.combineModal.hidden = true; combineFiles = []; };
els.combineAddBtn.onclick = () => els.combineInput.click();
els.combineInput.onchange = () => {
  for (const f of els.combineInput.files) combineFiles.push(f);
  els.combineInput.value = '';
  renderCombineList();
};
els.combineGo.onclick = async () => {
  if (combineFiles.length < 2) return;
  const form = new FormData();
  for (const f of combineFiles) form.append('file', f, f.name);
  const res = await apiFetch('/api/combine', { method: 'POST', body: form });
  if (!res.ok) return toast('could not combine the PDFs');
  await setDocumentFromServer(await res.json());
  els.combineModal.hidden = true;
  combineFiles = [];
  toast('Combined — reorder pages by dragging thumbnails, then Save As to keep it');
};

// --- Compare two PDFs --------------------------------------------------------
// All client-side: pdf.js is the only place a PDF's text layer and rendered
// pixels exist (the Go engine can't extract either). One modal, three modes:
//   Text         — word-diff the two text layers with jsdiff (what changed).
//   Side-by-side — render the same page of both documents next to each other.
//   Differences  — pixel-diff the rendered pages with pixelmatch (where it
//                  changed); works on scans too, since it compares pixels.
// documentText is the shared content-stream-order dump also used by "Export
// text" — deliberately NOT geometry-sorted (re-sorting scrambles columns),
// which keeps the diff reliable for two versions from the same producer.
// pageTexts returns each page's text in content-stream order (image-only pages →
// ""). It is the single extraction pass behind both documentText (joined) and the
// per-page fingerprints used for auto page-matching.
async function pageTexts(doc) {
  const out = [];
  for (let n = 1; n <= doc.numPages; n++) {
    let s = '';
    try {
      const tc = await (await doc.getPage(n)).getTextContent();
      for (const it of tc.items) { s += it.str; if (it.hasEOL) s += '\n'; }
    } catch { /* image-only page: no text layer */ }
    out.push(s);
  }
  return out;
}

async function documentText(doc) {
  return (await pageTexts(doc)).join('\n') + '\n';
}

// pageFingerprints normalizes each page's text into a comparison key. A blank or
// text-less page gets a per-document, per-index sentinel so it can never match
// another blank across the two documents (which would mis-pair). meaningful is
// the count of pages that carry real text.
function pageFingerprints(texts, tag) {
  let meaningful = 0;
  const keys = texts.map((t, i) => {
    const norm = t.replace(/\s+/g, ' ').trim().toLowerCase();
    if (norm === '') return `\x00empty:${tag}:${i}`;
    meaningful++;
    return norm;
  });
  return { keys, meaningful };
}

// alignPages aligns two page-key sequences via a longest-common-subsequence pass,
// returning ordered steps {a,b} of 1-based page numbers — with b=null for a page
// only in the open document (deleted) and a=null for one only in the compared
// document (added). Page counts are small, so the O(m·n) table is trivial. eq is
// the match test: exact equality for text fingerprints, or a perceptual-hash
// distance threshold for scans — within-threshold pairs become aligned diagonals,
// everything else a gap, which is exactly LCS with a fuzzy equality predicate.
function alignPages(ka, kb, eq = (x, y) => x === y) {
  const m = ka.length, n = kb.length;
  const dp = Array.from({ length: m + 1 }, () => new Int32Array(n + 1));
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      dp[i][j] = eq(ka[i], kb[j]) ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const steps = [];
  let i = 0, j = 0;
  while (i < m && j < n) {
    if (eq(ka[i], kb[j])) { steps.push({ a: i + 1, b: j + 1 }); i++; j++; }
    else if (dp[i + 1][j] >= dp[i][j + 1]) { steps.push({ a: i + 1, b: null }); i++; }
    else { steps.push({ a: null, b: j + 1 }); j++; }
  }
  while (i < m) { steps.push({ a: i + 1, b: null }); i++; }
  while (j < n) { steps.push({ a: null, b: j + 1 }); j++; }
  return steps;
}

// detectMoves layers move detection over the order-preserving alignment: LCS can
// only show a reordered page as a delete on one side plus an insert on the other,
// so this matches each removed page (b=null, an A-page) to an unused added page
// (a=null, a B-page) by the same eq, and annotates the pair as a move
// (movedTo/movedFrom carry the partner's 1-based page). Greedy, each added gap used
// once; gaps with no counterpart stay genuine deletes/inserts. Mutates and returns
// steps. ka/kb are the same key arrays alignPages aligned on.
function detectMoves(steps, ka, kb, eq = (x, y) => x === y) {
  const added = steps.filter((s) => s.a == null); // B-only pages, candidates to move *into*
  for (const rem of steps) {
    if (rem.b != null) continue; // not a removed page
    const hit = added.find((s) => s.movedFrom == null && eq(ka[rem.a - 1], kb[s.b - 1]));
    if (hit) { rem.movedTo = hit.b; hit.movedFrom = rem.a; }
  }
  return steps;
}

// --- Perceptual page hashing: align two scans (or a scan vs a digital PDF) when
// neither side has text to fingerprint. dHash renders each page tiny, reduces it
// to a 9×8 grayscale grid, and records whether each pixel is brighter than its
// right neighbour — 64 bits robust to scan noise, slight skew, and resampling.
const DHASH_T = 12; // max Hamming distance (of 64) for two pages to count as "the same page"

// pageDHash rasterises one page small and returns its 64-bit dHash as 8 bytes.
async function pageDHash(doc, n) {
  const { canvas } = await renderPageCanvas(doc, n, 0.25); // small render; downscaled again below
  const g = document.createElement('canvas');
  g.width = 9; g.height = 8;
  const ctx = g.getContext('2d');
  ctx.drawImage(canvas, 0, 0, 9, 8); // box-filter down to the dHash grid
  const px = ctx.getImageData(0, 0, 9, 8).data;
  const lum = (i) => 0.299 * px[i] + 0.587 * px[i + 1] + 0.114 * px[i + 2];
  const bytes = new Uint8Array(8);
  let bit = 0;
  for (let y = 0; y < 8; y++) {
    for (let x = 0; x < 8; x++) {
      const here = lum((y * 9 + x) * 4), right = lum((y * 9 + x + 1) * 4);
      if (here > right) bytes[bit >> 3] |= 1 << (bit & 7);
      bit++;
    }
  }
  return bytes;
}

// pagePixelHashes hashes every page of a doc, reporting progress as it goes.
async function pagePixelHashes(doc, onProgress) {
  const hashes = [];
  for (let n = 1; n <= doc.numPages; n++) {
    hashes.push(await pageDHash(doc, n));
    if (onProgress) onProgress();
  }
  return hashes;
}

// hamming counts differing bits between two equal-length byte arrays.
function hamming(a, b) {
  let d = 0;
  for (let i = 0; i < a.length; i++) {
    let v = a[i] ^ b[i];
    while (v) { d += v & 1; v >>= 1; }
  }
  return d;
}

const CMP_SCALE = 2; // 144 DPI — matches the flatten/redact raster scale
let cmpDoc = null;   // the picked pdf.js doc, kept alive across page nav until close
let cmpName = '';    // picked filename (shown as a caption / in summaries)
let cmpMode = 'text';// 'text' | 'side' | 'diff'
let cmpPageA = 1, cmpPageB = 1; // independent pages (visual modes) so an inserted/
                                // deleted page can be re-aligned across the shift
let cmpText = null;  // cached {a,b} text dumps so mode-switching doesn't re-extract
let cmpSeq = 0;      // render token — discards a stale paint if the user flips fast
let cmpAlign = null; // [{a,b}] auto page-matching steps (null until computed / when not meaningful)
let cmpAlignIdx = 0; // position within cmpAlign for lockstep nav
let cmpAlignTried = false; // whether fingerprinting+alignment has run for this pair

function closeCmpDoc() {
  cmpSeq++; // teardown participates in the render token: in-flight aligns/paints bail instead of reading the nulled doc
  if (cmpDoc) { cmpDoc.loadingTask.destroy(); cmpDoc = null; } // free the comparison doc (destroy lives on the loading task, not the proxy)
  cmpText = null; cmpName = ''; cmpPageA = 1; cmpPageB = 1; cmpMode = 'text';
  cmpAlign = null; cmpAlignIdx = 0; cmpAlignTried = false;
  if (els.cmAuto) els.cmAuto.checked = true;
}

// autoActive reports whether auto page-matching is currently driving navigation:
// the user hasn't turned it off and an alignment was computed (both docs have text).
function autoActive() { return els.cmAuto.checked && cmpAlign && cmpAlign.length > 0; }

// alignStat summarises the alignment for the toolbar: "+added −removed ⇄moved"
// (added = pages only in the compared doc, removed = pages only in the open doc,
// moved = a page that simply changed position), or a "pages aligned" note when the
// pagination matches 1:1. A moved page is counted once and excluded from add/remove.
function alignStat() {
  let added = 0, removed = 0, moved = 0;
  for (const s of cmpAlign) {
    if (s.movedTo != null) moved++;          // counted once, on the A-side step
    else if (s.movedFrom != null) continue;  // the B-side of a move, already counted
    else if (s.a == null) added++;
    else if (s.b == null) removed++;
  }
  if (!added && !removed && !moved) return 'pages aligned';
  return [added ? `+${added}` : '', removed ? `−${removed}` : '', moved ? `⇄${moved}` : ''].filter(Boolean).join(' ');
}

// ensureAlignment lazily aligns the two documents once per pair. Text PDFs align
// instantly on per-page text fingerprints (LCS). When a side has no text to
// fingerprint (a scan, or a scan vs a digital PDF) it falls back to rasterising
// every page and aligning on perceptual hashes within a Hamming threshold — a
// render pass, so it shows progress. Only if rendering fails does nav go manual.
async function ensureAlignment() {
  if (cmpAlignTried) return;
  cmpAlignTried = true;
  if (!cmpDoc) return;
  const seq = cmpSeq; // closeCmpDoc bumps this; bail after each await rather than read the nulled doc
  const [ta, tb] = await Promise.all([pageTexts(pdfDocument), pageTexts(cmpDoc)]);
  if (seq !== cmpSeq) return;
  const a = pageFingerprints(ta, 'a'), b = pageFingerprints(tb, 'b');
  if (a.meaningful > 0 && b.meaningful > 0) {
    cmpAlign = detectMoves(alignPages(a.keys, b.keys), a.keys, b.keys);
  } else {
    try {
      const total = pdfDocument.numPages + cmpDoc.numPages;
      let done = 0;
      const tick = () => { els.compareBody.innerHTML = `<p class="scan-where">Aligning pages… ${++done}/${total}</p>`; };
      const ha = await pagePixelHashes(pdfDocument, tick);
      if (seq !== cmpSeq) return;
      const hb = await pagePixelHashes(cmpDoc, tick);
      if (seq !== cmpSeq) return;
      const near = (x, y) => hamming(x, y) <= DHASH_T;
      cmpAlign = detectMoves(alignPages(ha, hb, near), ha, hb, near);
    } catch {
      cmpAlign = null;
      els.cmAuto.checked = false; // couldn't render to hash → manual paging only
    }
  }
  els.cmAuto.disabled = !cmpAlign;
}
function openCompare() {
  if (!pdfDocument) return toast('Open a PDF first');
  closeCmpDoc();
  els.compareTools.hidden = true;
  els.compareSummary.hidden = true;
  els.compareBody.innerHTML = '<p class="scan-where">Choose a PDF to compare against the open document.</p>';
  els.compareModal.hidden = false;
}
els.compareBtn.onclick = openCompare;
els.compareClose.onclick = () => { els.compareModal.hidden = true; closeCmpDoc(); };
els.comparePick.onclick = () => els.compareInput.click();
els.compareInput.onchange = async () => {
  const f = els.compareInput.files[0];
  els.compareInput.value = '';
  if (!f) return;
  closeCmpDoc();
  els.compareBody.innerHTML = '<p class="scan-where">Loading…</p>';
  try {
    const buf = new Uint8Array(await f.arrayBuffer());
    cmpDoc = await pdfjsLib.getDocument({ data: buf }).promise;
  } catch {
    els.compareBody.innerHTML = '<p class="scan-where">Could not read that PDF.</p>';
    return;
  }
  cmpName = f.name;
  els.compareTools.hidden = false;
  setCompareMode('text'); // default to the text diff (instant; preserves prior behaviour)
};

// The mode toolbar: switch view, re-rendering the current page for the visual modes.
for (const btn of document.querySelectorAll('.cmmode')) {
  btn.onclick = () => setCompareMode(btn.dataset.mode);
}
// Lockstep prev/next: with auto-align on, walk the computed page pairing (so an
// inserted/deleted page stays aligned); otherwise step both documents together.
els.cmPrev.onclick = () => lockstep(-1);
els.cmNext.onclick = () => lockstep(1);
// The per-side ‹ › steppers are the manual escape hatch — they only act when
// auto-align is off (disabled while it's on; uncheck Auto-align to nudge a side).
els.cmAPrev.onclick = () => stepManual(-1, 0);
els.cmANext.onclick = () => stepManual(1, 0);
els.cmBPrev.onclick = () => stepManual(0, -1);
els.cmBNext.onclick = () => stepManual(0, 1);
els.cmAuto.onchange = () => { cmpAlignIdx = 0; renderCompareVisual(cmpMode); };

function lockstep(dir) {
  if (autoActive()) {
    cmpAlignIdx = Math.min(Math.max(cmpAlignIdx + dir, 0), cmpAlign.length - 1);
  } else {
    cmpPageA = Math.min(Math.max(cmpPageA + dir, 1), pdfDocument.numPages);
    cmpPageB = Math.min(Math.max(cmpPageB + dir, 1), cmpDoc.numPages);
  }
  renderCompareVisual(cmpMode);
}

function stepManual(da, db) {
  cmpPageA = Math.min(Math.max(cmpPageA + da, 1), pdfDocument.numPages);
  cmpPageB = Math.min(Math.max(cmpPageB + db, 1), cmpDoc.numPages);
  renderCompareVisual(cmpMode);
}

function setCompareMode(mode) {
  cmpMode = mode;
  for (const btn of document.querySelectorAll('.cmmode')) {
    btn.classList.toggle('active', btn.dataset.mode === mode);
  }
  els.comparePager.hidden = (mode === 'text');
  if (mode === 'text') renderCompareText();
  else renderCompareVisual(mode);
}

// Text mode: extract both text layers once (cached), then word-diff via renderCompare.
async function renderCompareText() {
  els.compareSummary.hidden = true;
  if (!cmpText) {
    els.compareBody.innerHTML = '<p class="scan-where">Comparing text…</p>';
    cmpText = { a: await documentText(pdfDocument), b: await documentText(cmpDoc) };
  }
  renderCompare(cmpText.a, cmpText.b, cmpName);
}

// Visual modes: rasterise the selected page of each document (lazily, one page at
// a time) and either show them side by side or paint a pixelmatch difference map.
// The two pages are chosen independently (cmpPageA/cmpPageB) so an inserted or
// deleted page can be re-aligned; a size-mismatched pair is shown without a diff.
async function renderCompareVisual(mode) {
  const seq = ++cmpSeq;
  if (!cmpAlignTried) {
    els.compareSummary.hidden = true;
    els.compareBody.innerHTML = '<p class="scan-where">Aligning pages…</p>';
  }
  await ensureAlignment();
  if (seq !== cmpSeq) return; // a newer render started while fingerprinting
  const nA = pdfDocument.numPages, nB = cmpDoc.numPages;
  const auto = autoActive();

  // In auto mode the current alignment step drives which pages show; a gap step
  // (one side null) means a page was added or removed and is shown on its own.
  let step = null;
  if (auto) {
    cmpAlignIdx = Math.min(Math.max(cmpAlignIdx, 0), cmpAlign.length - 1);
    step = cmpAlign[cmpAlignIdx];
    if (step.a != null) cmpPageA = step.a;
    if (step.b != null) cmpPageB = step.b;
  }
  cmpPageA = Math.min(Math.max(cmpPageA, 1), nA);
  cmpPageB = Math.min(Math.max(cmpPageB, 1), nB);

  els.cmPageLabelA.textContent = step && step.a == null ? `— / ${nA}` : `${cmpPageA} / ${nA}`;
  els.cmPageLabelB.textContent = step && step.b == null ? `— / ${nB}` : `${cmpPageB} / ${nB}`;
  els.cmPrev.disabled = auto ? cmpAlignIdx <= 0 : (cmpPageA <= 1 && cmpPageB <= 1);
  els.cmNext.disabled = auto ? cmpAlignIdx >= cmpAlign.length - 1 : (cmpPageA >= nA && cmpPageB >= nB);
  els.cmAPrev.disabled = auto || cmpPageA <= 1; els.cmANext.disabled = auto || cmpPageA >= nA;
  els.cmBPrev.disabled = auto || cmpPageB <= 1; els.cmBNext.disabled = auto || cmpPageB >= nB;
  els.cmAlignStat.textContent = auto ? alignStat() : '';

  // A gap step renders the single present page with a banner. A page that simply
  // changed position is labelled as a move (cross-referencing its other position)
  // rather than a delete/insert, so a reorder doesn't read as lost + new content.
  if (step && step.b == null) {
    const ra = await renderPageCanvas(pdfDocument, cmpPageA, CMP_SCALE);
    if (seq !== cmpSeq) return;
    els.compareSummary.hidden = false;
    els.compareSummary.textContent = step.movedTo != null
      ? `Page ${cmpPageA} moved to page ${step.movedTo} of ${cmpName}.`
      : `Page ${cmpPageA} is only in the open document (removed from ${cmpName}).`;
    els.compareBody.textContent = '';
    showCompareCanvases([[`Open document — page ${cmpPageA}`, ra.canvas]]);
    return;
  }
  if (step && step.a == null) {
    const rb = await renderPageCanvas(cmpDoc, cmpPageB, CMP_SCALE);
    if (seq !== cmpSeq) return;
    els.compareSummary.hidden = false;
    els.compareSummary.textContent = step.movedFrom != null
      ? `Page ${cmpPageB} of ${cmpName} moved from page ${step.movedFrom} of the open document.`
      : `Page ${cmpPageB} was added in ${cmpName} (not in the open document).`;
    els.compareBody.textContent = '';
    showCompareCanvases([[`${cmpName} — page ${cmpPageB}`, rb.canvas]]);
    return;
  }

  const [ra, rb] = await Promise.all([
    renderPageCanvas(pdfDocument, cmpPageA, CMP_SCALE),
    renderPageCanvas(cmpDoc, cmpPageB, CMP_SCALE),
  ]);
  if (seq !== cmpSeq) return;

  const capA = `Open document — page ${cmpPageA}`, capB = `${cmpName} — page ${cmpPageB}`;
  const countNote = nA !== nB ? `Documents have different page counts (${nA} vs ${nB}).` : '';
  let summary = '', items;
  if (mode === 'side') {
    summary = countNote;
    items = [[capA, ra.canvas], [capB, rb.canvas]];
  } else if (ra.canvas.width !== rb.canvas.width || ra.canvas.height !== rb.canvas.height) {
    summary = `Page sizes differ (${Math.round(ra.w)}×${Math.round(ra.h)} vs ${Math.round(rb.w)}×${Math.round(rb.h)} pt) — can’t build a difference map. Normalise page sizes first; showing both pages.`;
    items = [[capA, ra.canvas], [capB, rb.canvas]];
  } else {
    const w = ra.canvas.width, h = ra.canvas.height;
    const da = ra.canvas.getContext('2d').getImageData(0, 0, w, h);
    const db = rb.canvas.getContext('2d').getImageData(0, 0, w, h);
    const out = document.createElement('canvas');
    out.width = w; out.height = h;
    const octx = out.getContext('2d');
    const od = octx.createImageData(w, h);
    const changed = pixelmatch(da.data, db.data, od.data, w, h, { threshold: 0.1, alpha: 0.15 });
    octx.putImageData(od, 0, 0);
    const pct = 100 * changed / (w * h);
    const diffMsg = changed === 0
      ? 'No visible differences between these pages.'
      : `${changed.toLocaleString()} pixels changed (${pct < 0.1 ? pct.toFixed(2) : pct.toFixed(1)}%), highlighted in red.`;
    summary = countNote ? `${diffMsg} ${countNote}` : diffMsg;
    items = [[`Differences — open page ${cmpPageA} vs ${cmpName} page ${cmpPageB}`, out]];
  }
  els.compareSummary.hidden = !summary;
  els.compareSummary.textContent = summary;
  els.compareBody.textContent = '';
  showCompareCanvases(items);
}

// Lay labelled rendered pages into the compare body. Canvas pixels are the 2×
// raster; CSS scales each down to fit. Labels go in via textContent (filenames
// could otherwise inject markup).
function showCompareCanvases(items) {
  const row = document.createElement('div');
  row.className = 'comparecanvasrow' + (items.length > 1 ? ' two' : '');
  for (const [label, canvas] of items) {
    const col = document.createElement('figure');
    col.className = 'comparecol';
    const cap = document.createElement('figcaption');
    cap.textContent = label;
    canvas.classList.add('comparecanvas');
    col.appendChild(cap);
    col.appendChild(canvas);
    row.appendChild(col);
  }
  els.compareBody.appendChild(row);
}

// --- fill from spreadsheet (CSV mail-merge) ----------------------------------
// Surfaces `nib fill`'s CSV mail-merge in the GUI: post the open form template
// (AcroForm fields intact — bakedBytes preserves them) plus the CSV, and the
// server fills one PDF per row and returns them as a ZIP. Reuses pdfops.FillFormCSV
// unchanged; the only new server code is the zip-bundling handler.
function openFillCsv() {
  if (!pdfDocument) return toast('Open a form first');
  els.fillCsvStatus.textContent = '';
  els.fillCsvModal.hidden = false;
}
els.fillCsvBtn.onclick = openFillCsv;
els.fillCsvClose.onclick = () => { els.fillCsvModal.hidden = true; };
els.fillCsvPick.onclick = () => els.fillCsvInput.click();
els.fillCsvInput.onchange = async () => {
  const f = els.fillCsvInput.files[0];
  els.fillCsvInput.value = '';
  if (!f) return;
  els.fillCsvStatus.textContent = 'Merging…';
  let csv;
  try { csv = await f.text(); } catch { els.fillCsvStatus.textContent = 'Could not read that file.'; return; }
  try {
    const form = new FormData();
    form.append('pdf', new Blob([await bakedBytes()], { type: 'application/pdf' }), 'doc.pdf');
    form.append('data', csv);
    const res = await apiFetch('/api/form/fill-csv', { method: 'POST', body: form });
    if (!res.ok) {
      els.fillCsvStatus.textContent = 'Merge failed — does this PDF have fillable fields, and do the CSV headers match the field names?';
      return;
    }
    els.fillCsvModal.hidden = true;
    openSaveAs(await res.blob(), exportBase() + '-filled.zip', 'Save merged PDFs (ZIP)');
  } catch (e) {
    els.fillCsvStatus.textContent = 'Merge failed — ' + e.message;
  }
};

// --- import form data (XFDF) -------------------------------------------------
// Surfaces `nib fill --data x.xfdf` in the GUI: post the open form template
// (AcroForm fields intact — bakedBytes preserves them) plus the XFDF, and the
// server fills one PDF and returns it. Reuses pdfops.FillFormXFDF unchanged.
function openImportXfdf() {
  if (!pdfDocument) return toast('Open a form first');
  els.importXfdfStatus.textContent = '';
  els.importXfdfModal.hidden = false;
}
els.importXfdfBtn.onclick = openImportXfdf;
els.importXfdfClose.onclick = () => { els.importXfdfModal.hidden = true; };
els.importXfdfPick.onclick = () => els.importXfdfInput.click();
els.importXfdfInput.onchange = async () => {
  const f = els.importXfdfInput.files[0];
  els.importXfdfInput.value = '';
  if (!f) return;
  els.importXfdfStatus.textContent = 'Filling…';
  let xfdf;
  try { xfdf = await f.text(); } catch { els.importXfdfStatus.textContent = 'Could not read that file.'; return; }
  try {
    const form = new FormData();
    form.append('pdf', new Blob([await bakedBytes()], { type: 'application/pdf' }), 'doc.pdf');
    form.append('data', xfdf);
    const res = await apiFetch('/api/form/fill-xfdf', { method: 'POST', body: form });
    if (!res.ok) {
      els.importXfdfStatus.textContent = 'Fill failed — does this PDF have fillable fields, and do the XFDF field names match?';
      return;
    }
    els.importXfdfModal.hidden = true;
    openSaveAs(await res.blob(), exportBase() + '-filled.pdf', 'Save filled PDF');
  } catch (e) {
    els.importXfdfStatus.textContent = 'Fill failed — ' + e.message;
  }
};

// --- convert to PDF/A-2b archival candidate ----------------------------------
// Surfaces `nib pdfa` in the GUI: post the current document (overlay edits baked
// in) and the server injects the sRGB OutputIntent + PDF/A XMP. The server refuses
// documents with non-embedded fonts (or encryption) with a specific 400 message
// shown in the modal. The result is a candidate — the modal says "verify with
// veraPDF" because Nib can't validate conformance itself.
function openPdfa() {
  if (!pdfDocument) return toast('Open a PDF first');
  els.pdfaStatus.textContent = '';
  els.pdfaGsGo.hidden = true; // revealed only when the pure-Go path refuses and gs is installed
  els.pdfaModal.hidden = false;
}
els.pdfaBtn.onclick = openPdfa;
els.pdfaClose.onclick = () => { els.pdfaModal.hidden = true; };
els.pdfaGo.onclick = () => runPdfa('');
els.pdfaGsGo.onclick = () => runPdfa('gs');

// runPdfa posts the current document to the PDF/A converter. Default is the pure-Go
// normalizer; engine 'gs' is the Ghostscript path (re-embeds fonts, converts
// colour) — offered as a fallback when the pure-Go path refuses and gs is installed.
async function runPdfa(engine) {
  const gs = engine === 'gs';
  els.pdfaStatus.textContent = gs ? 'Converting with Ghostscript…' : 'Converting…';
  els.pdfaGo.disabled = els.pdfaGsGo.disabled = true;
  try {
    const res = await apiFetch(gs ? '/api/pdfa?engine=gs' : '/api/pdfa', { method: 'POST', body: await bakedForm() });
    if (!res.ok) {
      let msg = await errText(res, 'Could not convert to PDF/A.');
      if (!gs && gsAvailable) {
        els.pdfaGsGo.hidden = false; // the heavier converter can handle what pure-Go refused
        msg += ' — Ghostscript can convert it (re-embeds fonts, converts colour).';
      }
      els.pdfaStatus.textContent = msg;
      return;
    }
    els.pdfaModal.hidden = true;
    openSaveAs(await res.blob(), exportBase() + '-pdfa.pdf', 'Save archival PDF (PDF/A-2b)');
  } catch (e) {
    els.pdfaStatus.textContent = 'Could not convert to PDF/A — ' + e.message;
  } finally {
    els.pdfaGo.disabled = els.pdfaGsGo.disabled = false;
  }
}

// renderCompare word-diffs the open document (a) against the chosen file (b) and
// paints the result inline: removed runs struck through in red, additions in
// green, unchanged text muted. Empty text on either side means a scan with no
// text layer — say so rather than show a bogus all-changed diff. DOM is built
// with textContent (never innerHTML) so PDF text can't inject markup.
function renderCompare(a, b, name) {
  const body = els.compareBody;
  body.textContent = '';
  const note = (msg) => {
    const p = document.createElement('p');
    p.className = 'scan-where';
    p.textContent = msg;
    body.appendChild(p);
  };
  if (!a.trim() || !b.trim()) {
    note(!a.trim()
      ? 'The open document has no extractable text (a scan?). Run OCR on it first, then compare.'
      : `“${name}” has no extractable text (a scan?). Run OCR on it first, then compare.`);
    return;
  }
  const parts = diffWords(a, b);
  if (!parts.some((p) => p.added || p.removed)) {
    note('No text differences — the two documents’ text is identical.');
    return;
  }
  const dels = parts.filter((p) => p.removed).length;
  const adds = parts.filter((p) => p.added).length;
  note(`Open document → “${name}”: ${dels} removed and ${adds} added section(s). Removed text is struck through in red, additions in green.`);
  const pre = document.createElement('div');
  pre.className = 'difftext';
  for (const part of parts) {
    const span = document.createElement('span');
    span.textContent = part.value;
    span.className = part.added ? 'diffadd' : part.removed ? 'diffdel' : 'diffsame';
    pre.appendChild(span);
  }
  body.appendChild(pre);
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
    if (!res.ok) { toast(await errText(res, 'save failed')); return; }
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
els.scanMetaBtn.onclick = () => runSanitize('metadata', 'Flatten');
els.scanSafeBtn.onclick = () => runSanitize('safe', 'Flatten');

// --- remove password protection ----------------------------------------------
// Two entry points, one /api/decrypt: the Secure-tab button (an open document —
// owner-only restrictions, or a no-op on a plain doc; empty password) and the
// on-open prompt below (a PDF that won't render without its open password). The
// server replaces the working copy with the decrypted bytes; only the supplied
// password is tried — Nib never guesses one.
async function postDecrypt(password) {
  const res = await apiFetch('/api/decrypt', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: 'password=' + encodeURIComponent(password),
  });
  if (!res.ok) { toast('could not remove protection'); return null; }
  return res.json();
}

els.decryptBtn.onclick = async () => {
  if (!pdfDocument) return toast('Open a PDF first');
  if (!confirmSignatureLoss()) return;
  const out = await postDecrypt(''); // an open doc decrypts with the empty user password
  if (!out) return;
  if (out.reason === 'plain') return toast('This document isn’t password-protected');
  if (out.reason === 'password') return openDecryptPrompt(); // shouldn't occur for an open doc
  await setDocumentFromServer(out);
  toast('Password protection removed');
};

// openDecryptPrompt collects the open password when a PDF can't render without it.
function openDecryptPrompt() {
  els.decryptError.hidden = true;
  els.decryptPw.value = '';
  els.decryptModal.hidden = false;
  els.decryptPw.focus();
}
els.decryptCancel.onclick = () => { els.decryptModal.hidden = true; };
els.decryptGo.onclick = async () => {
  const out = await postDecrypt(els.decryptPw.value);
  if (!out) return;
  if (out.reason === 'password') {
    els.decryptError.textContent = 'Incorrect password — try again.';
    els.decryptError.hidden = false;
    els.decryptPw.select();
    return;
  }
  els.decryptModal.hidden = true;
  await setDocumentFromServer(out.reason === 'plain' ? docMeta : out);
  // Unlocking rewrites the PDF, which breaks any signature it carried. On this
  // on-open path the user never saw the signed state (the doc couldn't render
  // until now), so the usual confirmSignatureLoss prompt never fired — warn here
  // instead, now that the decrypted bytes reveal the (now-invalid) signature.
  if (out.reason !== 'plain' && out.signature?.state === 'invalid') {
    alert('Unlocked — but this document was signed, and unlocking a password-protected PDF rewrites it, which has invalidated that signature.');
  } else {
    toast('Document unlocked');
  }
};
els.decryptPw.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') { e.preventDefault(); els.decryptGo.onclick(); }
});

// --- add password protection -------------------------------------------------
// Save a separate AES-256 protected copy via /api/encrypt. Unlike decrypt this is
// an EXPORT, not an in-place mutation: the server returns the encrypted bytes and
// the open working copy is left unprotected and editable (encrypted bytes can't be
// re-rendered without the password). The password is typed twice — a typo would
// lock the copy permanently, and Nib can't recover one.
els.encryptBtn.onclick = () => {
  if (!pdfDocument) return toast('Open a PDF first');
  // The open doc is untouched — only the protected copy loses any signature — so this
  // is a tailored warning, not the in-place confirmSignatureLoss() the editing ops use.
  if (isSigned() && !confirm('This document is signed. The protected copy won’t carry that signature (encrypting rewrites the file); your open document is unchanged. Continue?')) return;
  els.encryptError.hidden = true;
  els.encryptPw.value = '';
  els.encryptPw2.value = '';
  els.encryptModal.hidden = false;
  els.encryptPw.focus();
};
els.encryptCancel.onclick = () => { els.encryptModal.hidden = true; };
els.encryptGo.onclick = async () => {
  const pw = els.encryptPw.value;
  if (!pw) {
    els.encryptError.textContent = 'Enter a password.';
    els.encryptError.hidden = false;
    els.encryptPw.focus();
    return;
  }
  if (pw !== els.encryptPw2.value) {
    els.encryptError.textContent = 'The passwords don’t match.';
    els.encryptError.hidden = false;
    els.encryptPw2.select();
    return;
  }
  // Send the current document with overlay edits baked in (the shared export path),
  // so the protected copy reflects what's on screen, not the server's committed bytes.
  const form = await bakedForm();
  form.append('password', pw);
  const res = await apiFetch('/api/encrypt', { method: 'POST', body: form });
  if (!res.ok) { toast('could not protect the document'); return; }
  els.encryptModal.hidden = true;
  openSaveAs(await res.blob(), exportBase() + '-protected.pdf', 'Save password-protected PDF');
};
els.encryptPw.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') { e.preventDefault(); els.encryptPw2.focus(); }
});
els.encryptPw2.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') { e.preventDefault(); els.encryptGo.onclick(); }
});

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

// --- embedded attachments: list / extract / add ------------------------------
// Lists the document's embedded files; each row extracts to a saved file, and
// "Attach a file…" embeds one (a doc mutation, reloaded like the page ops).
async function loadAttachments() {
  els.attachBody.innerHTML = '<p class="scan-where">Loading…</p>';
  try {
    const res = await apiFetch('/api/attachments');
    if (!res.ok) throw new Error('list');
    renderAttachments((await res.json()).attachments || []);
  } catch { els.attachBody.innerHTML = '<p class="scan-where">Could not list attachments</p>'; }
}
function renderAttachments(items) {
  const body = els.attachBody;
  body.innerHTML = '';
  if (!items.length) {
    const p = document.createElement('p');
    p.className = 'scan-empty';
    p.textContent = 'No embedded files.';
    body.appendChild(p);
    return;
  }
  for (const a of items) {
    const row = document.createElement('div');
    row.className = 'attachrow';
    const meta = document.createElement('div');
    const name = document.createElement('div');
    name.className = 'scan-detail';
    name.textContent = a.name;
    meta.appendChild(name);
    if (a.desc) {
      const desc = document.createElement('div');
      desc.className = 'scan-where';
      desc.textContent = a.desc;
      meta.appendChild(desc);
    }
    const btn = document.createElement('button');
    btn.textContent = 'Extract';
    btn.onclick = () => extractAttachment(a.name);
    row.append(meta, btn);
    body.appendChild(row);
  }
}
async function extractAttachment(name) {
  const form = new FormData();
  form.append('name', name);
  const res = await apiFetch('/api/attachments/extract', { method: 'POST', body: form });
  if (!res.ok) return toast('extract failed');
  openSaveAs(await res.blob(), name, 'Save attachment');
}
els.attachBtn.onclick = () => {
  if (!pdfDocument) return toast('Open a PDF first');
  els.attachmentsModal.hidden = false;
  loadAttachments();
};
els.attachClose.onclick = () => { els.attachmentsModal.hidden = true; };
els.attachAddBtn.onclick = () => els.attachInput.click();
els.attachInput.onchange = async () => {
  const file = els.attachInput.files[0];
  els.attachInput.value = '';
  if (!file) return;
  if (!confirmSignatureLoss()) return;
  const form = new FormData();
  form.append('file', file, file.name);
  form.append('name', file.name);
  const res = await apiFetch('/api/attachments/add', { method: 'POST', body: form });
  if (!res.ok) return toast('could not attach the file (a same-named attachment may already exist)');
  await setDocumentFromServer(await res.json());
  await loadAttachments();
  toast('File attached');
};

// --- thumbnails sidebar ------------------------------------------------------
async function buildThumbnails(gen = docGen) {
  els.thumbGrid.innerHTML = '';
  clearSelection(); // a rebuild means a new/edited doc — old page numbers no longer apply
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
    canvas.onclick = (e) => onThumbClick(e, n);

    const acts = document.createElement('div');
    acts.className = 'thumbacts';
    const rotL = document.createElement('button'); rotL.textContent = '↺'; rotL.title = 'Rotate left';
    rotL.onclick = (e) => { e.stopPropagation(); pageOp('rotate', { pages: String(n), deg: 270 }); };
    const rot = document.createElement('button'); rot.textContent = '↻'; rot.title = 'Rotate right';
    rot.onclick = (e) => { e.stopPropagation(); pageOp('rotate', { pages: String(n), deg: 90 }); };
    const del = document.createElement('button'); del.textContent = '×'; del.title = 'Delete page';
    del.onclick = (e) => { e.stopPropagation(); if (pdfDocument.numPages > 1) pageOp('delete', { pages: String(n) }); };
    acts.append(rotL, rot, del);

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

// --- thumbnail multi-select -------------------------------------------------
// A set of selected page numbers (1-based current positions) drives the bulk
// rotate/delete bar. The grid is rebuilt only on document load (buildThumbnails)
// and a page op renumbers pages, so the selection is cleared on every rebuild —
// it only ever names pages currently on screen. Bulk ops route through the same
// pageOp rail as the per-thumbnail buttons, so they inherit the signature guard,
// overlay bake, and reload for free.
let selectedPages = new Set(); // page numbers selected in the grid
let selAnchor = null;          // last clicked page, for shift-range selection

// onThumbClick handles a thumbnail click with its modifier keys: shift extends a
// range from the anchor, ctrl/cmd toggles one page, a plain click clears the
// selection and navigates (the pre-existing behaviour).
function onThumbClick(e, n) {
  if (e.shiftKey && selAnchor != null) {
    const lo = Math.min(selAnchor, n), hi = Math.max(selAnchor, n);
    selectedPages.clear();
    for (let p = lo; p <= hi; p++) selectedPages.add(p);
  } else if (e.ctrlKey || e.metaKey) {
    if (!selectedPages.delete(n)) selectedPages.add(n);
    selAnchor = n;
  } else {
    selectedPages.clear();
    selAnchor = n;
    viewer.currentPageNumber = n;
  }
  markSelectedThumbs();
}

function clearSelection() {
  selectedPages.clear();
  selAnchor = null;
  markSelectedThumbs();
}

function markSelectedThumbs() {
  els.thumbGrid.querySelectorAll('.thumbwrap').forEach((c) => {
    c.classList.toggle('selected', selectedPages.has(Number(c.dataset.page)));
  });
  els.thumbSelBar.hidden = selectedPages.size === 0;
  els.thumbSelCount.textContent = selectedPages.size + ' selected';
}

// selectedPagesParam joins the selection into the comma list pageOp/the server
// expect, in ascending page order.
function selectedPagesParam() {
  return [...selectedPages].sort((a, b) => a - b).join(',');
}

// --- thumbnail drag-to-reorder ----------------------------------------------
// Dragging a thumbnail live-moves it among its siblings; on drop, the new
// thumbnail order (each .thumbwrap's original page number) is sent as a reorder.
// Grabbing a thumbnail that's part of a multi-selection drags the whole selection
// as one contiguous block (relative order kept). A cancelled drag restores the
// original DOM order so a thumbnail's click target never diverges from its position.
let dragBlock = null;    // the .thumbwrap node(s) being dragged — one, or a selected block
let dragOrig = null;     // snapshot of the grid's child order, for cancel revert
let dragDropped = false; // a real drop committed this drag

function onThumbDragStart(e) {
  const wrap = e.target.closest('.thumbwrap');
  if (!wrap) return;
  // Grabbing a selected thumbnail drags the whole selection; an unselected one
  // drags just itself. Read the block from the DOM .selected class (kept in sync
  // with selectedPages by markSelectedThumbs), in document order.
  const sel = [...els.thumbGrid.querySelectorAll('.thumbwrap.selected')];
  dragBlock = wrap.classList.contains('selected') && sel.length > 1 ? sel : [wrap];
  dragOrig = [...els.thumbGrid.children];
  dragDropped = false;
  dragBlock.forEach((w) => w.classList.add('dragging'));
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/plain', wrap.dataset.page); // payload required to drag in Firefox
}

function onThumbDragOver(e) {
  if (!dragBlock) return;
  e.preventDefault(); // allow drop
  e.dataTransfer.dropEffect = 'move';
  const after = thumbBelow(e.clientY); // never a block node — all are .dragging, which thumbBelow skips
  for (const w of dragBlock) {
    if (after) els.thumbGrid.insertBefore(w, after);
    else els.thumbGrid.appendChild(w);
  }
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
  if (!dragBlock) return;
  e.preventDefault();
  dragDropped = true;
  const order = [...els.thumbGrid.querySelectorAll('.thumbwrap')].map((w) => w.dataset.page);
  if (order.join(',') !== dragOrig.map((w) => w.dataset.page).join(',')) {
    pageOp('reorder', { pages: order.join(',') });
  }
}

function onThumbDragEnd() {
  if (dragBlock) dragBlock.forEach((w) => w.classList.remove('dragging'));
  if (!dragDropped && dragOrig) dragOrig.forEach((w) => els.thumbGrid.appendChild(w)); // cancelled — restore order
  dragBlock = null;
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
  const form = await bakedForm();
  form.append('op', op);
  if (extra.pages) form.append('pages', extra.pages);
  if (extra.deg != null) form.append('deg', String(extra.deg));
  if (extra.file) form.append('append', extra.file, 'append.pdf');
  if (extra.page != null) form.append('page', String(extra.page));
  if (extra.cols != null) form.append('cols', String(extra.cols));
  if (extra.rows != null) form.append('rows', String(extra.rows));
  if (extra.resize) form.append('resize', '1');
  if (extra.rects) form.append('rects', extra.rects);
  if (extra.rect) form.append('rect', extra.rect);
  if (extra.position) form.append('position', extra.position);
  if (extra.prefix != null) form.append('prefix', extra.prefix);
  if (extra.start != null) form.append('start', String(extra.start));
  if (extra.pad != null) form.append('pad', String(extra.pad));
  if (extra.total) form.append('total', '1');
  if (extra.ranges) form.append('ranges', extra.ranges);
  if (extra.n != null) form.append('n', String(extra.n));
  if (extra.border != null) form.append('border', extra.border ? '1' : '0');
  const res = await apiFetch('/api/pages', { method: 'POST', body: form });
  if (!res.ok) { toast('page operation failed'); return false; }
  await setDocumentFromServer(await res.json());
  return true;
}

els.appendBtn.onclick = () => els.appendInput.click();
els.appendInput.onchange = () => {
  if (els.appendInput.files[0]) pageOp('append', { file: els.appendInput.files[0] });
  els.appendInput.value = '';
};

// --- split an imposed page into a grid of separate pages ---------------------
// The page is rendered once into an offscreen canvas; the modal shows it with
// the proposed cut-lines drawn over it and redraws as the grid changes. Splitting
// is a server-side re-crop (op:'split' on /api/pages); see pdfops.SplitPage.
let splitSrc = null; // offscreen render of the page being split

async function openSplit() {
  if (!pdfDocument) return;
  const page = await pdfDocument.getPage(viewer.currentPageNumber);
  const base = page.getViewport({ scale: 1 });
  const vp = page.getViewport({ scale: Math.min(2, 900 / base.width) });
  const cv = document.createElement('canvas');
  cv.width = Math.ceil(vp.width); cv.height = Math.ceil(vp.height);
  const c = cv.getContext('2d');
  c.fillStyle = '#fff'; c.fillRect(0, 0, cv.width, cv.height);
  await page.render({ canvasContext: c, viewport: vp }).promise;
  splitSrc = cv;
  drawSplitPreview();
  els.splitModal.hidden = false;
}

function splitGrid() {
  const cols = Math.max(1, Math.min(8, parseInt(els.splitCols.value, 10) || 1));
  const rows = Math.max(1, Math.min(8, parseInt(els.splitRows.value, 10) || 1));
  return { cols, rows };
}

function drawSplitPreview() {
  if (!splitSrc) return;
  const pv = els.splitPreview;
  pv.width = splitSrc.width; pv.height = splitSrc.height;
  const c = pv.getContext('2d');
  c.drawImage(splitSrc, 0, 0);
  const { cols, rows } = splitGrid();
  const dash = Math.max(4, pv.width / 80);
  c.strokeStyle = '#1e66f5'; // catppuccin blue, vivid on the white page
  c.lineWidth = Math.max(2, pv.width / 300);
  c.setLineDash([dash, dash]);
  for (let i = 1; i < cols; i++) {
    const x = (pv.width * i) / cols;
    c.beginPath(); c.moveTo(x, 0); c.lineTo(x, pv.height); c.stroke();
  }
  for (let j = 1; j < rows; j++) {
    const y = (pv.height * j) / rows;
    c.beginPath(); c.moveTo(0, y); c.lineTo(pv.width, y); c.stroke();
  }
}

async function splitGo() {
  const { cols, rows } = splitGrid();
  if (cols * rows < 2) { toast('choose at least two pieces (more than one column or row)'); return; }
  const page = viewer.currentPageNumber;
  els.splitModal.hidden = true;
  await pageOp('split', { page, cols, rows, resize: els.splitResize.checked });
}

// Rotate-all: omitting `pages` rotates every page server-side. CCW uses 270
// (≡ −90 mod 360) so the stored /Rotate stays a non-negative multiple of 90.
els.rotateLeftBtn.onclick = () => pageOp('rotate', { deg: 270 });
els.rotateRightBtn.onclick = () => pageOp('rotate', { deg: 90 });
els.normalizeBtn.onclick = async () => {
  if (await pageOp('normalize')) toast('Pages resized to the document’s most common size');
};

// Bulk actions on the thumbnail multi-selection. Delete is blocked when it would
// empty the document (mirrors the per-thumbnail "numPages > 1" guard, generalized).
els.selRotateLeftBtn.onclick = () => { if (selectedPages.size) pageOp('rotate', { pages: selectedPagesParam(), deg: 270 }); };
els.selRotateRightBtn.onclick = () => { if (selectedPages.size) pageOp('rotate', { pages: selectedPagesParam(), deg: 90 }); };
els.selDeleteBtn.onclick = () => {
  if (!selectedPages.size) return;
  if (pdfDocument.numPages <= selectedPages.size) { toast("can't delete every page"); return; }
  pageOp('delete', { pages: selectedPagesParam() });
};
els.selClearBtn.onclick = () => clearSelection();
// Move the selection to the front or back as a contiguous block, keeping the
// pages' relative order. Routes through the same reorder rail as the single-page
// drag (pageOp('reorder') → Collect), so it bakes overlays, warns on signature
// loss, and is undoable for free. A move that wouldn't change the order is a no-op.
function moveSelected(toFront) {
  if (!selectedPages.size) return;
  const selected = [...selectedPages].sort((a, b) => a - b);
  const rest = [];
  for (let p = 1; p <= pdfDocument.numPages; p++) if (!selectedPages.has(p)) rest.push(p);
  const order = toFront ? [...selected, ...rest] : [...rest, ...selected];
  const identity = Array.from({ length: pdfDocument.numPages }, (_, i) => i + 1);
  if (order.join(',') === identity.join(',')) return; // already at that end
  pageOp('reorder', { pages: order.join(',') });
}
els.selMoveFrontBtn.onclick = () => moveSelected(true);
els.selMoveBackBtn.onclick = () => moveSelected(false);

// Insert a blank page after the page on screen — a replace-in-place mutation, so
// it routes through pageOp like rotate/delete (the blank matches the neighbour).
els.insertBlankBtn.onclick = () => pageOp('insertblank', { page: viewer.currentPageNumber });

// Duplicate the page on screen — the copy lands right after it (replace-in-place
// mutation, same pageOp rail as insert-blank).
els.duplicatePageBtn.onclick = async () => {
  if (await pageOp('duplicate', { page: viewer.currentPageNumber })) toast('Page duplicated');
};

// Insert another PDF BEFORE the page on screen (before page 1 prepends; use
// "+ Append PDF" for the end). Reuses pageOp's `file` field (the `append` part).
els.insertPdfBtn.onclick = () => els.insertPdfInput.click();
els.insertPdfInput.onchange = async () => {
  const file = els.insertPdfInput.files[0];
  els.insertPdfInput.value = '';
  if (file && await pageOp('insertpdf', { file, page: viewer.currentPageNumber })) toast('PDF inserted');
};

// --- extract a page range into a new PDF -------------------------------------
// Unlike the page mutations above, extract derives a NEW file and hands it to
// Save-as (openSaveAs) without touching the open document.
// parsePageRanges turns "1-3, 5, 8" into a sorted, deduped, in-range page list;
// returns null on any malformed or out-of-range token.
function parsePageRanges(spec, max) {
  const out = new Set();
  for (const part of spec.split(',')) {
    const s = part.trim();
    if (!s) continue;
    const m = s.match(/^(\d+)(?:-(\d+))?$/);
    if (!m) return null;
    let a = Number(m[1]);
    let b = m[2] ? Number(m[2]) : a;
    if (a < 1 || b < 1 || a > max || b > max) return null;
    if (a > b) [a, b] = [b, a];
    for (let p = a; p <= b; p++) out.add(p);
  }
  return [...out].sort((x, y) => x - y);
}

function openExtract() {
  if (!pdfDocument) return;
  const n = pdfDocument.numPages;
  els.extractPages.value = '';
  els.extractHint.textContent = `This document has ${n} page${n === 1 ? '' : 's'}.`;
  els.extractModal.hidden = false;
  els.extractPages.focus();
}

async function extractGo() {
  const pages = parsePageRanges(els.extractPages.value, pdfDocument.numPages);
  if (!pages || pages.length === 0) {
    els.extractHint.textContent = 'Enter pages like "1-3, 5" within the document range.';
    return;
  }
  const form = await bakedForm();
  form.append('pages', pages.join(','));
  const res = await apiFetch('/api/extract', { method: 'POST', body: form });
  if (!res.ok) return toast('could not extract pages');
  els.extractModal.hidden = true;
  openSaveAs(await res.blob(), exportBase() + '-pages.pdf', 'Extract pages');
}

els.extractBtn.onclick = openExtract;
els.extractCancel.onclick = () => { els.extractModal.hidden = true; };
els.extractGo.onclick = extractGo;

// --- page numbers / Bates ----------------------------------------------------
// Stamp a running number onto every page, in place (op:'pagenum' on /api/pages →
// pdfops.StampPageNumbers). The number, prefix, zero-pad and start are formatted
// server-side; here we just gather them and show a live first/last preview.
// pnPrefixVal is the one place the prefix is read: % is stripped (pdfcpu treats
// it as a placeholder marker) so the preview and the stamped output can't disagree.
function pnPrefixVal() { return els.pnPrefix.value.replace(/%/g, ''); }
function pnFormat(num, start, n) {
  const pad = Math.min(12, Math.max(0, parseInt(els.pnPad.value, 10) || 0));
  return pnPrefixVal() + String(num).padStart(pad, '0') + (els.pnTotal.checked ? ' of ' + (start + n - 1) : '');
}
function pnPreview() {
  const start = Math.max(1, parseInt(els.pnStart.value, 10) || 1);
  const n = pdfDocument ? pdfDocument.numPages : 1;
  els.pnPreview.textContent = n === 1
    ? `1 page → “${pnFormat(start, start, n)}”`
    : `${n} pages → “${pnFormat(start, start, n)}” … “${pnFormat(start + n - 1, start, n)}”`;
}
function openPageNum() {
  if (!pdfDocument) return;
  pnPreview();
  els.pageNumModal.hidden = false;
}
async function pageNumGo() {
  const ok = await pageOp('pagenum', {
    position: els.pnPosition.value,
    prefix: pnPrefixVal(),
    start: Math.max(1, parseInt(els.pnStart.value, 10) || 1),
    pad: Math.min(12, Math.max(0, parseInt(els.pnPad.value, 10) || 0)),
    total: els.pnTotal.checked,
  });
  if (ok) { els.pageNumModal.hidden = true; toast('Page numbers added'); }
}
els.pageNumBtn.onclick = openPageNum;
els.pnCancel.onclick = () => { els.pageNumModal.hidden = true; };
els.pnGo.onclick = pageNumGo;
['pnPosition', 'pnStart', 'pnPad', 'pnPrefix', 'pnTotal'].forEach((id) => els[id].addEventListener('input', pnPreview));

// --- page labels -------------------------------------------------------------
// Author the /PageLabels number tree as a flat, page-ordered list of ranges
// (front matter i, ii, iii then body 1, 2, 3). Each range = a start page, a
// numbering style, a first value, and an optional prefix; pages before the first
// range carry no label. Routes through pageOp('pagelabels') → SetPageLabels.
const PL_STYLES = [
  ['decimal', '1, 2, 3'],
  ['roman-lower', 'i, ii, iii'],
  ['roman-upper', 'I, II, III'],
  ['alpha-lower', 'a, b, c'],
  ['alpha-upper', 'A, B, C'],
  ['none', 'prefix only'],
];
let plRanges = [];
function toRoman(n) {
  if (n < 1) return String(n);
  const m = [[1000, 'M'], [900, 'CM'], [500, 'D'], [400, 'CD'], [100, 'C'], [90, 'XC'], [50, 'L'], [40, 'XL'], [10, 'X'], [9, 'IX'], [5, 'V'], [4, 'IV'], [1, 'I']];
  let s = '';
  for (const [v, r] of m) while (n >= v) { s += r; n -= v; }
  return s;
}
function toAlpha(n) { // PDF /A,/a: 1→A … 26→Z, 27→AA, 28→AB … (repeated letter)
  if (n < 1) return String(n);
  return String.fromCharCode(65 + (n - 1) % 26).repeat(Math.floor((n - 1) / 26) + 1);
}
// plLabel renders the label `idx` pages into a range (idx 0 = its start page).
function plLabel(style, first, idx, prefix) {
  const v = first + idx;
  const body = style === 'roman-lower' ? toRoman(v).toLowerCase()
    : style === 'roman-upper' ? toRoman(v)
    : style === 'alpha-lower' ? toAlpha(v).toLowerCase()
    : style === 'alpha-upper' ? toAlpha(v)
    : style === 'none' ? '' : String(v);
  return (prefix || '') + body;
}
function updatePlPreview() {
  const n = pdfDocument ? pdfDocument.numPages : 1;
  const rs = [...plRanges].sort((a, b) => a.start - b.start);
  els.plPreview.textContent = rs.map((r, i) => {
    const start = Math.max(1, Math.min(r.start, n));
    const last = Math.max(start, i + 1 < rs.length ? Math.min(rs[i + 1].start - 1, n) : n);
    const span = start === last ? `p${start}` : `p${start}–${last}`;
    if (r.style === 'none') return `${span}: “${r.prefix || '(none)'}”`;
    const labels = [plLabel(r.style, r.first, 0, r.prefix)];
    if (last > start) labels.push(plLabel(r.style, r.first, 1, r.prefix));
    if (last > start + 1) labels.push('…', plLabel(r.style, r.first, last - start, r.prefix));
    return `${span}: ${labels.join(', ')}`;
  }).join('   ·   ');
}
function renderPageLabels() {
  plRanges.sort((a, b) => a.start - b.start);
  const n = pdfDocument ? pdfDocument.numPages : 1;
  els.plList.innerHTML = '';
  plRanges.forEach((r, i) => {
    const row = document.createElement('div');
    row.className = 'outlinerow';
    const page = document.createElement('input');
    page.type = 'number'; page.className = 'outline-page'; page.min = '1'; page.max = String(n);
    page.value = String(r.start); page.title = 'From page';
    page.onchange = () => { r.start = Math.max(1, Math.min(n, parseInt(page.value, 10) || 1)); renderPageLabels(); };
    const style = document.createElement('select');
    PL_STYLES.forEach(([val, lbl]) => {
      const o = document.createElement('option'); o.value = val; o.textContent = lbl;
      if (val === r.style) o.selected = true;
      style.appendChild(o);
    });
    style.onchange = () => { r.style = style.value; renderPageLabels(); };
    const first = document.createElement('input');
    first.type = 'number'; first.className = 'outline-page'; first.min = '1';
    first.value = String(r.first); first.title = 'Start numbering at';
    first.onchange = () => { r.first = Math.max(1, parseInt(first.value, 10) || 1); renderPageLabels(); };
    const prefix = document.createElement('input');
    prefix.type = 'text'; prefix.className = 'outline-title'; prefix.placeholder = 'Prefix (optional)';
    prefix.value = r.prefix; prefix.oninput = () => { r.prefix = prefix.value; updatePlPreview(); };
    const del = document.createElement('button');
    del.className = 'keydel'; del.textContent = '✕'; del.title = 'Remove range'; del.disabled = plRanges.length <= 1;
    del.onclick = () => { plRanges.splice(i, 1); renderPageLabels(); };
    row.append(page, style, first, prefix, del);
    els.plList.appendChild(row);
  });
  updatePlPreview();
}
function openPageLabels() {
  if (!pdfDocument) return;
  plRanges = [{ start: 1, style: 'decimal', first: 1, prefix: '' }];
  renderPageLabels();
  els.pageLabelsModal.hidden = false;
}
async function pageLabelsGo() {
  const ranges = plRanges
    .map((r) => ({ start: Math.max(1, parseInt(r.start, 10) || 1), style: r.style, first: Math.max(1, parseInt(r.first, 10) || 1), prefix: r.prefix || '' }))
    .sort((a, b) => a.start - b.start);
  for (let i = 1; i < ranges.length; i++) {
    if (ranges[i].start <= ranges[i - 1].start) { toast('each range must start on a later page than the one before'); return; }
  }
  const ok = await pageOp('pagelabels', { ranges: JSON.stringify(ranges) });
  if (ok) { els.pageLabelsModal.hidden = true; toast('Page labels set'); }
}
els.pageLabelsBtn.onclick = openPageLabels;
els.plCancel.onclick = () => { els.pageLabelsModal.hidden = true; };
els.plGo.onclick = pageLabelsGo;
els.plAdd.onclick = () => {
  const n = pdfDocument ? pdfDocument.numPages : 1;
  const last = plRanges.reduce((m, r) => Math.max(m, r.start), 0);
  plRanges.push({ start: Math.min(n, last + 1), style: 'decimal', first: 1, prefix: '' });
  renderPageLabels();
};

// --- N-up: combine several pages onto each sheet -----------------------------
// Whole-document re-imposition in place (op:'nup' on /api/pages → pdfops.NUp).
function openNup() {
  if (!pdfDocument) return;
  els.nupModal.hidden = false;
}
async function nupGo() {
  const ok = await pageOp('nup', { n: parseInt(els.nupN.value, 10), border: els.nupBorder.checked });
  if (ok) { els.nupModal.hidden = true; toast('Pages combined onto sheets'); }
}
els.nupBtn.onclick = openNup;
els.nupCancel.onclick = () => { els.nupModal.hidden = true; };
els.nupGo.onclick = nupGo;

els.splitBtn.onclick = openSplit;
els.splitCancel.onclick = () => { els.splitModal.hidden = true; };
els.splitGo.onclick = splitGo;
els.splitCols.oninput = drawSplitPreview;
els.splitRows.oninput = drawSplitPreview;

// --- outline sidebar ---------------------------------------------------------
async function buildOutline(gen = docGen) {
  els.outline.innerHTML = '';
  const edit = document.createElement('button');
  edit.className = 'outline-edit';
  edit.textContent = 'Edit outline…';
  edit.onclick = openOutlineEditor;
  els.outline.appendChild(edit);
  const outline = await pdfDocument.getOutline();
  if (gen !== docGen) return; // a newer document loaded — drop this stale outline
  if (!outline || !outline.length) {
    const empty = document.createElement('div');
    empty.className = 'thumb-label';
    empty.textContent = 'No outline';
    els.outline.appendChild(empty);
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

// --- outline editor ----------------------------------------------------------
// Author the bookmark tree: a flat, page-ordered, leveled list (indent to nest).
// Reads/writes through the server (pdfcpu), so each bookmark carries a real page
// and what's written matches what's read. Save replaces the whole outline.
let outlineItems = [];
function sortOutline() { outlineItems.sort((a, b) => a.page - b.page); }
function renderOutlineEditor() {
  sortOutline();
  const list = els.outlineEditList;
  list.innerHTML = '';
  if (!outlineItems.length) {
    const p = document.createElement('p');
    p.className = 'scan-empty';
    p.textContent = 'No bookmarks yet — add one.';
    list.appendChild(p);
  }
  const n = pdfDocument ? pdfDocument.numPages : 1;
  let prev = -1;
  outlineItems.forEach((it, i) => {
    it.level = Math.max(0, Math.min(it.level, prev + 1)); // nesting may deepen one level at a time
    prev = it.level;
    const row = document.createElement('div');
    row.className = 'outlinerow';
    row.style.paddingLeft = it.level * 16 + 'px';
    const title = document.createElement('input');
    title.type = 'text'; title.className = 'outline-title'; title.value = it.title; title.placeholder = 'Bookmark title';
    title.oninput = () => { it.title = title.value; };
    const page = document.createElement('input');
    page.type = 'number'; page.className = 'outline-page'; page.min = '1'; page.max = String(n); page.value = String(it.page); page.title = 'Page';
    page.onchange = () => { it.page = Math.max(1, Math.min(n, parseInt(page.value, 10) || 1)); renderOutlineEditor(); };
    const indent = document.createElement('button');
    indent.textContent = '→'; indent.title = 'Nest under the bookmark above'; indent.disabled = i === 0;
    indent.onclick = () => { it.level += 1; renderOutlineEditor(); };
    const outdent = document.createElement('button');
    outdent.textContent = '←'; outdent.title = 'Un-nest'; outdent.disabled = it.level === 0;
    outdent.onclick = () => { it.level = Math.max(0, it.level - 1); renderOutlineEditor(); };
    const del = document.createElement('button');
    del.className = 'keydel'; del.textContent = '✕'; del.title = 'Delete';
    del.onclick = () => { outlineItems.splice(i, 1); renderOutlineEditor(); };
    row.append(title, page, indent, outdent, del);
    list.appendChild(row);
  });
}
async function openOutlineEditor() {
  if (!pdfDocument) return;
  try {
    const res = await apiFetch('/api/outline');
    if (!res.ok) throw new Error('outline');
    outlineItems = ((await res.json()).items || []).map((it) => ({ title: it.title, page: it.page, level: it.level }));
  } catch { outlineItems = []; }
  renderOutlineEditor();
  els.outlineModal.hidden = false;
}
els.outlineCancel.onclick = () => { els.outlineModal.hidden = true; };
els.outlineAddBtn.onclick = () => {
  outlineItems.push({ title: 'New bookmark', page: viewer.currentPageNumber || 1, level: 0 });
  renderOutlineEditor();
};
els.outlineSave.onclick = async () => {
  sortOutline();
  const titles = new Set();
  for (const it of outlineItems) {
    const t = it.title.trim();
    if (!t) return toast('Every bookmark needs a title');
    if (titles.has(t)) return toast(`Bookmark titles must be unique: “${t}”`);
    titles.add(t);
  }
  const form = await bakedForm();
  form.append('outline', JSON.stringify(outlineItems.map((it) => ({ title: it.title.trim(), page: it.page, level: it.level }))));
  const res = await apiFetch('/api/outline', { method: 'POST', body: form });
  if (!res.ok) return toast(await errText(res, 'could not save the outline'));
  await setDocumentFromServer(await res.json());
  els.outlineModal.hidden = true;
  toast('Outline saved');
};

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
  const gen = docGen;
  const img = new Image();
  img.onload = () => {
    if (gen !== docGen) return; // a reload superseded this placement — pv.div is now detached
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

  const remove = () => deleteField(f);
  del.onclick = (e) => { e.stopPropagation(); remove(); };
  el.addEventListener('keydown', (e) => { if (e.key === 'Delete' || e.key === 'Backspace') remove(); });
  enableStampGestures(f, el, handle);

  overlayFields.push(f);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f);
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
    } else if (f.aspect) {
      let nw = Math.max((x1 - x0) + dx, 12 / W);
      let nh = nw * W / (f.aspect * H); // keep image aspect (page-pixel terms)
      const k = Math.min(1, (1 - x0) / nw, (1 - y0) / nh); // clamp, preserve aspect
      f.frac = [x0, y0, x0 + nw * k, y0 + nh * k];
    } else {
      // No aspect lock (border boxes): each side resizes freely.
      const nw = Math.max((x1 - x0) + dx, 12 / W), nh = Math.max((y1 - y0) + dy, 12 / H);
      f.frac = [x0, y0, Math.min(x0 + nw, 1), Math.min(y0 + nh, 1)];
    }
    layoutField(f, pv);
  });
  el.addEventListener('pointerup', (e) => {
    if (mode && start && f.frac.join() !== start.join()) recordMove(f, start.slice(), f.frac.slice());
    mode = null;
    try { el.releasePointerCapture(e.pointerId); } catch { /* already released */ }
  });
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
    card.onclick = (e) => {
      if (e.target.closest('.del')) return;
      const src = '/api/images/' + m.id;
      if (fillTarget) resolveFillTarget(src); else placeStamp(src);
    };
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
  else toast(await errText(res, 'could not add image'));
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

// textStampURL renders a typed value (name/title/company flag) to a transparent
// PNG in ink, placed through the same stamp path as a date or signature image.
function textStampURL(text) {
  const cv = document.createElement('canvas');
  const ctx = cv.getContext('2d');
  ctx.font = '36px sans-serif';
  cv.width = Math.ceil(ctx.measureText(text).width) + 24;
  cv.height = 56;
  ctx.font = '36px sans-serif'; // resizing the canvas resets the context
  ctx.fillStyle = INK; ctx.textBaseline = 'middle';
  ctx.fillText(text, 12, 30);
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

// browseDir drives a folder picker. t names the four elements it writes (dir
// input, here label, up button, list ul) so the same browser backs both the
// Save-as dialog and the bookmark-split dialog.
const saveAsDirEls = () => ({ dir: els.saveAsDir, here: els.saveAsHere, up: els.saveAsUp, list: els.saveAsList });
async function browseDir(path, t = saveAsDirEls()) {
  const res = await apiFetch('/api/listdir' + (path ? '?path=' + encodeURIComponent(path) : ''));
  if (!res.ok) return;
  const info = await res.json();
  t.dir.value = info.path;
  t.here.textContent = info.path;
  t.up.disabled = !info.parent;
  t.up.dataset.parent = info.parent || '';
  t.list.innerHTML = '';
  for (const d of info.dirs) {
    const li = document.createElement('li');
    li.textContent = d;
    li.onclick = () => browseDir(joinPath(info.path, d), t);
    t.list.appendChild(li);
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

// b64ToBlob decodes a base64 string (e.g. an upgraded .ots from the server) into
// a Blob the save dialog can write.
function b64ToBlob(b64, mime) {
  const bin = atob(b64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return new Blob([bytes], { type: mime });
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
  if (!res.ok) { toast(await errText(res, 'could not save')); return; }
  const meta = await res.json();
  els.saveAsModal.hidden = true;
  saveAsBlob = null;
  toast('Saved to ' + meta.path);
};

// --- split by bookmarks to a folder ------------------------------------------
// Save one PDF per top-level bookmark into a chosen folder (PDFExplode-style: a
// scored orchestration in, one file per part out). Server-side via
// /api/split-bookmarks; the open document is left untouched.
const bookmarkDirEls = () => ({ dir: els.bsDir, here: els.bsHere, up: els.bsUp, list: els.bsList });
let bsOutline = []; // the current doc's top-level outline, for the preview

function updateBsPreview() {
  const titles = bsOutline.map((o) => o.title || '(untitled)');
  const sample = titles.slice(0, 6).join(', ') + (titles.length > 6 ? ', …' : '');
  const px = els.bsPrefix.value;
  els.bsPreview.textContent = `${titles.length} part${titles.length === 1 ? '' : 's'}: ${sample}  —  saved as “${px}<bookmark>.pdf”`;
}

async function openBookmarkSplit() {
  if (!pdfDocument) return;
  const outline = await pdfDocument.getOutline();
  if (!outline || !outline.length) { toast('This PDF has no bookmarks to split by'); return; }
  bsOutline = outline;
  els.bsPrefix.value = '';
  updateBsPreview();
  els.bookmarkSplitModal.hidden = false;
  browseDir('', bookmarkDirEls()); // server resolves '' to ~/nib
}

async function bookmarkSplitGo() {
  const dir = els.bsDir.value.trim().replace(/\/+$/, '');
  if (!dir) return toast('Choose a folder');
  const count = bsOutline.length;
  if (!confirm(`Write ${count} file${count === 1 ? '' : 's'} to ${dir}? Files with the same name will be replaced.`)) return;
  const form = await bakedForm();
  form.append('dir', dir);
  form.append('prefix', els.bsPrefix.value);
  const res = await apiFetch('/api/split-bookmarks', { method: 'POST', body: form });
  if (!res.ok) { toast(await errText(res, 'could not split')); return; }
  const meta = await res.json();
  els.bookmarkSplitModal.hidden = true;
  toast(`Wrote ${meta.count} file${meta.count === 1 ? '' : 's'} to ${meta.dir}`);
}

els.exportBookmarkSplitBtn.onclick = openBookmarkSplit;
els.bsCancel.onclick = () => { els.bookmarkSplitModal.hidden = true; };
els.bsGo.onclick = bookmarkSplitGo;
els.bsPrefix.oninput = updateBsPreview;
els.bsUp.onclick = () => { const p = els.bsUp.dataset.parent; if (p) browseDir(p, bookmarkDirEls()); };
els.bsDir.onchange = () => browseDir(els.bsDir.value.trim(), bookmarkDirEls());

// --- split into files by page range / every N pages --------------------------
// Divide the page SEQUENCE into several PDFs in a folder (distinct from the
// imposed-page geometry split). Server-side via /api/split-pages; the open
// document is left untouched. The preview mirrors the server's span logic
// best-effort — the toast reports the real count written.
const pageSplitDirEls = () => ({ dir: els.psDir, here: els.psHere, up: els.psUp, list: els.psList });
const psMode = () => document.querySelector('input[name="psMode"]:checked').value;
const spanLabel = (from, thru) => (from === thru ? String(from) : `${from}-${thru}`);

// psSpans computes the preview/confirm span labels client-side. Range tokens that
// are malformed or out of bounds are skipped here; the server is the real guard.
function psSpans() {
  const n = pdfDocument ? pdfDocument.numPages : 0;
  const spans = [];
  if (psMode() === 'every') {
    const k = Math.max(1, parseInt(els.psEvery.value, 10) || 1);
    for (let from = 1; from <= n; from += k) spans.push(spanLabel(from, Math.min(from + k - 1, n)));
  } else {
    for (const tok of els.psRanges.value.split(/[\s,]+/).filter(Boolean)) {
      const m = tok.match(/^(\d+)(?:-(\d+))?$/);
      if (!m) continue;
      const from = +m[1], thru = m[2] ? +m[2] : from;
      if (from >= 1 && thru >= from && thru <= n) spans.push(spanLabel(from, thru));
    }
  }
  return spans;
}
function updatePsPreview() {
  els.psRanges.disabled = psMode() !== 'ranges';
  els.psEvery.disabled = psMode() !== 'every';
  const spans = psSpans();
  if (!spans.length) { els.psPreview.textContent = 'Enter page ranges like 1-3, 4-8.'; return; }
  const sample = spans.slice(0, 6).join(', ') + (spans.length > 6 ? ', …' : '');
  els.psPreview.textContent = `${spans.length} file${spans.length === 1 ? '' : 's'}: ${sample}  —  saved as “${els.psPrefix.value}<range>.pdf”`;
}
function openPageSplit() {
  if (!pdfDocument) return;
  els.psPrefix.value = '';
  updatePsPreview();
  els.pageSplitModal.hidden = false;
  browseDir('', pageSplitDirEls());
}
async function pageSplitGo() {
  const dir = els.psDir.value.trim().replace(/\/+$/, '');
  if (!dir) return toast('Choose a folder');
  const count = psSpans().length;
  if (!count) return toast('Enter page ranges like 1-3, 4-8.');
  if (!confirm(`Write ${count} file${count === 1 ? '' : 's'} to ${dir}? Files with the same name will be replaced.`)) return;
  const form = await bakedForm();
  form.append('dir', dir);
  form.append('mode', psMode());
  form.append('prefix', els.psPrefix.value);
  if (psMode() === 'every') form.append('every', String(Math.max(1, parseInt(els.psEvery.value, 10) || 1)));
  else form.append('ranges', els.psRanges.value);
  const res = await apiFetch('/api/split-pages', { method: 'POST', body: form });
  if (!res.ok) { toast(await errText(res, 'could not split')); return; }
  const meta = await res.json();
  els.pageSplitModal.hidden = true;
  toast(`Wrote ${meta.count} file${meta.count === 1 ? '' : 's'} to ${meta.dir}`);
}
els.exportPageSplitBtn.onclick = openPageSplit;
els.psCancel.onclick = () => { els.pageSplitModal.hidden = true; };
els.psGo.onclick = pageSplitGo;
['psEvery', 'psRanges', 'psPrefix'].forEach((id) => els[id].addEventListener('input', updatePsPreview));
document.querySelectorAll('input[name="psMode"]').forEach((r) => r.addEventListener('change', updatePsPreview));
els.psUp.onclick = () => { const p = els.psUp.dataset.parent; if (p) browseDir(p, pageSplitDirEls()); };
els.psDir.onchange = () => browseDir(els.psDir.value.trim(), pageSplitDirEls());

// renderPageBlob rasterises one page of an already-parsed doc to a PNG at the
// given scale, runs the optional paint hook over the canvas after rendering (used
// to burn redaction boxes in), and returns the blob plus the page's true point
// size. The shared atom behind renderFilledPages and flattenPages.
async function renderPageBlob(doc, n, scale, paint, mime, quality) {
  const page = await doc.getPage(n);
  const base = page.getViewport({ scale: 1 }); // points: the page's true physical size
  const vp = page.getViewport({ scale });
  const cv = document.createElement('canvas');
  cv.width = vp.width; cv.height = vp.height;
  const ctx = cv.getContext('2d');
  if (mime === 'image/jpeg') { ctx.fillStyle = '#fff'; ctx.fillRect(0, 0, cv.width, cv.height); } // JPEG has no alpha
  await page.render({ canvasContext: ctx, viewport: vp, annotationMode: pdfjsLib.AnnotationMode.ENABLE }).promise;
  if (paint) paint(ctx, cv, n);
  const blob = await new Promise((r) => cv.toBlob(r, mime || 'image/png', quality));
  return { blob, w: base.width, h: base.height };
}

// renderPageCanvas rasterises one page to a fresh canvas and returns it with the
// page's true point size. A sibling of renderPageBlob kept separate so the
// redaction/flatten bake path (which needs a blob) is untouched; visual compare
// needs the live canvas to read pixels back via getImageData.
async function renderPageCanvas(doc, n, scale) {
  const page = await doc.getPage(n);
  const base = page.getViewport({ scale: 1 }); // points: the page's true physical size
  const vp = page.getViewport({ scale });
  const cv = document.createElement('canvas');
  cv.width = vp.width; cv.height = vp.height;
  const ctx = cv.getContext('2d');
  await page.render({ canvasContext: ctx, viewport: vp, annotationMode: pdfjsLib.AnnotationMode.ENABLE }).promise;
  return { canvas: cv, w: base.width, h: base.height };
}

// --- OCR: make a scanned PDF searchable --------------------------------------
// Rasterize each page, recognize its text entirely in the browser (tesseract.js,
// loaded from the vendored engine — nothing leaves the machine), map each word's
// pixel box to PDF points, and bake an invisible searchable text layer on the
// server. The scan still looks identical; its text becomes selectable + findable.
let tesseractReady = false;
function loadTesseract() {
  if (tesseractReady) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const s = document.createElement('script');
    s.src = './vendor/tesseract/tesseract.min.js';
    s.onload = () => { tesseractReady = true; resolve(); };
    s.onerror = () => reject(new Error('could not load the OCR engine'));
    document.head.appendChild(s);
  });
}


async function runOCR() {
  if (!pdfDocument || !confirmSignatureLoss()) return;
  const btn = els.ocrBtn, label = btn.textContent;
  btn.disabled = true;
  let worker;
  try {
    btn.textContent = 'Loading OCR…';
    await loadTesseract();
    // Absolute URLs (resolved against the page): tesseract.js runs its worker from
    // a blob: URL, and relative paths don't resolve against our origin from that
    // context — they must be full URLs or the worker's fetches fail ("Failed to
    // fetch") on the core wasm and the traineddata.
    const base = new URL('./vendor/tesseract/', location.href).href;
    // Single language for best accuracy; the picker defaults to English so the
    // common case is unchanged. Each <lang>.traineddata.gz is vendored alongside.
    const lang = (els.ocrLang && els.ocrLang.value) || 'eng';
    // Render DPI is a per-run choice: Fast (200, the default) or Best (300, the
    // tesseract-recommended optimum — slower but more accurate, esp. small text).
    const dpi = Number(els.ocrQuality && els.ocrQuality.value) || 200;
    const ocrScale = dpi / 72; // canvas px per PDF point at the chosen DPI
    worker = await window.Tesseract.createWorker(lang, 1, {
      workerPath: base + 'worker.min.js',
      corePath: base + 'tesseract-core-simd.wasm.js',
      langPath: base,
      gzip: true, // <lang>.traineddata.gz
    });
    // Tell tesseract the true DPI instead of letting it estimate from the bitmap
    // (a wrong guess skews its layout/word-spacing heuristics); matches ocrScale.
    await worker.setParameters({ user_defined_dpi: String(dpi) });
    const words = [];
    const n = pdfDocument.numPages;
    for (let p = 1; p <= n; p++) {
      btn.textContent = `OCR ${p}/${n}…`;
      const { blob, h } = await renderPageBlob(pdfDocument, p, ocrScale, null, 'image/png');
      const { data } = await worker.recognize(blob);
      for (const word of data.words || []) {
        const t = (word.text || '').trim();
        if (!t) continue;
        // bbox is pixels (top-left origin) at ocrScale; map to PDF points
        // (bottom-left origin): x/scale, and flip Y about the page height h.
        const b = word.bbox;
        words.push({ page: p, text: t, rect: [b.x0 / ocrScale, h - b.y1 / ocrScale, b.x1 / ocrScale, h - b.y0 / ocrScale] });
      }
    }
    if (!words.length) { toast('No text found to add'); return; }
    btn.textContent = 'Saving…';
    const res = await apiFetch('/api/ocr', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ lang, words }) });
    if (!res.ok) { toast('Could not add the text layer'); return; }
    await setDocumentFromServer(await res.json());
    toast(`Added a searchable text layer (${words.length} words)`);
  } catch (e) {
    toast(e.message || 'OCR failed');
  } finally {
    if (worker) { try { await worker.terminate(); } catch { /* already gone */ } }
    btn.disabled = false; btn.textContent = label;
  }
}
if (els.ocrBtn) els.ocrBtn.onclick = runOCR;

// renderFilledPages rasterises the saved (form-filled, stamped) document so the
// raster reflects every edit. Used for flatten and image export. mime/quality
// default to PNG; "image/jpeg" + a quality drives the lossy "reduce size" path.
async function renderFilledPages(scale, onlyPage, mime, quality) {
  const bytes = await bakedBytes();
  const doc = await pdfjsLib.getDocument({ data: bytes }).promise;
  const pages = [];
  const from = onlyPage || 1;
  const to = onlyPage || doc.numPages;
  for (let n = from; n <= to; n++) pages.push(await renderPageBlob(doc, n, scale, null, mime, quality));
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

// compressBlob rasterises every page to JPEG at the given render scale (DPI) and
// quality and assembles a much smaller image-PDF — the lossy "reduce size" path.
// It flattens the document (text becomes images), so the caller warns and shows
// the before/after size before saving.
async function compressBlob(scale, quality) {
  const pages = await renderFilledPages(scale, 0, 'image/jpeg', quality);
  const form = new FormData();
  pages.forEach((p, i) => {
    form.append('image', p.blob, `page-${i + 1}.jpg`);
    form.append('pageW', String(p.w));
    form.append('pageH', String(p.h));
  });
  form.append('format', 'pdf');
  const res = await apiFetch('/api/assemble', { method: 'POST', body: form });
  if (!res.ok) { toast('compress failed'); return null; }
  return res.blob();
}

function fmtBytes(n) {
  if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MB';
  if (n >= 1024) return Math.round(n / 1024) + ' KB';
  return n + ' B';
}
function sizeSummary(before, after) {
  const pct = before > 0 ? Math.round((1 - after / before) * 100) : 0;
  const change = pct > 0 ? pct + '% smaller' : (pct < 0 ? Math.abs(pct) + '% larger' : 'no change');
  return fmtBytes(before) + ' → ' + fmtBytes(after) + ' (' + change + ')';
}

els.saveFlatBtn.onclick = async () => {
  if (!pdfDocument) return toast('Open a PDF first');
  const blob = await assembleBlob('pdf');
  if (blob) openSaveAs(blob, exportBase() + '-flattened.pdf', 'Save flattened PDF');
};

// Reduce file size. Renders the result, shows before→after size, and only then
// reveals Save — so the user backs out if compress grew (or flattened) the file.
let reduceBlob = null;
function resetReduce() {
  reduceBlob = null;
  els.reduceResult.hidden = true; els.reduceResult.textContent = '';
  els.reduceSave.hidden = true; els.reduceGo.hidden = false;
  els.reduceModal.querySelector('input[value="optimize"]').checked = true;
  els.reduceQuality.hidden = true;
}
els.reduceBtn.onclick = () => { if (!pdfDocument) return toast('Open a PDF first'); resetReduce(); els.reduceModal.hidden = false; };
els.reduceCancel.onclick = () => { els.reduceModal.hidden = true; };
all('input[name="reduceMode"]').forEach((r) => {
  r.onchange = () => { els.reduceQuality.hidden = r.value !== 'compress' || !r.checked; };
});
els.reduceGo.onclick = async () => {
  const mode = els.reduceModal.querySelector('input[name="reduceMode"]:checked').value;
  els.reduceGo.disabled = true; els.reduceGo.textContent = 'Working…';
  let blob, before, after;
  try {
    if (mode === 'optimize') {
      const res = await apiFetch('/api/optimize', { method: 'POST' });
      if (!res.ok) return toast('Could not optimize');
      blob = await res.blob();
      before = Number(res.headers.get('X-Original-Size')) || 0;
      after = Number(res.headers.get('X-New-Size')) || blob.size;
    } else {
      const presets = { low: [96 / 72, 0.55], med: [150 / 72, 0.7], high: [220 / 72, 0.82] };
      const [scale, q] = presets[els.reduceQ.value] || presets.med;
      before = (await bakedBytes()).length;
      blob = await compressBlob(scale, q);
      if (!blob) return;
      after = blob.size;
    }
  } finally { els.reduceGo.disabled = false; els.reduceGo.textContent = 'Reduce'; }
  reduceBlob = blob;
  els.reduceResult.textContent = sizeSummary(before, after);
  els.reduceResult.hidden = false;
  els.reduceGo.hidden = true;
  els.reduceSave.hidden = false;
};
els.reduceSave.onclick = () => {
  if (!reduceBlob) return;
  els.reduceModal.hidden = true;
  openSaveAs(reduceBlob, exportBase() + '-smaller.pdf', 'Save reduced PDF');
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

// Save as fillable PDF: turn detected/placed text + checkbox fields into real
// interactive AcroForm widgets (a blank form to distribute), not flattened text.
// "Save as fillable form…" opens a naming step: one row per authorable field with
// an editable name (default field_N), then authors real AcroForm widgets.
// pageTextItems returns a page's text-layer runs as {str,x,y,w,h} in PDF points,
// bottom-left (y = the baseline) — the same space rectPoints produces, so field
// rects and text positions compare directly. Empty on an image-only page (no
// text layer). Mirrors the text gather in the Detect / Edit-text handlers.
async function pageTextItems(n) {
  try {
    const tc = await (await pdfDocument.getPage(n)).getTextContent();
    return tc.items.filter((it) => it.str && it.str.trim()).map((it) => ({
      str: it.str, x: it.transform[4], y: it.transform[5],
      w: it.width, h: it.height || Math.hypot(it.transform[2], it.transform[3]),
    }));
  } catch { return []; } // image-only PDF: no text layer
}

// suggestFieldName derives a default AcroForm field name from the form's own
// text: the label just left of the field on the same row (reassembling pdf.js
// text fragments right-to-left until a wide gap), or else a single run directly
// above it. CONSERVATIVE BY DESIGN — returns '' (caller falls back to field_N)
// unless a label is clearly adjacent, because clearing a wrong guess costs the
// user more than accepting a clean field_N. rect is [x0,yBot,x1,yTop] and items
// are {str,x,y,w,h}, both PDF points bottom-left.
function suggestFieldName(rect, items) {
  const [x0, yBot, x1, yTop] = rect;
  const fh = Math.max(yTop - yBot, 8); // field line height, floored
  const slug = (s) => s.normalize('NFD').replace(/[\u0300-\u036f]/g, '') // drop diacritics: Prénom → prenom
    .toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 40);
  const ok = (s) => (/[a-z]/.test(s) ? s : ''); // require a letter, else no suggestion
  // 1. Label to the LEFT, same row (band overlap), reassembled until a wide gap.
  const left = items
    .filter((it) => it.y + it.h * 0.8 >= yBot && it.y - it.h * 0.2 <= yTop && it.x + it.w <= x0 + 2)
    .sort((a, b) => b.x - a.x); // nearest the field first
  if (left.length && x0 - (left[0].x + left[0].w) <= fh * 2.5) { // nearest label must be adjacent
    const run = [left[0]];
    for (let i = 1; i < left.length; i++) {
      if (run[run.length - 1].x - (left[i].x + left[i].w) > fh * 1.2) break; // wide gap → stop
      run.push(left[i]);
    }
    const name = ok(slug(run.reverse().map((it) => it.str).join(' ')));
    if (name) return name;
  }
  // 2. Single run directly ABOVE, horizontally overlapping the field.
  const above = items
    .filter((it) => it.y > yTop - fh * 0.2 && it.y - yTop <= fh * 1.6 && it.x < x1 && it.x + it.w > x0)
    .sort((a, b) => a.y - b.y); // closest above first
  return above.length ? ok(slug(above[0].str)) : '';
}

let pendingAuthor = []; // candidate fields awaiting naming in fieldNameModal
els.saveFillableBtn.onclick = async () => {
  if (!pdfDocument) return;
  pendingAuthor = collectAuthorFields();
  if (!pendingAuthor.length) { toast('Run Detect or place text/checkbox fields first'); return; }
  // Pre-name each field from the form's own text (see suggestFieldName); each
  // page's text layer is fetched once. Conservative, so anything unclear — and
  // every field on an image-only scan — stays field_N.
  const textByPage = new Map();
  for (const f of pendingAuthor) {
    if (!textByPage.has(f.page)) textByPage.set(f.page, await pageTextItems(f.page));
    f.suggested = suggestFieldName(f.rect, textByPage.get(f.page));
  }
  els.fieldNameList.innerHTML = '';
  pendingAuthor.forEach((f, i) => {
    const row = document.createElement('label');
    row.className = 'fieldname-row';
    const tag = document.createElement('span');
    tag.className = 'fieldname-kind';
    tag.textContent = (f.kind === 'check' ? '☑' : f.kind === 'dropdown' ? '▾' : f.kind === 'radio' ? '◉' : '✎') + ' p' + f.page;
    const inp = document.createElement('input');
    inp.type = 'text';
    inp.value = f.suggested || ('field_' + (i + 1));
    inp.onfocus = () => f.el && f.el.classList.add('naming-hilite');
    inp.onblur = () => f.el && f.el.classList.remove('naming-hilite');
    row.append(tag, inp);
    els.fieldNameList.appendChild(row);
    f.input = inp;
  });
  els.fieldNameModal.hidden = false;
};

function clearNamingHilite() {
  for (const f of pendingAuthor) if (f.el) f.el.classList.remove('naming-hilite');
}

els.fieldNameCancel.onclick = () => { els.fieldNameModal.hidden = true; clearNamingHilite(); };

els.fieldNameGo.onclick = async () => {
  els.fieldNameModal.hidden = true;
  clearNamingHilite();
  // Normalize names client-side so authoring never hits pdfcpu's empty/duplicate
  // field-id errors: trim, blank → field_N, then de-dupe with a numeric suffix.
  const seen = new Set();
  const fields = [];
  pendingAuthor.forEach((f, i) => {
    if (f.kind === 'dropdown' && !(f.options && f.options.length)) {
      toast('Skipped a dropdown with no options'); return; // pdfcpu requires ≥1
    }
    if (f.kind === 'radio' && !(f.options && f.options.length >= 2)) {
      toast('Skipped a radio group with fewer than two choices'); return; // pdfcpu requires ≥2
    }
    const base = (f.input.value || '').trim() || ('field_' + (i + 1));
    let name = base;
    for (let n = 2; seen.has(name); n++) name = base + '_' + n;
    seen.add(name);
    const spec = { page: f.page, rect: f.rect, kind: f.kind, name };
    if (f.kind === 'dropdown' || f.kind === 'radio') spec.options = f.options;
    // A radio group inherits its layout from the drawn box's aspect: a wide box
    // lays the buttons out horizontally, a tall box stacks them vertically.
    if (f.kind === 'radio') {
      const w = f.rect[2] - f.rect[0], h = f.rect[3] - f.rect[1];
      spec.orientation = w >= h ? 'hor' : 'vert';
    }
    fields.push(spec);
  });
  if (!fields.length) return;
  // Post the base document WITHOUT baking the overlay fields — they become widgets.
  const saved = pdfDocument.annotationStorage.size > 0
    ? await pdfDocument.saveDocument()
    : await pdfDocument.getData();
  const form = new FormData();
  form.append('pdf', new Blob([saved], { type: 'application/pdf' }), 'doc.pdf');
  form.append('fields', JSON.stringify(fields));
  const res = await apiFetch('/api/form/author', { method: 'POST', body: form });
  if (!res.ok) { toast('could not create fillable form'); return; }
  openSaveAs(await res.blob(), exportBase() + '-fillable.pdf', 'Save fillable PDF');
};

els.exportPngBtn.onclick = async () => {
  if (!pdfDocument) return;
  const [{ blob }] = await renderFilledPages(2, viewer.currentPageNumber);
  openSaveAs(blob, exportBase() + '-page' + viewer.currentPageNumber + '.png', 'Export page (PNG)');
};

els.exportTextBtn.onclick = async () => {
  if (!pdfDocument) return toast('Open a PDF first');
  // Best-effort dump of the document's text layer via pdf.js (see documentText:
  // content-stream order, hasEOL newlines; scanned pages have no text layer and
  // contribute nothing). Compare uses the same helper so the two stay in step.
  const out = await documentText(pdfDocument);
  openSaveAs(new Blob([out], { type: 'text/plain' }), exportBase() + '.txt', 'Export text (.txt)');
};

// extractTable clusters a page's pdf.js text items into a row/column grid — the
// canonical position-based table extraction (works on ruled and unruled grid
// tables, since it reads text alignment, not drawn rules). It is best-effort:
// merged cells, multi-line cells, and irregular layouts mis-extract, so the export
// UI tells the user to review the result. Returns rows of cell strings.
function median(xs) {
  if (!xs.length) return 0;
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.floor(s.length / 2)];
}
async function extractTable(page) {
  const tc = await page.getTextContent();
  const items = tc.items
    .filter((it) => it.str && it.str.trim() !== '')
    .map((it) => ({ str: it.str.trim(), x: it.transform[4], y: it.transform[5], h: it.height || Math.hypot(it.transform[2], it.transform[3]) || 10 }));
  if (!items.length) return [];
  const h = median(items.map((i) => i.h)) || 10;
  const rowTol = h * 0.6, colTol = h * 1.5; // same baseline → same row; x-starts within colTol → same column

  // Rows: group by baseline y (PDF y grows upward, so top of page first).
  items.sort((a, b) => b.y - a.y);
  const rows = [];
  for (const it of items) {
    let row = rows.find((r) => Math.abs(r.y - it.y) <= rowTol);
    if (!row) { row = { y: it.y, items: [] }; rows.push(row); }
    row.items.push(it);
  }

  // Columns: cluster all item x-starts into left-edge anchors.
  const anchors = [];
  for (const x of items.map((i) => i.x).sort((a, b) => a - b)) {
    if (!anchors.length || x - anchors[anchors.length - 1] > colTol) anchors.push(x);
  }
  const colOf = (x) => {
    let best = 0;
    for (let i = 1; i < anchors.length; i++) if (Math.abs(x - anchors[i]) < Math.abs(x - anchors[best])) best = i;
    return best;
  };

  return rows.map((row) => {
    const cells = new Array(anchors.length).fill('');
    row.items.sort((a, b) => a.x - b.x);
    for (const it of row.items) {
      const c = colOf(it.x);
      cells[c] = cells[c] ? cells[c] + ' ' + it.str : it.str;
    }
    return cells;
  });
}

// exportTable extracts the current page's table and saves it as a spreadsheet.
// The grid is built client-side (only pdf.js can read PDF text); the server just
// serializes it (CSV via encoding/csv, XLSX as minimal OOXML).
async function exportTable(format) {
  if (!pdfDocument) return toast('Open a PDF first');
  const page = await pdfDocument.getPage(viewer.currentPageNumber);
  const grid = await extractTable(page);
  if (!grid.length) return toast('No text on this page to extract (a scanned page? run OCR first)');
  const res = await apiFetch('/api/table?format=' + format, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ grid }),
  });
  if (!res.ok) { toast('Could not build the spreadsheet'); return; }
  const ext = { csv: '.csv', ods: '.ods', xlsx: '.xlsx' }[format] || '.xlsx';
  openSaveAs(await res.blob(), exportBase() + '-p' + viewer.currentPageNumber + '-table' + ext, 'Export table (' + format.toUpperCase() + ')');
}
els.exportTableXlsxBtn.onclick = () => exportTable('xlsx');
els.exportTableCsvBtn.onclick = () => exportTable('csv');
els.exportTableOdsBtn.onclick = () => exportTable('ods');

els.exportImagesBtn.onclick = async () => {
  if (!pdfDocument) return toast('Open a PDF first');
  const res = await apiFetch('/api/extract-images', { method: 'POST' });
  if (!res.ok) { toast('Could not extract images'); return; }
  if (res.headers.get('X-Image-Count') === '0') { toast('No extractable images found'); return; }
  openSaveAs(await res.blob(), exportBase() + '-images.zip', 'Export embedded images (ZIP)');
};

els.exportFormJsonBtn.onclick = () => { window.location = '/api/form-data?format=json'; };
els.exportFormCsvBtn.onclick = () => { window.location = '/api/form-data?format=csv'; };
els.exportFormXfdfBtn.onclick = () => { window.location = '/api/form-data?format=xfdf'; };
els.exportCertBtn.onclick = () => { window.location = '/api/identity'; };

els.printBtn.onclick = async () => {
  if (!pdfDocument) return toast('Open a PDF first');
  // Print the real PDF bytes (WYSIWYG, vector) through the browser's own print
  // dialog. Printing the on-screen #viewer would capture pdf.js's screen-DPI
  // page canvases plus the app chrome, not the document — so feed the same
  // baked bytes Save/Export use into a hidden iframe and print that. A hidden
  // iframe (not window.open) sidesteps popup blockers; the Blob URL must outlive
  // the print call, so it's revoked on afterprint with a timeout fallback for
  // browsers that never fire the event.
  let bytes;
  try { bytes = await bakedBytes(); } catch (e) { console.error('print bake failed', e); return toast('could not prepare the document to print'); }
  const url = URL.createObjectURL(new Blob([bytes], { type: 'application/pdf' }));
  const frame = document.createElement('iframe');
  frame.style.display = 'none';
  frame.src = url;
  let cleaned = false;
  const cleanup = () => { if (cleaned) return; cleaned = true; URL.revokeObjectURL(url); frame.remove(); };
  frame.onload = () => {
    frame.contentWindow.focus();
    frame.contentWindow.addEventListener('afterprint', cleanup);
    frame.contentWindow.print();
    setTimeout(cleanup, 60000);
  };
  document.body.appendChild(frame);
};

// Finalize & sign.
els.finalizeBtn.onclick = async () => {
  if (!pdfDocument) return;
  await refreshSignAs();
  els.finalizeModal.hidden = false;
};
els.fzCancel.onclick = () => { els.finalizeModal.hidden = true; };

// refreshSignAs rebuilds the "Sign as" picker: always the native Nib identity,
// plus an imported certificate when one is stored. The passphrase field shows
// only while the imported cert is selected.
async function refreshSignAs() {
  els.fzSignAs.querySelectorAll('option[value="external"]').forEach((o) => o.remove());
  try {
    const info = await (await apiFetch('/api/identity/external')).json();
    if (info.present) {
      const o = document.createElement('option');
      o.value = 'external';
      o.textContent = 'Imported: ' + (info.subject || 'certificate');
      els.fzSignAs.appendChild(o);
    }
  } catch { /* leave native-only */ }
  syncSignAs();
}
function syncSignAs() { els.fzPassphrase.hidden = els.fzSignAs.value !== 'external'; }
els.fzSignAs.onchange = () => { syncSignAs(); if (els.fzSignAs.value === 'external') els.fzPassphrase.focus(); };
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
  const signAs = els.fzSignAs.value;
  if (signAs === 'external' && !els.fzPassphrase.value) { els.fzPassphrase.focus(); return toast('Enter the certificate passphrase'); }
  let text = els.fzText.value.trim();
  if (text && els.fzDate.checked) text += ' ' + new Date().toLocaleDateString();

  const form = await bakedForm();
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
    signAs,
    passphrase: signAs === 'external' ? els.fzPassphrase.value : '',
  }));
  const res = await apiFetch('/api/finalize', { method: 'POST', body: form });
  if (res.status === 401) { els.fzPassphrase.focus(); els.fzPassphrase.select(); return toast('Wrong certificate passphrase'); }
  if (!res.ok) { toast('Could not finalize'); return; }
  els.finalizeModal.hidden = true;
  els.fzPassphrase.value = '';
  openSaveAs(await res.blob(), exportBase() + '-finalized.pdf', 'Save finalized PDF');
};

// OpenTimestamps: stamp the current document's hash into the Bitcoin blockchain
// and save the .ots proof as a sidecar. The proof is over the same baked bytes
// Finalize signs, so it matches what the user keeps; the PDF itself is untouched.
els.timestampBtn.onclick = () => { if (pdfDocument) els.timestampModal.hidden = false; };
els.tsCancel.onclick = () => { els.timestampModal.hidden = true; };
els.tsGo.onclick = async () => {
  els.timestampModal.hidden = true;
  const form = await bakedForm();
  toast('Contacting OpenTimestamps calendars…');
  const res = await apiFetch('/api/timestamp', { method: 'POST', body: form });
  if (!res.ok) { toast('Could not reach an OpenTimestamps calendar'); return; }
  openSaveAs(await res.blob(), exportBase() + '.pdf.ots', 'Save OpenTimestamps proof');
};

// Verify an OpenTimestamps proof against the open document: pick the .ots, send it
// with the document's bytes, and report whether (and when) it's anchored to Bitcoin.
let upgradedProofB64 = null; // a now-complete .ots offered for saving after verify
els.timestampVerifyBtn.onclick = () => {
  if (!pdfDocument) return;
  els.tvResult.hidden = true; els.tvResult.textContent = '';
  els.tvSave.hidden = true; upgradedProofB64 = null;
  els.tsVerifyModal.hidden = false;
};
els.tvCancel.onclick = () => { els.tsVerifyModal.hidden = true; };
els.tvSave.onclick = () => {
  if (!upgradedProofB64) return;
  openSaveAs(b64ToBlob(upgradedProofB64, 'application/octet-stream'), exportBase() + '.ots', 'Save complete proof');
};
els.tvExplorerOn.onchange = () => { els.tvExplorer.disabled = !els.tvExplorerOn.checked; };
els.tvPick.onclick = () => els.tvFile.click();
els.tvFile.onchange = async () => {
  const f = els.tvFile.files[0];
  els.tvFile.value = '';
  if (!f) return;
  els.tvResult.hidden = false;
  els.tvResult.textContent = 'Checking…';
  els.tvSave.hidden = true; upgradedProofB64 = null;
  try {
    const form = await bakedForm();
    form.append('ots', f, f.name);
    if (els.tvExplorerOn.checked && els.tvExplorer.value.trim()) form.append('explorer', els.tvExplorer.value.trim());
    const res = await apiFetch('/api/timestamp/verify', { method: 'POST', body: form });
    if (!res.ok) {
      const e = await res.json().catch(() => ({}));
      els.tvResult.textContent = '✗ ' + (e.error || 'verification failed');
      return;
    }
    const r = await res.json();
    els.tvResult.textContent = timestampVerifyMessage(r);
    if (r.upgraded) { upgradedProofB64 = r.upgraded; els.tvSave.hidden = false; }
  } catch {
    els.tvResult.textContent = '✗ verification failed';
  }
};

function timestampVerifyMessage(r) {
  switch (r.state) {
    case 'confirmed': {
      const when = r.time ? new Date(r.time).toLocaleString() : 'a Bitcoin block';
      const src = (r.sources || 1) > 1 ? (r.sources + ' sources') : '1 source';
      return `✓ Existed, unaltered, by ${when} — anchored to Bitcoin block ${r.height} (confirmed by ${src}).`;
    }
    case 'pending':
      return '⏳ Not yet confirmed — still waiting on a Bitcoin block. Try again in a few hours.';
    case 'mismatch':
      return '✗ This proof is not for the open document — its hash doesn’t match.';
    case 'invalid':
      return '✗ The proof does not match the Bitcoin blockchain — it may be forged or corrupt.';
    default:
      return 'Unexpected verification result.';
  }
}

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
  if (res.ok) profileCache = null; // a text flag re-reads the fresh values
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
  if (redactMode) { setMarkerMode(null); exitSplitBox(); exitBorder(); exitCrop(); exitNote(); exitDropdown(); exitRadio(); exitShape(); } // one box tool at a time
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

// --- search / pattern redaction ----------------------------------------------
// Find every occurrence of a term or PII pattern in the text layer and mark each
// one for redaction, feeding the SAME redactMarks → applyRedact pipeline as the
// hand-drawn boxes. All client-side: pdf.js is the only place the text layer
// lives (the Go engine can't extract text). Marks are reviewed as boxes, then the
// user presses Apply — the existing irreversible, true-removal bake.
const PII_PATTERNS = {
  // Fixed (not user-supplied), so no catastrophic-backtracking risk. Separators
  // are optional so the common formatted and bare forms both match.
  rtSSN: /\b\d{3}[-.\s]?\d{2}[-.\s]?\d{4}\b/g,
  rtEmail: /\b[\w.+-]+@[\w-]+\.[\w.-]+\b/g,
  rtPhone: /(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b/g,
  rtCard: /\b(?:\d[ -]?){13,19}\b/g, // 19 covers the long Maestro/UnionPay PANs, not just 13-16

};
function escapeRegExp(s) { return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); }

// drawRedactMarks (re)draws every redaction box for one rendered page from
// redactMarks (page-content top-left fractions → the page's live content box).
// relayoutRedactMarks does it for all rendered pages, and both run on every
// pagerendered/scalechanging — so marks show on EVERY marked page and reflow on
// zoom, not just the page that was on screen when they were generated. (The boxes
// are pure review overlays; the source of truth is redactMarks, which applyRedact
// flattens page by page regardless of what's currently drawn.)
function drawRedactMarks(pv, pageNum) {
  pv.div.querySelectorAll('.redactmark').forEach((el) => el.remove());
  const cr = pageContentRect(pv.div);
  for (const m of redactMarks) {
    if (m.page !== pageNum) continue;
    const div = document.createElement('div');
    div.className = 'redactmark';
    div.style.left = (m.fx * cr.width) + 'px';
    div.style.top = (m.fy * cr.height) + 'px';
    div.style.width = (m.fw * cr.width) + 'px';
    div.style.height = (m.fh * cr.height) + 'px';
    pv.div.appendChild(div);
  }
}
function relayoutRedactMarks() {
  // No marks → nothing to draw or sweep (marks only reach length 0 via a full
  // document reload, which tears the boxes down with the old page DOM). Bail
  // before the all-pages walk: this runs on every pagerendered/scalechanging,
  // so it's scroll-hot-path work in the common no-marks case.
  if (!pdfDocument || !redactMarks.length) return;
  for (let i = 0; i < pdfDocument.numPages; i++) {
    const pv = viewer.getPageView(i);
    if (pv?.div) drawRedactMarks(pv, i + 1);
  }
}
eventBus.on('pagerendered', relayoutRedactMarks);
eventBus.on('scalechanging', relayoutRedactMarks);

// scanTextMatches walks every page's text, runs each pattern over the per-row
// reconstructed string (buildTextRows — the same per-character geometry the field
// detectors use), and returns over-covering marks for each hit. Text runs are
// mapped into top-left page space (like the Detect path) so the box fractions go
// straight into redactMarks. Because pdf.js fragments a run at arbitrary points and
// buildTextRows joins fragments with a space, a "compact" string drops those
// injected boundary spaces (cx === NaN) so a pattern split across runs still matches.
async function scanTextMatches(patterns) {
  const marks = [];
  for (let n = 1; n <= pdfDocument.numPages; n++) {
    const page = await pdfDocument.getPage(n);
    // Match + build the box in the UNROTATED viewport, where buildTextRows' "text
    // advances horizontally" assumption holds; vp is the rendered viewport (page
    // /Rotate applied) the finished box is mapped back into — what the apply bake
    // and the review overlay actually use. For a /Rotate 0 page the two are equal,
    // so the mapping is the identity and the output is unchanged.
    const vp0 = page.getViewport({ scale: 1, rotation: 0 });
    const vp = page.getViewport({ scale: 1 });
    let tc;
    try { tc = await page.getTextContent(); } catch { continue; } // image-only: no text
    // Keep pdf.js's whitespace-only items (str " "): they carry the word gap between
    // adjacent runs. Dropping them (a trim() filter) let a styled inline run abut its
    // neighbour — "SSN " + bold "123-45-6789" → "SSN123-45-6789" — so the patterns'
    // leading \b stopped matching and the PII leaked past the bake. The compact string
    // still drops the synthetic inter-run boundary, so true mid-token splits rejoin.
    const items = tc.items.filter((it) => it.str).map((it) => {
      const t = pdfjsLib.Util.transform(vp0.transform, it.transform);
      return { str: it.str, x: t[4], y: t[5], w: it.width, h: it.height || Math.hypot(it.transform[2], it.transform[3]) };
    });
    for (const row of buildTextRows(items)) {
      let compact = ''; const map = [];
      for (let k = 0; k < row.s.length; k++) {
        if (Number.isNaN(row.cx[k])) continue; // injected inter-run boundary space
        compact += row.s[k]; map.push(k);
      }
      for (const re of patterns) {
        re.lastIndex = 0;
        let m;
        while ((m = re.exec(compact))) {
          if (!m[0].length) { re.lastIndex++; continue; } // zero-width guard
          const lo = map[m.index], hi = map[m.index + m[0].length - 1];
          let x0 = Infinity, x1 = -Infinity, hh = 0;
          for (let k = lo; k <= hi; k++) {
            if (Number.isNaN(row.cx[k])) continue;
            x0 = Math.min(x0, row.cx[k]); x1 = Math.max(x1, row.cx[k]); hh = Math.max(hh, row.ch[k]);
          }
          if (!isFinite(x0)) continue;
          // cx are glyph centres and the advance is an estimate, so pad generously:
          // redaction must over-cover, never leave an edge of the match showing.
          const bx0 = x0 - hh * 0.8, bx1 = x1 + hh * 0.8;
          const by0 = row.y - hh * 1.15, by1 = row.y + hh * 0.45; // baseline (y-down) ± ascender/descender
          // Map the unrotated box to the rendered viewport (vp0 → PDF → vp) so it
          // lands on the glyphs on a rotated page; the axis-aligned bounding box of
          // the four mapped corners is the redaction rect. Identity when /Rotate 0.
          let rx0 = Infinity, ry0 = Infinity, rx1 = -Infinity, ry1 = -Infinity;
          for (const p of [[bx0, by0], [bx1, by0], [bx1, by1], [bx0, by1]]) {
            const pdf = vp0.convertToPdfPoint(p[0], p[1]);     // unrotated viewport → PDF
            const r = vp.convertToViewportPoint(pdf[0], pdf[1]); // PDF → rendered viewport
            rx0 = Math.min(rx0, r[0]); ry0 = Math.min(ry0, r[1]);
            rx1 = Math.max(rx1, r[0]); ry1 = Math.max(ry1, r[1]);
          }
          marks.push({ page: n, fx: rx0 / vp.width, fy: ry0 / vp.height,
            fw: (rx1 - rx0) / vp.width, fh: (ry1 - ry0) / vp.height });
        }
      }
    }
  }
  return marks;
}

function openRedactText() {
  if (!pdfDocument) return toast('Open a PDF first');
  els.rtStatus.textContent = '';
  els.redactTextModal.hidden = false;
}
els.redactTextBtn.onclick = openRedactText;
els.rtCancel.onclick = () => { els.redactTextModal.hidden = true; };
els.rtFind.onclick = async () => {
  const patterns = [];
  const term = els.rtTerm.value.trim();
  if (term) patterns.push(new RegExp(escapeRegExp(term), 'gi'));
  for (const id of ['rtSSN', 'rtEmail', 'rtPhone', 'rtCard']) {
    if (els[id].checked) patterns.push(new RegExp(PII_PATTERNS[id].source, 'g'));
  }
  if (!patterns.length) { els.rtStatus.textContent = 'Enter a word/phrase or tick a pattern.'; return; }
  els.rtStatus.textContent = 'Searching…';
  const marks = await scanTextMatches(patterns);
  if (!marks.length) {
    els.rtStatus.textContent = 'No matches found in the text layer (a scan? run OCR first).';
    return;
  }
  const pages = new Set(marks.map((m) => m.page));
  redactMarks.push(...marks); // {page,fx,fy,fw,fh}; drawn per page on render
  relayoutRedactMarks();
  els.redactTextModal.hidden = true;
  toast(`${marks.length} match(es) marked on ${pages.size} page(s) — review the boxes (scroll to see them all), then “Apply redactions”.`);
};

// --- split by hand-drawn regions ---------------------------------------------
// Draw rectangles on the current page; on Apply, each becomes its own page (the
// page cropped to that rectangle, server-side via op:'splitrects'). The regions
// live OUTSIDE overlayFields, so the bake step never burns them in as marks.
let splitBoxMode = false;
let splitRects = []; // {fx, fy, fw, fh} fractions of the page (top-left origin)
let sbStart = null, sbDiv = null, sbHit = null, sbPage = 0;

function reflectSplitBox() {
  all('#splitBoxBtn, [data-forward="splitBoxBtn"]').forEach((b) => b.classList.toggle('active', splitBoxMode));
}
function clearSplitRects() {
  splitRects = [];
  all('.splitmark').forEach((d) => d.remove());
}
function exitSplitBox() {
  if (!splitBoxMode) return;
  splitBoxMode = false;
  clearSplitRects();
  reflectSplitBox();
  els.viewerContainer.style.cursor = '';
}
els.splitBoxBtn.onclick = () => {
  if (splitBoxMode) { exitSplitBox(); return; }
  if (!pdfDocument) return;
  splitBoxMode = true;
  sbPage = viewer.currentPageNumber; // regions apply to the page you start on
  setMarkerMode(null);
  exitBorder();
  exitShape();
  exitCrop();
  exitNote(); exitDropdown(); exitRadio();
  if (redactMode) { redactMode = false; reflectRedact(); }
  if (editMode) { editMode = false; reflectEdit(); }
  reflectSplitBox();
  els.viewerContainer.style.cursor = 'crosshair';
};

els.viewerContainer.addEventListener('pointerdown', (e) => {
  if (!splitBoxMode) return;
  sbHit = pageAt(e.clientX, e.clientY);
  if (!sbHit || sbHit.n !== sbPage) return; // only the page the split started on
  sbStart = { x: e.clientX, y: e.clientY };
  sbDiv = document.createElement('div');
  sbDiv.className = 'splitmark';
  sbHit.pv.div.appendChild(sbDiv);
  sizeMark(sbDiv, sbHit.r, sbStart, sbStart);
  e.preventDefault();
});
els.viewerContainer.addEventListener('pointermove', (e) => {
  if (sbStart) sizeMark(sbDiv, sbHit.r, sbStart, { x: e.clientX, y: e.clientY });
});
els.viewerContainer.addEventListener('pointerup', (e) => {
  if (!sbStart) return;
  const r = sbHit.r;
  const x0 = Math.min(sbStart.x, e.clientX), y0 = Math.min(sbStart.y, e.clientY);
  const fw = Math.abs(e.clientX - sbStart.x) / r.width;
  const fh = Math.abs(e.clientY - sbStart.y) / r.height;
  if (fw > 0.01 && fh > 0.01) {
    splitRects.push({ fx: (x0 - r.left) / r.width, fy: (y0 - r.top) / r.height, fw, fh });
  } else {
    sbDiv.remove();
  }
  sbStart = null; sbDiv = null; sbHit = null;
});

els.applyBoxSplitBtn.onclick = async () => {
  if (!splitBoxMode || !splitRects.length) return toast('Draw split regions first');
  const page = sbPage;
  const base = (await pdfDocument.getPage(page)).getViewport({ scale: 1 }); // PDF points
  const f = { pageW: base.width, pageH: base.height };
  const rects = splitRects.map((m) => rectPoints(f, [m.fx, m.fy, m.fx + m.fw, m.fy + m.fh]));
  splitBoxMode = false;
  clearSplitRects();
  reflectSplitBox();
  els.viewerContainer.style.cursor = '';
  const ok = await pageOp('splitrects', { page, rects: JSON.stringify(rects) });
  if (ok) toast(rects.length === 1
    ? `Page ${page} replaced by 1 region`
    : `Page ${page} split into ${rects.length} regions`);
};

// --- crop: trim pages down to a drawn box ------------------------------------
// Draw ONE keep-rectangle on the current page; on confirm, every page (or just
// this one) is trimmed to it server-side via op:'crop' → pdfops.Crop. Like the
// split regions, the mark lives OUTSIDE overlayFields so the bake never burns it
// in. Reuses the same display-space → PDF-points conversion (rectPoints) as Split
// by box; the server flattens any /Rotate before cropping.
let cropMode = false;
let cropRect = null; // {fx, fy, fw, fh} fraction of the page (top-left origin)
let cropStart = null, cropDiv = null, cropHit = null, cropPage = 0;

function reflectCrop() {
  all('#cropBtn, [data-forward="cropBtn"]').forEach((b) => b.classList.toggle('active', cropMode));
}
function clearCropRect() {
  cropRect = null;
  all('.cropmark').forEach((d) => d.remove());
}
function exitCrop() {
  if (!cropMode) return;
  cropMode = false;
  clearCropRect();
  els.cropModal.hidden = true;
  reflectCrop();
  els.viewerContainer.style.cursor = '';
}
els.cropBtn.onclick = () => {
  if (cropMode) { exitCrop(); return; }
  if (!pdfDocument) return;
  cropMode = true;
  cropPage = viewer.currentPageNumber; // the box is measured in this page's space
  setMarkerMode(null);
  exitBorder();
  exitShape();
  exitSplitBox();
  exitNote(); exitDropdown(); exitRadio();
  if (redactMode) { redactMode = false; reflectRedact(); }
  if (editMode) { editMode = false; reflectEdit(); }
  reflectCrop();
  els.viewerContainer.style.cursor = 'crosshair';
  toast('Draw the area to keep, then confirm');
};

els.viewerContainer.addEventListener('pointerdown', (e) => {
  if (!cropMode) return;
  cropHit = pageAt(e.clientX, e.clientY);
  if (!cropHit || cropHit.n !== cropPage) return; // measure on the page you started on
  clearCropRect(); // a single keep-rectangle: a fresh draw replaces the old one
  cropStart = { x: e.clientX, y: e.clientY };
  cropDiv = document.createElement('div');
  cropDiv.className = 'cropmark';
  cropHit.pv.div.appendChild(cropDiv);
  sizeMark(cropDiv, cropHit.r, cropStart, cropStart);
  e.preventDefault();
});
els.viewerContainer.addEventListener('pointermove', (e) => {
  if (cropStart) sizeMark(cropDiv, cropHit.r, cropStart, { x: e.clientX, y: e.clientY });
});
els.viewerContainer.addEventListener('pointerup', (e) => {
  if (!cropStart) return;
  const r = cropHit.r;
  const x0 = Math.min(cropStart.x, e.clientX), y0 = Math.min(cropStart.y, e.clientY);
  const fw = Math.abs(e.clientX - cropStart.x) / r.width;
  const fh = Math.abs(e.clientY - cropStart.y) / r.height;
  cropStart = null; cropHit = null;
  if (fw > 0.01 && fh > 0.01) {
    cropRect = { fx: (x0 - r.left) / r.width, fy: (y0 - r.top) / r.height, fw, fh };
    els.cropModal.hidden = false; // confirm: all-pages choice + the honest note
  } else if (cropDiv) {
    cropDiv.remove(); cropDiv = null;
  }
});

els.cropCancel.onclick = () => { els.cropModal.hidden = true; clearCropRect(); }; // stay in crop mode for a redraw
els.cropGo.onclick = async () => {
  if (!cropRect) { els.cropModal.hidden = true; return; }
  const page = cropPage;
  // Send the keep-box as page fractions (top-left origin) [fx, fy, fw, fh]; the
  // server scales it to each target page's own size, so a mixed-size document
  // crops proportionally instead of to a fixed window. On a uniform document this
  // is identical to the old absolute-points path.
  const frac = [cropRect.fx, cropRect.fy, cropRect.fw, cropRect.fh];
  const allPages = els.cropAllPages.checked;
  exitCrop();
  const ok = await pageOp('crop', { rect: JSON.stringify(frac), pages: allPages ? '' : String(page) });
  if (ok) toast(allPages ? 'All pages cropped to the box' : `Page ${page} cropped to the box`);
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
  if (editMode) { setMarkerMode(null); exitSplitBox(); exitBorder(); exitCrop(); exitNote(); exitDropdown(); exitRadio(); exitShape(); }
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
  recordAdd(f);
  return f;
}

// --- sign / date / initial markers -------------------------------------------
// Place "fill here" markers (kind 'marker') on the page, then walk through them:
// clicking a marker fills it and auto-advances to the next. A date marker stamps
// today's date; a sign/initial marker drops your signature/initials image (picked
// from the Library once, then reused). Filling REPLACES the marker with an
// ordinary image stamp (makeStamp), so markers are pure placeholders that never
// bake — only the resulting stamps do, via the existing collectStamps path.
//
// These place a VISIBLE appearance for filling out a form; they are NOT a
// cryptographic signature — that remains the separate Finalize & sign step.
let markerMode = null;        // 'sign' | 'date' | 'initial' while armed to place
let activeMarker = null;      // the marker highlighted as the current fill target
let fillTarget = null;        // a sign/initial marker awaiting a Library pick
let markerSig = null, markerInit = null; // remembered sign/initial fill sources for this session
let docHadFlags = false;      // the open document arrived with embedded signing flags
let signLocked = false;       // "signing marks completed": doc is signing-only, non-editable
let signTotal = 0;            // flag count when the signing banner appeared, for "X of N"
let signStarted = false;      // the recipient has hit Start, so the action is now "Next field"

// Two gates govern what can be done to flags. flagsEditable: the preparer may place,
// drag, and delete flags (only while unlocked). flagsFillable: a flag may be clicked
// to fill it — true while unlocked (a preparer self-filling a form) and, even when
// locked, on a received document (the recipient signs but can't reshape the layout).
function flagsEditable() { return !signLocked; }
function flagsFillable() { return !signLocked || docHadFlags; }
const MARKER_SIZES = {
  sign: [0.22, 0.05], date: [0.13, 0.035], initial: [0.07, 0.05],
  name: [0.24, 0.035], title: [0.18, 0.035], company: [0.24, 0.035],
};
const MARKER_LABELS = { sign: 'Sign', date: 'Date', initial: 'Initial', name: 'Name', title: 'Title', company: 'Company' };
// Text flags fill from the autofill profile (typed values), as sign/initial fill
// from the Library (images) and date fills automatically — three fill sources.
const TEXT_MARKERS = new Set(['name', 'title', 'company']);
// Profile keys (case-insensitive) each text flag accepts, in preference order.
const TEXT_MARKER_KEYS = {
  name: ['name', 'full name', 'fullname'],
  title: ['title', 'job title'],
  company: ['company', 'organization', 'organisation', 'employer'],
};

document.querySelectorAll('.markers button').forEach((b) => {
  b.onclick = () => { if (!pdfDocument) return toast('Open a PDF first'); setMarkerMode(markerMode === b.dataset.marker ? null : b.dataset.marker); };
});
els.saveForSigningBtn.onclick = () => { if (!pdfDocument) return toast('Open a PDF first'); saveForSigning(); };

// "Signing marks completed" locks the document into signing-only mode: flag
// placement and every content-editing tool turn off and the placed flags freeze.
// Toggling it ("Edit marks again") restores editing. A received document (one that
// arrived with flags) opens locked with no toggle — it is the counterparty's copy
// and must stay non-editable; see setDocumentFromServer.
els.signCompleteBtn.onclick = () => {
  if (!pdfDocument) return;
  if (!signLocked && !markerFields().length) return toast('Place at least one flag first.');
  setSignLocked(!signLocked);
  toast(signLocked
    ? 'Marks locked — the document is in signing mode and can no longer be edited.'
    : 'Editing re-enabled — you can change the flags again.');
};

// The content-editing tools (Edit + Protect menus) that signing mode switches off.
// Certify (cryptographic signing/timestamp/co-sign), File, and View stay available —
// they are part of, or orthogonal to, signing the document.
const EDITING_TOOLS = [
  'textToolBtn', 'highlightToolBtn', 'drawToolBtn', 'detectBtn',
  'editTextBtn', 'removeOriginalsBtn', 'autofillBtn',
  'redactBtn', 'redactTextBtn', 'applyRedactBtn', 'scanBtn',
];
function setEditingEnabled(on) {
  for (const id of EDITING_TOOLS) {
    const b = $(id); if (b) b.disabled = !on;
    all(`#toolbar [data-forward="${id}"]`).forEach((t) => { t.disabled = !on; });
  }
  all('#toolbar [data-mode]').forEach((t) => { t.disabled = !on; }); // Text/Highlight/Draw twins
}

function setSignLocked(locked) {
  signLocked = locked;
  if (locked) { // leaving any active editing tool behind would be a dead, disabled mode
    setMarkerMode(null);
    if (redactMode) { redactMode = false; reflectRedact(); els.viewerContainer.style.cursor = ''; }
    if (editMode) { editMode = false; reflectEdit(); els.viewerContainer.style.cursor = ''; }
  }
  applySignLock();
}

// applySignLock reflects signLocked across the whole UI. Safe to call whenever the
// lock, the open document, or the flag count changes.
function applySignLock() {
  const open = !!pdfDocument;
  setEditingEnabled(open && !signLocked);
  els.viewerWrap.classList.toggle('signing-locked', signLocked);
  reflectSignControls();
}

// reflectSignControls keeps the Flags-panel controls consistent with the role
// (preparer vs recipient) and the lock state.
function reflectSignControls() {
  const open = !!pdfDocument;
  const recipient = docHadFlags;            // opened with flags => the counterparty's copy
  const n = markerFields().length;
  // A recipient never prepares; a locked preparer can't place until they edit again.
  all('.markers button').forEach((b) => { b.disabled = !open || recipient || signLocked; });
  els.saveForSigningBtn.disabled = !open || recipient;
  els.signCompleteBtn.hidden = !open || recipient;
  els.signCompleteBtn.disabled = !signLocked && n === 0;
  els.signCompleteBtn.textContent = signLocked ? 'Edit marks again' : 'Signing marks completed';
  els.signCompleteBtn.classList.toggle('active', signLocked);
}

function setMarkerMode(m) {
  markerMode = m;
  if (m) { // one placement tool at a time
    if (redactMode) { redactMode = false; reflectRedact(); }
    if (editMode) { editMode = false; reflectEdit(); }
    exitSplitBox();
    exitBorder();
    exitShape();
    exitCrop();
    exitNote(); exitDropdown(); exitRadio();
  }
  all('.markers button').forEach((b) => b.classList.toggle('active', b.dataset.marker === m));
  els.viewerContainer.style.cursor = m ? 'crosshair' : '';
}

// A single click on the page drops a default-sized marker centred at the click.
els.viewerContainer.addEventListener('pointerdown', (e) => {
  if (!markerMode) return;
  const hit = pageAt(e.clientX, e.clientY);
  if (!hit) return;
  e.preventDefault();
  // Snap to a detected blank if the click landed on one (the primary flow: Detect,
  // then flag each blank); otherwise free-place a default flag (the fallback).
  const field = overlayFields.find((f) => f.kind === 'text' && f.el.contains(e.target));
  if (field) return void convertFieldToFlag(field, markerMode);
  const r = hit.r, [fw, fh] = MARKER_SIZES[markerMode];
  const fx = Math.min(Math.max((e.clientX - r.left) / r.width - fw / 2, 0), 1 - fw);
  const fy = Math.min(Math.max((e.clientY - r.top) / r.height - fh / 2, 0), 1 - fh);
  makeMarker(markerMode, [fx, fy, fx + fw, fy + fh], hit);
});

// convertFieldToFlag turns a detected blank into a sign/date/initial flag bound to
// that blank. It keeps the blank's horizontal span and baseline, but gives the flag
// at least the type's default height — a detected underline is only ~15pt tall, which
// would crush a signature, so we rise to the type's height when the blank is shorter.
function convertFieldToFlag(field, type) {
  const [fx0, fy0, fx1, fy1] = field.frac;
  const fh = MARKER_SIZES[type][1];
  const top = Math.max(0, Math.min(fy0, fy1 - fh));
  removeField(field, false); // internal transform — not its own undo step
  const f = buildMarker(type, [fx0, top, fx1, fy1], field.page, false);
  const pv = viewer.getPageView(field.page - 1);
  if (pv?.div) { pv.div.appendChild(f.el); layoutField(f, pv); }
  return f;
}

// buildMarker creates a marker field + its DOM element and registers it, but does
// NOT attach it to a page — relayoutOverlays places it once the page renders.
// This lets reconstructFlags rebuild markers on pages that aren't on screen yet.
function buildMarker(type, frac, page, record = true) {
  const f = { page, frac, kind: 'marker', tagType: type };
  const el = document.createElement('div');
  el.className = 'ovl ovl-marker';
  el.tabIndex = 0;
  const label = document.createElement('span');
  label.className = 'marker-label';
  label.textContent = MARKER_LABELS[type];
  const del = document.createElement('button');
  del.className = 'marker-del'; del.textContent = '×'; del.title = 'Remove flag';
  del.onclick = (e) => { e.stopPropagation(); if (flagsEditable()) removeField(f); };
  el.append(label, del);
  f.el = el;
  enableMarkerGestures(f, el);
  el.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); if (flagsFillable()) fillMarker(f); }
    else if ((e.key === 'Delete' || e.key === 'Backspace') && flagsEditable()) removeField(f);
  });
  overlayFields.push(f);
  reflectSignControls();
  if (record) {
    recordOverlayEdit({
      undo: () => { detachField(f); reflectSignControls(); },
      redo: () => { reattachField(f); reflectSignControls(); },
    });
  }
  return f;
}

function makeMarker(type, frac, hit) {
  const f = buildMarker(type, frac, hit.n);
  hit.pv.div.appendChild(f.el);
  layoutField(f, hit.pv);
  return f;
}

function removeField(f, record = true) {
  if (record) {
    recordOverlayEdit({
      undo: () => { reattachField(f); reflectSignControls(); },
      redo: () => { detachField(f); reflectSignControls(); },
    });
  }
  detachField(f);
  if (activeMarker === f) activeMarker = null;
  if (fillTarget === f) fillTarget = null;
  reflectSignControls();
}

// enableMarkerGestures: drag repositions; a click (no drag) fills the marker. Both
// honour the signing-mode gates — a frozen flag (locked preparer copy) ignores the
// pointer entirely; a received flag fills but can't be dragged.
function enableMarkerGestures(f, el) {
  let down = null, moved = false;
  el.addEventListener('pointerdown', (e) => {
    if (e.target.closest('.marker-del')) return;
    if (!flagsFillable() && !flagsEditable()) return; // frozen placeholder
    e.preventDefault(); e.stopPropagation();
    down = { x: e.clientX, y: e.clientY, frac: f.frac.slice() }; moved = false;
    el.setPointerCapture(e.pointerId);
  });
  el.addEventListener('pointermove', (e) => {
    if (!down || !flagsEditable()) return; // a recipient signs but can't reshape the layout
    const pv = viewer.getPageView(f.page - 1); if (!pv?.div) return;
    const W = pv.div.clientWidth, H = pv.div.clientHeight;
    if (Math.abs(e.clientX - down.x) + Math.abs(e.clientY - down.y) > 3) moved = true;
    const [x0, y0, x1, y1] = down.frac, w = x1 - x0, h = y1 - y0;
    const nx = Math.min(Math.max(x0 + (e.clientX - down.x) / W, 0), 1 - w);
    const ny = Math.min(Math.max(y0 + (e.clientY - down.y) / H, 0), 1 - h);
    f.frac = [nx, ny, nx + w, ny + h]; layoutField(f, pv);
  });
  el.addEventListener('pointerup', (e) => {
    try { el.releasePointerCapture(e.pointerId); } catch { /* already released */ }
    const wasDown = down; down = null;
    if (wasDown && moved && f.frac.join() !== wasDown.frac.join()) recordMove(f, wasDown.frac.slice(), f.frac.slice());
    else if (wasDown && !moved && flagsFillable()) fillMarker(f);
  });
}

async function fillMarker(f) {
  if (f.kind !== 'marker') return;
  if (f.tagType === 'date') return placeIntoMarker(f, quickStampURL('date'));
  if (TEXT_MARKERS.has(f.tagType)) return fillTextMarker(f);
  const src = f.tagType === 'sign' ? markerSig : markerInit;
  if (src) return placeIntoMarker(f, src);
  // No remembered image yet — let the user pick one from the Library; the next
  // Library click resolves this target (resolveFillTarget) and is remembered.
  fillTarget = f;
  setActiveMarker(f);
  document.querySelector('.tab[data-panel="library"]')?.click();
  toast('Pick your ' + (f.tagType === 'sign' ? 'signature' : 'initials') + ' in the Library — it fills this marker and any others.');
}

function resolveFillTarget(src) {
  const f = fillTarget; fillTarget = null;
  if (!f) return;
  if (f.tagType === 'sign') markerSig = src; else markerInit = src;
  placeIntoMarker(f, src);
}

// Text flags (name/title/company) draw their value from the autofill profile —
// the same store autofill uses — so the value is typed once and reused. If the
// profile has no matching entry, point the user at the editor (the parallel to
// sending a sign/initial flag to the Library) and leave the flag for a retry.
let profileCache = null; // cached /api/profile; invalidated when the editor saves
async function loadProfile() {
  if (profileCache) return profileCache;
  const res = await apiFetch('/api/profile');
  profileCache = res.ok ? await res.json() : {};
  return profileCache;
}
function profileValue(profile, type) {
  const want = TEXT_MARKER_KEYS[type] || [type];
  for (const [k, v] of Object.entries(profile)) {
    if (want.includes(k.trim().toLowerCase()) && String(v).trim()) return String(v).trim();
  }
  return '';
}
async function fillTextMarker(f) {
  const v = profileValue(await loadProfile(), f.tagType);
  if (!v) {
    setActiveMarker(f);
    toast('Add your ' + MARKER_LABELS[f.tagType].toLowerCase() + ' to the autofill profile, then click this flag again.');
    els.editProfileBtn.click();
    return;
  }
  placeIntoMarker(f, textStampURL(v));
}

// placeIntoMarker swaps a marker for a real image stamp fitted inside the marker
// box (aspect preserved), then advances to the next marker in document order.
async function placeIntoMarker(f, src) {
  const pv = viewer.getPageView(f.page - 1);
  if (!pv?.div) return toast('Scroll the page into view, then try again');
  const base = (await pdfDocument.getPage(f.page)).getViewport({ scale: 1 });
  const next = nextMarkerAfter(f);
  await new Promise((resolve) => {
    const img = new Image();
    img.onload = () => {
      const aspect = (img.naturalWidth / img.naturalHeight) || 1;
      const W = pv.div.clientWidth, H = pv.div.clientHeight;
      const [x0, y0, x1, y1] = f.frac;
      let w = x1 - x0, h = w * W / (aspect * H); // fit width, derive height by aspect
      if (h > y1 - y0) { h = y1 - y0; w = h * aspect * H / W; } // too tall — fit height instead
      makeStamp(src, aspect, [x0, y0, x0 + w, y0 + h], { page: f.page, pageW: base.width, pageH: base.height }, pv);
      resolve();
    };
    img.onerror = () => { toast('could not load image'); resolve(); };
    img.src = src;
  });
  removeField(f);
  advanceTo(next);
}

// nextMarkerAfter returns the next unfilled marker after f in reading order
// (page, then top-to-bottom, then left-to-right), wrapping to the first if none.
function nextMarkerAfter(f) {
  const markers = overlayFields.filter((o) => o.kind === 'marker' && o !== f)
    .sort((a, b) => a.page - b.page || a.frac[1] - b.frac[1] || a.frac[0] - b.frac[0]);
  for (const m of markers) {
    if (m.page > f.page || (m.page === f.page && (m.frac[1] > f.frac[1] || (m.frac[1] === f.frac[1] && m.frac[0] >= f.frac[0])))) return m;
  }
  return markers[0] || null;
}

function advanceTo(m) {
  if (!m) { setActiveMarker(null); setSignBanner(); return toast('All fields filled'); }
  setActiveMarker(m);
  m.el.scrollIntoView({ block: 'center', behavior: 'smooth' });
  m.el.focus({ preventScroll: true });
  setSignBanner();
}

function setActiveMarker(m) {
  activeMarker?.el?.classList.remove('marker-active');
  activeMarker = m;
  m?.el?.classList.add('marker-active');
}

// --- signing flags (DocuSign-style place-then-email) -------------------------
// A preparer drops flags, saves for signing (the flags ride inside the one PDF
// as the NibFlags property — see internal/pdfops/flags.go), and emails it. The
// recipient's Nib rebuilds the flags on open and walks them flag-to-flag.

function markerFields() { return overlayFields.filter((f) => f.kind === 'marker'); }

function collectFlags() {
  return markerFields().map((f) => ({ page: f.page, frac: f.frac, type: f.tagType }));
}

// embedFlags posts the current document to /api/flags: a non-empty set embeds the
// placeholders; null strips them. Returns the new bytes.
async function embedFlags(bytes, flags) {
  const form = new FormData();
  form.append('pdf', new Blob([bytes], { type: 'application/pdf' }), 'doc.pdf');
  if (flags && flags.length) form.append('flags', JSON.stringify(flags));
  const res = await apiFetch('/api/flags', { method: 'POST', body: form });
  if (!res.ok) throw new Error('flags update failed');
  return new Uint8Array(await res.arrayBuffer());
}

// reconstructFlags rebuilds embedded placeholders as markers on open and shows the
// signing banner. Coordinates are clamped defensively — a hand-edited property
// can never place a flag off-page or on a page that doesn't exist.
function reconstructFlags(flags) {
  if (!Array.isArray(flags)) return;
  for (const fl of flags) {
    if (!fl || !MARKER_LABELS[fl.type] || !Array.isArray(fl.frac) || fl.frac.length !== 4) continue;
    const page = Math.max(1, Math.min(pdfDocument.numPages, fl.page | 0));
    const frac = fl.frac.map((n) => Math.max(0, Math.min(1, +n || 0)));
    buildMarker(fl.type, frac, page, false); // load on open — not an undoable edit
  }
  relayoutOverlays(); // attach flags on already-rendered pages; the rest follow on pagerendered
  showSignBanner();
}

function firstMarker() {
  return markerFields().sort((a, b) => a.page - b.page || a.frac[1] - b.frac[1] || a.frac[0] - b.frac[0])[0] || null;
}

function showSignBanner() {
  signTotal = markerFields().length;
  signStarted = false;
  if (!signTotal) { els.signBanner.hidden = true; return; }
  els.signBanner.hidden = false;
  setSignBanner();
}

// setSignBanner reflects progress: before the first fill it offers Start; after,
// it offers Next; once every flag is filled the primary becomes the complete-and-
// sign action. A secondary "Finish & sign" stays available the whole time any field
// is still unfilled, so the user is never stuck — e.g. when they place signatures
// from the Library (which doesn't consume a flag) or simply leave a field blank.
function setSignBanner() {
  if (els.signBanner.hidden) return;
  const remaining = markerFields().length;
  if (!remaining) {
    els.signMsg.textContent = signTotal ? 'All fields filled.' : 'No fields to fill.';
    els.signAction.textContent = 'Mark complete & sign';
    els.signAction.onclick = completeAndSign;
    els.signDone.hidden = true;
    return;
  }
  els.signMsg.textContent = signStarted
    ? `Field ${signTotal - remaining + 1} of ${signTotal}.`
    : `This document has ${signTotal} field${signTotal === 1 ? '' : 's'} to fill.`;
  els.signAction.textContent = signStarted ? 'Next field' : 'Start';
  els.signAction.onclick = signNext;
  els.signDone.hidden = false;
  els.signDone.onclick = completeAndSign;
}

function signNext() {
  signStarted = true;
  const from = (activeMarker && activeMarker.kind === 'marker') ? activeMarker : null;
  advanceTo(from ? nextMarkerAfter(from) : firstMarker());
}

// saveForSigning embeds the placed flags and offers the single emailable file.
async function saveForSigning() {
  const flags = collectFlags();
  if (!flags.length) return toast('Place at least one flag first (Sign / Date / Initial).');
  els.saveBtn.disabled = true;
  try {
    const baked = await bakedBytes();           // bake any non-flag edits; strips a prior flag set
    const out = await embedFlags(baked, flags); // embed the current flags
    openSaveAs(new Blob([out], { type: 'application/pdf' }), exportBase() + '-for-signing.pdf', 'Save for signing — email this exact file');
    toast('Saved for signing. Email this file as-is; printing or re-exporting it elsewhere drops the flags.');
  } catch (e) {
    toast('could not prepare the document: ' + e.message);
  } finally {
    els.saveBtn.disabled = !pdfDocument;
  }
}

// completeAndSign ends the recipient's signing flow in one step: it bakes the
// filled flags (bakedBytes strips the NibFlags property so the file won't reopen
// in signing mode), then applies a tamper-evident certification signature via the
// same /api/finalize path Finalize & sign uses. The baked-then-signed file is
// flat and frozen — any later edit breaks the signature.
async function completeAndSign() {
  const empty = markerFields().length;
  if (empty && !confirm(`${empty} field${empty === 1 ? '' : 's'} still empty — complete and sign anyway?`)) return;
  els.signAction.disabled = true; els.signDone.disabled = true;
  try {
    const form = await bakedForm(); // baked, flag-stripped bytes as the "pdf" part
    form.append('params', JSON.stringify({ reason: 'Signed in Nib', watermark: { text: '' }, tsaUrl: '' }));
    const res = await apiFetch('/api/finalize', { method: 'POST', body: form });
    if (!res.ok) { toast('Could not complete and sign'); return; }
    // Drop the "-for-signing" the preparer's save added, so the file lands as
    // <doc>.signed.pdf rather than <doc>-for-signing.signed.pdf.
    const base = exportBase().replace(/-for-signing$/i, '');
    openSaveAs(await res.blob(), base + '.signed.pdf', 'Save completed & signed PDF');
    els.signBanner.hidden = true;
  } catch (e) {
    toast('could not complete: ' + e.message);
  } finally {
    els.signAction.disabled = false; els.signDone.disabled = false;
  }
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

async function saveSettings(body) {
  try {
    const res = await apiFetch('/api/settings', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    });
    if (!res.ok) toast('Could not save settings');
  } catch { toast('Could not save settings'); }
}
els.autoUpdateChk.onchange = () => saveSettings({ checkUpdatesOnStartup: els.autoUpdateChk.checked });

// Appearance: dark (default) or light. Drives a data attribute on <html> (not
// body, where the layout attr lives) so the palette override reaches html's own
// background; the saved value is applied in applyStatus and persisted on toggle.
function applyAppearance(mode) {
  document.documentElement.dataset.appearance = mode;
  els.themeToggle.title = mode === 'light' ? 'Switch to dark theme' : 'Switch to light theme';
}
els.themeToggle.onclick = () => {
  const next = document.documentElement.dataset.appearance === 'light' ? 'dark' : 'light';
  applyAppearance(next);
  saveSettings({ appearance: next });
};

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
  if (activeTool) { exitBorder(); exitNote(); exitDropdown(); exitRadio(); exitShape(); } // Nib-side tools, not pdf.js modes — one at a time
  // Mirror the active mode onto every control bound to it (Edit menu + toolbar).
  // Scope out the compare tabs: they share the data-mode attribute (text/side/diff)
  // but are wired to setCompareMode, not the annotation tools.
  document.querySelectorAll('[data-mode]:not(.cmmode)').forEach((b) => b.classList.toggle('active', b.dataset.mode === activeTool));
  // The highlight color row is contextual — show it only while highlighting (or
  // drawing a border), and re-assert the selected color so the next highlight
  // uses it (not pdf.js yellow).
  reflectAnnoControls();
  if (activeTool === 'HIGHLIGHT') applyHighlightColor(selectedHlColor);
}
document.querySelectorAll('[data-mode]:not(.cmmode)').forEach((b) => {
  b.onclick = () => setTool(b.dataset.mode);
});

// --- highlight colors --------------------------------------------------------
// pdf.js defaults every highlight to pale yellow because Nib configures no
// palette. We drive the color through the same eventBus param the built-in color
// picker uses, and keep a small MRU of recent colors (persisted in the vault) so
// the last-used five are one click away. Nib renders the swatches itself — the
// viewer carries no editor-params toolbar DOM.
const DEFAULT_HL_COLORS = ['#fff066', '#93e0a3', '#8fb8ff', '#ffa6c9', '#ffb454'];
let recentHlColors = DEFAULT_HL_COLORS.slice();
// The color the user last chose — sticky per session/document. It only changes
// when the user picks a new color (setHighlightColor); tool re-activation and
// document reloads re-assert it, so a highlight color persists until changed.
let selectedHlColor = DEFAULT_HL_COLORS[0];

function applyHighlightColor(hex) {
  eventBus.dispatch('switchannotationeditorparams', {
    source: window,
    type: pdfjsLib.AnnotationEditorParamsType.HIGHLIGHT_COLOR,
    value: hex,
  });
}

function renderHlSwatches() {
  els.hlSwatches.replaceChildren();
  for (const c of recentHlColors) {
    const b = document.createElement('button');
    b.className = 'hlswatch';
    b.style.background = c;
    b.title = c;
    b.classList.toggle('active', c === selectedHlColor);
    b.onclick = () => setHighlightColor(c);
    els.hlSwatches.appendChild(b);
  }
  els.hlCustom.value = selectedHlColor;
}

// setHighlightColor is the explicit-pick path (a swatch or the custom picker): it
// makes hex the sticky selected color, applies it to new (and any selected)
// highlights, moves it to the front of the MRU, and persists (server validates +
// caps). Re-asserting the existing color elsewhere uses applyHighlightColor.
function setHighlightColor(hex) {
  hex = hex.toLowerCase();
  selectedHlColor = hex;
  applyHighlightColor(hex);
  recentHlColors = [hex, ...recentHlColors.filter((c) => c !== hex)].slice(0, 5);
  renderHlSwatches();
  saveSettings({ recentHighlightColors: recentHlColors });
}

els.hlCustom.onchange = () => setHighlightColor(els.hlCustom.value);

// --- border boxes ------------------------------------------------------------
// A "Border" is a colored outline with no fill — the unfilled sibling of a
// highlight. pdf.js owns highlights and has no border mode, so Nib draws this
// itself: drag a rectangle, get a draggable/resizable outline overlay (kind
// 'box'), and on save it bakes as a transparent stroked-rectangle PNG through the
// same /api/bake stamps path as the Y/N circle. Color comes from the shared
// highlight palette; thickness (points) is captured per box at draw time.
let borderMode = false;
let bdStart = null, bdDiv = null, bdHit = null;

// Show the highlight color row while highlighting OR drawing borders; the
// thickness control only while drawing borders.
function reflectAnnoControls() {
  els.hlColors.hidden = !(activeTool === 'HIGHLIGHT' || borderMode || shapeMode);
  els.borderWidth.hidden = !(borderMode || shapeMode);
  els.shapeOpts.hidden = !shapeMode;
}
function reflectBorder() {
  els.borderBtn.classList.toggle('active', borderMode);
  reflectAnnoControls();
}
function exitBorder() {
  if (!borderMode) return;
  borderMode = false;
  reflectBorder();
  els.viewerContainer.style.cursor = '';
}
const clampWeight = (v) => Math.min(10, Math.max(1, Number(v) || 2)); // points

els.borderBtn.onclick = () => {
  if (borderMode) { exitBorder(); return; }
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  borderMode = true;
  setTool(null); // clear any pdf.js editor tool
  setMarkerMode(null);
  if (redactMode) { redactMode = false; reflectRedact(); }
  if (editMode) { editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitNote(); exitDropdown(); exitRadio();
  reflectBorder();
  els.viewerContainer.style.cursor = 'crosshair';
};

els.viewerContainer.addEventListener('pointerdown', (e) => {
  if (!borderMode) return;
  bdHit = pageAt(e.clientX, e.clientY);
  if (!bdHit) return;
  bdStart = { x: e.clientX, y: e.clientY };
  bdDiv = document.createElement('div');
  bdDiv.className = 'bordermark';
  bdDiv.style.borderColor = selectedHlColor;
  bdHit.pv.div.appendChild(bdDiv);
  sizeMark(bdDiv, bdHit.r, bdStart, bdStart);
  e.preventDefault();
});
els.viewerContainer.addEventListener('pointermove', (e) => {
  if (bdStart) sizeMark(bdDiv, bdHit.r, bdStart, { x: e.clientX, y: e.clientY });
});
els.viewerContainer.addEventListener('pointerup', async (e) => {
  if (!bdStart) return;
  const hit = bdHit, start = bdStart;
  bdDiv.remove();
  bdStart = null; bdDiv = null; bdHit = null;
  const r = hit.r;
  const fw = Math.abs(e.clientX - start.x) / r.width;
  const fh = Math.abs(e.clientY - start.y) / r.height;
  if (fw < 0.01 || fh < 0.01) return; // ignore a stray click
  const fx0 = (Math.min(start.x, e.clientX) - r.left) / r.width;
  const fy0 = (Math.min(start.y, e.clientY) - r.top) / r.height;
  const base = (await pdfDocument.getPage(hit.n)).getViewport({ scale: 1 }); // PDF points
  makeBox([fx0, fy0, fx0 + fw, fy0 + fh], { page: hit.n, pageW: base.width, pageH: base.height });
});

// makeBox registers a draggable/resizable outline overlay (kind 'box'), styled
// live with the chosen color and thickness; collectStamps bakes it via boxPNG.
function makeBox(frac, opts) {
  const f = { page: opts.page, frac, pageW: opts.pageW, pageH: opts.pageH, kind: 'box',
    color: selectedHlColor, weight: clampWeight(els.borderWidthInput.value) };
  const el = document.createElement('div');
  el.className = 'ovl ovl-box';
  el.tabIndex = 0;
  el.style.borderColor = f.color;
  const handle = document.createElement('span');
  handle.className = 'stamp-resize';
  const del = document.createElement('button');
  del.className = 'stamp-del'; del.textContent = '×'; del.title = 'Remove border';
  el.append(handle, del);
  f.el = el;

  const remove = () => deleteField(f);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  el.addEventListener('keydown', (ev) => { if (ev.key === 'Delete' || ev.key === 'Backspace') remove(); });
  enableStampGestures(f, el, handle);

  overlayFields.push(f);
  const pv = viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f);
}

// --- Dropdown tool: place a fillable dropdown (combobox) field ----------------
// A box-draw tool like Border, but the overlay carries an options list; on "Save
// as fillable form…" it authors a real AcroForm combobox (see collectAuthorFields
// → /api/form/author → pdfops.AuthorForm). Options are typed inline on the field.
let dropdownMode = false;
let ddStart = null, ddDiv = null, ddHit = null;
function reflectDropdown() { els.dropdownBtn.classList.toggle('active', dropdownMode); }
function exitDropdown() {
  if (!dropdownMode) return;
  dropdownMode = false;
  reflectDropdown();
  els.viewerContainer.style.cursor = '';
}
els.dropdownBtn.onclick = () => {
  if (dropdownMode) { exitDropdown(); return; }
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  dropdownMode = true;
  setTool(null);
  setMarkerMode(null);
  if (redactMode) { redactMode = false; reflectRedact(); }
  if (editMode) { editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitNote(); exitRadio(); // clear the sibling one-at-a-time tools (not dropdown — we're entering it)
  exitBorder();
  exitShape();
  reflectDropdown();
  els.viewerContainer.style.cursor = 'crosshair';
};
els.viewerContainer.addEventListener('pointerdown', (e) => {
  if (!dropdownMode) return;
  ddHit = pageAt(e.clientX, e.clientY);
  if (!ddHit) return;
  ddStart = { x: e.clientX, y: e.clientY };
  ddDiv = document.createElement('div');
  ddDiv.className = 'bordermark';
  ddHit.pv.div.appendChild(ddDiv);
  sizeMark(ddDiv, ddHit.r, ddStart, ddStart);
  e.preventDefault();
});
els.viewerContainer.addEventListener('pointermove', (e) => {
  if (ddStart) sizeMark(ddDiv, ddHit.r, ddStart, { x: e.clientX, y: e.clientY });
});
els.viewerContainer.addEventListener('pointerup', async (e) => {
  if (!ddStart) return;
  const hit = ddHit, start = ddStart;
  ddDiv.remove();
  ddStart = null; ddDiv = null; ddHit = null;
  const r = hit.r;
  const fw = Math.abs(e.clientX - start.x) / r.width;
  const fh = Math.abs(e.clientY - start.y) / r.height;
  if (fw < 0.01 || fh < 0.01) return; // ignore a stray click
  const fx0 = (Math.min(start.x, e.clientX) - r.left) / r.width;
  const fy0 = (Math.min(start.y, e.clientY) - r.top) / r.height;
  const base = (await pdfDocument.getPage(hit.n)).getViewport({ scale: 1 });
  makeDropdown([fx0, fy0, fx0 + fw, fy0 + fh], { page: hit.n, pageW: base.width, pageH: base.height });
});

// makeDropdown registers a draggable/resizable dropdown overlay (kind 'dropdown')
// with an inline comma-separated options input; the options ride to the server in
// collectAuthorFields, where each becomes a combobox choice.
function makeDropdown(frac, opts) {
  const f = { page: opts.page, frac, pageW: opts.pageW, pageH: opts.pageH, kind: 'dropdown' };
  const el = document.createElement('div');
  el.className = 'ovl ovl-dropdown';
  el.tabIndex = 0;
  const input = document.createElement('input');
  input.className = 'dd-opts';
  input.placeholder = 'options: a, b, c';
  input.addEventListener('pointerdown', (ev) => ev.stopPropagation());
  const caret = document.createElement('span');
  caret.className = 'dd-caret'; caret.textContent = '▾';
  const handle = document.createElement('span');
  handle.className = 'stamp-resize';
  const del = document.createElement('button');
  del.className = 'stamp-del'; del.textContent = '×'; del.title = 'Remove dropdown';
  el.append(input, caret, handle, del);
  f.el = el; f.optsInput = input;

  const remove = () => deleteField(f);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  // Delete/Backspace removes the field only when the box (not the options input) is focused.
  el.addEventListener('keydown', (ev) => { if ((ev.key === 'Delete' || ev.key === 'Backspace') && ev.target === el) remove(); });
  enableStampGestures(f, el, handle);

  overlayFields.push(f);
  const pv = viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f);
  input.focus();
}

// Radio-group tool — a sibling of the Dropdown tool. Draw a box to anchor a radio
// group, type ≥2 comma-separated choices; "Save as fillable form…" authors a real
// AcroForm radiobuttongroup whose buttons march to the right of the anchor, each
// labelled with its value. (Horizontal only for now.)
let radioMode = false;
let rdStart = null, rdDiv = null, rdHit = null;
function reflectRadio() { els.radioBtn.classList.toggle('active', radioMode); }
function exitRadio() {
  if (!radioMode) return;
  radioMode = false;
  reflectRadio();
  els.viewerContainer.style.cursor = '';
}
els.radioBtn.onclick = () => {
  if (radioMode) { exitRadio(); return; }
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  radioMode = true;
  setTool(null);
  setMarkerMode(null);
  if (redactMode) { redactMode = false; reflectRedact(); }
  if (editMode) { editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitNote(); exitDropdown();
  exitBorder();
  exitShape();
  radioMode = true;
  reflectRadio();
  els.viewerContainer.style.cursor = 'crosshair';
};
els.viewerContainer.addEventListener('pointerdown', (e) => {
  if (!radioMode) return;
  rdHit = pageAt(e.clientX, e.clientY);
  if (!rdHit) return;
  rdStart = { x: e.clientX, y: e.clientY };
  rdDiv = document.createElement('div');
  rdDiv.className = 'bordermark';
  rdHit.pv.div.appendChild(rdDiv);
  sizeMark(rdDiv, rdHit.r, rdStart, rdStart);
  e.preventDefault();
});
els.viewerContainer.addEventListener('pointermove', (e) => {
  if (rdStart) sizeMark(rdDiv, rdHit.r, rdStart, { x: e.clientX, y: e.clientY });
});
els.viewerContainer.addEventListener('pointerup', async (e) => {
  if (!rdStart) return;
  const hit = rdHit, start = rdStart;
  rdDiv.remove();
  rdStart = null; rdDiv = null; rdHit = null;
  const r = hit.r;
  const fw = Math.abs(e.clientX - start.x) / r.width;
  const fh = Math.abs(e.clientY - start.y) / r.height;
  if (fw < 0.01 || fh < 0.01) return; // ignore a stray click
  const fx0 = (Math.min(start.x, e.clientX) - r.left) / r.width;
  const fy0 = (Math.min(start.y, e.clientY) - r.top) / r.height;
  const base = (await pdfDocument.getPage(hit.n)).getViewport({ scale: 1 });
  makeRadio([fx0, fy0, fx0 + fw, fy0 + fh], { page: hit.n, pageW: base.width, pageH: base.height });
});

// makeRadio registers a draggable/resizable radio-group overlay (kind 'radio')
// with an inline comma-separated choices input (≥2); the options ride to the
// server in collectAuthorFields, where each becomes a radio button value.
function makeRadio(frac, opts) {
  const f = { page: opts.page, frac, pageW: opts.pageW, pageH: opts.pageH, kind: 'radio' };
  const el = document.createElement('div');
  el.className = 'ovl ovl-radio';
  el.tabIndex = 0;
  const input = document.createElement('input');
  input.className = 'dd-opts';
  input.placeholder = 'choices: a, b, c';
  input.addEventListener('pointerdown', (ev) => ev.stopPropagation());
  const caret = document.createElement('span');
  caret.className = 'dd-caret'; caret.textContent = '◉';
  const handle = document.createElement('span');
  handle.className = 'stamp-resize';
  const del = document.createElement('button');
  del.className = 'stamp-del'; del.textContent = '×'; del.title = 'Remove radio group';
  el.append(input, caret, handle, del);
  f.el = el; f.optsInput = input;

  const remove = () => deleteField(f);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  // Delete/Backspace removes the field only when the box (not the choices input) is focused.
  el.addEventListener('keydown', (ev) => { if ((ev.key === 'Delete' || ev.key === 'Backspace') && ev.target === el) remove(); });
  enableStampGestures(f, el, handle);

  overlayFields.push(f);
  const pv = viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f);
  input.focus();
}

// boxPNG renders a transparent PNG with a stroked rectangle outline in hex at the
// given pen weight (PDF points). Drawing at the rect's own point size (× super-
// sample) means the server's scale-to-rect is uniform, so the weight stays true.
function boxPNG(wPts, hPts, hex, weightPts) {
  const s = 3;
  const cv = document.createElement('canvas');
  cv.width = Math.max(8, Math.round(wPts * s));
  cv.height = Math.max(8, Math.round(hPts * s));
  const c = cv.getContext('2d');
  c.strokeStyle = hex;
  c.lineWidth = Math.max(1, weightPts * s);
  const m = c.lineWidth / 2; // inset by half the stroke so the border stays in the rect
  c.strokeRect(m, m, cv.width - c.lineWidth, cv.height - c.lineWidth);
  return cv.toDataURL('image/png').split(',')[1];
}

// --- shapes: line / arrow / rectangle / ellipse ------------------------------
// Drawn marks baked as PNG stamps (the Border path), so they render in every
// viewer. One paintShape renderer drives the live drag preview, the persistent
// overlay (layoutField), and the baked PNG (shapeMarkPNG).
let shapeMode = false;
let shapeType = 'line';
let shStart = null, shCanvas = null, shHit = null;

function reflectShape() {
  els.shapeBtn.classList.toggle('active', shapeMode);
  reflectAnnoControls();
}
function exitShape() {
  if (!shapeMode) return;
  shapeMode = false;
  reflectShape();
  els.viewerContainer.style.cursor = '';
}
els.shapeBtn.onclick = () => {
  if (shapeMode) { exitShape(); return; }
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  shapeMode = true;
  setTool(null);
  setMarkerMode(null);
  if (redactMode) { redactMode = false; reflectRedact(); }
  if (editMode) { editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitNote(); exitDropdown(); exitRadio();
  exitBorder();
  reflectShape();
  els.viewerContainer.style.cursor = 'crosshair';
};
all('.shapetype').forEach((b) => {
  b.onclick = () => { shapeType = b.dataset.shape; all('.shapetype').forEach((x) => x.classList.toggle('active', x === b)); };
});

// shapePad: the margin a line/arrow's holding box needs beyond the segment so the
// arrowhead barbs and round stroke caps clear the box edge instead of being
// clipped. Mirrors paintShape's head size (max(8, weight*3.5)) plus the cap.
function shapePad(weight) { return Math.max(8, weight * 3.5) + weight; }

// shapeGeom: from a drag's two endpoints (any consistent unit) and a per-axis
// margin, return the holding box {x0,y0,w,h} grown by the margin on every side,
// plus the segment's from/to as fractions of that grown box (tail at the start
// corner). The endpoints sit inset, so the arrowhead has room — for diagonal
// arrows (corner overhang) and axis-aligned ones (no room across the stroke).
// With padX/padY 0 this is the plain drag bbox with 0|1 corners (rect/ellipse).
function shapeGeom(ax, ay, bx, by, padX, padY) {
  const x0 = Math.min(ax, bx) - padX, y0 = Math.min(ay, by) - padY;
  const w = Math.abs(bx - ax) + 2 * padX, h = Math.abs(by - ay) + 2 * padY;
  const fromX = w ? (ax <= bx ? padX / w : 1 - padX / w) : 0;
  const fromY = h ? (ay <= by ? padY / h : 1 - padY / h) : 0;
  return { x0, y0, w, h, from: { x: fromX, y: fromY }, to: { x: 1 - fromX, y: 1 - fromY } };
}

// paintShape draws the current shape into a 2D context sized w×h px, pen weight in
// px. fill (rect/ellipse) is a translucent wash; from/to (0..1) orient line/arrow.
function paintShape(c, w, h, type, fill, hex, weightPx, from, to) {
  c.clearRect(0, 0, w, h);
  c.strokeStyle = hex;
  c.fillStyle = hex;
  c.lineWidth = Math.max(1, weightPx);
  c.lineJoin = 'round';
  c.lineCap = 'round';
  const m = c.lineWidth / 2;
  if (type === 'rect') {
    if (fill) { c.globalAlpha = 0.25; c.fillRect(m, m, w - c.lineWidth, h - c.lineWidth); c.globalAlpha = 1; }
    c.strokeRect(m, m, w - c.lineWidth, h - c.lineWidth);
  } else if (type === 'ellipse') {
    c.beginPath();
    c.ellipse(w / 2, h / 2, Math.max(1, w / 2 - m), Math.max(1, h / 2 - m), 0, 0, 2 * Math.PI);
    if (fill) { c.globalAlpha = 0.25; c.fill(); c.globalAlpha = 1; }
    c.stroke();
  } else { // line / arrow
    const ax = from.x * w, ay = from.y * h, bx = to.x * w, by = to.y * h;
    c.beginPath(); c.moveTo(ax, ay); c.lineTo(bx, by); c.stroke();
    if (type === 'arrow') {
      const ang = Math.atan2(by - ay, bx - ax);
      const hd = Math.max(8, c.lineWidth * 3.5);
      c.beginPath();
      c.moveTo(bx, by); c.lineTo(bx - hd * Math.cos(ang - Math.PI / 7), by - hd * Math.sin(ang - Math.PI / 7));
      c.moveTo(bx, by); c.lineTo(bx - hd * Math.cos(ang + Math.PI / 7), by - hd * Math.sin(ang + Math.PI / 7));
      c.stroke();
    }
  }
}

// shapeMarkPNG renders a shape to a transparent PNG at the rect's point size (×
// supersample) for baking via StampImages — the same rail Border uses.
function shapeMarkPNG(wPts, hPts, f) {
  const s = 3;
  const cv = document.createElement('canvas');
  cv.width = Math.max(8, Math.round(wPts * s));
  cv.height = Math.max(8, Math.round(hPts * s));
  paintShape(cv.getContext('2d'), cv.width, cv.height, f.type, f.fill, f.color, f.weight * s, f.from, f.to);
  return cv.toDataURL('image/png').split(',')[1];
}

els.viewerContainer.addEventListener('pointerdown', (e) => {
  if (!shapeMode) return;
  shHit = pageAt(e.clientX, e.clientY);
  if (!shHit) return;
  shStart = { x: e.clientX, y: e.clientY };
  shCanvas = document.createElement('canvas');
  shCanvas.className = 'shapemark';
  shHit.pv.div.appendChild(shCanvas);
  e.preventDefault();
});
els.viewerContainer.addEventListener('pointermove', (e) => {
  if (!shStart) return;
  const r = shHit.r;
  const w = clampWeight(els.borderWidthInput.value);
  const isLine = shapeType === 'line' || shapeType === 'arrow';
  const pad = isLine ? shapePad(w) : 0; // px; head/cap room (none for rect/ellipse)
  const g = shapeGeom(shStart.x - r.left, shStart.y - r.top, e.clientX - r.left, e.clientY - r.top, pad, pad);
  shCanvas.style.left = g.x0 + 'px'; shCanvas.style.top = g.y0 + 'px';
  shCanvas.width = Math.max(1, Math.round(g.w)); shCanvas.height = Math.max(1, Math.round(g.h));
  paintShape(shCanvas.getContext('2d'), shCanvas.width, shCanvas.height, shapeType, els.shapeFill.checked, selectedHlColor, w, g.from, g.to);
});
els.viewerContainer.addEventListener('pointerup', async (e) => {
  if (!shStart) return;
  const hit = shHit, start = shStart;
  shCanvas.remove(); shStart = null; shCanvas = null; shHit = null;
  const r = hit.r;
  const isLine = shapeType === 'line' || shapeType === 'arrow';
  const fw = Math.abs(e.clientX - start.x) / r.width, fh = Math.abs(e.clientY - start.y) / r.height;
  if (isLine ? (fw < 0.005 && fh < 0.005) : (fw < 0.01 || fh < 0.01)) return; // ignore a stray click
  const base = (await pdfDocument.getPage(hit.n)).getViewport({ scale: 1 }); // PDF points
  // Line/arrow: grow the holding box by an arrowhead-sized margin (in points, per
  // axis) and inset the endpoints so the head clears the box edge; this also makes
  // an axis-aligned line's box non-degenerate. rect/ellipse keep a tight bbox.
  const pad = isLine ? shapePad(clampWeight(els.borderWidthInput.value)) : 0;
  const g = shapeGeom((start.x - r.left) / r.width, (start.y - r.top) / r.height,
                      (e.clientX - r.left) / r.width, (e.clientY - r.top) / r.height,
                      pad / base.width, pad / base.height);
  makeShape([g.x0, g.y0, g.x0 + g.w, g.y0 + g.h],
    { page: hit.n, pageW: base.width, pageH: base.height, type: shapeType, fill: els.shapeFill.checked, from: g.from, to: g.to });
});

// makeShape registers a draggable/resizable shape overlay (kind 'shape') whose
// canvas redraws via layoutField; collectStamps bakes it via shapeMarkPNG.
function makeShape(frac, opts) {
  const f = { page: opts.page, frac, pageW: opts.pageW, pageH: opts.pageH, kind: 'shape',
    type: opts.type, fill: opts.fill, from: opts.from, to: opts.to,
    color: selectedHlColor, weight: clampWeight(els.borderWidthInput.value) };
  const el = document.createElement('div');
  el.className = 'ovl ovl-shape';
  el.tabIndex = 0;
  const canvas = document.createElement('canvas');
  canvas.className = 'shapecanvas';
  const handle = document.createElement('span');
  handle.className = 'stamp-resize';
  const del = document.createElement('button');
  del.className = 'stamp-del'; del.textContent = '×'; del.title = 'Remove shape';
  el.append(canvas, handle, del);
  f.el = el; f.canvas = canvas;

  const remove = () => deleteField(f);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  el.addEventListener('keydown', (ev) => { if (ev.key === 'Delete' || ev.key === 'Backspace') remove(); });
  enableStampGestures(f, el, handle);

  overlayFields.push(f);
  const pv = viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f);
}

// --- comment notes -----------------------------------------------------------
// A note is a Nib overlay (a small text card) you place, drag, and edit; at save
// it bakes into a native /Text sticky-note annotation (a clickable icon whose
// popup shows the comment) via pdfops.AddNotes.
let noteMode = false;
function reflectNote() { els.noteBtn.classList.toggle('active', noteMode); }
function exitNote() {
  if (!noteMode) return;
  noteMode = false;
  reflectNote();
  els.viewerContainer.style.cursor = '';
}
els.noteBtn.onclick = () => {
  if (noteMode) { exitNote(); exitDropdown(); exitRadio(); return; }
  if (!pdfDocument) { toast('Open a PDF first'); return; }
  noteMode = true;
  setTool(null);
  setMarkerMode(null);
  if (redactMode) { redactMode = false; reflectRedact(); }
  if (editMode) { editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitBorder();
  exitShape();
  reflectNote();
  els.viewerContainer.style.cursor = 'crosshair';
};
els.viewerContainer.addEventListener('pointerdown', async (e) => {
  if (!noteMode) return;
  const hit = pageAt(e.clientX, e.clientY);
  if (!hit) return;
  e.preventDefault();
  const r = hit.r;
  const fx = (e.clientX - r.left) / r.width;
  const fy = (e.clientY - r.top) / r.height;
  const base = (await pdfDocument.getPage(hit.n)).getViewport({ scale: 1 }); // PDF points
  const fw = Math.min(0.3, 150 / r.width), fh = Math.min(0.2, 72 / r.height); // default card size
  makeNote([fx, fy, Math.min(fx + fw, 1), Math.min(fy + fh, 1)], { page: hit.n, pageW: base.width, pageH: base.height });
  exitNote(); exitDropdown(); exitRadio(); // place one; re-click the tool for another
});

// makeNote registers a draggable note card (kind 'note') with an inline comment
// textarea; collectNotes turns it into a /Text annotation at bake.
function makeNote(frac, opts) {
  const f = { page: opts.page, frac, pageW: opts.pageW, pageH: opts.pageH, kind: 'note', text: '' };
  const el = document.createElement('div');
  el.className = 'ovl ovl-note';
  el.tabIndex = 0;
  const head = document.createElement('div');
  head.className = 'note-head';
  const icon = document.createElement('span');
  icon.className = 'note-icon'; icon.textContent = '🗨';
  const del = document.createElement('button');
  del.className = 'stamp-del'; del.textContent = '×'; del.title = 'Remove note';
  head.append(icon, del);
  const ta = document.createElement('textarea');
  ta.className = 'note-text'; ta.placeholder = 'Comment…';
  ta.addEventListener('pointerdown', (ev) => ev.stopPropagation()); // edit the text, don't drag the card
  ta.addEventListener('input', () => { f.text = ta.value; });
  const handle = document.createElement('span');
  handle.className = 'stamp-resize';
  el.append(head, ta, handle);
  f.el = el;

  const remove = () => deleteField(f);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  enableStampGestures(f, el, handle);

  overlayFields.push(f);
  const pv = viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f);
  ta.focus();
}

// collectNotes turns each non-empty note overlay into a {page, x, y, text}: the
// icon anchors at the card's top-left corner, in PDF points.
function collectNotes() {
  const out = [];
  for (const f of overlayFields) {
    if (f.kind !== 'note' || !f.text.trim()) continue;
    const [x0, , , y1] = rectPoints(f, f.frac); // x0 = left, y1 = top edge (PDF points)
    out.push({ page: f.page, x: x0, y: y1, text: f.text.trim() });
  }
  return out;
}

// The toolbar mirrors the menus. Mode tools wire themselves via [data-mode]
// above; every other toolbar control forwards to its menu twin by id.
all('#toolbar [data-forward]').forEach((b) => { b.onclick = () => $(b.dataset.forward).click(); });

// Controls that act on the open document. Disabled (in both the menu and the
// toolbar twin) until one loads, so they read as "unavailable" rather than
// silently doing nothing. Doc-free actions — verify a timestamp, receive a
// co-signature, identity/peers/keys, the certificate export, About — stay live.
// Overlay state, declared before setDocControls(false) runs below: that call now
// reaches reflectSignControls() -> markerFields(), which reads overlayFields, so the
// binding must already be initialized (a `let` declared later would throw on its TDZ).
let overlayFields = []; // {page, frac:[fx0,fy0,fx1,fy1], pageW, pageH, kind, el}
let overlayHistory = { undo: [], redo: [] }; // client overlay-edit undo, drained before the server ring
let libraryImages = []; // cached /api/images list (the image-library panel)
const DOC_REQUIRED = [
  'saveFlatBtn', 'saveEditableBtn', 'saveFillableBtn', 'printBtn',
  'exportZipBtn', 'exportPngBtn', 'exportFormJsonBtn', 'exportFormCsvBtn', 'exportFormXfdfBtn', 'exportTableXlsxBtn', 'exportTableCsvBtn', 'exportTableOdsBtn', 'exportBookmarkSplitBtn',
  'exportPageSplitBtn', 'pdfaBtn',
  'textToolBtn', 'highlightToolBtn', 'drawToolBtn',
  'detectBtn', 'editTextBtn', 'removeOriginalsBtn', 'ocrBtn', 'ocrLang', 'ocrQuality', 'autofillBtn', 'splitBtn',
  'splitBoxBtn', 'applyBoxSplitBtn', 'rotateLeftBtn', 'rotateRightBtn',
  'extractBtn', 'insertBlankBtn', 'duplicatePageBtn', 'insertPdfBtn', 'pageNumBtn', 'pageLabelsBtn', 'nupBtn', 'normalizeBtn', 'cropBtn',
  'redactBtn', 'redactTextBtn', 'applyRedactBtn', 'scanBtn', 'attachBtn', 'encryptBtn', 'decryptBtn', 'compareBtn', 'fillCsvBtn', 'importXfdfBtn',
  'finalizeBtn', 'timestampBtn', 'cosignBtn', 'sessionInitBtn', 'sessionSendBtn',
];
function setDocControls(enabled) {
  for (const id of DOC_REQUIRED) {
    const b = $(id); if (b) b.disabled = !enabled;
    all(`#toolbar [data-forward="${id}"]`).forEach((t) => { t.disabled = !enabled; });
  }
  // The toolbar's Text/Highlight/Draw twins wire by data-mode, not data-forward.
  all('#toolbar [data-mode]').forEach((t) => { t.disabled = !enabled; });
  reflectSignControls(); // keep the Flags-panel controls in step with open/closed
  reflectUndoControls(enabled);
}
setDocControls(false); // nothing open yet

// Undo/Redo enable from the server's per-document history flags (docMeta.canUndo/
// canRedo, refreshed on every load); both off when no document is open.
function reflectUndoControls(enabled) {
  const m = docMeta || {};
  if (els.undoBtn) els.undoBtn.disabled = !(enabled && (m.canUndo || overlayHistory.undo.length));
  if (els.redoBtn) els.redoBtn.disabled = !(enabled && (m.canRedo || overlayHistory.redo.length));
}

// --- client overlay-edit undo (P2) ------------------------------------------
// Placing, deleting, or moving/resizing an overlay (stamp, cover-edit, border,
// shape, note, marker) is client-only — no server op — so the server undo ring
// never sees it. This small command stack records those edits and is drained by
// Ctrl+Z *before* falling through to the server undo. Because every server op
// reloads through setDocumentFromServer -> clearOverlays (which clears this
// stack too), it only ever holds the newest run of un-baked edits, so "client
// edits first, then server ops" is the correct chronological order for free.
function recordOverlayEdit(cmd) {
  overlayHistory.undo.push(cmd);
  overlayHistory.redo = [];
  reflectUndoControls(!!pdfDocument);
}
function clearOverlayHistory() { overlayHistory.undo = []; overlayHistory.redo = []; }
// detachField/reattachField toggle a field's presence without rebuilding it — the
// DOM element survives in the command closure, so add/delete undo is just a
// re-attach or detach. layoutFieldNow repositions a still-attached field (moves).
function detachField(f) {
  f.el.remove();
  overlayFields = overlayFields.filter((o) => o !== f);
}
function reattachField(f) {
  overlayFields.push(f);
  const pv = viewer.getPageView(f.page - 1);
  if (pv?.div) { pv.div.appendChild(f.el); layoutField(f, pv); }
}
function layoutFieldNow(f) {
  const pv = viewer.getPageView(f.page - 1);
  if (pv?.div) layoutField(f, pv);
}
// recordAdd / deleteField are the recorded create/remove used by the overlay
// factories and the per-overlay × buttons. recordMove records one command per
// move/resize gesture. detachField alone (no record) is the bulk-wipe path.
function recordAdd(f) {
  recordOverlayEdit({ undo: () => detachField(f), redo: () => reattachField(f) });
}
function deleteField(f, record = true) {
  if (record) recordOverlayEdit({ undo: () => reattachField(f), redo: () => detachField(f) });
  detachField(f);
}
function recordMove(f, before, after) {
  recordOverlayEdit({
    undo: () => { f.frac = before.slice(); layoutFieldNow(f); },
    redo: () => { f.frac = after.slice(); layoutFieldNow(f); },
  });
}
function undoOverlayEdit() {
  const c = overlayHistory.undo.pop();
  c.undo(); overlayHistory.redo.push(c);
  reflectUndoControls(!!pdfDocument);
}
function redoOverlayEdit() {
  const c = overlayHistory.redo.pop();
  c.redo(); overlayHistory.undo.push(c);
  reflectUndoControls(!!pdfDocument);
}
// undoAny/redoAny are the single dispatch for both Ctrl+Z and the ↶/↷ buttons:
// drain the client overlay stack first, then fall through to the server ring.
function undoAny() { if (overlayHistory.undo.length) undoOverlayEdit(); else doUndo(); }
function redoAny() { if (overlayHistory.redo.length) redoOverlayEdit(); else doRedo(); }

// doUndo/doRedo revert or re-apply the last server-side document operation (page
// ops, outline, sanitize, attachments). The server returns fresh doc metadata and
// the view reloads through the universal setDocumentFromServer path.
async function doUndo() {
  if (!pdfDocument || !(docMeta && docMeta.canUndo)) return;
  const res = await apiFetch('/api/undo', { method: 'POST' });
  if (!res.ok) { toast('undo failed'); return; }
  await setDocumentFromServer(await res.json());
}
async function doRedo() {
  if (!pdfDocument || !(docMeta && docMeta.canRedo)) return;
  const res = await apiFetch('/api/redo', { method: 'POST' });
  if (!res.ok) { toast('redo failed'); return; }
  await setDocumentFromServer(await res.json());
}
if (els.undoBtn) els.undoBtn.onclick = undoAny;
if (els.redoBtn) els.redoBtn.onclick = redoAny;

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
function clearOverlays() {
  overlayFields.forEach((f) => f.el.remove());
  overlayFields = [];
  activeMarker = null; fillTarget = null; // markers are gone with the old document
  exitSplitBox(); // a pending region selection doesn't carry to a new document
  exitBorder();   // nor a pending border-draw mode
  exitShape();    // nor a pending shape-draw mode
  exitCrop();     // nor a pending crop-draw mode
  exitNote(); exitDropdown(); exitRadio();     // nor a pending note-placement mode
  redactMarks = []; // pending redaction boxes don't carry to a new/reloaded document
  clearOverlayHistory(); // a new/reloaded document resets the overlay-edit undo stack
}
// clearDetected drops only auto-detected fields (text/check/circleone), keeping
// user-placed stamps, text edits, and sign/date/initial markers so re-running
// Detect doesn't wipe a signature/quick-stamp, a cover-and-replace edit, or a
// placed marker.
function clearDetected() {
  overlayFields = overlayFields.filter((f) => {
    if (f.kind === 'stamp' || f.kind === 'edit' || f.kind === 'marker' || f.kind === 'box' || f.kind === 'note' || f.kind === 'shape' || f.kind === 'dropdown') return true;
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
  // Border boxes carry a point thickness; scale it to the rendered page so the
  // live outline matches the baked stroke (css px per point = rendered W / page W).
  else if (f.kind === 'box') f.el.style.borderWidth = Math.max(1, f.weight * W / f.pageW) + 'px';
  // Shapes redraw their canvas to the overlay's current px size, with the pen
  // weight scaled from points to the rendered page (so it matches the bake).
  else if (f.kind === 'shape') {
    f.canvas.width = Math.max(1, Math.round((f.frac[2] - f.frac[0]) * W));
    f.canvas.height = Math.max(1, Math.round(h));
    paintShape(f.canvas.getContext('2d'), f.canvas.width, f.canvas.height, f.type, f.fill, f.color, Math.max(1, f.weight * W / f.pageW), f.from, f.to);
  }
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

// collectAuthorFields gathers detected/placed fields as interactive AcroForm
// widget specs — text fields and checkboxes, auto-named field_N. Unlike
// collectFields/collectStamps (which flatten values into the page), these become
// live, fillable form fields; the field is emitted blank regardless of any typed
// value, since the output is a distributable template.
// collectAuthorFields gathers the detected/placed fields authorable as interactive
// AcroForm widgets (text fields, checkboxes), in page order, WITHOUT names — the
// user names them in the fillable-form modal. `el` is the live overlay (client
// only, for the row-focus highlight); it is dropped from the server payload. Any
// typed value is ignored: authored fields are blank (a distributable template).
function collectAuthorFields() {
  const out = [];
  for (const f of overlayFields) {
    if (f.kind === 'text' || f.kind === 'check') {
      out.push({ page: f.page, rect: rectPoints(f, f.frac), kind: f.kind, el: f.el });
    } else if (f.kind === 'dropdown' || f.kind === 'radio') {
      const options = f.optsInput.value.split(',').map((s) => s.trim()).filter(Boolean);
      out.push({ page: f.page, rect: rectPoints(f, f.frac), kind: f.kind, options, el: f.el });
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
    } else if (f.kind === 'box') {
      const rect = rectPoints(f, f.frac);
      out.push({ page: f.page, rect, png: boxPNG(rect[2] - rect[0], rect[3] - rect[1], f.color, f.weight) });
    } else if (f.kind === 'shape') {
      const rect = rectPoints(f, f.frac);
      out.push({ page: f.page, rect, png: shapeMarkPNG(rect[2] - rect[0], rect[3] - rect[1], f) });
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
  const notes = collectNotes();
  let out = saved;
  if (fields.length || stamps.length || covers.length || notes.length) {
    const form = new FormData();
    form.append('pdf', new Blob([saved], { type: 'application/pdf' }), 'doc.pdf');
    if (covers.length) form.append('covers', JSON.stringify(covers));
    if (fields.length) form.append('fields', JSON.stringify(fields));
    if (stamps.length) form.append('stamps', JSON.stringify(stamps));
    if (notes.length) form.append('notes', JSON.stringify(notes));
    const res = await apiFetch('/api/bake', { method: 'POST', body: form });
    // A failed bake must abort the whole operation: returning the un-baked bytes
    // here would let save/print/flatten/sign proceed with a document silently
    // missing the user's covers, fields, stamps, and notes.
    if (!res.ok) throw new Error(await errText(res, 'could not apply edits'));
    out = new Uint8Array(await res.arrayBuffer());
  }
  // A document opened with embedded signing flags carries the NibFlags property,
  // which the bake preserves — strip it from any baked output so the finished
  // file doesn't reopen in signing mode. (Server no-op once it's already gone.)
  if (docHadFlags) {
    try { out = await embedFlags(out, null); } catch (e) {
      console.error('flag strip failed', e);
      toast('warning: could not remove the signing flags — this file may reopen in signing mode');
    }
  }
  return out;
}

// bakedForm builds the multipart body every endpoint that takes the current
// document shares: the baked bytes as the "pdf" file part. Callers append their
// own extra fields (params, ots, appearance, address, op) before sending.
async function bakedForm() {
  const form = new FormData();
  form.append('pdf', new Blob([await bakedBytes()], { type: 'application/pdf' }), 'doc.pdf');
  return form;
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
  const choiceGroups = dedupeGroups([...ynItems, ...findCircleOne(textItems), ...findSlashTemplates(textItems), ...findPipeChoices(textItems), ...findRunChoices(textItems, cells)]);
  for (const grp of choiceGroups) {
    const choices = snapChoices(canvas, grp.choices, grp.marker);
    const cf = choices.map((c) => ({ rect: [c.x0 / W, c.y0 / H, c.x1 / W, c.y1 / H], word: !!c.word }));
    const x0 = Math.min(...cf.map((c) => c.rect[0])), y0 = Math.min(...cf.map((c) => c.rect[1]));
    const x1 = Math.max(...cf.map((c) => c.rect[2])), y1 = Math.max(...cf.map((c) => c.rect[3]));
    makeField('circleone', [x0, y0, x1, y1], { page: n, pageW, pageH, choices: cf }, pv);
    added++;
  }

  toast(added ? `Added ${added} fillable field(s) — fill, then Save` : 'Nothing detected on this page');
};


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

// Page navigation + zoom: the toolbar buttons and the keyboard shortcuts share
// these, so the bounds logic lives in one place.
function prevPage() { if (pdfDocument && viewer.currentPageNumber > 1) viewer.currentPageNumber--; }
function nextPage() { if (pdfDocument && viewer.currentPageNumber < pdfDocument.numPages) viewer.currentPageNumber++; }
function firstPage() { if (pdfDocument) viewer.currentPageNumber = 1; }
function lastPage() { if (pdfDocument) viewer.currentPageNumber = pdfDocument.numPages; }
function zoomIn() { viewer.currentScale = viewer.currentScale * 1.15; }
function zoomOut() { viewer.currentScale = viewer.currentScale / 1.15; }
function fitWidth() { fitWidestWidth(); }
// fitWidestWidth fits the WIDEST page in the document to the container width and
// locks it as a NUMERIC scale, so a mixed-size document scrolls smoothly. A named
// 'page-width' re-fits to whichever page scrolls into view (pdf.js recomputes it
// against the current page), which on mixed sizes fought the scroll position and
// trapped you at the size boundary.
//
// Widths come from each page's RENDERED viewport (getPageView(i).viewport), which
// already accounts for /Rotate — so a rotated landscape page reports its true 792pt
// display width, not its 612pt portrait MediaBox. This must run at 'pagesloaded',
// not 'pagesinit': at pagesinit only page 1's view is populated (others still hold
// page 1's portrait viewport). By pagesloaded (sub-5000-page docs) every page view
// is set and the layout is settled; a page whose view isn't ready yet is skipped.
//
// Scale math mirrors pdf.js's own page-width formula (pdf_viewer.mjs #setScale /
// scrollIntoView): pdf.js renders a page at currentScale × PDF_TO_CSS_UNITS (the
// 72pt→96px conversion), so the scale that fits maxW points into `avail` CSS pixels
// is avail / maxW / PDF_TO_CSS_UNITS. Dropping that divisor zooms every page 4/3 too
// wide — the widest page then overflows.
function fitWidestWidth() {
  if (!pdfDocument) return;
  let maxW = 0;
  for (let i = 0; i < pdfDocument.numPages; i++) {
    const vp = viewer.getPageView(i)?.viewport;
    if (vp) {
      const w = vp.width / vp.scale; // rendered display width in points (rotation applied)
      if (w > maxW) maxW = w;
    }
  }
  const avail = els.viewerContainer.clientWidth - 40; // 40 = pdf.js SCROLLBAR_PADDING
  if (maxW > 0 && avail > 0) {
    viewer.currentScale = avail / maxW / pdfjsLib.PixelsPerInch.PDF_TO_CSS_UNITS;
  }
}
els.prevBtn.onclick = prevPage;
els.nextBtn.onclick = nextPage;
all('.pageNum').forEach((input) => input.addEventListener('change', () => {
  const n = Number(input.value);
  if (pdfDocument && n >= 1 && n <= pdfDocument.numPages) viewer.currentPageNumber = n;
}));
els.zoomInBtn.onclick = zoomIn;
els.zoomOutBtn.onclick = zoomOut;
els.fitBtn.onclick = fitWidth;

// Ctrl+scroll (and trackpad pinch, which Chromium/Firefox deliver as a ctrlKey
// wheel) zooms the DOCUMENT, not the browser. Left to the browser, ctrl+wheel
// scales the whole UI — menus crowd out the page — and pdf.js never re-renders
// for the new devicePixelRatio, so the document also goes permanently blurry.
// Routed into pdf.js's own updateScale instead: cursor-anchored, and drawingDelay
// CSS-scales during the gesture with one sharp redraw after the last tick (the
// official viewer's wheel behavior). Must be window-level and non-passive:
// hovering the toolbar shouldn't fall back to browser zoom, and preventDefault
// is what suppresses it. Keyboard Ctrl+=/Ctrl+0 stays untouched as the escape
// hatch for deliberately scaling the whole UI.
// A trackpad pinch also arrives as a ctrlKey wheel event, but with no physical
// Ctrl press — that's how it's told apart from a real Ctrl+scroll (the official
// viewer's trick). A pinch gets a continuous factor per event (its deltas are
// small and rapid); a mouse notch gets one discrete zoom step, whatever its
// deltaMode (Chromium reports notches as ±100 pixel-mode deltas).
let ctrlHeld = false;
window.addEventListener('keydown', (e) => { if (e.key === 'Control') ctrlHeld = true; });
window.addEventListener('keyup', (e) => { if (e.key === 'Control') ctrlHeld = false; });
window.addEventListener('blur', () => { ctrlHeld = false; });
let wheelTicks = 0; // fractional notches accumulated until a full step
window.addEventListener('wheel', (e) => {
  if (!e.ctrlKey && !e.metaKey) return; // plain scroll: not ours
  e.preventDefault();
  if (!pdfDocument || !e.deltaY) return;
  const origin = [e.clientX, e.clientY];
  const pixelMode = e.deltaMode === WheelEvent.DOM_DELTA_PIXEL;
  if (pixelMode && !ctrlHeld) {
    // Trackpad pinch: continuous factor per event.
    viewer.updateScale({ scaleFactor: 2 ** (-e.deltaY / 100), origin, drawingDelay: 400 });
  } else {
    // Notched wheel: one zoom step per notch, same 1.15 factor as the buttons.
    wheelTicks += -e.deltaY / (pixelMode ? 100 : e.deltaMode === WheelEvent.DOM_DELTA_LINE ? 3 : 1);
    const steps = Math.trunc(wheelTicks);
    if (steps) {
      wheelTicks -= steps;
      viewer.updateScale({ scaleFactor: 1.15 ** steps, origin, drawingDelay: 400 });
    }
  }
}, { passive: false });

// Re-render when devicePixelRatio changes (browser zoom via keyboard, OS display
// scaling, dragging to a different-DPI monitor): pdf.js only reads dpr at render
// time, so without this every canvas stays at the old resolution and is CSS-
// stretched — permanently soft text. All of those changes also resize the CSS
// viewport, so a resize listener comparing dpr is the reliable trigger; the
// resolution media query is a second net for any dpr change that somehow leaves
// the viewport size alone (it matches only the current dpr, so it re-arms with
// the new value after each change).
let lastDpr = devicePixelRatio;
function dprChanged() {
  if (devicePixelRatio === lastDpr) return;
  lastDpr = devicePixelRatio;
  if (pdfDocument) viewer.refresh();
}
window.addEventListener('resize', dprChanged);
function watchDpr() {
  matchMedia(`(resolution: ${devicePixelRatio}dppx)`).addEventListener('change', () => {
    dprChanged();
    watchDpr();
  }, { once: true });
}
watchDpr();

// In-document search: typing runs a fresh highlight-all find (debounced); Enter and
// the ‹ › buttons step through the matches via pdf.js's `type:'again'` — no re-scan,
// with query+flags held identical so the controller steps instead of falling back to
// a fresh search.
let searchTimer;
const findInput = () => document.querySelector('.searchInput');
function runFind(again = false, previous = false) {
  const input = findInput();
  if (!input) return;
  eventBus.dispatch('find', {
    type: again ? 'again' : '', query: input.value,
    caseSensitive: false, highlightAll: true, findPrevious: previous,
  });
}
function stepFind(previous) {
  clearTimeout(searchTimer); // drop a pending fresh search that would reset us to match 1
  if (findInput()?.value) runFind(true, previous);
}
all('.searchInput').forEach((input) => {
  input.addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => runFind(), 200);
  });
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); stepFind(e.shiftKey); }
  });
});
els.findPrevBtn.onclick = () => stepFind(true);
els.findNextBtn.onclick = () => stepFind(false);

// "N of M" readout + prev/next enablement. updatefindcontrolstate is the ONLY event
// the controller fires on an 'again' step (it carries the updated current match), so
// it's the source of truth here; updatefindmatchescount only keeps the total ticking
// up while the initial search scans a large document.
function renderFindCount(matchesCount) {
  const { current = 0, total = 0 } = matchesCount || {};
  els.findCount.textContent = !findInput()?.value ? '' : total ? `${current}/${total}` : '0/0';
  els.findPrevBtn.disabled = els.findNextBtn.disabled = total === 0;
}
eventBus.on('updatefindcontrolstate', ({ matchesCount }) => renderFindCount(matchesCount));
eventBus.on('updatefindmatchescount', ({ matchesCount }) => renderFindCount(matchesCount));

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

// --- mode tabs (View / Edit / Sign / Secure / Collaborate) -------------------
// The single navigation model: each tab is a workspace. Selecting one swaps the
// contextual toolbar (#toolbar .tbtab) and which sidebar panels are available.
const SIDEBAR_FOR = {
  file: ['thumbs', 'outline'],
  edit: ['thumbs'],
  sign: ['library'],
  secure: ['thumbs'],
  collaborate: ['flags'],
};
function syncSidebarForMode(tab) {
  const panels = SIDEBAR_FOR[tab] || [];
  all('.tabs .tab').forEach((t) => { t.hidden = !panels.includes(t.dataset.panel); });
  // If the showing panel isn't valid for this mode, switch to the mode's first.
  const active = document.querySelector('.tabs .tab.active');
  if ((!active || active.hidden) && panels.length) {
    document.querySelector(`.tabs .tab[data-panel="${panels[0]}"]`)?.click();
  }
}
function setMode(tab) {
  document.body.dataset.tab = tab;
  all('.modetab').forEach((b) => b.classList.toggle('active', b.dataset.tab === tab));
  all('#toolbar .tbtab').forEach((g) => g.classList.toggle('active', g.dataset.tab === tab));
  syncSidebarForMode(tab);
}
all('.modetab').forEach((b) => { b.onclick = () => setMode(b.dataset.tab); });

// Collaborate sub-mode: originate (own the document — prepare it, send it, await
// its return) vs receive (sign what arrives and send it back). Swaps the tools.
function setRole(role) {
  document.body.dataset.role = role;
  all('.roleopt').forEach((b) => b.classList.toggle('active', b.dataset.role === role));
  all('.roletools').forEach((g) => g.classList.toggle('active', g.dataset.role === role));
}
all('.roleopt').forEach((b) => { b.onclick = () => setRole(b.dataset.role); });

setRole('originate');
setMode('file');

// sidebar collapse
function toggleSidebar() {
  const hidden = $('sidebar').classList.toggle('collapsed');
  // Icon-only button — reflect the state in the tooltip.
  $('toggleSidebarBtn').title = hidden ? 'Show sidebar' : 'Hide sidebar';
}
$('toggleSidebarBtn').onclick = toggleSidebar;

// True when focus is in a text-entry surface, so plain-key shortcuts don't hijack
// typing. isContentEditable (not the attribute) is load-bearing: pdf.js's FreeText
// editor is a contenteditable <div>; AcroForm and overlay fields are real inputs.
function isTypingTarget(el) {
  if (!el) return false;
  if (el.isContentEditable) return true;
  return /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName);
}

// keyboard shortcuts. Ctrl/Cmd combos: S save, B sidebar, O open, F find,
// +/-/0 zoom in/out/fit. Plain keys (PageUp/Down, Home/End) page-navigate — but
// only when not typing and no modal is up. Every dialog is a `*Modal` div hidden
// via [hidden]; keep that naming so the guard below keeps catching them.
window.addEventListener('keydown', (e) => {
  if (e.ctrlKey || e.metaKey) {
    if (e.key === 's') { e.preventDefault(); save(); }
    else if (e.key === 'b') { e.preventDefault(); toggleSidebar(); }
    else if (e.key === 'o') { e.preventDefault(); openOpenDialog(); }
    else if (e.key === 'f') {
      e.preventDefault();
      // The find box lives in the File tab's toolbar — switch there, then focus it.
      setMode('file');
      document.querySelector('.searchInput')?.focus();
    } else if (e.key === '=' || e.key === '+') { e.preventDefault(); zoomIn(); }
    else if (e.key === '-') { e.preventDefault(); zoomOut(); }
    else if (e.key === '0') { e.preventDefault(); fitWidth(); }
    else if (e.key === 'z' || e.key === 'Z' || e.key === 'y') {
      // Undo/redo. Yield (no preventDefault) while typing, while a pdf.js
      // annotation editor is active (its own Ctrl+Z handles FreeText/Ink/
      // Highlight), or with a modal open. Otherwise drain the client overlay-edit
      // stack first, then fall through to the server document-op undo.
      if (isTypingTarget(e.target) || activeTool ||
          document.querySelector('div[id$="Modal"]:not([hidden])')) return;
      e.preventDefault();
      if (e.key === 'y' || e.shiftKey) redoAny(); else undoAny();
    }
    return;
  }
  if (!pdfDocument || isTypingTarget(e.target)) return;
  if (document.querySelector('div[id$="Modal"]:not([hidden])')) return;
  if (e.key === 'PageDown') { e.preventDefault(); nextPage(); }
  else if (e.key === 'PageUp') { e.preventDefault(); prevPage(); }
  else if (e.key === 'Home') { e.preventDefault(); firstPage(); }
  else if (e.key === 'End') { e.preventDefault(); lastPage(); }
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

// Backstop for async handlers without their own try/catch: an operation that
// throws (a failed bake, a mid-flight abort) surfaces as a toast instead of a
// silent unhandled rejection the user never sees.
window.addEventListener('unhandledrejection', (ev) => {
  console.error('unhandled rejection', ev.reason);
  toast(ev.reason?.message || 'operation failed');
});

// --- launch: check unlock state, then show the app or the first-run wizard ----
refreshStatus();
