// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"

	"github.com/go-puppet/puppet/ast"
)

func TestFinalReachable(t *testing.T) {
	// EPP: a `<%-` trims preceding inline whitespace on the same text run.
	if got := firstLog(t, `notice(inline_epp("a   <%- -%>b"))`); got != "ab" {
		t.Errorf("epp left-trim spaces got %q", got)
	}
	// EPP: a `%%>` inside a tag body is not treated as the tag close.
	if got := firstLog(t, `notice(inline_epp('<%= "x%%>y" %>'))`); !strings.Contains(got, "x") {
		t.Errorf("epp %%%%> escape got %q", got)
	}
	// A numbered match variable beyond the captured groups is undef.
	if got := firstLog(t, `if 'a' =~ /(a)/ { notice("[${9}]") }`); got != "[]" {
		t.Errorf("out-of-range match var got %q", got)
	}
	// Collector skips resources of other types.
	cat := catOf(t, `
@file { '/f': }
@user { 'u': }
File <| |>`)
	f, _ := cat.Get("File[/f]")
	u, _ := cat.Get("User[u]")
	if f.Virtual || !u.Virtual {
		t.Errorf("collector type filter: file.virtual=%v user.virtual=%v", f.Virtual, u.Virtual)
	}
}

func TestFinalReachableErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		// EPP default parameter whose expression errors.
		{`notice(inline_epp('<%- | $x = nope() | -%><%= $x %>'))`, "unknown function"},
		// Collector query whose right-hand side errors.
		{`@file { '/f': }
File <| tag == nope() |>`, "unknown function"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestEmptyVariableName(t *testing.T) {
	// A variable node with an empty name (unreachable through the parser) must
	// resolve to undef rather than being mis-treated as a numeric match var.
	e := New()
	v, err := e.eval(&ast.Variable{Name: ""}, e.top)
	if err != nil {
		t.Fatal(err)
	}
	if !isUndef(v) {
		t.Errorf("empty var name should be undef, got %v", v)
	}
}

func TestParseJSONSingleArgError(t *testing.T) {
	if err := evalErr(t, `notice(parsejson("["))`); !strings.Contains(err, "parsejson") {
		t.Errorf("parsejson 1-arg error got %q", err)
	}
	if err := evalErr(t, "notice(parseyaml(\"\\t- x\"))"); !strings.Contains(err, "parseyaml") {
		t.Errorf("parseyaml 1-arg error got %q", err)
	}
}

func TestImportExportedQueryError(t *testing.T) {
	store := NewMemoryExportedStore()
	catOf(t, `@@host { 'h': tag => 'x' }`, WithNodeName("a"), WithExportedStore(store))
	// A query error while importing exported resources propagates.
	err := evalErr(t, `Host <<| tag == nope() |>>`, WithNodeName("b"), WithExportedStore(store))
	if !strings.Contains(err, "unknown function") {
		t.Errorf("import query error got %q", err)
	}
}
