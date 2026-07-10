// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/go-pcore/pcore"
)

// evalPure evaluates notice(<expr>) and returns the rendered message.
func evalPure(t *testing.T, expr string) string {
	t.Helper()
	return firstLog(t, "notice("+expr+")")
}

// evalPureErr evaluates src and returns whether it errored with a message
// containing want.
func evalPureErr(t *testing.T, src, want string) {
	t.Helper()
	_, _, err := EvalString(src)
	if err == nil {
		t.Fatalf("EvalString(%q): expected error containing %q, got none", src, want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("EvalString(%q): error %q lacks %q", src, err.Error(), want)
	}
}

func TestFqdnUUID(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`fqdn_uuid("puppetlabs.com")`, "9c70320f-6815-5fc5-ab0f-debe68bf764c"},
		{`fqdn_uuid("google.com")`, "64ee70a4-8cc1-5d25-abf2-dea6c79a09c8"},
		{`fqdn_uuid("0")`, "6af613b6-569c-5c22-9c37-2ed93f31d3af"},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	evalPureErr(t, `notice(fqdn_uuid())`, "No arguments")
	evalPureErr(t, `notice(fqdn_uuid("a","b"))`, "Too many arguments")
	evalPureErr(t, `notice(fqdn_uuid(5))`, "must be a String")
}

func TestCRC32(t *testing.T) {
	cases := []struct {
		v    Value
		want string
	}{
		{"abc", "352441c2"},
		{"acb", "5b384015"},
		{"my string", "18fbd270"},
		{"0", "f4dbdf21"},
		{int64(0), "f4dbdf21"},
		{int64(100), "237750ea"},
		{200.3, "7d5469f0"},
		{100.0, "a3fd429a"},
		{true, "fdfc4c8d"},
		{false, "2bcd6830"},
		{"\xFE\xED\xBE\xEF", "ac3481a4"},
	}
	for _, tc := range cases {
		got, err := builtinCRC32(nil, []Value{tc.v}, nil)
		if err != nil {
			t.Fatalf("crc32(%v): %v", tc.v, err)
		}
		if got != tc.want {
			t.Errorf("crc32(%v) = %q, want %q", tc.v, got, tc.want)
		}
	}
	if _, err := builtinCRC32(nil, nil, nil); err == nil {
		t.Error("crc32() with no args should error")
	}
}

func TestRegexpEscape(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`regexpescape("one*")`, `one\*`},
		{`regexpescape("a b#c")`, `a\ b\#c`},
		{`regexpescape("a.b+c")`, `a\.b\+c`},
		{`regexpescape(["one*","two"])`, `["one\\*", "two"]`},
		{`regexpescape([])`, `[]`},
		{`regexpescape(["one*",1,true,"two"])`, `["one\\*", 1, true, "two"]`},
		{`regexpescape(["ŏŉε*"])`, `["ŏŉε\\*"]`},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	// control characters
	if got := rubyRegexpEscape("a\tb\nc\rd\fe\vf"); got != `a\tb\nc\rd\fe\vf` {
		t.Errorf("control escape = %q", got)
	}
	evalPureErr(t, `notice(regexpescape(1))`, "either array or string")
	evalPureErr(t, `notice(regexpescape())`, "Wrong number")
}

func TestShellEscapeCompat(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`shell_escape("foo")`, "foo"},
		{`shell_escape("foo bar")`, `foo\ bar`},
		{`shell_escape("")`, "''"},
		{`shell_escape(10)`, "10"},
		{`shell_escape(false)`, "false"},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	// per-character Ruby Shellwords.shellescape rules: safe set unescaped,
	// everything else backslash-prefixed
	chars := []struct{ in, want string }{
		{"=", `\=`}, {":", ":"}, {"@", "@"}, {"#", `\#`}, {"'", `\'`},
		{"\\", `\\`}, {"/", "/"}, {",", ","}, {".", "."}, {"+", "+"},
		{"-", "-"}, {"_", "_"}, {"a", "a"}, {"9", "9"}, {"!", `\!`},
	}
	for _, c := range chars {
		if got := rubyShellEscape(c.in); got != c.want {
			t.Errorf("shellescape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// UTF-8: each multibyte rune is backslash-escaped
	if got := rubyShellEscape("スペー ス"); got != "\\ス\\ペ\\ー\\ \\ス" {
		t.Errorf("shellescape utf8 = %q", got)
	}
	// newline handling
	if got := rubyShellEscape("a\nb"); got != "a'\n'b" {
		t.Errorf("shellescape newline = %q", got)
	}
	evalPureErr(t, `notice(shell_escape("a","b"))`, "wrong number")
}

func TestShellJoin(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`shell_join(["foo"])`, "foo"},
		{`shell_join(["foo","bar"])`, "foo bar"},
		{`shell_join(["foo","bar baz"])`, `foo bar\ baz`},
		{`shell_join([10,false,"foo"])`, "10 false foo"},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	evalPureErr(t, `notice(shell_join("foo"))`, "must be an Array")
	evalPureErr(t, `notice(shell_join(["a"],["b"]))`, "wrong number")
}

func TestShellSplit(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"foo", []string{"foo"}},
		{"foo bar", []string{"foo", "bar"}},
		{"  foo   bar  ", []string{"foo", "bar"}},
		{`foo"bar baz"qux`, []string{"foobar bazqux"}},
		{`'single quoted'`, []string{"single quoted"}},
		{`a\ b`, []string{"a b"}},
		{`trailing\`, []string{"trailing\\"}},
		{`"a\"b"`, []string{`a"b`}},
		{`"a\\b"`, []string{`a\b`}},
	}
	for _, tc := range cases {
		got, err := shellSplit(tc.in)
		if err != nil {
			t.Fatalf("shellSplit(%q): %v", tc.in, err)
		}
		if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
			t.Errorf("shellSplit(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
	if _, err := shellSplit(`"unmatched`); err == nil {
		t.Error("unmatched double quote should error")
	}
	if _, err := shellSplit(`"trailing\`); err == nil {
		t.Error("double quote with trailing backslash should error")
	}
	evalPureErr(t, `notice(shell_split("'unmatched"))`, "Unmatched")
	if _, err := shellSplit(`'unmatched`); err == nil {
		t.Error("unmatched single quote should error")
	}
	// round-trip: shell_split inverts shell_join for arbitrary tokens,
	// including one full of shell metacharacters and UTF-8
	tokens := []string{"~`!@#$", "%^&*()_+-=", `[]\{}|;':"`, ",./<>?", "μťƒ", "s p a c e"}
	joinArr := make([]any, len(tokens))
	for i, tok := range tokens {
		joinArr[i] = tok
	}
	joined, err := builtinShellJoin(nil, []Value{joinArr}, nil)
	if err != nil {
		t.Fatal(err)
	}
	back, err := shellSplit(joined.(string))
	if err != nil {
		t.Fatalf("round-trip shellSplit(%q): %v", joined, err)
	}
	if strings.Join(back, "\x00") != strings.Join(tokens, "\x00") {
		t.Errorf("round-trip = %#v, want %#v", back, tokens)
	}
	// stringification and arity through the builtin
	if got := evalPure(t, `shell_split(10)`); got != `["10"]` {
		t.Errorf("shell_split(10) = %q", got)
	}
	evalPureErr(t, `notice(shell_split("a","b"))`, "wrong number")
}

func TestBatchEscape(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`batch_escape("foo")`, `"foo"`},
		{`batch_escape("foo bar")`, `"foo bar"`},
		{`batch_escape(10)`, `"10"`},
		{`batch_escape(false)`, `"false"`},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	if got, _ := builtinBatchEscape(nil, []Value{"~`!@#$%^&*()_-=[]\\{}|;':\",./<>?"}, nil); got != `"~`+"`"+`!@#\$%^&*()_-=[]\\{}|;':"",./<>?"` {
		t.Errorf("batch_escape metachars = %q", got)
	}
	if _, err := builtinBatchEscape(nil, nil, nil); err == nil {
		t.Error("batch_escape() arity error expected")
	}
}

func TestPowershellEscape(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`powershell_escape("foo")`, "foo"},
		{`powershell_escape("foo bar")`, "foo` bar"},
		{`powershell_escape(10)`, "10"},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	if got, _ := builtinPowershellEscape(nil, []Value{"~`!@#$%^&*()_-=[]\\{}|;':\",./<>?"}, nil); got != "~``!@#`$%^&*()_-=[]\\{}`|;`':\\`\",./<>?" {
		t.Errorf("powershell_escape metachars = %q", got)
	}
	if _, err := builtinPowershellEscape(nil, nil, nil); err == nil {
		t.Error("powershell_escape() arity error expected")
	}
}

func TestStr2SaltedSHA512(t *testing.T) {
	saved := saltedSHA512Seed
	defer func() { saltedSHA512Seed = saved }()
	saltedSHA512Seed = func() []byte { return []byte{0xa8, 0x5c, 0x9d, 0x6f} }

	cases := []struct{ in, want string }{
		{"", "a85c9d6f8c1eb1a625fd59e3cbca7dc7ab04ff1758d19ab99f098446e14a0a2a42e11afd1f4d6f17adfe2c772a3e6a821ee66a2564711431e14da96a3bff44593cf158ab"},
		{"password", "a85c9d6ff4e4dd6655ec2922ee9752550f2df4dc370e9739dd94899f62be6a42cc31fbfce3d62be35e0e8482696c931f63fb9286cf7b13d283660720c55f2a6304d06958"},
		{"verylongpassword", "a85c9d6fb810d0b8311c9a065c026e3179ae91fee3dbaf556f297e2fda2a8e3d8dd363977f9ef5c9b5da0cd518a5151a4e537928533291d68c9539d4d4b83da53b22a869"},
	}
	for _, tc := range cases {
		got, err := builtinStr2SaltedSHA512(nil, []Value{tc.in}, nil)
		if err != nil {
			t.Fatalf("str2saltedsha512(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("str2saltedsha512(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// random seed path (default) yields 4+64 bytes = 136 hex chars
	saltedSHA512Seed = saved
	got, _ := builtinStr2SaltedSHA512(nil, []Value{"x"}, nil)
	if s := got.(string); len(s) != 136 {
		t.Errorf("random-seed length = %d, want 136", len(s))
	}
	if _, err := hex.DecodeString(got.(string)); err != nil {
		t.Errorf("random-seed output not hex: %v", err)
	}
	if _, err := builtinStr2SaltedSHA512(nil, []Value{"a", "b"}, nil); err == nil {
		t.Error("arity error expected")
	}
	if _, err := builtinStr2SaltedSHA512(nil, []Value{int64(1)}, nil); err == nil {
		t.Error("type error expected")
	}
}

func TestStr2SaltedPBKDF2(t *testing.T) {
	got, err := builtinStr2SaltedPBKDF2(nil, []Value{"Pa55w0rd", "Using s0m3 s@lt", int64(50000)}, nil)
	if err != nil {
		t.Fatalf("str2saltedpbkdf2: %v", err)
	}
	m := got.(map[string]any)
	if m["password_hex"] != "3577f79f7d2e73df1cf1eecc36da16fffcd3650126d79e797a8b227492d13de4cdd0656933b43118b7361692f755e5b3c1e0536f826d12442400f3467bcc8fb4ac2235d5648b0f1b0906d0712aecd265834319b5a42e98af2ced81597fd78d1ac916f6eff6122c3577bb120a9f534e2a5c9a58c7d1209e3914c967c6a467b594" {
		t.Errorf("password_hex = %v", m["password_hex"])
	}
	if m["salt_hex"] != "5573696e672073306d332073406c74" {
		t.Errorf("salt_hex = %v", m["salt_hex"])
	}
	if m["iterations"] != int64(50000) {
		t.Errorf("iterations = %v", m["iterations"])
	}
	errCases := []struct {
		args []Value
		want string
	}{
		{[]Value{"Pa55w0rd", int64(2)}, "wrong number"},
		{[]Value{int64(1), "Using s0m3 s@lt", int64(50000)}, "first argument must be a string"},
		{[]Value{"Pa55w0rd", int64(1), int64(50000)}, "second argument must be a string"},
		{[]Value{"Pa55w0rd", "short", int64(50000)}, "at least 8 bytes"},
		{[]Value{"Pa55w0rd", "Using s0m3 s@lt", "50000"}, "must be an integer"},
		{[]Value{"Pa55w0rd", "Using s0m3 s@lt", int64(1)}, "between 40,000 and 70,000"},
		{[]Value{"Pa55w0rd", "Using s0m3 s@lt", int64(80000)}, "between 40,000 and 70,000"},
	}
	for _, tc := range errCases {
		if _, err := builtinStr2SaltedPBKDF2(nil, tc.args, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v: err=%v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestValidateDomainName(t *testing.T) {
	for _, ok := range []string{"com", "com.", "x.com", "foo.example.com.", "2foo.example.com", "www.example.2com", "10.10.10.10.10"} {
		if _, _, err := EvalString(`stdlib::validate_domain_name('` + ok + `')`); err != nil {
			t.Errorf("validate_domain_name(%q) unexpected error: %v", ok, err)
		}
	}
	evalPureErr(t, `notice(validate_domain_name("invalid domain","bar.com"))`, "got 'invalid domain'")
	evalPureErr(t, `notice(validate_domain_name(1))`, "got Integer")
	evalPureErr(t, `notice(validate_domain_name([]))`, "got Array")
	evalPureErr(t, `notice(validate_domain_name("-foo.example.com"))`, "got '-foo.example.com'")
	evalPureErr(t, `notice(validate_domain_name(""))`, "got ''")
	evalPureErr(t, `notice(validate_domain_name())`, "at least one")
}

func TestValidateEmailAddress(t *testing.T) {
	for _, ok := range []string{"bob@gmail.com", "alice+puppetlabs.com@gmail.com"} {
		if _, _, err := EvalString(`validate_email_address('` + ok + `')`); err != nil {
			t.Errorf("validate_email_address(%q) unexpected error: %v", ok, err)
		}
	}
	evalPureErr(t, `notice(validate_email_address("one"))`, "got 'one'")
	evalPureErr(t, `notice(validate_email_address(1))`, "got Integer")
	evalPureErr(t, `notice(validate_email_address(true))`, "got Boolean")
	evalPureErr(t, `notice(stdlib::validate_email_address("bob@gmail.com","one"))`, "got 'one'")
}

func TestToPythonToRuby(t *testing.T) {
	py := []struct{ expr, want string }{
		{`to_python("")`, `""`},
		{`to_python(undef)`, `None`},
		{`to_python(true)`, `True`},
		{`to_python(false)`, `False`},
		{`to_python("one")`, `"one"`},
		{`to_python(42)`, `42`},
		{`to_python([])`, `[]`},
		{`to_python(["one","two"])`, `["one", "two"]`},
		{`to_python({})`, `{}`},
		{`to_python({"key"=>"value"})`, `{"key": "value"}`},
		{`to_python("‰")`, `"‰"`},
	}
	for _, tc := range py {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	rb := []struct{ expr, want string }{
		{`to_ruby(undef)`, `nil`},
		{`to_ruby(true)`, `true`},
		{`to_ruby("one")`, `"one"`},
		{`to_ruby(42)`, `42`},
		{`to_ruby(["one","two"])`, `["one", "two"]`},
		{`to_ruby({"key"=>"value"})`, `{"key" => "value"}`},
		{`to_ruby(1.5)`, `1.5`},
		{`stdlib::to_ruby("竹")`, `"竹"`},
	}
	for _, tc := range rb {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	// nested
	nestPy := `to_python({"one"=>{"oneA"=>"A","oneB"=>{"oneB1"=>"1","oneB2"=>"2"}},"two"=>["twoA","twoB"]})`
	if got := evalPure(t, nestPy); got != `{"one": {"oneA": "A", "oneB": {"oneB1": "1", "oneB2": "2"}}, "two": ["twoA", "twoB"]}` {
		t.Errorf("nested to_python = %q", got)
	}
	// a non-scalar, non-collection value (a Type) falls through to stringify
	if got := evalPure(t, `to_ruby(String)`); got != "String" {
		t.Errorf("to_ruby(String) = %q", got)
	}
	evalPureErr(t, `notice(to_python())`, "wrong number")
	evalPureErr(t, `notice(to_ruby())`, "wrong number")
}

func TestRubyFloatToS(t *testing.T) {
	cases := []struct {
		f    float64
		want string
	}{
		{100.0, "100.0"},
		{200.3, "200.3"},
		{1e20, "1.0e+20"},
		{1.5e20, "1.5e+20"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
		{math.NaN(), "NaN"},
	}
	for _, tc := range cases {
		if got := rubyFloatToS(tc.f); got != tc.want {
			t.Errorf("rubyFloatToS(%v) = %q, want %q", tc.f, got, tc.want)
		}
	}
}

func TestRubyStringInspect(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", `"plain"`},
		{"a\"b\\c", `"a\"b\\c"`},
		{"tab\tnl\ncr\r", `"tab\tnl\ncr\r"`},
		{"a#{b}", `"a\#{b}"`},
		{"a#b", `"a#b"`},
		{"\a\b\f\v\x1b", `"\a\b\f\v\e"`},
		{"\x01", `"\x01"`},
		{"‰", `"‰"`},
	}
	for _, tc := range cases {
		if got := rubyStringInspect(tc.in); got != tc.want {
			t.Errorf("rubyStringInspect(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseHocon(t *testing.T) {
	simple := []struct{ expr, want string }{
		{`parsehocon("")`, `{}`},
		{`parsehocon("{a: 1, b: 1.5}")`, `{"a" => 1, "b" => 1.5}`},
		{`parsehocon("{a: [1,2], b: {c: true}}")`, `{"a" => [1, 2], "b" => {"c" => true}}`},
		{`parsehocon("invalid", "default")`, `default`},
	}
	for _, tc := range simple {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	// HOCON null narrows to Puppet undef (rendered empty inside a hash)
	if got := evalPure(t, `parsehocon("{a: null}")`); got != `{"a" => }` {
		t.Errorf("parsehocon null = %q", got)
	}
	evalPureErr(t, `notice(parsehocon("invalid"))`, "parsehocon")
	evalPureErr(t, `notice(parsehocon())`, "wrong number")
	evalPureErr(t, `notice(parsehocon(5))`, "must be a String")
}

func TestIsA(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`is_a("hello", String)`, "true"},
		{`is_a(5, String)`, "false"},
		{`is_a(5, Integer)`, "true"},
		{`is_a("このテキスト", String)`, "true"},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	evalPureErr(t, `notice(is_a("x","y"))`, "must be a Type")
	evalPureErr(t, `notice(is_a("x"))`, "wrong number")
}

func TestEncloseIPv6(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`enclose_ipv6("::1")`, `["[::1]"]`},
		{`enclose_ipv6("192.168.0.1")`, `["192.168.0.1"]`},
		{`enclose_ipv6("*")`, `["*"]`},
		{`enclose_ipv6(["::1","::1","2001:db8::1"])`, `["[::1]", "[2001:db8::1]"]`},
		{`enclose_ipv6(["192.168.0.1", undef])`, `["192.168.0.1"]`},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	evalPureErr(t, `notice(enclose_ipv6("not an ip"))`, "not an ip address")
	evalPureErr(t, `notice(enclose_ipv6(5))`, "expected String or Array")
	evalPureErr(t, `notice(enclose_ipv6("a","b"))`, "Wrong number")
}

func TestIPInRange(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`stdlib::ip_in_range("192.168.100.12","192.168.100.0/24")`, "true"},
		{`stdlib::ip_in_range("192.168.100.12",["10.10.10.10/24","192.168.100.0/24"])`, "true"},
		{`stdlib::ip_in_range("10.10.10.10","192.168.100.0/24")`, "false"},
		{`stdlib::ip_in_range("192.168.100.12","192.168.100.12")`, "true"},
		{`stdlib::ip_in_range("::1","::/0")`, "true"},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	// bad CIDR / bad addr in range list yield no match
	if got := evalPure(t, `stdlib::ip_in_range("10.0.0.1",["bad/xx","nonsense"])`); got != "false" {
		t.Errorf("bad ranges = %q", got)
	}
	evalPureErr(t, `notice(stdlib::ip_in_range("nonsense","10.0.0.0/8"))`, "invalid IP")
	evalPureErr(t, `notice(stdlib::ip_in_range(5,"10.0.0.0/8"))`, "must be a String")
	evalPureErr(t, `notice(stdlib::ip_in_range("10.0.0.1",5))`, "String or Array")
	evalPureErr(t, `notice(stdlib::ip_in_range("a"))`, "expects 2")
}

func TestNestedValues(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`stdlib::nested_values({})`, `[]`},
		{`stdlib::nested_values({"key"=>"value"})`, `["value"]`},
		{`stdlib::nested_values({"key"=>{"key1"=>"value1","key2"=>"value2"}})`, `["value1", "value2"]`},
		{`stdlib::nested_values({"key1"=>"value1","key2"=>{"key1"=>"value21","key2"=>"value22"},"key3"=>"value3"})`, `["value1", "value21", "value22", "value3"]`},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	evalPureErr(t, `notice(stdlib::nested_values(2))`, "must be a Hash")
	evalPureErr(t, `notice(stdlib::nested_values())`, "wrong number")
}

func TestXMLEncode(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`stdlib::xml_encode("this is \"my\" complicated <String>")`, `this is "my" complicated &lt;String&gt;`},
		{`stdlib::xml_encode("this is \"my\" complicated <String>", "text")`, `this is "my" complicated &lt;String&gt;`},
		{`stdlib::xml_encode("this is \"my\" complicated <String>", "attr")`, `"this is &quot;my&quot; complicated &lt;String&gt;"`},
		{`stdlib::xml_encode("a & b")`, `a &amp; b`},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	evalPureErr(t, `notice(stdlib::xml_encode("x","bogus"))`, "text' or 'attr")
	evalPureErr(t, `notice(stdlib::xml_encode(5))`, "must be a String")
	evalPureErr(t, `notice(stdlib::xml_encode("x", 5))`, "must be a String")
	evalPureErr(t, `notice(stdlib::xml_encode())`, "wrong number")
}

func TestHasFunction(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`stdlib::has_function("stdlib::has_function")`, "true"},
		{`stdlib::has_function("md5")`, "true"},
		{`stdlib::has_function("not_a_function")`, "false"},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	evalPureErr(t, `notice(stdlib::has_function(5))`, "must be a String")
	evalPureErr(t, `notice(stdlib::has_function())`, "wrong number")
}

func TestSortBy(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`["The","quick","brown","fox"].stdlib::sort_by |$e| { $e }`, `["The", "brown", "fox", "quick"]`},
		{`[3,1,2].sort_by |$e| { $e }`, `[1, 2, 3]`},
		{`"dcba".stdlib::sort_by |$e| { $e }`, `abcd`},
		{`{"b"=>2,"a"=>1}.stdlib::sort_by |$e| { $e[1] }`, `{"a" => 1, "b" => 2}`},
		{`{"b"=>2,"a"=>1}.stdlib::sort_by |$k,$v| { $v }`, `{"a" => 1, "b" => 2}`},
	}
	for _, tc := range cases {
		if got := evalPure(t, tc.expr); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expr, got, tc.want)
		}
	}
	evalPureErr(t, `notice([1,2].stdlib::sort_by)`, "expects a block")
	evalPureErr(t, `notice(stdlib::sort_by(5) |$e| { $e })`, "expects an Array value, got Integer")
	evalPureErr(t, `notice(stdlib::sort_by([1],[2]) |$e| { $e })`, "wrong number")
	// block error propagates for each collection kind
	evalPureErr(t, `notice([1].stdlib::sort_by |$e| { fail("boom") })`, "boom")
	evalPureErr(t, `notice("ab".stdlib::sort_by |$e| { fail("boom") })`, "boom")
	evalPureErr(t, `notice({"a"=>1}.stdlib::sort_by |$e| { fail("boom") })`, "boom")
}

func TestSpaceship(t *testing.T) {
	if spaceship(int64(1), int64(2)) != -1 || spaceship(int64(2), int64(1)) != 1 || spaceship(int64(1), int64(1)) != 0 {
		t.Error("numeric spaceship wrong")
	}
	if spaceship("a", "b") != -1 {
		t.Error("string spaceship wrong")
	}
	// mixed types fall back to stringify comparison
	if spaceship(int64(1), "a") == 0 {
		t.Error("mixed spaceship should not be equal")
	}
}

func TestValidateX509RSAKeyPair(t *testing.T) {
	certPEM, keyPEM := genCertKey(t)
	_, otherKey := genCertKey(t)

	if _, err := builtinValidateX509RSAKeyPair(nil, []Value{certPEM, keyPEM}, nil); err != nil {
		t.Errorf("matching pair should validate: %v", err)
	}
	if _, err := builtinValidateX509RSAKeyPair(nil, []Value{certPEM, otherKey}, nil); err == nil {
		t.Error("mismatched key should fail")
	}
	errCases := []struct {
		args []Value
		want string
	}{
		{[]Value{certPEM}, "wrong number"},
		{[]Value{int64(1), keyPEM}, "must be a String"},
		{[]Value{certPEM, int64(1)}, "must be a String"},
		{[]Value{"not a cert", keyPEM}, "Not a valid x509"},
		{[]Value{certPEM, "not a key"}, "Not a valid RSA"},
	}
	for _, tc := range errCases {
		if _, err := builtinValidateX509RSAKeyPair(nil, tc.args, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v: err=%v, want %q", tc.args, err, tc.want)
		}
	}
	// PKCS8-encoded key is also accepted
	pkcs8 := pkcs8KeyFromPEM(t, keyPEM)
	if _, err := builtinValidateX509RSAKeyPair(nil, []Value{certPEM, pkcs8}, nil); err != nil {
		t.Errorf("PKCS8 key should validate: %v", err)
	}
}

// genCertKey returns a self-signed RSA certificate and its PKCS1 private key,
// both PEM-encoded.
func genCertKey(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(1<<31, 0),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return certPEM, keyPEM
}

func pkcs8KeyFromPEM(t *testing.T, keyPEM string) string {
	t.Helper()
	block, _ := pem.Decode([]byte(keyPEM))
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestStdlibAliasesRegistered(t *testing.T) {
	e := New()
	for _, name := range []string{
		"stdlib::end_with", "stdlib::start_with", "stdlib::extname", "stdlib::type_of",
		"stdlib::to_json", "stdlib::to_json_pretty", "stdlib::to_yaml",
		"stdlib::to_python", "stdlib::to_ruby",
		"stdlib::batch_escape", "stdlib::powershell_escape", "stdlib::shell_escape",
		"stdlib::crc32", "stdlib::sort_by", "stdlib::parsehocon",
	} {
		if _, ok := e.funcs[name]; !ok {
			t.Errorf("alias %q not registered", name)
		}
	}
	// non-RSA PKCS8 key path
	if _, err := parseRSAKeyPEM("-----BEGIN X-----\nAAAA\n-----END X-----\n"); err == nil {
		t.Error("garbage PEM should fail")
	}
	if _, err := parseRSAKeyPEM("no pem here"); err == nil {
		t.Error("non-PEM should fail")
	}
	// a valid PKCS8 key that is not RSA is rejected
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ec)
	if err != nil {
		t.Fatal(err)
	}
	ecPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if _, err := parseRSAKeyPEM(ecPEM); err == nil {
		t.Error("non-RSA PKCS8 key should be rejected")
	}
	if _, err := parseCertPEM("no pem here"); err == nil {
		t.Error("non-PEM cert should fail")
	}
	_ = pcore.Undef
}
