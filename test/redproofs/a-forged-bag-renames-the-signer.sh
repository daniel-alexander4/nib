# docs/red-proofs.md, tier 1: "the signer is the certificate the SignerInfo names" (ADR-051, /pending 613)
#
# The defect: reading the signer's identity out of the certificate bag's FIRST element. The bag is
# a PKCS#7 SET sitting in the /Contents hole in the /ByteRange — unsigned, unordered, and chosen by
# whoever produced the file — while p7.Verify resolves the signer by issuer and serial. So a
# signature made with the attacker's own key, carrying the victim's certificate first, verified
# Valid under the VICTIM's fingerprint, which p2p.Completeness counts towards the roster.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestAForgedBagDoesNotRenameTheSigner"
EXPECT="forged bag renamed the signer"
