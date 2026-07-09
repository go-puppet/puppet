// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

// mapLoader is a TemplateLoader backed by a map, for tests.
type mapLoader map[string]string

func (m mapLoader) Load(name string) (string, error) {
	s, ok := m[name]
	if !ok {
		return "", &Error{Msg: "template not found: " + name}
	}
	return s, nil
}

// stubERB is a trivial ERBRenderer that substitutes @vars, for testing the seam.
type stubERB struct{}

func (stubERB) RenderERB(src string, vars map[string]any) (string, error) {
	out := src
	for k, v := range vars {
		out = strings.ReplaceAll(out, "@"+k, stringify(v))
	}
	return out, nil
}

func TestInlineEppBasic(t *testing.T) {
	got := firstLog(t, `notice(inline_epp('<%- | $x | -%>
val=<%= $x %>', {'x' => 42}))`)
	if got != "val=42" {
		t.Errorf("got %q", got)
	}
}

func TestInlineEppNoParamTag(t *testing.T) {
	// Without a param tag, all supplied keys bind as variables.
	got := firstLog(t, `notice(inline_epp('a=<%= $a %> b=<%= $b %>', {'a' => 1, 'b' => 2}))`)
	if got != "a=1 b=2" {
		t.Errorf("got %q", got)
	}
}

func TestInlineEppDefaultAndTypecheck(t *testing.T) {
	got := firstLog(t, `notice(inline_epp('<%- | String $x, Integer $n = 5 | -%>
<%= $x %>:<%= $n %>', {'x' => 'hi'}))`)
	if got != "hi:5" {
		t.Errorf("got %q", got)
	}
}

func TestInlineEppControlFlow(t *testing.T) {
	got := firstLog(t, `notice(inline_epp('<%- | Array $xs | -%>
<% $xs.each |$x| { -%>
- <%= $x %>
<% } -%>
', {'xs' => ['a','b']}))`)
	if got != "- a\n- b\n" {
		t.Errorf("got %q", got)
	}
}

func TestEppLiteralEscape(t *testing.T) {
	got := firstLog(t, `notice(inline_epp('<%% literal %%> <%= 1 + 1 %>'))`)
	if got != "<% literal %%> 2" {
		t.Errorf("got %q", got)
	}
}

func TestEppComment(t *testing.T) {
	got := firstLog(t, `notice(inline_epp('a<%# nothing here %>b'))`)
	if got != "ab" {
		t.Errorf("got %q", got)
	}
}

func TestEppFromLoader(t *testing.T) {
	loader := mapLoader{"m/t.epp": "<%- | $n | -%>\nN=<%= $n %>"}
	got := firstLog(t, `notice(epp('m/t.epp', {'n' => 7}))`, WithTemplateLoader(loader))
	if got != "N=7" {
		t.Errorf("got %q", got)
	}
}

func TestEppMissingLoader(t *testing.T) {
	err := evalErr(t, `notice(epp('x/y.epp'))`)
	if !strings.Contains(err, "no template loader") {
		t.Errorf("got %q", err)
	}
}

func TestEppMissingParam(t *testing.T) {
	err := evalErr(t, `notice(inline_epp('<%- | $x | -%><%= $x %>'))`)
	if !strings.Contains(err, "missing value for parameter") {
		t.Errorf("got %q", err)
	}
}

func TestEppErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(inline_epp())`, "expects a template"},
		{`notice(inline_epp(5))`, "must be a String"},
		{`notice(inline_epp('x', 'y'))`, "must be a Hash"},
		{`notice(inline_epp('<% |$a $b| %>'))`, "unexpected"},
		{`notice(inline_epp('<% foo bar baz'))`, "unterminated EPP tag"},
		{`notice(inline_epp('a<%- | $x | -%>'))`, "must be the first tag"},
		{`notice(inline_epp('<%- | String $x | -%>', {'x' => 5}))`, "expects"},
		{`notice(epp())`, "expects a template"},
		{`notice(epp(5))`, "must be a String"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%q => %q, want substring %q", tc.src, err, tc.want)
		}
	}
}

func TestErbSeam(t *testing.T) {
	got := firstLog(t, `$greeting = 'hello'
notice(inline_template('say @greeting'))`, WithERBRenderer(stubERB{}))
	if got != "say hello" {
		t.Errorf("got %q", got)
	}
}

func TestErbFromLoader(t *testing.T) {
	loader := mapLoader{"m/t.erb": "x=@v"}
	got := firstLog(t, `$v = 9
notice(template('m/t.erb'))`, WithTemplateLoader(loader), WithERBRenderer(stubERB{}))
	if got != "x=9" {
		t.Errorf("got %q", got)
	}
}

func TestErbNoRenderer(t *testing.T) {
	err := evalErr(t, `notice(inline_template('x'))`)
	if !strings.Contains(err, "ERB templates require an ERB renderer") {
		t.Errorf("got %q", err)
	}
}

func TestErbErrors(t *testing.T) {
	cases := []struct {
		src  string
		opts []Option
		want string
	}{
		{`notice(inline_template())`, nil, "at least one template"},
		{`notice(template())`, nil, "at least one template"},
		{`notice(template(5))`, []Option{WithERBRenderer(stubERB{})}, "must be Strings"},
		{`notice(template('missing.erb'))`, []Option{WithTemplateLoader(mapLoader{}), WithERBRenderer(stubERB{})}, "template not found"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src, tc.opts...); !strings.Contains(err, tc.want) {
			t.Errorf("%q => %q, want %q", tc.src, err, tc.want)
		}
	}
}

// failERB always errors, to cover the renderer error path.
type failERB struct{}

func (failERB) RenderERB(string, map[string]any) (string, error) {
	return "", &Error{Msg: "boom"}
}

func TestErbRendererError(t *testing.T) {
	err := evalErr(t, `notice(inline_template('x'))`, WithERBRenderer(failERB{}))
	if !strings.Contains(err, "boom") {
		t.Errorf("got %q", err)
	}
}
