// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

// TestDeprecation checks that deprecation logs a warning at most once per key.
func TestDeprecation(t *testing.T) {
	_, logs, err := EvalString(`deprecation("k1", "first"); deprecation("k1", "again"); deprecation("k2", "second", false)`)
	if err != nil {
		t.Fatal(err)
	}
	var warnings []string
	for _, l := range logs {
		if l.Level == "warning" {
			warnings = append(warnings, l.Message)
		}
	}
	if len(warnings) != 2 || warnings[0] != "first" || warnings[1] != "second" {
		t.Fatalf("warnings = %v, want [first second]", warnings)
	}
}

// TestDeprecationErrors covers deprecation's argument-validation branches.
func TestDeprecationErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`deprecation("k")`, "wrong number of arguments"},
		{`deprecation(1, "m")`, "must be a String"},
		{`deprecation("k", 2)`, "must be a String"},
		{`deprecation("k", "m", "notbool")`, "must be a Boolean"},
	}
	for _, c := range cases {
		if e := evalErr(t, c.src); !strings.Contains(e, c.want) {
			t.Errorf("%s = %q, want %q", c.src, e, c.want)
		}
	}
}

// TestDefinedWithParams checks resource-attribute matching, including the
// undef-matches-absent rule.
func TestDefinedWithParams(t *testing.T) {
	cases := []struct{ src, want string }{
		// Matching params.
		{`package { 'a': ensure => 'present' } notice(defined_with_params(Package['a'], {'ensure' => 'present'}))`, "true"},
		// Empty params: just existence.
		{`package { 'a': ensure => 'present' } notice(defined_with_params(Package['a']))`, "true"},
		// Mismatched value.
		{`package { 'a': ensure => 'present' } notice(defined_with_params(Package['a'], {'ensure' => 'absent'}))`, "false"},
		// Not in catalog.
		{`notice(defined_with_params(Package['nope'], {'ensure' => 'present'}))`, "false"},
		// undef expectation matches an absent attribute.
		{`package { 'a': ensure => 'present' } notice(defined_with_params(Package['a'], {'tag' => undef}))`, "true"},
		// String reference form.
		{`package { 'a': ensure => 'present' } notice(defined_with_params("Package[a]", {'ensure' => 'present'}))`, "true"},
	}
	for _, c := range cases {
		if got := firstLog(t, c.src); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestDefinedWithParamsErrors covers the argument-validation branches.
func TestDefinedWithParamsErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(defined_with_params())`, "wrong number of arguments"},
		{`notice(defined_with_params(42))`, "must be a resource reference"},
		{`notice(defined_with_params("bareword"))`, "must be a resource reference"},
		{`notice(defined_with_params(Package['a'], "notahash"))`, "must be a Hash"},
	}
	for _, c := range cases {
		if e := evalErr(t, c.src); !strings.Contains(e, c.want) {
			t.Errorf("%s = %q, want %q", c.src, e, c.want)
		}
	}
}

// TestEnsureResources checks idempotent bulk declaration with per-resource
// attribute overrides.
func TestEnsureResources(t *testing.T) {
	src := `ensure_resources('user', {'dan' => {'uid' => '600'}, 'alex' => {}}, {'ensure' => 'present'})
notice("${defined_with_params(User['dan'], {'ensure' => 'present', 'uid' => '600'})}-${defined_with_params(User['alex'], {'ensure' => 'present'})}")`
	if got := firstLog(t, src); got != "true-true" {
		t.Fatalf("ensure_resources = %q, want true-true", got)
	}
}

// TestEnsureResourcesErrors covers the argument-validation branches.
func TestEnsureResourcesErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`ensure_resources('user')`, "wrong number of arguments"},
		{`ensure_resources(1, {})`, "must be a String"},
		{`ensure_resources('user', "notahash")`, "Requires second argument to be a Hash"},
		{`ensure_resources('user', {'a' => {}}, "notahash")`, "must be a Hash"},
	}
	for _, c := range cases {
		if e := evalErr(t, c.src); !strings.Contains(e, c.want) {
			t.Errorf("%s = %q, want %q", c.src, e, c.want)
		}
	}
}

// TestEnsurePackages checks the String, Array and Hash forms and the
// default-ensure rule.
func TestEnsurePackages(t *testing.T) {
	cases := []struct{ src, want string }{
		// Array form: ensure defaults to "installed".
		{`stdlib::ensure_packages(['vim']) notice(getparam(Package['vim'], 'ensure'))`, "installed"},
		// String form with explicit attributes.
		{`stdlib::ensure_packages('curl', {'ensure' => 'latest'}) notice(getparam(Package['curl'], 'ensure'))`, "latest"},
		// Hash form merges default + per-package attributes.
		{`stdlib::ensure_packages({'git' => {'provider' => 'apt'}}, {'ensure' => 'latest'}) notice(getparam(Package['git'], 'ensure'))`, "latest"},
		// default_ensure: a package already declared present keeps present.
		{`package { 'ssh': ensure => 'present' } stdlib::ensure_packages(['ssh']) notice(getparam(Package['ssh'], 'ensure'))`, "present"},
		// Unnamespaced alias.
		{`ensure_packages(['nginx']) notice(getparam(Package['nginx'], 'ensure'))`, "installed"},
	}
	for _, c := range cases {
		if got := firstLog(t, c.src); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestEnsurePackagesErrors covers the argument-validation branches.
func TestEnsurePackagesErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`stdlib::ensure_packages()`, "wrong number of arguments"},
		{`stdlib::ensure_packages(['a'], "notahash")`, "must be a Hash"},
		{`stdlib::ensure_packages([42])`, "package names must be Strings"},
		{`stdlib::ensure_packages(42)`, "expects a String, Array, or Hash"},
	}
	for _, c := range cases {
		if e := evalErr(t, c.src); !strings.Contains(e, c.want) {
			t.Errorf("%s = %q, want %q", c.src, e, c.want)
		}
	}
}

// TestBulkDeclareErrorPropagation covers the error-return branches of the bulk
// declaration functions by routing each through a defined type whose body fails.
func TestBulkDeclareErrorPropagation(t *testing.T) {
	cases := []struct{ src, want string }{
		// ensure_resources propagates a declaration error.
		{`define widget() { fail("boom") } ensure_resources('widget', {'a' => {}})`, "boom"},
		// ensure_packages, each input form, via a failing user-defined `package`.
		{`define package(String $ensure) { fail("boom") } stdlib::ensure_packages(['x'])`, "boom"},
		{`define package(String $ensure) { fail("boom") } stdlib::ensure_packages('x')`, "boom"},
		{`define package(String $ensure) { fail("boom") } stdlib::ensure_packages({'x' => {}})`, "boom"},
	}
	for _, c := range cases {
		if e := evalErr(t, c.src); !strings.Contains(e, c.want) {
			t.Errorf("%s = %q, want %q", c.src, e, c.want)
		}
	}
	// ensure_resources with an undef per-resource value uses shared params only
	// (merged is nil), exercising orEmptyHash's nil path.
	if _, _, err := EvalString(`define ok() {} ensure_resources('ok', {'a' => undef})`); err != nil {
		t.Fatalf("ensure_resources undef per-value errored: %v", err)
	}
}

// TestOSVersionGTE checks the fact-driven version comparison.
func TestOSVersionGTE(t *testing.T) {
	facts := WithFacts(MapFacts{
		"os": map[string]any{
			"name":    "Debian",
			"release": map[string]any{"major": "12"},
		},
	})
	cases := []struct{ expr, want string }{
		{`stdlib::os_version_gte("Debian", "11")`, "true"},
		{`stdlib::os_version_gte("Debian", "12")`, "true"},
		{`stdlib::os_version_gte("Debian", "13")`, "false"},
		{`stdlib::os_version_gte("Ubuntu", "1")`, "false"},
		{`os_version_gte("Debian", "11")`, "true"},
	}
	for _, c := range cases {
		if got := evalOut(t, c.expr, facts); got != c.want {
			t.Errorf("%s = %q, want %q", c.expr, got, c.want)
		}
	}
}

// TestOSVersionGTEErrors covers arity, type, and missing-fact branches.
func TestOSVersionGTEErrors(t *testing.T) {
	facts := WithFacts(MapFacts{"os": map[string]any{"name": "Debian"}})
	cases := []struct {
		src  string
		want string
		opts []Option
	}{
		{`notice(stdlib::os_version_gte("Debian"))`, "wrong number of arguments", nil},
		{`notice(stdlib::os_version_gte(1, "2"))`, "must be a String", nil},
		{`notice(stdlib::os_version_gte("Debian", 2))`, "must be a String", nil},
		// Facts absent entirely.
		{`notice(stdlib::os_version_gte("Debian", "1"))`, "facts are required", nil},
		// os.release.major missing.
		{`notice(stdlib::os_version_gte("Debian", "1"))`, "facts are required", []Option{facts}},
	}
	for _, c := range cases {
		if e := evalErr(t, c.src, c.opts...); !strings.Contains(e, c.want) {
			t.Errorf("%s = %q, want %q", c.src, e, c.want)
		}
	}
}
