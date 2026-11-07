# ADR-217 — Open…'s tooltip groups the formats by kind and says whether each program is installed

Status: accepted, 2026-11-07 (Dan: "group the doctypes alphabetically within type, Image, Office, etc. If a helper
program like libreoffice is installed the helper parens should indicate it like (Libreoffice is installed)").
Supersedes ADR-216 §1 only; §2 (the tool named short) stands.

1. **Every row of the format table has a `Kind`**, the group a person knows it by: Books, Images, Office documents,
   PostScript, Text, Web pages and email (`pdfops.Kind*`). It is a label for people and decides nothing; the status
   answer carries it as `kind`, and `TestTheTableViewsAgree` refuses a row without one of the six.
2. **The tooltip lists the groups alphabetically, and each group's formats alphabetically**, one line each.
3. **The parentheses say what this computer has:** `built in`, `<tool> is installed`, or `needs <tool>, not found
   on this computer`. "Installed" here means Nib found it, the same search every refusal reports on.
