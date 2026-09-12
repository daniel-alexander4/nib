# docs/red-proofs.md, tier 1: "the language table says one thing and the code does another"
# (PLAN-accessibility.md P03.S02, v1.129.33)
#
# The defect: a door is classified `declares` — meaning it routes through pdfops.SetLang — and it
# does not. The document ships with no language while a table in the tree says it has one.
#
# Recorded because this cross-check is the ONLY thing separating langdoor_test.go from a comment
# table, and a comment table keeps passing forever after the code stops matching it. It is checked
# in both directions: a `declares` row that reaches no door, and a row of any other class that
# does.
TIER="tier 1 — go test"
PROVE="go test . -run TestEveryAuthoringDoorSaysWhereItsLanguageComesFrom"
EXPECT="a language classification the code contradicts"
