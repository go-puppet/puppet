// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package hcl_test

import (
	"strings"
	"testing"

	puppet "github.com/go-puppet/puppet"
	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/hcl"
)

// equiv asserts that the HCL2 manifest and the twin Puppet manifest parse to
// structurally identical programs. Comparison is via the position-insensitive
// s-expression renderer, so front-end position differences are ignored.
func equiv(t *testing.T, name, hclSrc, ppSrc string) {
	t.Helper()
	gotProg, err := hcl.Parse(hclSrc)
	if err != nil {
		t.Fatalf("%s: hcl.Parse error: %v", name, err)
	}
	wantProg, err := puppet.Parse(ppSrc)
	if err != nil {
		t.Fatalf("%s: puppet.Parse error: %v", name, err)
	}
	got, want := ast.Sexpr(gotProg), ast.Sexpr(wantProg)
	if got != want {
		t.Errorf("%s:\n  hcl  = %s\n  pp   = %s", name, got, want)
	}
}

// TestEquivalence checks that every mainstream construct maps to the same AST
// as its hand-written Puppet twin.
func TestEquivalence(t *testing.T) {
	cases := []struct{ name, hcl, pp string }{
		{"int", `x = 1`, `$x = 1`},
		{"float", `x = 1.5`, `$x = 1.5`},
		{"bool", `x = true`, `$x = true`},
		{"null", `x = null`, `$x = undef`},
		{"string", `x = "hi"`, `$x = 'hi'`},
		{"interp-local", `x = "a ${local.n} b"`, `$x = "a ${n} b"`},
		{"interp-only", `x = "${local.n}"`, `$x = "${n}"`},
		{"escapes", `x = "a\nb\tc\rd\"e\\f"`, `$x = "a\nb\tc\rd\"e\\f"`},
		{"unicode-u", `x = "\u0041z"`, `$x = 'Az'`},
		{"unicode-U", `x = "\U00000041z"`, `$x = 'Az'`},
		{"dollar-escape", `x = "a $${b} c"`, `$x = 'a ${b} c'`},
		{"pct-escape", `x = "a %%{b} c"`, `$x = 'a %{b} c'`},
		{"locals", "locals {\n  a = 1\n  b = \"x\"\n}", "$a = 1\n$b = 'x'"},
		{"root-and-block", "g = 7\nlocals {\n  a = 1\n}", "$g = 7\n$a = 1"},
		{"resource", "resource \"file\" \"app\" {\n  ensure = \"present\"\n  mode   = \"0644\"\n}",
			`file { 'app': ensure => 'present', mode => '0644' }`},
		{"resource-empty", `resource "file" "app" {}`, `file { 'app': }`},
		{"resource-ref-attr", `resource "file" "app" { require = resource.package.app }`,
			`file { 'app': require => Package['app'] }`},
		{"resource-ref-index", `x = resource.file["app"]`, `$x = File['app']`},
		{"array", `x = [1, 2, 3]`, `$x = [1, 2, 3]`},
		{"array-empty", `x = []`, `$x = []`},
		{"hash", `x = { a = 1, b = 2 }`, `$x = { 'a' => 1, 'b' => 2 }`},
		{"hash-quoted-key", `x = { "a" = 1 }`, `$x = { 'a' => 1 }`},
		{"binary-add", `x = 1 + 2`, `$x = 1 + 2`},
		{"binary-and", `x = local.a && local.b`, `$x = $a and $b`},
		{"binary-or", `x = local.a || local.b`, `$x = $a or $b`},
		{"comparison", `x = 1 <= 2`, `$x = 1 <= 2`},
		{"unary-neg", `x = -local.n`, `$x = -$n`},
		{"unary-not", `x = !local.b`, `$x = !$b`},
		{"index", `x = local.list[0]`, `$x = $list[0]`},
		{"heredoc", "x = <<-EOT\nhi ${local.n}\nEOT", `$x = "hi ${n}\n"`},
	}
	for _, c := range cases {
		equiv(t, c.name, c.hcl, c.pp)
	}
}

// sexpr asserts hcl.Parse succeeds and renders to the expected s-expression.
// Used for scanner edge cases whose Puppet twin would stress the .pp lexer's
// interpolation nesting rather than this front-end.
func sexpr(t *testing.T, name, hclSrc, want string) {
	t.Helper()
	prog, err := hcl.Parse(hclSrc)
	if err != nil {
		t.Fatalf("%s: hcl.Parse error: %v", name, err)
	}
	if got := ast.Sexpr(prog); got != want {
		t.Errorf("%s:\n  got  = %s\n  want = %s", name, got, want)
	}
}

// TestTemplateScanner exercises the interpolation brace/quote matcher and the
// escape decoder on inputs whose Puppet twin is awkward to write.
func TestTemplateScanner(t *testing.T) {
	sexpr(t, "interp-object", `x = "v${ {a = 1} }"`,
		`(program [(= $x (concat "v" (hash ("a" => 1))))])`)
	sexpr(t, "interp-nested-string", `x = "v${ "hi" }"`,
		`(program [(= $x (concat "v" "hi"))])`)
	sexpr(t, "escaped-quote-in-string", `x = "v${ "a\"" }"`,
		`(program [(= $x (concat "v" "a\""))])`)
	// hex-escape digit classes a-f and A-F
	sexpr(t, "hex-lower", `x = "\u006aY"`, `(program [(= $x "jY")])`)
	sexpr(t, "hex-upper", `x = "\u004AY"`, `(program [(= $x "JY")])`)
}

// parseErr asserts hcl.Parse fails with an error containing substr.
func parseErr(t *testing.T, name, hclSrc, substr string) {
	t.Helper()
	_, err := hcl.Parse(hclSrc)
	if err == nil {
		t.Fatalf("%s: expected error, got nil", name)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("%s: error %q does not contain %q", name, err.Error(), substr)
	}
}

// TestErrors covers HCL2 parse-error propagation and every v0.2 "unsupported"
// path.
func TestErrors(t *testing.T) {
	cases := []struct{ name, hcl, substr string }{
		// propagated HCL2 parse errors
		{"malformed", `x = )`, "expected"},
		{"embedded-parse-error", `x = "${ 1 + }"`, "expected"},
		// blocks
		{"unknown-block", "variable \"x\" {\n}", `block type "variable"`},
		{"locals-label", "locals \"x\" {\n  a = 1\n}", "labels on a locals block"},
		{"locals-nested-block", "locals {\n  foo {\n  }\n}", "nested block inside locals"},
		{"locals-attr-error", "locals {\n  k = foo()\n}", "unsupported in HCL2 v0.1"},
		{"resource-one-label", "resource \"file\" {\n}", "TYPE and TITLE"},
		{"resource-three-labels", "resource \"a\" \"b\" \"c\" {\n}", "TYPE and TITLE"},
		{"resource-nested-block", "resource \"file\" \"a\" {\n  x {\n  }\n}", "nested block inside a resource"},
		{"resource-op-error", "resource \"file\" \"a\" {\n  m = foo()\n}", "unsupported in HCL2 v0.1"},
		// expression kinds staged for v0.2
		{"call", `x = foo()`, "hcl2.CallExpr"},
		{"var-bare", `x = bare`, "hcl2.VarExpr"},
		{"cond", `x = local.c ? 1 : 2`, "hcl2.CondExpr"},
		{"for-tuple", `x = [for v in local.l : v]`, "hcl2.ForTupleExpr"},
		{"for-object", `x = {for k, v in local.m : k => v}`, "hcl2.ForObjectExpr"},
		// traversal roots
		{"unknown-root", `x = var.region`, "traversal root"},
		// error propagation through composite expressions
		{"array-elem-error", `x = [foo()]`, "unsupported in HCL2 v0.1"},
		{"object-key-error", `x = { (foo()) = 1 }`, "unsupported in HCL2 v0.1"},
		{"object-val-error", `x = { a = foo() }`, "unsupported in HCL2 v0.1"},
		{"binary-left-error", `x = foo() + 1`, "unsupported in HCL2 v0.1"},
		{"binary-right-error", `x = 1 + foo()`, "unsupported in HCL2 v0.1"},
		{"unary-error", `x = -foo()`, "unsupported in HCL2 v0.1"},
		{"index-coll-error", `x = foo()[0]`, "unsupported in HCL2 v0.1"},
		{"index-idx-error", `x = local.list[foo()]`, "unsupported in HCL2 v0.1"},
		{"resource-index-error", `x = resource.file[foo()]`, "unsupported in HCL2 v0.1"},
		// template edge cases
		{"template-directive", `x = "%{ if true }a%{ endif }"`, "template directive"},
		{"strip-marker", `x = "${~ local.x ~}"`, "strip marker"},
		{"unterminated-interp", "x = <<-EOT\n${ local.x\nEOT", "unterminated"},
		{"unknown-escape", `x = "a\qb"`, "unknown escape"},
		{"truncated-unicode", `x = "a\u12"`, "truncated unicode"},
		{"bad-unicode-digit", `x = "a\uzzzz"`, "invalid unicode escape digit"},
	}
	for _, c := range cases {
		parseErr(t, c.name, c.hcl, c.substr)
	}
}
