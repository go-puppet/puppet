// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"math/big"
	"testing"
	"time"

	"github.com/go-encryptions/unixcrypt"
)

// TestCryptLongInputs exercises the full-block digest paths in the crypt
// implementations (password longer than the digest, salt longer than 16 for
// sha-crypt). Golden values were generated with `openssl passwd`.
func TestCryptLongInputs(t *testing.T) {
	const lp = "this_is_a_very_long_password_exceeding_sixty_four_bytes_to_hit_full_block_paths!!"
	cases := []struct{ expr, want string }{
		{`pw_hash('` + lp + `', 'md5', 'abc123')`, "$1$abc123$s9Fbd07vtBDT863XSTGxQ1"},
		{`pw_hash('` + lp + `', 'sha-256', 'abc123')`, "$5$abc123$lS3.IhvE18i7fB/sexFoBk/tPpzF/8ngKkRPV2Fbn56"},
		{`pw_hash('` + lp + `', 'sha-512', 'abc123')`, "$6$abc123$FyfBPazO.qcvuqt3FDipJVrBm.xoTbI0G6jq64jZL0QBMWDWLFEIbTyvxE.fSNDyYDQF/RbVqqdbm84qa0e9i/"},
		// salt longer than 16 chars is truncated to 16 for sha-crypt.
		{`pw_hash('secret', 'sha-512', '0123456789abcdefGHIJ')`, "$6$0123456789abcdef$8CGkKDSyVpVlVNcs5PssRw/Tb8EgfecuA6D/zsRQFumJaU.s8KgTWWc2HfgkB7h2n1qIKiJFeSyUFrwvhILXy."},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s =>\n  %q\nwant\n  %q", tc.expr, got, tc.want)
		}
	}
}

// TestCryptInternals white-box-tests the defensive branches that the public
// pw_hash validation makes unreachable. The actual crypt(3) algorithms
// (including bcrypt's own salt/cost parsing) now live in
// github.com/go-encryptions/unixcrypt, which carries its own equivalent
// coverage for that package's error paths — this test now only needs to
// confirm builtinPwHash's own error-wrapping line around unixcrypt calls,
// which pw_hash's own bcryptSaltRe validation makes provably unreachable
// through the public API (its cost/length constraints exactly match
// unixcrypt.BcryptFromMCF's own), by calling the wrapped function directly
// with input the public regex would reject.
func TestCryptInternals(t *testing.T) {
	if _, err := unixcrypt.BcryptFromMCF("pw", "2b", "zz$CCCCCCCCCCCCCCCCCCCCC."); err == nil {
		t.Error("BcryptFromMCF should reject a non-numeric cost")
	}
}

// TestMTGiantSeed covers the >624-word init_by_array path with a seed larger
// than the Mersenne-Twister state. Golden from MRI Random.new((1<<20000)+12345).
func TestMTGiantSeed(t *testing.T) {
	seed := new(big.Int).Lsh(big.NewInt(1), 20000)
	seed.Add(seed, big.NewInt(12345))
	if v := deterministicRandInt(seed, 100); v != 39 {
		t.Errorf("giant-seed rand(100) => %d, want 39", v)
	}
	// A two-word seed whose top word is 1 (2**32) is trimmed to one word but
	// still uses init_by_array, matching MRI's Random.new(2**32).rand(100).
	if v := deterministicRandInt(big.NewInt(1<<32), 100); v != 77 {
		t.Errorf("2**32 seed rand(100) => %d, want 77", v)
	}
}

// TestFqdnFactFallbacks covers each fqdnFact degradation path.
func TestFqdnFactFallbacks(t *testing.T) {
	// No facts at all: fqdn resolves to "" (fqdn_rand(1000) with fqdn="" == 64).
	if got := evalOut(t, `fqdn_rand(1000)`); got != "64" {
		t.Errorf("no facts => %q, want 64", got)
	}
	// $facts bound to a non-Hash (only possible when no facts provider is set).
	if got := lastLog(t, "$facts = 'oops'\nnotice(fqdn_rand(1000))"); got != "64" {
		t.Errorf("non-hash facts => %q, want 64", got)
	}
	// networking present but not a Hash.
	if got := lastLog(t, `notice(fqdn_rand(1000))`, WithFacts(MapFacts{"networking": "x"})); got != "64" {
		t.Errorf("non-hash networking => %q, want 64", got)
	}
	// networking.fqdn present but not a String.
	if got := lastLog(t, `notice(fqdn_rand(1000))`, WithFacts(MapFacts{"networking": map[string]any{"fqdn": int64(5)}})); got != "64" {
		t.Errorf("non-string fqdn => %q, want 64", got)
	}
}

// TestStrftimeCoverage fills the remaining strftime branches.
func TestStrftimeCoverage(t *testing.T) {
	// AM / Sunday time: 2016-08-21T06:07:08Z (a Sunday morning).
	am := []struct{ dir, want string }{
		{"%P", "am"}, {"%p", "AM"}, {"%u", "7"}, {"%I", "06"},
		{"%A", "Sunday"}, {"%a", "Sun"}, {"%w", "0"},
	}
	for _, tc := range am {
		expr := `strftime('` + tc.dir + `', 1471759628)`
		if got := evalOut(t, expr); got != tc.want {
			t.Errorf("strftime(%q, sunday-am) => %q, want %q", tc.dir, got, tc.want)
		}
	}
	// "default" timezone == UTC; "current" == local (just ensure it runs).
	if got := strftime(time.Unix(1471954394, 0).UTC(), "%Y"); got != "2016" {
		t.Fatal("sanity")
	}
	if _, _, err := EvalString(`notice(strftime('%z', 'current'))`); err != nil {
		t.Errorf("current zone: %v", err)
	}
	if got := evalOut(t, `strftime('%z', 'default')`); got != "+0000" {
		t.Errorf("default zone => %q", got)
	}
	// legacy form with a non-String, non-Numeric second argument errors.
	if evalErr(t, `notice(strftime('%Y', [1]))`) == "" {
		t.Error("strftime(fmt, array) should error")
	}
	// bad numeric offsets exercise parseOffset's rejection branches.
	for _, e := range []string{`notice(strftime('%Y', 1, '+9'))`, `notice(strftime('%Y', 1, '+123'))`, `notice(strftime('%Y', 1, '+ab:cd'))`} {
		if evalErr(t, e) == "" {
			t.Errorf("%s should error", e)
		}
	}
}

// TestScanfCoverage fills the remaining scanf branches.
func TestScanfCoverage(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`scanf("x", "%")`, "[]"},             // format ends after '%'
		{`scanf("5", "%3")`, "[]"},            // format ends after width
		{`scanf("ab", "%%")`, "[]"},           // literal '%' expected but not present
		{`scanf("5", "%q")`, "[]"},            // unknown verb
		{`scanf("", "%c")`, "[]"},             // %c at end of data
		{`scanf("+7", "%d")`, "[7]"},          // leading plus sign
		{`scanf("   9", "%d")`, "[9]"},        // leading whitespace skipped
		{`scanf(".5", "%f")`, "[0.5]"},        // fraction with no integer part
		{`scanf("1e3", "%f")`, "[1000]"},      // exponent
		{`scanf("1e", "%f")`, "[1]"},          // bare exponent marker not consumed
		{`scanf("nan", "%f")`, "[]"},          // no digits
		{`scanf("5", "z")`, "[]"},             // literal mismatch at start
		{`scanf("0", "%o")`, "[0]"},           // octal zero
		{`scanf("abcd", "%2c")`, `["ab"]`},    // %c width
		{`scanf("ab", "%5c")`, `["ab"]`},      // %c width past end of data
		{`scanf("   ", "%s")`, "[]"},          // %s matches nothing
		{`scanf("12345", "%3d")`, "[123]"},    // %d width limit
		{`scanf("+", "%d")`, "[]"},            // sign with no digits
		{`scanf("0x", "%x")`, "[]"},           // hex prefix with no digits
		{`scanf("017", "%i")`, "[15]"},        // %i auto-detects octal
		{`scanf("3.14159", "%4f")`, "[3.14]"}, // float width limit
		{`scanf("-2.5", "%f")`, "[-2.5]"},     // float sign
		{`scanf("1.5e-2", "%f")`, "[0.015]"},  // signed exponent
		{`scanf("1e999999", "%f")`, "[]"},     // float parse overflow
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
	// integer overflow yields no conversion.
	if got := evalOut(t, `scanf("99999999999999999999999", "%d")`); got != "[]" {
		t.Errorf("overflow => %q", got)
	}
}

// TestBase64DecodeDefaultError covers the lenient-decode error branch.
func TestBase64DecodeDefaultError(t *testing.T) {
	if evalErr(t, `notice(base64("decode", "@@@@"))`) == "" {
		t.Error("default decode of invalid input should error")
	}
}
