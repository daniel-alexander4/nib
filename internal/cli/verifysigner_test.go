package cli

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestVerifyAgainstExpectedSigners — /pending 626. The CLI has no vault, so `nib verify` cannot tell a
// co-signer from a stranger: one party signs, someone else signs over the result to the end of the file, every
// signature is valid and the exit is 0. `--signer` names the keys the caller expects; the exit is then 2, with
// one line saying so, when a signature is from a key that was not named or a named key did not sign — and the
// JSON carries the same verdict. Without the option nothing changes.
func TestVerifyAgainstExpectedSigners(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	fps := map[string]string{}
	for _, name := range []string{"First", "Stranger", "Absent"} {
		certPEM, keyPEM, err := sign.GenerateIdentity(name)
		if err != nil {
			t.Fatal(err)
		}
		fp, err := sign.Fingerprint(certPEM)
		if err != nil {
			t.Fatal(err)
		}
		fps[name] = hex.EncodeToString(fp)
		if name == "Absent" {
			continue
		}
		if pdf, err = sign.SignApproval(pdf, certPEM, keyPEM, sign.Options{Name: name, When: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)}); err != nil {
			t.Fatalf("sign as %s: %v", name, err)
		}
	}
	p := filepath.Join(t.TempDir(), "resigned.pdf")
	if err := os.WriteFile(p, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	first, stranger, absent := fps["First"], fps["Stranger"], fps["Absent"]

	for _, tc := range []struct {
		name     string
		args     []string
		code     int
		says     string
		notSaid  string
		verdict  bool
		unexpect int
		missing  []string
	}{
		{"no key named: as before", nil, 0, "", "signers:", true, 0, nil},
		{"both keys named, one flag each", []string{"--signer", first, "--signer", stranger}, 0, "signers: every signature is from a key you named", "NOT THE EXPECTED", true, 0, nil},
		{"both keys named, comma-separated and upper case", []string{"--signer", strings.ToUpper(first) + "," + stranger}, 0, "signers: every signature", "NOT THE EXPECTED", true, 0, nil},
		{"a stranger signed too", []string{"--signer", first}, 2, "NOT THE EXPECTED SIGNERS: 1 signature(s) from a key you did not name", "did not sign", false, 1, []string{}},
		{"a named key did not sign", []string{"--signer", first, "--signer", stranger, "--signer", absent}, 2, "NOT THE EXPECTED SIGNERS: 1 key(s) you named did not sign (" + absent + ")", "did not name", false, 0, []string{absent}},
	} {
		out, code := captureStdout(t, func() int { return cmdVerify(append(append([]string{}, tc.args...), p)) })
		if code != tc.code {
			t.Errorf("%s: exit = %d, want %d\n%s", tc.name, code, tc.code, out)
		}
		if tc.says != "" && !strings.Contains(out, tc.says) {
			t.Errorf("%s: the report does not say %q:\n%s", tc.name, tc.says, out)
		}
		if strings.Contains(out, tc.notSaid) {
			t.Errorf("%s: the report says %q:\n%s", tc.name, tc.notSaid, out)
		}

		out, jcode := captureStdout(t, func() int { return cmdVerify(append(append([]string{"--json"}, tc.args...), p)) })
		if jcode != tc.code {
			t.Errorf("%s: --json exit = %d, want %d", tc.name, jcode, tc.code)
		}
		var got struct {
			ExpectedSigners *struct {
				AsExpected bool     `json:"asExpected"`
				Unexpected []string `json:"unexpected"`
				Missing    []string `json:"missing"`
			} `json:"expectedSigners"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("%s: --json: %v\n%s", tc.name, err, out)
		}
		if tc.args == nil {
			if got.ExpectedSigners != nil {
				t.Errorf("%s: --json carries expectedSigners with no key named: %s", tc.name, out)
			}
			continue
		}
		e := got.ExpectedSigners
		if e == nil {
			t.Fatalf("%s: --json carries no expectedSigners: %s", tc.name, out)
		}
		if e.AsExpected != tc.verdict || len(e.Unexpected) != tc.unexpect || strings.Join(e.Missing, ",") != strings.Join(tc.missing, ",") {
			t.Errorf("%s: --json verdict %+v, want asExpected %v, %d unexpected, missing %v", tc.name, *e, tc.verdict, tc.unexpect, tc.missing)
		}
		if tc.unexpect == 1 && (len(e.Unexpected) != 1 || !strings.EqualFold(e.Unexpected[0], stranger)) {
			t.Errorf("%s: --json unexpected = %v, want the stranger's key %s", tc.name, e.Unexpected, stranger)
		}
	}

	// A signature that did not verify discharges no expected key, and one whose key Nib could not establish is
	// from nobody who was named (ADR-051: an empty fingerprint matches nothing).
	e := signersAgainst(sign.Status{Signers: []sign.SignerInfo{{Fingerprint: first, Valid: false}, {Fingerprint: "", Valid: true}}}, []string{first})
	if e.AsExpected || len(e.Missing) != 1 || e.Missing[0] != first || len(e.Unexpected) != 1 || e.Unexpected[0] != "" {
		t.Errorf("an invalid signature by the named key beside an unidentified signer: %+v, want the key missing and one unexpected", *e)
	}

	// A value that is not a fingerprint is a usage error (1), never "that key did not sign" (2).
	if _, code := captureStdout(t, func() int { return cmdVerify([]string{"--signer", "First", p}) }); code != 1 {
		t.Errorf("--signer with a name instead of a fingerprint: exit = %d, want 1", code)
	}
}
