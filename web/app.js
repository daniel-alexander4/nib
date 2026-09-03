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
  pixelsOf,
  dedupeGroups,
  buildTextRows,
} from './detect.js';
import { diffWords } from './vendor/diff/diff.min.mjs';
import pixelmatch from './vendor/pixelmatch/pixelmatch.mjs';

pdfjsLib.GlobalWorkerOptions.workerSrc = './vendor/pdfjs/pdf.worker.min.mjs';

// Every getDocument() in this file must carry these, so they live in one object that
// each call spreads rather than in five literals that can drift apart.
//
// `cMapUrl` is not decoration. pdf.js compiles in the NAMES of the ~150 predefined Adobe
// CMaps and nothing else: the tables live in separate `.bcmap` files it fetches at read
// time. Without them a document that says `/Encoding /UniJIS-UCS2-H` — which is what a
// Japanese, Chinese or Korean office suite writes when it names a standard Adobe face —
// has no route from its bytes to characters, and pdf.js yields an EMPTY text layer:
// nothing to select, nothing to copy, nothing to search. Measured that way before the
// tables were vendored (test/ui/cmap.test.mjs).
//
// `cMapPacked` says the tables are the binary `.bcmap` form, which is what ships.
//
// Deliberately NOT set here, each for a stated reason rather than by omission:
//   * `standardFontDataUrl` — pdf.js defaults `useSystemFonts: true` and substitutes a
//     system face for the standard 14, fetching font data only for Symbol and
//     ZapfDingbats. Vendoring 800 KB for that pair with no case that proves it would be
//     size behind an untested claim.
//   * `wasmUrl` / `iccUrl` — JBIG2, JPEG-2000 and ICC colour. pdf.js ships JS fallbacks
//     for the first two (`*_nowasm_fallback.js`), so these degrade rather than fail, and
//     each needs its own fixture to tell degraded from broken.
const PDFJS_OPTS = { cMapUrl: './vendor/pdfjs/cmaps/', cMapPacked: true };

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
  viewerWrap: $('viewerWrap'), empty: $('empty'), tabstrip: $('tabstrip'), closeAllBtn: $('closeAllBtn'),
  thumbs: $('thumbs'), outline: $('outline'),
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
  verifyModal: $('verifyModal'), verifyWords: $('verifyWords'),
  verifyConfirm: $('verifyConfirm'), verifyCancel: $('verifyCancel'),
  peerSelfFp: $('peerSelfFp'), peerSelfName: $('peerSelfName'),
  peerPaste: $('peerPaste'), peerLabel: $('peerLabel'),
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
  srvInvite: $('srvInvite'), srvInviteNote: $('srvInviteNote'),
  srvSelfFp: $('srvSelfFp'), srvSelfName: $('srvSelfName'), srvSelfCopy: $('srvSelfCopy'),
  srvCancel: $('srvCancel'), srvArmGo: $('srvArmGo'),
  srvWaitAddr: $('srvWaitAddr'), srvWaitPeer: $('srvWaitPeer'), srvDisarm: $('srvDisarm'),
  srvWaitTiers: $('srvWaitTiers'),
  srvWaitWhy: $('srvWaitWhy'), srvWaitWhyMore: $('srvWaitWhyMore'),
  srvWaitWhyDetail: $('srvWaitWhyDetail'),
  srvPeerLabel: $('srvPeerLabel'), srvPeerFp: $('srvPeerFp'), srvPeerCopy: $('srvPeerCopy'),
  srvReasonCap: $('srvReasonCap'), srvPeerReason: $('srvPeerReason'), srvPreview: $('srvPreview'),
  srvSigners: $('srvSigners'),
  srvIntentRow: $('srvIntentRow'), srvIntent: $('srvIntent'),
  srvDecline: $('srvDecline'), srvAccept: $('srvAccept'),
  authOverlay: $('authOverlay'), authForm: $('authForm'), authTitle: $('authTitle'),
  authHint: $('authHint'), authPw: $('authPw'), authPwLabel: $('authPwLabel'), migrateRow: $('migrateRow'),
  keyChoice: $('keyChoice'), keySelect: $('keySelect'), keyPath: $('keyPath'),
  createPath: $('createPath'), authWarn: $('authWarn'), armedPill: $('armedPill'),
  sessionNotice: $('sessionNotice'), sessionNoticeText: $('sessionNoticeText'),
  sessionNoticeDismiss: $('sessionNoticeDismiss'), sessionNoticeAction: $('sessionNoticeAction'),
  repointRow: $('repointRow'), repointPath: $('repointPath'),
  repointPw: $('repointPw'), repointGo: $('repointGo'),
  introBlock: $('introBlock'),
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
  exportCertBtn: $('exportCertBtn'), printBtn: $('printBtn'), closeBtn: $('closeBtn'),
  finalizeModal: $('finalizeModal'), fzText: $('fzText'), fzDate: $('fzDate'),
  fzTsa: $('fzTsa'), fzTsaOn: $('fzTsaOn'), fzCancel: $('fzCancel'), fzGo: $('fzGo'),
  fzOpacity: $('fzOpacity'), fzSize: $('fzSize'), fzAngle: $('fzAngle'), fzColor: $('fzColor'),
  fzSignAs: $('fzSignAs'), fzPassphrase: $('fzPassphrase'),
  timestampBtn: $('timestampBtn'), timestampModal: $('timestampModal'), tsCancel: $('tsCancel'), tsGo: $('tsGo'),
  timestampVerifyBtn: $('timestampVerifyBtn'), tsVerifyModal: $('tsVerifyModal'), tvExplorerOn: $('tvExplorerOn'),
  tvExplorer: $('tvExplorer'), tvFile: $('tvFile'), tvResult: $('tvResult'), tvCancel: $('tvCancel'), tvPick: $('tvPick'), tvSave: $('tvSave'),
  fzPreviewMark: $('fzPreviewMark'),
  pnStamped: $('pnStamped'), fzStamped: $('fzStamped'),
  staleBanner: $('staleBanner'), staleMsg: $('staleMsg'), staleRetry: $('staleRetry'),
  staleReload: $('staleReload'),
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

// repointKey is the key-missing recovery: unlock with a key the user points at, and have
// the server rewrite the slot's recorded path so the NEXT launch finds it too. Without
// that rewrite the user would re-type the path on every start, having been told it was
// fixed.
async function repointKey() {
  const keyPath = els.repointPath.value.trim();
  if (!keyPath) { els.authError.textContent = 'Enter the path to your SSH private key'; return; }
  els.repointGo.disabled = true;
  try {
    const res = await fetch('/api/ssh/repoint', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ keyPath, passphrase: els.repointPw.value }),
    });
    if (!res.ok) { els.authError.textContent = await errText(res, 'could not unlock with that key'); return; }
    els.authError.textContent = '';
    els.repointPw.value = '';
    await refreshStatus();
  } finally {
    els.repointGo.disabled = false;
  }
}
let gsAvailable = false; // Ghostscript installed (from /api/status) → offer the general PDF/A converter
let loAvailable = false; // LibreOffice installed (from /api/status) → offer office-document → PDF conversion

// apiFetch wraps fetch with the CSRF header on writes; a 401 reopens the wizard.
async function apiFetch(url, opts = {}) {
  opts.headers = { ...(opts.headers || {}) };
  if (opts.method && opts.method !== 'GET') opts.headers['X-CSRF-Token'] = csrf;
  // Name the document on every request, so no call site can omit it by
  // forgetting. That is the whole point of doing it here: the server accepts a
  // missing id and falls back to "whatever is active" — a compatibility path the
  // CLI and the older Go tests need — so a handler that simply forgets the header
  // gets the active document during exactly the switch operation-pinning exists to
  // survive, having passed no check. A pinned call and an unpinned one would
  // differ only by an ABSENT header, which nothing in review can see.
  //
  // Attached unconditionally rather than to a list of document routes: a list here
  // would be a second copy of the server's knowledge of which routes those are,
  // in a different language, drifting the first time a route is added. The header
  // is inert on the routes that never resolve a document.
  //
  // This sends the CURRENT id. Carrying the id captured before an operation's
  // first await is operation pinning (P04) — the same value while there is one
  // view, a different one the moment there are several.
  //
  // `opts.unpinned` opts one call out. It exists for exactly one question — "what is
  // the active document now?" — which is about the SESSION rather than about a
  // document, and which a pinned call cannot ask: pinning it means asking after the
  // document the client already knows about, which is never the one it needs to learn.
  // See openArrivalInNewView.
  //
  // `opts.docId` is the other direction, and it is operation pinning itself (D7): the
  // caller captured a document id BEFORE its first await and names it explicitly, so
  // the request goes to the document the payload came from rather than to whatever is
  // current by the time the request is built. Without it, an operation that bakes
  // document A's bytes and then posts them is addressed to B if the document changed
  // in between — and /api/save writes the posted bytes to the addressed document's
  // path, so A's contents land in B's file.
  //
  // This is only safe because ADR-001 makes ids monotonic and never reused: a captured
  // id whose document is gone gets a 409 and the operation is REFUSED. Under a
  // recycled id the same request would be silently redirected at whatever inherited
  // the number, which is worse than the bug it replaces.
  // `'docId' in opts` rather than a truthiness test, deliberately. A caller that
  // captured a document and found no id has a bug, and falling back to the CURRENT
  // id would send exactly the request pinning exists to prevent — silently, and
  // through the very option that was meant to stop it. Presence is the intent;
  // an absent value stays absent.
  const unpinned = opts.unpinned === true;
  const hasPin = Object.prototype.hasOwnProperty.call(opts, 'docId');
  const pinned = opts.docId;
  delete opts.unpinned;
  delete opts.docId;
  if (hasPin) { if (pinned) opts.headers['X-Nib-Doc'] = pinned; }
  else if (!unpinned && view.docMeta && view.docMeta.id) opts.headers['X-Nib-Doc'] = view.docMeta.id;
  const res = await fetch(url, opts);
  if (res.status === 401) { refreshStatus(); throw new Error('locked'); }
  // A 409 ("the document you named is gone") is deliberately NOT thrown the way a 401
  // is. Every document-route call site here already handles it correctly, with the
  // shape `if (!res.ok) { toast('…'); return; }` — 15 of them — so throwing would
  // convert fifteen clear, user-visible failures into unhandled rejections that say
  // nothing at all. The refusal is handled where refusals are already handled; what
  // must not happen is a refusal BODY being mistaken for a document, and that is
  // guarded in setDocumentFromServer, at the one place the mistake is possible.
  //
  // **P06.S03 adds a reconciliation on top of that handling, not instead of it.** A 409
  // means the server no longer holds the document this request named, and the tab for it
  // is then a tab where everything fails. One 409 is usually an ordinary race — a close
  // that landed first — and the reconcile simply drops that one tab. The case the
  // plan-review pin names is the other end: a SERVER RESTART makes every id stale at
  // once, because docFor refuses a foreign epoch before it compares anything else, and
  // the app must resolve to the launch empty state rather than to N tabs that each
  // error. Both are "make the client match the server", which is why there is one
  // function and not two.
  //
  // Fired here rather than at fifteen call sites, for the same reason the header is
  // attached here: a list of the routes that can 409 would be a second copy of the
  // server's knowledge, in a different language, drifting the first time a route is
  // added. Not awaited — the caller's own refusal handling runs now, and the strip
  // catches up a moment later. Guarded against re-entry so a reconcile that itself gets
  // a 409 cannot recurse.
  if (res.status === 409 && !reconciling && !unpinned) {
    reconciling = true;
    reconcileWithServer().catch(() => {}).finally(() => { reconciling = false; });
  }
  return res;
}

// reconciling guards apiFetch's 409 hook against re-entry: reconcileWithServer issues
// requests of its own, and a 409 from one of those would call it again.
let reconciling = false;

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
    // Restore FIRST, then honour ?open=. The order is load-bearing twice over: the
    // guard this replaces was `!view.pdfDocument`, which is false the moment a restore
    // has put a document in the view — so the file the OS asked nib to open would
    // silently not open — and a ?open= handled before the restore would be buried under
    // the documents that arrive after it.
    if (!restored) {
      restored = true;
      reconcileWithServer()
        .catch((e) => console.error('restore failed', e))
        .then(() => {
          const params = new URLSearchParams(location.search);
          // A hand-off that did not open a document says so here. The launch that sent
          // it has no terminal — a double-click's stderr goes nowhere a user will look —
          // so the message travels on the URL it surfaces this window with.
          //
          // A CODE, mapped to words HERE. The launch names which thing happened and the
          // UI owns the sentence: nothing attacker-influenced is ever rendered, and the
          // wording lives where wording is edited rather than in a Go string.
          //
          // Its honest limit, stated because it is easy to mistake for a delivery
          // guarantee: this arrives only when the browser opens a NEW window for the
          // surfaced URL. If it focuses the existing one instead, that page never
          // reloads and never sees the parameter — the same reason Nib cannot promise
          // to raise a window it does not own.
          const NOTICES = {
            'handoff-refused': 'That document could not be opened here — Nib may be full.',
            'handoff-queued': 'That document will open once you unlock Nib.',
          };
          const notice = NOTICES[params.get('notice')];
          if (notice) toast(notice);
          const initial = params.get('open');
          if (initial) openOrActivate(initial).catch((e) => toast('could not open: ' + e.message));
        });
    }
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
    // Two ways out, and the second one is the addition: the vault is sealed to the key's
    // PUBLIC half, so a private key found anywhere on disk opens it. Before this the only
    // offered action was "restore that key file", pointing at a path that may be
    // meaningless — `id_ed25519` with no directory, or a folder literally named `~`.
    els.authHint.textContent = `Nib can't read the SSH key it was set up with (${st.keyPath || 'unknown path'}). Put that file back and retry, or tell Nib where it is now.`;
    els.authWarn.hidden = true;
    els.keyChoice.hidden = true;
    els.repointRow.hidden = false;
    if (!els.repointPath.value) els.repointPath.value = st.keyPath || '';
    els.authSubmit.textContent = 'Retry';
    return;
  }
  els.repointRow.hidden = true;
  els.keyChoice.hidden = false;
  els.authWarn.hidden = false;
  els.authTitle.textContent = st.state === 'migrate' ? 'Migrate to SSH-key unlock' : 'Set up Nib';
  els.authHint.textContent = st.state === 'migrate'
    ? 'Enter your old vault password once; Nib will re-key the vault to your SSH key.'
    : 'Choose the SSH key that unlocks Nib. No password is used.';
  // Prefill only a real path. The old fallback put the literal "~/.ssh/id_ed25519"
  // in the field, which the server now rejects (a "~" path is expanded, but this
  // was a placeholder standing in for a path the server couldn't work out). Both
  // hints come from the server too, so Windows isn't shown a POSIX example.
  els.createPath.value = st.defaultKeyPath || '';
  if (st.defaultKeyPath) {
    els.createPath.placeholder = st.defaultKeyPath;
    els.keyPath.placeholder = st.defaultKeyPath;
  }
  els.authForm.querySelector('input[value="use"]').checked = haveCandidates;
  els.authForm.querySelector('input[value="create"]').checked = !haveCandidates;
  els.authSubmit.textContent = st.state === 'migrate' ? 'Migrate' : 'Enable';
  syncKeyMode();
  // First run only, and IN the card rather than over it (v1.109.3). Set on every render
  // rather than once per session: the old overlay was shown by this line and hidden only
  // by the user, so a status change while it was up left it stranded over a surface it no
  // longer described — a key-missing screen wearing a welcome card. Driving it from the
  // state each time means it cannot outlive the state it belongs to.
  els.introBlock.hidden = st.state !== 'setup';
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
  // Recovery, and it is checked BEFORE the key-mode block below rather than after.
  //
  // The vault is still sealed to the enrolled key's PUBLIC half; only the path was lost.
  // So retrying is a status re-check (ensureUnlocked runs server-side) and needs no key
  // choice at all — which is why `#keyChoice` is hidden in this state and `keySelect` is
  // never populated.
  //
  // **It used to sit after the key-mode block and was therefore unreachable.** With no
  // key choice offered, `selectedKeyMode()` returned its default `use`, `keySelect.value`
  // was "", and the block returned early with "No key selected." — so the Retry button on
  // the key-missing screen showed an error about a control the user could not see, and
  // never re-read the status. The documented way out of a misplaced key was dead; the
  // repoint button beside it was the only one that worked.
  if (authState === 'key-missing') { await refreshStatus(); return; }

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

// emptyNote puts an empty-state message into a container as REAL TEXT.
//
// These four messages lived in the stylesheet as `:empty::after` content, which cost
// twice over: generated content cannot be selected or copied, and assistive tech exposes
// it inconsistently — so a sentence telling the user WHY a list is blank reached some
// users and not others. The house shape already existed (renderOutlineEditor has built a
// real element for "No bookmarks yet" all along); these four just never used it.
function emptyNote(el, text) {
  const p = document.createElement('p');
  p.className = 'emptynote';
  p.textContent = text;
  el.appendChild(p);
  return p;
}

function renderKeys(data) {
  els.keysList.innerHTML = '';
  if (!data.keys.length) emptyNote(els.keysList, 'No keys enrolled.');
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

// peerOptionLabel is the one label every peer <select> uses.
//
// There are FOUR of them — co-sign, sign-in-person, serve, and send — and before this they
// carried four copies of the same expression, with the serve one carrying a fifth in its
// dataset. Four copies of a display rule is four places to forget when the rule changes,
// which is exactly what happened here: the slice was scoped against three, and the fourth
// and fifth were found by a count assertion rather than by reading.
//
// Hex is deliberately gone from the label. Eight characters of a fingerprint is neither
// readable nor speakable, and the six words are the identity a person can actually check;
// the option VALUE still carries the full hex, which is what addresses the peer.
function peerOptionLabel(p) {
  if (p.label && p.name) return p.label + ' — ' + p.name;
  return p.label || p.name || 'Unlabelled peer';
}

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
  // The name is read off the payload at each render site rather than into a second
  // module global beside selfFingerprint. That variable is already written by two
  // different loaders and is only benign because both write the same value; a parallel
  // selfName would double a hazard rather than add a field.
  //
  // The fallback is NOT the fingerprint. Falling back to hex would put it on the default
  // screen in exactly the case the criterion is about — and a criterion that holds only
  // when the happy path fires is not a criterion. Hex has a home two lines below, behind
  // the disclosure; this slot says the name is missing.
  els.peerSelfName.textContent = data.name || '(name unavailable)';
  els.peersList.innerHTML = '';
  if (!(data.peers || []).length) emptyNote(els.peersList, 'No peers pinned yet.');
  for (const p of data.peers || []) {
    const row = document.createElement('div');
    row.className = 'keyrow';
    const meta = document.createElement('div');
    meta.className = 'keymeta';
    const name = document.createElement('div');
    name.className = 'keyfp';
    // Titled by what identifies them. A label is what YOU called them and wins when set;
    // otherwise the six-word name, which is derived from their key and is the thing that
    // makes an otherwise anonymous row identifiable (D3). "Unlabelled peer" survives only
    // where neither exists.
    name.textContent = p.label || p.name || 'Unlabelled peer';
    const fp = document.createElement('div');
    fp.className = 'keysub';
    // The subtitle carries the name only when the label took the title — otherwise the
    // name is already up there. Never the fingerprint: this row is on the default pairing
    // screen, and Copy is how hex is reached from here.
    fp.textContent = (p.label && p.name) ? p.name : '';
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
  if (!view.pdfDocument) return;
  const res = await apiFetch('/api/peers');
  if (!res.ok) { toast('could not load peers'); return; }
  const peers = (await res.json()).peers || [];
  els.cosignPeer.innerHTML = '';
  for (const p of peers) {
    const o = document.createElement('option');
    // The VALUE stays hex: it is the addressing key posted to the co-sign, serve and
    // send routes, and L1 forbids a name resolving a pin. Only the label changes.
    o.value = p.fingerprint;
    o.textContent = peerOptionLabel(p);
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
// (Go AppearanceLines), never rebuilt here.
//
// **It CAN drift from /Reason inside a ceremony, and did (/pending 317, corrected 2026-08-30).**
// The quote's lines come from `cosignAttestation`, which never calls `StampCommitment`, while the
// signature is built by `coSignExchange`, which does. Measured at the route with an 8-signer
// roster: the block draws 5 lines and the signature carries 6, and only the header matches.
// Outside a ceremony the two do agree, because `StampCommitment` early-returns. Fixing it is a
// P06 slice; this comment states what is true until then.
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
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
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
  openSaveAs(await sr.blob(), exportName + '-cosigned.pdf', 'Save co-signed PDF');
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
  if (!view.pdfDocument) { toast('Open a PDF first'); return; }
  const res = await apiFetch('/api/peers');
  if (!res.ok) { toast('could not load peers'); return; }
  const peers = (await res.json()).peers || [];
  els.sinPeer.innerHTML = '';
  for (const p of peers) {
    const o = document.createElement('option');
    // The VALUE stays hex: it is the addressing key posted to the co-sign, serve and
    // send routes, and L1 forbids a name resolving a pin. Only the label changes.
    o.value = p.fingerprint;
    o.textContent = peerOptionLabel(p);
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
  // Captured before the first await, and this is the operation where getting it wrong
  // is worst: three awaits (the quote, the attestation render, the bake) separate the
  // click from the POST, and what goes out is a DOCUMENT SENT TO A PEER. A bare
  // bakedForm() on the far side of them sends whichever document is active by then,
  // signed and addressed to a pinned counterparty, and the reload then installs the
  // co-signed result over whatever view is active when the session returns — which for
  // a live co-sign is minutes later.
  const owner = view;
  const opDoc = owner.docMeta;
  const fingerprint = els.sinPeer.value;
  if (!fingerprint) return;
  // P05.S12: an empty address is the LADDER default, not an error — the server's LAN browse (and, for
  // an invited ceremony, the DHT) finds the armed peer. The typed address is the manual fallback (D8
  // tier 5), reachable from the Advanced disclosure. A failure now carries S11's D19 diagnosis in the
  // response body; the client surfaces its plain summary via errText — P06 renders the cause and detail.
  const address = els.sinAddr.value.trim();
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
    const form = await bakedForm(owner);
    form.append('params', JSON.stringify({ fingerprint, intent, when: q.when }));
    form.append('appearance', png, 'attestation.png');
    form.append('address', address);
    // The spoken check happens while this request is in flight — the server is blocked
    // waiting for it — so the poller runs beside the call, not before or after it.
    const stopVerify = startVerifyPoll();
    let res;
    try {
      res = await apiFetch('/api/session/initiate', { method: 'POST', body: form, docId: opDoc && opDoc.id });
    } finally {
      stopVerify();
    }
    if (!res.ok) { toast(await errText(res, 'co-signing did not complete')); return; }
    els.sessionInitModal.hidden = true;
    await setDocumentFromServer(await res.json(), owner);
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
// reflectArmed shows or hides the persistent armed-session indicator.
//
// **Why this exists at all.** An armed co-signing session is a separate lifecycle from
// the open document: internal/server/session.go calls setDoc() when an exchange
// completes, so after the user closes their document a completed co-sign can make a
// document appear again with NO user action. That is not a defect in the close route —
// arming a session IS a request to receive a document, and disarming on close would
// destroy state the user set up independently, silently. The defect is that nothing told
// the user it could happen: after a Close the app looked idle, and it wasn't.
//
// So the behaviour is left alone and the STATE is surfaced. Deliberately in the header
// rather than in the receive dialog, because the dialog is exactly what is not on screen
// when this matters. Checked (2026-08-17) that P06's tabs do NOT resolve this on their
// own: an arrival after a Close still lands in the single empty view, because after a
// Close there is nothing to arrive beside.
function reflectArmed(on) {
  if (els.armedPill) els.armedPill.hidden = !on;
}

// reflectNotice is the READER for sessionStatus.notice — the sticky failure surface P08.S08
// built and nothing consumed.
//
// **It exists because the fields were published and unread, and `published.test.mjs` said so.**
// A status field with no reader means the user is never told, and here that is the whole point of
// the field. **The producer list was "two" and is now five** (/pending 345/346): "signed but not
// saved", "received but not saved", "the hop never reached disk", "the proceeding has ended" and
// "this arrival was refused". The first three ask the user to do something before they close Nib;
// the last two tell them why a ceremony they are armed for is not progressing, which is the other
// thing a sticky notice is for — a background arm has no response to say it in.
//
// Rendered, not toasted, for the reason `noticeView`'s own doc gives: the disarm IS the symptom,
// so a message that goes away with the session is a message nobody reads. It persists until the
// user dismisses it or a later session replaces it.
let noticeShownAt = '';
function reflectNotice(n) {
  if (!els.sessionNotice) return;
  if (!n || !n.summary) return; // never CLEAR on absence — see below
  // Keyed on the timestamp so a poll every 1.5 s does not re-announce the same sentence to a
  // screen reader forever, while a genuinely new failure does re-announce.
  if (n.at === noticeShownAt) return;
  noticeShownAt = n.at;
  els.sessionNoticeText.textContent = n.detail ? n.summary + ' — ' + n.detail : n.summary;
  // `what` is the stable key a surface can branch on — that is its declared purpose, and it
  // exists so the branch is on a key rather than on the prose of `summary`, which is a sentence
  // that will be reworded. It selects the RECOVERY ACTION, because a warning over a state the
  // user cannot act on is a warning that only tells them they have lost something.
  //
  // The two "not saved" failures leave the bytes in an open tab, which is exactly what makes the
  // rescue possible: `signed-not-saved`'s own detail says "The document is open — save a copy
  // somewhere with space". `hop-not-mirrored` has no local document to offer, so it gets the
  // sentence and no button rather than a control that would do nothing.
  const rescuable = n.what === 'signed-not-saved' || n.what === 'received-not-saved';
  els.sessionNoticeAction.hidden = !rescuable;
  els.sessionNoticeAction.textContent = rescuable ? 'Save a copy…' : '';
  els.sessionNotice.hidden = false;
}

// reflectDiagnosis is the READER for sessionStatus.diagnosis — D19's live "why has nobody
// connected yet", computed since P05.S11 and consumed by nothing until /pending 349.
//
// **The field states its own purpose and the gap was exactly that sentence**: "so the polling UI
// shows why nothing has connected yet, RATHER THAN A BLANK WAIT". A named search found `diagnosis`
// in this file once, inside a comment. So the user watching an arm got the blank wait for the whole
// life of the feature — `reflectNotice`'s shape (published at P08.S08, read thirteen versions
// later), one layer over.
//
// # It CLEARS on absence, where reflectNotice deliberately does not
//
// A notice is a STICKY record of something that already failed; clearing it on a poll that happens
// not to carry one would erase a failure the user has not read. A diagnosis is LIVE STATE — the
// answer to "why is nothing happening *now*" — so when the server stops sending one, the reason has
// stopped applying and leaving the old sentence on screen would be a stale explanation of a
// condition that has passed. Opposite fields, opposite rules; the difference is why this is not
// folded into reflectNotice.
//
// # `cause` selects the TONE, and that is what it is for
//
// It is the machine key, and printing it would put `peer-not-started` in front of somebody. What it
// decides is whether this reads as a PROBLEM: "the other side hasn't started" is the ordinary state
// of a ceremony arm whose counterparty is still reading their email, and dressing it as a fault
// teaches the user to ignore the line that will one day say the rendezvous is unreachable.
let diagnosisShownFor = '';
function reflectDiagnosis(d) {
  if (!els.srvWaitWhy) return;
  if (!d || !d.summary) {
    els.srvWaitWhy.hidden = true;
    els.srvWaitWhyMore.hidden = true;
    diagnosisShownFor = '';
    return;
  }
  // Keyed on the rendered text, because this polls every 1.5 s and `aria-live` would otherwise
  // re-announce an unchanged sentence to a screen reader forever. `reflectNotice` keys on a
  // timestamp; a diagnosis carries none, and the text is what the user would notice changing.
  const key = d.cause + '\u0000' + d.summary + '\u0000' + (d.detail || '');
  if (key === diagnosisShownFor) return;
  diagnosisShownFor = key;
  els.srvWaitWhy.textContent = d.summary;
  // Benign while waiting: a counterparty who has not started yet is the expected early state of
  // every ceremony arm, not a fault.
  els.srvWaitWhy.dataset.cause = d.cause || '';
  els.srvWaitWhyDetail.textContent = d.detail || '';
  els.srvWaitWhyMore.hidden = !d.detail;
  els.srvWaitWhy.hidden = false;
}

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
  els.srvSelfName.textContent = data.name || '(name unavailable)';
  const peers = data.peers || [];
  els.srvPeer.innerHTML = '';
  for (const p of peers) {
    const o = document.createElement('option');
    o.value = p.fingerprint;
    // dataset.label feeds recvArmedLabel, which names the peer in "waiting for …" and in
    // the consent card's fallback — so it gets the same identity the option shows, or the
    // two disagree about who you armed for.
    o.dataset.label = peerOptionLabel(p);
    o.textContent = peerOptionLabel(p);
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
  // P05.S12 twin: an empty bind is the LAN receive default, not an error — the server binds an
  // ephemeral port (0.0.0.0:0) and announces it over the local network and the DHT, which is the
  // whole of P03's first exit criterion. A hardcoded default (the old 0.0.0.0:8443) also broke
  // two Nibs on one machine, since the second cannot take the same port. The typed address is the
  // manual fallback, behind the Advanced disclosure.
  const bind = els.srvBind.value.trim();
  recvArmedLabel = opt.dataset.label;
  els.srvArmGo.disabled = true;
  const res = await apiFetch('/api/session/arm', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    // The invitation is what gives this arm a ceremony identity — a roster, a hop, and the
    // secret every rendezvous derivation needs. Optional: without one this is the manual and
    // LAN path, and nothing reaches the internet.
    body: JSON.stringify({
      fingerprint: opt.value, bind, mode: recvMode,
      invitation: (els.srvInvite && els.srvInvite.value.trim()) || undefined,
    }),
  });
  els.srvArmGo.disabled = false;
  if (!res.ok) { toast(await errText(res, 'could not arm')); return; }
  const st = await res.json();
  els.srvWaitAddr.textContent = st.address || bind;
  els.srvWaitPeer.textContent = recvArmedLabel;
  recvStage = 'wait';
  reflectArmed(true);
  showRecvView('srvWait');
  const token = ++recvPoll;
  setTimeout(() => pollRecv(token), 1200);
}

// --- the spoken check (D4, L2) ------------------------------------------------
// Both sides must confirm four words before ANY document byte crosses. The words are
// derived from the session on both machines and are shown here; one party reads them out.
//
// **It runs for both roles, and the initiating role is the one that is easy to miss.** The
// receiving side is already polling, so its card falls out of the poller below. The
// initiating side is blocked inside its own /api/session/initiate or /send request — the
// server is waiting on this answer while that request is in flight — so the confirmation
// has to arrive on a SEPARATE request. Hence a poller of its own, started beside the call
// and stopped when it returns. Without it the gate is mandatory and unanswerable: a
// co-sign would hang for the consent window and then time out, every time.
let verifyPoll = 0;

function showVerify(words) {
  els.verifyWords.textContent = words;
  els.verifyModal.hidden = false;
}

async function answerVerify(confirmed) {
  els.verifyModal.hidden = true;
  try {
    await apiFetch('/api/session/verify', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirmed }),
    });
  } catch { /* the session will time out and say so; a toast here would be a second story */ }
  if (!confirmed) toast('Stopped — the words did not match');
}
els.verifyConfirm.onclick = () => answerVerify(true);
els.verifyCancel.onclick = () => answerVerify(false);

// startVerifyPoll watches for the spoken check while an outbound request is in flight.
// Returns a stop function; the caller stops it in a finally, so a failed request does not
// leave a poller running against the next session.
function startVerifyPoll() {
  const token = ++verifyPoll;
  let shown = false;
  const tick = async () => {
    if (token !== verifyPoll) return;
    try {
      const st = await (await apiFetch('/api/session/status')).json();
      if (token !== verifyPoll) return;
      if (st.verify && !shown) { shown = true; showVerify(st.verify.words); }
    } catch { /* keep polling; the request in flight owns the error reporting */ }
    if (token === verifyPoll) setTimeout(tick, 800);
  };
  tick();
  return () => { verifyPoll++; els.verifyModal.hidden = true; };
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
  reflectArmed(!!st.armed); // the server can disarm on its own (a timeout), and then so does this
  reflectNotice(st.notice);
  reflectArmProgress(st.progress);
  reflectDiagnosis(st.diagnosis);
  // `until` is when this arm gives up. Read here so the indicator says how long is left rather
  // than only that something is armed — which is what the field was added for (C05): a five-minute
  // manual bound and a thirty-day ceremony bound are indistinguishable from the OUTCOME and
  // trivially distinguishable from the figure.
  //
  // **It says what the figure is a bound ON, because a ceremony's arm bound is thirty days
  // (/pending 366).** `until` is `ceremony.MaxCeremonyLife` for a ceremony arm — an invitation
  // carries no deadline, which is `/pending 247` — so a user arming for a two-day proceeding was
  // told they were "armed until" a date a month away. That is a true sentence about the arm and a
  // misleading one about the ceremony, and on the old wording the two were indistinguishable. The
  // proceeding's own deadline is the panel's "Open until", a few pixels away and correct; this
  // pill is not a second place to read it.
  if (els.armedPill && st.armed && st.until) {
    els.armedPill.title = 'This machine stays reachable for a co-signing session until ' +
      new Date(st.until).toLocaleString() +
      ' — that is this arm\u2019s own bound, not the ceremony deadline. Click to open it';
  }
  // The spoken check comes BEFORE the document, so it is checked before `pending`.
  if (st.verify) {
    if (els.verifyModal.hidden) showVerify(st.verify.words);
  } else if (!els.verifyModal.hidden) {
    els.verifyModal.hidden = true; // the server moved on (answered elsewhere, or timed out)
  }
  if (st.pending && recvStage === 'wait') {
    recvStage = 'consent';
    showConsent(st.pending);
  } else if (!st.armed) {
    if (recvStage === 'applying' && recvMode === 'receive') {
      // `peer` is who it came from — published for exactly this and never read until
      // the shape scan asked. "Saved <path>" alone is the one fact the user can already
      // see; who sent it is the one they cannot.
      // Same rule as the co-sign branch below: with no `received` AND a recorded failure, the
      // document was NOT saved, and "Document received" is the wrong sentence for that.
      if (st.received) {
        toast('Saved ' + st.received.path + (st.received.peer ? ' — from ' + st.received.peer : ''));
      } else if (!st.notice) {
        toast('Document received');
      }
    } else if (recvStage === 'applying') {
      await openArrivalInNewView();
      // **The success sentence is withheld when the session recorded a failure.** It used to
      // fire unconditionally: a persist failure sets `rerr = nil` so the delivery proceeds
      // (D24 as amended), which meant a signer whose machine kept NO copy was told
      // "Co-signed — it opened alongside your document". The notice beside it said the
      // opposite, and the toast is the louder of the two. Telling someone it worked is worse
      // than the silence this surface replaced.
      if (!st.notice) toast('Co-signed — it opened alongside your document');
    }
    else if (recvStage === 'wait') toast('Session ended — no peer connected');
    else if (recvStage === 'consent') toast('Session timed out');
    endRecv();
    return;
  }
  setTimeout(() => pollRecv(token), 1500);
}

function showConsent(pending) {
  recvPeerFp = pending.fingerprint || '';
  // `pending.signer` is who the SERVER saw connect; recvArmedLabel is the label this
  // client remembers arming for. They are usually the same and the difference matters
  // when they are not — the consent dialog is the one place the user decides based on
  // who this is, so it shows the observed identity and falls back to the remembered one.
  els.srvPeerLabel.textContent = pending.signer || recvArmedLabel || 'your pinned peer';
  els.srvPeerFp.textContent = groupFingerprint(recvPeerFp);
  els.srvPeerReason.textContent = pending.reason || '(none given)';
  renderConsentSigners(pending.signers || []);
  showRecvView('srvConsent');
  loadPendingPreview(recvPoll);
}

// renderConsentSigners lists everyone already on the document the user is being asked to join
// (D27 item 3, C09).
//
// **The connected peer is not the roster, and in a ceremony it is often not even a signer.**
// `showConsent` above names one person — who the server saw connect — and under a carry route
// that is a non-signing convener. A party asked to sign sixth was told about one person while
// holding a document bearing five signatures.
//
// **Both states render.** An empty list produces the sentence rather than nothing, because
// "nobody has signed this yet" and "Nib did not look" are different facts and an absent element
// says the second. The first hop of a ceremony and every one-way transfer take that branch, and
// they are the ordinary case rather than an error.
//
// An INVALID signature is listed and marked, never dropped: a document arriving with a broken
// signature on it is exactly what the user needs to see before adding theirs, and silently
// omitting it would make the list shorter and the document look cleaner than it is.
function renderConsentSigners(signers) {
  const box = els.srvSigners;
  if (!box) return;
  box.innerHTML = '';
  const cap = document.createElement('div');
  cap.className = 'cidlabel';
  cap.textContent = 'Already signed by';
  box.appendChild(cap);
  if (!signers.length) {
    const p = document.createElement('p');
    p.className = 'libhint';
    p.textContent = 'Nobody yet — you would be the first to sign this document.';
    box.appendChild(p);
    return;
  }
  for (const s of signers) {
    const row = document.createElement('div');
    row.className = 'cidrow';
    const who = document.createElement('strong');
    who.textContent = s.signer || '(unnamed)';
    row.appendChild(who);
    const fp = document.createElement('span');
    fp.className = 'cidcap';
    fp.textContent = ' ' + groupFingerprint((s.fingerprint || '').slice(0, 8)) + '…';
    row.appendChild(fp);
    if (!s.valid) {
      const bad = document.createElement('span');
      bad.className = 'sigatt-warn';
      bad.textContent = ' — this signature does not verify';
      row.appendChild(bad);
    }
    box.appendChild(row);
  }
}

// loadPendingPreview renders the received document in its own pdf.js instance,
// entirely apart from the main viewer, so reviewing (and declining) a peer's
// document never disturbs the open document or its unsaved edits.
async function loadPendingPreview(token) {
  els.srvPreview.innerHTML = '';
  const loading = emptyNote(els.srvPreview, 'Loading the document…');
  let doc;
  try { doc = await pdfjsLib.getDocument({ ...PDFJS_OPTS, url: '/api/session/pending-pdf?t=' + Date.now() }).promise; }
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
      loading.remove(); // no-op after the first page; the note is gone once anything renders
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
  // The responder's visible block is placed server-side; the quote gives only the canonical lines.
  //
  // **The drift is closed (P06.S06, /pending 317).** This comment used to say the rendered image
  // DOES drift from the signed /Reason inside a ceremony, and it did: the quote never applied
  // `StampCommitment`, so the six fields a roster overwrites — the recital, the signer's label,
  // the capacity, the position, the roster size and its hash — were absent from the block the
  // party read and present in the signature underneath it. The quote stamps now, and it pins the
  // TIME, which is echoed below so the signature carries the moment the party was asked rather
  // than the moment the bytes were signed.
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
    // `when` is the quote's pinned time, echoed so the signature carries the block the party
    // actually consented to. The server bounds it by the same `maxWhenSkew` the initiating side
    // has always applied, and drops it out of range rather than refusing.
    body: JSON.stringify({ accept: true, intent, appearance, when: q.when }),
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

// openArrivalInNewView is what happens after runSession applies a received co-signature
// out of band (no response carries the new metadata, so the UI fetches it).
//
// It ADDS a view. Until P05.S04 this function called setDocumentFromServer, which pointed
// the single view at the arrival — and its own comment already said why that was wrong:
// "A co-signature applied out of band ADDS a document and makes it active (D10); the
// document this client currently names is still open and still perfectly valid." The
// server half of that landed in P03.S05; the client half did not, so the arrival took the
// view. Every overlay element and typed value on the document the user was working on went
// through clearOverlays(), and that document survived only on the server, unreachable.
//
// **No longer the only path that makes a second view exist (P06.S01).** A user Open now
// adds too, on the server and here, and the tab strip makes the result reachable — the
// pairing this comment said was P06's. The build-load-activate body below became
// openInNewView so both paths run one implementation rather than two that agree today.
async function openArrivalInNewView() {
  // UNPINNED, deliberately. A co-signature applied out of band ADDS a document and
  // makes it active (D10); the document this client currently names is still open and
  // still perfectly valid. So a pinned fetch here answers the wrong question — it
  // reports the document the user was already looking at, and the arrival never
  // appears. What this call needs is "what is active now?", which is a question about
  // the session and the only one in the app that is.
  try {
    const res = await apiFetch('/api/doc', { unpinned: true });
    // The ok check this function did not have, and the whole of the original defect:
    // it parsed the body unconditionally and handed it to setDocumentFromServer. Every
    // other document-route call site in this file already had this line.
    if (!res.ok) { toast('co-signed, but could not refresh the view'); return; }
    const meta = await res.json();

    // installOpened, not openInNewView directly: after a Close there is one EMPTY view,
    // and building a second one beside it would put an "Untitled" tab in the strip for a
    // view holding no document. Before the strip existed that was invisible; it is the
    // sort of thing a new surface exposes rather than causes.
    if (!(await installOpened(meta))) return;
  } catch { toast('co-signed, but could not refresh the view'); }
}

// openInNewView builds a view for `meta`, loads the document into it while it is
// hidden, and activates it. Returns false when the load refused, having left no
// half-built view behind.
//
// One implementation for both callers — the co-sign arrival and every user Open —
// because they are the same three steps and the failure handling is the part that is
// easy to get subtly different. The repo has paid for that before: `openBrowse` and
// `browseDir` were one folder browser written twice behind four dialogs, and the two
// copies had drifted.
//
// **The view is built and loaded BEFORE the switch**, so a failure part-way through
// leaves the user on the document they had rather than on a half-built one. newView()
// hides it automatically when another container already exists, which is what makes
// this the hidden-load path — and, since P06.S01, what makes that path reachable by
// ordinary use rather than only by a live co-sign.
async function openInNewView(meta) {
  const v = newView();
  addView(v);
  // Caught, so the cleanup below is reached on the THROW path too. Without this, an
  // exception anywhere in the load left the half-built view in `views` with a tab in the
  // strip for a document that never arrived — the cleanup ran only on the return path.
  try {
    await setDocumentFromServer(meta, v);
  } catch (e) {
    console.error('loading a document into a new view threw', e);
  }
  // The liveness test is `pdfDocument`, and it is only sound because this view is
  // FRESH: all four of setDocumentFromServer's early returns leave it null on a new
  // record. A reused view could carry one from a previous load.
  if (!v.pdfDocument) { // the load refused; drop the empty view rather than switch to it
    v.container.remove();
    v.thumbGrid.remove();
    v.outlineList.remove();
    // removeView is a no-op when the view is already gone, which it can be: a Close
    // can land during the load above and resetViews drops everything but the active
    // view. The bare `views.splice(views.indexOf(v), 1)` this replaces would have
    // removed the LAST element on indexOf -1 — the live active view.
    removeView(v);
    return false;
  }
  activateView(v);
  return true;
}

// endRecv stops polling, discards the isolated preview, and closes the modal.
function endRecv() {
  recvPoll++; // invalidate any in-flight poll / preview render
  recvStage = 'arm';
  reflectArmed(false);
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
  if (!view.pdfDocument) { toast('Open a PDF first'); return; }
  const res = await apiFetch('/api/peers');
  if (!res.ok) { toast('could not load peers'); return; }
  const data = await res.json();
  const peers = data.peers || [];
  els.ssnPeer.innerHTML = '';
  for (const p of peers) {
    const o = document.createElement('option');
    // The VALUE stays hex: it is the addressing key posted to the co-sign, serve and
    // send routes, and L1 forbids a name resolving a pin. Only the label changes.
    o.value = p.fingerprint;
    o.textContent = peerOptionLabel(p);
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
  // Captured before the first await, and threaded into every helper below: they
  // receive bytes derived from THIS document, and their own entry is already too late.
  const owner = view;
  const opDoc = owner.docMeta;
  let bytes = await bakedBytes(opDoc && opDoc.id, owner);
  // collectFlags reads the OWNER's markers. It used to read the active view's, on
  // the far side of the bake, under a comment claiming everything below was threaded
  // — so one document's bytes could be sent to a pinned peer carrying another
  // document's signing-flag layout, and the peer would open it in signing mode at
  // coordinates that mean nothing.
  const flags = collectFlags(owner);
  if (flags.length) bytes = await embedFlags(bytes, flags, opDoc && opDoc.id);
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
    const stopVerify = startVerifyPoll();
    let res;
    try {
      res = await apiFetch('/api/session/send', { method: 'POST', body: form });
    } finally {
      stopVerify();
    }
    if (!res.ok) { toast(await errText(res, 'could not send')); return; }
    const r = await res.json();
    // FIVE outcomes, because the server publishes four booleans and reading fewer than all of
    // them reports one of the others. It reported "Sent" for a transfer that neither completed
    // nor was declined; then `timedOut` was added because the server had been calling an
    // unanswered request a decline, and reading only `declined` would have gone on telling the
    // user a person refused when nobody was there. `notStored` is the same lesson a third time
    // (P08.S05a): the peer accepted it and their disk failed, which is neither a decline nor a
    // transport fault, and the action it calls for — ask them to arm again and resend — is not
    // the action either of those calls for.
    toast(r.declined ? 'The peer declined the document'
      : r.timedOut ? 'Nobody answered on the other machine — the document was not kept'
        : r.notStored ? 'They accepted it, but their machine could not save it — ask them to try again'
          : r.sent ? 'Sent — the peer has the document'
            : 'The peer did not take the document');
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

// --- the view record ---------------------------------------------------------
//
// One open document's client-side state. Today exactly one view exists and `view` is
// always it, so behaviour is identical — the record exists so that P05.S03 can give each
// document its own viewer and DOM, and so the bindings below stop being shared before
// anything can share them.
//
// **Swap-on-switch was refused** (ADR-002). Leaving these as module-level
// bindings and saving/restoring them at the switch boundary would cost no reference
// churn, and would make the module-level values a cache whose correctness depends on
// nothing async reading them across a switch — precisely the class P04 spent three
// slices closing on the server-addressed side, reintroduced where there is no id to
// check and no 409 to refuse.
//
// The five marked SAFETY below are not UI state — three at S01, plus selectedPages (S04)
// and overlayHistory (S06), both found by deepdives of other slices. Sharing them does not
// produce a stale
// label; it produces destroyed content, a broken promise to a counterparty, and a
// misreported cryptographic fact, in that order.
// Everything one view owns is built HERE, in one function, and that is deliberate:
// this is the only place the four pdf.js objects and the two DOM nodes appear by bare
// name, so every other site in the file must reach them through the record. The
// source guard in view.test.mjs excludes exactly this function's body for that reason.
// viewSeq numbers view containers so each gets a unique DOM id. Monotonic and never
// reused, for the same reason document ids are (ADR-001): a recycled id makes a stale
// aria-controls or querySelector resolve to the wrong view while still resolving.
let viewSeq = 0;

function newView() {
  const v = {
    // SAFETY — marks drawn on this document, baked by /api/redact through
    // commitBarrier, which clears the undo history BY DESIGN. Marks belonging to one
    // document baked onto another is irreversible destruction with no path back: the
    // worst single outcome anywhere in this plan.
    redactMarks: [],
    // SAFETY — a received signing document opens locked and non-editable, which is a
    // guarantee made to a counterparty. Ambiguity must resolve toward LOCKED: unlocking
    // the wrong document breaks the promise, locking the wrong one is friction.
    signLocked: false,
    // SAFETY — the signature-details modal is where a trust decision is made. Showing
    // one document's verification result under another's name is not a stale label; it
    // misreports a cryptographic fact.
    lastSig: null,

    // Staleness token: bumps on each load so a stale async render/build can bail. Per
    // view because a shared one lets a background document's finishing build abort the
    // foreground's.
    docGen: 0,
    // Whether this view has ever been given a real scale. A view that loads while
    // HIDDEN reports clientWidth 0, so fitWidestWidth silently no-ops and the view is
    // left at pdf.js's default — indistinguishable, by reading currentScale alone, from
    // a user who deliberately zoomed to 100%. activateView re-fits only when this is
    // false, which is what lets a switch preserve zoom (P06 exit criterion) while still
    // rescuing a view that never got one (P05's carried clause).
    hasScale: false,
    // Whether the scale in effect is one the USER chose, as opposed to one we fitted
    // for them. It is a different question from `hasScale` and it has to be, because
    // the two automatic fits answer to it differently: the widest-page refine at
    // `pagesloaded` must not run over a zoom (it lands AFTER the load, and on a long
    // document that is long after the user could have zoomed), while activateView's
    // rescue-fit must not run over one either. A view with a user scale is a view
    // whose scale is nobody else's to set.
    userScale: false,
    // Why the pixels on screen are not what `docMeta` describes, or '' when they are.
    //
    // `setDocumentFromServer` assigns `docMeta` BEFORE awaiting the render, and two of its
    // failure paths return after that. Restoring the old meta would be wrong — for an
    // operation reload the server HAS applied the operation, so the new meta names the
    // state the server holds and the stale thing is the RENDER. Tearing the view down
    // would be worse: it loses the user's document because a re-render failed. So the view
    // keeps both and says which is which.
    stale: '',

    // The file this document was opened from now holds something else — a DIFFERENT
    // condition from `stale` above, and kept apart from it rather than folded in.
    // `stale` says the pixels disagree with the server; this says the server disagrees
    // with the disk. They have different causes, different remedies and different
    // buttons, and one string could only carry whichever was written last.
    diskChanged: false,
    outlineItems: [],
    originalName: '', // basename of the opened file, for default export names

    // The parsed document and its server metadata. `docMeta.id` is what P04's captured
    // pinning sends, so a view holding its own docMeta is what makes an operation
    // started in one view addressable to that view once several exist.
    pdfDocument: null,
    docMeta: { canSave: false, path: '' },

    // The overlay widgets rendered over this document's pages. Per view for D4's
    // reason — an overlay's value lives in its DOM element — and read on the ONE
    // genuinely frequent path this feature has: relayoutOverlays runs on scroll and
    // zoom. It walks `view.overlayFields`, which is the active view's and only the
    // active view's. A version iterating every open document would turn that path into
    // an N x regression on the thing a user feels most (ADR-002's hot-path constraint).
    overlayFields: [],

    // The armed-tool flags and the geometry they capture, per document for the same reason
    // the bulk bindings are.
    //
    // The justification first written here was checked and was mostly wrong, so it is
    // corrected rather than quietly dropped: it claimed `sbPage`, `splitRects`, `cropPage`,
    // `cropRect` and `selectedPages` were all read after an await. Only `splitRects` was —
    // `sbPage` is captured before the await beside it, the crop pair is read synchronously
    // before its first await, and every selection read happens at click time. The one true
    // case is now pinned at its operation entry.
    //
    // What actually makes them per-view is simpler and holds for all of them: they are one
    // document's state reaching shared toolbar buttons, a shared cursor, and — for
    // `selectedPages` — a destructive bulk operation.
    //
    // Enumerated here because the phase's own binding count never was: it counted
    // bindings with many references, and the question activation asks is which bindings
    // must SWAP. Those are different sets, and this is the difference.
    redactMode: false,
    editMode: false,
    markerMode: null,   // 'sign' | 'date' | 'initial' while armed to place
    activeMarker: null, // the marker highlighted as the current fill target
    fillTarget: null,   // a sign/initial marker awaiting a Library pick
    activeTool: null,   // mirrors viewer.annotationEditorMode, which is per viewer
    splitBoxMode: false,
    splitRects: [],     // {fx, fy, fw, fh} fractions of the page (top-left origin)
    sbPage: 0,
    cropMode: false,
    cropRect: null,     // {fx, fy, fw, fh} fraction of the page (top-left origin)
    cropPage: 0,
    borderMode: false,
    dropdownMode: false,
    radioMode: false,
    shapeMode: false,
    noteMode: false,

    // SAFETY — the client overlay-edit undo/redo stacks. Its entries are CLOSURES over
    // overlay elements, and undoAny drains this stack before falling through to the server
    // ring — so one shared stack means Ctrl+Z in document A pops a command recorded against
    // document B and mutates B's element, from a keystroke aimed at the one on screen.
    // Irreversible and silent: the redactMarks family, not the stale-label family.
    //
    // The fourteenth binding. The phase-open enumeration counted thirteen because it
    // counted bindings with many references, and this one has few — which is why it took a
    // deepdive of a different slice to find it.
    overlayHistory: { undo: [], redo: [] },

    // dirty — has this document changed since it was opened or last SAVED. The four
    // signals it replaces (annotationStorage.size, overlayFields.length, the overlay
    // undo depth, docMeta.canUndo) answered "since it was opened", and NONE of them is
    // cleared by a save: a fill with no overlays keeps the same annotationStorage
    // entries, and the server's handleSave deliberately leaves the undo ring alone. So
    // the confirm fired on every close after every save, and a prompt that always fires
    // is one the user learns to dismiss — it stops protecting the close where it
    // mattered. See hasUnsavedWork for how it is set and cleared.
    dirty: false,

    // SAFETY — 1-based page numbers in THIS document's pagination, driving the bulk
    // rotate / delete / reorder bar. Shared across views it applies one document's page
    // numbers to another's pages, which is a destructive wrong-document operation and not
    // a stale label. It appears in no enumeration this plan ever made.
    selectedPages: new Set(),
    selAnchor: null,    // last clicked page, for shift-range selection

    // The signing-flag state of THIS document. `docHadFlags` decides whether the Flags
    // panel treats the document as a counterparty's copy, so a shared one either exposes a
    // received document as editable or refuses to strip NibFlags from the other's export.
    docHadFlags: false,
    signTotal: 0,       // flag count when the banner appeared, for "X of N"
    signStarted: false, // the recipient has hit Start, so the action is now "Next field"

    // Captured at the moment this view is switched away from, and read when it is switched
    // back. All three start undefined DELIBERATELY: activateView skips the scroll and page
    // restore on a view that has never been switched away from, and treats an undefined
    // renderedDpr as "never rendered at a known dpr", forcing one refresh on first show.
    scrollTop: undefined,
    pageNumber: undefined,
    renderedDpr: undefined,

    // The document id this view has already reported an eviction for, so the notice
    // fires once per eviction rather than on every reflectUndoControls call — which
    // runs on every load, every op and every activation.
    lastEvictionSeen: undefined,

    // This view's own DOM and its own pdf.js engine (ADR-002). `container` is the scroll
    // box; the page stack inside it is pdf.js's to own (it empties it on setDocument) and
    // is reachable as `viewer.viewer`, so it is not duplicated here.
    //
    // `thumbGrid` and `outlineList` are the sidebars' half of the same decision. The phase
    // chose per-view containers over rebuild-on-activation because rebuilding is what
    // ADR-002 exists to avoid — so a hidden document keeps its rendered thumbnails and its
    // outline, and a switch is a `hidden` toggle rather than an N-page re-render.
    container: null,
    thumbGrid: null,
    outlineList: null,
    viewer: null,
    eventBus: null,
    linkService: null,
    findController: null,
  };

  // The DOM, built VISIBLE and only then hidden by the caller if this is not the
  // active view. That order is load-bearing: pdf.js's "container must be absolutely
  // positioned" check is guarded by `container.offsetParent` (pdf_viewer.mjs:8000),
  // which is null while the element is display:none — so constructing into a hidden
  // container SKIPS the check rather than satisfying it, and it never re-runs. A
  // mistake in .viewerContainer's CSS would then throw only for the first visible
  // view and stay silent for every other one.
  //
  // Inserted before #empty so the launch message paints over an empty view rather
  // than under it.
  v.container = document.createElement('div');
  v.container.className = 'viewerContainer';
  // A unique id per view, which is what lets the tab strip's aria-controls name the
  // region each tab switches to. NOT the static #viewerContainer P05.S03 deleted —
  // that one was a single id for a thing there can be several of; this is one id each.
  v.container.id = 'viewerContainer-' + (++viewSeq);
  v.container.setAttribute('role', 'tabpanel');
  const pagesEl = document.createElement('div');
  pagesEl.className = 'pdfViewer viewerPages';
  v.container.appendChild(pagesEl);
  const isFirstView = els.viewerWrap.querySelector('.viewerContainer') === null;
  els.viewerWrap.insertBefore(v.container, els.empty);

  // The sidebars' per-view halves. Both go INSIDE the shared panels rather than replacing
  // them: `#thumbs` keeps the append-PDF row and the selection bar, which are chrome for
  // whichever document is active, and `#outline` has to stay one unique id because the tab
  // machinery resolves panels by getElementById and `#outline a` styles the links as a
  // descendant selector. Neither new class carries a `display` rule, so the `hidden`
  // attribute works — the same trap style.css already documents for the selection bar.
  v.thumbGrid = document.createElement('div');
  v.thumbGrid.className = 'thumbgrid';
  els.thumbs.appendChild(v.thumbGrid);
  v.outlineList = document.createElement('div');
  v.outlineList.className = 'outlinelist';
  els.outline.appendChild(v.outlineList);

  // Each of these four is per view because the vendored pdf.js forces it, not for
  // tidiness — verified at the line during P05.S03's deepdive:
  //   * PDFViewer's constructor registers 'thumbnailrendered' on the bus it is handed
  //     (pdf_viewer.mjs:8065), so N viewers on one bus index _pages[] from each other's
  //     events and cleanup() the wrong document's page.
  //   * The same constructor MUTATES the find controller — onIsPageVisible (:8012) is
  //     one slot, last writer wins.
  //   * PDFFindController registers find/findbarclose/pagesedited on the bus (:927), so
  //     one dispatch('find') makes every open document search at once and race to answer
  //     the single counter.
  //   * PDFLinkService holds one pdfViewer field and setViewer is 1:1 (:1582), so
  //     outline clicks would drive the last-constructed viewer.
  // From here on the view owns three attached nodes, and the PDFViewer constructor below
  // can throw — its "container must be absolutely positioned" check is the reason the
  // container is built visible in the first place. Without this, a throw leaves an orphan
  // .viewerContainer inserted before #empty: transparent, inset:0, never hidden, sitting on
  // top of the active document and swallowing its pointer events. The catch removes what
  // this function appended and re-throws, so the caller's own failure path still runs.
  try {
  v.eventBus = new EventBus();
  v.linkService = new PDFLinkService({ eventBus: v.eventBus });
  v.findController = new PDFFindController({ eventBus: v.eventBus, linkService: v.linkService });
  v.viewer = new PDFViewer({
    container: v.container,
    viewer: pagesEl, // explicit, so the page stack needs no id (pdf_viewer.mjs:7995)
    eventBus: v.eventBus,
    linkService: v.linkService,
    findController: v.findController,
    l10n: new GenericL10n('en-US'),
    annotationMode: pdfjsLib.AnnotationMode.ENABLE_FORMS, // render fillable fields
    imageResourcesPath: './vendor/pdfjs/images/', // sticky-note icons resolve here (see annotation-note.svg)
  });
  v.linkService.setViewer(v.viewer);

  // One global that N viewers share, deliberately left alone: each PDFViewer's own
  // ResizeObserver writes --viewer-container-height onto document.documentElement
  // (pdf_viewer.mjs:9590), so a hidden view's 0px clobbers the active view's value. Inert
  // in Nib — the only consumers are .dummyPage, created solely in presentation/spread
  // mode (:8779), and a pdf.js sidebar we do not render — and _resetView pins
  // ScrollMode.VERTICAL (:8733). It goes live the day spread or presentation mode does.

  // Hidden AFTER construction, not before, and the order is the whole point: pdf.js's
  // "container must be absolutely positioned" check is guarded by `container.offsetParent`
  // (pdf_viewer.mjs:8000), which is null while the element is display:none. Constructing
  // into an already-hidden container SKIPS that check rather than satisfying it, and it
  // never re-runs — so a mistake in .viewerContainer's CSS would throw for the first
  // visible view and stay silent for every later one. Hiding here means every view's
  // geometry is validated by pdf.js, whichever order they are created in.
  //
  // Only the construction-time half lives here. SWITCHING (and the re-fit a hidden view
  // needs, because its container reports clientWidth 0) is P05.S04's. Hidden, never
  // destroyed: the page DOM stays, which is the whole of ADR-002.
  v.container.hidden = !isFirstView;
  v.thumbGrid.hidden = !isFirstView;
  v.outlineList.hidden = !isFirstView;
  } catch (e) {
    v.container.remove(); v.thumbGrid.remove(); v.outlineList.remove();
    throw e;
  }

  // The bus is per view, so these registrations are per view too. Two kinds, and the
  // difference is the whole point:
  //
  //   * Handlers that act on THIS view's own DOM take `v` and must never read the
  //     module-level `view` — a background viewer firing while another is active would
  //     otherwise pair this view's page geometry with the active view's field list, and
  //     relayoutOverlays would move the active document's overlay elements into a
  //     background page div.
  //   * Handlers that write SHARED chrome — the page-number inputs and the find counter
  //     — bail unless this view is the active one. (The thumbnail highlight was in this
  //     list until P05.S05 gave each view its own grid; it is now the view's own DOM.) A
  //     background document finishing its load must not repaint the foreground's.
  v.eventBus.on('pagesinit', () => {
    // On a HIDDEN view this computes a negative scale — 'page-width' routes to
    // (container.clientWidth - 40) / width, and clientWidth is 0 while display:none. It is
    // left unclamped deliberately: P05.S04's re-fit on activation overwrites it, and
    // papering over it here with a fallback width would hide the very measurement problem
    // S04 exists to solve. Named because this is the FIRST place it bites, before
    // fitWidestWidth's own guard.
    v.viewer.currentScaleValue = 'page-width'; // immediate fit (page 1) so there's no 100%-then-fit flash…
    scaleFrom(v, 'pagesinit');
  });
  // …then refine to the widest page once every page view is populated and the layout
  // has settled (pagesinit is too early — see fitWidestWidth) — UNLESS the user has
  // already chosen a scale. The refine used to be unconditional, and "once every page
  // view is populated" is late: on a long document a zoom made while it was still
  // loading was overwritten the instant it finished. The same silent loss of the user's
  // zoom that P06's exit criterion names at the switch, through the load door instead.
  v.eventBus.on('pagesloaded', () => { if (!v.userScale) fitWidestWidth(v, 'pagesloaded'); });
  v.eventBus.on('pagechanging', (e) => {
    // The thumbnail highlight CHANGED CATEGORY in P05.S05. With a shared grid it was
    // shared chrome and belonged behind the gate; with a per-view grid it is this view's
    // own DOM, so it runs ungated — otherwise a background document's page changes never
    // reach its own grid and switching to it shows `.current` on a stale page.
    markCurrentThumb(v, e.pageNumber);
    if (v !== view) return;
    all('.pageNum').forEach((i) => { i.value = e.pageNumber; }); // shared chrome, still gated
  });
  v.eventBus.on('pagerendered', () => relayoutRedactMarks(v));
  v.eventBus.on('scalechanging', () => relayoutRedactMarks(v));
  v.eventBus.on('scalechanging', () => relayoutOverlays(v));
  v.eventBus.on('pagerendered', () => relayoutOverlays(v));
  v.eventBus.on('updatefindcontrolstate', ({ matchesCount }) => {
    if (v === view) renderFindCount(matchesCount);
  });
  v.eventBus.on('updatefindmatchescount', ({ matchesCount }) => {
    if (v === view) renderFindCount(matchesCount);
  });

  return v;
}

// The active view. Reassigned, never mutated wholesale, so a captured `view` reference
// stays valid for the document it was captured from — the same property P04's captured
// document id has, for the same reason.
let view = newView();

// Every open view, in creation order. The ACTIVE one is `view`; the rest are hidden with
// their page DOM intact (ADR-002). Since P06.S01 a user Open adds here too, not only an
// arrival, and the tab strip below makes the result reachable.
const views = [view];

// addView / removeView / resetViews are the ONLY places `views` changes, and every one of
// them re-renders the strip.
//
// The alternative — call syncTabs() after each mutation — is a hand-maintained list of
// obligations, and this repo has now been bitten four separate times by exactly that
// shape: a keep-list that omitted `radio`, a route inventory reconciled against nothing, a
// scanner whose population was never probed, a pinning guard that policed one of a law's
// two halves. The rule that survives is the server's, one layer over: registerLocked is
// "the ONE place an id is issued … a second issuer would defeat the law by collision".
// Same argument, same shape — a second mutator leaves the strip describing a set of
// documents that is not the set the app holds, and nothing would say so.
//
// A source guard in view.test.mjs refuses `views.push`, `views.splice` and `views.length =`
// anywhere outside these three, so a fourth mutator fails a test rather than a user.
function addView(v) {
  views.push(v);
  syncTabs();
  return v;
}

// removeView drops one view if it is present, and is a NO-OP when it is not. Both halves
// are load-bearing: the arrival/open failure path calls it on a view a concurrent Close may
// already have dropped, and the bare `views.splice(views.indexOf(v), 1)` it replaces would
// have spliced at -1 on a miss — removing the LAST element, which is the live active view.
function removeView(v) {
  const i = views.indexOf(v);
  if (i < 0) return false;
  views.splice(i, 1);
  syncTabs();
  return true;
}

// resetViews collapses to the single view given — what a Close ALL leaves behind. The
// caller has already torn down the others' DOM; this is the bookkeeping half.
function resetViews(keep) {
  views.length = 0;
  views.push(keep);
  syncTabs();
}

// syncTabs renders the document switcher from `views`. Called by the three mutators
// above, by activateView (the active marker moves), and at the end of a load (the name
// arrives with the document, not with the view).
//
// **Rebuilt wholesale rather than patched.** Eight buttons is nothing to rebuild, and a
// diffing render is a second model of the same list — the failure mode being a strip that
// describes a set of documents the app does not hold, which is the whole thing the single
// mutator seam exists to prevent. Cheap and total beats clever and drifting.
//
// The strip is hidden below two documents: the logged phase-open default, so a
// single-document session is chrome-identical to what it was before tabs.
function syncTabs() {
  const strip = els.tabstrip;
  if (!strip) return; // the jsdom harness boots the real index.html, so this is defensive only
  // Which tab had focus, so a rebuild does not throw a keyboard user back to the body.
  // Rebuilding wholesale is the right call for eight buttons — see above — but it
  // destroys the focused element, and "the control you were on vanished" is a real
  // regression for anyone not using a mouse, not a cosmetic one.
  const focusedIndex = [...strip.children].indexOf(document.activeElement);
  const several = views.length > 1;
  // The two close controls track the same threshold as the strip. With one document
  // open, "Close view" and "Close all" name the same act, so the app shows one button
  // reading "Close" — chrome-identical to before tabs, which is the whole point of the
  // appear-at-two rule.
  els.closeBtn.textContent = several ? 'Close view' : 'Close';
  els.closeBtn.title = several
    ? 'Close this document and switch to the next one'
    : 'Close this document and return to the empty state (Nib keeps running)';
  els.closeAllBtn.hidden = !several;
  strip.hidden = !several;
  strip.textContent = '';
  if (!several) return;
  for (const v of views) {
    // A DIV with role="tab", not a <button>. The close affordance is a real <button>
    // inside it, and a button inside a button is invalid HTML that browsers reparent —
    // which would split each tab into two siblings and take the strip's positional
    // addressing with it. role="tab" on a div plus tabIndex is the valid shape, and it
    // keeps the close control separately focusable instead of buried inside the tab.
    const b = document.createElement('div');
    b.className = 'tab' + (v === view ? ' active' : '');
    b.tabIndex = 0;
    b.setAttribute('role', 'tab');
    // Names the region this tab switches to. A tablist whose tabs control nothing is
    // ARIA that announces a widget and then cannot describe it — worse than plain
    // buttons, because the promise is louder.
    if (v.container.id) b.setAttribute('aria-controls', v.container.id);
    // aria-selected, not colour alone: the active tab differs from the rest by two
    // greys and an accent bar, neither of which a screen reader can report.
    b.setAttribute('aria-selected', v === view ? 'true' : 'false');
    const name = document.createElement('span');
    name.className = 'tabname';
    // The same fallback the rest of the app uses for a path-less document. `docMeta.name`
    // is "." for every upload, combine, office conversion and arrival — filepath.Base("")
    // — which is why originalName is the field to read and why it is guarded.
    name.textContent = v.originalName || 'Untitled';
    b.appendChild(name);
    b.title = v.docMeta && v.docMeta.path ? v.docMeta.path : (v.originalName || 'Untitled');
    b.onclick = () => activateView(v);
    // A div is not a button, so it does not activate on Enter/Space for free.
    b.onkeydown = (e) => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); activateView(v); }
    };

    const x = document.createElement('button');
    x.type = 'button';
    x.className = 'tabclose';
    x.setAttribute('aria-label', 'Close ' + (v.originalName || 'this document'));
    x.textContent = '×';
    // stopPropagation, or closing a tab is also a click on the tab: the switch would
    // run first and the close would then act on a view the user never meant to leave.
    // Same shape as the flag ×, which plants a flag instead of deleting one because its
    // pointerdown reaches the placement handler — a standing item on the pending list.
    const closeThis = (e) => { e.stopPropagation(); closeView(v); };
    x.onclick = closeThis;
    b.appendChild(x);
    strip.appendChild(b);
  }
  if (focusedIndex >= 0) strip.children[Math.min(focusedIndex, strip.children.length - 1)]?.focus();
}

// Modals whose contents are derived from ONE document: page ranges bounded by its
// numPages, element references into its page DOM, bytes rendered from it, an outline read
// out of it. A switch invalidates all of them.
//
// The ones deliberately NOT here are the session- and app-scoped ones — about, background,
// combine, cosign peer picker, keys, open, peers, profile, the signature-capture pad, and
// the three session modals — plus `saveAsModal`, which is the interesting exclusion: its
// bytes are already in hand and its filename was captured at operation entry, so it writes
// to a path and not to a document. Leaving it open across a switch is correct.
const DOC_BOUND_MODALS = [
  'attachmentsModal', 'bookmarkSplitModal', 'cropModal', 'decryptModal', 'encryptModal',
  'extractModal', 'fieldNameModal', 'fillCsvModal', 'finalizeModal', 'importXfdfModal',
  'nupModal', 'outlineModal', 'pageLabelsModal', 'pageNumModal', 'pageSplitModal',
  'pdfaModal', 'redactTextModal', 'reduceModal', 'scanModal', 'sigDetailsModal',
  'splitModal', 'timestampModal', 'tsVerifyModal',
];

// --- switching between views -------------------------------------------------
//
// The order below is not stylistic. Each step is pinned by something that breaks if it
// moves, and the ones that bite silently are called out where they sit.
//
// What must NEVER appear in here: `setDocument`, `setDocumentFromServer`,
// `resetSharedDocState`, `clearOverlays`, `clearOverlayHistory`, `reconstructFlags`,
// `showSignBanner`, `updateBadge(null)`, `closeDocument`, `loadingTask.destroy()` or
// `newView()`. Every one of them tears down or rebuilds state that ADR-002 exists to
// preserve — `setDocument` alone runs `_resetView()`, which empties the page stack and
// takes every overlay element and its typed value with it. Activation is a REPAINT.
function activateView(v) {
  if (v === view) return;
  const out = view;

  // ── A: quiesce the outgoing view ────────────────────────────────────────────
  // Drags first, and before the hide: the preview nodes live in the outgoing view's page
  // divs, and a pointerup arriving after the swap would write the drag into whichever
  // document is active by then.
  abortDrags();
  // An armed fill target is transient the same way a half-drawn box is: `fillMarker`
  // parks a flag awaiting the NEXT Library click, and that click now belongs to another
  // document. Disarmed rather than threaded — the user has not chosen an image yet, so
  // there is nothing in flight to finish, and threading it would fill a flag on a
  // document that is not on screen with no sign that anything happened. `activeMarker`
  // deliberately stays: it is where the user had got to in the signing walk, which is
  // document progress rather than a pending gesture.
  out.fillTarget = null;
  // Captured before hiding, because a display:none element reports scrollTop 0.
  out.scrollTop = out.container.scrollTop;
  out.pageNumber = out.viewer.currentPageNumber;
  closeDocBoundModals();
  out.container.hidden = true;
  out.thumbGrid.hidden = true;
  out.outlineList.hidden = true;

  // ── B: the swap ─────────────────────────────────────────────────────────────
  // One assignment, and everything downstream depends on it having happened: eleven
  // repaint functions resolve the module-level `view` internally.
  view = v;

  // ── C: geometry ─────────────────────────────────────────────────────────────
  // Unhide BEFORE the fit, or clientWidth is 0 and fitWidestWidth silently no-ops.
  v.container.hidden = false;
  v.thumbGrid.hidden = false;
  v.outlineList.hidden = false;
  // **Only a view that never got a scale**, which is the difference between rescuing a
  // hidden load and destroying the user's zoom.
  //
  // This was unconditional, and unconditional is a defect the phase's own exit criterion
  // names: "switching preserves scroll, zoom, page". A user who zoomed to 200%, switched
  // away and switched back was handed fit-width, silently, every time. Nothing else
  // depended on the re-fit happening on every activation — the resize listener heals dpr
  // and does not re-fit at all — so the narrow version loses nothing.
  // `userScale` covers the second door onto the same defect: a view whose fit no-opped
  // (activated before any page view was populated, so `maxW` was 0) still reads
  // hasScale false, and re-fitting it on the NEXT activation would discard a zoom the
  // user has set in the meantime.
  if (!v.hasScale && !v.userScale) fitWidestWidth(v, 'activateView');
  // Explicitly, not via the bus: a re-fit that computes the SAME scale fires no
  // `scalechanging`, so the self-serving path cannot be relied on to have run.
  relayoutOverlays(v);
  relayoutRedactMarks(v);
  // Per-view dpr. `lastDpr` is one module global and dprChanged refreshes only the active
  // viewer, so a dpr change while this view was hidden was recorded and never delivered —
  // its canvases would stay at the old resolution, CSS-stretched, permanently soft.
  if (v.renderedDpr !== devicePixelRatio) {
    v.renderedDpr = devicePixelRatio;
    v.viewer.refresh();
  }
  // display:none drops scrollTop, so without this the view returns to page 1. ADR-002's
  // "preservation is the browser's default" covers DOM content; scroll offset is ours.
  if (v.pageNumber) v.viewer.currentPageNumber = v.pageNumber;
  if (v.scrollTop) v.container.scrollTop = v.scrollTop;
  applyHighlightColor(selectedHlColor); // the colour is session state; the dispatch is per bus

  // ── D: repaint the shared chrome ────────────────────────────────────────────
  repaintForActiveView();
  syncTabs(); // the active marker moves with the view
}

// activeGestures holds one `end` per pointer-capture gesture currently in flight —
// the stamp and marker drags, which are the only gestures whose state lives in a
// closure rather than in the module-level drag vars below.
//
// They need a registry because `setPointerCapture` is released when the element leaves
// the DOM, NOT when it is hidden: an inactive view is `display:none` and still in the
// document, so its stamp keeps receiving `pointermove` after a switch. Unregistered,
// that continuation lays the field out against whichever document is active — reading
// the new view's page geometry and pushing the move onto the new view's history. There
// is no cancel hook on a captured pointer, so each gesture registers its own.
const activeGestures = new Set();
function cancelGestures() {
  for (const end of [...activeGestures]) end(); // each `end` removes itself
  activeGestures.clear(); // belt and braces: an `end` that threw would otherwise leak
}

// abortDrags cancels any gesture in flight and removes its preview node. Transient state
// is aborted, never restored: a half-drawn box belongs to a moment, not to a document.
function abortDrags() {
  cancelGestures();
  for (const d of [redDiv, sbDiv, cropDiv, edDiv, bdDiv, ddDiv, rdDiv, shCanvas]) d?.remove();
  redStart = redDiv = redHit = null;
  sbStart = sbDiv = sbHit = null;
  cropStart = cropDiv = cropHit = null;
  edStart = edDiv = edHit = null;
  bdStart = bdDiv = bdHit = null;
  ddStart = ddDiv = ddHit = null;
  rdStart = rdDiv = rdHit = null;
  shStart = shCanvas = shHit = null;
  onThumbDragEnd(); // also restores the grid's DOM order
}

// Modals holding state derived from ONE document. Left open across a switch they would
// act on the new document with the old one's numbers — page ranges bounded by the old
// numPages, element references into the old view's page DOM, bytes from the old document.
// Closing them is one rule for twenty bindings, and it is the honest one: their state
// cannot be meaningfully carried.
function closeDocBoundModals() {
  els.compareModal.hidden = true;
  closeCmpDoc();
  for (const id of DOC_BOUND_MODALS) { const m = $(id); if (m) m.hidden = true; }
}

// repaintForActiveView drives every shared element from the active view. Ordering:
// updateBadge AFTER the swap (its first statement WRITES view.lastSig, so called earlier
// it would overwrite the outgoing view's — one of the three SAFETY fields); docHadFlags is
// already on the record so reflectSignControls reads the right one; and applySignLock
// AFTER setDocControls, because EDITING_TOOLS is a strict subset of DOC_REQUIRED and the
// latter re-enables everything the former just disabled.
function repaintForActiveView() {
  const open = !!view.pdfDocument;
  els.viewerWrap.classList.toggle('has-doc', open);
  els.saveBtn.disabled = !open;
  els.saveBtn.title = open
    ? (view.docMeta.canSave ? 'Save (overwrites ' + view.docMeta.path + ')' : 'Save a copy (downloads — opened without a local path)')
    : 'Save (overwrites the original)';
  all('.pageCount').forEach((s) => { s.textContent = '/ ' + (open ? view.pdfDocument.numPages : 0); });
  all('.pageNum').forEach((i) => { i.value = open ? view.viewer.currentPageNumber : 1; });
  updateBadge(view.lastSig, view.inCeremony); // idempotent re-assignment; NEVER updateBadge(null) here

  // The sidebars are NOT rebuilt. P05.S05 gave each view its own grid and outline list, so
  // activation SHOWS them — which is the phase-open decision, taken because
  // rebuild-on-activation is "precisely what ADR-002 exists to avoid". The snapshot-and-
  // restore of selectedPages that used to sit here went with the rebuild: it existed only
  // because buildThumbnails calls clearSelection on the view it is building.
  markSelectedThumbs(); // repaint the shared selection bar from the incoming view

  // The search box and counter are one shared pair for N documents. Cleared rather than
  // restored: leaving A's query over B's count is the wrong-document display this whole
  // phase is about, and per-view query restore is a feature P06 can decide on.
  const si = findInput(); if (si) si.value = '';
  renderFindCount(null);

  setDocControls(open);
  applySignLock();

  paintStale();
  els.signBanner.hidden = !view.signTotal;
  if (view.signTotal) setSignBanner(); // NOT showSignBanner — that resets a recipient's progress

  reflectRedact(); reflectEdit(); reflectSplitBox(); reflectCrop();
  reflectBorder(); reflectDropdown(); reflectRadio(); reflectShape(); reflectNote();
  all('.markers button').forEach((b) => b.classList.toggle('active', b.dataset.marker === view.markerMode));
  all('#toolbar [data-mode]:not(.cmmode)').forEach((t) => t.classList.toggle('active', t.dataset.mode === view.activeTool));
  reflectAnnoControls();
  els.viewerWrap.style.cursor = anyToolArmed() ? 'crosshair' : '';
}

function anyToolArmed() {
  return !!(view.redactMode || view.editMode || view.markerMode || view.splitBoxMode
    || view.cropMode || view.borderMode || view.dropdownMode || view.radioMode
    || view.shapeMode || view.noteMode);
}


// Full-rewrite edits (sanitise, flatten, redact, remove-originals, page ops) drop
// any signature the document carries. These guards keep one from being destroyed
// unawares: signatureWarning is a caveat appended to an action's existing confirm,
// and confirmSignatureLoss is the standalone prompt for actions that don't already
// confirm. Both are silent on an unsigned document, so ordinary edits stay
// frictionless; after the first such edit the result is unsigned, so they don't
// nag on a doc that no longer has a signature to lose.
function isSigned() {
  return !!(view.lastSig && view.lastSig.state && view.lastSig.state !== 'unsigned');
}
function signatureWarning() {
  return isSigned() ? '\n\nThis will also invalidate the document’s existing signature.' : '';
}
function confirmSignatureLoss() {
  return !isSigned() || confirm('This document is signed. Editing it will invalidate the existing signature. Continue?');
}

// --- open / load -------------------------------------------------------------
// `target` is the view the document loads INTO — the active one unless a caller says
// otherwise. An arrival passes the freshly-built hidden view, which is the one path that
// loads a document into a view the user is not looking at. Everything that writes shared
// chrome below is therefore guarded on `target === view`: a background load must not
// repaint the foreground's Save label, page count, badge or banner.
// markStale records that a view's pixels no longer match the document its metadata names,
// and paints it if that view is the one on screen.
//
// **Only when there is something stale to look at.** A load that fails with no previous
// document leaves an empty view, and telling that user they are seeing an old version
// would be a lie — the toast the caller already raised is the whole story there.
function markStale(target, why) {
  if (!target.pdfDocument) return;
  target.stale = why + ' You are looking at the previous version; the server has already moved on.';
  paintStale();
}

// paintStale renders the ACTIVE view's staleness. Shared chrome, so it reads `view` and
// never its caller's target: a background load that failed must not put its banner over
// the document the user is reading. Called from repaintForActiveView too, so switching
// tabs shows each document's own answer.
// DISK_CHANGED is the whole message, and every clause of it is chosen for being true in
// every case it can fire (/pending 333):
//
//   - No attribution. Not "another program", not "someone else" — it may well have been
//     the user's own `nib … -w` in a terminal. Nib cannot know which, and naming a
//     culprit it cannot identify is the false-statement shape this repo keeps finding.
//   - Not "newer". mtime can go backwards, and restoring a backup over the file makes
//     "newer" false while "changed" stays true.
//   - The second sentence is the one that matters: it names what saving would cost,
//     while the user is reading the banner rather than after the dialog she dismissed.
//   - No jargon. She has one machine and no IT, and must never be asked to reason about
//     a cache or an in-memory copy to understand her own document.
const DISK_CHANGED = 'This file has changed on disk since Nib opened it. You are still looking at the copy Nib opened, and saving would replace the changed file with it.';

function paintStale() {
  // A render failure OUTRANKS a disk change: a document that cannot be displayed at all
  // is the more urgent fact, and it is also the one the retry button belongs to.
  const failed = !!view.stale;
  const changed = !failed && !!view.diskChanged;
  els.staleBanner.hidden = !failed && !changed;
  els.staleMsg.textContent = failed ? view.stale : (changed ? DISK_CHANGED : '');
  // The two conditions do not share a button. Retry re-runs the same fetch, which for a
  // disk change would re-render the SAME in-memory bytes and then clear the banner on
  // success — the warning would vanish while the file was still different, at exactly
  // the moment before the user saves. A green lie is worse than no button.
  els.staleRetry.hidden = !failed;
  els.staleReload.hidden = !changed;
  // Both banners occupy the same spot over the document. When both are up, drop this one
  // below the signing one rather than letting z-order decide which fact goes unread.
  els.staleBanner.classList.toggle('stacked', !!view.signTotal);
}

// recheckDisk re-asks the server whether the ACTIVE document's file still matches.
//
// **The check has to run on return-to-foreground, and that is not a refinement.** The
// file changes while Nib is in the background by construction — the user is in a
// terminal running `nib … -w`, or in another application — so a check that only runs
// when a document loads tells her nothing until her next operation. In the case this
// was filed for (/pending 333) her next operation would have been the Save that
// destroyed the newer file.
//
// Event-driven, not polled: there is no timer here and none is wanted. The app's only
// interval work is the co-sign session poll, and a second recurring request against
// every open document to answer a question whose answer is almost always "no" would be
// a poll added to a local-first app for a background condition.
async function recheckDisk() {
  const target = view;
  const d = target.docMeta;
  if (!d || !d.id || !d.canSave) return; // a path-less document has no file to differ from
  let meta;
  try {
    const res = await apiFetch('/api/doc', { docId: d.id });
    if (!res.ok) return; // a 409 is already handled by apiFetch's reconcile hook
    meta = await res.json();
  } catch { return; } // the server is unreachable; the banner is not the place to say so
  if (!meta || meta.error || meta.id !== d.id) return;
  // The view may have been switched, closed or reloaded while the request was in
  // flight. Assigning then would put one document's answer onto another — the same
  // pinning rule the operation paths follow, applied to a background refresh.
  if (target.docMeta !== d) return;
  target.diskChanged = !!meta.diskChanged;
  if (target === view) paintStale();
}

async function setDocumentFromServer(meta, target = view) {
  // A refusal body is not a document. The server answers a stale id with
  // `{"error": "..."}` and a 200 body has no `error` field, so this is exact rather
  // than heuristic.
  //
  // The guard lives HERE, not at the fetch, because this is the one place where the
  // mistake does damage: assigning a refusal to view.docMeta leaves view.docMeta.id undefined,
  // so every later request silently stops sending the header and the session reverts
  // to the unpinned path P03.S03 exists to close — while the document still renders,
  // because /api/pdf with no id falls back to the active one. Silent in both
  // directions. Any of the twenty document-route call sites could grow the same
  // missing ok-check that openArrivalInNewView had; one guard at the sink covers them all.
  if (!meta || meta.error) {
    toast('could not read the document');
    console.error('setDocumentFromServer got a refusal, not a document:', meta);
    return;
  }
  const gen = ++target.docGen;
  // Every arrival through this sink is a server-side change to the document — the
  // twenty page operations, undo, redo, OCR, sanitize, decrypt, flatten. Set HERE, at
  // the one place all of them pass through, rather than at each caller: the asymmetry
  // is what makes that the right choice. A missed *set* under-reports and loses the
  // user's work silently; a missed *clear* only prompts when nothing would be lost.
  // The fresh-open path clears it again in installOpened, which is the single funnel
  // for open / open-url / upload / combine / office and the boot restore.
  target.dirty = true;
  // A different document in the same view needs its own fit: page sizes differ, so the
  // outgoing document's scale is not this one's.
  target.hasScale = false;
  // **Only when a DIFFERENT document lands here.** A reload of the same document — every
  // page operation, undo, redo, OCR, and the boot restore re-adopting what is already open
  // — is not a new document and must not throw away the scale the user chose for it.
  //
  // This is the door the flake instrument named. `scaleFrom` recorded `pagesloaded` as the
  // path that re-fitted a view the user had zoomed, which can only happen with `userScale`
  // false; clearing it unconditionally here is what made it false, and the refine that
  // follows every load then had nothing stopping it. Rotating a page should not reset your
  // zoom either, so this is the right behaviour on its own merits and not only for the test.
  if (!target.docMeta || target.docMeta.id !== meta.id) {
    setUserScale(target, false, 'load:' + (target.docMeta && target.docMeta.id ? 'newdoc' : 'firstdoc'));
  }
  target.docMeta = meta;
  // No `!== '.'` guard any more: the server emits an EMPTY name for a path-less
  // document rather than filepath.Base("")'s ".", so the falsy check is the whole test.
  // The sentinel and the compensation were two bugs holding each other up.
  if (meta.name) target.originalName = meta.name;
  resetSharedDocState(target); // was clearOverlays() — see the four modes it never covered
  let doc;
  try {
    // The id goes in the URL, not a header: pdf.js issues this fetch itself, so a
    // header would mean opting into its httpHeaders plumbing to gain a uniformity
    // nobody reads (D15). The URL already carries a cache-buster; the id joins it.
    const docParam = meta.id ? '&doc=' + encodeURIComponent(meta.id) : '';
    doc = await pdfjsLib.getDocument({ ...PDFJS_OPTS, url: '/api/pdf?t=' + Date.now() + docParam }).promise;
  } catch (e) {
    // An encrypted PDF needs its open password before pdf.js can render it; prompt
    // for it and unlock the working copy rather than dead-end on a generic error.
    // The decrypt prompt is one shared modal and its Unlock handler reloads the ACTIVE
    // view, so offering it for a background load would put the password the user typed for
    // an arrival onto the document they are looking at. A background load simply fails.
    if (e && e.name === 'PasswordException') {
      if (target === view) openDecryptPrompt();
      else toast('the document that arrived is password-protected — open it to unlock');
      markStale(target, 'This document is encrypted and could not be displayed.');
      return;
    }
    toast('could not render the document');
    console.error('pdf load failed', e);
    markStale(target, 'This document could not be displayed.');
    return;
  }
  // NOT marked stale: a newer load is already in flight for this view and owns what it
  // shows. Marking here would leave a banner the successful load then has to clear, and
  // the window in between is exactly when the user is looking.
  if (gen !== target.docGen) return; // a newer load superseded this one
  target.stale = ''; // this render IS the document `docMeta` names
  // Recorded on the TARGET, not painted directly, for the reason the block above gives:
  // a background load must not put its banner over the document the user is reading.
  // paintStale reads the active view, and repaintForActiveView brings it forward when
  // the user switches to this tab.
  target.diskChanged = !!meta.diskChanged;
  paintStale();

  const old = target.pdfDocument;
  target.pdfDocument = doc;
  // pdf.js's own change detection, and it is exact where a size comparison is not:
  // AnnotationStorage.setValue compares each property and fires this only when a value
  // actually changed, so editing an existing field's text counts while a no-op write
  // does not. Re-installed per document because the storage belongs to the document.
  if (doc.annotationStorage) doc.annotationStorage.onSetModified = () => { target.dirty = true; };
  target.viewer.setDocument(target.pdfDocument);
  target.linkService.setDocument(target.pdfDocument, null);
  // Free the superseded document's worker-side resources once the viewer and
  // link service have been repointed — without this, every edit op (each of
  // which reloads through here) orphans a document for the session's lifetime.
  if (old && old !== doc) old.loadingTask.destroy().catch(() => {});
  // A fresh document gets a new pdf.js editor manager; re-assert the chosen
  // highlight color so it sticks across reloads (page ops) until the user changes it.
  applyHighlightColor(selectedHlColor, target);

  // Everything from here to the sidebars is SHARED chrome: one element for N documents.
  // A background load records its state on the target and paints nothing; activateView is
  // what makes the chrome show a view, and it is the only thing that should.
  if (target === view) {
    els.viewerWrap.classList.add('has-doc');
    all('.pageCount').forEach((s) => { s.textContent = '/ ' + target.pdfDocument.numPages; });
    els.saveBtn.disabled = false;
    setDocControls(true);
    els.saveBtn.title = meta.canSave ? 'Save (overwrites ' + meta.path + ')' : 'Save a copy (downloads — opened without a local path)';
  }
  // updateBadge WRITES view.lastSig as its first statement, so a background load must not
  // call it — that would overwrite the ACTIVE view's signature result, which is the trust
  // decision the details modal shows. The target's own value is recorded directly instead.
  if (target === view) {
    updateBadge(meta.signature, meta.inCeremony);
  } else {
    target.lastSig = meta.signature;
    target.inCeremony = !!meta.inCeremony;
  }
  // Rebuild any embedded signing flags as markers and offer the signing flow.
  target.docHadFlags = Array.isArray(meta.flags) && meta.flags.length > 0;
  target.signLocked = target.docHadFlags; // a received signing document opens locked, non-editable
  if (target === view) els.signBanner.hidden = true;
  reconstructFlags(meta.flags, target);
  if (target === view) applySignLock();
  // Sidebars are non-essential; a build failure must not break the load. Ungated since
  // P05.S05: each view owns its grid and list, so a background load renders into its own
  // and is ready the moment the user switches to it.
  buildThumbnails(gen, target).catch((e) => console.error('thumbnails failed', e));
  buildOutline(gen, target).catch((e) => console.error('outline failed', e));
  // The tab's label comes from the DOCUMENT, which only exists now — addView ran before
  // this load and rendered a tab with whatever name the view had then (none, for a fresh
  // one). Re-rendered here rather than pre-seeded, so the strip cannot show a stale name
  // after a reload replaces a document in place.
  syncTabs();
}

// closeDocument puts the open document down and returns the app to exactly the
// state it launches in — the client half of POST /api/close (v1.102.6).
//
// Its caller is requestClose, which is bound to els.closeBtn and runs the
// unsaved-work confirm first. (This comment used to say it had NO caller "yet" and
// that its behaviour was therefore verified through the open path rather than by
// driving a Close — every clause of which became false when P01.S04 landed the
// control, and a reader trusting it would not go looking for the close-all
// behaviour that makes editedViews necessary.)
//
// Order is load-bearing in two places. view.pdfDocument is nulled FIRST, before
// anything that can throw, so a failure part-way through cannot leave a
// half-closed state that poisons the next open. And applySignLock reads
// !!view.pdfDocument, so it must run after that null for the whole sign surface to
// drive itself closed.
//
// view.docGen is bumped so in-flight sidebar builds bail — necessary, but not
// sufficient on its own: see the generation re-check in buildThumbnails, which
// would otherwise append into the grid this function just cleared.
function closeDocument() {
  // Close is CLOSE ALL, and it is that way because the server says so: handleClose calls
  // setDoc(nil), which empties the whole registry and clears every open document's undo
  // rings. An arrival can already leave two documents open, so a client that dropped only
  // the active view would leave the other one showing a document the server no longer
  // holds — every pinned request against it a 409, with the page still rendered.
  //
  // Matching the server is honest rather than a regression: Close has always meant "close
  // everything". P06 splits Close view from Close all, and that is where the server needs
  // a per-document remove to split against.
  // Through tearDownView, not a second copy of it. This loop WAS that copy — nine lines
  // duplicating the helper P06.S02 added a hundred lines below, which is the duplicate
  // derivation this repo keeps paying for: a fix to one teardown silently leaves the
  // other behind. Iterated over a copy, because tearDownView removes from `views`.
  for (const v of [...views]) if (v !== view) tearDownView(v);
  resetViews(view); // `views` is already down to the active view; this makes that explicit and re-renders

  // The same quiescing a switch does, for the same reason: a gesture in flight holds live
  // nodes that setDocument(null) is about to detach, and a document-bound modal outlives
  // the document it describes.
  abortDrags();
  closeDocBoundModals();

  const doc = view.pdfDocument;
  view.pdfDocument = null;
  view.docGen++;

  els.compareModal.hidden = true;
  closeCmpDoc();
  resetSharedDocState();

  // pdf.js's own teardown: it cancels rendering, resets the view (emptying the
  // page DOM), and drops the find controller and the annotation editor manager.
  view.viewer.setDocument(null);
  view.linkService.setDocument(null, null);
  if (doc) doc.loadingTask.destroy().catch(() => {});

  view.docMeta = { canSave: false, path: '' };
  view.originalName = '';
  view.docHadFlags = false;
  view.signLocked = false;
  els.signBanner.hidden = true;
  updateBadge(null); // resets view.lastSig, view.inCeremony, the badge, and the details button together

  // Back to the launch markup, not merely to something empty (index.html).
  els.viewerWrap.classList.remove('has-doc');
  els.saveBtn.disabled = true;
  els.saveBtn.title = 'Save (overwrites the original)';
  all('.pageCount').forEach((s) => { s.textContent = '/ 0'; });
  all('.pageNum').forEach((i) => { i.value = 1; });
  view.thumbGrid.innerHTML = '';
  clearSelection();
  view.outlineList.innerHTML = '';

  applySignLock();
  setDocControls(false);
}

// hasUnsavedWork reports whether this document has changed since it was opened or last
// SAVED — which is what the close confirm has always claimed to ask and, until v1.108.7,
// could not answer.
//
// It used to read four signals directly: annotationStorage.size, overlayFields.length,
// the overlay undo depth, and docMeta.canUndo. Every one of them survives a save. save()
// reloads only when there are overlay fields, so an AcroForm fill with no overlays keeps
// the same annotationStorage entries; and the server's handleSave never touches the undo
// ring, because undo-after-save is deliberate. So the confirm fired on EVERY close after
// EVERY save. The error direction was safe, but the cost was not cosmetic: a prompt that
// always fires is one the user learns to dismiss, so it stops protecting the close where
// it mattered.
//
// `dirty` is set in four places and cleared in two, and the split is chosen so a mistake
// falls on the safe side:
//   set — setDocumentFromServer (the sink every server-side change passes through),
//         recordOverlayEdit (the funnel for every recorded overlay add/delete/move),
//         makeField and convertFieldToFlag (the two overlay paths that deliberately
//         record no command), and pdf.js's own annotationStorage.onSetModified.
//   cleared — installOpened, the single funnel for every fresh open (to openedDirty(meta),
//         not to false: see there), and a successful in-place save.
// A missed set loses work silently; a missed clear only prompts when nothing is at stake.
//
// One thing it deliberately does NOT clear: "Save a copy" on a path-less document, which
// downloads rather than writing a file Nib can see. Nib cannot know the download landed,
// so that close still prompts.
function hasUnsavedWork(v = view) { return !!v.dirty; }

// editedViews is what the confirm has to ask, because Close is CLOSE ALL:
// closeDocument tears down every entry in `views`, while the prompt used to inspect
// only the active one. With a second document open — which a co-signature arrival
// creates today — the other document's typed overlays, its overlay undo stack and
// its server history were discarded with NO prompt at all, and the prompt that did
// fire said "Close this document?".
function editedViews() { return views.filter((v) => v.pdfDocument && hasUnsavedWork(v)); }

// tearDownView drops one view's DOM and pdf.js document. The bookkeeping half —
// removing it from `views` — is removeView's, so the strip re-renders exactly once.
//
// Deliberately NOT closeDocument: that one returns the whole app to the launch state,
// which is right when the last document goes and wrong when six others are still open.
function tearDownView(v) {
  const doc = v.pdfDocument;
  v.pdfDocument = null;
  v.docGen++; // an in-flight thumbnail or outline build bails rather than painting into a removed grid
  v.viewer.setDocument(null);
  v.linkService.setDocument(null, null);
  if (doc) doc.loadingTask.destroy().catch(() => {});
  v.container.remove();
  v.thumbGrid.remove();
  v.outlineList.remove();
  removeView(v);
}

// closeView closes ONE document — the active one — and moves to the neighbour the
// SERVER names. Closing the last document is a close-all and goes the other way.
//
// **Server first, and then follow its answer rather than computing one.** Which
// document is active after a close is the server's to decide (removeDoc), because two
// derivations of that rule diverge silently: the strip would highlight one document
// while the unpinned fallback on /api/pdf served another.
//
// **A 409 still removes the tab.** It means the server has already dropped this
// document — a concurrent close, or a close-all from another pane. Leaving the tab
// would make it permanently unclosable, since every future close of it 409s too.
// closeView closes ONE document — `owner`, defaulting to the active one.
//
// **Closing a background tab does not switch to it first**, and the first draft did.
// Activating and then closing was one code path instead of two and it silently changed
// the behaviour: closing tab 1 while you are reading tab 3 would move you to tab 1 and
// then to whatever the neighbour logic chose, so the document you were reading is not
// the one you end up on. It also made the × 's stopPropagation unobservable — the
// bubbled click activated the same view the handler was about to activate anyway — so a
// red-proof of that guard came back green, which is how the whole thing was caught.
async function closeView(owner = view) {
  if (!owner.pdfDocument) return;
  // The last document: Close view and Close all are the same act, and the app owes the
  // launch state rather than an emptied view record sitting in the strip.
  if (views.length === 1) return requestClose();
  if (hasUnsavedWork(owner)) {
    const name = owner.originalName || 'this document';
    if (!confirm(`Close ${name}? Any unsaved changes will be lost.`)) return;
  }
  const doc = owner.docMeta;
  const res = await apiFetch('/api/close-view', { method: 'POST', docId: doc && doc.id });
  if (!res.ok && res.status !== 409) return toast(await errText(res, 'could not close the document'));
  const next = res.ok ? await res.json() : null;

  // Switch FIRST, tear down second, and never assign `view` here — all three were wrong
  // in the first draft.
  //
  // Assigning `view = target` before calling activateView makes activateView return at
  // its own `if (v === view)` guard, so the swap happens and the shared chrome never
  // repaints: the page count, the save title and the badge go on describing the document
  // that was just closed. Tearing down before switching hands activateView an outgoing
  // view whose container it has already removed, so it quiesces a dead record. And
  // `view` is assigned in exactly one place in this file, which is the single-seam rule
  // `views` got in S01.
  //
  // Only when the closed document WAS the active one: the server leaves its active id
  // alone when an inactive document is closed (removeDoc), so a client that switched
  // anyway would be the half of the pair that disagrees. Follow the server's answer,
  // falling back to any remaining view when it names a document this client has no view
  // for.
  if (owner === view) {
    const named = next && next.id
      ? views.find((v) => v !== owner && v.docMeta && v.docMeta.id === next.id)
      : null;
    const target = named || views.find((v) => v !== owner);
    if (target) activateView(target);
  }
  tearDownView(owner); // removeView re-renders the strip
}

// requestClose is the Close control: confirm if anything has been edited, drop the
// document server-side, and only then tear the client down.
//
// Server first, deliberately. Tearing the client down before the route answers
// would leave a window where the UI shows the empty state while the server still
// holds the document — and /api/pdf would still serve it. The route is idempotent
// and has no failure mode of its own, so ordering it first costs nothing.
async function requestClose() {
  if (!view.pdfDocument) return;
  // The wording tracks what the app can now actually answer. It read "any edits made
  // since the last save" while the signals could only support "since it was opened" —
  // hedged, because a save cleared none of them. With a real dirty flag the honest word
  // is "unsaved", and the count of documents going away is unchanged.
  const edited = editedViews();
  if (edited.length) {
    const many = views.length > 1;
    const what = !many ? 'Close this document?'
      : edited.length === views.length
        ? `Close all ${views.length} open documents?`
        : `Close all ${views.length} open documents? ${edited.length} of them have edits.`;
    if (!confirm(`${what} Any unsaved changes will be lost.`)) return;
  }
  const res = await apiFetch('/api/close', { method: 'POST' });
  if (!res.ok) return toast(await errText(res, 'could not close the document'));
  closeDocument();
}
els.repointGo.onclick = () => repointKey();
// Clicking the indicator reopens the dialog it belongs to — an indicator that only
// informs leaves the user knowing something is armed and with no way to reach it.
els.armedPill.onclick = () => { els.sessionRecvModal.hidden = false; };
els.sessionNoticeDismiss.onclick = () => { els.sessionNotice.hidden = true; };
// The recovery action. It saves the ACTIVE document, which is the one the arrival opened —
// `openArrivalInNewView` puts it in its own view and activates it, and the notice is raised on
// the same poll. Deliberately the ordinary Save-As flow rather than a bespoke route: the bytes
// are already here, and a second download path for the same document would be a second thing to
// keep correct.
els.sessionNoticeAction.onclick = async () => {
  const d = view && view.docMeta;
  if (!d || !d.id) { toast('No document is open to save'); return; }
  // **Both the id and the NAME are captured here, at operation entry** (ADR-001). The first
  // cut called `exportBase()` after the await, and tier 2's pinning guard went red on it: an
  // export that names its file when the fetch RESOLVES names it for whatever document is
  // current then, which for a rescue is the one case where the user has two documents open and
  // is about to lose one of them.
  const exportName = exportBase();
  const res = await apiFetch('/api/pdf?id=' + encodeURIComponent(d.id), { docId: d.id });
  if (!res.ok) { toast('Could not read the document to save it'); return; }
  openSaveAs(await res.blob(), (exportName || 'document') + '-cosigned.pdf', 'Save a copy');
};
els.closeBtn.onclick = () => closeView();
els.closeAllBtn.onclick = requestClose;

// installOpened lands a just-opened document in a view — a NEW one, unless the app is
// still showing the empty state.
//
// The exception is what stops the strip growing an empty tab: at launch and after a
// Close, `views` is one view with no document in it, and the first Open belongs there.
// The condition is deliberately the stricter `views.length === 1 && !view.pdfDocument`
// rather than `!view.pdfDocument` alone: an empty ACTIVE view alongside a loaded one is
// not reachable today, and if it ever becomes reachable, adding a view is harmless while
// reusing one would strand whatever the other view holds.
//
// Since P06.S01 the server ADDS on every one of these routes, so a client that kept
// re-pointing the active view would leave the previous document open server-side and
// unreachable here — which is the orphaning this slice exists to end, arriving from the
// other direction.
// installOpened is also where `dirty` is cleared, and it is the right place because it
// is the single funnel every FRESH open passes through — the five document-installing
// routes, the co-signature arrival, and the boot restore. A document that has just been
// opened has no unsaved work by definition; setDocumentFromServer sets the flag for
// everything that reaches it, and this is the one caller for which that is wrong.
async function installOpened(meta) {
  if (views.length === 1 && !view.pdfDocument) {
    // Named explicitly, though `view` is also the default. The condition guarantees
    // there is exactly one view, so the default would be correct — but "every reload
    // names the view it lands in" is a law that is worth nothing if it is followed
    // except where someone judged it unnecessary, and the guard that enforces it
    // cannot read a condition three lines up.
    await setDocumentFromServer(meta, view);
    view.dirty = openedDirty(meta);
    return !!view.pdfDocument;
  }
  const opened = await openInNewView(meta);
  if (opened) {
    const v = views.find((x) => x.docMeta && meta.id && x.docMeta.id === meta.id);
    if (v) v.dirty = openedDirty(meta);
  }
  return opened;
}

// openedDirty is what a freshly installed document's `dirty` starts at, and it is NOT
// simply false. A genuine open returns canUndo false (the server gives a new document an
// empty history), so this reads false and the flag clears — which is the whole point of
// clearing here. But installOpened is ALSO the boot restore, and a browser reload throws
// away every client-side flag while the server keeps holding documents with operations
// applied and unsaved. Clearing to false there would be an UNDER-report: close after a
// reload and the page rotations go without a prompt. canUndo is the server's own answer
// to "has an operation been applied to this document", so it seeds the flag exactly.
// It over-reports for a document saved before the reload (handleSave leaves the ring
// alone, deliberately) — the safe direction, and the one this whole item trades on.
function openedDirty(meta) { return !!(meta && meta.canUndo); }

// restored guards the boot restore to once per page load. applyStatus runs again after
// an unlock and after a migration, and a second restore would re-adopt every document
// the client already has a view for.
let restored = false;

// reconcileWithServer makes the client's views match what the server actually holds.
//
// **It is the answer to two questions that turned out to be one.** At boot it is the
// restore: before P06.S03 the client asked "what is the ACTIVE document" from exactly
// one place — the co-sign arrival poll — and never asked what else was open, so a reload
// came back showing NOTHING while the server still held N, and a path-less document (an
// upload, a combine, an office conversion, an arrival) was unreachable for the rest of
// the process's life, because the only way back to a document was to open it by path.
// On a 409 it is the all-tabs-stale case from P03's plan-review pin: a server restart
// makes every id stale at once — `docFor` refuses a foreign epoch before it compares
// anything else — and the app must resolve to the launch empty state rather than to N
// tabs that each error. Same function, because "adopt what the server has, drop what it
// does not" describes both.
async function reconcileWithServer() {
  const res = await apiFetch('/api/docs', { unpinned: true });
  if (!res.ok) return false; // the server is unreachable or locked; leave the UI alone
  const { docs = [], activeId = '' } = await res.json();

  // **Empty first, then adopt, then drop — and the order is the whole correctness of
  // this function.** Dropping first was the obvious shape and it is wrong twice over.
  // With nothing held, the drop loop tears down the last view, `views` goes EMPTY and
  // `view` points at a torn-down record — and the empty-case branch then does nothing,
  // because it tests `view.pdfDocument`, which the teardown just nulled. Observed: the
  // app kept its `has-doc` chrome over no document at all. With something held but none
  // of it known to the client, the same drop leaves `view` dangling until an adoption
  // happens to replace it. Adopting and activating first means there is always a live
  // view to stand on before anything is removed.
  if (!docs.length) {
    // Nothing open server-side. closeDocument is the launch state AND re-seats `views`
    // to a single empty view, which is exactly what is owed here; a hand-rolled teardown
    // would be a second copy of it. Guarded so a boot with nothing open stays untouched.
    if (view.pdfDocument || views.length > 1) closeDocument();
    return true;
  }

  // Adopt everything the client does not already have, in the server's order.
  for (const meta of docs) {
    if (views.some((v) => v.docMeta && v.docMeta.id === meta.id)) continue;
    await installOpened(meta);
  }

  // Activate the one the server says is active, rather than whichever adoption finished
  // last — the same "follow the server, do not re-derive it" rule as the close. Done
  // BEFORE the drops, so the view being dropped is never the active one.
  const target = views.find((v) => v.docMeta && v.docMeta.id === activeId);
  if (target && target !== view) activateView(target);

  // Drop what the server no longer holds: a view for a vanished document is a tab where
  // every action 409s, and leaving it is the "N error tabs" the pin refuses.
  const held = new Set(docs.map((d) => d.id).filter(Boolean));
  for (const v of [...views]) {
    const id = v.docMeta && v.docMeta.id;
    if (v !== view && v.pdfDocument && id && !held.has(id)) tearDownView(v);
  }
  syncTabs();
  return true;
}

// openOrActivate opens a path, or brings its tab forward when it is already open.
//
// Opening the same file twice is legitimate when a user asks for it — the server allows
// it and `TestOpenAddsRatherThanReplacing` relies on it — but `?open=` after a restore is
// not that ask: it is the OS handing nib a file, and answering with a second copy of a
// document already on screen is surprising rather than helpful.
async function openOrActivate(path) {
  const already = views.find((v) => v.pdfDocument && v.docMeta && v.docMeta.path === path);
  if (already) { if (already !== view) activateView(already); return; }
  return openPath(path);
}

async function openPath(path) {
  if (!path) return;
  const res = await apiFetch('/api/open', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  });
  // errText surfaces the server's own message, which is what carries the cap refusal
  // ("too many documents open (limit 8) — close one first"). A generic fallback here
  // would tell a user at the cap that opening failed and not why.
  if (!res.ok) return toast(await errText(res, 'could not open file'));
  const meta = await res.json();
  await installOpened(meta);
  // Two tabs on one file is legitimate and deliberate — the server argues that exemption
  // at its own door — but it was SILENT: the user got two identically named tabs and
  // nothing said why (/pending 339). They then meet the consequence later, as a refusal
  // on the second save they have no way to connect to a tab opened a minute earlier, so
  // the sentence is spent naming that consequence rather than the bare fact.
  if (meta.sameFileOpen) {
    toast('That file is already open in another tab — these are two separate copies, and saving one will refuse to overwrite the other');
  }
}

async function uploadFile(file) {
  const form = new FormData();
  form.append('file', file);
  const res = await apiFetch('/api/upload', { method: 'POST', body: form });
  if (!res.ok) return toast(await errText(res, 'could not open file'));
  await installOpened(await res.json());
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
    await installOpened(await res.json());
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
  // Named before the install and applied after it, to whichever view actually took the
  // document: installOpened may build a NEW view, and this was writing the name onto
  // whatever happened to be active when the fetch returned.
  const urlName = (url.split('/').pop() || '').split('?')[0] || 'document.pdf';
  // Gated on the install having SUCCEEDED, and this is the bug the return value exists
  // to prevent rather than a defensive flourish: on a failed load `view` is still
  // whatever the user was looking at, so an ungated assignment renames THAT document
  // after a fetch that never produced one.
  if (await installOpened(await res.json())) {
    view.originalName = urlName;
    syncTabs();
  }
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
  // errText rather than a fixed string, so the cap refusal reaches the user with its
  // own wording instead of "could not combine the PDFs", which would be a lie: the
  // combine succeeded and the ninth document is what was refused.
  if (!res.ok) return toast(await errText(res, 'could not combine the PDFs'));
  await installOpened(await res.json());
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
// **Exported for tier 3 (/pending 282).** The threshold's effect is not visible at the threshold:
// what the user sees is how many pages the LCS below aligns, and a test that reimplemented this
// would be a private copy of the thing under measurement — the failure this file's compare-hash
// test records twice, once where a copy kept every assertion green against a product that had
// changed, and once where the copy AGREED with a product defect and confirmed it back.
export function alignPages(ka, kb, eq = (x, y) => x === y) {
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
// neither side has text to fingerprint. Each page is rendered small, reduced to a 9×8
// greyscale grid, and every cell compared with its right neighbour — 128 bits.
//
// ── Why a RATIO, and why three states per comparison ─────────────────────────
// This was a plain dHash — `here > right`, one bit, 64 of them — until it was measured
// (/pending 8, v1.117.137). Three things came out of that, and each shapes a line below:
//
//   * **22 of the 64 comparisons are TIES**, adjacent cells of equal brightness, because a
//     page is mostly paper. A strict `>` files every tie under "darker", so a gradient that
//     brightens left-to-right left all 22 alone (distance 0) while one that darkened flipped
//     all 22 (distance 21). The SIGN of an illumination gradient decided everything.
//   * **The margin was INVERTED**: a genuinely different page measured 10 bits away while
//     the same page under a reverse gradient measured 21, so no threshold could separate
//     them. Retuning was not available.
//   * **A blank page paired with every content page** — 6 to 11 bits — because a blank hashes
//     to all-zeros and so does every tie. A dropped or double-fed page aligned silently.
//
// So: compare in LOG space, which makes a multiplicative illumination change shift every pair
// by the same amount whatever its brightness (an absolute difference does not — measured, the
// same 10% ramp moves a dark pair by under 1 level and a bright pair by 10). Emit a TRIT —
// lighter, darker, or neither — so "tie" is its own symbol instead of being merged into
// "darker"; that is what restores discrimination. And make the band ADAPTIVE — a quantile of the
// page's own distribution rather than a constant. That last part is reasoning, not a measurement:
// a pale scan compresses every log-ratio toward zero in proportion to its contrast, so any band
// wide enough to reject noise on a normal page swallows the real edges on a faint one, and the
// page stops matching itself. What IS measured is that the adaptive band survives it — a
// 20%-contrast render of a page is 0 bits from its full-contrast render.
// The threshold is 12 of 128, MEASURED in a real browser against real renders rather than reasoned
// about (`test/ui/compare-hash.test.mjs`, which is where these fixtures live):
//
//   same page, degraded   heavy sensor noise 0 · a 20%-contrast scan 0 · a ±25% illumination
//                         gradient 0 · 1° of skew 0 · 2° 2 · 4° 12 · 6° 18 · a 2 px translation 0 ·
//                         4 px 4 · 6 px 10 · 10 px 30 · noise+skew+gradient+shift together 8
//   a different page      21 at the nearest, then 26, 26, 28, 49, 55, 72, 84
//
// Twelve is above every same-page figure up to 4° of skew and a 6 px shift, and below the nearest
// different page. Beyond that the two populations overlap and no threshold exists: 10 px of
// translation moves 30 bits, more than a different page does. That is the honest ceiling of a 9x8
// reduction, not something a constant can fix.
//
// **It is NOT tuned for pages that are mostly paper.** Two scanned text pages differing only in
// where their lines end measure 9-12 apart — astride this threshold. That limit is asserted rather
// than described, in the same file, so improving it has to come back here.
export const DHASH_T = 12; // max Hamming distance (of 128) for two pages to count as "the same page"

// DHASH_BAND_Q is where the band sits in the page's own distribution of |log ratio|. Below it
// a comparison is called a tie. It is a quantile rather than a constant so the band scales
// with the page's contrast.
const DHASH_BAND_Q = 0.30;
const DHASH_BAND_MIN = 0.02; // a floor, so a blank page does not get a band of zero

// dhashFromGrid turns 72 cell luminances (9 wide, 8 tall, row-major) into the 128-bit hash.
//
// **It is a separate, EXPORTED function so the tier-3 instrument can call the product's own
// encoder** (`test/ui/compare-hash.test.mjs`). That file used to reimplement this loop and said so:
// "the render is the app's and the reduction is this file's — the seam where a future change to
// `pageDHash` could drift from what is asserted here". The seam was real, and it was load-bearing:
// every assertion about gradients, blank pages and inverted margins would have kept passing against
// a product that no longer encoded this way. Exporting a pure function of 72 numbers is not the
// `toolbarStyle` shape this repo deleted a feature over — there is no default that can be wrong and
// no code path it adds; nothing in the product imports it.
export function dhashFromGrid(lums) {
  // The 64 log-ratios, left cell against its right neighbour. +1 keeps log finite on black.
  const ratios = [];
  for (let y = 0; y < 8; y++) {
    for (let x = 0; x < 8; x++) {
      ratios.push(Math.log(lums[y * 9 + x] + 1) - Math.log(lums[y * 9 + x + 1] + 1));
    }
  }
  const mags = ratios.map(Math.abs).sort((a, b) => a - b);
  const band = Math.max(DHASH_BAND_MIN, mags[Math.floor(mags.length * DHASH_BAND_Q)]);

  const bytes = new Uint8Array(16);
  let bit = 0;
  for (const d of ratios) {
    if (d > band) bytes[bit >> 3] |= 1 << (bit & 7);
    bit++;
    if (d < -band) bytes[bit >> 3] |= 1 << (bit & 7);
    bit++;
  }
  return bytes;
}

// gridMeans reduces a rendered page canvas to 9x8 CELL MEANS — every source pixel counted once.
//
// **The mean is computed here rather than by `drawImage(canvas, 0, 0, 9, 8)`, and that is not a
// micro-optimisation.** The old code did exactly that and its comment called it a box filter; a
// tier-3 probe printing the resulting grid showed it was nothing of the kind. Chromium's default
// `imageSmoothingQuality` on a ~150px-to-9px reduction effectively POINT-SAMPLES: a page of nine
// flat columns came back as the exact column greys with no blending at all, and a page of text
// came back as 63 cells of untouched paper and 9 cells of untouched ink. The hash was reading 72
// individual pixels. Everything a perceptual hash is for — noise, texture, halftoning, a page of
// text averaging to grey — depends on the average actually being taken.
export function gridMeans(canvas) {
  const w = canvas.width, h = canvas.height;
  const px = canvas.getContext('2d').getImageData(0, 0, w, h).data;
  const sums = new Float64Array(72), counts = new Float64Array(72);
  for (let y = 0; y < h; y++) {
    const gy = Math.min(7, (y * 8 / h) | 0);
    for (let x = 0; x < w; x++) {
      const gx = Math.min(8, (x * 9 / w) | 0), o = (y * w + x) * 4, c = gy * 9 + gx;
      sums[c] += 0.299 * px[o] + 0.587 * px[o + 1] + 0.114 * px[o + 2];
      counts[c]++;
    }
  }
  const lums = [];
  for (let i = 0; i < 72; i++) lums.push(counts[i] ? sums[i] / counts[i] : 0);
  return lums;
}

// pageDHash rasterises one page small and returns its 128-bit hash as 16 bytes.
async function pageDHash(doc, n) {
  const { canvas } = await renderPageCanvas(doc, n, 0.25); // small render; averaged down below
  return dhashFromGrid(gridMeans(canvas));
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

// hamming counts differing bits between two equal-length byte arrays. Exported for the same
// reason as dhashFromGrid: the instrument compares with the product's comparison, not a copy.
export function hamming(a, b) {
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
  // Free the comparison doc (destroy lives on the loading task, not the proxy).
  // The .catch matches the other two destroy sites: an unhandled rejection here
  // would fire during a Close, which is exactly the half-closed state to avoid.
  if (cmpDoc) { cmpDoc.loadingTask.destroy().catch(() => {}); cmpDoc = null; }
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
  const [ta, tb] = await Promise.all([pageTexts(view.pdfDocument), pageTexts(cmpDoc)]);
  if (seq !== cmpSeq) return;
  const a = pageFingerprints(ta, 'a'), b = pageFingerprints(tb, 'b');
  if (a.meaningful > 0 && b.meaningful > 0) {
    cmpAlign = detectMoves(alignPages(a.keys, b.keys), a.keys, b.keys);
  } else {
    try {
      const total = view.pdfDocument.numPages + cmpDoc.numPages;
      let done = 0;
      const tick = () => { els.compareBody.innerHTML = `<p class="scan-where">Aligning pages… ${++done}/${total}</p>`; };
      const ha = await pagePixelHashes(view.pdfDocument, tick);
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
  if (!view.pdfDocument) return toast('Open a PDF first');
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
    cmpDoc = await pdfjsLib.getDocument({ ...PDFJS_OPTS, data: buf }).promise;
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
    cmpPageA = Math.min(Math.max(cmpPageA + dir, 1), view.pdfDocument.numPages);
    cmpPageB = Math.min(Math.max(cmpPageB + dir, 1), cmpDoc.numPages);
  }
  renderCompareVisual(cmpMode);
}

function stepManual(da, db) {
  cmpPageA = Math.min(Math.max(cmpPageA + da, 1), view.pdfDocument.numPages);
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
    cmpText = { a: await documentText(view.pdfDocument), b: await documentText(cmpDoc) };
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
  const nA = view.pdfDocument.numPages, nB = cmpDoc.numPages;
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
    const ra = await renderPageCanvas(view.pdfDocument, cmpPageA, CMP_SCALE);
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
    renderPageCanvas(view.pdfDocument, cmpPageA, CMP_SCALE),
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
  if (!view.pdfDocument) return toast('Open a form first');
  els.fillCsvStatus.textContent = '';
  els.fillCsvModal.hidden = false;
}
els.fillCsvBtn.onclick = openFillCsv;
els.fillCsvClose.onclick = () => { els.fillCsvModal.hidden = true; };
els.fillCsvPick.onclick = () => els.fillCsvInput.click();
els.fillCsvInput.onchange = async () => {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
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
    openSaveAs(await res.blob(), exportName + '-filled.zip', 'Save merged PDFs (ZIP)');
  } catch (e) {
    els.fillCsvStatus.textContent = 'Merge failed — ' + e.message;
  }
};

// --- import form data (XFDF) -------------------------------------------------
// Surfaces `nib fill --data x.xfdf` in the GUI: post the open form template
// (AcroForm fields intact — bakedBytes preserves them) plus the XFDF, and the
// server fills one PDF and returns it. Reuses pdfops.FillFormXFDF unchanged.
function openImportXfdf() {
  if (!view.pdfDocument) return toast('Open a form first');
  els.importXfdfStatus.textContent = '';
  els.importXfdfModal.hidden = false;
}
els.importXfdfBtn.onclick = openImportXfdf;
els.importXfdfClose.onclick = () => { els.importXfdfModal.hidden = true; };
els.importXfdfPick.onclick = () => els.importXfdfInput.click();
els.importXfdfInput.onchange = async () => {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
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
    openSaveAs(await res.blob(), exportName + '-filled.pdf', 'Save filled PDF');
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
  if (!view.pdfDocument) return toast('Open a PDF first');
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
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
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
    openSaveAs(await res.blob(), exportName + '-pdfa.pdf', 'Save archival PDF (PDF/A-2b)');
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
  if (!view.pdfDocument) return;
  // CAPTURED, before the first await — D7, and this is the site the phase's deepdive
  // called the worst. bakedBytes() below is asynchronous and can take seconds on a
  // large document; if the open document changes while it runs, every later read of
  // `view.docMeta` describes a DIFFERENT document than the bytes came from. Concretely:
  // apiFetch stamps the current id, /api/save writes the posted bytes to the addressed
  // document's path, and A's contents land in B's file — past the signature guard,
  // with a "Saved" toast and no error anywhere.
  //
  // `doc` is used for all three of them: the id that addresses the request, the
  // canSave that decides download-vs-overwrite, and nothing read from the live
  // `view.docMeta` after this line.
  const doc = view.docMeta;
  // The view behind that meta, captured in the same breath. save() already refuses to
  // reload when the open document changed, so this is not what makes it safe — it is
  // what makes the RULE uniform: every setDocumentFromServer call names its target, so
  // the guard below can be "all of them" with one exemption class instead of a list of
  // functions each excused for its own reason.
  const owner = view;
  els.saveBtn.disabled = true;
  try {
    const bytes = await bakedBytes();
    // No local path to overwrite (drag-dropped or opened by URL) — save a copy
    // by downloading it instead. Read off the CAPTURED document: the live one may by
    // now be a different document with a different answer, and getting this wrong
    // either downloads a file the user asked to have overwritten or overwrites a file
    // the user asked to have downloaded.
    if (!doc.canSave) {
      downloadBlob(new Blob([bytes], { type: 'application/pdf' }), 'document.pdf');
      return;
    }
    let res = await apiFetch('/api/save', {
      method: 'POST',
      headers: { 'Content-Type': 'application/pdf' },
      body: bytes,
      docId: doc.id,
    });
    // 412: the file changed on disk since Nib opened it. Asked HERE, on the server's
    // answer, rather than pre-flight on the banner's flag — the flag is a snapshot from
    // the last time the client asked, and the file can move after it. The server checks
    // immediately before the write, so its refusal is the one worth acting on.
    //
    // Overwriting is offered rather than imposed or forbidden: the newer file may be
    // hers and unwanted, or it may be the only copy of something else. Only she knows,
    // so the dialog names both halves and the default on Cancel is to touch nothing.
    if (res.status === 412) {
      const name = doc.name || 'This file';
      if (!confirm(name + ' has changed on disk since Nib opened it. Overwrite it with the copy you have open? The changes on disk will be lost.')) return;
      res = await apiFetch('/api/save?overwrite=1', {
        method: 'POST',
        headers: { 'Content-Type': 'application/pdf' },
        body: bytes,
        docId: doc.id,
      });
    }
    if (!res.ok) { toast(await errText(res, 'save failed')); return; }
    const meta = await res.json();

    // Everything below reports on the SAVE, and the save was of `doc`. If the open
    // document changed while the bytes were in flight, none of it belongs on screen:
    // the badge would describe A's signature under B, and the reload would yank the
    // user back to A after they deliberately opened B.
    //
    // The save itself already succeeded and was correctly addressed — that is the
    // whole point of the captured id — so this is not an error path. It is the
    // difference between "the save happened" and "the save is what you are looking
    // at", which are separate facts the old code could not tell apart.
    // Compared by ID, not by object identity. `setDocumentFromServer` builds a fresh
    // meta object every time it runs, including when it reloads the SAME document — so
    // an identity check would report "the document changed" after any concurrent
    // refresh and silently skip a reload that was owed. The question here is which
    // document is open, and that is what the id answers.
    if (!view.docMeta || view.docMeta.id !== doc.id) { toast('Saved'); return; }

    updateBadge(meta.signature, meta.inCeremony);
    toast('Saved');
    // If detected fields were baked in, reload so the page shows the stamped
    // text and the transient input widgets are cleared. view.overlayFields is read only
    // once the document is known not to have changed, above.
    // AFTER the reload, which runs through the sink and therefore re-sets the flag.
    // Ordering it the other way round would clear the flag and then immediately dirty
    // the document again — the exact shape of bug this whole item is about.
    if (view.overlayFields.length) await setDocumentFromServer(meta, owner);
    owner.dirty = false;
  } catch (err) {
    toast('save failed: ' + err.message);
  } finally {
    els.saveBtn.disabled = !view.pdfDocument;
  }
}

// --- signature badge ---------------------------------------------------------
function updateBadge(sig, inCeremony) {
  view.lastSig = sig;
  view.inCeremony = !!inCeremony;
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
  // Details only exist for a signed document (valid or modified) — OR for a document in a
  // ceremony, which has an obliged-signer count to report before anyone has signed at all.
  els.sigDetailsBtn.hidden = !signers.length && !view.inCeremony;
}

// timeLabel turns a signer's time backing into honest plain English.
function timeLabel(s) {
  if (s.timeBacking === 'tsa') return 'Timestamped ' + s.when + ' by an independent timestamp authority';
  if (s.timeBacking === 'self-asserted') return 'Dated ' + s.when + ' by the signer\'s own device';
  return 'No signing time recorded';
}

async function openSigDetails() {
  const signers = view.lastSig?.signers || [];
  // A ceremony document with NO signatures still has something to say: how many parties are
  // obliged, and that none of them has signed. That is C18's extreme case, and until P07.S05a
  // it was unreachable — this returned early and the button was hidden besides.
  if (!signers.length && !view.inCeremony) return;
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
  if (view.lastSig?.addedAfter) {
    const note = document.createElement('div');
    note.className = 'signote';
    note.textContent = '⚠ Content was added after the last signature — it is not covered by any signature.';
    body.appendChild(note);
  }
  els.sigDetailsModal.hidden = false;
  augmentSigDetails(rows);
}

// ATTESTATION_TAG_VERSION is the attestation format version this build can read — the client half
// of `p2p.attestationTagVersion`. A signature declaring a higher one is reported as unreadable
// rather than as a party who disagreed (P07.S09c, D32).
//
// A literal here and a constant there is two copies of one number, and the guard that keeps them
// equal is `test/jsdom/tagskew.test.mjs` reading the Go source — the same shape the repo uses for
// every other cross-language constant, because the alternative is a comment asking nicely.
const ATTESTATION_TAG_VERSION = 1;

// augmentSigDetails fetches the co-signing attestations and adds, per signer that
// carries one, what they accept + whether it cross-binds to a real co-signer's
// key + whether the viewer has pinned them. The parse + cross-binding are done in
// Go (p2p); this only renders. Wording stays key-level — it confirms each party
// attests to the other's fingerprint, not a CA-vouched identity.
async function augmentSigDetails(rows) {
  let atts, body;
  try {
    const res = await apiFetch('/api/attestations');
    if (!res.ok) return;
    // The WHOLE body, not only the attestations: `obliged`/`signed` are the ceremony's
    // completeness (C16/C18) and live beside the list rather than inside it, because they are a
    // fact about the roster and not about any one signature.
    body = await res.json();
    atts = body.attestations || [];
  } catch { return; }
  const attested = [];
  atts.forEach((a, i) => {
    // **Every co-signing signature gets a row, including the one that accepts nobody
    // (P07.S07c, C09, C14).**
    //
    // This read `if (!a.acceptedPeer) return`, which is exactly the FIRST SIGNER of a ceremony:
    // `PredecessorOf` returns "" for them because there is nobody before them, which C14 as
    // amended calls out by name. So on a nine-party document the panel rendered EIGHT rows and
    // silently dropped the party who went first — the one a reader is most likely to be
    // checking. A signature carrying a roster commitment is part of the proceeding whether or
    // not it accepts a predecessor.
    if ((!a.acceptedPeer && !a.rosterHash) || !rows[i]) return;
    attested.push(a);
    const box = document.createElement('div');
    box.className = 'sigatt';
    const what = document.createElement('div');
    what.textContent = a.reason.replace(/\[SPKI:([0-9a-fA-F]{64})\]/, (_, h) => '[' + groupFingerprint(h.slice(0, 8)) + '…]');
    box.appendChild(what);
    const verdict = document.createElement('div');
    if (a.unrostered) {
      // **/pending 324. The worst rendering this panel had, because it was AFFIRMATIVE.**
      //
      // A signature carrying a copy of the document's roster token, from an identity the roster
      // does not name, took the `!acceptedPeer && rosterHash` branch below and was drawn as
      // `✓ First signer` — a green tick on an intruder, on the row a reader is most likely to be
      // checking. `Completeness` could not see it either: it counts how many of the OBLIGED
      // signed and can never exceed the roster, so the summary still read "3 of 3 ✓ Complete"
      // over a four-signature document.
      verdict.className = 'sigatt-bad';
      verdict.textContent = '⚠ Not on this ceremony\u2019s roster — this signature claims the '
        + 'ceremony and its signer is not one of the parties the record names.';
    } else if (!a.acceptedPeer && a.rosterHash) {
      // C14's own words, given a surface for the first time. This is not a failure to match —
      // it is the correct and expected state of the party who signed first, and rendering it as
      // "not a confirmed co-signer" would accuse the one signature that cannot be anything else.
      verdict.className = 'sigatt-ok';
      verdict.textContent = '✓ First signer — there was no earlier party for this signature to accept.';
    } else if (a.matched) {
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
  // Which signatures claim a ceremony. Computed here rather than below, because it now gates
  // the mutual-co-sign sentence as well as the proceeding line.
  const claimed = attested.filter((a) => a.rosterHash);
  // Unrostered signatures are counted OUT of the party count below: the chain sentence said
  // "4 parties" on a three-party roster, which is the same false statement the row made.
  const unrostered = attested.filter((a) => a.unrostered);
  // **A signature this build cannot READ is not a signature that disagrees (P07.S09c, D32).**
  //
  // `tagVersion` is the attestation format version the /Reason declares. A newer one means Go
  // deliberately parsed none of its fields, so the signature arrives with no accepted peer and no
  // roster commitment — and the "not one proceeding" line below reads exactly that state as a
  // disagreement. One counterparty on a newer Nib would therefore be told, about a document
  // everybody signed correctly, that it "was not produced by a single agreed proceeding": an
  // accusation about the parties, caused by an upgrade.
  //
  // Said FIRST and instead, on the same reasoning as the roster-version branch below: where the
  // reader cannot interpret the evidence, the honest report is that it cannot, not a verdict
  // drawn from evidence it could not read.
  const unreadable = atts.filter((a) => a.tagVersion && a.tagVersion > ATTESTATION_TAG_VERSION);
  if (unreadable.length) {
    const p = document.createElement('div');
    p.className = 'sigatt-warn';
    p.textContent = '⚠ ' + unreadable.length + ' of ' + atts.length + ' signature(s) were made by '
      + 'a newer version of Nib and this one cannot read them, so it cannot say whether every '
      + 'party agreed to the same ceremony. This is a version difference, not a disagreement — '
      + 'update Nib and check again.';
    els.sigDetailsBody.appendChild(p);
  }
  // **"each party’s signature attests to the OTHER’s key" is a two-party sentence, and it fired
  // on nine-party ceremonies (P07.S07c, C09).**
  //
  // `matched` is per-pair, so on a completed baton every signature after the first matches its
  // predecessor and the condition held — printing a sentence about "the other party" over a
  // document with nine of them. It is not merely imprecise: a reader checking a nine-party deed
  // was told the document is a mutual exchange between two people. And "mutually" is itself
  // false of a baton: party 1 accepts nobody and nobody accepts party 9.
  //
  // **The discriminator is the PARTY COUNT, not whether a record is present**, and the slice's
  // own acceptance said the latter until this was driven. A two-party ceremony IS mutual and its
  // sentence is true; `oneproceeding.test.mjs` drives exactly that document — two parties naming
  // different ceremonies — and asserts the positive survives so a disagreement is reported
  // rather than summarised away. Suppressing on the record would have deleted a true sentence to
  // fix a false one. The plan clause is pinned to C09's own text, which says nine.
  //
  // The chain branch is `>= 2`, not `> 2`, and driving it is what found that. A TWO-party
  // ceremony is a baton as well — party 0 accepts nobody — so `every(matched)` is false for it
  // and it fell through both branches, saying nothing positive at all. The mutual branch is
  // exactly the documents where "the other" is a correct word: two signatures, each matching.
  // Its condition is C14 as amended: every signature THAT HAS A PREDECESSOR is matched, the
  // first having none.
  if (attested.length === 2 && attested.every((a) => a.matched)) {
    const m = document.createElement('div');
    m.className = 'sigmutual';
    m.textContent = '✓ Mutually co-signed — each party’s signature attests to the other’s key.';
    els.sigDetailsBody.appendChild(m);
  } else if (attested.length >= 2 && unrostered.length === 0 &&
      attested.every((a) => a.matched || (!a.acceptedPeer && a.rosterHash))) {
    const m = document.createElement('div');
    m.className = 'sigmutual';
    m.textContent = '✓ Every signature after the first attests to the party before it — ' +
      attested.length + ' parties, each one bound to the one ahead of them.';
    els.sigDetailsBody.appendChild(m);
  }
  // **The unrostered summary, and it sits ABOVE the completeness line deliberately** (/pending 324).
  // "3 of 3 ✓ Complete" is TRUE of the obliged signers and says nothing about a fourth signature
  // from outside the roster; a reader who sees the tick first has already stopped reading.
  if (unrostered.length > 0) {
    const u = document.createElement('div');
    u.className = 'sigatt-bad';
    u.textContent = '⚠ ' + unrostered.length + ' signature(s) claim this ceremony from '
      + 'identities its roster does not name. The count below is of the obliged parties only, '
      + 'so it can read complete while this document carries a signature nobody agreed to.';
    els.sigDetailsBody.appendChild(u);
  }
  // Whether they all signed the SAME proceeding, which "mutually co-signed" above does
  // not answer and used to be left unsaid.
  //
  // `oneProceeding` is true when every VALID signature carries the same non-empty roster
  // commitment. Nothing read it: it was computed in Go, serialized, and rendered nowhere —
  // so the summary above announced "Mutually co-signed" over a document whose signers had
  // agreed to DIFFERENT ceremonies, which is the field's own doc describing the failure it
  // exists to prevent: "a verifier that said only co-signed about such a document would be
  // describing a proceeding that did not happen".
  //
  // **Three states, not two, and the third is why `!oneProceeding` alone is not the test.**
  // An ordinary two-party co-sign carries no record at all, so its rosterHash is "" and
  // `oneProceeding` is false — identical to the disagreement case if read naively. So the
  // discriminator is whether ANY signature claims a ceremony; only then is agreement a
  // question that has been asked.
  if (claimed.length && !unreadable.length) {
    const p = document.createElement('div');
    if (attested.every((a) => a.oneProceeding)) {
      p.className = 'sigmutual';
      p.textContent = '✓ One proceeding — every signature on this document commits to the same ceremony.';
    } else {
      // **A FORMAT SKEW is not a disagreement, and saying so is D32 (P07.S04).**
      //
      // `rosterVersion` is the record format version the commitment was computed under, and
      // `FormatVersion` is the first substantive axis of the roster preimage — so two builds at
      // different versions digest the IDENTICAL roster to different hashes. Without this branch
      // the sentence below fires, and it tells two people who agreed on everything that their
      // document "was not produced by a single agreed proceeding" because one of them updated
      // Nib. That is an accusation caused by a version difference, through the one surface D32
      // excused.
      const versions = [...new Set(claimed.map((a) => a.rosterVersion || 0))];
      p.className = 'sigatt-warn';
      if (versions.length > 1) {
        p.textContent = '⚠ These signatures were made by versions of Nib that record a ceremony '
          + 'differently (formats ' + versions.join(' and ') + '), so their commitments are not '
          + 'comparable. This is a version difference, not a disagreement — update every party '
          + 'to the same version of Nib before relying on this check.';
      } else {
        // Deliberately not phrased as tampering. The honest statement is about disagreement:
        // it covers a mixed document (one ceremony signature and one ordinary co-sign) as
        // well as two different ceremonies, and a verifier must not accuse where it can only
        // observe.
        // **The denominator is every signature on the document, not the filtered subset.**
        // `attested.length` counts only the rows this function drew, so a document with a
        // foreign approval signature on it reported "2 of 2" while carrying three — and before
        // P07.S07c it under-counted a ceremony by one, because the first signer was skipped.
        // What a reader compares this against is the signature count Go reports.
        p.textContent = '⚠ Not one proceeding — ' + claimed.length + ' of ' + atts.length +
          ' signature(s) name a ceremony, and they do not all commit to the same one. '
          + 'This document was not produced by a single agreed proceeding.';
      }
    }
    els.sigDetailsBody.appendChild(p);
  }
  // **Whether the ceremony is FINISHED, which none of the lines above answer** (C16/C18).
  //
  // "Mutually co-signed" and "one proceeding" are both true of a nine-party ceremony abandoned at
  // hop five: every signature verifies, every attestation cross-binds, and they all name the same
  // record. C18's own words — a document like that renders *untampered, 5 signers, every
  // attestation matched, one proceeding*, and no surface says four obliged parties never signed.
  //
  // **`obliged === 0` means the server could read no ceremony record**, which is an ordinary
  // two-party co-sign or a document whose record is unreadable. Saying "0 of 0 signed" about one
  // would be a verdict on a proceeding that does not exist, so the whole block is skipped — the
  // same three-state discipline the proceeding line above uses.
  //
  // A `signs:false` convener is not counted as obliged, which is C16: a ceremony they carried to
  // completion reads complete rather than short a signer.
  const obliged = Number(body.obliged || 0);
  const signedCount = Number(body.signed || 0);
  if (obliged > 0) {
    const p = document.createElement('div');
    if (signedCount >= obliged && unrostered.length === 0) {
      p.className = 'sigmutual';
      p.textContent = '✓ Complete — all ' + obliged + ' obliged signer(s) of this ceremony have signed.';
    } else if (signedCount >= obliged) {
      // Obliged-complete, but carrying a signature from off the roster: the tick would be read
      // as a verdict on the whole document, and it is not one.
      p.className = 'sigatt-warn';
      p.textContent = '⚠ All ' + obliged + ' obliged signer(s) have signed, but this document also '
        + 'carries a signature from outside the roster — see above.';
    } else {
      p.className = 'sigatt-warn';
      p.textContent = '⚠ Incomplete — ' + signedCount + ' of ' + obliged + ' obliged signer(s) '
        + 'have signed. ' + (obliged - signedCount) + ' have not, so this ceremony is not finished.';
    }
    els.sigDetailsBody.appendChild(p);
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
  if (!view.pdfDocument) return toast('Open a PDF first');
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
  // /api/sanitize resolves the addressed document and commits into it, so the id is
  // the pin for the request; `owner` is the pin for the RELOAD, which lands after the
  // round-trip and would otherwise wipe the overlays and undo stack of whichever view
  // is active by then.
  const owner = view;
  const opDoc = owner.docMeta;
  if (!owner.pdfDocument) return;
  if (!confirmSignatureLoss()) return;
  const res = await apiFetch('/api/sanitize?method=' + method, { method: 'POST', docId: opDoc && opDoc.id });
  if (!res.ok) return toast('removal failed');
  const out = await res.json();
  if (!out.ok) return toast('Could not cleanly remove it — try ' + stepDown + '.');
  await setDocumentFromServer(out, owner);
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
async function postDecrypt(password, owner = view) {
  const res = await apiFetch('/api/decrypt', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: 'password=' + encodeURIComponent(password),
    docId: owner.docMeta && owner.docMeta.id,
  });
  if (!res.ok) { toast('could not remove protection'); return null; }
  return res.json();
}

els.decryptBtn.onclick = async () => {
  const owner = view;
  if (!owner.pdfDocument) return toast('Open a PDF first');
  if (!confirmSignatureLoss()) return;
  const out = await postDecrypt('', owner); // an open doc decrypts with the empty user password
  if (!out) return;
  if (out.reason === 'plain') return toast('This document isn’t password-protected');
  if (out.reason === 'password') return openDecryptPrompt(); // shouldn't occur for an open doc
  await setDocumentFromServer(out, owner);
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
  // The prompt is only ever raised for the ACTIVE view (setDocumentFromServer refuses
  // to offer it for a background load, because the password the user types would
  // otherwise be applied to the document they are looking at), so entry is the right
  // capture point — and the reload on the far side of the round-trip still needs it.
  const owner = view;
  const out = await postDecrypt(els.decryptPw.value, owner);
  if (!out) return;
  if (out.reason === 'password') {
    els.decryptError.textContent = 'Incorrect password — try again.';
    els.decryptError.hidden = false;
    els.decryptPw.select();
    return;
  }
  els.decryptModal.hidden = true;
  await setDocumentFromServer(out.reason === 'plain' ? owner.docMeta : out, owner);
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
  if (!view.pdfDocument) return toast('Open a PDF first');
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
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
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
  openSaveAs(await res.blob(), exportName + '-protected.pdf', 'Save password-protected PDF');
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
  // Flatten rasterises every page and installs the result as the open document, so
  // both halves need the pin: the pages come from `owner`, and the reload lands on
  // `owner` — a rasterise of a 300-page scan is the longest window in the Secure tab.
  const owner = view;
  const opDoc = owner.docMeta;
  if (!owner.pdfDocument) return;
  if (!confirmSignatureLoss()) return;
  const pages = await renderFilledPages(2, undefined, undefined, undefined, owner);
  const form = new FormData();
  pages.forEach((p, i) => {
    form.append('image', p.blob, `page-${i + 1}.png`);
    form.append('pageW', String(p.w));
    form.append('pageH', String(p.h));
  });
  form.append('format', 'pdf');
  form.append('reload', '1');
  const res = await apiFetch('/api/assemble', { method: 'POST', body: form, docId: opDoc && opDoc.id });
  if (!res.ok) return toast('flatten failed');
  await setDocumentFromServer(await res.json(), owner);
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
  if (!view.pdfDocument) return toast('Open a PDF first');
  els.attachmentsModal.hidden = false;
  loadAttachments();
};
els.attachClose.onclick = () => { els.attachmentsModal.hidden = true; };
els.attachAddBtn.onclick = () => els.attachInput.click();
els.attachInput.onchange = async () => {
  const owner = view;
  const opDoc = owner.docMeta;
  const file = els.attachInput.files[0];
  els.attachInput.value = '';
  if (!file) return;
  if (!confirmSignatureLoss()) return;
  const form = new FormData();
  form.append('file', file, file.name);
  form.append('name', file.name);
  const res = await apiFetch('/api/attachments/add', { method: 'POST', body: form, docId: opDoc && opDoc.id });
  if (!res.ok) return toast('could not attach the file (a same-named attachment may already exist)');
  await setDocumentFromServer(await res.json(), owner);
  await loadAttachments();
  toast('File attached');
};

// --- thumbnails sidebar ------------------------------------------------------
// The staleness token is a (view, generation) PAIR, not a generation. `docGen` is per-view
// and every view starts at 0, so two freshly-loaded views commonly both sit at 1 and a bare
// `gen !== view.docGen` reads 1 !== 1 — false, and the stale build carries on appending.
// That is the id-reuse failure ADR-001 names: the comparison still passes. Capturing the
// owner makes the identity half explicit.
//
// It does NOT bail when the owner is inactive. S04's version did, because there was one
// shared grid and a background build would have painted into it. The grid is the owner's
// own now, so a background build renders into its own container and is ready when the user
// switches — which is the corollary of this slice's second acceptance clause, and the whole
// reason the phase chose per-view containers over rebuilding.
async function buildThumbnails(gen = view.docGen, owner = view) {
  owner.thumbGrid.innerHTML = '';
  clearSelection(owner); // a rebuild means a new/edited doc — old page numbers no longer apply
  // numPages is read ONCE, while the document is still alive. In the loop condition
  // it would be re-evaluated after every await below, against a binding a Close can
  // null. In practice the render cancellation (see the note at the append) unwinds
  // the loop before the condition is reached, so this prevents a TypeError that
  // does not currently occur — it is here so the loop does not silently depend on
  // that, not because a null deref was observed.
  const total = owner.pdfDocument.numPages;
  for (let n = 1; n <= total; n++) {
    if (gen !== owner.docGen) return; // a newer document loaded into THIS view
    const page = await owner.pdfDocument.getPage(n);
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
    canvas.onclick = (e) => onThumbClick(owner, e, n);

    const acts = document.createElement('div');
    acts.className = 'thumbacts';
    const rotL = document.createElement('button'); rotL.textContent = '↺'; rotL.title = 'Rotate left';
    // Each of these three is baked into ONE view's thumbnail and fires arbitrarily later,
    // so it captures its owner and refuses rather than acting on whatever is active. Not
    // reachable today — a hidden grid is display:none and its buttons cannot be clicked —
    // but before P05.S05 the grid was shared and always the active document's, so resolving
    // `view` was correct; per-view grids are what make that stop being true.
    rotL.onclick = (e) => { e.stopPropagation(); if (owner === view) pageOp('rotate', { pages: String(n), deg: 270 }); };
    const rot = document.createElement('button'); rot.textContent = '↻'; rot.title = 'Rotate right';
    rot.onclick = (e) => { e.stopPropagation(); if (owner === view) pageOp('rotate', { pages: String(n), deg: 90 }); };
    const del = document.createElement('button'); del.textContent = '×'; del.title = 'Delete page';
    del.onclick = (e) => { e.stopPropagation(); if (owner === view && owner.pdfDocument.numPages > 1) pageOp('delete', { pages: String(n) }); };
    acts.append(rotL, rot, del);

    const label = document.createElement('div');
    label.className = 'thumb-label';
    label.textContent = n;

    wrap.append(canvas, acts, label);
    // Belt and braces, and honest about which is which. What actually unwinds this
    // loop when the document goes away is pdf.js: tearing the document down cancels
    // the in-flight page render below, and the RenderingCancelledException it throws
    // exits the whole function through the caller's .catch — measured, not assumed
    // (a build interrupted by a Close logs exactly that). So the window between the
    // getPage await and this append is NOT reachable via a Close today.
    //
    // The check stays because that protection is a third-party guarantee this
    // function never states it depends on: the guard at the top of the loop sits on
    // the wrong side of the await, so if a pdf.js bump ever stopped cancelling
    // renders, one orphan thumbnail would land in a grid closeDocument had already
    // emptied, and nothing here would say why. One comparison to make the staleness
    // contract self-contained.
    if (gen !== owner.docGen) return;
    owner.thumbGrid.appendChild(wrap);
    await page.render({ canvasContext: canvas.getContext('2d'), viewport }).promise;
  }
  markCurrentThumb(owner, owner.viewer.currentPageNumber || 1);
}

function markCurrentThumb(owner, n) {
  owner.thumbGrid.querySelectorAll('.thumbwrap').forEach((c) => {
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

// onThumbClick handles a thumbnail click with its modifier keys: shift extends a
// range from the anchor, ctrl/cmd toggles one page, a plain click clears the
// selection and navigates (the pre-existing behaviour).
// `owner` is the view whose thumbnail was clicked, captured when the thumbnail was built.
// The selection it edits is that view's, and it is 1-based page numbers in that document's
// pagination — the SAFETY reason selectedPages went onto the record in P05.S04.
function onThumbClick(owner, e, n) {
  if (e.shiftKey && owner.selAnchor != null) {
    const lo = Math.min(owner.selAnchor, n), hi = Math.max(owner.selAnchor, n);
    owner.selectedPages.clear();
    for (let p = lo; p <= hi; p++) owner.selectedPages.add(p);
  } else if (e.ctrlKey || e.metaKey) {
    if (!owner.selectedPages.delete(n)) owner.selectedPages.add(n);
    owner.selAnchor = n;
  } else {
    owner.selectedPages.clear();
    owner.selAnchor = n;
    owner.viewer.currentPageNumber = n;
  }
  markSelectedThumbs(owner);
}

function clearSelection(owner = view) {
  owner.selectedPages.clear();
  owner.selAnchor = null;
  markSelectedThumbs(owner);
}

function markSelectedThumbs(owner = view) {
  owner.thumbGrid.querySelectorAll('.thumbwrap').forEach((c) => {
    c.classList.toggle('selected', owner.selectedPages.has(Number(c.dataset.page)));
  });
  // The selection BAR is one element for N documents — shared chrome. A background view
  // records its own selection on its own thumbnails and paints nothing here, or a document
  // nobody is looking at would set the count the user reads.
  if (owner !== view) return;
  els.thumbSelBar.hidden = owner.selectedPages.size === 0;
  els.thumbSelCount.textContent = owner.selectedPages.size + ' selected';
}

// selectedPagesParam joins the selection into the comma list pageOp/the server
// expect, in ascending page order.
function selectedPagesParam() {
  return [...view.selectedPages].sort((a, b) => a - b).join(',');
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
// Captured at dragstart, and this is the load-bearing pair. The listeners live on the
// STABLE #thumbs, so every handler after dragstart has to be told which grid — and which
// document — the gesture belongs to.
//
// Resolving them at call time instead would put the worst defect in this slice into
// onThumbDragEnd: its cancel path re-appends every snapshotted node into a grid, so a
// cancelled drag whose view is no longer active would physically move one document's
// .thumbwrap nodes INTO another's grid. The next drop there reads dataset.page off those
// nodes and fires pageOp('reorder') against the wrong document — a destructive
// wrong-document page operation that NO docGen comparison catches, because a drag never
// touches docGen. ADR-001's law, applied to a DOM node instead of a document id.
let dragGrid = null;     // the .thumbgrid this gesture started in
let dragView = null;     // the view that owns it

function onThumbDragStart(e) {
  const wrap = e.target.closest('.thumbwrap');
  if (!wrap) return;
  dragGrid = wrap.closest('.thumbgrid');
  if (!dragGrid) return;
  dragView = view;
  // Grabbing a selected thumbnail drags the whole selection; an unselected one
  // drags just itself. Read the block from the DOM .selected class (kept in sync
  // with selectedPages by markSelectedThumbs), in document order.
  const sel = [...dragGrid.querySelectorAll('.thumbwrap.selected')];
  dragBlock = wrap.classList.contains('selected') && sel.length > 1 ? sel : [wrap];
  dragOrig = [...dragGrid.children];
  dragDropped = false;
  dragBlock.forEach((w) => w.classList.add('dragging'));
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/plain', wrap.dataset.page); // payload required to drag in Firefox
}

function onThumbDragOver(e) {
  if (!dragBlock) return;
  if (!overDragGrid(e)) return;
  e.preventDefault(); // allow drop
  e.dataTransfer.dropEffect = 'move';
  const after = thumbBelow(dragGrid, e.clientY); // never a block node — all are .dragging, which thumbBelow skips
  for (const w of dragBlock) {
    if (after) dragGrid.insertBefore(w, after);
    else dragGrid.appendChild(w);
  }
}

// The origin guard, and the reason it exists is a behaviour change this slice would
// otherwise have made silently. The listeners moved from the grid to the stable #thumbs,
// whose subtree ALSO holds the append-PDF row and the selection bar — both above the grid.
// Without this, dragging a page upward and releasing on the "6 selected" bar fires a drop:
// thumbBelow returns the first thumbnail, the block splices to position 0, and a
// whole-document reorder commits. Before the re-homing that release produced no drop event
// at all and dragend restored the original order. So the guard restores "release outside
// the grid means cancel" rather than adding a new rule.
//
// Same shape as startedInActiveView for the pointer listeners, for the same reason.
function overDragGrid(e) {
  return !!dragGrid && e.target.closest('.thumbgrid') === dragGrid;
}

// thumbBelow returns the first thumbnail whose vertical midpoint is below the
// cursor (where the dragged thumbnail should be inserted before), or null for end.
function thumbBelow(grid, y) {
  return [...grid.querySelectorAll('.thumbwrap:not(.dragging)')].find((w) => {
    const r = w.getBoundingClientRect();
    return y < r.top + r.height / 2;
  }) || null;
}

function onThumbDrop(e) {
  if (!dragBlock) return;
  if (!overDragGrid(e)) return; // a release outside the grid is a cancel, as it was before
  e.preventDefault();
  // Refused rather than misapplied: pageOp resolves the module-level `view`, so a reorder
  // committed after a switch would send this document's page order to another one.
  //
  // The refusal comes BEFORE `dragDropped = true`, which is not cosmetic: dragend only
  // reverts when the drop did not commit, so setting the flag first would leave the grid
  // showing the dragged order while the server kept the original — and the NEXT drag would
  // snapshot dragOrig from that permuted baseline and reorder against an order the document
  // never had.
  if (dragView !== view) { toast('that document is no longer in front — the reorder was not applied'); return; }
  dragDropped = true;
  const order = [...dragGrid.querySelectorAll('.thumbwrap')].map((w) => w.dataset.page);
  if (order.join(',') !== dragOrig.map((w) => w.dataset.page).join(',')) {
    pageOp('reorder', { pages: order.join(',') });
  }
}

function onThumbDragEnd() {
  if (dragBlock) dragBlock.forEach((w) => w.classList.remove('dragging'));
  // Into the grid the gesture STARTED in — see the dragGrid comment above.
  if (!dragDropped && dragOrig && dragGrid) dragOrig.forEach((w) => dragGrid.appendChild(w)); // cancelled — restore order
  dragBlock = null;
  dragOrig = null;
  dragDropped = false;
  dragGrid = null;
  dragView = null;
}

// Bound to the STABLE #thumbs, not to a grid: a per-view grid would serve only the view
// that existed at module evaluation, and every document opened later would have thumbnails
// that are draggable and completely inert. Same reason the pointer listeners live on
// #viewerWrap (P05.S03) — and note the guard written for those cannot see these, because
// it keys on `addEventListener('pointer`.
els.thumbs.addEventListener('dragstart', onThumbDragStart);
els.thumbs.addEventListener('dragover', onThumbDragOver);
els.thumbs.addEventListener('drop', onThumbDrop);
els.thumbs.addEventListener('dragend', onThumbDragEnd);

// --- page operations (M7): bake edits, apply server-side, reload -------------
// Captured at entry, per ADR-001 and D7. pageOp is the single entry point for
// rotate, delete, reorder, append, insert, duplicate, split, splitrects, crop,
// pagenum, pagelabels, nup and normalize, and it used to capture NOTHING: the bake
// below is an await, apiFetch stamps view.docMeta.id as it exists AFTER it, and
// server/pages.go commits the posted bytes into whichever document that header
// names. So a switch landing during the bake committed one document's pages over
// another — past the pin, with a success toast, and with the undo history cleared
// by the commit so there was no way back.
//
// It is reachable: activateView's only call site is the co-sign arrival poll, which
// fires on a timer the user does not control, and a bake on a large document takes
// seconds. save() is the operation that already did this correctly and is the shape
// copied here.
async function pageOp(op, extra = {}) {
  const owner = view;
  const opDoc = owner.docMeta;
  if (!owner.pdfDocument) return;
  if (!confirmSignatureLoss()) return;
  const form = await bakedForm(owner);
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
  const res = await apiFetch('/api/pages', { method: 'POST', body: form, docId: opDoc && opDoc.id });
  if (!res.ok) { toast('page operation failed'); return false; }
  await setDocumentFromServer(await res.json(), owner);
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
  if (!view.pdfDocument) return;
  const page = await view.pdfDocument.getPage(view.viewer.currentPageNumber);
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
  const page = view.viewer.currentPageNumber;
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
els.selRotateLeftBtn.onclick = () => { if (view.selectedPages.size) pageOp('rotate', { pages: selectedPagesParam(), deg: 270 }); };
els.selRotateRightBtn.onclick = () => { if (view.selectedPages.size) pageOp('rotate', { pages: selectedPagesParam(), deg: 90 }); };
els.selDeleteBtn.onclick = () => {
  if (!view.selectedPages.size) return;
  if (view.pdfDocument.numPages <= view.selectedPages.size) { toast("can't delete every page"); return; }
  pageOp('delete', { pages: selectedPagesParam() });
};
els.selClearBtn.onclick = () => clearSelection();
// Move the selection to the front or back as a contiguous block, keeping the
// pages' relative order. Routes through the same reorder rail as the single-page
// drag (pageOp('reorder') → Collect), so it bakes overlays, warns on signature
// loss, and is undoable for free. A move that wouldn't change the order is a no-op.
function moveSelected(toFront) {
  if (!view.selectedPages.size) return;
  const selected = [...view.selectedPages].sort((a, b) => a - b);
  const rest = [];
  for (let p = 1; p <= view.pdfDocument.numPages; p++) if (!view.selectedPages.has(p)) rest.push(p);
  const order = toFront ? [...selected, ...rest] : [...rest, ...selected];
  const identity = Array.from({ length: view.pdfDocument.numPages }, (_, i) => i + 1);
  if (order.join(',') === identity.join(',')) return; // already at that end
  pageOp('reorder', { pages: order.join(',') });
}
els.selMoveFrontBtn.onclick = () => moveSelected(true);
els.selMoveBackBtn.onclick = () => moveSelected(false);

// Insert a blank page after the page on screen — a replace-in-place mutation, so
// it routes through pageOp like rotate/delete (the blank matches the neighbour).
els.insertBlankBtn.onclick = () => pageOp('insertblank', { page: view.viewer.currentPageNumber });

// Duplicate the page on screen — the copy lands right after it (replace-in-place
// mutation, same pageOp rail as insert-blank).
els.duplicatePageBtn.onclick = async () => {
  if (await pageOp('duplicate', { page: view.viewer.currentPageNumber })) toast('Page duplicated');
};

// Insert another PDF BEFORE the page on screen (before page 1 prepends; use
// "+ Append PDF" for the end). Reuses pageOp's `file` field (the `append` part).
els.insertPdfBtn.onclick = () => els.insertPdfInput.click();
els.insertPdfInput.onchange = async () => {
  const file = els.insertPdfInput.files[0];
  els.insertPdfInput.value = '';
  if (file && await pageOp('insertpdf', { file, page: view.viewer.currentPageNumber })) toast('PDF inserted');
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
  if (!view.pdfDocument) return;
  const n = view.pdfDocument.numPages;
  els.extractPages.value = '';
  els.extractHint.textContent = `This document has ${n} page${n === 1 ? '' : 's'}.`;
  els.extractModal.hidden = false;
  els.extractPages.focus();
}

async function extractGo() {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  const pages = parsePageRanges(els.extractPages.value, view.pdfDocument.numPages);
  if (!pages || pages.length === 0) {
    els.extractHint.textContent = 'Enter pages like "1-3, 5" within the document range.';
    return;
  }
  const form = await bakedForm();
  form.append('pages', pages.join(','));
  const res = await apiFetch('/api/extract', { method: 'POST', body: form });
  if (!res.ok) return toast('could not extract pages');
  els.extractModal.hidden = true;
  openSaveAs(await res.blob(), exportName + '-pages.pdf', 'Extract pages');
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
  const n = view.pdfDocument ? view.pdfDocument.numPages : 1;
  els.pnPreview.textContent = n === 1
    ? `1 page → “${pnFormat(start, start, n)}”`
    : `${n} pages → “${pnFormat(start, start, n)}” … “${pnFormat(start + n - 1, start, n)}”`;
}
// warnIfStamped shows a dialog's "already stamped" note, from /api/stamps.
//
// Both stamping paths bake onto whatever they are given, so running either twice puts a
// second set on top of the first — overlapping page numbers, a doubled watermark. The
// document itself is what knows: pdfcpu files every watermark it writes into an
// optional-content group, so nothing has to be remembered between sessions or kept in
// step with the undo ring.
//
// Asked HERE, when a dialog opens, and not carried on the document metadata: the answer
// costs a full PDF parse, and the metadata is rebuilt for every document route's reply.
//
// One bit, and the wording says only what the bit supports — the group does not
// distinguish page numbers from a watermark, so neither does the sentence. A failed
// probe says nothing at all rather than guessing: this warns, it does not gate, and a
// dialog that refused to open because a probe failed would be worse than the doubled
// stamp it is trying to prevent.
async function warnIfStamped(el) {
  el.hidden = true;
  try {
    const res = await apiFetch('/api/stamps');
    if (!res.ok) return;
    el.hidden = !(await res.json()).stamped;
  } catch { /* offline or refused — the dialog opens either way */ }
}

function openPageNum() {
  if (!view.pdfDocument) return;
  pnPreview();
  els.pageNumModal.hidden = false;
  warnIfStamped(els.pnStamped);
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
  const n = view.pdfDocument ? view.pdfDocument.numPages : 1;
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
  const n = view.pdfDocument ? view.pdfDocument.numPages : 1;
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
  if (!view.pdfDocument) return;
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
  const n = view.pdfDocument ? view.pdfDocument.numPages : 1;
  const last = plRanges.reduce((m, r) => Math.max(m, r.start), 0);
  plRanges.push({ start: Math.min(n, last + 1), style: 'decimal', first: 1, prefix: '' });
  renderPageLabels();
};

// --- N-up: combine several pages onto each sheet -----------------------------
// Whole-document re-imposition in place (op:'nup' on /api/pages → pdfops.NUp).
function openNup() {
  if (!view.pdfDocument) return;
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
async function buildOutline(gen = view.docGen, owner = view) {
  // Captured at entry, not read at click time. Each link's onclick escapes into the DOM
  // and fires arbitrarily later, so resolving `view` inside it would navigate whichever
  // document happened to be active then — ADR-001's law applied to a client-side
  // closure. Same shape as P04's captured document id, for the same reason.
  owner.outlineList.innerHTML = '';
  const edit = document.createElement('button');
  edit.className = 'outline-edit';
  edit.textContent = 'Edit outline…';
  // Guarded like the thumbnail buttons, and for the same reason — it is the last handler in
  // this file baked into one view's DOM that resolved the active view at click time, sitting
  // directly under the paragraph above stating that rule. openOutlineEditor reads
  // view.pdfDocument and view.outlineItems throughout.
  edit.onclick = () => { if (owner === view) openOutlineEditor(); };
  owner.outlineList.appendChild(edit);
  const outline = await owner.pdfDocument.getOutline();
  // Against the OWNER's token, not the active view's. `docGen` became per-view in S02 and
  // every view starts at 0, so comparing a token captured from A's counter against B's
  // counter is the id-reuse failure ADR-001 names: the comparison still passes, and A's
  // stale outline renders while B is on screen.
  if (gen !== owner.docGen) return; // a newer document loaded into THIS view (see buildThumbnails)
  if (!outline || !outline.length) {
    const empty = document.createElement('div');
    empty.className = 'thumb-label';
    empty.textContent = 'No outline';
    owner.outlineList.appendChild(empty);
    return;
  }
  const render = (items, depth) => {
    for (const it of items) {
      const a = document.createElement('a');
      a.textContent = it.title;
      a.style.paddingLeft = 4 + depth * 12 + 'px';
      a.onclick = () => owner.linkService.goToDestination(it.dest);
      owner.outlineList.appendChild(a);
      if (it.items?.length) render(it.items, depth + 1);
    }
  };
  render(outline, 0);
}

// --- outline editor ----------------------------------------------------------
// Author the bookmark tree: a flat, page-ordered, leveled list (indent to nest).
// Reads/writes through the server (pdfcpu), so each bookmark carries a real page
// and what's written matches what's read. Save replaces the whole outline.
function sortOutline() { view.outlineItems.sort((a, b) => a.page - b.page); }
function renderOutlineEditor() {
  sortOutline();
  const list = els.outlineEditList;
  list.innerHTML = '';
  if (!view.outlineItems.length) {
    const p = document.createElement('p');
    p.className = 'scan-empty';
    p.textContent = 'No bookmarks yet — add one.';
    list.appendChild(p);
  }
  const n = view.pdfDocument ? view.pdfDocument.numPages : 1;
  let prev = -1;
  view.outlineItems.forEach((it, i) => {
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
    del.onclick = () => { view.outlineItems.splice(i, 1); renderOutlineEditor(); };
    row.append(title, page, indent, outdent, del);
    list.appendChild(row);
  });
}
async function openOutlineEditor() {
  if (!view.pdfDocument) return;
  try {
    const res = await apiFetch('/api/outline');
    if (!res.ok) throw new Error('outline');
    view.outlineItems = ((await res.json()).items || []).map((it) => ({ title: it.title, page: it.page, level: it.level }));
  } catch { view.outlineItems = []; }
  renderOutlineEditor();
  els.outlineModal.hidden = false;
}
els.outlineCancel.onclick = () => { els.outlineModal.hidden = true; };
els.outlineAddBtn.onclick = () => {
  view.outlineItems.push({ title: 'New bookmark', page: view.viewer.currentPageNumber || 1, level: 0 });
  renderOutlineEditor();
};
els.outlineSave.onclick = async () => {
  // Captured at entry: the bake and the POST are both awaits, and the outline being
  // saved belongs to the document the editor was opened on.
  const owner = view;
  const opDoc = owner.docMeta;
  sortOutline();
  const titles = new Set();
  for (const it of view.outlineItems) {
    const t = it.title.trim();
    if (!t) return toast('Every bookmark needs a title');
    if (titles.has(t)) return toast(`Bookmark titles must be unique: “${t}”`);
    titles.add(t);
  }
  const form = await bakedForm(owner);
  form.append('outline', JSON.stringify(owner.outlineItems.map((it) => ({ title: it.title.trim(), page: it.page, level: it.level }))));
  const res = await apiFetch('/api/outline', { method: 'POST', body: form, docId: opDoc && opDoc.id });
  if (!res.ok) return toast(await errText(res, 'could not save the outline'));
  await setDocumentFromServer(await res.json(), owner);
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
  // `docGen` alone guarded the wrong hazard: it detects a RELOAD of this view, and says
  // nothing about the active view having become a different document. Two views can hold
  // the same generation number, so the check passed and the stamp was registered on the
  // active document while its element was appended into this one's page div.
  const owner = view;
  if (!owner.pdfDocument) { toast('Open a PDF first'); return; }
  const n = owner.viewer.currentPageNumber;
  const pv = owner.viewer.getPageView(n - 1);
  if (!pv?.div || !pv.viewport) { toast('Scroll the page into view, then try again'); return; }
  const base = (await owner.pdfDocument.getPage(n)).getViewport({ scale: 1 }); // PDF points
  const gen = owner.docGen;
  const img = new Image();
  img.onload = () => {
    if (gen !== owner.docGen) return; // a reload superseded this placement — pv.div is now detached
    const W = pv.div.clientWidth, H = pv.div.clientHeight;
    const aspect = (img.naturalWidth / img.naturalHeight) || 1;
    const dispW = Math.min(W * 0.3, img.naturalWidth || W * 0.3);
    const dispH = dispW / aspect;
    const x = (W - dispW) / 2, y = (H - dispH) / 2;
    const frac = [x / W, y / H, (x + dispW) / W, (y + dispH) / H];
    makeStamp(bitmapUrl, aspect, frac, { page: n, pageW: base.width, pageH: base.height }, pv, owner);
  };
  img.onerror = () => toast('could not load image');
  img.src = bitmapUrl;
}

// makeStamp registers a draggable/resizable image overlay (kind 'stamp'). The
// baking source is the library id (server resolves bytes) or an inline base64
// PNG for a generated stamp.
function makeStamp(src, aspect, frac, opts, pv, owner = view) {
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

  const remove = () => deleteField(f, true, owner);
  del.onclick = (e) => { e.stopPropagation(); remove(); };
  el.addEventListener('keydown', (e) => { if (e.key === 'Delete' || e.key === 'Backspace') remove(); });
  enableStampGestures(f, el, handle);

  owner.overlayFields.push(f);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f, owner);
}

// enableStampGestures wires move (anywhere on the stamp) and resize (the corner
// handle), updating the field's page-fraction rect. Pointer math is in page-div
// pixels; resize preserves the image aspect and everything clamps to the page.
function enableStampGestures(f, el, handle) {
  let mode = null, sx = 0, sy = 0, start = null, owner = null, pid = null;
  // `end` is BOTH the normal finish and the abort, deliberately one function: the two
  // paths cannot then drift, and abortDrags() reaches it through activeGestures. It
  // clears `mode`, which is what makes a later pointermove and a later pointerup no-ops
  // — an aborted gesture must not move the field and must not record a command.
  const end = () => {
    mode = null; start = null; owner = null;
    if (pid !== null) { try { el.releasePointerCapture(pid); } catch { /* already released */ } pid = null; }
    activeGestures.delete(end);
  };
  const begin = (m) => (e) => {
    if (m === 'drag' && e.target.closest('.stamp-resize, .stamp-del')) return;
    e.preventDefault(); e.stopPropagation();
    mode = m; sx = e.clientX; sy = e.clientY; start = f.frac.slice();
    owner = view; pid = e.pointerId;
    el.setPointerCapture(e.pointerId);
    activeGestures.add(end);
  };
  el.addEventListener('pointerdown', begin('drag'));
  handle.addEventListener('pointerdown', begin('resize'));
  el.addEventListener('pointermove', (e) => {
    if (!mode) return;
    const pv = owner.viewer.getPageView(f.page - 1);
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
  el.addEventListener('pointerup', () => {
    // Read before end() clears them; a gesture aborted by a switch has mode null here,
    // so the move is neither applied nor recorded onto the document now on screen.
    const wasMode = mode, wasStart = start, wasOwner = owner;
    end();
    if (wasMode && wasStart && f.frac.join() !== wasStart.join()) recordMove(f, wasStart.slice(), f.frac.slice(), wasOwner);
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
      if (view.fillTarget) resolveFillTarget(src); else placeStamp(src);
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
// exportBase reads `view.originalName`/`view.docMeta` at CALL time, so it must be called at an
// operation's ENTRY and its result carried — never called at the point the file is
// handed to openSaveAs.
//
// The reason is easy to miss and it is why all 19 export scopes capture into
// `exportName` first: in `openSaveAs(await res.blob(), exportBase() + '-filled.pdf')`
// the arguments evaluate left to right, so the blob resolves BEFORE exportBase runs.
// The name was therefore taken from whatever document was current once the export
// finished, not from the one the bytes came from — B's name on A's contents.
//
// Worst on the signing names (`-cosigned`, `-for-signing`), where the filename is how
// a user tells two documents apart in a workflow whose whole point is which document
// was signed.
function exportBase() {
  const b = (view.originalName || view.docMeta.name || 'document').replace(/\.[Pp][Dd][Ff]$/, '');
  return b || 'document';
}

let saveAsBlob = null; // the bytes the dialog will write on confirm

// The server names the reason a listing came back empty in one word; turning it
// into a sentence is the UI's job, the same split the decrypt dialog uses.
const LIST_REASON = {
  missing: 'That folder doesn’t exist.',
  denied: 'You don’t have permission to read that folder.',
  notdir: 'That’s a file, not a folder.',
  unreadable: 'That folder can’t be read.',
};

// browseDir drives the folder browser behind every dialog that picks a folder:
// Save-as, the two splits, and Open. t names the four elements it writes (dir
// input, here label, up button, list ul). onFile, when given, additionally
// renders the folder's PDFs as clickable rows — that's the Open dialog, the only
// one that opens a file rather than choosing a destination.
//
// Every path the list navigates or opens with comes fully built from the server,
// which is the only side that knows the separator. This used to be four dialogs
// over two near-identical browsers, both joining with "/" — which is why a
// Windows path displayed and behaved inconsistently.
const saveAsDirEls = () => ({ dir: els.saveAsDir, here: els.saveAsHere, up: els.saveAsUp, list: els.saveAsList });
async function browseDir(path, t = saveAsDirEls(), onFile = null) {
  const res = await apiFetch('/api/listdir' + (path ? '?path=' + encodeURIComponent(path) : ''));
  if (!res.ok) return toast('could not list folder');
  const info = await res.json();
  t.dir.value = info.path;
  t.here.textContent = info.path;
  t.up.disabled = !info.parent;
  t.up.dataset.parent = info.parent || '';
  t.list.innerHTML = '';
  const row = (label, cls, onclick) => {
    const li = document.createElement('li');
    li.textContent = label;
    if (cls) li.className = cls;
    if (onclick) li.onclick = onclick;
    t.list.appendChild(li);
  };
  // Say why it's empty, so an unreadable folder can't pass for an empty one.
  if (info.reason) row(LIST_REASON[info.reason] || LIST_REASON.unreadable, 'idle', null);
  // Rendered LAST would read better, but the rows below can each add one, so the count is
  // only final at the end of the function — see the tail.
  // At a filesystem root the parent walk is over. On Windows that's only the end
  // of one drive, so the server offers the others here — without this the browser
  // can never leave the drive holding the user's profile.
  for (const r of (info.roots || [])) {
    if (r !== info.path) row(r, 'root', () => browseDir(r, t, onFile));
  }
  if (onFile) for (const f of info.files) row(f.name, 'file', () => onFile(f.path));
  for (const d of info.dirs) row(d.name, null, () => browseDir(d.path, t, onFile));
  // "Nothing here", as a real row rather than a stylesheet `:empty::after`. Checked after
  // everything above, because `info.reason` and the drive roots are rows too — a folder
  // that could not be READ is not an empty one and already says so in its own words.
  if (!t.list.children.length) row('no sub-folders', 'blank', null);
}

function openSaveAs(blob, defaultName, title) {
  // One dialog, one set of bytes. `saveAsBlob` is a single module global, so a second
  // export finishing while this dialog is open used to replace both the bytes and the
  // name field under the user — including a name they had already typed. Refusing is
  // the honest answer: the cost is that the second export must be re-run, and the
  // alternative is writing bytes the user did not think they were writing.
  if (!els.saveAsModal.hidden) {
    toast('Finish the save already open first, then export again');
    return;
  }
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
  const dir = els.saveAsDir.value.trim();
  if (!name) return toast('Enter a file name');
  if (!dir) return toast('Choose a folder');
  const form = new FormData();
  // Folder and name go over separately: the server joins them, because only it
  // knows the separator, and joining there is what keeps a typed "../" from
  // escaping the folder this dialog says it's writing to.
  form.append('dir', dir);
  form.append('name', name);
  form.append('data', saveAsBlob, name);
  let res = await apiFetch('/api/write', { method: 'POST', body: form });
  // 412: something is already at that name (/pending 340). Asked here, on the server's
  // answer, rather than pre-flighted from the folder listing — the dialog does not show
  // the folder's existing PDFs at all, so the client has nothing to pre-flight against,
  // and the file can appear between a listing and the write in any case.
  //
  // Offered rather than imposed: replacing is often exactly what the user means on a
  // re-export. The wording names what is lost, and Cancel leaves the dialog open with the
  // name still typed so they can change it.
  if (res.status === 412) {
    if (!confirm(name + ' already exists in that folder. Replace it? The file there now will be lost.')) return;
    form.append('overwrite', '1');
    res = await apiFetch('/api/write', { method: 'POST', body: form });
  }
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
  if (!view.pdfDocument) return;
  const outline = await view.pdfDocument.getOutline();
  if (!outline || !outline.length) { toast('This PDF has no bookmarks to split by'); return; }
  bsOutline = outline;
  els.bsPrefix.value = '';
  updateBsPreview();
  els.bookmarkSplitModal.hidden = false;
  browseDir('', bookmarkDirEls()); // server resolves '' to ~/nib
}

async function bookmarkSplitGo() {
  const dir = els.bsDir.value.trim(); // the server Cleans it
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
  const n = view.pdfDocument ? view.pdfDocument.numPages : 0;
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
  if (!view.pdfDocument) return;
  els.psPrefix.value = '';
  updatePsPreview();
  els.pageSplitModal.hidden = false;
  browseDir('', pageSplitDirEls());
}
async function pageSplitGo() {
  const dir = els.psDir.value.trim(); // the server Cleans it
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
  if (!view.pdfDocument || !confirmSignatureLoss()) return;
  // CAPTURED before the first await (D7). OCR is the longest operation in the app —
  // loading the engine, then a recognition pass per page — so the window in which the
  // open document can change is measured in tens of seconds. The text layer is stamped
  // onto the document the words were read FROM.
  // The VIEW as well as the id. The pin already sent the words to the right
  // document; the reload afterwards still landed on whichever view was active, and
  // the page loop below still read the active viewer — so words read from one
  // document were stamped, correctly pinned, onto it, while the OTHER document's
  // view was replaced by the result.
  const owner = view;
  const doc = owner.docMeta;
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
    const n = owner.pdfDocument.numPages;
    for (let p = 1; p <= n; p++) {
      btn.textContent = `OCR ${p}/${n}…`;
      const { blob, h } = await renderPageBlob(owner.pdfDocument, p, ocrScale, null, 'image/png');
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
    const res = await apiFetch('/api/ocr', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ lang, words }), docId: doc && doc.id });
    if (!res.ok) { toast('Could not add the text layer'); return; }
    await setDocumentFromServer(await res.json(), owner);
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
// `owner` is the document the pages come FROM, and it is threaded rather than
// defaulted-and-forgotten: bakedBytes reads its overlays, stamps, covers and notes,
// so a bare bakedBytes() here rasterises whichever document is active when this
// runs. Every caller below has already awaited by the time it reaches this, which
// is exactly when "the active view" stops meaning "the document the user acted on".
async function renderFilledPages(scale, onlyPage, mime, quality, owner = view) {
  const bytes = await bakedBytes(owner.docMeta && owner.docMeta.id, owner);
  const doc = await pdfjsLib.getDocument({ ...PDFJS_OPTS, data: bytes }).promise;
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
// `docId` defaults to the document open AT ENTRY, which is a defined moment before
// any await inside this function — not "whatever is current when the request is
// built", which is the defect (D7). A caller that has already awaited before reaching
// here must pass its OWN captured id, because entry is already too late for it.
async function flattenPages(pages, paint, docId = view.docMeta && view.docMeta.id) {
  const bytes = await bakedBytes();
  // pdf.js detaches the buffer it parses, but `bytes` is also uploaded as the pdf
  // field, so render from a copy and keep the original intact for the upload.
  const doc = await pdfjsLib.getDocument({ ...PDFJS_OPTS, data: bytes.slice() }).promise;
  const form = new FormData();
  form.append('pdf', new Blob([bytes], { type: 'application/pdf' }), 'doc.pdf');
  for (const n of pages) {
    const { blob, w, h } = await renderPageBlob(doc, n, 2, paint);
    form.append('page', blob, `page-${n}.png`);
    form.append('pageNum', String(n));
    form.append('pageW', String(w));
    form.append('pageH', String(h));
  }
  return apiFetch('/api/redact', { method: 'POST', body: form, docId });
}

// assembleBlob rasterises every (filled, stamped) page and packages it server-
// side into a flattened image-PDF or a ZIP of PNGs. Returns the blob, or null on
// failure.
// `owner` defaults to the document open AT ENTRY, which is a defined moment before
// any await inside this function — not "whatever is current when the request is
// built", which is the defect (D7). A caller that has already awaited before reaching
// here must pass its OWN captured view, because entry is already too late for it.
// The owner IS the pin: the id addresses the request and the same record supplies the
// bytes. Taking a bare `docId` while renderFilledPages read the active view was the
// half-threaded shape — the request named A and carried B's pages.
async function assembleBlob(format, owner = view) {
  const docId = owner.docMeta && owner.docMeta.id;
  const pages = await renderFilledPages(2, undefined, undefined, undefined, owner);
  const form = new FormData();
  pages.forEach((p, i) => {
    form.append('image', p.blob, `page-${i + 1}.png`);
    form.append('pageW', String(p.w));
    form.append('pageH', String(p.h));
  });
  form.append('format', format);
  const res = await apiFetch('/api/assemble', { method: 'POST', body: form, docId });
  if (!res.ok) { toast('export failed'); return null; }
  return res.blob();
}

// compressBlob rasterises every page to JPEG at the given render scale (DPI) and
// quality and assembles a much smaller image-PDF — the lossy "reduce size" path.
// It flattens the document (text becomes images), so the caller warns and shows
// the before/after size before saving.
// `owner` defaults to the document open AT ENTRY, which is a defined moment before
// any await inside this function — not "whatever is current when the request is
// built", which is the defect (D7). A caller that has already awaited before reaching
// here must pass its OWN captured view, because entry is already too late for it.
async function compressBlob(scale, quality, owner = view) {
  const docId = owner.docMeta && owner.docMeta.id;
  const pages = await renderFilledPages(scale, 0, 'image/jpeg', quality, owner);
  const form = new FormData();
  pages.forEach((p, i) => {
    form.append('image', p.blob, `page-${i + 1}.jpg`);
    form.append('pageW', String(p.w));
    form.append('pageH', String(p.h));
  });
  form.append('format', 'pdf');
  const res = await apiFetch('/api/assemble', { method: 'POST', body: form, docId });
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
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  const owner = view;
  if (!owner.pdfDocument) return toast('Open a PDF first');
  const blob = await assembleBlob('pdf', owner);
  if (blob) openSaveAs(blob, exportName + '-flattened.pdf', 'Save flattened PDF');
};

// Reduce file size. Renders the result, shows before→after size, and only then
// reveals Save — so the user backs out if compress grew (or flattened) the file.
let reduceBlob = null;
// Captured alongside the blob, because the Save button is a SEPARATE handler from the one
// that produced it — `exportName` is a const in reduceGo's scope and is not in scope here.
// It read it anyway until 2026-08-16 and threw ReferenceError on every click; `node --check`
// passes on a scope error, so tier 0 could never have seen it. This is the D7 capture rule
// applied to a two-handler flow: capture at operation entry, then carry it to wherever the
// result is consumed rather than re-deriving it there.
let reduceName = '';
function resetReduce() {
  reduceBlob = null; reduceName = '';
  els.reduceResult.hidden = true; els.reduceResult.textContent = '';
  els.reduceSave.hidden = true; els.reduceGo.hidden = false;
  els.reduceModal.querySelector('input[value="optimize"]').checked = true;
  els.reduceQuality.hidden = true;
}
els.reduceBtn.onclick = () => { if (!view.pdfDocument) return toast('Open a PDF first'); resetReduce(); els.reduceModal.hidden = false; };
els.reduceCancel.onclick = () => { els.reduceModal.hidden = true; };
all('input[name="reduceMode"]').forEach((r) => {
  r.onchange = () => { els.reduceQuality.hidden = r.value !== 'compress' || !r.checked; };
});
els.reduceGo.onclick = async () => {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  // Captured HERE, not inside the helpers. This handler awaits (optimize, bakedBytes)
  // before it reaches compressBlob, so the helpers' entry-time default would already be
  // too late — entry is only a safe capture point for a helper entered before the
  // operation's first await, and this is the one caller where it is not.
  const owner = view;
  const opDoc = owner.docMeta;
  const mode = els.reduceModal.querySelector('input[name="reduceMode"]:checked').value;
  els.reduceGo.disabled = true; els.reduceGo.textContent = 'Working…';
  let blob, before, after;
  try {
    if (mode === 'optimize') {
      const res = await apiFetch('/api/optimize', { method: 'POST', docId: opDoc && opDoc.id });
      if (!res.ok) return toast('Could not optimize');
      blob = await res.blob();
      before = Number(res.headers.get('X-Original-Size')) || 0;
      after = Number(res.headers.get('X-New-Size')) || blob.size;
    } else {
      const presets = { low: [96 / 72, 0.55], med: [150 / 72, 0.7], high: [220 / 72, 0.82] };
      const [scale, q] = presets[els.reduceQ.value] || presets.med;
      before = (await bakedBytes(opDoc && opDoc.id, owner)).length;
      blob = await compressBlob(scale, q, owner);
      if (!blob) return;
      after = blob.size;
    }
  } finally { els.reduceGo.disabled = false; els.reduceGo.textContent = 'Reduce'; }
  reduceBlob = blob; reduceName = exportName;
  els.reduceResult.textContent = sizeSummary(before, after);
  els.reduceResult.hidden = false;
  els.reduceGo.hidden = true;
  els.reduceSave.hidden = false;
};
els.reduceSave.onclick = () => {
  if (!reduceBlob) return;
  els.reduceModal.hidden = true;
  openSaveAs(reduceBlob, reduceName + '-smaller.pdf', 'Save reduced PDF');
};
els.exportZipBtn.onclick = async () => {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  const owner = view;
  if (!owner.pdfDocument) return toast('Open a PDF first');
  const blob = await assembleBlob('zip', owner);
  if (blob) openSaveAs(blob, exportName + '-pages.zip', 'Export pages (ZIP)');
};

els.saveEditableBtn.onclick = async () => {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  if (!view.pdfDocument) return;
  const bytes = await bakedBytes();
  openSaveAs(new Blob([bytes], { type: 'application/pdf' }), exportName + '-editable.pdf', 'Save editable copy');
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
    const tc = await (await view.pdfDocument.getPage(n)).getTextContent();
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
  if (!view.pdfDocument) return;
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
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
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
  const saved = view.pdfDocument.annotationStorage.size > 0
    ? await view.pdfDocument.saveDocument()
    : await view.pdfDocument.getData();
  const form = new FormData();
  form.append('pdf', new Blob([saved], { type: 'application/pdf' }), 'doc.pdf');
  form.append('fields', JSON.stringify(fields));
  const res = await apiFetch('/api/form/author', { method: 'POST', body: form });
  if (!res.ok) { toast('could not create fillable form'); return; }
  openSaveAs(await res.blob(), exportName + '-fillable.pdf', 'Save fillable PDF');
};

els.exportPngBtn.onclick = async () => {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  const owner = view;
  if (!owner.pdfDocument) return;
  // The page number is read ONCE, before the await: read again on the far side it
  // would name a page of whatever document is active then, and the file would be
  // named for a page it does not contain.
  const pageNum = owner.viewer.currentPageNumber;
  const [{ blob }] = await renderFilledPages(2, pageNum, undefined, undefined, owner);
  openSaveAs(blob, exportName + '-page' + pageNum + '.png', 'Export page (PNG)');
};

els.exportTextBtn.onclick = async () => {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  if (!view.pdfDocument) return toast('Open a PDF first');
  // Best-effort dump of the document's text layer via pdf.js (see documentText:
  // content-stream order, hasEOL newlines; scanned pages have no text layer and
  // contribute nothing). Compare uses the same helper so the two stay in step.
  const out = await documentText(view.pdfDocument);
  openSaveAs(new Blob([out], { type: 'text/plain' }), exportName + '.txt', 'Export text (.txt)');
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
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  if (!view.pdfDocument) return toast('Open a PDF first');
  const page = await view.pdfDocument.getPage(view.viewer.currentPageNumber);
  const grid = await extractTable(page);
  if (!grid.length) return toast('No text on this page to extract (a scanned page? run OCR first)');
  const res = await apiFetch('/api/table?format=' + format, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ grid }),
  });
  if (!res.ok) { toast('Could not build the spreadsheet'); return; }
  const ext = { csv: '.csv', ods: '.ods', xlsx: '.xlsx' }[format] || '.xlsx';
  openSaveAs(await res.blob(), exportName + '-p' + view.viewer.currentPageNumber + '-table' + ext, 'Export table (' + format.toUpperCase() + ')');
}
els.exportTableXlsxBtn.onclick = () => exportTable('xlsx');
els.exportTableCsvBtn.onclick = () => exportTable('csv');
els.exportTableOdsBtn.onclick = () => exportTable('ods');

els.exportImagesBtn.onclick = async () => {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  if (!view.pdfDocument) return toast('Open a PDF first');
  const res = await apiFetch('/api/extract-images', { method: 'POST' });
  if (!res.ok) { toast('Could not extract images'); return; }
  if (res.headers.get('X-Image-Count') === '0') { toast('No extractable images found'); return; }
  openSaveAs(await res.blob(), exportName + '-images.zip', 'Export embedded images (ZIP)');
};

// Form-data export goes through apiFetch, not window.location.
//
// A plain navigation carries no X-Nib-Doc header, so /api/form-data resolved via the
// SERVER's active document. Nothing tells the server which view the user switched to,
// so after a co-signature arrival — which activates the new view client-side only —
// switching back and exporting silently wrote out the OTHER document's field values.
// A document-scoped route cannot be reached by navigation for that reason; the header
// is the only thing that names the document, and only apiFetch attaches it.
//
// exportBase() is captured at entry per D7, so the filename names the document the
// export came from rather than whatever is active when the bytes arrive.
async function exportFormData(format, ext) {
  if (!view.pdfDocument) return toast('Open a PDF first');
  const exportName = exportBase();
  const res = await apiFetch('/api/form-data?format=' + format);
  if (!res.ok) { toast(await errText(res, 'Could not export the form data')); return; }
  openSaveAs(await res.blob(), exportName + '-form.' + ext, 'Export form data');
}
els.exportFormJsonBtn.onclick = () => exportFormData('json', 'json');
els.exportFormCsvBtn.onclick = () => exportFormData('csv', 'csv');
els.exportFormXfdfBtn.onclick = () => exportFormData('xfdf', 'xfdf');
els.exportCertBtn.onclick = () => { window.location = '/api/identity'; };

els.printBtn.onclick = async () => {
  if (!view.pdfDocument) return toast('Open a PDF first');
  // Print the real PDF bytes (WYSIWYG, vector) through the browser's own print
  // dialog. Printing the on-screen page stack would capture pdf.js's screen-DPI
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
  if (!view.pdfDocument) return;
  await refreshSignAs();
  els.finalizeModal.hidden = false;
  warnIfStamped(els.fzStamped);
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
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
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
  openSaveAs(await res.blob(), exportName + '-finalized.pdf', 'Save finalized PDF');
};

// OpenTimestamps: stamp the current document's hash into the Bitcoin blockchain
// and save the .ots proof as a sidecar. The proof is over the same baked bytes
// Finalize signs, so it matches what the user keeps; the PDF itself is untouched.
els.timestampBtn.onclick = () => { if (view.pdfDocument) els.timestampModal.hidden = false; };
els.tsCancel.onclick = () => { els.timestampModal.hidden = true; };
els.tsGo.onclick = async () => {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  els.timestampModal.hidden = true;
  const form = await bakedForm();
  toast('Contacting OpenTimestamps calendars…');
  const res = await apiFetch('/api/timestamp', { method: 'POST', body: form });
  if (!res.ok) { toast('Could not reach an OpenTimestamps calendar'); return; }
  openSaveAs(await res.blob(), exportName + '.pdf.ots', 'Save OpenTimestamps proof');
};

// Verify an OpenTimestamps proof against the open document: pick the .ots, send it
// with the document's bytes, and report whether (and when) it's anchored to Bitcoin.
let upgradedProofB64 = null; // a now-complete .ots offered for saving after verify
let upgradedProofName = ''; // captured with it — same two-handler ReferenceError as reduceName
els.timestampVerifyBtn.onclick = () => {
  if (!view.pdfDocument) return;
  els.tvResult.hidden = true; els.tvResult.textContent = '';
  els.tvSave.hidden = true; upgradedProofB64 = null; upgradedProofName = '';
  els.tsVerifyModal.hidden = false;
};
els.tvCancel.onclick = () => { els.tsVerifyModal.hidden = true; };
els.tvSave.onclick = () => {
  if (!upgradedProofB64) return;
  openSaveAs(b64ToBlob(upgradedProofB64, 'application/octet-stream'), upgradedProofName + '.ots', 'Save complete proof');
};
els.tvExplorerOn.onchange = () => { els.tvExplorer.disabled = !els.tvExplorerOn.checked; };
els.tvPick.onclick = () => els.tvFile.click();
els.tvFile.onchange = async () => {
  // Export name captured at operation entry — see exportBase (D7). This handler awaits
  // before it produces the proof, and the Save button is a different handler again, so the
  // name is carried on upgradedProofName rather than re-derived at either later point.
  const exportName = exportBase();
  const f = els.tvFile.files[0];
  els.tvFile.value = '';
  if (!f) return;
  els.tvResult.hidden = false;
  els.tvResult.textContent = 'Checking…';
  els.tvSave.hidden = true; upgradedProofB64 = null; upgradedProofName = '';
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
    if (r.upgraded) { upgradedProofB64 = r.upgraded; upgradedProofName = exportName; els.tvSave.hidden = false; }
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
  if (!view.pdfDocument) return toast('Open a PDF first');
  const res = await apiFetch('/api/profile');
  const profile = res.ok ? await res.json() : {};
  if (!Object.keys(profile).length) return toast('No profile yet — edit it first');
  const objs = await view.pdfDocument.getFieldObjects();
  if (!objs) return toast('This PDF has no form fields');
  let count = 0;
  for (const [name, arr] of Object.entries(objs)) {
    if (profile[name] === undefined) continue;
    for (const o of arr) {
      view.pdfDocument.annotationStorage.setValue(o.id, { value: profile[name] });
      // **And the element, if one is on screen already.** pdf.js builds a widget's input
      // FROM storage at render time and never pushes a later storage change into an
      // element it has already made — and `refresh()` below does not rebuild the
      // annotation layer either, it re-runs `pageView.update()`. So the storage write
      // landed, the toast reported N fields filled, and the form the user was looking at
      // did not move. Storage stays the write of record: it is what a page not yet
      // rendered reads when it renders.
      const el = view.container.querySelector(`[data-element-id="${CSS.escape(o.id)}"]`);
      if (el && (el.tagName === 'TEXTAREA' || el.tagName === 'SELECT'
        || (el.tagName === 'INPUT' && el.type !== 'checkbox' && el.type !== 'radio'))) {
        el.value = profile[name];
      }
      count++;
    }
  }
  view.viewer.refresh?.();
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
let redStart = null, redDiv = null, redHit = null;

els.redactBtn.onclick = () => {
  view.redactMode = !view.redactMode;
  if (view.redactMode) { setMarkerMode(null); exitSplitBox(); exitBorder(); exitCrop(); exitNote(); exitDropdown(); exitRadio(); exitShape(); } // one box tool at a time
  reflectRedact();
  els.viewerWrap.style.cursor = view.redactMode ? 'crosshair' : '';
};
// Keep both the Edit-menu and toolbar redact buttons lit while redact mode is on.
function reflectRedact() {
  all('#redactBtn, [data-forward="redactBtn"]').forEach((b) => b.classList.toggle('active', view.redactMode));
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

// The ten drawing tools listen on the STABLE #viewerWrap rather than on a view's own
// container, so that one binding serves every view including ones created later
// (ADR-002). The wrap has two other children, and one of them matters: #signBanner
// floats OVER the page with z-index 6, and today — as a sibling of the container — a
// pointerdown on it does not reach these handlers at all. On the wrap it would, and
// seven of the tools (splitBox, crop, border, dropdown, radio, shape, note) are not in
// EDITING_TOOLS, so they stay armable while the banner is up: a click meant for
// "Finish & sign" would land a note or start a drag on the page beneath it, because
// pageAt hit-tests raw coordinates and knows nothing about what is painted on top.
//
// #empty needs no guard — it is pointer-events:none and display:none whenever a
// document is open — but this asks the general question rather than naming the banner,
// so a future child of the wrap is covered without anyone remembering to come back.
//
// Only pointerdown is guarded. Every pointermove/pointerup handler is already gated on
// its own drag-state variable, which only a successful pointerdown sets, so they are
// inert unless a drag is live. That also closes one narrow gap: a release over
// #signBanner used to miss the container's pointerup entirely, stranding the drag state
// non-null with an orphaned preview div. Only over the banner — the container is inset:0
// on the wrap, so the two have identical geometry, and a release over the sidebar or the
// toolbar still strands the drag exactly as before.
function startedInActiveView(e) {
  return e.target.closest('.viewerContainer') === view.container;
}

// onExistingOverlay: a pointerdown that landed inside an overlay belongs to that
// overlay's own controls — its × button, its resize handle, its text box — and not to
// whichever placement tool happens to be armed.
//
// Without it the tool acts FIRST. Placement listens on `pointerdown` on #viewerWrap
// while a delete button only calls `stopPropagation()` on its `click`, and pointerdown
// runs first and is never stopped: clicking a flag's × with the flag tool still armed
// planted a SECOND flag under the cursor and masked the delete (measured by the tier-3
// harness: 1 marker in, 2 out). The note tool has the same shape, and the drag-draw
// tools start a stray drag from the button.
//
// Same shape and the same position as startedInActiveView, in all ten pointer handlers,
// because it is the same kind of rule: whose gesture is this.
function onExistingOverlay(e) {
  return !!e.target.closest('.ovl');
}

function pageAt(x, y) {
  for (let i = 0; i < (view.pdfDocument?.numPages || 0); i++) {
    const pv = view.viewer.getPageView(i);
    const r = pv?.div?.getBoundingClientRect();
    if (r && x >= r.left && x <= r.right && y >= r.top && y <= r.bottom) {
      return { pv, n: i + 1, r: pageContentRect(pv.div) };
    }
  }
  return null;
}
// clampToRect pins a pointer position inside a page's content rect.
//
// **Every box tool measures against the page the drag STARTED on**, and nothing bounded the
// other end. Page divs are `overflow: visible` (pdf_viewer.css), and in continuous scroll
// the next page is directly below — so a drag from near the bottom of page N carried on
// into page N+1, and the preview, absolutely positioned inside page N's div, painted right
// over page N+1's content.
//
// On the redaction tool that is not a cosmetic overflow, it is the worst failure this app
// has: the user watches a black rectangle cover text on the next page, applies, and that
// text is untouched. applyRedact groups marks BY PAGE and flattens only the pages named, so
// page N+1 is never rasterised at all — its content stays selectable in the output. The
// mark is the user's only evidence at Apply time, and it was showing coverage that was
// never going to happen.
//
// Clamping rather than splitting the drag across pages: on a security path the conservative
// answer is that what you see marked is exactly what gets removed. The box visibly stops at
// the page edge, which is honest feedback, and a user who wants both pages draws two boxes.
// Splitting one gesture into per-page marks is the nicer gesture and is NOT done here —
// more moving parts on the one path where being wrong is unrecoverable.
function clampToRect(r, p) {
  return {
    x: Math.min(Math.max(p.x, r.left), r.left + r.width),
    y: Math.min(Math.max(p.y, r.top), r.top + r.height),
  };
}

// sizeMark clamps BOTH ends, so every preview drawn through it is bounded by its page —
// seven tools fixed at one site. Each tool's pointerup clamps again for the fraction it
// records: a preview that stops at the edge while the recorded box does not would be the
// same lie with an extra step.
function sizeMark(div, r, a, b) {
  a = clampToRect(r, a);
  b = clampToRect(r, b);
  div.style.left = (Math.min(a.x, b.x) - r.left) + 'px';
  div.style.top = (Math.min(a.y, b.y) - r.top) + 'px';
  div.style.width = Math.abs(b.x - a.x) + 'px';
  div.style.height = Math.abs(b.y - a.y) + 'px';
}
els.viewerWrap.addEventListener('pointerdown', (e) => {
  if (!view.redactMode) return;
  if (!startedInActiveView(e)) return;
  if (onExistingOverlay(e)) return;
  redHit = pageAt(e.clientX, e.clientY);
  if (!redHit) return;
  redStart = { x: e.clientX, y: e.clientY };
  redDiv = document.createElement('div');
  redDiv.className = 'redactmark';
  redHit.pv.div.appendChild(redDiv);
  sizeMark(redDiv, redHit.r, redStart, redStart);
  e.preventDefault();
});
els.viewerWrap.addEventListener('pointermove', (e) => {
  if (redStart) sizeMark(redDiv, redHit.r, redStart, { x: e.clientX, y: e.clientY });
});
els.viewerWrap.addEventListener('pointerup', (e) => {
  if (!redStart) return;
  const r = redHit.r;
  // Clamped to the page this drag started on — see clampToRect. Unclamped, a box dragged
  // onto the next page recorded a fraction past 1, applyRedact never flattened that page,
  // and the content the user watched go black survived intact.
  const rp = clampToRect(r, { x: e.clientX, y: e.clientY });
  const rs = clampToRect(r, redStart);
  const x0 = Math.min(rs.x, rp.x), y0 = Math.min(rs.y, rp.y);
  const fw = Math.abs(rp.x - rs.x) / r.width;
  const fh = Math.abs(rp.y - rs.y) / r.height;
  if (fw > 0.005 && fh > 0.005) {
    view.redactMarks.push({ page: redHit.n, fx: (x0 - r.left) / r.width, fy: (y0 - r.top) / r.height, fw, fh });
  } else {
    redDiv.remove();
  }
  redStart = null; redDiv = null; redHit = null;
});

els.applyRedactBtn.onclick = async () => {
  // Captured at entry: flattenPages is already pinned, but the disarm below runs after its
  // await and would otherwise clear whichever view is active by then — leaving this one
  // armed over marks already baked, where a second Apply redacts flat pages.
  const owner = view;
  if (!owner.redactMarks.length) return toast('Draw redaction boxes first');
  if (!confirm('Permanently redact the marked pages? Those pages become flat images and the content under each box is removed. This cannot be undone.' + signatureWarning())) return;

  const byPage = {};
  for (const m of owner.redactMarks) (byPage[m.page] ||= []).push(m);
  const res = await flattenPages(Object.keys(byPage).map(Number), (ctx, cv, n) => {
    ctx.fillStyle = '#000';
    for (const m of byPage[n]) ctx.fillRect(m.fx * cv.width, m.fy * cv.height, m.fw * cv.width, m.fh * cv.height);
  });
  if (!res.ok) return toast('redaction failed');
  owner.redactMarks = [];
  owner.redactMode = false;
  reflectRedact();
  els.viewerWrap.style.cursor = '';
  // The capture is used here too. This function opens by capturing `owner` and
  // threading it through owner.redactMarks and owner.redactMode, with a comment
  // saying the disarm must not touch another view — and then dropped it on the
  // reload, which is the line that actually replaces a document.
  await setDocumentFromServer(await res.json(), owner);
  toast('Redacted — affected pages are now flattened images');
};

// --- search / pattern redaction ----------------------------------------------
// Find every occurrence of a term or PII pattern in the text layer and mark each
// one for redaction, feeding the SAME view.redactMarks → applyRedact pipeline as the
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
// view.redactMarks (page-content top-left fractions → the page's live content box).
// relayoutRedactMarks does it for all rendered pages, and both run on every
// pagerendered/scalechanging — so marks show on EVERY marked page and reflow on
// zoom, not just the page that was on screen when they were generated. (The boxes
// are pure review overlays; the source of truth is view.redactMarks, which applyRedact
// flattens page by page regardless of what's currently drawn.)
function drawRedactMarks(owner, pv, pageNum) {
  pv.div.querySelectorAll('.redactmark').forEach((el) => el.remove());
  const cr = pageContentRect(pv.div);
  for (const m of owner.redactMarks) {
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
// Takes the OWNING view, never the active one: this fires on that view's own bus, and
// pairing one view's page geometry with another's marks is the SAFETY failure at the
// top of newView(), reached without any document-wide selector.
function relayoutRedactMarks(owner) {
  // No marks → nothing to draw or sweep. Bail before the all-pages walk: this runs on
  // every pagerendered/scalechanging, so it's scroll-hot-path work in the common
  // no-marks case.
  //
  // The old note here said marks only reach length 0 via a full reload "which tears the
  // boxes down with the old page DOM". ADR-002 retires that premise — a hidden view's
  // page DOM is never torn down — so the bail is now justified only by the emptiness of
  // the list, which is all it ever actually needed.
  if (!owner.pdfDocument || !owner.redactMarks.length) return;
  for (let i = 0; i < owner.pdfDocument.numPages; i++) {
    const pv = owner.viewer.getPageView(i);
    if (pv?.div) drawRedactMarks(owner, pv, i + 1);
  }
}
// Bound per view in newView() — the owning view is passed in, so a background
// viewer's event lays out ITS marks and never the active document's.

// scanTextMatches walks every page's text, runs each pattern over the per-row
// reconstructed string (buildTextRows — the same per-character geometry the field
// detectors use), and returns over-covering marks for each hit. Text runs are
// mapped into top-left page space (like the Detect path) so the box fractions go
// straight into view.redactMarks. Because pdf.js fragments a run at arbitrary points and
// buildTextRows joins fragments with a space, a "compact" string drops those
// injected boundary spaces (cx === NaN) so a pattern split across runs still matches.
async function scanTextMatches(patterns) {
  const marks = [];
  for (let n = 1; n <= view.pdfDocument.numPages; n++) {
    const page = await view.pdfDocument.getPage(n);
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
  if (!view.pdfDocument) return toast('Open a PDF first');
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
  view.redactMarks.push(...marks); // {page,fx,fy,fw,fh}; drawn per page on render
  relayoutRedactMarks(view);
  els.redactTextModal.hidden = true;
  toast(`${marks.length} match(es) marked on ${pages.size} page(s) — review the boxes (scroll to see them all), then “Apply redactions”.`);
};

// --- split by hand-drawn regions ---------------------------------------------
// Draw rectangles on the current page; on Apply, each becomes its own page (the
// page cropped to that rectangle, server-side via op:'splitrects'). The regions
// live OUTSIDE view.overlayFields, so the bake step never burns them in as marks.
let sbStart = null, sbDiv = null, sbHit = null; // transient: aborted on switch, never restored

function reflectSplitBox() {
  all('#splitBoxBtn, [data-forward="splitBoxBtn"]').forEach((b) => b.classList.toggle('active', view.splitBoxMode));
}
// View-scoped, not document-scoped. `all()` is document.querySelectorAll, and a
// .splitmark only ever lives inside a page div — so under ADR-002 the document-wide
// sweep this used to do reaches into every OTHER open document's page DOM and removes
// regions the user drew there. Rooting at the view's own container is the fix; the
// per-page model at drawRedactMarks is the same idea one level finer.
function clearSplitRects(owner = view) {
  owner.splitRects = [];
  owner.container.querySelectorAll('.splitmark').forEach((d) => d.remove());
}
function exitSplitBox() {
  if (!view.splitBoxMode) return;
  view.splitBoxMode = false;
  clearSplitRects();
  reflectSplitBox();
  els.viewerWrap.style.cursor = '';
}
els.splitBoxBtn.onclick = () => {
  if (view.splitBoxMode) { exitSplitBox(); return; }
  if (!view.pdfDocument) return;
  view.splitBoxMode = true;
  view.sbPage = view.viewer.currentPageNumber; // regions apply to the page you start on
  setMarkerMode(null);
  exitBorder();
  exitShape();
  exitCrop();
  exitNote(); exitDropdown(); exitRadio();
  if (view.redactMode) { view.redactMode = false; reflectRedact(); }
  if (view.editMode) { view.editMode = false; reflectEdit(); }
  reflectSplitBox();
  els.viewerWrap.style.cursor = 'crosshair';
};

els.viewerWrap.addEventListener('pointerdown', (e) => {
  if (!view.splitBoxMode) return;
  if (!startedInActiveView(e)) return;
  if (onExistingOverlay(e)) return;
  sbHit = pageAt(e.clientX, e.clientY);
  if (!sbHit || sbHit.n !== view.sbPage) return; // only the page the split started on
  sbStart = { x: e.clientX, y: e.clientY };
  sbDiv = document.createElement('div');
  sbDiv.className = 'splitmark';
  sbHit.pv.div.appendChild(sbDiv);
  sizeMark(sbDiv, sbHit.r, sbStart, sbStart);
  e.preventDefault();
});
els.viewerWrap.addEventListener('pointermove', (e) => {
  if (sbStart) sizeMark(sbDiv, sbHit.r, sbStart, { x: e.clientX, y: e.clientY });
});
els.viewerWrap.addEventListener('pointerup', (e) => {
  if (!sbStart) return;
  const r = sbHit.r;
  // Clamped to the page the drag started on — see clampToRect. A page fraction outside
  // [0,1] describes a box that is not on the page it names.
  const sp = clampToRect(r, { x: e.clientX, y: e.clientY }), ss = clampToRect(r, sbStart);
  const x0 = Math.min(ss.x, sp.x), y0 = Math.min(ss.y, sp.y);
  const fw = Math.abs(sp.x - ss.x) / r.width;
  const fh = Math.abs(sp.y - ss.y) / r.height;
  if (fw > 0.01 && fh > 0.01) {
    view.splitRects.push({ fx: (x0 - r.left) / r.width, fy: (y0 - r.top) / r.height, fw, fh });
  } else {
    sbDiv.remove();
  }
  sbStart = null; sbDiv = null; sbHit = null;
});

els.applyBoxSplitBtn.onclick = async () => {
  // Captured at operation entry, all of it. `sbPage` already was; `splitRects` was not,
  // and it is read after the getPage await below — so a switch landing in that window gave
  // the arrival's empty array, posted an empty region list against the wrong document, and
  // disarmed the arrival while this view kept its marks and its armed cursor.
  const owner = view;
  if (!owner.splitBoxMode || !owner.splitRects.length) return toast('Draw split regions first');
  const page = owner.sbPage;
  const rectsFrac = owner.splitRects.slice();
  const base = (await owner.pdfDocument.getPage(page)).getViewport({ scale: 1 }); // PDF points
  const f = { pageW: base.width, pageH: base.height };
  const rects = rectsFrac.map((m) => rectPoints(f, [m.fx, m.fy, m.fx + m.fw, m.fy + m.fh]));
  owner.splitBoxMode = false;
  clearSplitRects(owner);
  reflectSplitBox();
  els.viewerWrap.style.cursor = '';
  const ok = await pageOp('splitrects', { page, rects: JSON.stringify(rects) });
  if (ok) toast(rects.length === 1
    ? `Page ${page} replaced by 1 region`
    : `Page ${page} split into ${rects.length} regions`);
};

// --- crop: trim pages down to a drawn box ------------------------------------
// Draw ONE keep-rectangle on the current page; on confirm, every page (or just
// this one) is trimmed to it server-side via op:'crop' → pdfops.Crop. Like the
// split regions, the mark lives OUTSIDE view.overlayFields so the bake never burns it
// in. Reuses the same display-space → PDF-points conversion (rectPoints) as Split
// by box; the server flattens any /Rotate before cropping.
let cropStart = null, cropDiv = null, cropHit = null; // transient: aborted on switch

function reflectCrop() {
  all('#cropBtn, [data-forward="cropBtn"]').forEach((b) => b.classList.toggle('active', view.cropMode));
}
// View-scoped for the same reason as clearSplitRects above.
function clearCropRect() {
  view.cropRect = null;
  view.container.querySelectorAll('.cropmark').forEach((d) => d.remove());
}
function exitCrop() {
  if (!view.cropMode) return;
  view.cropMode = false;
  clearCropRect();
  els.cropModal.hidden = true;
  reflectCrop();
  els.viewerWrap.style.cursor = '';
}
els.cropBtn.onclick = () => {
  if (view.cropMode) { exitCrop(); return; }
  if (!view.pdfDocument) return;
  view.cropMode = true;
  view.cropPage = view.viewer.currentPageNumber; // the box is measured in this page's space
  setMarkerMode(null);
  exitBorder();
  exitShape();
  exitSplitBox();
  exitNote(); exitDropdown(); exitRadio();
  if (view.redactMode) { view.redactMode = false; reflectRedact(); }
  if (view.editMode) { view.editMode = false; reflectEdit(); }
  reflectCrop();
  els.viewerWrap.style.cursor = 'crosshair';
  toast('Draw the area to keep, then confirm');
};

els.viewerWrap.addEventListener('pointerdown', (e) => {
  if (!view.cropMode) return;
  if (!startedInActiveView(e)) return;
  if (onExistingOverlay(e)) return;
  cropHit = pageAt(e.clientX, e.clientY);
  if (!cropHit || cropHit.n !== view.cropPage) return; // measure on the page you started on
  clearCropRect(); // a single keep-rectangle: a fresh draw replaces the old one
  cropStart = { x: e.clientX, y: e.clientY };
  cropDiv = document.createElement('div');
  cropDiv.className = 'cropmark';
  cropHit.pv.div.appendChild(cropDiv);
  sizeMark(cropDiv, cropHit.r, cropStart, cropStart);
  e.preventDefault();
});
els.viewerWrap.addEventListener('pointermove', (e) => {
  if (cropStart) sizeMark(cropDiv, cropHit.r, cropStart, { x: e.clientX, y: e.clientY });
});
els.viewerWrap.addEventListener('pointerup', (e) => {
  if (!cropStart) return;
  const r = cropHit.r;
  // Clamped to the page the drag started on — see clampToRect. A page fraction outside
  // [0,1] describes a box that is not on the page it names.
  const cp = clampToRect(r, { x: e.clientX, y: e.clientY }), cs = clampToRect(r, cropStart);
  const x0 = Math.min(cs.x, cp.x), y0 = Math.min(cs.y, cp.y);
  const fw = Math.abs(cp.x - cs.x) / r.width;
  const fh = Math.abs(cp.y - cs.y) / r.height;
  cropStart = null; cropHit = null;
  if (fw > 0.01 && fh > 0.01) {
    view.cropRect = { fx: (x0 - r.left) / r.width, fy: (y0 - r.top) / r.height, fw, fh };
    els.cropModal.hidden = false; // confirm: all-pages choice + the honest note
  } else if (cropDiv) {
    cropDiv.remove(); cropDiv = null;
  }
});

els.cropCancel.onclick = () => { els.cropModal.hidden = true; clearCropRect(); }; // stay in crop mode for a redraw
els.cropGo.onclick = async () => {
  if (!view.cropRect) { els.cropModal.hidden = true; return; }
  const page = view.cropPage;
  // Send the keep-box as page fractions (top-left origin) [fx, fy, fw, fh]; the
  // server scales it to each target page's own size, so a mixed-size document
  // crops proportionally instead of to a fixed window. On a uniform document this
  // is identical to the old absolute-points path.
  const frac = [view.cropRect.fx, view.cropRect.fy, view.cropRect.fw, view.cropRect.fh];
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
let edStart = null, edDiv = null, edHit = null;

els.editTextBtn.onclick = () => {
  if (!view.pdfDocument) { toast('Open a PDF first'); return; }
  view.editMode = !view.editMode;
  if (view.editMode && view.redactMode) { view.redactMode = false; reflectRedact(); } // one box tool at a time
  if (view.editMode) { setMarkerMode(null); exitSplitBox(); exitBorder(); exitCrop(); exitNote(); exitDropdown(); exitRadio(); exitShape(); }
  reflectEdit();
  els.viewerWrap.style.cursor = view.editMode ? 'crosshair' : '';
};
function reflectEdit() {
  all('#editTextBtn, [data-forward="editTextBtn"]').forEach((b) => b.classList.toggle('active', view.editMode));
}

els.viewerWrap.addEventListener('pointerdown', (e) => {
  if (!view.editMode) return;
  if (!startedInActiveView(e)) return;
  if (onExistingOverlay(e)) return;
  edHit = pageAt(e.clientX, e.clientY);
  if (!edHit) return;
  edStart = { x: e.clientX, y: e.clientY };
  edDiv = document.createElement('div');
  edDiv.className = 'editmark';
  edHit.pv.div.appendChild(edDiv);
  sizeMark(edDiv, edHit.r, edStart, edStart);
  e.preventDefault();
});
els.viewerWrap.addEventListener('pointermove', (e) => {
  if (edStart) sizeMark(edDiv, edHit.r, edStart, { x: e.clientX, y: e.clientY });
});
els.viewerWrap.addEventListener('pointerup', async (e) => {
  if (!edStart) return;
  const r = edHit.r, hit = edHit;
  // Clamped to the page the drag started on — see clampToRect. A page fraction outside
  // [0,1] describes a box that is not on the page it names.
  const ep = clampToRect(r, { x: e.clientX, y: e.clientY }), es = clampToRect(r, edStart);
  const x0 = Math.min(es.x, ep.x), y0 = Math.min(es.y, ep.y);
  const fw = Math.abs(ep.x - es.x) / r.width;
  const fh = Math.abs(ep.y - es.y) / r.height;
  const fx0 = (x0 - r.left) / r.width, fy0 = (y0 - r.top) / r.height;
  edDiv.remove();
  edStart = null; edDiv = null; edHit = null;
  if (fw > 0.004 && fh > 0.004) await addEdit(hit, [fx0, fy0, fx0 + fw, fy0 + fh]);
});

// addEdit reads the text run under the drawn box and its style, then builds the
// prefilled, covered edit overlay. frac is the box in page fractions (top-left).
async function addEdit(hit, frac) {
  const owner = view; // captured at entry: everything below runs past several awaits
  const n = hit.n, pv = hit.pv;
  const page = await owner.pdfDocument.getPage(n);
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
  const f = makeEditField(frac, { page: n, pageW, pageH, text, size, color, bg, font, coverFrac }, pv, owner);
  f.el.focus();
  f.el.select();
}

function makeEditField(frac, opts, pv, owner = view) {
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
  owner.overlayFields.push(f);
  layoutField(f, pv);
  pv.div.appendChild(el);
  recordAdd(f, owner);
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
let markerSig = null, markerInit = null; // remembered sign/initial fill sources for this session

// Two gates govern what can be done to flags. flagsEditable: the preparer may place,
// drag, and delete flags (only while unlocked). flagsFillable: a flag may be clicked
// to fill it — true while unlocked (a preparer self-filling a form) and, even when
// locked, on a received document (the recipient signs but can't reshape the layout).
function flagsEditable() { return !view.signLocked; }
function flagsFillable() { return !view.signLocked || view.docHadFlags; }
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
  b.onclick = () => { if (!view.pdfDocument) return toast('Open a PDF first'); setMarkerMode(view.markerMode === b.dataset.marker ? null : b.dataset.marker); };
});
els.saveForSigningBtn.onclick = () => { if (!view.pdfDocument) return toast('Open a PDF first'); saveForSigning(); };

// "Signing marks completed" locks the document into signing-only mode: flag
// placement and every content-editing tool turn off and the placed flags freeze.
// Toggling it ("Edit marks again") restores editing. A received document (one that
// arrived with flags) opens locked with no toggle — it is the counterparty's copy
// and must stay non-editable; see setDocumentFromServer.
els.signCompleteBtn.onclick = () => {
  if (!view.pdfDocument) return;
  if (!view.signLocked && !markerFields().length) return toast('Place at least one flag first.');
  setSignLocked(!view.signLocked);
  toast(view.signLocked
    ? 'Marks locked — the document is in signing mode and can no longer be edited.'
    : 'Editing re-enabled — you can change the flags again.');
};

// The content-editing tools (Edit + Protect menus) that signing mode switches off.
// Certify (cryptographic signing/timestamp/co-sign), File, and View stay available —
// they are part of, or orthogonal to, signing the document.
const EDITING_TOOLS = [
  // **The same five omissions as DOC_REQUIRED, and here they cost more** (/pending 331,
  // found while fixing that list rather than filed with it). These five draw CONTENT onto
  // the page, so signing mode has to switch them off with the rest — and it did not.
  // Locking the marks toasts "the document is in signing mode and can no longer be
  // edited" while Border, Note, Dropdown, Radio and Shapes stayed armable, which makes
  // that sentence false. Named search for any other door that disables them:
  // `grep -nE 'borderBtn|noteBtn|dropdownBtn|radioBtn|shapeBtn' web/app.js | grep -i disabled`
  // returned nothing, so these two lists are the only gates they have.
  'textToolBtn', 'highlightToolBtn', 'drawToolBtn', 'detectBtn',
  'borderBtn', 'noteBtn', 'dropdownBtn', 'radioBtn', 'shapeBtn',
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
  view.signLocked = locked;
  if (locked) { // leaving any active editing tool behind would be a dead, disabled mode
    setMarkerMode(null);
    if (view.redactMode) { view.redactMode = false; reflectRedact(); els.viewerWrap.style.cursor = ''; }
    if (view.editMode) { view.editMode = false; reflectEdit(); els.viewerWrap.style.cursor = ''; }
  }
  applySignLock();
}

// applySignLock reflects view.signLocked across the whole UI. Safe to call whenever the
// lock, the open document, or the flag count changes.
function applySignLock() {
  const open = !!view.pdfDocument;
  setEditingEnabled(open && !view.signLocked);
  els.viewerWrap.classList.toggle('signing-locked', view.signLocked);
  reflectSignControls();
}

// reflectSignControls keeps the Flags-panel controls consistent with the role
// (preparer vs recipient) and the lock state.
function reflectSignControls() {
  const open = !!view.pdfDocument;
  const recipient = view.docHadFlags;            // opened with flags => the counterparty's copy
  const n = markerFields().length;
  // A recipient never prepares; a locked preparer can't place until they edit again.
  all('.markers button').forEach((b) => { b.disabled = !open || recipient || view.signLocked; });
  els.saveForSigningBtn.disabled = !open || recipient;
  els.signCompleteBtn.hidden = !open || recipient;
  els.signCompleteBtn.disabled = !view.signLocked && n === 0;
  els.signCompleteBtn.textContent = view.signLocked ? 'Edit marks again' : 'Signing marks completed';
  els.signCompleteBtn.classList.toggle('active', view.signLocked);
}

function setMarkerMode(m) {
  view.markerMode = m;
  if (m) { // one placement tool at a time
    if (view.redactMode) { view.redactMode = false; reflectRedact(); }
    if (view.editMode) { view.editMode = false; reflectEdit(); }
    exitSplitBox();
    exitBorder();
    exitShape();
    exitCrop();
    exitNote(); exitDropdown(); exitRadio();
  }
  all('.markers button').forEach((b) => b.classList.toggle('active', b.dataset.marker === m));
  els.viewerWrap.style.cursor = m ? 'crosshair' : '';
}

// A single click on the page drops a default-sized marker centred at the click.
els.viewerWrap.addEventListener('pointerdown', (e) => {
  if (!view.markerMode) return;
  if (!startedInActiveView(e)) return;
  // The one placement tool that does NOT take onExistingOverlay's blanket bail, because
  // for this tool one overlay IS a placement target: a detected blank, which the flag
  // tool converts. That is the documented primary flow — Detect, pick a flag, click a
  // blank — so a plain `if (onExistingOverlay(e)) return;` here would fix the × and
  // break the flow it exists for. Every OTHER overlay under the cursor is its own
  // control, and the flag's × is the one that bites.
  const ovl = e.target.closest('.ovl');
  if (ovl) {
    const field = view.overlayFields.find((f) => f.kind === 'text' && f.el === ovl);
    if (!field) return;
    e.preventDefault();
    return void convertFieldToFlag(field, view.markerMode);
  }
  const hit = pageAt(e.clientX, e.clientY);
  if (!hit) return;
  e.preventDefault();
  const r = hit.r, [fw, fh] = MARKER_SIZES[view.markerMode];
  const fx = Math.min(Math.max((e.clientX - r.left) / r.width - fw / 2, 0), 1 - fw);
  const fy = Math.min(Math.max((e.clientY - r.top) / r.height - fh / 2, 0), 1 - fh);
  makeMarker(view.markerMode, [fx, fy, fx + fw, fy + fh], hit);
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
  view.dirty = true;         // …and therefore not caught by the funnel either
  const f = buildMarker(type, [fx0, top, fx1, fy1], field.page, false);
  const pv = view.viewer.getPageView(field.page - 1);
  if (pv?.div) { pv.div.appendChild(f.el); layoutField(f, pv); }
  return f;
}

// buildMarker creates a marker field + its DOM element and registers it, but does
// NOT attach it to a page — relayoutOverlays places it once the page renders.
// This lets reconstructFlags rebuild markers on pages that aren't on screen yet.
function buildMarker(type, frac, page, record = true, owner = view) {
  const f = { page, frac, kind: 'marker', tagType: type };
  const el = document.createElement('div');
  el.className = 'ovl ovl-marker';
  el.tabIndex = 0;
  const label = document.createElement('span');
  label.className = 'marker-label';
  label.textContent = MARKER_LABELS[type];
  const del = document.createElement('button');
  del.className = 'marker-del'; del.textContent = '×'; del.title = 'Remove flag';
  del.onclick = (e) => { e.stopPropagation(); if (flagsEditable()) removeField(f, true, owner); };
  el.append(label, del);
  f.el = el;
  enableMarkerGestures(f, el);
  el.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); if (flagsFillable()) fillMarker(f); }
    else if ((e.key === 'Delete' || e.key === 'Backspace') && flagsEditable()) removeField(f, true, owner);
  });
  owner.overlayFields.push(f);
  reflectSignControls();
  if (record) {
    recordOverlayEdit({
      undo: () => { detachField(owner, f); reflectSignControls(); },
      redo: () => { reattachField(owner, f); reflectSignControls(); },
    }, owner);
  }
  return f;
}

function makeMarker(type, frac, hit) {
  const f = buildMarker(type, frac, hit.n);
  hit.pv.div.appendChild(f.el);
  layoutField(f, hit.pv);
  return f;
}

// `owner` for a stronger reason than the keydown handler it is bound to: buildMarker is
// called with a BACKGROUND owner from reconstructFlags on the arrival load path, so this is
// reachable for a view that is not on screen — and activeMarker/fillTarget are that view's
// bindings. (The × buttons in the overlay factories are the same shape and stayed on the
// default; they are safe only because a display:none view receives no pointer events, which
// is the weaker reason and is why they are filed rather than fixed.)
function removeField(f, record = true, owner = view) {
  if (record) {
    recordOverlayEdit({
      undo: () => { reattachField(owner, f); reflectSignControls(); },
      redo: () => { detachField(owner, f); reflectSignControls(); },
    }, owner);
  }
  detachField(owner, f);
  if (owner.activeMarker === f) owner.activeMarker = null;
  if (owner.fillTarget === f) owner.fillTarget = null;
  reflectSignControls();
}

// enableMarkerGestures: drag repositions; a click (no drag) fills the marker. Both
// honour the signing-mode gates — a frozen flag (locked preparer copy) ignores the
// pointer entirely; a received flag fills but can't be dragged.
function enableMarkerGestures(f, el) {
  let down = null, moved = false, owner = null, pid = null;
  // One `end` for the normal finish and the abort — see enableStampGestures for why.
  const end = () => {
    down = null; owner = null;
    if (pid !== null) { try { el.releasePointerCapture(pid); } catch { /* already released */ } pid = null; }
    activeGestures.delete(end);
  };
  el.addEventListener('pointerdown', (e) => {
    if (e.target.closest('.marker-del')) return;
    if (!flagsFillable() && !flagsEditable()) return; // frozen placeholder
    e.preventDefault(); e.stopPropagation();
    down = { x: e.clientX, y: e.clientY, frac: f.frac.slice() }; moved = false;
    owner = view; pid = e.pointerId;
    el.setPointerCapture(e.pointerId);
    activeGestures.add(end);
  });
  el.addEventListener('pointermove', (e) => {
    if (!down || !flagsEditable()) return; // a recipient signs but can't reshape the layout
    const pv = owner.viewer.getPageView(f.page - 1); if (!pv?.div) return;
    const W = pv.div.clientWidth, H = pv.div.clientHeight;
    if (Math.abs(e.clientX - down.x) + Math.abs(e.clientY - down.y) > 3) moved = true;
    const [x0, y0, x1, y1] = down.frac, w = x1 - x0, h = y1 - y0;
    const nx = Math.min(Math.max(x0 + (e.clientX - down.x) / W, 0), 1 - w);
    const ny = Math.min(Math.max(y0 + (e.clientY - down.y) / H, 0), 1 - h);
    f.frac = [nx, ny, nx + w, ny + h]; layoutField(f, pv);
  });
  el.addEventListener('pointerup', () => {
    const wasDown = down, wasOwner = owner;
    end();
    // An aborted gesture has `down` null, so neither branch runs: a switch mid-press
    // must not move the flag AND must not count as the click that fills it.
    if (wasDown && moved && f.frac.join() !== wasDown.frac.join()) recordMove(f, wasDown.frac.slice(), f.frac.slice(), wasOwner);
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
  view.fillTarget = f;
  setActiveMarker(f);
  document.querySelector('.tab[data-panel="library"]')?.click();
  toast('Pick your ' + (f.tagType === 'sign' ? 'signature' : 'initials') + ' in the Library — it fills this marker and any others.');
}

function resolveFillTarget(src) {
  const f = view.fillTarget; view.fillTarget = null;
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
  // Captured at entry, and this one is REACHABLE rather than latent. A co-sign arrival
  // creates a second view in production today (pollRecv -> openArrivalInNewView), so the
  // await on the image below is a real window: the continuation would register the stamp in
  // the ARRIVAL's overlayFields while appending its element into THIS document's page div,
  // and the arrival's bake would then burn it onto the arrival at this document's
  // coordinates. Everything after the first await uses `owner`.
  const owner = view;
  const pv = owner.viewer.getPageView(f.page - 1);
  if (!pv?.div) return toast('Scroll the page into view, then try again');
  const base = (await owner.pdfDocument.getPage(f.page)).getViewport({ scale: 1 });
  const next = nextMarkerAfter(f, owner);
  await new Promise((resolve) => {
    const img = new Image();
    img.onload = () => {
      const aspect = (img.naturalWidth / img.naturalHeight) || 1;
      const W = pv.div.clientWidth, H = pv.div.clientHeight;
      const [x0, y0, x1, y1] = f.frac;
      let w = x1 - x0, h = w * W / (aspect * H); // fit width, derive height by aspect
      if (h > y1 - y0) { h = y1 - y0; w = h * aspect * H / W; } // too tall — fit height instead
      makeStamp(src, aspect, [x0, y0, x0 + w, y0 + h], { page: f.page, pageW: base.width, pageH: base.height }, pv, owner);
      resolve();
    };
    img.onerror = () => { toast('could not load image'); resolve(); };
    img.src = src;
  });
  removeField(f, true, owner);
  // The banner and the active-marker highlight are SHARED chrome, so they are repainted only
  // when this document is still the one on screen — rather than threaded, which would mean
  // painting one document's progress while another is displayed.
  if (owner === view) advanceTo(next);
}

// nextMarkerAfter returns the next unfilled marker after f in reading order
// (page, then top-to-bottom, then left-to-right), wrapping to the first if none.
function nextMarkerAfter(f, owner = view) {
  const markers = owner.overlayFields.filter((o) => o.kind === 'marker' && o !== f)
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
  view.activeMarker?.el?.classList.remove('marker-active');
  view.activeMarker = m;
  m?.el?.classList.add('marker-active');
}

// --- signing flags (DocuSign-style place-then-email) -------------------------
// A preparer drops flags, saves for signing (the flags ride inside the one PDF
// as the NibFlags property — see internal/pdfops/flags.go), and emails it. The
// recipient's Nib rebuilds the flags on open and walks them flag-to-flag.

function markerFields(owner = view) { return owner.overlayFields.filter((f) => f.kind === 'marker'); }

function collectFlags(owner = view) {
  return markerFields(owner).map((f) => ({ page: f.page, frac: f.frac, type: f.tagType }));
}

// embedFlags posts the current document to /api/flags: a non-empty set embeds the
// placeholders; null strips them. Returns the new bytes.
// embedFlags is the one site the pinning SCAN cannot see: its apiFetch is its own
// first await, so nothing changes underneath it *here* — but it receives
// document-derived `bytes` from a caller that has already awaited to produce them. The
// corruption is real and the capture point belongs to the caller, so the id is a
// required-in-practice parameter rather than an entry-time default. A scanner that
// looks only at await-ordering inside a function is structurally blind to this shape,
// which is why it is named here rather than left to the guard.
async function embedFlags(bytes, flags, docId = view.docMeta && view.docMeta.id) {
  const form = new FormData();
  form.append('pdf', new Blob([bytes], { type: 'application/pdf' }), 'doc.pdf');
  if (flags && flags.length) form.append('flags', JSON.stringify(flags));
  const res = await apiFetch('/api/flags', { method: 'POST', body: form, docId });
  if (!res.ok) throw new Error('flags update failed');
  return new Uint8Array(await res.arrayBuffer());
}

// reconstructFlags rebuilds embedded placeholders as markers on open and shows the
// signing banner. Coordinates are clamped defensively — a hand-edited property
// can never place a flag off-page or on a page that doesn't exist.
function reconstructFlags(flags, owner = view) {
  if (!Array.isArray(flags)) return;
  for (const fl of flags) {
    if (!fl || !MARKER_LABELS[fl.type] || !Array.isArray(fl.frac) || fl.frac.length !== 4) continue;
    const page = Math.max(1, Math.min(owner.pdfDocument.numPages, fl.page | 0));
    const frac = fl.frac.map((n) => Math.max(0, Math.min(1, +n || 0)));
    buildMarker(fl.type, frac, page, false, owner); // load on open — not an undoable edit
  }
  relayoutOverlays(owner); // attach flags on already-rendered pages; the rest follow on pagerendered
  showSignBanner(owner);
}

function firstMarker() {
  return markerFields().sort((a, b) => a.page - b.page || a.frac[1] - b.frac[1] || a.frac[0] - b.frac[0])[0] || null;
}

function showSignBanner(owner = view) {
  owner.signTotal = markerFields(owner).length;
  owner.signStarted = false;
  // The banner is one element for N documents, so a background load records its own count
  // and paints nothing. activateView repaints it from the incoming view's signTotal.
  if (owner !== view) return;
  if (!owner.signTotal) { els.signBanner.hidden = true; return; }
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
    els.signMsg.textContent = view.signTotal ? 'All fields filled.' : 'No fields to fill.';
    els.signAction.textContent = 'Mark complete & sign';
    els.signAction.onclick = completeAndSign;
    els.signDone.hidden = true;
    return;
  }
  els.signMsg.textContent = view.signStarted
    ? `Field ${view.signTotal - remaining + 1} of ${view.signTotal}.`
    : `This document has ${view.signTotal} field${view.signTotal === 1 ? '' : 's'} to fill.`;
  els.signAction.textContent = view.signStarted ? 'Next field' : 'Start';
  els.signAction.onclick = signNext;
  els.signDone.hidden = false;
  els.signDone.onclick = completeAndSign;
}

function signNext() {
  view.signStarted = true;
  const from = (view.activeMarker && view.activeMarker.kind === 'marker') ? view.activeMarker : null;
  advanceTo(from ? nextMarkerAfter(from) : firstMarker());
}

// saveForSigning embeds the placed flags and offers the single emailable file.
async function saveForSigning() {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
  const flags = collectFlags();
  if (!flags.length) return toast('Place at least one flag first (Sign / Date / Initial).');
  // Captured before the first await, and threaded into every helper below: they
  // receive bytes derived from THIS document, and their own entry is already too late.
  const opDoc = view.docMeta;
  els.saveBtn.disabled = true;
  try {
    const baked = await bakedBytes(opDoc && opDoc.id); // bake any non-flag edits; strips a prior flag set
    const out = await embedFlags(baked, flags, opDoc && opDoc.id); // embed the current flags
    openSaveAs(new Blob([out], { type: 'application/pdf' }), exportName + '-for-signing.pdf', 'Save for signing — email this exact file');
    toast('Saved for signing. Email this file as-is; printing or re-exporting it elsewhere drops the flags.');
  } catch (e) {
    toast('could not prepare the document: ' + e.message);
  } finally {
    els.saveBtn.disabled = !view.pdfDocument;
  }
}

// completeAndSign ends the recipient's signing flow in one step: it bakes the
// filled flags (bakedBytes strips the NibFlags property so the file won't reopen
// in signing mode), then applies a tamper-evident certification signature via the
// same /api/finalize path Finalize & sign uses. The baked-then-signed file is
// flat and frozen — any later edit breaks the signature.
async function completeAndSign() {
  // Export name captured at operation entry — see exportBase (D7).
  const exportName = exportBase();
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
    const base = exportName.replace(/-for-signing$/i, '');
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
  const owner = view; // captured at entry — everything below the await acts on THIS document
  const pages = [...new Set(owner.overlayFields.filter((f) => f.kind === 'edit').map((f) => f.page))];
  if (!pages.length) return toast('No text edits to flatten');
  if (!confirm('Make the text edits permanent? The edited page(s) become flat images and the original text underneath is removed. This cannot be undone.' + signatureWarning())) return;

  const res = await flattenPages(pages);
  if (!res.ok) return toast('could not flatten edits');
  owner.overlayFields = owner.overlayFields.filter((f) => { if (f.kind === 'edit' && pages.includes(f.page)) { f.el.remove(); return false; } return true; });
  owner.editMode = false; if (owner === view) reflectEdit();
  if (owner === view) els.viewerWrap.style.cursor = '';
  await setDocumentFromServer(await res.json(), owner);
  toast('Edits flattened — original text removed on edited page(s)');
};

// --- open dialog -------------------------------------------------------------
// The Open… dialog is the single open surface: type a path or URL, or browse the
// filesystem. Browsing opens BY PATH (via openPath -> /api/open), so the file can
// be saved in place and is remembered in Recent — unlike a drag-drop upload. It
// runs on the same browseDir as the three destination pickers; the only thing
// that makes it different is that its rows can be files.
const openDirEls = () => ({ dir: els.openDir, here: els.openHere, up: els.openUp, list: els.openList });
const openFileRow = (path) => { els.openModal.hidden = true; openPath(path); };
// The dir input keeps whatever folder the last browse left in it, so it doubles
// as the "where was I?" memory across opens; '~' seeds the very first one at home.
const openBrowse = (path) => browseDir(path || '~', openDirEls(), openFileRow);

function openOpenDialog() {
  els.openModal.hidden = false;
  els.pathInput.value = '';
  openBrowse(els.openDir.value.trim());
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
// **Focus goes back to the trigger, or it goes to `<body>`** (/pending 327). Removing
// `.open` puts the focused item into a `display: none` subtree, and the browser then
// drops focus to the document — SC 2.4.3, and measured three ways (Export, ⚙, and Save-as
// via click-outside). The dialog focus-restore code below already names this exact harm
// in its own comment and calls it "relocated"; it had a door for dialogs and none for
// menus, which is ADR-009's shape.
//
// **Read BEFORE the class comes off**, which is what makes this simpler than the dialog
// case. That one runs in a MutationObserver a microtask later, by which time a real
// browser has already blurred to `<body>` and `activeElement` always answers "not inside"
// — so it has to consult the focus trail instead. Here the check is synchronous and
// `activeElement` is still true.
//
// The `held` test is also what keeps this from STEALING focus. A dropdown item's own
// onclick fires before the menubar's delegated handler, so a command that opens a dialog
// and focuses a field synchronously has already moved focus out by the time this runs —
// `held` is false and nothing is touched. For the dialogs that focus a microtask later,
// `held` is true, the trigger takes focus, and the dialog's opener-restore then resolves
// to that trigger: connected and laid out, which is precisely what the relocated harm was.
function closeMenu() {
  if (!openMenu) return;
  const held = openMenu.contains(document.activeElement);
  const trigger = openMenu.querySelector('.menutop');
  openMenu.classList.remove('open');
  trigger?.setAttribute('aria-expanded', 'false');
  openMenu = null;
  if (held) trigger?.focus();
}
function showMenu(menu) {
  if (openMenu === menu) return;
  closeMenu();
  menu.classList.add('open');
  menu.querySelector('.menutop')?.setAttribute('aria-expanded', 'true');
  openMenu = menu;
  if (menu.querySelector('.recentSlot')) refreshRecent();
}

// Stamped at boot from the live DOM rather than written into index.html — the same
// argument the dialog block below makes: menu five is then covered by existing, not by
// its author remembering. There were zero `aria-haspopup` and zero `aria-expanded` in the
// front end, so a screen-reader user was told neither that a control opens a menu nor
// whether it is currently open.
for (const t of document.querySelectorAll('.menu > .menutop')) {
  t.setAttribute('aria-haspopup', 'true');
  t.setAttribute('aria-expanded', String(t.parentElement.classList.contains('open')));
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
// Escape closes what is open, in one place. Before this, `closeMenu()` was the ONLY
// thing in the file bound to Escape: none of the 37 *Modal dialogs closed on it, and the
// first-run intro overlay had no close control of any kind — a keyboard-only or
// screen-reader user on first run had no announced way past it to the setup form beneath.
//
// It CLICKS the dialog's own cancel rather than hiding the element, and that is the whole
// design: hiding would skip the cleanup each cancel performs — saveAsCancel drops the
// pending export's bytes, srvCancel disarms a live listening session, compareClose tears
// down a second pdf.js document. A dismissal that skipped those would leave state behind
// that the same button, clicked, removes. Every one of the 37 has such a control and they
// follow one naming convention, checked across the whole file.
//
// The LAST open dialog in document order is the one dismissed: dialogs stack in that
// order, and Escape means "the one in front of me".
document.addEventListener('keydown', (e) => {
  if (e.key !== 'Escape') return;
  closeMenu();
  const open = [...document.querySelectorAll('div[id$="Modal"]:not([hidden])')];
  const top = open[open.length - 1];
  if (!top) return;
  e.preventDefault();
  const dismiss = top.querySelector('button[id$="Cancel"], button[id$="Close"]');
  if (dismiss) dismiss.click();
  else top.hidden = true; // a dialog with no cancel control: hide it rather than trap the user
});

// --- tab semantics -------------------------------------------------------------
// /pending 329. Four tab-like surfaces, one rule, and it reached one of them — and that
// one announced a widget it had not built.
//
// The mode tabs, the sidebar tabs and Collaborate's Originate/Receive toggle carried NO
// role, no aria-selected and no aria-current: the active one differed by two greys and a
// 2px underline, which is colour alone (WCAG 1.4.1) and invisible to a reader. The
// document strip did claim `role="tablist"` correctly — and gave EVERY tab `tabIndex = 0`
// while binding no arrow key, so it promised arrow navigation and did not implement it.
// The strip's own comment states the principle it then broke: "a tablist whose tabs
// control nothing is ARIA that announces a widget and then cannot describe it — worse
// than plain buttons, because the promise is louder."
//
// **Manual activation, not automatic** (APG allows either). Arrows move focus; Enter or
// Space selects. Automatic activation would be defensible for the three cheap surfaces
// and is WRONG for the strip, where selecting rebuilds the strip and destroys the very
// element holding focus — so one behaviour across all four beats a rule with an exception
// nobody remembers.
//
// **Roving tabindex**, which is the half that makes a tablist one Tab stop instead of N:
// the selected tab is 0 and the rest are -1. That is also the specific thing the strip
// had backwards.
//
// The active tab is read from the `.active` class every one of these four already
// maintains, and re-read through a MutationObserver rather than by calling a refresh from
// each activation site — same argument the dialog block below makes for enumerating from
// the live DOM: the strip is rebuilt wholesale on every open and close, and a refresh
// wired per call site is one the next call site will not have.
function wireTablist(container, tabSelector) {
  if (!container) return;
  container.setAttribute('role', 'tablist');
  const tabs = () => [...container.querySelectorAll(tabSelector)];
  const sync = () => {
    for (const t of tabs()) {
      const on = t.classList.contains('active');
      t.setAttribute('role', 'tab');
      t.setAttribute('aria-selected', String(on));
      t.tabIndex = on ? 0 : -1;
    }
    // Nothing selected yet (the strip before its first activation) would leave every tab
    // at -1 and the whole widget unreachable by keyboard — worse than the defect being
    // fixed. The first tab takes the stop in that case.
    const list = tabs();
    if (list.length && !list.some((t) => t.tabIndex === 0)) list[0].tabIndex = 0;
  };
  container.addEventListener('keydown', (e) => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) return;
    const list = tabs().filter((t) => !t.hidden && !t.disabled);
    const cur = list.indexOf(document.activeElement.closest(tabSelector));
    if (cur === -1 || !list.length) return;
    e.preventDefault();
    const next = e.key === 'Home' ? 0
      : e.key === 'End' ? list.length - 1
      : e.key === 'ArrowLeft' ? (cur - 1 + list.length) % list.length
      : (cur + 1) % list.length;
    list[next].tabIndex = 0;
    list[cur].tabIndex = -1;
    list[next].focus();
  });
  new MutationObserver(sync).observe(container, {
    subtree: true, childList: true, attributes: true, attributeFilter: ['class'],
  });
  sync();
}

wireTablist(document.querySelector('.modetabs'), '.modetab');
wireTablist(document.querySelector('.sidebar .tabs') || document.querySelector('nav.tabs'), '.tab');
wireTablist(document.querySelector('.roletoggle'), '.roleopt');
wireTablist(document.getElementById('tabstrip'), '.tab');

// --- dialog semantics and focus -----------------------------------------------
// Every `body > div[id$="Modal"]` is a modal dialog and none of them said so: 38
// overlays, zero `role="dialog"`, zero `aria-modal`, zero `aria-labelledby`, and
// opening one moved no focus while closing one restored none. Escape already worked
// (above), so what was broken was SC 2.4.3 and 4.1.2, not 2.1.2.
//
// **Stamped at boot from the live DOM, not written into index.html.** The 38 are
// enumerated by matching the selector, so dialog 39 is covered by existing rather
// than by its author remembering — which is the same argument the Escape handler's
// own guard makes ("the next dialog added is the one that will not have it"). It is
// also why the guard for this boots the app and reads the DOM: the attributes are
// never in the file, so a static scan of index.html would be red forever.
//
// `aria-modal="true"` tells a screen reader to prune everything outside the dialog.
// Shipping that WITHOUT containment is a net regression — the user would Tab to
// elements the reader has been told do not exist — so the redirect below is not
// optional garnish, it is the other half of the attribute.
const modalSelector = 'body > div[id$="Modal"]';

for (const m of document.querySelectorAll(modalSelector)) {
  m.setAttribute('role', 'dialog');
  m.setAttribute('aria-modal', 'true');
  if (!m.hasAttribute('tabindex')) m.setAttribute('tabindex', '-1');
  // aria-labelledby POINTS at the heading rather than copying its text, because three
  // of these titles are rewritten by JS at runtime (saveAsTitle, srvTitle, aboutTitle);
  // a string copied at boot would announce "Save" for an export. A dialog with no
  // heading deliberately gets no label — an invented one is a lie, and the guard is
  // what should complain.
  const h = m.querySelector('h3, h2');
  if (h) {
    if (!h.id) h.id = m.id + 'Title';
    m.setAttribute('aria-labelledby', h.id);
  }
}

// The last two focus targets. Two, not one, because dialogs genuinely stack — showVerify
// opens verifyModal while sessionInitModal is still open — so the opener to restore may
// itself be inside another dialog; and because the dialog being opened may already hold
// focus (below), in which case the most recent target is the wrong answer.
let focusTrail = [];
document.addEventListener('focusin', (e) => {
  focusTrail.push(e.target);
  if (focusTrail.length > 2) focusTrail.shift();
});

// Containment. On focusin, if the topmost open dialog does not contain the target, pull
// focus back to the dialog itself. Reading the live DOM each time rather than computing
// first/last focusable at open time, because six of these dialogs build their rows AFTER
// opening (loadKeys, loadPeers, browseDir, the outline and attachment lists) and a
// sentinel captured at open would be stale before the user pressed Tab.
//
// **It cannot loop or strand**: the redirect target is the dialog container, which was
// given tabindex="-1" above, so the focus call always succeeds. Do not "simplify" that
// tabindex away.
document.addEventListener('focusin', (e) => {
  const open = [...document.querySelectorAll(modalSelector + ':not([hidden])')];
  const top = open[open.length - 1];
  if (!top || top.contains(e.target)) return;
  top.focus();
});

// Focus in on open, focus back on close. One observer per dialog rather than one subtree
// observer on body: all 38 are direct children of body and none is created at runtime, so
// a boot-time enumeration is total — and observing them individually sees 113 relevant
// mutations instead of the whole app's 190.
for (const m of document.querySelectorAll(modalSelector)) {
  let wasOpen = !m.hidden;
  let opener = null;
  new MutationObserver(() => {
    const open = !m.hidden;
    // A state machine, not a reaction: closeDocBoundModals() hides twenty-odd dialogs
    // unconditionally on every view switch, most of them already hidden, and firing
    // restore-focus on each of those would teleport the user on every tab change.
    if (open === wasOpen) return;
    wasOpen = open;
    if (open) {
      // The most recent focus that is NOT already inside this dialog. Five dialogs focus
      // their own field synchronously right after unhiding (decryptPw, encryptPw,
      // extractPages, saveAsName+select, and openModal via openBrowse) and the observer
      // record arrives a microtask later — measured — so without this they would be
      // stomped half a tick after they landed. They need no edit of their own.
      opener = [...focusTrail].reverse().find((el) => el && !m.contains(el)) || null;
      if (!m.contains(document.activeElement)) m.focus();
      return;
    }
    // Restore only if focus was actually inside this dialog, and only to an opener that
    // is still connected AND still laid out. Most of these are launched from a dropdown
    // item, and the menu collapses in the same click that opens the dialog — so by the
    // time it closes the saved opener is inside a `display: none` subtree, where .focus()
    // is a silent no-op and the browser drops focus to <body>. That is the same SC 2.4.3
    // harm this code exists to remove, relocated.
    // Whether focus was inside is read from the TRAIL, not from activeElement — because
    // by the time this observer runs (a microtask after `hidden` is set) a real browser
    // has already blurred the focused descendant to <body>, so activeElement always says
    // "not inside" and the restore never fires. jsdom does NOT blur on hide, so tier 2 is
    // green against that defect; only test/ui/dialogfocus.test.mjs can see it, and it did.
    const hadFocus = focusTrail.some((el) => el && (el === m || m.contains(el)));
    if (!hadFocus) return;
    if (opener && opener.isConnected && opener.offsetParent !== null) opener.focus();
    else document.getElementById('menubar')?.querySelector('button')?.focus();
    opener = null;
  }).observe(m, { attributes: true, attributeFilter: ['hidden'] });
}

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
    for (const e of recent) {
      const b = document.createElement('button');
      // The server sends the display name: only it can tell where a path ends.
      b.textContent = e.name; b.title = e.path;
      b.onclick = () => openPath(e.path);
      slot.appendChild(b);
    }
  }
}

// Annotation tools (M4 Text + M8 Highlight/Draw): mutually exclusive toggles of
// the pdf.js editor mode. Each is baked into the PDF by saveDocument(). Modes
// come from the buttons' data-mode (FREETEXT, HIGHLIGHT, INK).
function setTool(mode) {
  view.activeTool = view.activeTool === mode ? null : mode;
  view.viewer.annotationEditorMode = {
    mode: view.activeTool ? pdfjsLib.AnnotationEditorType[view.activeTool] : pdfjsLib.AnnotationEditorType.NONE,
  };
  if (view.activeTool) { exitBorder(); exitNote(); exitDropdown(); exitRadio(); exitShape(); } // Nib-side tools, not pdf.js modes — one at a time
  // Mirror the active mode onto every control bound to it (Edit menu + toolbar).
  // Scope out the compare tabs: they share the data-mode attribute (text/side/diff)
  // but are wired to setCompareMode, not the annotation tools.
  document.querySelectorAll('[data-mode]:not(.cmmode)').forEach((b) => b.classList.toggle('active', b.dataset.mode === view.activeTool));
  // The highlight color row is contextual — show it only while highlighting (or
  // drawing a border), and re-assert the selected color so the next highlight
  // uses it (not pdf.js yellow).
  reflectAnnoControls();
  if (view.activeTool === 'HIGHLIGHT') applyHighlightColor(selectedHlColor);
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

function applyHighlightColor(hex, owner = view) {
  owner.eventBus.dispatch('switchannotationeditorparams', {
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
let bdStart = null, bdDiv = null, bdHit = null;

// Show the highlight color row while highlighting OR drawing borders; the
// thickness control only while drawing borders.
function reflectAnnoControls() {
  els.hlColors.hidden = !(view.activeTool === 'HIGHLIGHT' || view.borderMode || view.shapeMode);
  els.borderWidth.hidden = !(view.borderMode || view.shapeMode);
  els.shapeOpts.hidden = !view.shapeMode;
}
function reflectBorder() {
  els.borderBtn.classList.toggle('active', view.borderMode);
  reflectAnnoControls();
}
function exitBorder() {
  if (!view.borderMode) return;
  view.borderMode = false;
  reflectBorder();
  els.viewerWrap.style.cursor = '';
}
const clampWeight = (v) => Math.min(10, Math.max(1, Number(v) || 2)); // points

els.borderBtn.onclick = () => {
  if (view.borderMode) { exitBorder(); return; }
  if (!view.pdfDocument) { toast('Open a PDF first'); return; }
  view.borderMode = true;
  setTool(null); // clear any pdf.js editor tool
  setMarkerMode(null);
  if (view.redactMode) { view.redactMode = false; reflectRedact(); }
  if (view.editMode) { view.editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitNote(); exitDropdown(); exitRadio();
  reflectBorder();
  els.viewerWrap.style.cursor = 'crosshair';
};

els.viewerWrap.addEventListener('pointerdown', (e) => {
  if (!view.borderMode) return;
  if (!startedInActiveView(e)) return;
  if (onExistingOverlay(e)) return;
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
els.viewerWrap.addEventListener('pointermove', (e) => {
  if (bdStart) sizeMark(bdDiv, bdHit.r, bdStart, { x: e.clientX, y: e.clientY });
});
els.viewerWrap.addEventListener('pointerup', async (e) => {
  if (!bdStart) return;
  const hit = bdHit, start = bdStart;
  bdDiv.remove();
  bdStart = null; bdDiv = null; bdHit = null;
  const r = hit.r;
  // Clamped to the page the drag started on — see clampToRect. A page fraction outside
  // [0,1] describes a box that is not on the page it names.
  const bp = clampToRect(r, { x: e.clientX, y: e.clientY }), bs = clampToRect(r, start);
  const fw = Math.abs(bp.x - bs.x) / r.width;
  const fh = Math.abs(bp.y - bs.y) / r.height;
  if (fw < 0.01 || fh < 0.01) return; // ignore a stray click
  const fx0 = (Math.min(bs.x, bp.x) - r.left) / r.width;
  const fy0 = (Math.min(bs.y, bp.y) - r.top) / r.height;
  const owner = view; // captured before the await — everything after it names this document
  const base = (await owner.pdfDocument.getPage(hit.n)).getViewport({ scale: 1 }); // PDF points
  makeBox([fx0, fy0, fx0 + fw, fy0 + fh], { page: hit.n, pageW: base.width, pageH: base.height }, owner);
});

// makeBox registers a draggable/resizable outline overlay (kind 'box'), styled
// live with the chosen color and thickness; collectStamps bakes it via boxPNG.
function makeBox(frac, opts, owner = view) {
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

  const remove = () => deleteField(f, true, owner);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  el.addEventListener('keydown', (ev) => { if (ev.key === 'Delete' || ev.key === 'Backspace') remove(); });
  enableStampGestures(f, el, handle);

  owner.overlayFields.push(f);
  const pv = owner.viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f, owner);
}

// --- Dropdown tool: place a fillable dropdown (combobox) field ----------------
// A box-draw tool like Border, but the overlay carries an options list; on "Save
// as fillable form…" it authors a real AcroForm combobox (see collectAuthorFields
// → /api/form/author → pdfops.AuthorForm). Options are typed inline on the field.
let ddStart = null, ddDiv = null, ddHit = null;
function reflectDropdown() { els.dropdownBtn.classList.toggle('active', view.dropdownMode); }
function exitDropdown() {
  if (!view.dropdownMode) return;
  view.dropdownMode = false;
  reflectDropdown();
  els.viewerWrap.style.cursor = '';
}
els.dropdownBtn.onclick = () => {
  if (view.dropdownMode) { exitDropdown(); return; }
  if (!view.pdfDocument) { toast('Open a PDF first'); return; }
  view.dropdownMode = true;
  setTool(null);
  setMarkerMode(null);
  if (view.redactMode) { view.redactMode = false; reflectRedact(); }
  if (view.editMode) { view.editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitNote(); exitRadio(); // clear the sibling one-at-a-time tools (not dropdown — we're entering it)
  exitBorder();
  exitShape();
  reflectDropdown();
  els.viewerWrap.style.cursor = 'crosshair';
};
els.viewerWrap.addEventListener('pointerdown', (e) => {
  if (!view.dropdownMode) return;
  if (!startedInActiveView(e)) return;
  if (onExistingOverlay(e)) return;
  ddHit = pageAt(e.clientX, e.clientY);
  if (!ddHit) return;
  ddStart = { x: e.clientX, y: e.clientY };
  ddDiv = document.createElement('div');
  ddDiv.className = 'bordermark';
  ddHit.pv.div.appendChild(ddDiv);
  sizeMark(ddDiv, ddHit.r, ddStart, ddStart);
  e.preventDefault();
});
els.viewerWrap.addEventListener('pointermove', (e) => {
  if (ddStart) sizeMark(ddDiv, ddHit.r, ddStart, { x: e.clientX, y: e.clientY });
});
els.viewerWrap.addEventListener('pointerup', async (e) => {
  if (!ddStart) return;
  const hit = ddHit, start = ddStart;
  ddDiv.remove();
  ddStart = null; ddDiv = null; ddHit = null;
  const r = hit.r;
  // Clamped to the page the drag started on — see clampToRect. A page fraction outside
  // [0,1] describes a box that is not on the page it names.
  const bp = clampToRect(r, { x: e.clientX, y: e.clientY }), bs = clampToRect(r, start);
  const fw = Math.abs(bp.x - bs.x) / r.width;
  const fh = Math.abs(bp.y - bs.y) / r.height;
  if (fw < 0.01 || fh < 0.01) return; // ignore a stray click
  const fx0 = (Math.min(bs.x, bp.x) - r.left) / r.width;
  const fy0 = (Math.min(bs.y, bp.y) - r.top) / r.height;
  const owner = view; // captured before the await — see makeBox's caller
  const base = (await owner.pdfDocument.getPage(hit.n)).getViewport({ scale: 1 });
  makeDropdown([fx0, fy0, fx0 + fw, fy0 + fh], { page: hit.n, pageW: base.width, pageH: base.height }, owner);
});

// makeDropdown registers a draggable/resizable dropdown overlay (kind 'dropdown')
// with an inline comma-separated options input; the options ride to the server in
// collectAuthorFields, where each becomes a combobox choice.
function makeDropdown(frac, opts, owner = view) {
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

  const remove = () => deleteField(f, true, owner);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  // Delete/Backspace removes the field only when the box (not the options input) is focused.
  el.addEventListener('keydown', (ev) => { if ((ev.key === 'Delete' || ev.key === 'Backspace') && ev.target === el) remove(); });
  enableStampGestures(f, el, handle);

  owner.overlayFields.push(f);
  const pv = owner.viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f, owner);
  input.focus();
}

// Radio-group tool — a sibling of the Dropdown tool. Draw a box to anchor a radio
// group, type ≥2 comma-separated choices; "Save as fillable form…" authors a real
// AcroForm radiobuttongroup whose buttons march to the right of the anchor, each
// labelled with its value. (Horizontal only for now.)
let rdStart = null, rdDiv = null, rdHit = null;
function reflectRadio() { els.radioBtn.classList.toggle('active', view.radioMode); }
function exitRadio() {
  if (!view.radioMode) return;
  view.radioMode = false;
  reflectRadio();
  els.viewerWrap.style.cursor = '';
}
els.radioBtn.onclick = () => {
  if (view.radioMode) { exitRadio(); return; }
  if (!view.pdfDocument) { toast('Open a PDF first'); return; }
  view.radioMode = true;
  setTool(null);
  setMarkerMode(null);
  if (view.redactMode) { view.redactMode = false; reflectRedact(); }
  if (view.editMode) { view.editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitNote(); exitDropdown();
  exitBorder();
  exitShape();
  view.radioMode = true;
  reflectRadio();
  els.viewerWrap.style.cursor = 'crosshair';
};
els.viewerWrap.addEventListener('pointerdown', (e) => {
  if (!view.radioMode) return;
  if (!startedInActiveView(e)) return;
  if (onExistingOverlay(e)) return;
  rdHit = pageAt(e.clientX, e.clientY);
  if (!rdHit) return;
  rdStart = { x: e.clientX, y: e.clientY };
  rdDiv = document.createElement('div');
  rdDiv.className = 'bordermark';
  rdHit.pv.div.appendChild(rdDiv);
  sizeMark(rdDiv, rdHit.r, rdStart, rdStart);
  e.preventDefault();
});
els.viewerWrap.addEventListener('pointermove', (e) => {
  if (rdStart) sizeMark(rdDiv, rdHit.r, rdStart, { x: e.clientX, y: e.clientY });
});
els.viewerWrap.addEventListener('pointerup', async (e) => {
  if (!rdStart) return;
  const hit = rdHit, start = rdStart;
  rdDiv.remove();
  rdStart = null; rdDiv = null; rdHit = null;
  const r = hit.r;
  // Clamped to the page the drag started on — see clampToRect. A page fraction outside
  // [0,1] describes a box that is not on the page it names.
  const bp = clampToRect(r, { x: e.clientX, y: e.clientY }), bs = clampToRect(r, start);
  const fw = Math.abs(bp.x - bs.x) / r.width;
  const fh = Math.abs(bp.y - bs.y) / r.height;
  if (fw < 0.01 || fh < 0.01) return; // ignore a stray click
  const fx0 = (Math.min(bs.x, bp.x) - r.left) / r.width;
  const fy0 = (Math.min(bs.y, bp.y) - r.top) / r.height;
  const owner = view; // captured before the await — see makeBox's caller
  const base = (await owner.pdfDocument.getPage(hit.n)).getViewport({ scale: 1 });
  makeRadio([fx0, fy0, fx0 + fw, fy0 + fh], { page: hit.n, pageW: base.width, pageH: base.height }, owner);
});

// makeRadio registers a draggable/resizable radio-group overlay (kind 'radio')
// with an inline comma-separated choices input (≥2); the options ride to the
// server in collectAuthorFields, where each becomes a radio button value.
function makeRadio(frac, opts, owner = view) {
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

  const remove = () => deleteField(f, true, owner);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  // Delete/Backspace removes the field only when the box (not the choices input) is focused.
  el.addEventListener('keydown', (ev) => { if ((ev.key === 'Delete' || ev.key === 'Backspace') && ev.target === el) remove(); });
  enableStampGestures(f, el, handle);

  owner.overlayFields.push(f);
  const pv = owner.viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f, owner);
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
let shapeType = 'line';
let shStart = null, shCanvas = null, shHit = null;

function reflectShape() {
  els.shapeBtn.classList.toggle('active', view.shapeMode);
  reflectAnnoControls();
}
function exitShape() {
  if (!view.shapeMode) return;
  view.shapeMode = false;
  reflectShape();
  els.viewerWrap.style.cursor = '';
}
els.shapeBtn.onclick = () => {
  if (view.shapeMode) { exitShape(); return; }
  if (!view.pdfDocument) { toast('Open a PDF first'); return; }
  view.shapeMode = true;
  setTool(null);
  setMarkerMode(null);
  if (view.redactMode) { view.redactMode = false; reflectRedact(); }
  if (view.editMode) { view.editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitNote(); exitDropdown(); exitRadio();
  exitBorder();
  reflectShape();
  els.viewerWrap.style.cursor = 'crosshair';
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

els.viewerWrap.addEventListener('pointerdown', (e) => {
  if (!view.shapeMode) return;
  if (!startedInActiveView(e)) return;
  if (onExistingOverlay(e)) return;
  shHit = pageAt(e.clientX, e.clientY);
  if (!shHit) return;
  shStart = { x: e.clientX, y: e.clientY };
  shCanvas = document.createElement('canvas');
  shCanvas.className = 'shapemark';
  shHit.pv.div.appendChild(shCanvas);
  e.preventDefault();
});
els.viewerWrap.addEventListener('pointermove', (e) => {
  if (!shStart) return;
  const r = shHit.r;
  const w = clampWeight(els.borderWidthInput.value);
  const isLine = shapeType === 'line' || shapeType === 'arrow';
  const pad = isLine ? shapePad(w) : 0; // px; head/cap room (none for rect/ellipse)
  // Clamped like every other tool's preview — sizeMark does this for the seven that use it,
  // and the shape tool paints its own canvas, so it does the same here.
  const sp = clampToRect(r, { x: e.clientX, y: e.clientY }), ss = clampToRect(r, shStart);
  const g = shapeGeom(ss.x - r.left, ss.y - r.top, sp.x - r.left, sp.y - r.top, pad, pad);
  shCanvas.style.left = g.x0 + 'px'; shCanvas.style.top = g.y0 + 'px';
  shCanvas.width = Math.max(1, Math.round(g.w)); shCanvas.height = Math.max(1, Math.round(g.h));
  paintShape(shCanvas.getContext('2d'), shCanvas.width, shCanvas.height, shapeType, els.shapeFill.checked, selectedHlColor, w, g.from, g.to);
});
els.viewerWrap.addEventListener('pointerup', async (e) => {
  if (!shStart) return;
  const hit = shHit, start = shStart;
  shCanvas.remove(); shStart = null; shCanvas = null; shHit = null;
  const r = hit.r;
  // Clamped to the page the drag started on — see clampToRect. The shape tool draws onto
  // its own canvas rather than through sizeMark, so its preview is bounded by the same
  // clamp applied to the geometry below rather than by sizeMark's.
  const shp = clampToRect(r, { x: e.clientX, y: e.clientY }), shs = clampToRect(r, start);
  const isLine = shapeType === 'line' || shapeType === 'arrow';
  const fw = Math.abs(shp.x - shs.x) / r.width, fh = Math.abs(shp.y - shs.y) / r.height;
  if (isLine ? (fw < 0.005 && fh < 0.005) : (fw < 0.01 || fh < 0.01)) return; // ignore a stray click
  const owner = view; // captured before the await — see makeBox's caller
  const base = (await owner.pdfDocument.getPage(hit.n)).getViewport({ scale: 1 }); // PDF points
  // Line/arrow: grow the holding box by an arrowhead-sized margin (in points, per
  // axis) and inset the endpoints so the head clears the box edge; this also makes
  // an axis-aligned line's box non-degenerate. rect/ellipse keep a tight bbox.
  const pad = isLine ? shapePad(clampWeight(els.borderWidthInput.value)) : 0;
  const g = shapeGeom((shs.x - r.left) / r.width, (shs.y - r.top) / r.height,
                      (shp.x - r.left) / r.width, (shp.y - r.top) / r.height,
                      pad / base.width, pad / base.height);
  makeShape([g.x0, g.y0, g.x0 + g.w, g.y0 + g.h],
    { page: hit.n, pageW: base.width, pageH: base.height, type: shapeType, fill: els.shapeFill.checked, from: g.from, to: g.to }, owner);
});

// makeShape registers a draggable/resizable shape overlay (kind 'shape') whose
// canvas redraws via layoutField; collectStamps bakes it via shapeMarkPNG.
function makeShape(frac, opts, owner = view) {
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

  const remove = () => deleteField(f, true, owner);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  el.addEventListener('keydown', (ev) => { if (ev.key === 'Delete' || ev.key === 'Backspace') remove(); });
  enableStampGestures(f, el, handle);

  owner.overlayFields.push(f);
  const pv = owner.viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f, owner);
}

// --- comment notes -----------------------------------------------------------
// A note is a Nib overlay (a small text card) you place, drag, and edit; at save
// it bakes into a native /Text sticky-note annotation (a clickable icon whose
// popup shows the comment) via pdfops.AddNotes.
function reflectNote() { els.noteBtn.classList.toggle('active', view.noteMode); }
function exitNote() {
  if (!view.noteMode) return;
  view.noteMode = false;
  reflectNote();
  els.viewerWrap.style.cursor = '';
}
els.noteBtn.onclick = () => {
  if (view.noteMode) { exitNote(); exitDropdown(); exitRadio(); return; }
  if (!view.pdfDocument) { toast('Open a PDF first'); return; }
  view.noteMode = true;
  setTool(null);
  setMarkerMode(null);
  if (view.redactMode) { view.redactMode = false; reflectRedact(); }
  if (view.editMode) { view.editMode = false; reflectEdit(); }
  exitSplitBox();
  exitCrop();
  exitBorder();
  exitShape();
  reflectNote();
  els.viewerWrap.style.cursor = 'crosshair';
};
els.viewerWrap.addEventListener('pointerdown', async (e) => {
  if (!view.noteMode) return;
  if (!startedInActiveView(e)) return;
  if (onExistingOverlay(e)) return;
  const hit = pageAt(e.clientX, e.clientY);
  if (!hit) return;
  e.preventDefault();
  const r = hit.r;
  const fx = (e.clientX - r.left) / r.width;
  const fy = (e.clientY - r.top) / r.height;
  const owner = view; // captured before the await — see makeBox's caller
  const base = (await owner.pdfDocument.getPage(hit.n)).getViewport({ scale: 1 }); // PDF points
  const fw = Math.min(0.3, 150 / r.width), fh = Math.min(0.2, 72 / r.height); // default card size
  makeNote([fx, fy, Math.min(fx + fw, 1), Math.min(fy + fh, 1)], { page: hit.n, pageW: base.width, pageH: base.height }, owner);
  exitNote(); exitDropdown(); exitRadio(); // place one; re-click the tool for another
});

// makeNote registers a draggable note card (kind 'note') with an inline comment
// textarea; collectNotes turns it into a /Text annotation at bake.
function makeNote(frac, opts, owner = view) {
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

  const remove = () => deleteField(f, true, owner);
  del.onclick = (ev) => { ev.stopPropagation(); remove(); };
  enableStampGestures(f, el, handle);

  owner.overlayFields.push(f);
  const pv = owner.viewer.getPageView(f.page - 1);
  pv.div.appendChild(el);
  layoutField(f, pv);
  recordAdd(f, owner);
  ta.focus();
}

// collectNotes turns each non-empty note overlay into a {page, x, y, text}: the
// icon anchors at the card's top-left corner, in PDF points.
function collectNotes(owner = view) {
  const out = [];
  for (const f of owner.overlayFields) {
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
// reaches reflectSignControls() -> markerFields(), which reads view.overlayFields, so the
// binding must already be initialized (a `let` declared later would throw on its TDZ).
let libraryImages = []; // cached /api/images list (the image-library panel)
const DOC_REQUIRED = [
  'saveFlatBtn', 'saveEditableBtn', 'saveFillableBtn', 'printBtn',
  'exportZipBtn', 'exportPngBtn', 'exportFormJsonBtn', 'exportFormCsvBtn', 'exportFormXfdfBtn', 'exportTableXlsxBtn', 'exportTableCsvBtn', 'exportTableOdsBtn', 'exportBookmarkSplitBtn',
  'exportPageSplitBtn', 'pdfaBtn',
  // The whole drawing row, not the first three of it (/pending 331). Border, Note,
  // Dropdown, Radio and Shapes sit beside Text/Highlight/Draw in the Edit tab and were
  // never added here, so they stayed clickable on a clean vault with nothing open —
  // arming a tool that then has no page to act on. They carry no `data-mode` either, so
  // the toolbar's data-mode sweep below did not reach them as it reaches the first three.
  'textToolBtn', 'highlightToolBtn', 'drawToolBtn',
  'borderBtn', 'noteBtn', 'dropdownBtn', 'radioBtn', 'shapeBtn',
  'detectBtn', 'editTextBtn', 'removeOriginalsBtn', 'ocrBtn', 'ocrLang', 'ocrQuality', 'autofillBtn', 'splitBtn',
  'splitBoxBtn', 'applyBoxSplitBtn', 'rotateLeftBtn', 'rotateRightBtn',
  'extractBtn', 'insertBlankBtn', 'duplicatePageBtn', 'insertPdfBtn', 'pageNumBtn', 'pageLabelsBtn', 'nupBtn', 'normalizeBtn', 'cropBtn',
  'redactBtn', 'redactTextBtn', 'applyRedactBtn', 'scanBtn', 'attachBtn', 'encryptBtn', 'decryptBtn', 'compareBtn', 'fillCsvBtn', 'importXfdfBtn',
  'closeBtn',
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

// Undo/Redo enable from the server's per-document history flags (view.docMeta.canUndo/
// canRedo, refreshed on every load); both off when no document is open.
function reflectUndoControls(enabled) {
  const m = view.docMeta || {};
  if (els.undoBtn) els.undoBtn.disabled = !(enabled && (m.canUndo || view.overlayHistory.undo.length));
  if (els.redoBtn) els.redoBtn.disabled = !(enabled && (m.canRedo || view.overlayHistory.redo.length));

  // Eviction is observable or it is not eviction (ADR-003). The server has always
  // reported historyEvicted when it dropped a document's history whole to keep the
  // global budget, and nothing in the client read it — so the user's undo button
  // simply stopped reaching, and `canUndo:false` reads identically for "you have
  // made no edits" and "your edits are no longer undoable". That is the exact
  // ambiguity the ADR says whole-history eviction exists to resolve, left
  // unresolved because the last step was missing.
  //
  // Told once per eviction, on the control itself rather than as a toast: a toast
  // for something that happened while the user was working on another document is
  // gone before they look, and this is a standing fact about the button, not an
  // event.
  if (els.undoBtn) {
    els.undoBtn.title = m.historyEvicted
      ? 'Undo — earlier history for this document was released to stay within the memory budget'
      : 'Undo';
    els.undoBtn.classList.toggle('evicted', !!m.historyEvicted);
  }
  if (m.historyEvicted && view.lastEvictionSeen !== view.docMeta.id) {
    view.lastEvictionSeen = view.docMeta.id;
    toast('Earlier undo history for this document was released to stay within the memory budget');
  }
}

// --- client overlay-edit undo (P2) ------------------------------------------
// Placing, deleting, or moving/resizing an overlay (stamp, cover-edit, border,
// shape, note, marker) is client-only — no server op — so the server undo ring
// never sees it. This small command stack records those edits and is drained by
// Ctrl+Z *before* falling through to the server undo. Because every server op
// reloads through setDocumentFromServer -> clearOverlays (which clears this
// stack too), it only ever holds the newest run of un-baked edits, so "client
// edits first, then server ops" is the correct chronological order for free.
function recordOverlayEdit(cmd, owner = view) {
  owner.dirty = true; // the single funnel for every recorded overlay add, delete and move
  owner.overlayHistory.undo.push(cmd);
  owner.overlayHistory.redo = [];
  reflectUndoControls(!!view.pdfDocument);
}
function clearOverlayHistory(owner = view) { owner.overlayHistory.undo = []; owner.overlayHistory.redo = []; }
// detachField/reattachField toggle a field's presence without rebuilding it — the
// DOM element survives in the command closure, so add/delete undo is just a
// re-attach or detach. layoutFieldNow repositions a still-attached field (moves).
// All three take the view that OWNS the field. Every stored-closure use of them lives in a
// recordOverlayEdit command, and a command outliving a switch would otherwise splice one
// document's field into another's list and append its element into another's page div.
//
// With the stack now per-view and drained only while its view is active, the owner IS the
// active view at drain time — so reading `view` here would be correct today. The parameter
// goes in anyway, so the property holds by construction rather than by depending on that
// invariant. Same reasoning as P05.S05's captured dragGrid.
function detachField(owner, f) {
  f.el.remove();
  owner.overlayFields = owner.overlayFields.filter((o) => o !== f);
}
function reattachField(owner, f) {
  owner.overlayFields.push(f);
  const pv = owner.viewer.getPageView(f.page - 1);
  if (pv?.div) { pv.div.appendChild(f.el); layoutField(f, pv); }
}
function layoutFieldNow(owner, f) {
  const pv = owner.viewer.getPageView(f.page - 1);
  if (pv?.div) layoutField(f, pv);
}
// recordAdd / deleteField are the recorded create/remove used by the overlay
// factories and the per-overlay × buttons. recordMove records one command per
// move/resize gesture. detachField alone (no record) is the bulk-wipe path.
function recordAdd(f, owner = view) {
  recordOverlayEdit({ undo: () => detachField(owner, f), redo: () => reattachField(owner, f) }, owner);
}
function deleteField(f, record = true, owner = view) {
  if (record) recordOverlayEdit({ undo: () => reattachField(owner, f), redo: () => detachField(owner, f) }, owner);
  detachField(owner, f);
}
function recordMove(f, before, after, owner = view) {
  recordOverlayEdit({
    undo: () => { f.frac = before.slice(); layoutFieldNow(owner, f); },
    redo: () => { f.frac = after.slice(); layoutFieldNow(owner, f); },
  }, owner);
}
function undoOverlayEdit() {
  const c = view.overlayHistory.undo.pop();
  c.undo(); view.overlayHistory.redo.push(c);
  reflectUndoControls(!!view.pdfDocument);
}
function redoOverlayEdit() {
  const c = view.overlayHistory.redo.pop();
  c.redo(); view.overlayHistory.undo.push(c);
  reflectUndoControls(!!view.pdfDocument);
}
// undoAny/redoAny are the single dispatch for both Ctrl+Z and the ↶/↷ buttons:
// drain the client overlay stack first, then fall through to the server ring.
// The ACTIVE view's stack, deliberately: Ctrl+Z aims at the document on screen. Every command
// is pushed onto the same owner it closes over, so one drained from `view`'s stack has
// `owner === view` by construction rather than by an invariant about record timing. That
// holds for the field mutation; the two marker commands also call reflectSignControls(),
// which repaints shared chrome from the active view — correct here only because a drain
// implies the owner is active.
function undoAny() { if (view.overlayHistory.undo.length) undoOverlayEdit(); else doUndo(); }
function redoAny() { if (view.overlayHistory.redo.length) redoOverlayEdit(); else doRedo(); }

// doUndo/doRedo revert or re-apply the last server-side document operation (page
// ops, outline, sanitize, attachments). The server returns fresh doc metadata and
// the view reloads through the universal setDocumentFromServer path.
async function doUndo() {
  // Pinned and owned: the id names the document whose history is being walked, and
  // the reload lands on the view that asked. Unpinned, an undo issued on A and
  // answered after a switch reverted B a step and installed the result over A.
  const owner = view;
  const doc = owner.docMeta;
  if (!owner.pdfDocument || !(doc && doc.canUndo)) return;
  const res = await apiFetch('/api/undo', { method: 'POST', docId: doc && doc.id });
  if (!res.ok) { toast('undo failed'); return; }
  await setDocumentFromServer(await res.json(), owner);
}
async function doRedo() {
  const owner = view;
  const doc = owner.docMeta;
  if (!owner.pdfDocument || !(doc && doc.canRedo)) return;
  const res = await apiFetch('/api/redo', { method: 'POST', docId: doc && doc.id });
  if (!res.ok) { toast('redo failed'); return; }
  await setDocumentFromServer(await res.json(), owner);
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
// `owner` is the view being torn down — the active one unless a background load says
// otherwise. It is not decoration: this function empties overlayFields, nulls the marker
// bindings and clears redactMarks, so running it against the active view during an
// ARRIVAL's load destroys the typed values and redaction marks on the document the user is
// looking at. That is the exact defect openArrivalInNewView exists to remove, and it was
// still live after the call site moved, because the call site was not the destroyer.
function clearOverlays(owner = view) {
  owner.overlayFields.forEach((f) => f.el.remove());
  owner.overlayFields = [];
  owner.activeMarker = null; owner.fillTarget = null; // markers are gone with the old document
  owner.redactMarks = []; // pending redaction boxes don't carry to a new/reloaded document
  // ABOVE the return, because the undo stack is per-view DATA rather than shared chrome —
  // its entries are closures over the overlay elements this function removed at the top.
  // Left below it, a background reload would keep a stack whose reattachField pushes a
  // pre-reload field back in and appends its element to a page div from the new document.
  // It also closes a second defect nobody had named: the old shared clear wiped the ACTIVE
  // document's undo stack whenever a background document reloaded.
  clearOverlayHistory(owner); // a new/reloaded document resets the overlay-edit undo stack
  // The draw-mode exits repaint SHARED toolbar buttons and the shared cursor, so they are
  // the active view's business only. A background view is freshly built and has none armed.
  if (owner !== view) return;
  exitSplitBox(); // a pending region selection doesn't carry to a new document
  exitBorder();   // nor a pending border-draw mode
  exitShape();    // nor a pending shape-draw mode
  exitCrop();     // nor a pending crop-draw mode
  exitNote(); exitDropdown(); exitRadio();     // nor a pending note-placement mode
}

// resetSharedDocState is the ONE teardown both the open path and the close path
// call, so a Close can never be stricter than an open — two teardowns that drift
// is the whole failure this slice exists to avoid.
//
// clearOverlays covers the seven Nib draw modes it can exit, but four armed modes
// were never in it: redactMode, editMode, markerMode and activeTool. So opening a
// second document already leaves a lit tool button and a crosshair cursor
// describing a mode whose marks were just wiped — a pre-existing open-over-open
// bug that this fixes as a side effect. That widening is deliberate and is the
// only behaviour change this makes to an existing path.
//
// It deliberately does NOT touch viewer.annotationEditorMode: pdf.js resets its
// own editor mode inside setDocument (destroying the editor UI manager and
// setting the mode to NONE), on both the null and the new-document call. Doing it
// here as well would be a second expression of one rule — and on the close path
// it would run against a manager that no longer exists.
//
// Session-scoped state stays: markerSig/markerInit are remembered fill sources for
// the session, not the document, so clearing them here would be a regression.
function resetSharedDocState(owner = view) {
  clearOverlays(owner);
  // Everything below repaints shared chrome. A background load records nothing here and
  // paints nothing; activateView is what makes the chrome show a view.
  if (owner !== view) return;
  setMarkerMode(null); // clears the lit .markers button and the cursor
  if (view.redactMode) { view.redactMode = false; reflectRedact(); }
  if (view.editMode) { view.editMode = false; reflectEdit(); }
  view.activeTool = null;
  document.querySelectorAll('[data-mode]:not(.cmmode)').forEach((b) => b.classList.remove('active'));
  reflectAnnoControls();
  els.viewerWrap.style.cursor = '';
  // Compare goes with the document it was computed against (D11).
  //
  // The switch path already did this — activateView calls closeDocBoundModals — and the
  // OPEN path never did, which is the standing defect: `closeCmpDoc` had four call sites
  // and setDocumentFromServer was not among them, so opening a different document left
  // the modal on screen showing a cached text diff of the PREVIOUS document against the
  // picked file, with the viewer behind it showing something else. `cmpText` is a cache,
  // so the stale content was retained rather than re-derived on the next look. Reachable
  // by ordinary use: #compareModal has no scrim, so Open… stays clickable while Compare
  // is open. Its home is here because this is the shared reset — the exact place whose
  // job is to stop the open and close paths disagreeing.
  els.compareModal.hidden = true;
  closeCmpDoc();
}
// DETECTED_KINDS is exactly what makeField produces — the auto-detected widgets a
// re-run of Detect is entitled to replace. Everything else on the page was put
// there by the user.
const DETECTED_KINDS = ['text', 'check', 'circleone'];

// clearDetected drops only auto-detected fields, keeping user-placed stamps, text
// edits, markers, boxes, notes, shapes, dropdowns and radio groups — so re-running
// Detect doesn't wipe a signature, a cover-and-replace edit, or a placed field.
//
// Stated as what to DELETE, not what to keep. The keep-list this replaces named
// seven kinds and omitted 'radio', so re-running Detect silently destroyed every
// radio group the user had drawn — with its typed option strings — while its
// sibling 'dropdown', created ninety lines earlier by near-identical code,
// survived. A keep-list has to be updated every time a kind is added and fails
// DESTRUCTIVELY when it is not; a delete-list of the three detected kinds fails
// safe, because an unrecognised kind is kept.
function clearDetected() {
  view.overlayFields = view.overlayFields.filter((f) => {
    if (!DETECTED_KINDS.includes(f.kind)) return true;
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
// HOT PATH — runs on scroll and zoom. It walks ONE view's fields, the owning one, and
// never a collection of views: a version iterating every open document turns the path a
// user feels most into an N x regression (ADR-002's hot-path constraint).
function relayoutOverlays(owner) {
  for (const f of owner.overlayFields) {
    const pv = owner.viewer.getPageView(f.page - 1);
    if (pv?.div && pv.viewport) {
      if (f.el.parentElement !== pv.div) pv.div.appendChild(f.el);
      layoutField(f, pv);
    }
  }
}
// Bound per view in newView(), with the owning view passed in — see the hot-path
// note on relayoutOverlays above.

// page-fraction (top-left origin) -> PDF points (bottom-left origin)
function rectPoints(f, frac) {
  const [fx0, fy0, fx1, fy1] = frac;
  return [fx0 * f.pageW, (1 - fy1) * f.pageH, fx1 * f.pageW, (1 - fy0) * f.pageH];
}

function collectFields(owner = view) {
  const out = [];
  for (const f of owner.overlayFields) {
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
  for (const f of view.overlayFields) {
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
function collectCovers(owner = view) {
  const out = [];
  for (const f of owner.overlayFields) {
    if (f.kind !== 'edit') continue;
    const rect = rectPoints(f, f.coverFrac || f.frac);
    out.push({ page: f.page, rect, png: coverPNG(rect[2] - rect[0], rect[3] - rect[1], f.bg) });
  }
  return out;
}

// collectStamps gathers image stamps: placed images/quick-stamps (library id or
// inline PNG), circled choices (a pill/ellipse PNG over the picked option), and
// checkbox X's.
function collectStamps(owner = view) {
  const out = [];
  for (const f of owner.overlayFields) {
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
// docId threads the CALLER's capture down. bakedBytes is entered from operations that
// have usually already awaited, so its own entry is not a safe capture point — the
// default exists only for the call sites that enter it first thing.
async function bakedBytes(docId = view.docMeta && view.docMeta.id, owner = view) {
  // The request was already pinned by docId; the PREDICATE that decides whether to strip
  // NibFlags was not. Read after the bake round-trip, a save of a flagged document that
  // raced a switch got the OTHER document's answer, and the finished signed file kept its
  // flags and reopened in signing mode.
  // saveDocument() bakes pdf.js annotation-storage edits (form fills, FREETEXT/
  // INK/HIGHLIGHT/STAMP) and warns + does a needless rewrite when storage is
  // empty. Our overlay edits (covers/fields/stamps) bake server-side via
  // /api/bake, so when there are no pdf.js edits, getData() is the library's
  // prescribed lighter equivalent (raw bytes, no bake).
  const saved = owner.pdfDocument.annotationStorage.size > 0
    ? await owner.pdfDocument.saveDocument()
    : await owner.pdfDocument.getData();
  // Threaded, not merely accepted. bakedBytes took an `owner` and used it for
  // owner.pdfDocument and owner.docHadFlags while these four read the ACTIVE view's
  // overlayFields — so the parameter read as if the function were owner-safe when
  // the first caller to use it would have baked one document's pages with another
  // document's fields, stamps, covers and notes. No caller passed it, so nothing
  // exposed that.
  const fields = collectFields(owner);
  const stamps = collectStamps(owner);
  const covers = collectCovers(owner);
  const notes = collectNotes(owner);
  let out = saved;
  if (fields.length || stamps.length || covers.length || notes.length) {
    const form = new FormData();
    form.append('pdf', new Blob([saved], { type: 'application/pdf' }), 'doc.pdf');
    if (covers.length) form.append('covers', JSON.stringify(covers));
    if (fields.length) form.append('fields', JSON.stringify(fields));
    if (stamps.length) form.append('stamps', JSON.stringify(stamps));
    if (notes.length) form.append('notes', JSON.stringify(notes));
    const res = await apiFetch('/api/bake', { method: 'POST', body: form, docId });
    // A failed bake must abort the whole operation: returning the un-baked bytes
    // here would let save/print/flatten/sign proceed with a document silently
    // missing the user's covers, fields, stamps, and notes.
    if (!res.ok) throw new Error(await errText(res, 'could not apply edits'));
    out = new Uint8Array(await res.arrayBuffer());
  }
  // A document opened with embedded signing flags carries the NibFlags property,
  // which the bake preserves — strip it from any baked output so the finished
  // file doesn't reopen in signing mode. (Server no-op once it's already gone.)
  if (owner.docHadFlags) {
    try { out = await embedFlags(out, null, docId); } catch (e) {
      console.error('flag strip failed', e);
      toast('warning: could not remove the signing flags — this file may reopen in signing mode');
    }
  }
  return out;
}

// bakedForm builds the multipart body every endpoint that takes the current
// document shares: the baked bytes as the "pdf" file part. Callers append their
// own extra fields (params, ots, appearance, address, op) before sending.
async function bakedForm(owner = view) {
  const form = new FormData();
  const doc = owner.docMeta;
  form.append('pdf', new Blob([await bakedBytes(doc && doc.id, owner)], { type: 'application/pdf' }), 'doc.pdf');
  return form;
}

els.detectBtn.onclick = async () => {
  // Captured at entry: a page render and a getTextContent sit between here and the
  // makeField calls at the end, and each of those builds a field for THIS document.
  const owner = view;
  if (!owner.pdfDocument) { toast('Open a PDF first'); return; }
  clearDetected();
  const n = owner.viewer.currentPageNumber;
  const pv = owner.viewer.getPageView(n - 1);
  if (!pv?.div || !pv.viewport) { toast('Scroll the page into view, then try again'); return; }

  // Render the page to an offscreen canvas at a consistent resolution, so
  // detection doesn't depend on the current zoom (faint thin rules need it).
  const page = await owner.pdfDocument.getPage(n);
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
    makeField('text', [c.x0 / W, c.y0 / H, c.x1 / W, c.y1 / H], { page: n, pageW, pageH }, pv, owner);
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
    makeField(kind, [fx0, fy0, fx1, fy1], { page: n, pageW, pageH }, pv, owner);
    added++;
  }

  // 3. "Circle one" choices (incl. Y/N) become a circle-my-answer widget: a
  //    radio set of choices, each circled (pill around a word) when picked.
  const choiceGroups = dedupeGroups([...ynItems, ...findCircleOne(textItems), ...findSlashTemplates(textItems), ...findPipeChoices(textItems), ...findRunChoices(textItems, cells)]);
  // The canvas is read ONCE for the whole loop. snapChoices used to read it per group —
  // a full getImageData over a page-sized canvas, ~13 MB each, for a picture that does
  // not change between groups. Skipped entirely when there are no groups.
  const groupPixels = choiceGroups.length ? pixelsOf(canvas) : null;
  for (const grp of choiceGroups) {
    const choices = snapChoices(canvas, grp.choices, grp.marker);
    const cf = choices.map((c) => ({ rect: [c.x0 / W, c.y0 / H, c.x1 / W, c.y1 / H], word: !!c.word }));
    const x0 = Math.min(...cf.map((c) => c.rect[0])), y0 = Math.min(...cf.map((c) => c.rect[1]));
    const x1 = Math.max(...cf.map((c) => c.rect[2])), y1 = Math.max(...cf.map((c) => c.rect[3]));
    makeField('circleone', [x0, y0, x1, y1], { page: n, pageW, pageH, choices: cf }, pv, owner);
    added++;
  }

  toast(added ? `Added ${added} fillable field(s) — fill, then Save` : 'Nothing detected on this page');
};


// makeField creates an auto-detected overlay widget of the given kind and rect
// (page fractions, top-left origin) and registers it. kinds: text, check,
// circleone (circle one of N choices). Signatures aren't a field kind — place a
// signature image from the library instead.
function makeField(kind, frac, opts, pv, owner = view) {
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
  owner.dirty = true; // detected blanks record no undo command, so the funnel misses them
  owner.overlayFields.push(f);
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
// Retry is what makes the banner an affordance rather than an epitaph: the common cause is
// transient, and without this the only way back to the document the server holds is to
// close the tab and open the file again.
els.staleRetry.onclick = () => { if (view.docMeta) setDocumentFromServer(view.docMeta, view); };

// Reload from disk is the escape hatch /pending 333 was really asking for: the item's
// own words were "a hard reload didn't update it", and it could not, because a browser
// reload re-fetches from the same in-memory copy the server has held since open.
//
// It goes through /api/open rather than a new reload route, deliberately. That path
// already re-reads the file, and already carries the size cap, the LooksLikePDF gate
// and the document cap — a refresh route would be a second, less-checked way to install
// a document, which is the mistake openHandedOff's own comment names. The cost is that
// the document gets a NEW id, which is honest: the bytes are different, so it is a
// different document, and pinning it as the same one would be the lie.
//
// Open BEFORE close, never the other way round. If the open is refused — the document
// cap is the reachable case — the user keeps the tab she has instead of losing it to a
// failure, and the toast tells her why.
els.staleReload.onclick = async () => {
  const target = view;
  const d = target.docMeta;
  if (!d || !d.path) return;
  if (hasUnsavedWork(target) && !confirm('Reload ' + (d.name || 'this document') + ' from disk? Your unsaved changes to it will be lost.')) return;
  const before = views.length;
  await openPath(d.path);
  // Only close the stale view if a new one actually arrived. openPath toasts its own
  // refusal, so a failed reload needs nothing further said about it here.
  if (views.length <= before) return;
  // closeView asks about unsaved work too, and the question above already asked it — in
  // wording that names the reload rather than a close. Cleared HERE rather than before
  // the open, so a REFUSED reload leaves the document still marked unsaved and its next
  // close still warns.
  target.dirty = false;
  closeView(target);
};

// The file changes while Nib is in the background, so the answer is re-asked when the
// user comes back to it. See recheckDisk.
document.addEventListener('visibilitychange', () => { if (!document.hidden) recheckDisk(); });
window.addEventListener('focus', () => { recheckDisk(); });
els.saveBtn.onclick = save;

// Page navigation + zoom: the toolbar buttons and the keyboard shortcuts share
// these, so the bounds logic lives in one place.
function prevPage() { if (view.pdfDocument && view.viewer.currentPageNumber > 1) view.viewer.currentPageNumber--; }
function nextPage() { if (view.pdfDocument && view.viewer.currentPageNumber < view.pdfDocument.numPages) view.viewer.currentPageNumber++; }
function firstPage() { if (view.pdfDocument) view.viewer.currentPageNumber = 1; }
function lastPage() { if (view.pdfDocument) view.viewer.currentPageNumber = view.pdfDocument.numPages; }
// Every one of these records that the scale is now the user's — see `userScale`. Fit is
// the one that CLEARS it: the user asking for fit-width is the user handing the scale
// back, so a later refine to the widest page is welcome rather than an override.
// scaleFrom stamps WHICH code path last set a view's scale onto its container, where a
// test can read it.
//
// It exists because a one-in-ten flake has now outlived two explanations. The zoom test
// can see that a document came back at fit-width instead of the zoom it was left at; it
// cannot see WHICH of the four things that set a scale did it, and neither could I — the
// v1.109.12 fix was reasoned from the code and reported as an explanation rather than a
// sighting, and a later sighting contradicted it. Three characters of state turn the next
// occurrence from another round of theorising into a named door.
//
// A dataset write on an element that already exists, on paths that are already doing
// layout — not a hot path, and nothing reads it in production.
function scaleFrom(owner, src) { if (owner.container) owner.container.dataset.scaleSrc = src; }

// setUserScale is the ONE writer of `userScale`, and it keeps a short log of who wrote what
// on the view's own container. `scaleSrc` named the path that re-fitted a zoomed view
// (`pagesloaded`, which only runs when the flag is false); this answers the next question,
// which is what made it false. Bounded to the last eight writes so it cannot grow.
function setUserScale(owner, value, why) {
  owner.userScale = value;
  if (!owner.container) return;
  const log = (owner.container.dataset.userScaleLog || '').split('|').filter(Boolean);
  log.push(`${why}=${value}`);
  owner.container.dataset.userScaleLog = log.slice(-8).join('|');
}

function zoomIn() { setUserScale(view, true, 'zoomIn'); scaleFrom(view, 'zoomIn'); view.viewer.currentScale = view.viewer.currentScale * 1.15; }
function zoomOut() { setUserScale(view, true, 'zoomOut'); scaleFrom(view, 'zoomOut'); view.viewer.currentScale = view.viewer.currentScale / 1.15; }
function fitWidth() { setUserScale(view, false, 'fitWidthButton'); fitWidestWidth(view, 'fitWidthButton'); }
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
function fitWidestWidth(owner, fitReason = 'fit') {
  if (!owner.pdfDocument) return;
  let maxW = 0;
  for (let i = 0; i < owner.pdfDocument.numPages; i++) {
    const vp = owner.viewer.getPageView(i)?.viewport;
    if (vp) {
      const w = vp.width / vp.scale; // rendered display width in points (rotation applied)
      if (w > maxW) maxW = w;
    }
  }
  // The owning view's container, not the active one's — and a HIDDEN container reports
  // clientWidth 0, so `avail` goes negative and the guard below makes this a silent
  // no-op. That is ADR-002's stated consequence, and re-fitting on activation is
  // P05.S04's; this slice must not paper over it with a fallback width.
  const avail = owner.container.clientWidth - 40; // 40 = pdf.js SCROLLBAR_PADDING
  if (maxW > 0 && avail > 0) {
    owner.viewer.currentScale = avail / maxW / pdfjsLib.PixelsPerInch.PDF_TO_CSS_UNITS;
    scaleFrom(owner, fitReason);
    owner.hasScale = true; // it applied; activateView need not rescue this view
  }
}
els.prevBtn.onclick = prevPage;
els.nextBtn.onclick = nextPage;
all('.pageNum').forEach((input) => input.addEventListener('change', () => {
  const n = Number(input.value);
  if (view.pdfDocument && n >= 1 && n <= view.pdfDocument.numPages) view.viewer.currentPageNumber = n;
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
  if (!view.pdfDocument || !e.deltaY) return;
  const origin = [e.clientX, e.clientY];
  const pixelMode = e.deltaMode === WheelEvent.DOM_DELTA_PIXEL;
  if (pixelMode && !ctrlHeld) {
    // Trackpad pinch: continuous factor per event.
    setUserScale(view, true, 'pinch'); // the user choosing a scale, same as the buttons
    scaleFrom(view, 'pinch');
    view.viewer.updateScale({ scaleFactor: 2 ** (-e.deltaY / 100), origin, drawingDelay: 400 });
  } else {
    // Notched wheel: one zoom step per notch, same 1.15 factor as the buttons.
    wheelTicks += -e.deltaY / (pixelMode ? 100 : e.deltaMode === WheelEvent.DOM_DELTA_LINE ? 3 : 1);
    const steps = Math.trunc(wheelTicks);
    if (steps) {
      wheelTicks -= steps;
      // Inside `if (steps)`, not above it: a fraction of a notch moves nothing, and
      // marking the view user-scaled for it would suppress the widest-page refine
      // over a gesture that changed no scale at all.
      setUserScale(view, true, 'ctrlWheel');
      scaleFrom(view, 'ctrlWheel');
      view.viewer.updateScale({ scaleFactor: 1.15 ** steps, origin, drawingDelay: 400 });
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
  // Recording what was refreshed is not bookkeeping — activateView skips its refresh when
  // renderedDpr already matches, so without this line a view refreshed here keeps a stale
  // renderedDpr, and a later activation at that same dpr sees a match and skips. Its
  // canvases stay at the resolution they were rasterised at, CSS-stretched, permanently.
  if (view.pdfDocument) { view.renderedDpr = devicePixelRatio; view.viewer.refresh(); }
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
  view.eventBus.dispatch('find', {
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
// Bound per view in newView(). The find counter is SHARED chrome, so those two
// registrations bail unless the firing view is the active one.

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
  // The ceremony panel joins Collaborate (P06.S02). The mode's goal is "convene, invite, connect,
  // review, sign, deliver as a sidebar panel rather than a tab of modals", and this is the panel.
  //
  // **SECOND, and the order in this map is load-bearing in a way nothing here said.**
  // `syncSidebarForMode` activates `panels[0]` whenever the showing panel is not valid for the
  // mode, so the first entry IS the mode's default surface. Listing the ceremony panel first
  // silently made it the Collaborate landing screen and took Flags off it — eight tier-3 tests
  // went red waiting for `[data-marker="date"]`, a Flags control that was no longer displayed.
  // Making this panel the default is a real product decision about where Collaborate lands, and it
  // is not this slice's: S02 adds a read-only panel and changes no flow. It becomes the sensible
  // default when it has actions to offer, which is S04's and S05's.
  collaborate: ['flags', 'ceremony'],
};
function syncSidebarForMode(tab) {
  const panels = SIDEBAR_FOR[tab] || [];
  // Loaded when the panel becomes reachable rather than on a timer or at boot. It reads the local
  // mirror, so it is cheap and needs no network — but it is also not free (the server opens each
  // record), and a user who never goes near Collaborate should not pay for it.
  if (panels.includes('ceremony')) loadCeremonyPanel();
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
      if (isTypingTarget(e.target) || view.activeTool ||
          document.querySelector('div[id$="Modal"]:not([hidden])')) return;
      e.preventDefault();
      if (e.key === 'y' || e.shiftKey) redoAny(); else undoAny();
    }
    return;
  }
  if (!view.pdfDocument || isTypingTarget(e.target)) return;
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
    // The result of essentially every command in the app arrives here — 210 call sites —
    // and until /pending 328 none of it was announced: the node carried no role and no
    // live region, so a screen-reader user got silence for "Saved", "Key removed" and
    // every failure. WCAG SC 4.1.3.
    //
    // POLITE, not assertive. #sessionNotice is the app's one role="alert" and it is spent
    // on "you are about to lose a signature"; making 210 ordinary command results
    // assertive would interrupt the user constantly and devalue the one notice that
    // should. Set once, at creation, because the element is created once and reused.
    toastEl.setAttribute('role', 'status');
    toastEl.setAttribute('aria-live', 'polite');
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

// --- The Signing Ceremony panel (P06.S02) -----------------------------------
//
// **Read-only, and that is a decision rather than a stage.** The panel renders the roster and this
// machine's position, and offers no action. The next action is P06.S03's, computed by the same
// function the server's L3 check uses — so building a rule for it here would be a second
// implementation with a one-slice lifetime, which is the duplicate-derivation defect ADR-009
// exists to refuse.
//
// **It renders while the vault is LOCKED**, which is why P06.S01 moved `GET /api/ceremonies` off
// `requireUnlocked` onto `requirePublicLoopback`. Nothing here is sealed: every field comes from
// `record.json`, which D29 leaves unsealed by design. The one thing a locked reader cannot work
// out for itself is which roster entry it is, and the server records that at convene and accept
// time, when the vault IS open, into the ceremony's own folder.
//
// **And it renders with no network**, because the route reads the local mirror and nothing else.
// That is D24's resumption criterion: a screen that silently needed the DHT would be exactly the
// failure it was written to catch.

// CEREMONY_STATE_WORDS maps a load class to the sentence a person reads.
//
// The server already sends a `reason` for every degraded class, written for a human, and this map
// is the HEADING above it rather than a second copy of it — a client-side rewrite of those four
// sentences would be the two-implementations shape one language over.
const CEREMONY_STATE_WORDS = {
  ok: '',
  absent: 'Nothing on disk',
  unparseable: 'Damaged',
  'version-skew': 'Newer version of Nib',
  unverifiable: 'Does not verify',
};

// renderCeremonyPanel draws the ceremonies this machine knows about.
//
// **Every party is named and no hex fingerprint is shown**, which is one of this phase's exit
// criteria. A roster entry with no label falls back to its six-word pairing name from the server,
// never to the hex — the hex is the thing users are asked to compare aloud once, out of band, and
// putting it in a list is what makes people stop reading it.
function renderCeremonyPanel(data) {
  const host = document.getElementById('ceremonyList');
  if (!host) return;
  host.textContent = '';
  const list = (data && data.ceremonies) || [];
  if (!list.length) {
    const p = document.createElement('p');
    p.className = 'libhint';
    p.textContent = 'No signing ceremonies on this machine yet.';
    host.appendChild(p);
    return;
  }
  // **`primary` is the field and `note` is its sentence.** Read the flag rather than only the
  // prose: a second Nib on this machine must not continue or remove these ceremonies, and a
  // surface that inferred that from whether a string happened to be present would be reading the
  // server's wording instead of the server's answer.
  if (data.primary === false) {
    const n = document.createElement('p');
    n.className = 'libhint cernote';
    n.textContent = data.note || 'Another copy of Nib is already running on this machine.';
    host.appendChild(n);
  }
  for (const c of list) {
    host.appendChild(ceremonyCard(c));
  }
  renderEndedCeremonies(host, (data && data.ended) || []);
}

// renderEndedCeremonies lists what this machine has closed out.
//
// **This is why the close-out prune is a move rather than a delete, made visible** (ADR-012). A
// ceremony that ends leaves the live list by construction, and without this a user would watch it
// vanish with no trace and no way to find the signed contribution the move deliberately preserved.
// The receipt carries the state and the date THIS machine observed it — `Termination` has no
// `When` on purpose, so a convener cannot drive what other machines believe about timing.
function renderEndedCeremonies(host, ended) {
  if (!ended.length) return;
  const h = document.createElement('p');
  h.className = 'libhint cerendedhead';
  h.textContent = 'Finished — your copies are kept in the "ended" folder beside your ceremonies.';
  host.appendChild(h);
  for (const r of ended) {
    const row = document.createElement('div');
    row.className = 'cerended-row';
    const what = document.createElement('span');
    what.textContent = r.state === 'declined' ? 'Declined'
      : r.state === 'completed' ? 'Completed'
      : r.state === 'expired' ? 'Ran out of time'
      // Deliberately NOT the word 'heard':  matches published field names as
      // substrings of this file, so that word alone would satisfy the LAN-browse response's list
      // field — a
      // reader claimed by a coincidence in prose, which is the blind spot /pending 252 nearly
      // died on. Caught by that guard on this slice's own commit.
      : 'No further word';
    row.appendChild(what);
    const when = new Date(r.observed_at);
    if (!Number.isNaN(when.getTime())) {
      const w = document.createElement('span');
      w.className = 'cerwhen';
      w.textContent = when.toLocaleDateString();
      row.appendChild(w);
    }
    host.appendChild(row);
  }
}

// ceremonyCard builds one ceremony's entry.
//
// **A degraded ceremony still gets a card**, with its class and the server's sentence. It must not
// vanish: a ceremony Nib will not admit exists is one whose only remedy is finding and deleting
// the folder by hand, which is where the user already is. That is C12's client half, and the same
// rule `ListStored` holds on the server.
function ceremonyCard(c) {
  const card = document.createElement('div');
  card.className = 'cercard';
  card.dataset.ceremony = c.id;
  card.dataset.state = c.state || '';

  const head = document.createElement('div');
  head.className = 'cerhead';
  const title = document.createElement('span');
  title.className = 'cerintent';
  // The recital is what the parties agreed to, and it is the only thing here worth reading first.
  // A degraded ceremony has none, so it is named by what it is instead — never by its hex id.
  title.textContent = c.intent || 'A ceremony on this machine';
  head.appendChild(title);
  if (c.state && c.state !== 'ok') {
    const badge = document.createElement('span');
    badge.className = 'cerbadge';
    badge.textContent = CEREMONY_STATE_WORDS[c.state] || c.state;
    head.appendChild(badge);
  }
  if (c.ended) {
    const e = document.createElement('span');
    e.className = 'cerbadge cerended';
    e.textContent = c.ended === 'declined' ? 'Declined' : 'Completed';
    head.appendChild(e);
  }
  card.appendChild(head);

  if (c.reason) {
    const r = document.createElement('p');
    r.className = 'cerreason';
    r.textContent = c.reason;
    card.appendChild(r);
  }

  // **The deadline, in human units — and it is the only one this panel ever shows.** The phase's
  // criterion is that the ceremony deadline appears in human units and neither the connect
  // deadline nor the exchange deadline appears as a countdown at all. This is that one, and it is
  // a date rather than a ticking figure: a countdown invites a user to watch it, and the thing
  // they can act on is the date.
  if (c.expires && !String(c.expires).startsWith('0001-')) {
    const d = document.createElement('p');
    d.className = 'cerdeadline';
    const when = new Date(c.expires);
    d.textContent = Number.isNaN(when.getTime())
      ? ''
      : `Open until ${when.toLocaleDateString()} at ${when.toLocaleTimeString()}`;
    if (d.textContent) card.appendChild(d);
  }

  const roster = c.roster || [];
  if (roster.length) {
    card.appendChild(ceremonyRoster(roster, c.me));
  }

  // **Whose turn it is, fetched when the user opens this card and not before (P06.S03).**
  // `/api/ceremony/next` opens the DOCUMENT to answer, which is the cost `ListStored` was designed
  // around never paying — measured at P08.S03 as 10/69/195 ms for 100/500/1000 pages. Per card on
  // demand puts that cost where somebody asked a question, and nowhere else.
  //
  // **The answer is the server's and the rule is never rewritten here.** It comes from
  // `p2p.NextContributor`, the same function `AdmitContribution` refuses with, which P07.S03a
  // wrote in its question form for exactly this. A JS predicate over the roster would be a second
  // derivation that agrees on the day it is written — the shape ADR-009 refuses.
  if (c.state === 'ok') {
    const next = document.createElement('div');
    next.className = 'cernext';
    const btn = document.createElement('button');
    btn.className = 'cernextbtn';
    btn.type = 'button';
    btn.textContent = 'What happens next?';
    btn.addEventListener('click', async () => {
      btn.disabled = true;
      next.textContent = '';
      next.appendChild(await ceremonyNextLine(c.id));
    });
    next.appendChild(btn);
    card.appendChild(next);
  }
  return card;
}

// ceremonyRoster renders the parties, marking which one is this machine.
//
// **An absent `me` is UNKNOWN and never "you are not a party."** A ceremony mirrored before the
// marker shipped has none, and its user is still very much a party; reading the empty string as
// absence would tell every one of them they are looking at somebody else's proceeding. So with no
// marker the list is drawn with nobody marked and a line saying so, rather than with everybody
// implicitly marked as somebody else.
function ceremonyRoster(roster, me) {
  const wrap = document.createElement('div');
  wrap.className = 'cerroster';
  const known = typeof me === 'string' && me !== '';
  let mine = -1;
  roster.forEach((p, i) => {
    const row = document.createElement('div');
    row.className = 'cerparty';
    const isMe = known && typeof p.fingerprint === 'string'
      && p.fingerprint.toLowerCase() === me.toLowerCase();
    if (isMe) { row.classList.add('cerme'); mine = i; }
    const who = document.createElement('span');
    who.className = 'cerwho';
    // Label, then the six-word name, then "Party k" — and never the hex. Position is a fact about
    // the roster and reads as one; a fingerprint in a list is a string nobody checks.
    who.textContent = p.label || p.name || `Party ${i + 1}`;
    row.appendChild(who);
    const role = document.createElement('span');
    role.className = 'cerrole';
    // **Capacity, where the party declared one.** It is the difference between "Alice Tenant" and
    // "Alice Tenant, as attorney-in-fact for X", which is the whole reason D20 put it on the block
    // — a roster that renders the name and drops the capacity shows a different agreement from the
    // one the signature covers.
    role.textContent = p.capacity
      ? `${p.capacity} · ${p.signs === false ? 'does not sign' : 'signs'}`
      : (p.signs === false ? 'does not sign' : 'signs');
    row.appendChild(role);
    if (isMe) {
      const tag = document.createElement('span');
      tag.className = 'certag';
      tag.textContent = 'you';
      row.appendChild(tag);
    }
    wrap.appendChild(row);
  });
  const pos = document.createElement('p');
  pos.className = 'cerpos';
  pos.textContent = known && mine >= 0
    ? `You are party ${mine + 1} of ${roster.length}.`
    : 'This copy of Nib cannot tell which of these parties you are.';
  wrap.appendChild(pos);
  return wrap;
}

// loadCeremonyPanel fetches the listing and renders it.
//
// **Unpinned**, because the question is about this machine and not about a document: `apiFetch`
// attaches `X-Nib-Doc` to every call by design, and pinning this one would ask after a document
// that has nothing to do with which ceremonies are on disk.
//
// A failure renders a sentence rather than an empty panel, for the reason a degraded ceremony
// still gets a card: an empty shelf and a broken read look identical, and only one of them is the
// user's problem to act on.
async function loadCeremonyPanel() {
  try {
    const res = await apiFetch('/api/ceremonies', { unpinned: true });
    if (!res.ok) throw new Error(String(res.status));
    renderCeremonyPanel(await res.json());
  } catch (e) {
    const host = document.getElementById('ceremonyList');
    if (!host) return;
    host.textContent = '';
    const p = document.createElement('p');
    p.className = 'libhint';
    p.textContent = 'Nib could not read the ceremonies on this machine.';
    host.appendChild(p);
  }
}

// ceremonyNextLine asks the server whose turn it is and renders the sentence.
//
// **Every branch of the server's three states gets its own sentence**, because they want different
// things from the user: `waiting` is somebody's turn, `complete` is finished, and `unavailable`
// means Nib could not read enough to say — and folding the last two together would tell a user
// whose ceremony finished that their document is damaged.
async function ceremonyNextLine(id) {
  const p = document.createElement('p');
  p.className = 'cernextline';
  let d;
  try {
    const res = await apiFetch(`/api/ceremony/next?ceremony=${encodeURIComponent(id)}`,
      { unpinned: true });
    if (!res.ok) throw new Error(String(res.status));
    d = await res.json();
  } catch (e) {
    p.textContent = 'Nib could not work out what happens next for this ceremony.';
    return p;
  }
  // **The echoed id is checked.** The panel can have several cards and each has its own button; a
  // slow answer for one must never be rendered under another. The server echoes what it was asked
  // about precisely so this comparison is possible.
  if (d.ceremony && d.ceremony !== id) {
    p.textContent = 'Nib could not work out what happens next for this ceremony.';
    return p;
  }
  if (d.state === 'complete') {
    p.textContent = 'Everyone has signed. This ceremony is finished.';
    return p;
  }
  if (d.state !== 'waiting') {
    p.textContent = d.reason || 'Nib cannot tell what happens next for this ceremony.';
    return p;
  }
  const who = d.label || `party ${d.position}`;
  const where = d.position && d.of ? ` (${d.position} of ${d.of} signing)` : '';
  const cap = d.capacity ? `, ${d.capacity}` : '';
  // **`meKnown` and `isMe` are different facts.** A machine that never recorded which party it is
  // must not be told "it is not your turn" — that is an answer, and the honest one is that Nib
  // does not know which of these parties the user is.
  if (!d.meKnown) {
    p.textContent = `Waiting for ${who}${cap}${where}. This copy of Nib cannot tell whether that is you.`;
  } else if (d.isMe) {
    p.textContent = `It is your turn to sign${cap}${where}.`;
    p.classList.add('certurn');
  } else {
    p.textContent = `Waiting for ${who}${cap}${where}.`;
  }
  return p;
}

// --- Convene and accept (P06.S04) -------------------------------------------
//
// **The roster is picked, never typed, and that is the criterion rather than a preference.** P06
// says the primary flow contains no hex fingerprint, and a fingerprint is hex — so a convener
// chooses from the peers this machine has already pinned, shown by the six-word pairing name every
// other peer control in the product uses. A party who is not pinned yet is pinned first, through
// Identity & peers, which is where D21's read-it-aloud comparison already happens.
//
// **Nothing here is a modal.** The phase's goal is "convene, invite, connect, review, sign, deliver
// as a sidebar panel rather than a tab of modals", and these are two forms inside that panel.

// cerEls resolves the panel's controls once per call rather than caching them, because the panel's
// markup is static and a cached reference would survive a document swap that replaced it.
function cerEls() {
  return {
    convene: document.getElementById('ceremonyConveneForm'),
    accept: document.getElementById('ceremonyAcceptForm'),
    result: document.getElementById('ceremonyResult'),
    pick: document.getElementById('cerPeerPick'),
  };
}

// showCeremonyForm reveals one of the two forms and hides the other.
//
// Exclusive because they are two answers to one question — "am I starting this or joining it" —
// and a screen showing both invites a user to fill in the wrong one.
function showCeremonyForm(which) {
  const e = cerEls();
  if (!e.convene || !e.accept) return;
  e.convene.hidden = which !== 'convene';
  e.accept.hidden = which !== 'accept';
  // **Cleared when a form OPENS, never when one closes**, and the difference is a defect a test
  // caught. Both submit paths render their result and then call this with `null` to put the form
  // away — so clearing on close wiped the invitations in the same tick they were drawn, and the
  // screen went blank on success. Clearing on open is what the wipe was actually for: starting a
  // second ceremony should not leave the first one's invitations above the form.
  if (which && e.result) e.result.textContent = '';
}

// loadPeerPicker fills the roster chooser from the peers this machine has pinned.
//
// **The six-word name is the label and the hex is the value**, carried in a data attribute the user
// never sees. That is the whole no-hex criterion in one line: the fingerprint has to reach the
// server, and it does not have to reach the screen.
async function loadPeerPicker() {
  const e = cerEls();
  if (!e.pick) return;
  e.pick.textContent = '';
  let peers = [];
  try {
    const res = await apiFetch('/api/peers', { unpinned: true });
    if (res.ok) peers = (await res.json()).peers || [];
  } catch (err) { /* rendered as the empty case below */ }
  if (!peers.length) {
    const p = document.createElement('p');
    p.className = 'libhint';
    p.textContent = 'You have not paired with anyone yet.';
    e.pick.appendChild(p);
    return;
  }
  for (const peer of peers) {
    const row = document.createElement('div');
    row.className = 'cerpeerrow';
    const box = document.createElement('input');
    box.type = 'checkbox';
    box.className = 'cerpeerbox';
    // The fingerprint travels here and is never rendered as text.
    box.dataset.fingerprint = peer.fingerprint || '';
    row.appendChild(box);
    const who = document.createElement('span');
    who.className = 'cerpeername';
    who.textContent = peer.label || peer.name || 'a paired peer';
    row.appendChild(who);
    const cap = document.createElement('input');
    cap.type = 'text';
    cap.className = 'cerpeercap';
    cap.placeholder = 'capacity (optional)';
    cap.maxLength = 120;
    row.appendChild(cap);
    e.pick.appendChild(row);
  }
}

// conveneFromPanel posts the roster the user picked.
async function conveneFromPanel() {
  const err = document.getElementById('cerConveneError');
  const say = (m) => { if (err) { err.textContent = m; err.hidden = !m; } };
  say('');
  const roster = [];
  for (const row of document.querySelectorAll('#cerPeerPick .cerpeerrow')) {
    const box = row.querySelector('.cerpeerbox');
    if (!box || !box.checked) continue;
    roster.push({
      fingerprint: box.dataset.fingerprint || '',
      label: row.querySelector('.cerpeername')?.textContent || '',
      capacity: row.querySelector('.cerpeercap')?.value || '',
      signs: true,
    });
  }
  if (!roster.length) { say('Choose at least one other person to sign.'); return; }
  const expires = document.getElementById('cerExpires')?.value || '';
  if (!expires) { say('Set the date this ceremony stays open until.'); return; }
  const body = {
    roster,
    intent: document.getElementById('cerIntent')?.value || '',
    // **An absolute instant, because the API takes one.** `conveneRequest.Expires`'s own doc says
    // why it is not a count of days: a real transaction has a date, and a convener asked "how many
    // days?" guesses. `datetime-local` yields local wall time with no zone, so it is converted here
    // rather than sent as typed — sending it raw would be off by the user's offset.
    expires: new Date(expires).toISOString(),
    convenerSigns: !!document.getElementById('cerISign')?.checked,
  };
  try {
    const res = await apiFetch('/api/ceremony/convene', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) { say(await errText(res, 'this ceremony could not be convened')); return; }
    renderInvitations(await res.json());
    showCeremonyForm(null);
    loadCeremonyPanel();
  } catch (e) {
    say('this ceremony could not be convened');
  }
}

// renderInvitations shows one invitation per party, with what an invitation IS.
//
// **D21's sentence, in D21's terms, and the criterion asks for those terms.** Its own words: *"What
// an intercepted invitation gets its holder: the rendezvous, and nothing beyond it. The pin is the
// fingerprint in the roster and they do not hold the private key, so they are refused at the
// handshake. The invitation is a channel secret, never a signing credential."* A user who forwards
// one should know what they did and did not give away, so the screen says it beside the thing
// itself rather than in help nobody opens.
function renderInvitations(d) {
  const e = cerEls();
  if (!e.result) return;
  e.result.textContent = '';
  const head = document.createElement('p');
  head.className = 'cerok';
  head.textContent = `Convened. Send each person their own invitation — they are not interchangeable.`;
  e.result.appendChild(head);

  const warn = document.createElement('p');
  warn.className = 'libhint cersecret';
  warn.textContent = 'An invitation is a channel secret, not a signing credential. It lets its '
    + 'holder find this ceremony and nothing more: signing needs the private key of a party the '
    + 'roster names, so anyone else is refused at the handshake.';
  e.result.appendChild(warn);

  for (const inv of (d.invites || [])) {
    const row = document.createElement('div');
    row.className = 'cerinvite';
    const who = document.createElement('span');
    who.className = 'cerwho';
    // The name, never the fingerprint — the payload carries both and only one belongs on screen.
    who.textContent = inv.label || inv.name || 'a party';
    row.appendChild(who);
    const text = document.createElement('textarea');
    text.className = 'cerinvitetext';
    text.rows = 2;
    text.readOnly = true;
    text.value = inv.invitation || '';
    row.appendChild(text);
    const copy = document.createElement('button');
    copy.type = 'button';
    copy.className = 'cercopy';
    copy.textContent = 'Copy';
    copy.addEventListener('click', () => {
      text.select();
      try { document.execCommand('copy'); } catch (err) { /* selection is the fallback */ }
    });
    row.appendChild(copy);
    if (inv.signs === false) {
      const n = document.createElement('span');
      n.className = 'cerrole';
      n.textContent = 'does not sign';
      row.appendChild(n);
    }
    e.result.appendChild(row);
  }
  // **A warning is BOUND to the control that caused it, which is what its `code` is for.**
  // `conveneResponse.Warnings`' own doc says they are *"machine-tagged so a panel can bind one to
  // the control that caused it rather than re-parsing English"* — so the code selects the control
  // and the text is what the user reads. There is one code today (`sitting-ceiling`, D22's ~8) and
  // the map is what makes the second one a one-line addition rather than a rewrite; an unknown
  // code still shows its text, because a warning nobody renders is a soft refusal nobody hears.
  const WARN_CONTROL = { 'sitting-ceiling': 'cerPeerPick' };
  for (const wmsg of (d.warnings || [])) {
    const p = document.createElement('p');
    p.className = 'cerwarn';
    p.textContent = wmsg.text || '';
    if (!p.textContent) continue;
    const target = document.getElementById(WARN_CONTROL[wmsg.code] || '');
    if (target && target.parentNode) {
      p.dataset.warn = wmsg.code;
      target.parentNode.insertBefore(p, target.nextSibling);
    } else {
      e.result.appendChild(p);
    }
  }
}

// acceptFromPanel pastes an invitation in and shows what the user has joined.
async function acceptFromPanel() {
  const err = document.getElementById('cerAcceptError');
  const say = (m) => { if (err) { err.textContent = m; err.hidden = !m; } };
  say('');
  const text = document.getElementById('cerInviteText')?.value?.trim() || '';
  if (!text) { say('Paste the invitation you were sent.'); return; }
  try {
    const res = await apiFetch('/api/ceremony/accept', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ invitation: text }),
    });
    if (!res.ok) { say(await errText(res, 'this invitation could not be accepted')); return; }
    renderAccepted(await res.json());
    showCeremonyForm(null);
    loadCeremonyPanel();
  } catch (e) {
    say('this invitation could not be accepted');
  }
}

// renderAccepted shows the roster the invitee has just joined.
//
// **`self` and `convener` are read rather than re-derived.** The server marks both — its own doc
// says they are "the two entries a reader needs to find without re-deriving them" — and a client
// that worked them out from fingerprints would be a second derivation of a fact already sent, in
// the one place the criterion forbids fingerprints from appearing at all.
function renderAccepted(d) {
  const e = cerEls();
  if (!e.result) return;
  e.result.textContent = '';
  const head = document.createElement('p');
  head.className = 'cerok';
  head.textContent = `Joined. ${d.signing || 0} of ${(d.roster || []).length} parties sign this document.`;
  e.result.appendChild(head);
  for (const p of (d.roster || [])) {
    const row = document.createElement('div');
    row.className = 'cerparty';
    if (p.self) row.classList.add('cerme');
    const who = document.createElement('span');
    who.className = 'cerwho';
    who.textContent = p.label || p.name || 'a party';
    row.appendChild(who);
    const role = document.createElement('span');
    role.className = 'cerrole';
    const bits = [];
    if (p.capacity) bits.push(p.capacity);
    bits.push(p.signs === false ? 'does not sign' : 'signs');
    if (p.convener) bits.push('convened this');
    role.textContent = bits.join(' · ');
    row.appendChild(role);
    if (p.self) {
      const tag = document.createElement('span');
      tag.className = 'certag';
      tag.textContent = 'you';
      row.appendChild(tag);
    }
    e.result.appendChild(row);
  }
  if (d.pinned) {
    const p = document.createElement('p');
    p.className = 'libhint';
    p.textContent = d.pinned === 1
      ? 'The convener is now a pinned peer, so no fingerprint has to be typed again.'
      : `${d.pinned} parties are now pinned peers.`;
    e.result.appendChild(p);
  }
}

document.getElementById('ceremonyConveneBtn')?.addEventListener('click', () => {
  showCeremonyForm('convene');
  loadPeerPicker();
});
document.getElementById('ceremonyAcceptBtn')?.addEventListener('click', () => showCeremonyForm('accept'));
document.getElementById('cerConveneCancel')?.addEventListener('click', () => showCeremonyForm(null));
document.getElementById('cerAcceptCancel')?.addEventListener('click', () => showCeremonyForm(null));
document.getElementById('ceremonyConveneForm')?.addEventListener('submit', (ev) => {
  ev.preventDefault();
  conveneFromPanel();
});
document.getElementById('ceremonyAcceptForm')?.addEventListener('submit', (ev) => {
  ev.preventDefault();
  acceptFromPanel();
});

// reflectArmProgress is the READER for sessionStatus.progress (P06.S05, D16 amendment).
//
// **It is not the diagnosis and it renders above one.** `reflectDiagnosis` answers *why nothing has
// connected*; this answers *what is happening*. They are published under different conditions on
// purpose: the diagnosis waits for `bootstrapDone` so a cause cannot accuse a tier that has not had
// its chance, and under ADR-011 the bootstrap itself waits for the local link — on a LAN, for
// `lanFirstBudget`, thirty seconds. **That wait is exactly the window D16 says must never be a
// blank spinner**, and it is the window the diagnosis structurally cannot speak in.
//
// # Every line is plain language and none of them is a countdown
//
// D16's amendment says only the ceremony deadline appears in human units; neither the connect
// deadline nor the exchange deadline appears as a countdown. So the tiers report STATE, never
// remaining time. A countdown here would also invite a user to watch it, and what they can act on
// is the router line.
const TIER_WORDS = {
  'link:watching': 'Listening for the other party on your local network.',
  // **Deliberately not the word 'heard'**, for the second time in this phase.
  // `observables_test.go` matches published field names as substrings of this file, so that word
  // alone satisfies the LAN-browse response's list field — which is genuinely unread and parked
  // under
  // `/pending 23`. Rewording is the honest move both times: a reader claimed by a coincidence in
  // prose would silently un-park a field nothing shows.
  'link:found': 'Found the other party on your local network.',
  'dht:holding': 'Not using the internet yet \u2014 giving your local network its chance first.',
  'dht:reaching': 'Looking for the other party through the public rendezvous.',
  // The router tier's states, and they are separate because the next action differs. Silence may
  // mean there is no gateway to ask; a refusal means the router is reachable and said no; an
  // unroutable answer means a second layer of NAT and points at a VPN rather than a port-forward.
  'router:silent': 'Asked your router for a temporary opening; it did not answer.',
  'router:refused': 'Asked your router for a temporary opening; it answered and declined.',
  'router:unroutable': 'Your router answered, but the address it gave cannot be reached from '
    + 'outside \u2014 usually a second router in front of it. A private network such as Tailscale '
    + 'or WireGuard is the way through.',
};
let tiersShownFor = '';
function reflectArmProgress(p) {
  const host = els.srvWaitTiers;
  if (!host) return;
  if (!p) {
    host.hidden = true;
    host.textContent = '';
    tiersShownFor = '';
    return;
  }
  // **The router line is built rather than looked up, because it names the PORT.** D15's criterion
  // is that the screen discloses that a temporary router opening was requested and names the port,
  // and says so when no mapping was obtained rather than staying silent \u2014 so the open case
  // carries a number and each other case carries its own sentence.
  const lines = [];
  if (p.link) lines.push(TIER_WORDS['link:' + p.link] || '');
  if (p.dht) lines.push(TIER_WORDS['dht:' + p.dht] || '');
  if (p.router === 'open') {
    lines.push('Your router opened a temporary opening on port ' + p.port
      + '. It closes when this ceremony ends.');
  } else if (p.router) {
    lines.push(TIER_WORDS['router:' + p.router] || '');
  }
  const shown = lines.filter(Boolean);
  // Keyed on the rendered text for `reflectDiagnosis`' reason: this polls every 1.5 s and an
  // aria-live region would otherwise re-announce an unchanged ladder to a screen reader forever.
  const key = shown.join(' ');
  if (key === tiersShownFor) return;
  tiersShownFor = key;
  host.textContent = '';
  for (const line of shown) {
    const li = document.createElement('li');
    li.className = 'waittier';
    li.textContent = line;
    host.appendChild(li);
  }
  host.hidden = shown.length === 0;
}
