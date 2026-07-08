// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"testing"

	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/catalog"
	"github.com/go-puppet/puppet/parser"
)

func TestEqualsHelper(t *testing.T) {
	tt := []struct {
		a, b Value
		want bool
	}{
		{int64(1), int64(1), true},
		{int64(1), int64(2), false},
		{int64(2), 2.0, true},
		{int64(1), 2.0, false},
		{int64(1), "a", false},
		{1.0, int64(1), true},
		{1.5, int64(1), false},
		{1.5, 1.5, true},
		{1.5, "a", false},
		{"a", "a", true},
		{"a", "b", false},
		{"a", int64(1), false},
		{true, true, true},
		{true, false, false},
		{true, int64(1), false},
		{[]any{int64(1)}, []any{int64(1)}, true},
		{[]any{int64(1)}, []any{int64(1), int64(2)}, false},
		{[]any{int64(1)}, []any{int64(2)}, false},
		{[]any{int64(1)}, "x", false},
		{map[string]any{"a": int64(1)}, map[string]any{"a": int64(1)}, true},
		{map[string]any{"a": int64(1)}, map[string]any{"a": int64(1), "b": int64(2)}, false},
		{map[string]any{"a": int64(1)}, map[string]any{"b": int64(1)}, false},
		{map[string]any{"a": int64(1)}, map[string]any{"a": int64(2)}, false},
		{map[string]any{"a": int64(1)}, "x", false},
		{pcore.Undef, nil, true},
		{pcore.Undef, int64(1), false},
		{pcore.Default, pcore.Default, true},
	}
	for _, c := range tt {
		if got := equals(c.a, c.b); got != c.want {
			t.Errorf("equals(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestValueHelpers(t *testing.T) {
	if normalize(nil) != pcore.Undef {
		t.Error("normalize nil")
	}
	if normalize(int(5)) != int64(5) || normalize(int32(5)) != int64(5) || normalize(float32(1.5)) != 1.5 {
		t.Error("normalize widen")
	}
	if normalize("s") != "s" {
		t.Error("normalize default")
	}
	if !isUndef(nil) || !isUndef(pcore.Undef) || isUndef(int64(1)) {
		t.Error("isUndef")
	}
	if truthy(pcore.Undef) || !truthy(true) || truthy(false) || !truthy("x") {
		t.Error("truthy")
	}
	for _, c := range []struct {
		a, b Value
		want int
	}{{int64(1), int64(2), -1}, {int64(2), int64(1), 1}, {int64(1), int64(1), 0}, {"a", "b", -1}} {
		if got, err := compare(c.a, c.b); err != nil || got != c.want {
			t.Errorf("compare(%v,%v)=%v,%v", c.a, c.b, got, err)
		}
	}
	if _, err := compare(int64(1), "a"); err == nil {
		t.Error("compare mismatch should error")
	}
	if stringify(pcore.Undef) != "" {
		t.Error("stringify undef")
	}
	if stringify(pcore.Default) != "default" {
		t.Errorf("stringify default = %q", stringify(pcore.Default))
	}
}

func TestMapFactsDirect(t *testing.T) {
	f := MapFacts{
		"os":   map[string]any{"family": "Debian"},
		"list": []any{"a", "b"},
	}
	if v, ok := f.Fact("os.family"); !ok || v != "Debian" {
		t.Errorf("os.family = %v %v", v, ok)
	}
	if v, ok := f.Fact("list.1"); !ok || v != "b" {
		t.Errorf("list.1 = %v %v", v, ok)
	}
	for _, miss := range []string{"list.9", "list.x", "os.nope", "os.family.x", "missing"} {
		if _, ok := f.Fact(miss); ok {
			t.Errorf("expected miss for %q", miss)
		}
	}
}

func TestRegisterFunctionAndContext(t *testing.T) {
	e := New()
	e.RegisterFunction("mkres", func(c *Context, args []Value, block *Block) (Value, error) {
		c.Catalog().Add(&catalog.Resource{Type: "X", Title: "y", Parameters: map[string]any{}})
		c.Log("notice", "made "+stringify(args[0]))
		return pcore.Undef, nil
	})
	prog, err := parser.Parse(`mkres('z')`)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := e.EvalProgram(prog)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Get("X[y]"); !ok {
		t.Error("custom function did not add resource")
	}
	if len(e.Logs()) != 1 || e.Logs()[0].Message != "made z" {
		t.Errorf("logs = %+v", e.Logs())
	}
}

func TestMoreExpressions(t *testing.T) {
	cases := []struct{ src, want string }{
		// splat in array literal and in call args
		{`notice([1, *[2,3], 4])`, "[1, 2, 3, 4]"},
		{`notice([1, *5])`, "[1, 5]"},
		{`notice(max(*[3,1,2]))`, "3"},
		{`notice(max(*5))`, "5"},
		// capturesRest
		{`function f(*$rest) { $rest }
notice(f(1,2,3))`, "[1, 2, 3]"},
		// parameter default referencing earlier param
		{`function g($a, $b = $a) { $b }
notice(g(7))`, "7"},
		// data-type arguments (renderTypeArg arms)
		{`notice(5 =~ Integer[-10, 10])`, "true"},
		{`notice('a' =~ Enum['a', 'b'])`, "true"},
		{`notice([1,2] =~ Array[Integer])`, "true"},
		{`notice(1.5 =~ Float[1.0, 2.0])`, "true"},
		{`notice(3 =~ Integer[0, default])`, "true"},
		// case: no match, no default -> undef (no log); regexp vs non-string
		{`case 5 { /x/: {notice('a')} default: {notice('b')} }`, "b"},
		// relationships: reverse ops and arrays
		{`Notify['a'] <- Notify['b']
notice('ok')`, "ok"},
		{`[File['a'], File['b']] -> File['c']
notice('ok')`, "ok"},
		// resource reference array + stringify of a ResourceRef
		{`notice(File['a', 'b'])`, "[File[a], File[b]]"},
		{`notice("${Service['nginx']}")`, "Service[nginx]"},
		// empty hash / hash iteration via reduce & slice
		{`notice(empty({}))`, "true"},
		{`notice(empty({'a'=>1}))`, "false"},
		{`notice({'a'=>1,'b'=>2}.reduce([]) |$m,$p| { $m + [$p[0]] })`, `["a", "b"]`},
		{`notice({'a'=>1,'b'=>2}.slice(1)[0])`, `[["a", 1]]`},
		// negative/oob array slice clamping
		{`notice([1,2,3][-10, 2])`, "[1, 2]"},
		{`notice([1,2,3][5, 2])`, "[]"},
		// qualified & top-scope variable lookup
		{`notice($no::such)`, ""},
		// include with array and Class ref
		{`class a {} class b {}
include([a, b])
include(Class['a'])
notice('done')`, "done"},
	}
	for _, tc := range cases {
		if got := lastLog(t, tc.src, WithNodeName("n")); got != tc.want {
			t.Errorf("%q\n  got  %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

func TestTopScopeVarAndFacts(t *testing.T) {
	facts := MapFacts{"hostname": "web1"}
	if got := lastLog(t, `notice($::hostname)`, WithFacts(facts)); got != "web1" {
		t.Errorf("top-scope var got %q", got)
	}
}
