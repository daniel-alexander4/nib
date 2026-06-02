//go:build !embedkeys

package vault

// The default build ships with no built-in authorized keys or signatures: these
// stay empty, so every built-in-key code path is inert. A build may populate
// them via an embed.
var (
	builtinKeysRaw        string
	builtinSignaturesBlob []byte
)
