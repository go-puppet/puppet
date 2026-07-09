// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

func TestCreateResources(t *testing.T) {
	cat := catOf(t, `
create_resources('file', {
  '/a' => { 'ensure' => 'file', 'mode' => '0600' },
  '/b' => { 'ensure' => 'directory' },
}, { 'owner' => 'root' })`)
	a, ok := cat.Get("File[/a]")
	if !ok || a.Parameters["mode"] != "0600" || a.Parameters["owner"] != "root" {
		t.Errorf("create_resources /a: %v", a)
	}
	b, ok := cat.Get("File[/b]")
	if !ok || b.Parameters["ensure"] != "directory" || b.Parameters["owner"] != "root" {
		t.Errorf("create_resources /b: %v", b)
	}
}

func TestCreateResourcesDefinedType(t *testing.T) {
	cat := catOf(t, `
define web::vhost(String $root) {
  file { "/etc/${title}.conf": content => $root }
}
create_resources('web::vhost', { 'site' => { 'root' => '/srv' } })`)
	if _, ok := cat.Get("Web::Vhost[site]"); !ok {
		t.Errorf("defined type not created: %v", cat.Resources())
	}
	if _, ok := cat.Get("File[/etc/site.conf]"); !ok {
		t.Errorf("nested resource not created")
	}
}

func TestCreateResourcesClass(t *testing.T) {
	cat := catOf(t, `
class myc(Integer $n = 0) { notice("n=${n}") }
create_resources('class', { 'myc' => { 'n' => 5 } })`)
	if _, ok := cat.Get("Class[Myc]"); !ok {
		t.Errorf("class not declared: %v", cat.Resources())
	}
}

func TestCreateResourcesErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`create_resources()`, "wrong number"},
		{`create_resources(5, {})`, "must be a String"},
		{`create_resources('file', 5)`, "must be a Hash"},
		{`create_resources('file', {'x'=>{}}, 5)`, "must be a Hash"},
		{`create_resources('file', {'x'=>5})`, "must be a Hash of attributes"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestEnsureResource(t *testing.T) {
	cat := catOf(t, `
file { '/x': ensure => present }
ensure_resource('file', '/x', { 'mode' => '0644' })
ensure_resource('file', '/y', { 'ensure' => 'file' })
ensure_resource('package', ['a','b'])`)
	// /x already existed: not re-declared (no dup error), original kept.
	x, _ := cat.Get("File[/x]")
	if _, has := x.Parameters["mode"]; has {
		t.Errorf("ensure_resource should not modify existing: %v", x.Parameters)
	}
	if _, ok := cat.Get("File[/y]"); !ok {
		t.Errorf("/y not created")
	}
	if _, ok := cat.Get("Package[a]"); !ok {
		t.Errorf("package a not created")
	}
	if _, ok := cat.Get("Package[b]"); !ok {
		t.Errorf("package b not created")
	}
}

func TestEnsureResourceErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`ensure_resource()`, "wrong number"},
		{`ensure_resource(5, 'x')`, "must be a String"},
		{`ensure_resource('file', 'x', 5)`, "must be a Hash"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestDefined(t *testing.T) {
	cases := []struct{ src, want string }{
		{`class c {}
include c
notice(defined('c'))`, "true"},
		{`notice(defined('nosuch'))`, "false"},
		{`define d(){}
notice(defined('d'))`, "true"},
		{`notice(defined('notice'))`, "true"},
		{`function myf(){1}
notice(defined('myf'))`, "true"},
		{`file { '/z': }
notice(defined(File['/z']))`, "true"},
		{`notice(defined(File['/nope']))`, "false"},
		{`class c {}
include c
notice(defined(Class['c']))`, "true"},
		{`$v = 5
notice(defined('$v'))`, "true"},
		{`notice(defined('$missing'))`, "false"},
		{`notice(defined(Integer))`, "true"},
		{`notice(defined(5))`, "false"},
		{`notice(defined('a','notice'))`, "true"},
	}
	for _, tc := range cases {
		if got := firstLog(t, tc.src); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.src, got, tc.want)
		}
	}
	if err := evalErr(t, `notice(defined())`); !strings.Contains(err, "wrong number") {
		t.Errorf("defined() arity: %q", err)
	}
}

func TestGetvar(t *testing.T) {
	if got := firstLog(t, `$x = 42
notice(getvar('x'))`); got != "42" {
		t.Errorf("getvar got %q", got)
	}
	if got := firstLog(t, `notice(getvar('nope', 'def'))`); got != "def" {
		t.Errorf("getvar default got %q", got)
	}
	if got := firstLog(t, `notice("[${getvar('nope')}]")`); got != "[]" {
		t.Errorf("getvar undef got %q", got)
	}
	if err := evalErr(t, `notice(getvar())`); !strings.Contains(err, "wrong number") {
		t.Errorf("getvar arity: %q", err)
	}
	if err := evalErr(t, `notice(getvar(5))`); !strings.Contains(err, "must be a String") {
		t.Errorf("getvar type: %q", err)
	}
}

func TestGetparam(t *testing.T) {
	if got := firstLog(t, `file { '/p': mode => '0644' }
notice(getparam(File['/p'], 'mode'))`); got != "0644" {
		t.Errorf("getparam got %q", got)
	}
	if got := firstLog(t, `file { '/p': }
notice("[${getparam(File['/p'], 'nope')}]")`); got != "[]" {
		t.Errorf("getparam missing param got %q", got)
	}
	if got := firstLog(t, `notice("[${getparam(File['/none'], 'mode')}]")`); got != "[]" {
		t.Errorf("getparam missing resource got %q", got)
	}
	if err := evalErr(t, `notice(getparam(File['/p']))`); !strings.Contains(err, "wrong number") {
		t.Errorf("getparam arity: %q", err)
	}
	if err := evalErr(t, `notice(getparam(File['/p'], 5))`); !strings.Contains(err, "must be a String") {
		t.Errorf("getparam type: %q", err)
	}
	if got := firstLog(t, `notice("[${getparam('notaref', 'x')}]")`); got != "[]" {
		t.Errorf("getparam non-ref got %q", got)
	}
}

func TestAssertPrivate(t *testing.T) {
	if _, _, err := EvalString(`assert_private()`); err != nil {
		t.Errorf("assert_private: %v", err)
	}
}

func TestOverrideMetaparamAndTag(t *testing.T) {
	cat := catOf(t, `
package { 'p': }
file { '/o': }
File['/o'] { require => Package['p'], tag => 'extra' }`)
	o, _ := cat.Get("File[/o]")
	foundTag := false
	for _, tg := range o.Tags {
		if tg == "extra" {
			foundTag = true
		}
	}
	if !foundTag {
		t.Errorf("override tag not applied: %v", o.Tags)
	}
	// require adds a reverse edge Package[p] -> File[/o]
	found := false
	for _, e := range cat.Edges() {
		if e.Source == "Package[p]" && e.Target == "File[/o]" {
			found = true
		}
	}
	if !found {
		t.Errorf("override require edge missing: %v", cat.Edges())
	}
}

func TestOverrideAppendHashAndReplace(t *testing.T) {
	cat := catOf(t, `
file { '/o': metadata => {'a' => 1} }
File['/o'] { metadata +> {'b' => 2} }`)
	o, _ := cat.Get("File[/o]")
	m, _ := o.Parameters["metadata"].(map[string]any)
	if len(m) != 2 {
		t.Errorf("hash append failed: %v", o.Parameters["metadata"])
	}
}

func TestOverrideAppendScalarReplaces(t *testing.T) {
	// +> on a non-collection existing value replaces it.
	cat := catOf(t, `
file { '/o': mode => '0600' }
File['/o'] { mode +> '0700' }`)
	o, _ := cat.Get("File[/o]")
	if o.Parameters["mode"] != "0700" {
		t.Errorf("scalar +> should replace: %v", o.Parameters["mode"])
	}
}

func TestCollectorErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`@file { '/v': }
File <| title |>`, "unsupported collector query"},
		{`@file { '/v': }
File <| 1 + 1 |>`, "unsupported collector query operator"},
		{`@file { '/v': }
File <| "x" == 1 |>`, "attribute must be a bare name"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestCollectorOrQuery(t *testing.T) {
	cat := catOf(t, `
@file { '/a': tag => 'x' }
@file { '/b': tag => 'y' }
@file { '/c': tag => 'z' }
File <| tag == 'x' or tag == 'y' |>`)
	realized := map[string]bool{}
	for _, r := range cat.Resources() {
		if !r.Virtual {
			realized[r.Title] = true
		}
	}
	if !realized["/a"] || !realized["/b"] || realized["/c"] {
		t.Errorf("or-query realized wrong set: %v", realized)
	}
}

func TestResourceDefaultsError(t *testing.T) {
	// A resource default with a splat op (no bare name) evaluated wrong.
	if err := evalErr(t, `File { * => 5 }`); !strings.Contains(err, "requires a hash") {
		t.Errorf("got %q", err)
	}
}
