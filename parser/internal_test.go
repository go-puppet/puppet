// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parser

import (
	"strings"
	"testing"

	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/lexer"
)

// TestCursorAndTokenGuards exercises defensive branches that a well-formed
// token stream never reaches through the public API: out-of-range lookahead
// and a malformed literal token (the lexer never emits one, so we hand-craft
// it here).
func TestCursorAndTokenGuards(t *testing.T) {
	p := &parser{toks: []lexer.Token{{Kind: lexer.NAME, Text: "x"}, {Kind: lexer.EOF}}}
	if p.at(&cursor{i: 0}, 5).Kind != lexer.EOF {
		t.Error("at() past end should return EOF token")
	}
	if isBareVarStart([]rune("x"), 5) {
		t.Error("isBareVarStart past end should be false")
	}
	// A malformed FLOAT token cannot come from the lexer; craft one so the
	// strconv.ParseFloat error arm in parsePrimary is exercised.
	pf := &parser{toks: []lexer.Token{{Kind: lexer.FLOAT, Text: "1.2.3"}, {Kind: lexer.EOF}}}
	if _, err := pf.parsePrimary(&cursor{}); err == nil {
		t.Error("expected error for malformed float token")
	}
}

// interp is a direct wrapper over the unexported interpolate helper so the
// escape/interpolation paths can be exercised precisely (some are unreachable
// via the double-quote lexer, e.g. a raw trailing backslash from a heredoc).
func interp(t *testing.T, raw string) string {
	t.Helper()
	n, err := interpolate(raw, ast.Position{Line: 1, Column: 1})
	if err != nil {
		t.Fatalf("interpolate(%q) error: %v", raw, err)
	}
	return ast.Sexpr(n)
}

func TestInterpolateEscapes(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`plain`, `"plain"`},
		{``, `""`},
		{`a\nb\tc\rd\se`, `"a\nb\tc\rd e"`},
		{`q\"s\'d\\b\$z`, `"q\"s'd\\b$z"`},
		{`uAv`, `"uAv"`},
		{`u\u{1F600}v`, "\"u\U0001F600v\""},
		{"k\\u0041m", `"kAm"`},
		{`bad\uZZZZ`, `"bad\\uZZZZ"`},
		{`bad\u{ZZ}`, `"bad\\u{ZZ}"`},
		{`short\u12`, `"short\\u12"`},
		{`open\u{41`, `"open\\u{41"`},
		{`unknown\qesc`, `"unknown\\qesc"`},
		{`trail\`, `"trail\\"`},
		{`a$`, `"a$"`},
		{`a$ b`, `"a$ b"`},
		{`$x`, `(concat $x)`},
		{`${x}`, `(concat $x)`},
		{`${x[0]}`, `(concat (access $x 0))`},
		{`${x.up}`, `(concat (. $x up))`},
		{`${f(1)}`, `(concat (call f 1))`},
		{`${$y + 1}`, `(concat (+ $y 1))`},
		{`${ (1) }`, `(concat 1)`},
		{`${'has}brace' + x}`, `(concat (+ "has}brace" x))`},
		{`${"p}q"}`, `"p}q"`},
		{`pre${x}post`, `(concat "pre" $x "post")`},
		{`$a::b`, `(concat $a::b)`},
		{`$::h`, `(concat $::h)`},
		{`qAz`, `"qAz"`},
		{`${a::b}`, `(concat $a::b)`},
		{`${"a\nb"}`, `"a\nb"`},
		{`${foo (1)}`, `(concat (call foo 1))`},
	}
	for _, tc := range cases {
		if got := interp(t, tc.raw); got != tc.want {
			t.Errorf("interpolate(%q)\n  got  %s\n  want %s", tc.raw, got, tc.want)
		}
	}
}

func TestInterpolateErrors(t *testing.T) {
	cases := []struct{ raw, contains string }{
		{`${}`, "empty ${}"},
		{`${   }`, "empty ${}"},
		{`${x`, "unterminated ${...}"},
		{`${1 +}`, "unexpected"},
	}
	for _, tc := range cases {
		_, err := interpolate(tc.raw, ast.Position{Line: 1, Column: 1})
		if err == nil {
			t.Errorf("interpolate(%q): expected error, got nil", tc.raw)
			continue
		}
		if !strings.Contains(err.Error(), tc.contains) {
			t.Errorf("interpolate(%q): %q lacks %q", tc.raw, err.Error(), tc.contains)
		}
	}
}
