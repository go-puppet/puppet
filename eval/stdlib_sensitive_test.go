// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

// TestSensitiveType checks the Sensitive wrapper: it redacts in rendered form,
// unwrap yields the clear value, and a lambda receives the unwrapped value.
func TestSensitiveType(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`Sensitive("secret")`, "Sensitive [value redacted]"},
		{`unwrap(Sensitive("secret"))`, "secret"},
		{`unwrap(Sensitive(42))`, "42"},
		{`unwrap("plain")`, "plain"},
		{`unwrap(Sensitive("secret")) |$v| { "[${v}]" }`, "[secret]"},
	}
	for _, c := range cases {
		if got := evalOut(t, c.expr); got != c.want {
			t.Errorf("%s = %q, want %q", c.expr, got, c.want)
		}
	}
}

// TestUnwrapErrors covers unwrap's arity check.
func TestUnwrapErrors(t *testing.T) {
	if e := evalErr(t, `notice(unwrap())`); !strings.Contains(e, "wrong number of arguments") {
		t.Errorf("arity: %q", e)
	}
}

// TestRewrapSensitiveData covers stdlib::rewrap_sensitive_data: it re-wraps only
// when a Sensitive was present, unwraps nested structures, and threads a block.
func TestRewrapSensitiveData(t *testing.T) {
	// No sensitive present: plain data returned unchanged.
	if got := evalOut(t, `stdlib::rewrap_sensitive_data({"a" => 1, "b" => [2, 3]})`); got != `{"a" => 1, "b" => [2, 3]}` {
		t.Errorf("plain rewrap = %q", got)
	}
	// Sensitive nested in a hash/array: result is Sensitive, unwrap reveals the
	// deeply-unwrapped structure.
	if got := evalOut(t, `stdlib::rewrap_sensitive_data({"a" => Sensitive("x"), "b" => [Sensitive("y")]})`); got != "Sensitive [value redacted]" {
		t.Errorf("sensitive rewrap not redacted: %q", got)
	}
	if got := evalOut(t, `unwrap(stdlib::rewrap_sensitive_data({"a" => Sensitive("x"), "b" => [Sensitive("y")]}))`); got != `{"a" => "x", "b" => ["y"]}` {
		t.Errorf("unwrapped rewrap = %q", got)
	}
	// A Sensitive-of-Sensitive is fully unwrapped.
	if got := evalOut(t, `unwrap(stdlib::rewrap_sensitive_data(Sensitive(Sensitive("z"))))`); got != "z" {
		t.Errorf("nested sensitive rewrap = %q", got)
	}
	// Block form: the block runs on unwrapped data before re-wrapping.
	if got := evalOut(t, `unwrap(stdlib::rewrap_sensitive_data(Sensitive("s")) |$v| { upcase($v) })`); got != "S" {
		t.Errorf("block rewrap = %q", got)
	}
	// Scalar without sensitive passes through.
	if got := evalOut(t, `stdlib::rewrap_sensitive_data("plain")`); got != "plain" {
		t.Errorf("scalar rewrap = %q", got)
	}
	// A failing block propagates its error.
	if e := evalErr(t, `notice(stdlib::rewrap_sensitive_data(Sensitive("x")) |$v| { fail("boom") })`); !strings.Contains(e, "boom") {
		t.Errorf("block error not propagated: %q", e)
	}
}

// TestRewrapSensitiveErrors covers the arity branches.
func TestRewrapSensitiveErrors(t *testing.T) {
	if e := evalErr(t, `notice(stdlib::rewrap_sensitive_data())`); !strings.Contains(e, "wrong number of arguments") {
		t.Errorf("arity: %q", e)
	}
}
