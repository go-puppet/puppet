// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parser

import (
	"strings"
	"testing"

	"github.com/go-puppet/puppet/ast"
)

// ok parses src and returns the s-expression of the resulting program.
func ok(t *testing.T, src string) string {
	t.Helper()
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", src, err)
	}
	return ast.Sexpr(prog)
}

func TestParsePrograms(t *testing.T) {
	cases := []struct{ src, want string }{
		// arithmetic & precedence
		{`$x = 1 + 2 * 3`, `(program [(= $x (+ 1 (* 2 3)))])`},
		{`$x = (1 + 2) * 3`, `(program [(= $x (* (+ 1 2) 3))])`},
		{`$x = 10 % 3 / 2`, `(program [(= $x (/ (% 10 3) 2))])`},
		{`$x = 10 / 2`, `(program [(= $x (/ 10 2))])`},
		{`$x = [1] << 2`, `(program [(= $x (<< (array 1) 2))])`},
		{`$x = -$a`, `(program [(= $x (- $a))])`},
		{`$x = !$a`, `(program [(= $x (! $a))])`},
		{`$x += 1`, `(program [(+= $x 1)])`},
		{`$x -= 1`, `(program [(-= $x 1)])`},
		// comparison, boolean, match, in
		{`$x = 1 < 2`, `(program [(= $x (< 1 2))])`},
		{`$x = 1 <= 2`, `(program [(= $x (<= 1 2))])`},
		{`$x = 1 >= 2`, `(program [(= $x (>= 1 2))])`},
		{`$x = 1 > 2`, `(program [(= $x (> 1 2))])`},
		{`$x = 1 == 2`, `(program [(= $x (== 1 2))])`},
		{`$x = 1 != 2`, `(program [(= $x (!= 1 2))])`},
		{`$x = $a and $b or $c`, `(program [(= $x (or (and $a $b) $c))])`},
		{`$x = $s =~ /a/`, `(program [(= $x (=~ $s /a/))])`},
		{`$x = $s !~ /a/`, `(program [(= $x (!~ $s /a/))])`},
		{`$x = 'a' in ['a']`, `(program [(= $x (in "a" (array "a")))])`},
		// literals
		{`$x = [true, false, undef, default]`, `(program [(= $x (array true false undef default))])`},
		{`$x = 0755`, `(program [(= $x 0755)])`},
		{`$x = 0xFF`, `(program [(= $x 0xff)])`},
		{`$x = 1.5e3`, `(program [(= $x 1500)])`},
		{`$x = 'a\'b\\c'`, `(program [(= $x "a'b\\c")])`},
		{`$x = $::hostname`, `(program [(= $x $::hostname)])`},
		{`$x = ::Foo::Bar`, `(program [(= $x ::Foo::Bar)])`},
		{`$x = ::ntp`, `(program [(= $x ::ntp)])`},
		{"# line\n/* block */ $x = 1", `(program [(= $x 1)])`},
		{`/abc/`, `(program [/abc/])`},
		// interpolation
		{`$s = "hello ${name} and ${1+2} end"`, `(program [(= $s (concat "hello " $name " and " (+ 1 2) " end"))])`},
		{`$g = "greet $user!"`, `(program [(= $g (concat "greet " $user "!"))])`},
		{`$s = "${fn($x)}"`, `(program [(= $s (concat (call fn $x)))])`},
		{`$s = "${x.upcase}"`, `(program [(= $s (concat (. $x upcase)))])`},
		{`$s = "cost \$5\n\tA\u{42}"`, `(program [(= $s "cost $5\n\tAB")])`},
		{`$s = "plain"`, `(program [(= $s "plain")])`},
		{`$s = "$0"`, `(program [(= $s (concat $0))])`},
		{`$s = "a $ b"`, `(program [(= $s "a $ b")])`},
		// access, method, call
		{`$x = $a[1, 2]`, `(program [(= $x (access $a 1 2))])`},
		{`$x = $a.upcase`, `(program [(= $x (. $a upcase))])`},
		{`$x = $a.Foo`, `(program [(= $x (. $a Foo))])`},
		{`$x = foo(1, 2)`, `(program [(= $x (call foo 1 2))])`},
		{`[1,2,3].each |$i| { notice($i) }`, `(program [(. (array 1 2 3) each (lambda (params ($i)) [(call notice $i)]))])`},
		{`$x = $a.map |$e| { $e }`, `(program [(= $x (. $a map (lambda (params ($e)) [$e])))])`},
		// selector
		{`$y = $z ? { 'a' => 1, default => 0 }`, `(program [(= $y (? $z ("a" => 1) (default => 0)))])`},
		// resources
		{`file { '/tmp/x': ensure => present, mode => '0644' }`,
			`(program [(resource file (body "/tmp/x" (ensure => present) (mode => "0644")))])`},
		{`file { 'a': ; 'b': mode => '0644' }`,
			`(program [(resource file (body "a") (body "b" (mode => "0644")))])`},
		{`file { 'x': * => $attrs }`, `(program [(resource file (body "x" (* => $attrs)))])`},
		{`Package { ensure => installed }`, `(program [(defaults Package (ensure => installed))])`},
		{`File['x'] { mode => '0600' }`, `(program [(override (access File "x") (mode => "0600"))])`},
		{`File['x'] { tag +> 'a' }`, `(program [(override (access File "x") (tag +> "a"))])`},
		{`@file { 'x': }`, `(program [(resource@ file (body "x"))])`},
		{`@@nagios_service { 'svc': }`, `(program [(resource@@ nagios_service (body "svc"))])`},
		{`class { 'ntp': server => 'a' }`, `(program [(resource class (body "ntp" (server => "a")))])`},
		// collectors
		{`File <| tag == 'web' |>`, `(program [(collect File (== tag "web"))])`},
		{`File <| |>`, `(program [(collect File)])`},
		{`Nagios_service <<| |>>`, `(program [(collect-exported Nagios_service)])`},
		// relationships
		{`Package['n'] -> Service['n']`, `(program [(-> (access Package "n") (access Service "n"))])`},
		{`A['a'] ~> B['b']`, `(program [(~> (access A "a") (access B "b"))])`},
		{`A['a'] <- B['b']`, `(program [(<- (access A "a") (access B "b"))])`},
		{`A['a'] <~ B['b']`, `(program [(<~ (access A "a") (access B "b"))])`},
		// conditionals
		{`if $a == 1 { notice('one') } elsif $b { notice('b') } else { notice('c') }`,
			`(program [(if (== $a 1) [(call notice "one")] else [(if $b [(call notice "b")] else [(call notice "c")])])])`},
		{`if $a { notice(1) }`, `(program [(if $a [(call notice 1)])])`},
		{`unless $a { notice(1) } else { notice(2) }`,
			`(program [(unless $a [(call notice 1)] else [(call notice 2)])])`},
		{`unless $a { notice(1) }`, `(program [(unless $a [(call notice 1)])])`},
		{`case $os { 'debian', 'ubuntu': { include apt } default: { fail('no') } }`,
			`(program [(case $os (when "debian" "ubuntu" : [(call include apt)]) (when default : [(call fail "no")]))])`},
		// definitions
		{`class web(String $vhost = 'x', Integer[1,10] $n) inherits base { }`,
			`(program [(class web (params (String $vhost = "x") ((access Integer 1 10) $n)) inherits base [])])`},
		{`class simple { }`, `(program [(class simple [])])`},
		{`define site($p) { file { $p: ensure => directory } }`,
			`(program [(define site (params ($p)) [(resource file (body $p (ensure => directory)))])])`},
		{`define d(*$rest) { }`, `(program [(define d (params (*$rest)) [])])`},
		{`node 'web1', /^db/ { include base }`,
			`(program [(node "web1" /^db/ [(call include base)])])`},
		{`node default { }`, `(program [(node default [])])`},
		{`function f(Integer $a) >> Integer { $a }`,
			`(program [(function f (params (Integer $a)) >> Integer [$a])])`},
		{`function g() { 1 }`, `(program [(function g [1])])`},
		// statement calls
		{`include ntp, apache`, `(program [(call include ntp apache)])`},
		{`notice "hi"`, `(program [(call notice "hi")])`},
		{`each($xs) |$x| { notice($x) }`, `(program [(call each $xs (lambda (params ($x)) [(call notice $x)]))])`},
		{`create_resources($xs) |$k| { }`, `(program [(call create_resources $xs (lambda (params ($k)) []))])`},
		// heredocs
		{"$h = @(END)\n  line one $x\n  | END", `(program [(= $h (heredoc "" "line one $x\n"))])`},
		{"$h = @(\"END\")\n  hi $name\n  | END", `(program [(= $h (heredoc "" (concat "hi " $name "\n")))])`},
		{"$h = @(END:json)\n  {}\n  END", `(program [(= $h (heredoc "json" "  {}\n"))])`},
		{"$h = @(END/n)\n  a\n  | END", `(program [(= $h (heredoc "" "a\n"))])`},
		{"$h = @(END)\n  a\n  |- END", `(program [(= $h (heredoc "" "a"))])`},
		{"$x = @(END)\n  body\n  END\n$y = 2", `(program [(= $x (heredoc "" "  body\n")) (= $y 2)])`},
		{"$x = @(END) + 'z'\n  body\n  END", `(program [(= $x (+ (heredoc "" "  body\n") "z"))])`},
		// bare statement name
		{`foo`, `(program [foo])`},
		{`;; $x = 1 ;;`, `(program [(= $x 1)])`},
		// misc branch coverage
		{`$x = 0X1f`, `(program [(= $x 0x1f)])`},
		{`foo(*$args)`, `(program [(call foo (* $args))])`},
		{`Foo <<| tag == 'x' |>>`, `(program [(collect-exported Foo (== tag "x"))])`},
		{`$v { 'a' => 1 }`, `(program [$v (hash ("a" => 1))])`},
		// whitespace-sensitive '[': after a type reference it still indexes;
		// after a non-reference a spaced '[' starts a new statement.
		{`$x = Foo ['bar']`, `(program [(= $x (access Foo "bar"))])`},
		{`$x = $arr [1]`, `(program [(= $x $arr) (array 1)])`},
	}
	for _, tc := range cases {
		if got := ok(t, tc.src); got != tc.want {
			t.Errorf("Parse(%q)\n  got  %s\n  want %s", tc.src, got, tc.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct{ src, contains string }{
		{`$x = `, "unexpected EOF"},
		{`1 +`, "unexpected EOF"},
		{`[1, 2`, "expected ]"},
		{`{ 'a' => }`, "unexpected"},
		{`{ 'a' 1 }`, "expected =>"},
		{`foo(1`, "expected )"},
		{`(1`, "expected )"},
		{`$x[]`, "empty [] access"},
		{`$x ? { 'a' => }`, "unexpected"},
		{`$x ? 1`, "expected {"},
		{`$x ? { 'a' 1 }`, "expected =>"},
		{`if $x notice(1)`, "expected {"},
		{`case $x { : { } }`, "at least one match value"},
		{`case $x { 'a' { } }`, "expected :"},
		{`case $x 'a'`, "expected {"},
		{`file { 'x' mode => 1 }`, "expected :"},
		{`file { 'x': mode 1 }`, "expected => or +>"},
		{`file { 'x': 1 => 1 }`, "expected NAME"},
		{`file { 'x': mode => 1 `, "expected }"},
		{`.foo`, "unexpected ."},
		{`$x.`, "expected method name"},
		{`$x.5`, "expected method name"},
		{`@ 1`, "expected NAME"},
		{`@foo bar`, "expected '{'"},
		{`class`, "expected NAME"},
		{`class foo`, "expected {"},
		{`class foo(`, "expected )"},
		{`class foo inherits { }`, "expected NAME"},
		{`define`, "expected NAME"},
		{`node { }`, "at least one matcher"},
		{`function`, "expected NAME"},
		{`3(1)`, "cannot call"},
		{`File <| tag == 'x'`, "expected |>"},
		{`Nagios <<| tag == 'x' |>`, "expected |>>"},
		{`class foo(Integer) { }`, "expected VARIABLE"},
		// method calls
		{`$x.foo(1`, "expected )"},
		{`$x.foo(1 +)`, "unexpected"},
		{`$x.foo() |$a { }`, "expected |"},
		{`$x.foo() |$a|`, "expected {"},
		// access
		{`$x[1`, "expected ]"},
		{`$x[1 +]`, "unexpected"},
		// hash
		{`$h = { => 1 }`, "unexpected"},
		{`$h = { 'a' => 1 `, "expected }"},
		{`$h = { 'a' => 1, 'b' }`, "expected =>"},
		// array
		{`$a = [1 +]`, "unexpected"},
		// calls / lambdas
		{`foo(1 +)`, "unexpected"},
		{`foo() |$a { }`, "expected |"},
		{`foo() |Integer| { }`, "expected VARIABLE"},
		{`foo() |$a| notice()`, "expected {"},
		// selector detail
		{`$x ? { 'a' 1 => 2 }`, "expected =>"},
		{`$x ? { 'a' => 1 `, "expected }"},
		// unless / if bodies
		{`unless $x notice(1)`, "expected {"},
		{`unless $x { notice(1) `, "expected }"},
		{`unless $x { } else notice()`, "expected {"},
		{`if $x { } elsif $y notice()`, "expected {"},
		{`if $x { } else notice()`, "expected {"},
		// case detail
		{`case $x { 'a': notice() }`, "expected {"},
		{`case $x { 'a': { } `, "expected }"},
		// resource / defaults / override detail
		{`Package { ensure installed }`, "expected => or +>"},
		{`Package { ensure => installed `, "expected }"},
		{`File['x'] { mode }`, "expected => or +>"},
		{`File['x'] { mode => '0644' `, "expected }"},
		{`file { 'x': mode => '0644' ; 'y' }`, "expected :"},
		// definitions detail
		{`define d($x = ) { }`, "unexpected"},
		{`define d() notice()`, "expected {"},
		{`node 'n' notice()`, "expected {"},
		{`function f() >> Integer[ { }`, "expected ]"},
		{`function f() notice()`, "expected {"},
		{`class c { notice() `, "expected }"},
		// data type in param
		{`class c(Integer[1) { }`, "expected ]"},
		// unary operand error
		{`$x = -`, "unexpected EOF"},
		// conditional / case condition errors
		{`if )`, "unexpected"},
		{`unless )`, "unexpected"},
		{`case )`, "unexpected"},
		{`case $x { ) : { } }`, "unexpected"},
		{`case $x { 'a': { } `, "expected }"},
		{`class c { notice() `, "expected }"},
		// resource body / attribute errors
		{`file { : }`, "unexpected"},
		{`file { 'x': mode => }`, "unexpected"},
		{`file { 'x': * x }`, "expected =>"},
		// node / function definition errors
		{`node ) { }`, "unexpected"},
		{`function f(Integer) { }`, "expected VARIABLE"},
		{`function f() >> ) { }`, "unexpected"},
		// relationship / selector / collector errors
		{`A['a'] -> )`, "unexpected"},
		{`$x ? { ) => 1 }`, "unexpected"},
		{`$v <| |>`, "unexpected <|"},
		// integer overflow
		{`$x = 99999999999999999999`, "out of range"},
		// statement-call trailing lambda error
		{`notice x |$a { }`, "expected |"},
		// statement-call argument error
		{`notice 1 +`, "unexpected"},
		// block body statement error propagation
		{`class c { @ }`, "expected NAME"},
		// grouping inner-expression error
		{`(1 +)`, "unexpected"},
		// collector query error
		{`File <| tag == |>`, "unexpected"},
		// heredoc interpolation error
		{"$x = @(\"END\")\n  ${\n  | END", "unterminated ${...}"},
	}
	for _, tc := range cases {
		_, err := Parse(tc.src)
		if err == nil {
			t.Errorf("Parse(%q): expected error containing %q, got nil", tc.src, tc.contains)
			continue
		}
		if !strings.Contains(err.Error(), tc.contains) {
			t.Errorf("Parse(%q): error %q does not contain %q", tc.src, err.Error(), tc.contains)
		}
	}
}

func TestParseExpression(t *testing.T) {
	n, err := ParseExpression(`1 + 2`)
	if err != nil {
		t.Fatalf("ParseExpression error: %v", err)
	}
	if got := ast.Sexpr(n); got != `(+ 1 2)` {
		t.Errorf("got %s", got)
	}
	if _, err := ParseExpression(`1 2`); err == nil {
		t.Error("expected trailing-token error")
	}
	if _, err := ParseExpression(`1 +`); err == nil {
		t.Error("expected error")
	}
	if _, err := ParseExpression(`'unterminated`); err == nil {
		t.Error("expected lexer error")
	}
}

func TestLexErrorsThroughParser(t *testing.T) {
	cases := []struct{ src, contains string }{
		{`$x = 'abc`, "unterminated single-quoted"},
		{`$x = "abc`, "unterminated double-quoted"},
		{`$x = "abc\`, "unterminated double-quoted"},
		{`/* open`, "unterminated block comment"},
		{"$x = /ab", "unterminated regular expression"},
		{"$x = /ab\n/", "unterminated regular expression"},
		{`$x = 0x`, "malformed hexadecimal"},
		{`$x = 1e`, "malformed exponent"},
		{`$ = 1`, "expected variable name"},
		{"$x ~ 1", "unexpected character '~'"},
		{"$x = `y`", "unexpected character"},
		{"$x = @(END", "unterminated heredoc tag"},
		{"$x = @()\n", "empty heredoc tag"},
		{"$x = @(END)", "heredoc has no body"},
		{"$x = @(END)\n  body\n", "unterminated heredoc"},
	}
	for _, tc := range cases {
		_, err := Parse(tc.src)
		if err == nil {
			t.Errorf("Parse(%q): expected error containing %q, got nil", tc.src, tc.contains)
			continue
		}
		if !strings.Contains(err.Error(), tc.contains) {
			t.Errorf("Parse(%q): error %q does not contain %q", tc.src, err.Error(), tc.contains)
		}
	}
}
