// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package lexer

import (
	"strings"
	"testing"
)

// dump lexes src and renders the token stream as a compact string. Value
// tokens render as kind:text; structural tokens render as their kind name
// (which is the operator glyph). This also exercises Kind.String.
func dump(t *testing.T, src string) string {
	t.Helper()
	toks, err := Lex(src)
	if err != nil {
		t.Fatalf("Lex(%q) error: %v", src, err)
	}
	var parts []string
	for _, tk := range toks {
		switch tk.Kind {
		case EOF:
		case NAME, TYPE, VARIABLE, INT, FLOAT, SQSTRING, DQSTRING, REGEXP:
			parts = append(parts, tk.Kind.String()+":"+tk.Text)
		case HEREDOC:
			parts = append(parts, "HEREDOC:"+strings.ReplaceAll(tk.Text, "\n", `\n`))
		default:
			parts = append(parts, tk.Kind.String())
		}
	}
	return strings.Join(parts, " ")
}

func TestLexTokens(t *testing.T) {
	cases := []struct{ src, want string }{
		{`$x = 1`, "VARIABLE:x = INT:1"},
		{`$a::b $::c`, "VARIABLE:a::b VARIABLE:::c"},
		{`$1`, "VARIABLE:1"},
		{`foo Bar ::Baz ::qux`, "NAME:foo TYPE:Bar TYPE:::Baz NAME:::qux"},
		{`if elsif else unless case and or in class define inherits node function`,
			"if elsif else unless case and or in class define inherits node function"},
		{`true false undef default`, "true false undef default"},
		{`type`, "NAME:type"},
		{`0 42 0755 0xFF 0X1f`, "INT:0 INT:42 INT:0755 INT:0xFF INT:0X1f"},
		{`1.5 1e5 1e+5 1.5E-2`, "FLOAT:1.5 FLOAT:1e5 FLOAT:1e+5 FLOAT:1.5E-2"},
		{`'a\'b\\c\d'`, `SQSTRING:a'b\c\d`},
		{`"raw"`, "DQSTRING:raw"},
		{`= == != =~ !~ ! < > <= >= + - * %`,
			"= == != =~ !~ ! < > <= >= + - * %"},
		{`+= -= << -> ~> <- <~ => +>`, "+= -= << -> ~> <- <~ => +>"},
		{`{ } [ ] ( ) , ; : . ?`, "{ } [ ] ( ) , ; : . ?"},
		{`@ @@ | <| |> <<| |>>`, "@ @@ | <| |> <<| |>>"},
		{`$x = /a\/b/`, `VARIABLE:x = REGEXP:a\/b`},
		{`/^start/`, "REGEXP:^start"},
		{`1 / 2`, "INT:1 / INT:2"},
		{"# comment\n$x /* b */ = 1", "VARIABLE:x = INT:1"},
	}
	for _, tc := range cases {
		if got := dump(t, tc.src); got != tc.want {
			t.Errorf("Lex(%q)\n  got  %s\n  want %s", tc.src, got, tc.want)
		}
	}
}

func TestLexHeredoc(t *testing.T) {
	toks, err := Lex("$x = @(\"EOT\":json/n)\n  hi $y\n  | EOT\n$z = 1")
	if err != nil {
		t.Fatal(err)
	}
	var h Token
	for _, tk := range toks {
		if tk.Kind == HEREDOC {
			h = tk
		}
	}
	if h.Text != "hi $y\n" || h.Syntax != "json" || !h.Interp {
		t.Errorf("heredoc = %q syntax=%q interp=%v", h.Text, h.Syntax, h.Interp)
	}
	// Ensure lexing resumes past the body: $z=1 must appear.
	if got := dump(t, "$x = @(EOT)\n  body\n  | EOT\n$z = 1"); !strings.Contains(got, "VARIABLE:z = INT:1") {
		t.Errorf("resume failed: %s", got)
	}
}

func TestLexErrors(t *testing.T) {
	cases := []struct{ src, contains string }{
		{`'abc`, "unterminated single-quoted"},
		{`"abc`, "unterminated double-quoted"},
		{"\"abc\\", "unterminated double-quoted"},
		{`/* open`, "unterminated block comment"},
		{`/abc`, "unterminated regular expression"},
		{"/abc\n/", "unterminated regular expression"},
		{`0x`, "malformed hexadecimal"},
		{`1e`, "malformed exponent"},
		{`$`, "expected variable name"},
		{`~`, "unexpected character '~'"},
		{"`", "unexpected character"},
		{"@(EOT", "unterminated heredoc tag"},
		{"@()\n", "empty heredoc tag"},
		{"@(EOT)", "heredoc has no body"},
		{"@(EOT)\n  body\n", "unterminated heredoc"},
	}
	for _, tc := range cases {
		_, err := Lex(tc.src)
		if err == nil {
			t.Errorf("Lex(%q): expected error containing %q, got nil", tc.src, tc.contains)
			continue
		}
		if !strings.Contains(err.Error(), tc.contains) {
			t.Errorf("Lex(%q): error %q does not contain %q", tc.src, err.Error(), tc.contains)
		}
		var le *Error
		if !errorAs(err, &le) {
			t.Errorf("Lex(%q): error is not *lexer.Error", tc.src)
		}
	}
}

// errorAs is a tiny local errors.As to avoid importing errors just for the test
// while still asserting the concrete error type and its Error() rendering.
func errorAs(err error, target **Error) bool {
	le, ok := err.(*Error)
	if ok {
		*target = le
		_ = le.Error()
	}
	return ok
}
