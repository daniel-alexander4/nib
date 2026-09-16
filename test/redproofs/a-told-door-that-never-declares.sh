# docs/red-proofs.md, tier 1: "a door the table says declares what it is told, and it declares nothing"
# (/pending 471, v1.129.109)
#
# The defect: `nib office --lang de` accepts the language, validates it, and never writes it — the
# document ships carrying LibreOffice's locale guess while langdoor_test.go classifies the door
# `told`, meaning it reaches pdfops.SetLang when the user names a language.
#
# Recorded because `told` is new vocabulary in that guard, and a class the cross-check does not
# enforce is a comment. The patch keeps the LangTag call so the flag still refuses a bad tag: the
# only thing missing is the declaration, which is exactly what the table claims.
TIER="tier 1 — go test"
PROVE="go test . -run TestEveryAuthoringDoorSaysWhereItsLanguageComesFrom"
EXPECT='is classified "told" and never reaches pdfops.SetLang'
