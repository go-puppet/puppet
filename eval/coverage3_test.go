// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"

	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/catalog"
)

func TestEppEdgeCases(t *testing.T) {
	// default parameter used
	if got := firstLog(t, `notice(inline_epp('<%- | $x = 5 | -%><%= $x %>'))`); got != "5" {
		t.Errorf("epp default got %q", got)
	}
	// empty <%= %> is a no-op
	if got := firstLog(t, `notice(inline_epp('a<%= %>b'))`); got != "ab" {
		t.Errorf("epp empty expr got %q", got)
	}
	// backslash and single-quote in literal text survive quoting
	if got := firstLog(t, `notice(inline_epp('a\\b<%= 1 %>'))`); got != `a\b1` {
		t.Errorf("epp backslash got %q", got)
	}
	if got := firstLog(t, `notice(inline_epp('it\'s <%= 1 %>'))`); got != "it's 1" {
		t.Errorf("epp quote got %q", got)
	}
	// -%> followed by CRLF trims the whole line ending
	if got := firstLog(t, `notice(inline_epp("x<% if true { -%>\r\ny<% } -%>\r\n"))`); got != "xy" {
		t.Errorf("epp crlf got %q", got)
	}
}

func TestEppErrorsExtra(t *testing.T) {
	cases := []struct{ src, want string }{
		// compiled program has a Puppet syntax error
		{`notice(inline_epp('<% $x = %>'))`, "parse error"},
		// evaluation error inside a code tag
		{`notice(inline_epp('<% nope() %>'))`, "unknown function"},
		// evaluation error inside an expression tag
		{`notice(inline_epp('<%= nope() %>'))`, "unknown function"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestOverrideForwardMetaparam(t *testing.T) {
	cat := catOf(t, `
package { 'p': }
file { '/o': }
File['/o'] { before => Package['p'] }`)
	found := false
	for _, e := range cat.Edges() {
		if e.Source == "File[/o]" && e.Target == "Package[p]" {
			found = true
		}
	}
	if !found {
		t.Errorf("forward metaparam edge missing: %v", cat.Edges())
	}
}

func TestOverrideArrayAppend(t *testing.T) {
	cat := catOf(t, `
file { '/o': paths => ['a'] }
File['/o'] { paths +> ['b','c'] }`)
	o, _ := cat.Get("File[/o]")
	arr, _ := o.Parameters["paths"].([]any)
	if len(arr) != 3 {
		t.Errorf("array append: %v", o.Parameters["paths"])
	}
}

func TestOverrideArrayAppendScalar(t *testing.T) {
	cat := catOf(t, `
file { '/o': paths => ['a'] }
File['/o'] { paths +> 'b' }`)
	o, _ := cat.Get("File[/o]")
	arr, _ := o.Parameters["paths"].([]any)
	if len(arr) != 2 {
		t.Errorf("array append scalar: %v", o.Parameters["paths"])
	}
}

func TestOverrideEvalErrors(t *testing.T) {
	// target expression errors
	if err := evalErr(t, `File[nope()] { mode => '0644' }`); !strings.Contains(err, "unknown function") {
		t.Errorf("override target err: %q", err)
	}
	// attribute value errors
	if err := evalErr(t, `file { '/o': }
File['/o'] { mode => nope() }`); !strings.Contains(err, "unknown function") {
		t.Errorf("override attr err: %q", err)
	}
}

func TestCollectorMissingParamQuery(t *testing.T) {
	// A query on an attribute that a resource lacks -> not realized.
	cat := catOf(t, `
@file { '/v': tag => 'x' }
File <| mode == '0600' |>`)
	v, _ := cat.Get("File[/v]")
	if !v.Virtual {
		t.Errorf("should not realize on missing param: %v", v)
	}
}

func TestCollectorNonRefType(t *testing.T) {
	// Hand-built AST: a collector whose type is not a QualifiedReference.
	e := New()
	c := &ast.Collector{Type: &ast.QualifiedName{Value: "file"}}
	if _, err := e.eval(c, e.top); err == nil || !strings.Contains(err.Error(), "requires a type reference") {
		t.Errorf("got %v", err)
	}
}

func TestResourceDefaultsNonRefType(t *testing.T) {
	e := New()
	d := &ast.ResourceDefaults{Type: &ast.QualifiedName{Value: "file"}}
	if _, err := e.eval(d, e.top); err == nil || !strings.Contains(err.Error(), "require a type reference") {
		t.Errorf("got %v", err)
	}
}

func TestResourceDefaultsAttrError(t *testing.T) {
	if err := evalErr(t, `File { mode => nope() }`); !strings.Contains(err, "unknown function") {
		t.Errorf("defaults attr err: %q", err)
	}
}

func TestOverrideEmptyRefs(t *testing.T) {
	// Hand-built AST: override target evaluates to a non-reference (undef).
	e := New()
	o := &ast.ResourceOverride{Resource: &ast.Undef{}, Ops: []ast.AttributeOp{{Name: "x", Op: "=>", Value: &ast.Integer{Value: 1}}}}
	if _, err := e.eval(o, e.top); err == nil || !strings.Contains(err.Error(), "must be a resource reference") {
		t.Errorf("got %v", err)
	}
}

// failStore is an ExportedStore whose CollectExported and StoreExported fail.
type failStore struct{ onStore bool }

func (f failStore) StoreExported(node string, r *catalog.Resource) error {
	if f.onStore {
		return &Error{Msg: "store boom"}
	}
	return nil
}
func (f failStore) CollectExported(capType, excludeNode string) ([]*catalog.Resource, error) {
	return nil, &Error{Msg: "collect boom"}
}

func TestExportedStoreErrors(t *testing.T) {
	// CollectExported error propagates through a <<| |>> collection.
	if err := evalErr(t, `Sshkey <<| |>>`, WithExportedStore(failStore{})); !strings.Contains(err, "collect boom") {
		t.Errorf("collect error got %q", err)
	}
	// StoreExported error propagates when declaring an @@ resource.
	if err := evalErr(t, `@@sshkey { 'k': }`, WithExportedStore(failStore{onStore: true})); !strings.Contains(err, "store boom") {
		t.Errorf("store error got %q", err)
	}
}

func TestExportedImportSkipsDuplicate(t *testing.T) {
	store := NewMemoryExportedStore()
	catOf(t, `@@host { 'h': ip => '1' }`, WithNodeName("a"), WithExportedStore(store))
	// Node b declares its own Host[h] then collects; the imported duplicate is
	// skipped (no dup error), leaving one Host[h].
	cat := catOf(t, `host { 'h': ip => 'local' }
Host <<| |>>`, WithNodeName("b"), WithExportedStore(store))
	h, _ := cat.Get("Host[h]")
	if h.Parameters["ip"] != "local" {
		t.Errorf("local resource should win over duplicate import: %v", h.Parameters)
	}
}

func TestExportedImportNonMatch(t *testing.T) {
	store := NewMemoryExportedStore()
	catOf(t, `@@host { 'h': tag => 'x' }`, WithNodeName("a"), WithExportedStore(store))
	cat := catOf(t, `Host <<| tag == 'other' |>>`, WithNodeName("b"), WithExportedStore(store))
	if _, ok := cat.Get("Host[h]"); ok {
		t.Errorf("non-matching exported should not be imported")
	}
}
