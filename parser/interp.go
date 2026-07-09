// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parser

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/go-puppet/puppet/ast"
)

// interpolate expands a double-quoted / interpolating-heredoc body (raw, still
// containing escapes and `$`/`${}` interpolations) into a [ast.String] (when
// fully static) or an [ast.Concat] of literal and expression parts.
func interpolate(raw string, pos ast.Position) (ast.Node, error) {
	rs := []rune(raw)
	var parts []ast.Node
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			parts = append(parts, &ast.String{Base: base(pos), Value: buf.String()})
			buf.Reset()
		}
	}
	i := 0
	for i < len(rs) {
		r := rs[i]
		switch {
		case r == '\\':
			i = appendEscape(&buf, rs, i)
		case r == '$':
			node, ni, err := scanInterp(rs, i, pos)
			if err != nil {
				return nil, err
			}
			if node == nil {
				buf.WriteByte('$')
				i++
				continue
			}
			flush()
			parts = append(parts, node)
			i = ni
		default:
			buf.WriteRune(r)
			i++
		}
	}
	flush()
	if len(parts) == 0 {
		return &ast.String{Base: base(pos), Value: ""}, nil
	}
	if len(parts) == 1 {
		if s, ok := parts[0].(*ast.String); ok {
			return s, nil
		}
	}
	return &ast.Concat{Base: base(pos), Parts: parts}, nil
}

// appendEscape processes the escape starting at rs[i] (an '\\'), writing the
// decoded rune(s) to buf, and returns the index just past the escape.
func appendEscape(buf *strings.Builder, rs []rune, i int) int {
	if i+1 >= len(rs) {
		buf.WriteByte('\\')
		return i + 1
	}
	switch rs[i+1] {
	case 'n':
		buf.WriteByte('\n')
	case 't':
		buf.WriteByte('\t')
	case 'r':
		buf.WriteByte('\r')
	case 's':
		buf.WriteByte(' ')
	case '$', '"', '\'', '\\':
		buf.WriteRune(rs[i+1])
	case 'u':
		return appendUnicode(buf, rs, i)
	default:
		buf.WriteByte('\\')
		buf.WriteRune(rs[i+1])
	}
	return i + 2
}

// appendUnicode decodes `\uXXXX` or `\u{...}` at rs[i], returning the next
// index; an ill-formed escape is emitted literally.
func appendUnicode(buf *strings.Builder, rs []rune, i int) int {
	j := i + 2
	if j < len(rs) && rs[j] == '{' {
		k := j + 1
		for k < len(rs) && rs[k] != '}' {
			k++
		}
		if k < len(rs) {
			if cp, err := strconv.ParseInt(string(rs[j+1:k]), 16, 32); err == nil {
				buf.WriteRune(rune(cp))
				return k + 1
			}
		}
		buf.WriteString(`\u`)
		return i + 2
	}
	if j+4 <= len(rs) {
		if cp, err := strconv.ParseInt(string(rs[j:j+4]), 16, 32); err == nil {
			buf.WriteRune(rune(cp))
			return j + 4
		}
	}
	buf.WriteString(`\u`)
	return i + 2
}

// scanInterp handles a `$` at rs[i]. It returns the embedded node and the next
// index, or (nil, i+1, nil) when the `$` is not an interpolation (a literal
// dollar).
func scanInterp(rs []rune, i int, pos ast.Position) (ast.Node, int, error) {
	if i+1 >= len(rs) {
		return nil, i + 1, nil
	}
	if rs[i+1] == '{' {
		return scanBraced(rs, i, pos)
	}
	if isBareVarStart(rs, i+1) {
		name, ni := scanBareName(rs, i+1)
		return &ast.Variable{Base: base(pos), Name: name}, ni, nil
	}
	return nil, i + 1, nil
}

// scanBraced handles `${ ... }`, choosing variable-vs-expression semantics the
// way Puppet does: a leading bareword is a variable unless it is immediately
// called (`${f(...)}`).
func scanBraced(rs []rune, i int, pos ast.Position) (ast.Node, int, error) {
	depth := 0
	var quote rune
	j := i + 1 // at '{'
	start := j + 1
	for j < len(rs) {
		r := rs[j]
		if quote != 0 {
			if r == '\\' {
				j += 2
				continue
			}
			if r == quote {
				quote = 0
			}
			j++
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				inner := string(rs[start:j])
				node, err := embedExpr(inner, pos)
				return node, j + 1, err
			}
		}
		j++
	}
	return nil, 0, &Error{Pos: pos, Msg: "unterminated ${...} interpolation"}
}

// embedExpr parses the inside of `${...}`.
func embedExpr(inner string, pos ast.Position) (ast.Node, error) {
	trimmed := strings.TrimSpace(inner)
	if trimmed == "" {
		return nil, &Error{Pos: pos, Msg: "empty ${} interpolation"}
	}
	src := trimmed
	if leadingBarewordIsVariable(trimmed) || allDigits(trimmed) {
		// A bare leading word, or a purely numeric body (a `${0}`..`${n}` match
		// variable), is read as a variable reference.
		src = "$" + trimmed
	}
	return ParseExpression(src)
}

// leadingBarewordIsVariable reports whether a `${...}` body begins with a
// bareword that should be read as a variable reference (i.e. it is not an
// immediate function call and does not already start with `$`).
func leadingBarewordIsVariable(s string) bool {
	r := []rune(s)
	if len(r) == 0 || r[0] == '$' {
		return false
	}
	// Only a lower-case (or underscore) leading word is an implicit variable;
	// an upper-case word is a type or resource reference (e.g. ${File['x']}).
	if !(unicode.IsLower(r[0]) || r[0] == '_') {
		return false
	}
	k := 0
	for k < len(r) {
		if r[k] == '_' || unicode.IsLetter(r[k]) || unicode.IsDigit(r[k]) {
			k++
		} else if r[k] == ':' && k+1 < len(r) && r[k+1] == ':' {
			k += 2
		} else {
			break
		}
	}
	// Skip spaces after the bareword.
	for k < len(r) && (r[k] == ' ' || r[k] == '\t') {
		k++
	}
	// A following '(' means a function call; anything else keeps it a variable.
	return k >= len(r) || r[k] != '('
}

// allDigits reports whether s is a run of ASCII digits. It is only called by
// embedExpr on an already non-empty, trimmed body.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isBareVarStart(rs []rune, i int) bool {
	if i >= len(rs) {
		return false
	}
	r := rs[i]
	if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	return r == ':' && i+1 < len(rs) && rs[i+1] == ':'
}

// scanBareName reads a bare `$name` variable name (letters/digits/_ and `::`
// namespace separators) starting at rs[i], returning the name and next index.
func scanBareName(rs []rune, i int) (string, int) {
	var b strings.Builder
	for i < len(rs) {
		r := rs[i]
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			i++
		} else if r == ':' && i+1 < len(rs) && rs[i+1] == ':' {
			b.WriteString("::")
			i += 2
		} else {
			break
		}
	}
	return b.String(), i
}
