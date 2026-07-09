// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parser

import (
	"strings"
	"testing"
)

func TestParsePlans(t *testing.T) {
	cases := []struct{ src, want string }{
		{`plan p() { }`, `(program [(plan p [])])`},
		{`plan mod::deploy(String $x, Integer $n = 3) { notice($x) }`,
			`(program [(plan mod::deploy (params (String $x) (Integer $n = 3)) [(call notice $x)])])`},
		// apply with a pipeless block
		{`plan p() { apply($t) { include base } }`,
			`(program [(plan p [(call apply $t (lambda [(call include base)]))])])`},
		// numeric interpolation is a match variable
		{`$s = "${0}${1}"`, `(program [(= $s (concat $0 $1))])`},
	}
	for _, tc := range cases {
		if got := ok(t, tc.src); got != tc.want {
			t.Errorf("Parse(%q)\n  got  %s\n  want %s", tc.src, got, tc.want)
		}
	}
}

func TestParsePlanErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`plan`, "expected NAME"},
		{`plan p(`, "expected"},
		{`plan p()`, "expected {"},
		{`plan p() { apply($t) { `, "expected"},
	}
	for _, tc := range cases {
		_, err := Parse(tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Parse(%q) => %v, want %q", tc.src, err, tc.want)
		}
	}
}

func TestParseParametersFn(t *testing.T) {
	ps, err := ParseParameters(`String $x, Integer $n = 3, *$rest`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 3 || ps[0].Name != "x" || ps[1].Name != "n" || !ps[2].CapturesRest {
		t.Errorf("params: %+v", ps)
	}
	if _, err := ParseParameters(`$a $b`); err == nil {
		t.Error("expected error for malformed params")
	}
	if _, err := ParseParameters(`$a = `); err == nil {
		t.Error("expected error for incomplete default")
	}
	if ps, err := ParseParameters(``); err != nil || ps != nil {
		t.Errorf("empty params: %v %v", ps, err)
	}
	// a lexical error in the parameter source is surfaced
	if _, err := ParseParameters(`$a = '`); err == nil {
		t.Error("expected lex error for unterminated string")
	}
}

func TestAllDigitsInterp(t *testing.T) {
	// ${12} is a match variable; ${a1} is the variable $a1.
	if got := ok(t, `$s = "${12}"`); got != `(program [(= $s (concat $12))])` {
		t.Errorf("got %s", got)
	}
	if got := ok(t, `$s = "${a1}"`); got != `(program [(= $s (concat $a1))])` {
		t.Errorf("got %s", got)
	}
}
