// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"math/big"
	"sort"
	"strings"
	"testing"
	"time"
)

// hostFacts binds $facts['networking']['fqdn'] for the fqdn_* functions. The
// golden values below were generated with MRI Ruby using this exact fqdn.
func hostFacts(fqdn string) Option {
	return WithFacts(MapFacts{"networking": map[string]any{"fqdn": fqdn}})
}

// --- digests --------------------------------------------------------------

func TestDigests(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`md5("hello")`, "5d41402abc4b2a76b9719d911017c592"},
		{`sha1("hello")`, "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"},
		{`sha256("hello")`, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		{`sha512("hello")`, "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043"},
		{`stdlib::sha256("hello")`, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
	for _, e := range []string{`notice(md5())`, `notice(md5(5))`} {
		if got := evalErr(t, e); !strings.Contains(got, "argument") && !strings.Contains(got, "wrong number") {
			t.Errorf("%s => %q", e, got)
		}
	}
}

// --- deterministic random -------------------------------------------------

func TestFqdnRand(t *testing.T) {
	cases := []struct {
		expr, fqdn, want string
	}{
		{`fqdn_rand(30)`, "host.example.com", "3"},
		{`fqdn_rand(30, "seed1")`, "host.example.com", "25"},
		{`fqdn_rand(100)`, "web01.corp.internal", "2"},
		{`fqdn_rand(1000)`, "", "64"},
	}
	for _, tc := range cases {
		if got := lastLog(t, "notice("+tc.expr+")", hostFacts(tc.fqdn)); got != tc.want {
			t.Errorf("%s (fqdn=%q) => %q, want %q", tc.expr, tc.fqdn, got, tc.want)
		}
	}
	// downcase flag lowercases the fqdn before hashing.
	if got := lastLog(t, `notice(fqdn_rand(30, "seed1", true))`, hostFacts("HOST.EXAMPLE.COM")); got != "25" {
		t.Errorf("fqdn_rand downcase => %q, want 25", got)
	}
	for _, e := range []string{`notice(fqdn_rand())`, `notice(fqdn_rand(0))`, `notice(fqdn_rand("x"))`} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}

func TestSeededRand(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`seeded_rand(1000, "foo")`, "641"},
		{`seeded_rand(100, "bar")`, "35"},
		{`seeded_rand(6, "lucky")`, "1"},
		{`stdlib::seeded_rand(1000, "foo")`, "641"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
	for _, e := range []string{`notice(seeded_rand(1000))`, `notice(seeded_rand(0,"x"))`, `notice(seeded_rand("x","y"))`, `notice(seeded_rand(10, 5))`} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}

func TestSeededRandString(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`seeded_rand_string(8, "foo")`, "bIl9T7Hv"},
		{`seeded_rand_string(16, "password-seed")`, "r98dmOOkoyamBcPq"},
		{`seeded_rand_string(10, "x", "abcd")`, "caadbcbada"},
		{`stdlib::seeded_rand_string(8, "foo")`, "bIl9T7Hv"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
	for _, e := range []string{
		`notice(seeded_rand_string(8))`,
		`notice(seeded_rand_string(0, "s"))`,
		`notice(seeded_rand_string("x", "s"))`,
		`notice(seeded_rand_string(8, 5))`,
		`notice(seeded_rand_string(8, "s", "x"))`,
		`notice(seeded_rand_string(8, "s", 5))`,
	} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}

func TestFqdnRotate(t *testing.T) {
	fqdn := hostFacts("host.example.com")
	cases := []struct{ expr, want string }{
		{`fqdn_rotate(["a","b","c","d","e"])`, `["c", "d", "e", "a", "b"]`},
		{`fqdn_rotate(["a","b","c","d","e"], "seed")`, `["d", "e", "a", "b", "c"]`},
		{`fqdn_rotate("abcdef")`, "cdefab"},
		{`fqdn_rotate([1])`, "[1]"},
		{`fqdn_rotate("a")`, "a"},
		{`fqdn_rotate([])`, "[]"},
	}
	for _, tc := range cases {
		if got := lastLog(t, "notice("+tc.expr+")", fqdn); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
	if evalErr(t, `notice(fqdn_rotate())`) == "" {
		t.Error("fqdn_rotate() expected error")
	}
	if got := evalErr(t, `notice(fqdn_rotate(5))`); !strings.Contains(got, "Array or String") {
		t.Errorf("fqdn_rotate(5) => %q", got)
	}
}

func TestFqdnRandString(t *testing.T) {
	fqdn := hostFacts("host.example.com")
	cases := []struct{ expr, want string }{
		{`fqdn_rand_string(8)`, "fP9p0ZQy"},
		{`fqdn_rand_string(12, "", "seedA")`, "VH7m45sbLyZk"},
		{`fqdn_rand_string(6, "abcdef")`, "fecabe"},
		{`stdlib::fqdn_rand_string(8)`, "fP9p0ZQy"},
	}
	for _, tc := range cases {
		if got := lastLog(t, "notice("+tc.expr+")", fqdn); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
	for _, e := range []string{`notice(fqdn_rand_string())`, `notice(fqdn_rand_string(0))`, `notice(fqdn_rand_string("x"))`} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}

func TestShuffle(t *testing.T) {
	// shuffle is non-deterministic (matches upstream Kernel#rand): assert it
	// returns a permutation of the same multiset.
	logs := logsOf(t, `notice(sort(shuffle([5,4,3,2,1])))`)
	if logs[0] != "[1, 2, 3, 4, 5]" {
		t.Errorf("shuffle permutation => %q", logs[0])
	}
	got := evalOut(t, `shuffle("a")`)
	if got != "a" {
		t.Errorf("shuffle single => %q", got)
	}
	// a shuffled string is a permutation of its characters
	s := evalOut(t, `shuffle("abcdef")`)
	rs := []byte(s)
	sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
	if string(rs) != "abcdef" {
		t.Errorf("shuffle string => %q (sorted %q)", s, rs)
	}
	if evalOut(t, `shuffle([])`) != "[]" {
		t.Error("shuffle([]) should be []")
	}
	for _, e := range []string{`notice(shuffle())`, `notice(shuffle(5))`} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}

// TestMTUnit exercises the MT19937 seeding paths directly, including the
// single-word (init_genrand), zero, and >32-bit-max branches. Golden values
// were generated with MRI Ruby's Random.
func TestMTUnit(t *testing.T) {
	if v := deterministicRandInt(big.NewInt(5), 100); v != 99 {
		t.Errorf("Random.new(5).rand(100) => %d, want 99", v)
	}
	if v := deterministicRandInt(big.NewInt(0), 100); v != 44 {
		t.Errorf("Random.new(0).rand(100) => %d, want 44", v)
	}
	if v := deterministicRandInt(md5Seed("largemax"), 1<<40); v != 214520650833 {
		t.Errorf("rand(2**40) => %d, want 214520650833", v)
	}
	if v := deterministicRandInt(md5Seed("largemax"), 1000000000000); v != 214520650833 {
		t.Errorf("rand(1e12) => %d, want 214520650833", v)
	}
	// negative seed is absolute-valued like MRI.
	if deterministicRandInt(big.NewInt(-5), 100) != 99 {
		t.Error("negative seed should match its absolute value")
	}
	m := newMTFromBig(big.NewInt(1))
	if m.limitedRand(0) != 0 {
		t.Error("limitedRand(0) should be 0")
	}
}

// --- pw_hash --------------------------------------------------------------

func TestPwHash(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`pw_hash('secret', 'md5', 'abc123')`, "$1$abc123$5IJcAgUIzNOMrV9cXyMFd1"},
		{`pw_hash('secret', 'sha-256', 'abc123')`, "$5$abc123$wRJRuAN8mQOchT827qNg5d1eE/hXqPDshKnDFtr5Ny2"},
		{`pw_hash('secret', 'sha-512', 'abc123')`, "$6$abc123$658Dwx.o8ZaF5yCkRyY/MrvTU271tISGpwvd6zNaAjewb8ayeuf4FxTkJ60ipw4l7UGpyAJy40AGqB3EH4P3L0"},
		// case-insensitive hash type name.
		{`pw_hash('secret', 'SHA-512', 'abc123')`, "$6$abc123$658Dwx.o8ZaF5yCkRyY/MrvTU271tISGpwvd6zNaAjewb8ayeuf4FxTkJ60ipw4l7UGpyAJy40AGqB3EH4P3L0"},
		// bcrypt variants: canonical OpenBSD/crypt_blowfish test vectors.
		{`pw_hash('U*U', 'bcrypt-a', '05$CCCCCCCCCCCCCCCCCCCCC.')`, "$2a$05$CCCCCCCCCCCCCCCCCCCCC.E5YPO9kmyuRGyh0XouQYb4YMJKvyOeW"},
		{`pw_hash('U*U*', 'bcrypt-a', '05$CCCCCCCCCCCCCCCCCCCCC.')`, "$2a$05$CCCCCCCCCCCCCCCCCCCCC.VGOzA784oUp/Z0DY336zx7pLYAy0lwK"},
		{`pw_hash('password', 'bcrypt', '10$cgT08pfGUo9SUIIvXrIJ1u')`, "$2b$10$cgT08pfGUo9SUIIvXrIJ1uSXp0VJmOIKgEC6vqGvqRddK4Z3JB28G"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s =>\n  %q\nwant\n  %q", tc.expr, got, tc.want)
		}
	}
	// $2x and $2y prefixes only change the tag, not the digest.
	if got := evalOut(t, `pw_hash('password', 'bcrypt-y', '10$cgT08pfGUo9SUIIvXrIJ1u')`); !strings.HasPrefix(got, "$2y$10$cgT08pfGUo9SUIIvXrIJ1u") {
		t.Errorf("bcrypt-y => %q", got)
	}
	if got := evalOut(t, `pw_hash('password', 'bcrypt-x', '10$cgT08pfGUo9SUIIvXrIJ1u')`); !strings.HasPrefix(got, "$2x$10$") {
		t.Errorf("bcrypt-x => %q", got)
	}
	// empty / undef password hashes to undef (renders as "").
	if got := evalOut(t, `pw_hash('', 'sha-512', 'abc123')`); got != "" {
		t.Errorf("empty password => %q, want empty", got)
	}
	if got := evalOut(t, `pw_hash(undef, 'sha-512', 'abc123')`); got != "" {
		t.Errorf("undef password => %q, want empty", got)
	}
}

func TestPwHashErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(pw_hash('a','sha-512'))`, "wrong number"},
		{`notice(pw_hash(5,'sha-512','salt'))`, "first argument must be a string"},
		{`notice(pw_hash('a', 5, 'salt'))`, "second argument must be a string"},
		{`notice(pw_hash('a','nope','salt'))`, "not a valid hash type"},
		{`notice(pw_hash('a','sha-512', 5))`, "third argument must be a string"},
		{`notice(pw_hash('a','sha-512', ''))`, "must not be empty"},
		{`notice(pw_hash('a','sha-512', 'bad$salt'))`, "set [a-zA-Z0-9./]"},
		{`notice(pw_hash('a','bcrypt', 'nope'))`, "must match"},
	}
	for _, tc := range cases {
		if got := evalErr(t, tc.src); !strings.Contains(got, tc.want) {
			t.Errorf("%s => %q, want substring %q", tc.src, got, tc.want)
		}
	}
}

// --- strftime / time ------------------------------------------------------

func TestStrftimeDirectives(t *testing.T) {
	// 1471954394 = 2016-08-23T12:13:14Z
	if got := evalOut(t, `strftime('%s', 1471954394)`); got != "1471954394" {
		t.Fatalf("epoch => %q", got)
	}
	cases := []struct{ dir, want string }{
		{"%Y", "2016"}, {"%C", "20"}, {"%y", "16"}, {"%m", "08"},
		{"%B", "August"}, {"%b", "Aug"}, {"%h", "Aug"}, {"%d", "23"},
		{"%e", "23"}, {"%j", "236"}, {"%H", "12"}, {"%k", "12"},
		{"%I", "12"}, {"%l", "12"}, {"%M", "13"}, {"%S", "14"},
		{"%L", "000"}, {"%N", "000000000"}, {"%P", "pm"}, {"%p", "PM"},
		{"%z", "+0000"}, {"%Z", "UTC"}, {"%A", "Tuesday"}, {"%a", "Tue"},
		{"%u", "2"}, {"%w", "2"}, {"%G", "2016"}, {"%g", "16"},
		{"%V", "34"}, {"%U", "34"}, {"%W", "34"}, {"%s", "1471954394"},
		{"%n", "\n"}, {"%t", "\t"}, {"%%", "%"}, {"%D", "08/23/16"},
		{"%F", "2016-08-23"}, {"%v", "23-AUG-2016"}, {"%x", "08/23/16"},
		{"%X", "12:13:14"}, {"%r", "12:13:14 PM"}, {"%R", "12:13"},
		{"%T", "12:13:14"}, {"%c", "Tue Aug 23 12:13:14 2016"},
	}
	for _, tc := range cases {
		expr := `strftime('` + tc.dir + `', 1471954394)`
		if got := evalOut(t, expr); got != tc.want {
			t.Errorf("strftime(%q) => %q, want %q", tc.dir, got, tc.want)
		}
	}
}

func TestStrftimeEdge(t *testing.T) {
	// single-digit padding: 2017-01-04T00:04:05Z
	edge := []struct{ dir, want string }{
		{"%e", " 4"}, {"%k", " 0"}, {"%d", "04"}, {"%I", "12"},
		{"%l", "12"}, {"%U", "01"}, {"%W", "01"}, {"%V", "01"}, {"%G", "2017"},
	}
	for _, tc := range edge {
		expr := `strftime('` + tc.dir + `', 1483488245)`
		if got := evalOut(t, expr); got != tc.want {
			t.Errorf("strftime(%q, edge) => %q, want %q", tc.dir, got, tc.want)
		}
	}
	// timezone handling
	if got := evalOut(t, `strftime('%F %T %z', 1471954394, '-0800')`); got != "2016-08-23 04:13:14 -0800" {
		t.Errorf("PST => %q", got)
	}
	if got := evalOut(t, `strftime('%H:%M', 1471954394, '+05:30')`); got != "17:43" {
		t.Errorf("IST => %q", got)
	}
	if got := evalOut(t, `strftime('%Y', 1471954394, 'UTC')`); got != "2016" {
		t.Errorf("UTC LoadLocation => %q", got)
	}
	// legacy form strftime(format, timezone) formats "now"; just ensure it runs.
	if evalOut(t, `strftime('%z', 'UTC')`) != "+0000" {
		t.Error("legacy timezone form failed")
	}
	// flags / widths / unknown / literals via direct call for nanosecond control
	tm := time.Unix(1471954394, 123456789).UTC()
	directs := []struct{ format, want string }{
		{"%L", "123"}, {"%N", "123456789"}, {"%3N", "123"}, {"%6N", "123456"},
		{"%12N", "123456789000"}, {"%-d", "23"}, {"%_d", "23"}, {"%^B", "AUGUST"},
		{"literal %Y!", "literal 2016!"}, {"%q", "%q"}, {"100%", "100%"},
		{"%", "%"}, {"%:z", "+00:00"}, {"%::z", "+00:00:00"}, {"%0e", "23"},
	}
	for _, tc := range directs {
		if got := strftime(tm, tc.format); got != tc.want {
			t.Errorf("strftime(%q) => %q, want %q", tc.format, got, tc.want)
		}
	}
	// zone offset with negative offset colon forms
	pst := time.Unix(1471954394, 0).In(time.FixedZone("x", -28800))
	if got := strftime(pst, "%:z"); got != "-08:00" {
		t.Errorf("neg colon z => %q", got)
	}
	// a format that ends inside a directive after flags/width
	if got := strftime(tm, "%-"); got != "%-" {
		t.Errorf("trailing flags => %q", got)
	}
	for _, e := range []string{`notice(strftime())`, `notice(strftime(5))`, `notice(strftime('%Y', 1, 5))`, `notice(strftime('%Y', "notnum", "z"))`, `notice(strftime('%Y', 1, 'Bad/Zone'))`} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}

func TestTime(t *testing.T) {
	defer func() { nowFunc = time.Now }()
	nowFunc = func() time.Time { return time.Unix(1471954394, 0) }
	if got := evalOut(t, `time()`); got != "1471954394" {
		t.Errorf("time() => %q", got)
	}
	if got := evalOut(t, `stdlib::time("America/New_York")`); got != "1471954394" {
		t.Errorf("time(tz) => %q", got)
	}
	if got := evalOut(t, `strftime('%Y')`); got != "2016" {
		t.Errorf("strftime now => %q", got)
	}
	if evalErr(t, `notice(time("a","b"))`) == "" {
		t.Error("time(a,b) expected error")
	}
}

// --- base64 / convert_base / dos2unix / scanf -----------------------------

func TestBase64(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`base64("encode", "hello", "strict")`, "aGVsbG8="},
		{`base64("encode", "hello")`, "aGVsbG8=\n"},
		{`base64("decode", "aGVsbG8=", "strict")`, "hello"},
		{`base64("decode", "aGVsbG8=\n")`, "hello"},
		{`base64("encode", "hello", "urlsafe")`, "aGVsbG8="},
		{`base64("decode", "aGVsbG8=", "urlsafe")`, "hello"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
	// long input wraps at 60 chars for the default method.
	long := evalOut(t, `base64("encode", "The quick brown fox jumps over the lazy dog, and then some more text to exceed sixty chars")`)
	if !strings.Contains(long, "\n") || !strings.HasSuffix(long, "\n") {
		t.Errorf("default encode should wrap+trail newline: %q", long)
	}
	for _, e := range []string{
		`notice(base64("hello"))`,
		`notice(base64("nope", "x"))`,
		`notice(base64("encode", "x", "nope"))`,
		`notice(base64(5, "x"))`,
		`notice(base64("encode", 5))`,
		`notice(base64("encode", "x", 5))`,
		`notice(base64("decode", "!!!!", "strict"))`,
		`notice(base64("decode", "!!!!", "urlsafe"))`,
	} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}

func TestConvertBase(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`convert_base(11, 16)`, "b"},
		{`convert_base(255, 16)`, "ff"},
		{`convert_base(100, 2)`, "1100100"},
		{`convert_base("255", "16")`, "ff"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
	for _, e := range []string{
		`notice(convert_base(1))`,
		`notice(convert_base("xx", 16))`,
		`notice(convert_base(255, 40))`,
		`notice(convert_base(255, "zz"))`,
		`notice(convert_base(255, [1]))`,
		`notice(convert_base([1], 16))`,
	} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}

func TestDos2UnixUnix2Dos(t *testing.T) {
	if got := lastLog(t, "notice(dos2unix(\"a\r\nb\r\n\"))"); got != "a\nb\n" {
		t.Errorf("dos2unix => %q", got)
	}
	if got := lastLog(t, "notice(unix2dos(\"a\nb\n\"))"); got != "a\r\nb\r\n" {
		t.Errorf("unix2dos => %q", got)
	}
	// unix2dos must not double existing CRLF, and passes other bytes through.
	if got := lastLog(t, "notice(unix2dos(\"x\r\ny\"))"); got != "x\r\ny" {
		t.Errorf("unix2dos crlf => %q", got)
	}
	for _, e := range []string{`notice(dos2unix())`, `notice(dos2unix(5))`, `notice(unix2dos(5))`} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}

func TestScanf(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`scanf("42 3.14 foo", "%d %f %s")`, `[42, 3.14, "foo"]`},
		{`scanf("12,34", "%d,%d")`, "[12, 34]"},
		{`scanf("0xff", "%x")`, "[255]"},
		{`scanf("abc123", "%3s%d")`, `["abc", 123]`},
		{`scanf("hello", "%d")`, "[]"},
		{`scanf("777", "%o")`, "[511]"},
		{`scanf("-5", "%i")`, "[-5]"},
		{`scanf("0x1F", "%i")`, "[31]"},
		{`scanf("100%", "%d%%")`, "[100]"},
		{`scanf("ab", "%c%c")`, `["a", "b"]`},
		{`scanf("1.5e3", "%g")`, "[1500]"},
		{`scanf("x=5", "x=%d")`, "[5]"},
		{`scanf("nope", "x=%d")`, "[]"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
	// block form receives the result array.
	if got := firstLog(t, `notice(scanf("1 2", "%d %d") |$a| { $a[0] + $a[1] })`); got != "3" {
		t.Errorf("scanf block => %q", got)
	}
	for _, e := range []string{`notice(scanf("x"))`, `notice(scanf(5, "%d"))`, `notice(scanf("x", 5))`} {
		if evalErr(t, e) == "" {
			t.Errorf("%s expected error", e)
		}
	}
}
