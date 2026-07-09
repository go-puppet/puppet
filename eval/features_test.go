// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"testing"

	"github.com/go-puppet/puppet/catalog"
)

// logsOf evaluates src and returns the notice messages, failing on error.
func logsOf(t *testing.T, src string, opts ...Option) []string {
	t.Helper()
	_, logs, err := EvalString(src, opts...)
	if err != nil {
		t.Fatalf("EvalString(%q) error: %v", src, err)
	}
	var out []string
	for _, l := range logs {
		out = append(out, l.Message)
	}
	return out
}

func catOf(t *testing.T, src string, opts ...Option) *catalog.Catalog {
	t.Helper()
	cat, _, err := EvalString(src, opts...)
	if err != nil {
		t.Fatalf("EvalString(%q) error: %v", src, err)
	}
	return cat
}

func firstLog(t *testing.T, src string, opts ...Option) string {
	t.Helper()
	logs := logsOf(t, src, opts...)
	if len(logs) == 0 {
		t.Fatalf("EvalString(%q): no logs", src)
	}
	return logs[0]
}

func TestRegexCaptures(t *testing.T) {
	cases := []struct{ src, want string }{
		{`if 'abc123' =~ /([a-z]+)(\d+)/ { notice("${0}|${1}|${2}") }`, "abc123|abc|123"},
		{`$x='v1.2'
if $x =~ /(\d+)\.(\d+)/ { notice("${1}.${2}") }`, "1.2"},
		{`case 'foo42' { /o(\d+)/: { notice($1) } default: { notice('no') } }`, "42"},
		{`notice('n9' ? { /(\d)/ => "d${1}", default => 'x' })`, "d9"},
		{`if 'abc' =~ 'b' { notice('strmatch') }`, "strmatch"},
		{`if 'abc' =~ /(x)?(b)/ { notice("${1}-${2}") }`, "-b"},
	}
	for _, tc := range cases {
		if got := firstLog(t, tc.src); got != tc.want {
			t.Errorf("%q => %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestRegexNoMatchClearsVars(t *testing.T) {
	// A failed match followed by a lookup of $1 yields empty (undef).
	got := firstLog(t, `if 'abc' =~ /z/ { notice('yes') } else { notice("[${1}]") }`)
	if got != "[]" {
		t.Errorf("got %q, want %q", got, "[]")
	}
}

func TestResourceDefaults(t *testing.T) {
	cat := catOf(t, `
File { mode => '0644', owner => 'root' }
file { '/a': }
file { '/b': mode => '0600' }`)
	a, _ := cat.Get("File[/a]")
	if a.Parameters["mode"] != "0644" || a.Parameters["owner"] != "root" {
		t.Errorf("A defaults not applied: %v", a.Parameters)
	}
	b, _ := cat.Get("File[/b]")
	if b.Parameters["mode"] != "0600" {
		t.Errorf("B explicit should win: %v", b.Parameters)
	}
	if b.Parameters["owner"] != "root" {
		t.Errorf("B should inherit owner default: %v", b.Parameters)
	}
}

func TestResourceDefaultsNestedScope(t *testing.T) {
	cat := catOf(t, `
class inner { file { '/n': } }
class outer { File { mode => '0640' } include inner }
include outer`)
	// The default is scoped to `outer`; `inner` is a separate scope, so /n
	// should NOT inherit outer's default (dynamic scoping is per class body).
	n, _ := cat.Get("File[/n]")
	if _, ok := n.Parameters["mode"]; ok {
		t.Errorf("nested class should not inherit sibling default: %v", n.Parameters)
	}
}

func TestResourceOverride(t *testing.T) {
	cat := catOf(t, `
file { '/o': ensure => present }
File['/o'] { mode => '0700', owner => 'app' }`)
	o, _ := cat.Get("File[/o]")
	if o.Parameters["mode"] != "0700" || o.Parameters["owner"] != "app" {
		t.Errorf("override not applied: %v", o.Parameters)
	}
}

func TestResourceOverrideAppend(t *testing.T) {
	cat := catOf(t, `
file { '/o': require => Package['p'] }
package { 'p': }
File['/o'] { tag +> ['extra'] }`)
	o, _ := cat.Get("File[/o]")
	found := false
	for _, tg := range o.Tags {
		if tg == "extra" {
			found = true
		}
	}
	if !found {
		t.Errorf("append tag not applied: %v", o.Tags)
	}
}

func TestCollectorVirtual(t *testing.T) {
	cat := catOf(t, `
@file { '/v1': tag => 'web' }
@file { '/v2': tag => 'db' }
File <| tag == 'web' |>`)
	v1, _ := cat.Get("File[/v1]")
	if v1.Virtual {
		t.Errorf("/v1 should be realized")
	}
	v2, _ := cat.Get("File[/v2]")
	if !v2.Virtual {
		t.Errorf("/v2 should remain virtual")
	}
}

func TestCollectorQueryForms(t *testing.T) {
	cat := catOf(t, `
@file { '/q1': tag => 'a', mode => '0600' }
@file { '/q2': tag => 'b', mode => '0600' }
@file { '/q3': tag => 'a', mode => '0644' }
File <| tag == 'a' and mode == '0600' |>
File <| title == '/q2' |>`)
	realized := map[string]bool{}
	for _, r := range cat.Resources() {
		if !r.Virtual {
			realized[r.Title] = true
		}
	}
	if !realized["/q1"] || !realized["/q2"] || realized["/q3"] {
		t.Errorf("query realized wrong set: %v", realized)
	}
}

func TestCollectorAllAndNotEqual(t *testing.T) {
	cat := catOf(t, `
@user { 'alice': tag => 'x' }
@user { 'bob': tag => 'y' }
User <| tag != 'y' |>`)
	a, _ := cat.Get("User[alice]")
	b, _ := cat.Get("User[bob]")
	if a.Virtual || !b.Virtual {
		t.Errorf("!= query wrong: alice.virtual=%v bob.virtual=%v", a.Virtual, b.Virtual)
	}
}

func TestExportedCrossNode(t *testing.T) {
	store := NewMemoryExportedStore()
	catOf(t, `@@host { 'web1': ip => '10.0.0.1' }`, WithNodeName("web1"), WithExportedStore(store))
	cat := catOf(t, `Host <<| |>>`, WithNodeName("lb1"), WithExportedStore(store))
	h, ok := cat.Get("Host[web1]")
	if !ok || h.Parameters["ip"] != "10.0.0.1" {
		t.Errorf("exported host not collected: %v", cat.Resources())
	}
}

func TestExportedSameNodeExcluded(t *testing.T) {
	store := NewMemoryExportedStore()
	catOf(t, `@@host { 'self': ip => '1.1.1.1' }`, WithNodeName("me"), WithExportedStore(store))
	// The same node collecting should not re-import its own export as a new one.
	cat := catOf(t, `@@host { 'self': ip => '1.1.1.1' }
Host <<| |>>`, WithNodeName("me"), WithExportedStore(store))
	count := 0
	for _, r := range cat.Resources() {
		if r.Type == "Host" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 Host, got %d", count)
	}
}

func TestExportedInCatalogCollection(t *testing.T) {
	// Without a store, exported resources are still collectable within the
	// same compilation.
	cat := catOf(t, `
@@sshkey { 'k1': type => 'rsa' }
Sshkey <<| |>>`)
	k, ok := cat.Get("Sshkey[k1]")
	if !ok || k.Exported {
		t.Errorf("in-catalog exported not realized: %v", cat.Resources())
	}
}
