# docs/red-proofs.md, tier 1: "a doc comment sits on a const it does not name" (/pending 511,
# v1.133.7)
#
# The defect: `Vault.save`'s one-line doc put back above `envelopeVersion`'s, with no blank line
# between them — so both bind to `const envelopeVersion` and `func (v *Vault) save()` has no doc.
#
# It is recorded on a const rather than a func because that is precisely what
# `TestEveryDocCommentNamesItsOwnFunction` could not see: it walks `*ast.FuncDecl` only, so the
# const is never inspected, and the function on the receiving end reads as merely undocumented.
# With this patch applied that older guard stays GREEN and this one goes red, which is the whole
# reason the second guard exists.
TIER="tier 1 — go test"
PROVE="go test . -run TestEveryTypeConstAndVarDocNamesItsOwnDeclaration"
EXPECT="sit on a type, const or var they do not name"
