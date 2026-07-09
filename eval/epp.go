// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"

	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/parser"
)

// TemplateLoader resolves an EPP/ERB template reference (e.g.
// "apache/vhost.epp") to its raw source text. Wire one via [WithTemplateLoader]
// so epp()/template() can load files. Without a loader, only the inline_*
// forms work.
type TemplateLoader interface {
	Load(name string) (string, error)
}

// WithTemplateLoader wires a template loader for epp()/template().
func WithTemplateLoader(l TemplateLoader) Option {
	return func(e *Evaluator) { e.templates = l }
}

// renderEPP compiles an EPP template to a Puppet program that emits its output
// through render builtins, then evaluates it with params (and, optionally, the
// top scope's variables) bound, returning the rendered string.
//
// The compilation strategy mirrors Puppet::Pops: literal text becomes a render
// call, `<%= expr %>` becomes a render-stringify call, and `<% code %>` is
// emitted verbatim so that control flow (`if`, `each`, …) spanning template
// segments works exactly as in a manifest.
func (e *Evaluator) renderEPP(template string, params map[string]any, pos ast.Position) (Value, error) {
	src, declared, err := compileEPP(template)
	if err != nil {
		return nil, &Error{Pos: pos, Msg: "epp: " + err.Error()}
	}
	prog, err := parser.Parse(src)
	if err != nil {
		return nil, &Error{Pos: pos, Msg: "epp: " + err.Error()}
	}
	scope := newScope(e.top)
	if err := e.bindTemplateParams(scope, declared, params, pos); err != nil {
		return nil, err
	}
	var buf strings.Builder
	e.eppStack = append(e.eppStack, &buf)
	_, evalErr := e.evalBody(prog.Body, scope)
	e.eppStack = e.eppStack[:len(e.eppStack)-1]
	if evalErr != nil {
		return nil, evalErr
	}
	return buf.String(), nil
}

// bindTemplateParams binds an EPP template's declared parameters (if any) from
// the supplied hash, applying defaults and type checks; when the template
// declares no parameters, every supplied key is bound as a variable.
func (e *Evaluator) bindTemplateParams(scope *Scope, declared []ast.Parameter, params map[string]any, pos ast.Position) error {
	if declared == nil {
		for k, v := range params {
			scope.setForce(k, v)
		}
		return nil
	}
	for _, p := range declared {
		var val Value
		switch {
		case hasKey(params, p.Name):
			val = params[p.Name]
		case p.Default != nil:
			v, err := e.eval(p.Default, scope)
			if err != nil {
				return err
			}
			val = v
		default:
			return &Error{Pos: pos, Msg: "epp: missing value for parameter $" + p.Name}
		}
		if err := e.checkParamType(p, val); err != nil {
			return positioned(err, pos)
		}
		scope.setForce(p.Name, val)
	}
	return nil
}

// eppRender appends s to the current EPP output buffer.
func (e *Evaluator) eppRender(s string) {
	if n := len(e.eppStack); n > 0 {
		e.eppStack[n-1].WriteString(s)
	}
}

// registerEPPRenderers installs the internal render builtins used by compiled
// EPP programs.
func registerEPPRenderers(e *Evaluator) {
	e.funcs["__epp_render"] = func(c *Context, args []Value, _ *Block) (Value, error) {
		for _, a := range args {
			c.e.eppRender(stringify(a))
		}
		return pcore.Undef, nil
	}
}

// segment is one lexical piece of an EPP template.
type segKind int

const (
	segText segKind = iota // literal text
	segExpr                // <%= expr %>
	segCode                // <% code %>
	segComment
)

type eppSeg struct {
	kind segKind
	text string
}

// compileEPP scans an EPP template and returns equivalent Puppet source plus the
// template's declared parameters (nil if it declares none via a `<%- |...| -%>`
// parameter tag).
func compileEPP(tmpl string) (string, []ast.Parameter, error) {
	segs, err := scanEPP(tmpl)
	if err != nil {
		return "", nil, err
	}
	var params []ast.Parameter
	var b strings.Builder
	sawContent := false // any non-whitespace output/code seen before a param tag
	for _, seg := range segs {
		switch seg.kind {
		case segText:
			if seg.text != "" {
				b.WriteString("__epp_render(")
				b.WriteString(quoteSingle(seg.text))
				b.WriteString(")\n")
			}
			if strings.TrimSpace(seg.text) != "" {
				sawContent = true
			}
		case segExpr:
			body := strings.TrimSpace(seg.text)
			if body == "" {
				continue
			}
			b.WriteString("__epp_render(")
			b.WriteString(body)
			b.WriteString(")\n")
			sawContent = true
		case segCode:
			body := strings.TrimSpace(seg.text)
			if p, ok := parseParamTag(body); ok {
				if sawContent {
					return "", nil, &Error{Msg: "parameter tag must be the first tag in the template"}
				}
				pp, perr := parser.ParseParameters(p)
				if perr != nil {
					return "", nil, perr
				}
				params = pp
				sawContent = true
				continue
			}
			b.WriteString(body)
			b.WriteString("\n")
			sawContent = true
		case segComment:
			// dropped
		}
	}
	return b.String(), params, nil
}

// parseParamTag reports whether a `<% ... %>` body is a parameter tag `|...|`
// and returns the inner parameter text.
func parseParamTag(body string) (string, bool) {
	if strings.HasPrefix(body, "|") && strings.HasSuffix(body, "|") && len(body) >= 2 {
		return body[1 : len(body)-1], true
	}
	return "", false
}

// scanEPP tokenizes an EPP template into segments, honouring `<%% %%>` literal
// escapes and the `<%-`/`-%>` whitespace-trim markers.
func scanEPP(tmpl string) ([]eppSeg, error) {
	var segs []eppSeg
	var text strings.Builder
	i := 0
	for i < len(tmpl) {
		if strings.HasPrefix(tmpl[i:], "<%%") {
			text.WriteString("<%")
			i += 3
			continue
		}
		if strings.HasPrefix(tmpl[i:], "<%") {
			trimLeft := i+2 < len(tmpl) && tmpl[i+2] == '-'
			if trimLeft {
				trimTrailingInlineSpace(&text)
			}
			kind, start := classifyTag(tmpl, i)
			end, trimRight, err := findTagEnd(tmpl, start)
			if err != nil {
				return nil, err
			}
			segs = append(segs, eppSeg{kind: segText, text: text.String()})
			text.Reset()
			segs = append(segs, eppSeg{kind: kind, text: tmpl[start:end]})
			i = skipTagClose(tmpl, end, trimRight)
			continue
		}
		text.WriteByte(tmpl[i])
		i++
	}
	segs = append(segs, eppSeg{kind: segText, text: text.String()})
	return segs, nil
}

// classifyTag determines the tag kind at position i (pointing at "<%") and
// returns the kind and the index where the tag body starts.
func classifyTag(tmpl string, i int) (segKind, int) {
	j := i + 2
	if j < len(tmpl) && tmpl[j] == '-' {
		j++
	}
	if j < len(tmpl) {
		switch tmpl[j] {
		case '=':
			return segExpr, j + 1
		case '#':
			return segComment, j + 1
		}
	}
	return segCode, j
}

// findTagEnd locates the `%>` (or `-%>`) closing the tag whose body starts at
// start, returning the body-end index and whether a right-trim was requested.
func findTagEnd(tmpl string, start int) (end int, trimRight bool, err error) {
	for k := start; k < len(tmpl); k++ {
		if strings.HasPrefix(tmpl[k:], "%%>") {
			k += 2 // skip the whole escaped %%> (loop k++ adds the third byte)
			continue
		}
		if strings.HasPrefix(tmpl[k:], "-%>") {
			return k, true, nil
		}
		if strings.HasPrefix(tmpl[k:], "%>") {
			return k, false, nil
		}
	}
	return 0, false, &Error{Msg: "unterminated EPP tag"}
}

// skipTagClose returns the index just past the tag's closing marker, consuming a
// single trailing newline when the tag requested a right-trim.
func skipTagClose(tmpl string, end int, trimRight bool) int {
	if trimRight {
		i := end + 3 // past -%>
		if i < len(tmpl) && tmpl[i] == '\r' {
			i++
		}
		if i < len(tmpl) && tmpl[i] == '\n' {
			i++
		}
		return i
	}
	return end + 2 // past %>
}

// trimTrailingInlineSpace removes trailing spaces/tabs (up to and including a
// preceding newline's indentation) from the accumulated text, implementing the
// `<%-` left-trim marker.
func trimTrailingInlineSpace(text *strings.Builder) {
	s := text.String()
	trimmed := strings.TrimRight(s, " \t")
	if trimmed != s {
		text.Reset()
		text.WriteString(trimmed)
	}
}

// quoteSingle renders s as a Puppet single-quoted string literal (only `\` and
// `'` need escaping inside single quotes).
func quoteSingle(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		default:
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('\'')
	return b.String()
}
