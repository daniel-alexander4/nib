# Accessibility: Nib and Acrobat Pro, feature by feature

`PLAN-accessibility.md` P10.S04. This compares what Nib does for accessibility with what Adobe documents
for Acrobat Pro. **Every Acrobat claim is quoted from Adobe's own help pages** (listed under *Sources*),
and **every Nib claim names the test that shows it**. Where Nib has no test for something, the row says
*Unmeasured* rather than claiming it. Where Acrobat's cited pages say nothing, the row says so rather than
guessing.

`paritydoc_test.go` (repo root) holds this document to that. It fails when a cited test does not exist,
when an Acrobat cell cites no source, when a feature Adobe's pages name has no row, or when one of Nib's
accessibility routes or `nib tag` subcommands appears nowhere here.

**Status:** *Parity* — Nib does what the cited pages describe. *Partial* — Nib does part of it; the Nib
cell says which part is missing. *Gap* — Nib does not do it. *Nib only* — the cited pages describe no
counterpart. *Unmeasured* — Nib has the control, and no test shows what it does.

One number frames the checker rows. Nib checks **72 of the 106** rules veraPDF evaluates for PDF/UA-1, so
a document can pass every clause Nib checks and still fail one it does not.

## The ledger

| Feature | Acrobat Pro (Adobe's documentation) | Nib | Evidence | Status |
| --- | --- | --- | --- | --- |
| Accessibility Check | "verifies whether the document conforms to accessibility standards, such as PDF/UA and WCAG 2.0" [A1]; an item it cannot check is reported as needing a manual check [A1] | `nib ua IN` and **Check accessibility** (`/api/uacheck`) check 72 of the 106 rules veraPDF evaluates. A clause Nib could not check is never shown as a pass. There is no WCAG check | `TestEveryTreeRuleIsCannotCheckPastTheTreeBound`, `TestUAExitZeroSaysItIsNotACertificate`, `TestTheUIAndTheCLIReachTheSameConformanceDoor` | Partial |
| Fix from the check's results | "Fix accessibility issues (Acrobat Pro)" [A1] | The report offers no fix. Corrections are made in **Review Structure Tree**, one element at a time | — | Gap |
| Automatically tag PDF | "Select All tools > Prepare for accessibility > Automatically tag PDF. If there are any issues, the Add Tags Report appears in the navigation pane." [A1]; "Most tables are properly recognized using this command" [A2] | **Tag structure…** and `nib tag propose` propose headings, paragraphs and list items from how the pages look (`/api/tags/propose`). Nothing is written until a person reviews it and commits (`/api/tags/commit`, `nib tag commit`). No tables or figures are proposed | `TestHeadingLevelsFollowTheDistinctLargerSizes`, `TestAReviewIsAppliedInItsOwnOrderWithItsOwnRoles`, `TestTagCommitWritesTheProposalItReviewed` | Partial |
| Remove and replace existing tags | "Acrobat can retag an already tagged document after you first remove all existing tags from the tree." [A2] | Tagging refuses a document that is already tagged, and there is no way to remove a tree | — | Gap |
| Reading Order tool | "You can change the reading order of the highlighted regions by moving an item in the Order panel. Alternatively, you can drag it on the page in the document pane." [A2] | **Show reading order** numbers each element on its page. The order is changed by moving an element in **Review Structure Tree** from the keyboard, not by dragging on the page | `test/ui/tagedit.test.mjs: 'the reading order view numbers each element on its own page, over its text, and goes when switched off'`, `test/jsdom/tagtree.test.mjs: 'the reading order view is a toggle that says whether it is on'`, `TestAMoveHandlesEveryFormOfK` | Partial |
| Tag a selected region | "Tags the selection as a table or header cell." [A2] | Edits act on elements already in the tree; a region selected on the page cannot be tagged | — | Gap |
| Tags panel | "In the Tags panel, tags appear in a hierarchical order that indicates the reading sequence of the document." [A3]; "You can edit a tag title, change a tag location" [A3] | **Review Structure Tree** and `nib tag tree` show the tree in reading order (`/api/tags/tree`). Retype, move, alt text, scope and mark-as-decoration go through `/api/tags/edit` and `nib tag edit`. There is no tag title edit, no new tag, and no deleting a tag, and an element written inline (id 0) cannot be edited | `TestTagTreePrintsTheTreeTheDocumentHas`, `TestTheEditRouteCorrectsTheTreeAndOneUndoTakesTheBatchBack`, `TestTagEditAppliesTheBatch` | Partial |
| Alternate text | "You can add alternate text and multiple languages to a tag from the Tags panel." [A3]; "Once you define it as a figure, you can add alternate text to describe the figure." [A2] | Alt text is set or removed on an element, and a figure without it is flagged in the tree and in the report. This is measured on figures; alt text on links and abbreviations, which [A3] also documents, is not measured | `TestAltTextKeepsEveryCharacter`, `TestAFigureNeedsAltTextOrReplacementText`, `TestTagTreeShowsNestingAndWhatIsMissing` | Partial |
| Artifact (background) | "Tags the selection as a background element, or artifact, removing the item from the tag tree." [A2]; "Page numbers, headers, and footers are often best tagged as artifacts." [A3] | **Mark as decoration** takes a whole element out of the tree and leaves its content marked as an artifact. It refuses what it cannot rewrite honestly, and it cannot artifact an arbitrary selection | `TestArtifactingAnElementTakesItOutOfTheTreeAndLeavesItsContent`, `TestArtifactingRefusesWhatItCannotRewriteHonestly` | Partial |
| Table header cells and spans | "If the table contains rows that span two or more columns, set ColSpan and RowSpan attributes for these rows in the tag structure." [A3] | A header cell's scope can be set to Row, Column or Both, and the checker lays a table out the way veraPDF does — row and column spans, overlapping cells, rows of the wrong width, and data cells no header reaches. The editor writes no ColSpan or RowSpan attributes and no Headers/IDs, and there is no table recognition. The cited pages describe no scope control | `TestAScopeEditNeverWritesThroughASharedAttributeObject`, `TestATableAndAFigureReadBackTheirScopeAndAlt`, `TestATableIsCheckedTheWayVeraPDFLaysItOut` | Partial |
| Role map | "view and edit the role map of a PDF by choosing Options > Edit Role Map in the Tags panel" [A3] | The tree shows each element's type as written and as the role map resolves it. The role map cannot be edited | `TestTagTreePrintsTheTreeTheDocumentHas` | Partial |
| Document language | "This setting applies the primary language for the entire PDF." [A1]; a language can also be set "for a block of text by selecting the text element or container element" [A1] | Documents Nib writes declare a language, and OCR sets the scan's. No control sets an existing document's language, and there is no per-element language | `TestEveryAuthoringDoorSaysWhereItsLanguageComesFrom`, `TestSetLang` | Partial |
| Document title | The check "Reports whether there is a title in the Acrobat application title bar." [A1] | Documents Nib authors and exports get a title. No control sets an existing document's title | `TestEveryAuthoredDocumentGetsATitle` | Partial |
| Form field descriptions | "For accessibility, all form fields need a text description (tool tip)." [A1] | A field Nib authors announces the name the person typed (`/TU`) and is nested in a Form element. No control names the fields of a form Nib did not author | `TestEveryNamedFieldAnnouncesTheNameTheUserGave`, `TestEveryWidgetIsNestedInAFormElementThatPointsBack` | Partial |
| Recognize text (OCR) | "A document that consists of scanned images of text is inherently inaccessible because the content of the document is images, not searchable text." [A5]; Acrobat recognizes text in scanned images [A1] | On-device OCR writes a tagged text layer in the app (`/api/ocr`). Recognition runs in the browser, so the command line has no recogniser and cannot tag a scan | `TestTaggingAnOCRdScanCostsItNoUA1Clause` | Partial |
| Make Accessible action, and batch | "A predefined action automates many tasks, checks accessibility, and provides instructions for items that require manual fixes." [A1]; "You can run the action on the currently opened file or add more files, folders, or email attachments." [A4] | `nib watch DIR --do ua` writes each new PDF's report to `FILE.ua.txt`. Tagging a folder is refused by name (`--do tag`), because a proposal is reviewed before it is written | `TestWatchUAWritesTheReportNibUAPrints`, `TestWatchRefusesTagNamingLaw3` | Partial |
| Read Out Loud | "Read Out Loud for text-to-speech conversion." [A5] | **Read aloud** reads the current page's text layer | `test/jsdom/readaloud.test.mjs: 'a page with text is read, and only that page'` | Partial |
| Reflow | "Reflow capability, displaying PDF text in large type and converting multicolumn layouts to a single, readable column." [A5] | None: a search of `web/` for a reflow view found none | — | Gap |
| Save as accessible text | "read the saved text file in a word-processing application and emulate the end-user experience of readers who use a braille printer" [A1] | **Document text (.txt)** export exists. No test reads what it writes | — | Unmeasured |
| Tag annotations | "choose Tag Annotations from the Options menu. Comments or markups that you add to the PDF are tagged automatically." [A3] | Not measured for this ledger | — | Unmeasured |
| PDF/UA identification in the file | Not described on the cited pages | Written only on Nib's own Markdown conversion, when the user chose its language, where veraPDF measures every construct Nib renders conformant. Never on the checker's say-so (it covers 72 of the 106 rules), never on an office conversion, and removed by any later change | `TestNibsOwnMarkdownConversionConformsAcrossEveryConstruct`, `TestPassingEveryClauseNibChecksIsNotConformanceAndTheDocsSaySo` | Partial |
| Command line | Not described on the cited pages | `nib ua`, `nib tag tree`, `nib tag propose`, `nib tag commit` and `nib tag edit` reach the doors the app's routes reach, and a signed document is refused through the same door | `TestTheTagCommandsReachTheRoutesDoors`, `TestEveryStructureWriteReachesTheSignedDocumentDoor` | Nib only |
| Structure kept through edits | Not described on the cited pages | Edits that keep tags carry them. Edits that destroy tags remove the claim and say so | `TestCarryingStructureChangesNoDRAWNByte`, `TestDroppingATaggingClaimIsRecordedOnTheDocument` | Nib only |

## Gaps, named

Every *Gap* and *Unmeasured* row above, plus what the *Partial* rows say is missing:

- a fix from the check's results;
- a WCAG check, and the 34 PDF/UA-1 rules Nib does not check;
- proposing tables and figures;
- removing a tree to retag;
- tagging a region selected on the page, and reordering by dragging on the page;
- tag titles, new tags and deleting tags;
- editing inline (id 0) elements;
- alt text measured on links and abbreviations;
- artifacting an arbitrary selection;
- ColSpan, RowSpan and Headers/IDs, and table recognition;
- editing the role map;
- setting an existing document's language or title, and per-element language;
- naming the fields of a form Nib did not author;
- tagging a scan from the command line;
- tagging a folder, which is refused by design;
- reflow;
- tests for text export and for annotation tagging;
- the PDF/UA identifier.

## Sources

Adobe's help pages answered automated fetches with HTTP 403 on 2026-09-14. Each quotation was taken from
the Internet Archive's 2026 snapshot of the page, named after it.

- [A1] Create and verify PDF accessibility (Acrobat Pro) — https://helpx.adobe.com/acrobat/using/create-verify-pdf-accessibility.html (snapshot 2026-09-14)
- [A2] Reading Order tool for PDFs (Acrobat Pro) — https://helpx.adobe.com/acrobat/using/touch-reading-order-tool-pdfs.html (snapshot 2026-04-22)
- [A3] Edit document structure with the Content and Tags panels (Acrobat Pro) — https://helpx.adobe.com/acrobat/using/editing-document-structure-content-tags.html (snapshot 2026-09-11)
- [A4] Adobe Acrobat Pro Action Wizard — https://helpx.adobe.com/acrobat/using/action-wizard-acrobat-pro.html (snapshot 2026-05-17)
- [A5] Accessibility features in PDFs — https://helpx.adobe.com/acrobat/using/accessibility-features-pdfs.html (snapshot 2026-08-21)
