// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

func TestTypeConstructors(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`Integer("5")`, "5"},
		{`Integer(5)`, "5"},
		{`Integer(5.9)`, "5"},
		{`Float("1.5")`, "1.5"},
		{`Float(3)`, "3"},
		{`String(5)`, "5"},
		{`Boolean(0)`, "false"},
		{`Boolean(1)`, "true"},
		{`Numeric("5")`, "5"},
		{`Numeric("1.5")`, "1.5"},
		{`Numeric(3)`, "3"},
		{`Array("x")`, `["x"]`},
		{`Array([1,2])`, "[1, 2]"},
		{`Array({"a"=>1})`, `[["a", 1]]`},
		{`Array("x", true)`, `["x"]`},
		{`Array([1], true)`, "[[1]]"},
		{`Hash([["a",1]])`, `{"a" => 1}`},
		{`Hash([ "a",1 ])`, `{"a" => 1}`},
		{`Hash({"a"=>1})`, `{"a" => 1}`},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestTypeConstructorErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(Numeric("x"))`, "cannot convert"},
		{`notice(Array())`, "one or two arguments"},
		{`notice(Hash(5))`, "cannot convert"},
		{`notice(Hash())`, "expects one argument"},
		{`notice(Float(true))`, "cannot convert Boolean to Float"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestIntegerIteration(t *testing.T) {
	if got := firstLog(t, `$s = 0
3.each |$i| { notice($i) }`); got != "0" {
		t.Errorf("Integer.each first got %q", got)
	}
	if got := evalOut(t, `3.map |$i| { $i * 10 }`); got != "[0, 10, 20]" {
		t.Errorf("Integer.map got %q", got)
	}
	if got := evalOut(t, `4.reduce |$a,$b| { $a + $b }`); got != "6" {
		t.Errorf("Integer.reduce got %q", got)
	}
	if got := evalOut(t, `3.filter |$i| { $i > 0 }`); got != "[1, 2]" {
		t.Errorf("Integer.filter got %q", got)
	}
	if got := evalOut(t, `slice(4, 2)`); got != "[[0, 1], [2, 3]]" {
		t.Errorf("Integer.slice got %q", got)
	}
}

func TestReduceEmptyAndSeed(t *testing.T) {
	if got := evalOut(t, `[].reduce |$a,$b| { $a + $b }`); got != "" {
		t.Errorf("empty reduce got %q", got)
	}
	if got := evalOut(t, `[].reduce(7) |$a,$b| { $a + $b }`); got != "7" {
		t.Errorf("seeded empty reduce got %q", got)
	}
}

func TestSliceEmpty(t *testing.T) {
	if got := evalOut(t, `slice([], 2)`); got != "[]" {
		t.Errorf("slice empty got %q", got)
	}
}

func TestSortWithBlock(t *testing.T) {
	// A comparator returning -1/0/1.
	if got := evalOut(t, `[3,1,2].sort |$a,$b| { if $a < $b { -1 } elsif $a > $b { 1 } else { 0 } }`); got != "[1, 2, 3]" {
		t.Errorf("sort custom got %q", got)
	}
	if err := evalErr(t, `notice([1,2].sort |$a,$b| { nope() })`); !strings.Contains(err, "unknown function") {
		t.Errorf("sort block error got %q", err)
	}
	if err := evalErr(t, `notice(sort([1,"a"]))`); !strings.Contains(err, "cannot compare") {
		t.Errorf("sort incomparable got %q", err)
	}
}

func TestStringEdgeFns(t *testing.T) {
	if got := evalOut(t, `chop("")`); got != "" {
		t.Errorf("chop empty got %q", got)
	}
	if got := evalOut(t, `upcase_first("")`); got != "" {
		t.Errorf("upcase_first empty got %q", got)
	}
	if got := evalOut(t, `camelcase("a")`); got != "A" {
		t.Errorf("camelcase got %q", got)
	}
	// regsubst with a $-containing replacement (escaped to $$); single-quoted so
	// Puppet does not interpolate it.
	if got := evalOut(t, `regsubst("ab", "a", '$x')`); got != "$xb" {
		t.Errorf("regsubst dollar got %q", got)
	}
}

func TestReduceBlockError(t *testing.T) {
	if err := evalErr(t, `notice([1,2].reduce |$a,$b| { nope() })`); !strings.Contains(err, "unknown function") {
		t.Errorf("reduce block error got %q", err)
	}
	if err := evalErr(t, `notice([1,2].map |$x| { nope() })`); !strings.Contains(err, "unknown function") {
		t.Errorf("map block error got %q", err)
	}
	if err := evalErr(t, `[1,2].each |$x| { nope() }`); !strings.Contains(err, "unknown function") {
		t.Errorf("each block error got %q", err)
	}
	if err := evalErr(t, `notice([1,2].filter |$x| { nope() })`); !strings.Contains(err, "unknown function") {
		t.Errorf("filter block error got %q", err)
	}
	if err := evalErr(t, `notice({"a"=>1}.filter |$k,$v| { nope() })`); !strings.Contains(err, "unknown function") {
		t.Errorf("hash filter block error got %q", err)
	}
	if err := evalErr(t, `notice(index([1]) |$x| { nope() })`); !strings.Contains(err, "unknown function") {
		t.Errorf("index block error got %q", err)
	}
	if err := evalErr(t, `notice(any([1]) |$x| { nope() })`); !strings.Contains(err, "unknown function") {
		t.Errorf("any block error got %q", err)
	}
	if err := evalErr(t, `notice(all([1]) |$x| { nope() })`); !strings.Contains(err, "unknown function") {
		t.Errorf("all block error got %q", err)
	}
	if err := evalErr(t, `notice(tree_each([1]) |$x| { nope() })`); !strings.Contains(err, "unknown function") {
		t.Errorf("tree_each block error got %q", err)
	}
	if err := evalErr(t, `range(1,2) |$x| { nope() }`); !strings.Contains(err, "unknown function") {
		t.Errorf("range block error got %q", err)
	}
	if err := evalErr(t, `slice([1,2], 1) |$x| { nope() }`); !strings.Contains(err, "unknown function") {
		t.Errorf("slice block error got %q", err)
	}
}
