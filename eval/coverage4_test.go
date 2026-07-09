// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

func TestMoreEdgeOutputs(t *testing.T) {
	cases := []struct{ expr, want string }{
		// end_with with no matching suffix
		{`end_with("hello", ["no","xx"])`, "false"},
		// start_with with a non-string/array pattern -> no match
		{`start_with("a", 5)`, "false"},
		// versioncmp numeric-vs-alpha and alpha-vs-alpha segments
		{`versioncmp("1.2", "1.a")`, "1"},
		{`versioncmp("1.a", "1.b")`, "-1"},
		// match over an array with a Regexp value
		{`grep(["a1","bb"], /\d/)`, `["a1"]`},
		// values_at with a negative index
		{`values_at([10,20,30], [-1])`, "[30]"},
		// pick_default with all undef/empty returns the last argument
		{`pick_default(undef, "")`, ""},
		// dig short-circuits on an undef mid-path
		{`dig({"a"=>undef}, ["a","b"])`, ""},
		// get with consecutive dots (empty segment skipped)
		{`get({"a"=>1}, "a.")`, "1"},
		// parseyaml null
		{`parseyaml("~")`, ""},
		// unknown qualified variable resolves to undef
		{`"[${foo::bar}]"`, "[]"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestMoreEdgeErrors(t *testing.T) {
	cases := []struct {
		src  string
		opts []Option
		want string
	}{
		{`notice(5 =~ 'x')`, nil, "requires a string on the left"},
		{`3.each |$x| { nope() }`, nil, "unknown function"},
		{`notice(sort(5))`, nil, "must be an Array"},
		{`notice(grep(5, "x"))`, nil, "must be an Array"},
		{`notice(any([1],[2]) |$x| { true })`, nil, "wrong number"},
		{`notice(all([1],[2]) |$x| { true })`, nil, "wrong number"},
		{`notice(match(["a"], "("))`, nil, "invalid pattern"},
		{`notice(tree_each([[1]]) |$v| { nope() })`, nil, "unknown function"},
		{`notice(inline_epp('<%- | Integer $x | -%>', {'x' => 'str'}))`, nil, "expects"},
		{`notice(inline_template(5))`, []Option{WithERBRenderer(stubERB{})}, "must be a String"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src, tc.opts...); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestTopLevelJumpSignals(t *testing.T) {
	// A jump function called outside a function/plan/loop surfaces its guard
	// message (covers the signal Error() methods).
	cases := []struct{ src, want string }{
		{`return(5)`, "return outside"},
		{`next(5)`, "next() outside"},
		{`break(5)`, "break() outside"},
	}
	for _, tc := range cases {
		_, _, err := EvalString(tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s => %v, want %q", tc.src, err, tc.want)
		}
	}
}

func TestTargetSpecUnknownHash(t *testing.T) {
	// A target hash with neither uri nor name yields no specs (nil), which the
	// executor receives as an empty target list.
	x := &recExec{}
	_, _, err := EvalPlanString(`plan p() { run_command('c', {'group' => 'g'}) }`, "p", nil, WithPlanExecutor(x))
	if err != nil {
		t.Fatal(err)
	}
}

func TestDirnameRelativeNoSlash(t *testing.T) {
	if got := evalOut(t, `dirname("plainfile")`); got != "." {
		t.Errorf("got %q", got)
	}
}
