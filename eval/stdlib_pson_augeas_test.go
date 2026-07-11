// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

// TestParsePSON checks that parsepson reproduces JSON.parse/PSON.load semantics
// for the value model.
func TestParsePSON(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`parsepson('{"a":"1","b":"2"}')`, `{"a" => "1", "b" => "2"}`},
		{`parsepson('[1, 2, 3]')`, "[1, 2, 3]"},
		{`parsepson('{"n": 42, "f": 1.5, "t": true, "z": null}')`, `{"f" => 1.5, "n" => 42, "t" => true, "z" => }`},
		{`parsepson('"just a string"')`, "just a string"},
		{`parsepson('not json', "fallback")`, "fallback"},
	}
	for _, c := range cases {
		if got := evalOut(t, c.expr); got != c.want {
			t.Errorf("%s = %q, want %q", c.expr, got, c.want)
		}
	}
}

// TestParsePSONErrors covers the argument and parse-failure branches.
func TestParsePSONErrors(t *testing.T) {
	if e := evalErr(t, `notice(parsepson())`); !strings.Contains(e, "wrong number of arguments") {
		t.Errorf("arity: %q", e)
	}
	if e := evalErr(t, `notice(parsepson([1]))`); !strings.Contains(e, "must be a String") {
		t.Errorf("type: %q", e)
	}
	if e := evalErr(t, `notice(parsepson("not json"))`); !strings.Contains(e, "parsepson()") {
		t.Errorf("parse error: %q", e)
	}
}

// TestValidateAugeas exercises validate_augeas against the pure-Go go-augeas
// lens engine, matching puppetlabs-stdlib semantics.
func TestValidateAugeas(t *testing.T) {
	// Valid content parses cleanly and returns undef (no log, no error).
	if _, _, err := EvalString(`validate_augeas("127.0.0.1 localhost\n", "Hosts.lns")`); err != nil {
		t.Fatalf("valid Hosts content errored: %v", err)
	}
	// Undef third argument is accepted and skips the test-path phase.
	if _, _, err := EvalString(`validate_augeas("127.0.0.1 localhost\n", "Hosts.lns", undef)`); err != nil {
		t.Fatalf("undef tests arg errored: %v", err)
	}
	// Test paths that do not match pass.
	if _, _, err := EvalString(`validate_augeas("127.0.0.1 localhost\n", "Hosts.lns", ['$file/9999'])`); err != nil {
		t.Fatalf("non-matching test path errored: %v", err)
	}
}

// TestValidateAugeasFailures covers every failure branch.
func TestValidateAugeasFailures(t *testing.T) {
	cases := []struct{ src, want string }{
		{`validate_augeas("x")`, "wrong number of arguments"},
		{`validate_augeas("a", "b", "c", "d", "e")`, "wrong number of arguments"},
		{`validate_augeas([1], "Hosts.lns")`, "must be a String"},
		{`validate_augeas("x", [1])`, "must be a String"},
		{`validate_augeas("x", "Hosts.lns", "notarray")`, "must be an Array"},
		{`validate_augeas("x", "Hosts.lns", [], 5)`, "must be a String"},
		{`validate_augeas("x", "NoDotLensName")`, "invalid lens name"},
		{`validate_augeas("x", "Nonexistent.lns")`, "unknown lens"},
		// Malformed content the Passwd lens cannot parse.
		{`validate_augeas("this is totally broken passwd\n", "Passwd.lns")`, "Failed to validate content"},
		// Custom error message (fourth argument).
		{`validate_augeas("this is totally broken passwd\n", "Passwd.lns", [], "custom failure")`, "custom failure"},
		// A test path that DOES match a parsed entry triggers a failure.
		{`validate_augeas("127.0.0.1 localhost\n", "Hosts.lns", ['$file/1'])`, "testing path $file/1"},
		// A non-string test path element.
		{`validate_augeas("127.0.0.1 localhost\n", "Hosts.lns", [42])`, "each test path must be a String"},
	}
	for _, c := range cases {
		e := evalErr(t, c.src)
		if !strings.Contains(e, c.want) {
			t.Errorf("%s\n error=%q\n want substring %q", c.src, e, c.want)
		}
	}
}
