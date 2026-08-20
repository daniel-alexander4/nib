package rendezvous

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestNothingHereCanReachAnIdentity is L1 as a source guard.
//
// # It exists because this package's own doc comment said it already did
//
// The package comment shipped at P04.S01 saying "a guard enforces that — the same
// structural shape internal/discovery carries for the same law", and no such guard was
// written. That is the repo's most-repeated defect wearing its most convincing costume:
// a claim about code, in the code, that reads as verification.
//
// # And it is not a copy of discovery's, because that one has a hole
//
// `internal/discovery`'s version walks `p.Names` — the parameter and result *names* —
// and never looks at their types. So `func Classify([]byte) [32]byte` is invisible to
// it, and so is any accessor with unnamed results. A guard that reads names but not
// types is satisfied by prose, which is the shape this project has been bitten by more
// than once. This one renders the types.
//
// L1: nothing learned from the rendezvous may influence which peer is accepted. The
// classification this package produces is diagnostic — it changes messages and tier
// preference, never the pin check.
func TestNothingHereCanReachAnIdentity(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0) // no ParseComments — a comment must not be able to satisfy any of this
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["rendezvous"]
	if !ok || len(pkg.Files) == 0 {
		t.Fatal("no non-test files parsed — every check below would pass on nothing")
	}

	// 1. No import of anything that holds or decides identity.
	//
	// `internal/ceremony` is on this list although P04.S03 will want the rendezvous key
	// it derives. That is deliberate: the key must be handed in as opaque bytes by the
	// caller, exactly as `internal/server/discover.go` does the resolving that
	// `internal/discovery` may not. An Invitation carries a pinned fingerprint, and a
	// package that can read one is a package that can participate in acceptance.
	forbiddenImports := map[string]string{
		"nib/internal/vault":    "holds the identity key",
		"nib/internal/sign":     "is the signing path",
		"nib/internal/p2p":      "is where the pinned-peer check lives",
		"nib/internal/ceremony": "carries the invitation, and the invitation pins a fingerprint",
		"nib/internal/pairing":  "is the fingerprint comparison itself",
	}
	for name, f := range pkg.Files {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if why, bad := forbiddenImports[path]; bad {
				t.Errorf("%s imports %s, which %s. The rendezvous must not be able to "+
					"consult, or become, the thing that decides which peer is accepted (L1).",
					name, path, why)
			}
		}
	}

	// 2. No exported symbol names or CARRIES an identity — names and types both.
	// **Widened at P04.S03**, which is the slice that made the gap live. The original
	// alternation matches `ed25519.PublicKey` (via `publickey`) but NOT
	// `ed25519.PrivateKey`, `ed25519.Seed` or a bare `[32]byte` — so the natural WRITE
	// signature for a BEP-44 publisher would have sailed straight through the guard
	// written to keep key material out of this package, while the read signature tripped
	// it as a false positive. A guard that is wrong in both directions on the one change
	// it was built for is how guards get loosened until they guard nothing.
	//
	// `ed25519` is added rather than `key`/`seed`/`secret`: an opaque `seed []byte`
	// handed in by the caller is the COMPLIANT shape here — it is how the rendezvous key
	// crosses the line without this package being able to derive it — so banning the word
	// would forbid the very thing L1 requires. What must not appear is a KEY TYPE, which
	// is a package this file can name exactly.
	fp := regexp.MustCompile(`(?i)fingerprint|spki|pubkey|publickey|privatekey|ed25519|identity|certificate`)

	// A POSITIVE CONTROL for the pattern itself.
	//
	// The stimulus guards below prove the WALK is not vacuous — that it collected exported
	// symbols and rendered types. Nothing proved the PATTERN still matches anything: an
	// edit that broke the alternation would leave a walk over real symbols scoring zero
	// hits, which is indistinguishable from a clean package.
	for _, bad := range []string{
		"priv ed25519.PrivateKey", "ed25519.PublicKey", "fp fingerprint",
		"spki []byte", "cert certificate",
	} {
		if !fp.MatchString(bad) {
			t.Fatalf("the identity pattern does not match %q — it has stopped matching "+
				"anything, and every check below then scores zero on a dirty package "+
				"exactly as it does on a clean one", bad)
		}
	}
	if fp.MatchString("conn net.PacketConn dir string") {
		t.Fatal("the identity pattern matches an ordinary signature — it would refuse " +
			"everything, which is the other way a guard stops discriminating")
	}
	exported, sawType := 0, false
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || !fd.Name.IsExported() {
				return true
			}
			exported++
			var sb strings.Builder
			add := func(fl *ast.FieldList) {
				if fl == nil {
					return
				}
				for _, p := range fl.List {
					for _, nm := range p.Names {
						sb.WriteString(nm.Name + " ")
					}
					// The half discovery's guard is missing.
					if ts := types.ExprString(p.Type); ts != "" {
						sb.WriteString(ts + " ")
						sawType = true
					}
				}
			}
			add(fd.Type.Params)
			add(fd.Type.Results)
			if fp.MatchString(sb.String()) {
				t.Errorf("%s: exported %s names or carries an identity in its signature (%q) "+
					"— L1 forbids this package participating in who is accepted",
					name, fd.Name.Name, sb.String())
			}
			return true
		})
	}

	// 2b. The same rule for exported TYPES and their fields.
	//
	// The signature walk above only sees *ast.FuncDecl, so `type SelfAddress struct {
	// Fingerprint [32]byte }` passes it untouched — and a struct field is a far more
	// natural place for identity to arrive than a parameter. Same regex, applied to the
	// declaration this package actually publishes state through.
	types_ := 0
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || !ts.Name.IsExported() {
				return true
			}
			types_++
			var sb strings.Builder
			sb.WriteString(ts.Name.Name + " ")
			if st, ok := ts.Type.(*ast.StructType); ok && st.Fields != nil {
				for _, fld := range st.Fields.List {
					for _, nm := range fld.Names {
						sb.WriteString(nm.Name + " ")
					}
					sb.WriteString(types.ExprString(fld.Type) + " ")
				}
			}
			if fp.MatchString(sb.String()) {
				t.Errorf("%s: exported type %s names or carries an identity (%q) — L1 "+
					"forbids this package holding one at all", name, ts.Name.Name, sb.String())
			}
			return true
		})
	}
	if types_ == 0 {
		t.Fatal("no exported types were inspected; the type check is vacuous")
	}

	// 2c. A bare fixed-size byte array in an exported signature.
	//
	// The pattern above cannot see `[32]byte`, and the widening comment used to imply it
	// could. That type is exactly the shape of a BEP-44 public key — and of the value
	// `keyPair` returns — so an exported `func HopKey() [32]byte` would carry key material
	// straight past the guard written to keep key material out. Measured: it did.
	//
	// A TYPE check rather than a name check, because there is no word to match: the whole
	// hazard is that the type says nothing about itself. Opaque `[]byte` stays allowed —
	// that is the compliant shape L1 requires, since it is how the rendezvous key crosses
	// the line without this package being able to derive it. A fixed 32- or 64-byte array
	// is not opaque; it is a key or a signature wearing no name.
	arrayKey := regexp.MustCompile(`^\[(32|64)\]byte$`)
	arraysChecked := 0
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || !fd.Name.IsExported() {
				return true
			}
			for _, fl := range []*ast.FieldList{fd.Type.Params, fd.Type.Results} {
				if fl == nil {
					continue
				}
				for _, prm := range fl.List {
					arraysChecked++
					if arrayKey.MatchString(types.ExprString(prm.Type)) {
						t.Errorf("%s: exported %s carries a bare %s — that is the shape of a "+
							"BEP-44 key, and no name on it can be matched. Hand it across as "+
							"opaque []byte, as the rendezvous key already is.",
							name, fd.Name.Name, types.ExprString(prm.Type))
					}
				}
			}
			return true
		})
	}
	if arraysChecked == 0 {
		t.Fatal("the array walk inspected no parameters at all — it would pass on nothing")
	}

	// 3. The learned address must never become our own DHT node id.
	//
	// It is the natural next thought — "we know our public IP now, so let's have a
	// BEP-42-secure node id" — and it hands an attacker-chosen value control of our
	// position in the DHT keyspace: `InitNodeId` derives the id from `PublicIP`
	// (server.go:174-193), and an id other nodes judge insecure gets us dropped. Today
	// PublicIP is nil and the id is random, which is the safe state.
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && id.Name == "PublicIP" {
				t.Errorf("%s names PublicIP — a self-address learned from strangers must not "+
					"decide our own node id", name)
			}
			return true
		})
	}

	// Stimulus, both halves. Without these the checks above are equally true of a
	// package with no exported functions, and of a type walk that collected nothing.
	if exported == 0 {
		t.Fatal("no exported functions were inspected; the signature check is vacuous")
	}
	if !sawType {
		t.Fatal("the walk collected no TYPE text — it is reading names only, which is the " +
			"exact hole this guard was written to close")
	}
}
