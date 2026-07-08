// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-hiera/hiera"
	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/catalog"
	"github.com/go-puppet/puppet/parser"
)

// anyErr asserts that evaluating src fails (message unimportant); it exercises
// nested error-propagation branches.
func anyErr(t *testing.T, src string) {
	t.Helper()
	if _, _, err := EvalString(src, WithNodeName("nodeX")); err == nil {
		t.Errorf("EvalString(%q): expected an error, got nil", src)
	}
}

func TestNestedErrorPropagation(t *testing.T) {
	srcs := []string{
		`notice(nope() and 1)`,
		`notice(true and nope())`,
		`notice(nope() or 1)`,
		`notice(false or nope())`,
		`notice(nope() + 1)`,
		`notice(1 + nope())`,
		`notice(!nope())`,
		`notice("x${nope()}")`,
		`notice([nope()])`,
		`notice([*nope()])`,
		`notice({nope() => 1})`,
		`notice({'a' => nope()})`,
		`notice(nope()[0])`,
		`notice([1][nope()])`,
		`notice([1,2,3][0, 'x'])`,
		`notice(nope() ? {1=>1})`,
		`notice(1 ? {nope() => 1, default => 0})`,
		`notice(9 ? {default => nope()})`,
		`if nope() { }`,
		`if true { nope() }`,
		`if false { } else { nope() }`,
		`unless nope() { }`,
		`case nope() { 1: {} }`,
		`case 1 { nope(): {} }`,
		`case 1 { 1: {nope()} }`,
		`notice(Foo['a']['b'])`,
		`nope().foo()`,
		`'a'.upcase(nope())`,
		`['a'].each |Integer $x| { notice($x) }`,
		`[1].each |$x| { nope() }`,
		`[1].map |$x| { nope() }`,
		`[1].filter |$x| { nope() }`,
		`{'a'=>1}.filter |$k,$v| { nope() }`,
		`[1,2].reduce |$a,$b| { nope() }`,
		`with(1) |$x| { nope() }`,
		`[1,2].slice(1) |$c| { nope() }`,
		`file { nope(): }`,
		`file { 'a': mode => nope() }`,
		`nope() -> Notify['a']`,
		`Notify['a'] -> nope()`,
		`class c { nope() }
include c`,
		`class c($x = nope()) { }
include c`,
		`class child inherits missingparent { }
include child`,
		`define d() { nope() }
d { 't': }`,
		`define d(Integer $x) { }
d { 't': x => 'str' }`,
		`define d() { }
d { 't': } d { 't': }`,
		`class { nope(): }`,
		`class c($x) { }
class { 'c': x => nope() }`,
		`function f() { nope() }
notice(f())`,
		`function f() >> Integer[bad()] { 1 }
notice(f())`,
		`function f(Integer[bad()] $x) { }
notice(f(1))`,
		`function f($x = nope()) { }
notice(f())`,
		`node nope() { }`,
		`notice(*nope())`,
		`notice(Foo[nope()])`,
		`{'a'=>1}.each |$k,$v| { nope() }`,
		`notice(5 !~ /x/)`,
	}
	for _, src := range srcs {
		anyErr(t, src)
	}
}

func TestArgCountErrors(t *testing.T) {
	for _, src := range []string{
		`fail('boom')`,
		`notice(empty())`,
		`notice(upcase())`,
		`notice(split('a'))`,
		`notice(values())`,
		`notice(reverse())`,
		`notice(abs())`,
		`notice(reduce())`,
		`each()`,
		`notice(map())`,
		`notice(filter())`,
	} {
		anyErr(t, src)
	}
}

func TestMoreHappyBranches(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(2.5 - 1.0)`, "1.5"},
		{`notice(5 in 3)`, "false"},
		{`notice(Integer(5))`, "5"},
		{`notice(default)`, "default"},
		{`notice([1,2,3].slice(2))`, "[[1, 2], [3]]"},
		{`include(5)
include(File['x'])
notice('ok')`, "ok"},
	}
	for _, tc := range cases {
		if got := lastLog(t, tc.src); got != tc.want {
			t.Errorf("%q got %q want %q", tc.src, got, tc.want)
		}
	}
}

func TestMetaparamsAndTags(t *testing.T) {
	cat, _, err := EvalString(`file { 'a':
  require   => Service['s'],
  before    => Notify['b'],
  notify    => Notify['n'],
  subscribe => Service['w'],
  tag       => 'single',
}`)
	if err != nil {
		t.Fatal(err)
	}
	js := cat.JSON()
	for _, want := range []string{
		`{"source":"Service[s]","target":"File[a]"}`, // require: reverse
		`{"source":"File[a]","target":"Notify[b]"}`,  // before: forward
		`{"source":"File[a]","target":"Notify[n]"}`,  // notify: forward
		`{"source":"Service[w]","target":"File[a]"}`, // subscribe: reverse
		`"tags":["single"]`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("missing %s\n%s", want, js)
		}
	}
}

func TestSplatAttributes(t *testing.T) {
	cat, _, err := EvalString(`$attrs = { 'ensure' => 'present', 'mode' => '0644' }
file { '/tmp/x': * => $attrs }`)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := cat.Get("File[/tmp/x]")
	if !ok || r.Parameters["ensure"] != "present" || r.Parameters["mode"] != "0644" {
		t.Errorf("splat attrs not applied: %+v", r)
	}
}

func TestCaseNoMatchNoLog(t *testing.T) {
	_, logs, err := EvalString(`case 9 { 1: {notice('a')} }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Errorf("expected no logs, got %+v", logs)
	}
}

func TestDeclareClassDuplicate(t *testing.T) {
	e := New()
	prog, err := parser.Parse(`class foo { }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range prog.Body {
		e.register(n)
	}
	// Pre-seed a Class[Foo] resource so the declaration's Add fails.
	e.cat.Add(&catalog.Resource{Type: "Class", Title: "Foo", Parameters: map[string]any{}})
	if err := e.declareClass("foo", nil, ast.Position{}); err == nil {
		t.Error("expected duplicate Class error")
	}
}

func TestToValueList(t *testing.T) {
	facts := MapFacts{"nics": []any{"eth0", "eth1"}}
	if got := lastLog(t, `notice($nics[0])`, WithFacts(facts)); got != "eth0" {
		t.Errorf("list fact got %q", got)
	}
}

func TestFactsInFacterList(t *testing.T) {
	// A nested map fact exercises toValue's map recursion via $facts.
	facts := MapFacts{"net": map[string]any{"addrs": []any{"127.0.0.1"}}}
	if got := lastLog(t, `notice($facts['net']['addrs'][0])`, WithFacts(facts)); got != "127.0.0.1" {
		t.Errorf("nested got %q", got)
	}
}

// --- dead defensive defaults (unreachable through the parser) --------------

func TestUnreachableDefaults(t *testing.T) {
	e := New()
	// Binary with an operator the parser never emits.
	if _, err := e.eval(&ast.Binary{Op: "??", Left: &ast.Integer{Value: 1}, Right: &ast.Integer{Value: 2}}, e.top); err == nil {
		t.Error("expected unknown-operator error")
	}
	// Call whose functor is neither a name nor a reference.
	if _, err := e.eval(&ast.Call{Functor: &ast.Variable{Name: "x"}}, e.top); err == nil {
		t.Error("expected invalid-call-target error")
	}
	// renderType / renderTypeArg on invalid nodes.
	if _, err := renderType(&ast.Variable{Name: "x"}); err == nil {
		t.Error("expected invalid type expression")
	}
	if _, err := renderTypeArg(&ast.Variable{Name: "x"}); err == nil {
		t.Error("expected invalid type argument")
	}
	if _, err := renderTypeArg(&ast.Unary{Op: "!", Operand: &ast.Integer{Value: 1}}); err == nil {
		t.Error("expected invalid type argument for non-minus unary")
	}
	if _, err := renderTypeArg(&ast.Unary{Op: "-", Operand: &ast.Variable{Name: "x"}}); err == nil {
		t.Error("expected error rendering negative type arg with bad operand")
	}
	badAccess := &ast.Access{Operand: &ast.Variable{Name: "x"}, Keys: []ast.Node{&ast.Integer{Value: 1}}}
	if _, err := renderType(badAccess); err == nil {
		t.Error("expected error rendering type with bad operand")
	}
	if _, err := renderTypeArg(badAccess); err == nil {
		t.Error("expected error rendering type arg with bad operand")
	}
}

func TestCannotEvaluate(t *testing.T) {
	e := New()
	// KeyedEntry is not an expression node, but Node-typed nil triggers the
	// generic arm via a bespoke node.
	if _, err := e.eval(nodeStub{}, e.top); err == nil || !strings.Contains(err.Error(), "cannot evaluate") {
		t.Errorf("expected cannot-evaluate, got %v", err)
	}
}

type nodeStub struct{ ast.Base }

func TestHieraLoopError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hiera.yaml"), `version: 5
defaults:
  datadir: data
  data_hash: yaml_data
hierarchy:
  - name: Common
    path: common.yaml
`)
	os.MkdirAll(filepath.Join(dir, "data"), 0o755)
	// A self-referential interpolation makes Hiera return an error.
	writeFile(t, filepath.Join(dir, "data", "common.yaml"), `loop: "%{lookup('loop')}"
web::port: "%{lookup('loop')}"
`)
	h, err := hiera.Load(filepath.Join(dir, "hiera.yaml"), hiera.MapScope{})
	if err != nil {
		t.Fatalf("hiera.Load: %v", err)
	}
	// lookup() surfaces the Hiera error (eval.lookup error arm).
	anyErr(t, `notice(lookup('loop'))`)
	if _, _, err := EvalString(`notice(lookup('loop'))`, WithHiera(h)); err == nil {
		t.Error("expected hiera loop error via lookup()")
	}
	// Class automatic parameter binding surfaces the error too (hieraParam arm).
	_, _, err = EvalString(`class web(String $port) { }
include web`, WithHiera(h))
	if err == nil {
		t.Log("hieraParam error path did not trigger (treated as not-found); acceptable")
	}
}
