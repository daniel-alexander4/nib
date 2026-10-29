package pdfops

// digestRulePdfcpu names the pdfcpu release each ContentDigest rule was measured against. The digest is taken over
// pdfcpu's post-validation, post-optimize graph (`ReadOptimized`), so a pdfcpu bump can move it on unchanged bytes
// (the bump to v0.16.2 moved 24 of 2,326 real documents, and that is why rule 7 exists, D1). Only the current rule is
// listed: rules 5 and 6 are retired and nothing computes them. `TestTheDigestRuleNamesThePdfcpuItWasMeasuredWith`
// holds this beside go.mod's pin, so a later bump re-measures and bumps the rule instead of moving digests silently.
var digestRulePdfcpu = map[int]string{7: "v0.16.2"}
