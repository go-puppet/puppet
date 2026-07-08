// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package lexer

import (
	"strings"
	"unicode"

	"github.com/go-puppet/puppet/ast"
)

func isWordStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }

// emit1/emit2/emit3 build a single/double/triple-rune operator token.
func (lx *lexer) emit(pos ast.Position, k Kind, n int) (Token, error) {
	for range n {
		lx.adv()
	}
	return Token{Kind: k, Text: kindName[k], Pos: pos}, nil
}

// lexOperator scans a punctuation or operator token using longest-match.
func (lx *lexer) lexOperator(pos ast.Position) (Token, error) {
	r := lx.peek()
	r1, r2 := lx.at(1), lx.at(2)
	switch r {
	case '{':
		return lx.emit(pos, LBrace, 1)
	case '}':
		return lx.emit(pos, RBrace, 1)
	case '[':
		return lx.emit(pos, LBrack, 1)
	case ']':
		return lx.emit(pos, RBrack, 1)
	case '(':
		return lx.emit(pos, LParen, 1)
	case ')':
		return lx.emit(pos, RParen, 1)
	case ',':
		return lx.emit(pos, Comma, 1)
	case ';':
		return lx.emit(pos, Semi, 1)
	case ':':
		return lx.emit(pos, Colon, 1)
	case '.':
		return lx.emit(pos, Dot, 1)
	case '?':
		return lx.emit(pos, Query, 1)
	case '%':
		return lx.emit(pos, Mod, 1)
	case '/':
		return lx.emit(pos, Slash, 1)
	case '*':
		return lx.emit(pos, Star, 1)
	case '@':
		switch {
		case r1 == '@':
			return lx.emit(pos, AtAt, 2)
		case r1 == '(':
			return lx.lexHeredoc(pos)
		}
		return lx.emit(pos, At, 1)
	case '=':
		switch r1 {
		case '>':
			return lx.emit(pos, FArrow, 2)
		case '=':
			return lx.emit(pos, IsEq, 2)
		case '~':
			return lx.emit(pos, Match, 2)
		}
		return lx.emit(pos, Equals, 1)
	case '+':
		switch r1 {
		case '>':
			return lx.emit(pos, PArrow, 2)
		case '=':
			return lx.emit(pos, PlusEq, 2)
		}
		return lx.emit(pos, Plus, 1)
	case '-':
		switch r1 {
		case '>':
			return lx.emit(pos, Arrow, 2)
		case '=':
			return lx.emit(pos, MinusEq, 2)
		}
		return lx.emit(pos, Minus, 1)
	case '~':
		if r1 == '>' {
			return lx.emit(pos, TildeArrow, 2)
		}
		return Token{}, &Error{Pos: pos, Msg: "unexpected character '~'"}
	case '!':
		switch r1 {
		case '~':
			return lx.emit(pos, NotMatch, 2)
		case '=':
			return lx.emit(pos, NotEq, 2)
		}
		return lx.emit(pos, Not, 1)
	case '>':
		if r1 == '=' {
			return lx.emit(pos, Ge, 2)
		}
		return lx.emit(pos, Gt, 1)
	case '<':
		switch {
		case r1 == '<' && r2 == '|':
			return lx.emit(pos, LLCollect, 3)
		case r1 == '<':
			return lx.emit(pos, LShift, 2)
		case r1 == '|':
			return lx.emit(pos, LCollect, 2)
		case r1 == '-':
			return lx.emit(pos, LArrow, 2)
		case r1 == '~':
			return lx.emit(pos, LTilde, 2)
		case r1 == '=':
			return lx.emit(pos, Le, 2)
		}
		return lx.emit(pos, Lt, 1)
	case '|':
		switch {
		case r1 == '>' && r2 == '>':
			return lx.emit(pos, RRCollect, 3)
		case r1 == '>':
			return lx.emit(pos, RCollect, 2)
		}
		return lx.emit(pos, Pipe, 1)
	}
	return Token{}, &Error{Pos: pos, Msg: "unexpected character " + string(r)}
}

// lexHeredoc scans a heredoc opener `@(TAG[:syntax])` and its body. TAG in
// double quotes enables interpolation. The body runs from the next line to a
// terminator line matching optional `|` (indent marker) / `-` (chomp) and the
// tag; the `|` marker sets the margin stripped from each body line.
func (lx *lexer) lexHeredoc(pos ast.Position) (Token, error) {
	lx.adv() // @
	lx.adv() // (
	var spec strings.Builder
	for {
		if lx.i >= len(lx.src) || lx.peek() == '\n' {
			return Token{}, &Error{Pos: pos, Msg: "unterminated heredoc tag"}
		}
		r := lx.adv()
		if r == ')' {
			break
		}
		spec.WriteRune(r)
	}
	tag, syntax, interp, err := parseHeredocSpec(spec.String(), pos)
	if err != nil {
		return Token{}, err
	}
	// Find the newline that ends the opener line.
	nl := lx.i
	for nl < len(lx.src) && lx.src[nl] != '\n' {
		nl++
	}
	if nl >= len(lx.src) {
		return Token{}, &Error{Pos: pos, Msg: "heredoc has no body"}
	}
	body, end, chomp, margin, err := scanHeredocBody(lx.src, nl+1, tag, pos)
	if err != nil {
		return Token{}, err
	}
	text := renderHeredoc(body, chomp, margin)
	lx.resume = end // jump here when the opener line's newline is consumed
	return Token{Kind: HEREDOC, Text: text, Pos: pos, Interp: interp, Syntax: syntax}, nil
}

// parseHeredocSpec parses `TAG`, `"TAG"`, `TAG:syntax`, `"TAG":syntax` (an
// optional `/escapes` suffix is accepted and ignored in v0.1).
func parseHeredocSpec(spec string, pos ast.Position) (tag, syntax string, interp bool, err error) {
	spec = strings.TrimSpace(spec)
	if slash := strings.IndexByte(spec, '/'); slash >= 0 {
		spec = strings.TrimSpace(spec[:slash])
	}
	if colon := strings.IndexByte(spec, ':'); colon >= 0 {
		syntax = strings.TrimSpace(spec[colon+1:])
		spec = strings.TrimSpace(spec[:colon])
	}
	if len(spec) >= 2 && spec[0] == '"' && spec[len(spec)-1] == '"' {
		interp = true
		spec = spec[1 : len(spec)-1]
	}
	if spec == "" {
		return "", "", false, &Error{Pos: pos, Msg: "empty heredoc tag"}
	}
	return spec, syntax, interp, nil
}

// scanHeredocBody collects raw body lines from start until the terminator line
// for tag, returning the body lines, the offset just past the terminator line,
// whether trailing newline is chomped, and the indentation margin.
func scanHeredocBody(src []rune, start int, tag string, pos ast.Position) (lines []string, end int, chomp bool, margin int, err error) {
	i := start
	for {
		if i >= len(src) {
			return nil, 0, false, 0, &Error{Pos: pos, Msg: "unterminated heredoc (missing tag " + tag + ")"}
		}
		ls := i
		for i < len(src) && src[i] != '\n' {
			i++
		}
		line := string(src[ls:i])
		if c, m, ok := heredocTerminator(line, tag); ok {
			end = i // position at the newline (or EOF); adv consumes it
			return lines, end, c, m, nil
		}
		lines = append(lines, line)
		if i < len(src) {
			i++ // consume newline
		}
	}
}

// heredocTerminator reports whether line is the terminator for tag, and if so
// returns whether it chomps the final newline (`-`) and the margin column of
// the `|` indent marker (0 if absent).
func heredocTerminator(line, tag string) (chomp bool, margin int, ok bool) {
	rest := strings.TrimLeft(line, " \t")
	lead := len(line) - len(rest)
	if strings.HasPrefix(rest, "|") {
		margin = lead // strip up to the column of the '|' marker
		rest = strings.TrimLeft(rest[1:], " \t")
	}
	if strings.HasPrefix(rest, "-") {
		chomp = true
		rest = strings.TrimLeft(rest[1:], " \t")
	}
	if rest == tag {
		return chomp, margin, true
	}
	return false, 0, false
}

// renderHeredoc joins body lines, stripping up to margin leading columns from
// each and optionally chomping the final newline.
func renderHeredoc(lines []string, chomp bool, margin int) string {
	var b strings.Builder
	for idx, line := range lines {
		if margin > 0 {
			line = stripMargin(line, margin)
		}
		b.WriteString(line)
		if idx < len(lines)-1 || !chomp {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// stripMargin removes up to margin leading space/tab columns from line.
func stripMargin(line string, margin int) string {
	n := 0
	for n < len(line) && n < margin && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return line[n:]
}
