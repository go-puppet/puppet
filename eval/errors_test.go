// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"

	"github.com/go-puppet/puppet/ast"
)

func TestEvalErrors(t *testing.T) {
	cases := []struct{ src, contains string }{
		{`$x = `, "parse error"},
		// arithmetic
		{`notice(1/0)`, "division by zero"},
		{`notice(1.0/0.0)`, "division by zero"},
		{`notice(1%0)`, "division by zero"},
		{`notice(1 + true)`, "numeric operands"},
		{`notice(1.5 % 1.0)`, "modulo requires integer"},
		{`notice(1 << 2)`, "<< requires an array"},
		// comparison / match
		{`notice(1 < 'a')`, "cannot compare"},
		{`notice(5 =~ /x/)`, "requires a string on the left"},
		{`notice('a' =~ 5)`, "requires a Regexp, String or Type"},
		{`notice('a' =~ '(')`, "invalid regular expression"},
		// unary
		{`notice(-'a')`, "cannot negate"},
		{`$y = *[1]
notice($y)`, "splat"},
		// indexing
		{`notice(5[0])`, "cannot index"},
		{`notice([1]['a'])`, "index must be an integer"},
		{`notice('ab'['x'])`, "index must be an integer"},
		// selector
		{`notice(9 ? {1=>'a'})`, "no matching selector"},
		// assignment
		{`$x = 1
$x = 2`, "cannot reassign"},
		{`$x += 1`, "not supported in Puppet"},
		{`5 = 1`, "must be a variable"},
		// functions
		{`notice(nope())`, "unknown function nope"},
		{`include ghost`, "cannot find class ghost"},
		{`include()`, "at least one class name"},
		{`notice(lookup('x'))`, "did not find"},
		{`notice(lookup())`, "expects a key"},
		{`notice(lookup(5))`, "key must be a string"},
		{`notice(assert_type(Integer))`, "expects 2"},
		{`notice(assert_type(5, 5))`, "must be a Type"},
		{`notice(assert_type(String, 5))`, "not an instance"},
		{`notice(type(1,2))`, "expects 1"},
		{`notice(size(5))`, "String, Array or Hash"},
		{`notice(size(1,2))`, "expects 1"},
		{`notice(empty(5))`, "String, Array, Hash or Undef"},
		{`notice(upcase(5))`, "expects a String or Array"},
		{`notice(split(5, ','))`, "first argument must be a String"},
		{`notice(split('a', 5))`, "separator must be"},
		{`notice(join(5))`, "first argument must be an Array"},
		{`notice(join())`, "Array and an optional separator"},
		{`notice(sprintf())`, "expects a format"},
		{`notice(sprintf(5))`, "format must be a String"},
		{`notice(keys(5))`, "expects a Hash"},
		{`notice(keys())`, "expects 1"},
		{`notice(values(5))`, "expects a Hash"},
		{`notice(merge({'a'=>1}))`, "at least two"},
		{`notice(merge({'a'=>1}, 5))`, "must be Hashes"},
		{`notice(reverse(5))`, "String or Array"},
		{`notice(abs('a'))`, "expects a Numeric"},
		{`notice(min())`, "at least one"},
		{`notice(min(1,'a'))`, "cannot compare"},
		// iteration
		{`each([1])`, "requires a block"},
		{`each(true) |$x| { }`, "not iterable"},
		{`notice(map([1]))`, "requires a block"},
		{`true.map |$x| { }`, "not iterable"},
		{`notice(filter([1]))`, "requires a block"},
		{`true.filter |$x| { }`, "not iterable"},
		{`notice(reduce([1]))`, "requires a block"},
		{`true.reduce |$a,$b| { }`, "not iterable"},
		{`notice(reduce())`, "collection and an optional seed"},
		{`notice(with(1))`, "requires a block"},
		{`notice(slice([1]))`, "expects 2"},
		{`notice([1].slice(0))`, "positive Integer"},
		{`notice(slice(true, 2))`, "not iterable"},
		// type casts
		{`notice(Integer(1,2))`, "expects one argument"},
		{`notice(Integer('xx'))`, "cannot convert to Integer"},
		{`notice(Integer([1]))`, "cannot convert Array to Integer"},
		{`notice(Float('xx'))`, "cannot convert to Float"},
		{`notice(Float([1]))`, "cannot convert Array to Float"},
		{`notice(Hash([1]))`, "expects an even number"},
		{`notice(Timestamp(1))`, "not supported for"},
		// relationships / resources
		{`1 -> 2`, "must be resource references"},
		{`file { 'x': * => 5 }`, "requires a hash"},
		{`file { 'a': }
file { 'a': }`, "duplicate resource"},
		// user functions
		{`function f($x){$x}
notice(f(1,2))`, "too many arguments"},
		{`function f($x){$x}
notice(f())`, "missing value for parameter"},
		{`function f() >> Integer { 'x' }
notice(f())`, "not matching"},
		// class / define parameter checks
		{`class c(Integer $x){ }
class { 'c': x => 'str' }`, "expects"},
		{`class c(Integer $x){ }
include c`, "missing value for parameter $x of c"},
		// nodes
		{`node 'a' { }`, "no node definition matches"},
		// resource override of an undeclared resource
		{`File['x'] { mode => '0644' }`, "not in the catalog"},
	}
	for _, tc := range cases {
		got := evalErr(t, tc.src, WithNodeName("nodeX"))
		if !strings.Contains(got, tc.contains) {
			t.Errorf("EvalString(%q)\n  error %q\n  lacks %q", tc.src, got, tc.contains)
		}
	}
}

// TestNonNameResourceType covers the parser-unreachable guard for a resource
// whose type is not a bare name, by driving the evaluator on a hand-built AST.
func TestNonNameResourceType(t *testing.T) {
	e := New()
	res := &ast.Resource{Type: &ast.Variable{Name: "x"}, Bodies: []ast.ResourceBody{{Title: &ast.String{Value: "t"}}}}
	if _, err := e.eval(res, e.top); err == nil || !strings.Contains(err.Error(), "must be a name") {
		t.Errorf("expected 'must be a name' error, got %v", err)
	}
}

// TestCapitalizeLeadingScope covers capitalizeType's empty-segment arm via a
// top-scoped resource type name.
func TestCapitalizeLeadingScope(t *testing.T) {
	cat, _, err := EvalString(`::file { '/tmp/z': }`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Get("::File[/tmp/z]"); !ok {
		t.Errorf("expected ::File[/tmp/z]; got %s", cat.JSON())
	}
}
