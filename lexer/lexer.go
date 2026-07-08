// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package lexer

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/go-puppet/puppet/ast"
)

// Error is a lexical error with a source position.
type Error struct {
	Pos ast.Position
	Msg string
}

// Error implements error.
func (e *Error) Error() string {
	return fmt.Sprintf("lex error at %s: %s", e.Pos, e.Msg)
}

// lexer holds scanning state over a rune slice.
type lexer struct {
	src    []rune
	i      int
	line   int
	col    int
	toks   []Token
	resume int // offset to jump to at the next newline (heredoc body end)
}

// Lex tokenizes src, returning the tokens (terminated by an [EOF] token) or a
// lexical [Error]. The last token before a following operator determines
// whether `/` begins a regexp or is division; the lexer tracks that with the
// previously emitted token.
func Lex(src string) ([]Token, error) {
	lx := &lexer{src: []rune(src), line: 1, col: 1, resume: -1}
	for {
		tok, err := lx.next()
		if err != nil {
			return nil, err
		}
		lx.toks = append(lx.toks, tok)
		if tok.Kind == EOF {
			return lx.toks, nil
		}
	}
}

func (lx *lexer) pos() ast.Position {
	return ast.Position{Offset: lx.i, Line: lx.line, Column: lx.col}
}

func (lx *lexer) peek() rune {
	if lx.i >= len(lx.src) {
		return -1
	}
	return lx.src[lx.i]
}

func (lx *lexer) at(off int) rune {
	if lx.i+off >= len(lx.src) {
		return -1
	}
	return lx.src[lx.i+off]
}

// adv consumes one rune, updating line/column and jumping past a pending
// heredoc body when a newline is crossed.
func (lx *lexer) adv() rune {
	r := lx.src[lx.i]
	lx.i++
	if r == '\n' {
		lx.line++
		lx.col = 1
		if lx.resume >= 0 {
			lx.i = lx.resume
			lx.resume = -1
		}
	} else {
		lx.col++
	}
	return r
}

func (lx *lexer) prevKind() Kind {
	if len(lx.toks) == 0 {
		return EOF
	}
	return lx.toks[len(lx.toks)-1].Kind
}

func (lx *lexer) skipSpaceAndComments() error {
	for lx.i < len(lx.src) {
		r := lx.peek()
		switch {
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			lx.adv()
		case r == '#':
			for lx.i < len(lx.src) && lx.peek() != '\n' {
				lx.adv()
			}
		case r == '/' && lx.at(1) == '*':
			start := lx.pos()
			lx.adv()
			lx.adv()
			for {
				if lx.i >= len(lx.src) {
					return &Error{Pos: start, Msg: "unterminated block comment"}
				}
				if lx.peek() == '*' && lx.at(1) == '/' {
					lx.adv()
					lx.adv()
					break
				}
				lx.adv()
			}
		default:
			return nil
		}
	}
	return nil
}

func (lx *lexer) next() (Token, error) {
	if err := lx.skipSpaceAndComments(); err != nil {
		return Token{}, err
	}
	pos := lx.pos()
	if lx.i >= len(lx.src) {
		return Token{Kind: EOF, Pos: pos}, nil
	}
	r := lx.peek()

	switch {
	case r == '$':
		return lx.lexVariable(pos)
	case r == '\'':
		return lx.lexSingle(pos)
	case r == '"':
		return lx.lexDouble(pos)
	case r == '/' && lx.regexpAllowed():
		return lx.lexRegexp(pos)
	case unicode.IsDigit(r):
		return lx.lexNumber(pos)
	case r == '_' || unicode.IsLetter(r):
		return lx.lexWord(pos, "")
	case r == ':' && lx.at(1) == ':' && isWordStart(lx.at(2)):
		lx.adv()
		lx.adv()
		return lx.lexWord(pos, "::")
	}
	return lx.lexOperator(pos)
}

// regexpAllowed reports whether a `/` at the current position begins a regexp
// rather than the division operator. A regexp may start wherever a value is
// expected: at the beginning, or after an operator/opening punctuation.
func (lx *lexer) regexpAllowed() bool {
	switch lx.prevKind() {
	case NAME, TYPE, VARIABLE, INT, FLOAT, SQSTRING, DQSTRING, HEREDOC, REGEXP,
		RParen, RBrack, RBrace, KwTrue, KwFalse, KwDefault:
		return false
	}
	return true
}

func (lx *lexer) lexVariable(pos ast.Position) (Token, error) {
	lx.adv() // $
	var b strings.Builder
	if lx.peek() == ':' && lx.at(1) == ':' {
		b.WriteString("::")
		lx.adv()
		lx.adv()
	}
	for lx.i < len(lx.src) {
		r := lx.peek()
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lx.adv()
		} else if r == ':' && lx.at(1) == ':' {
			b.WriteString("::")
			lx.adv()
			lx.adv()
		} else {
			break
		}
	}
	if b.Len() == 0 {
		return Token{}, &Error{Pos: pos, Msg: "expected variable name after '$'"}
	}
	return Token{Kind: VARIABLE, Text: b.String(), Pos: pos}, nil
}

func (lx *lexer) lexWord(pos ast.Position, prefix string) (Token, error) {
	var b strings.Builder
	b.WriteString(prefix)
	upper := unicode.IsUpper(lx.peek())
	for lx.i < len(lx.src) {
		r := lx.peek()
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lx.adv()
		} else if r == ':' && lx.at(1) == ':' {
			b.WriteString("::")
			lx.adv()
			lx.adv()
		} else {
			break
		}
	}
	s := b.String()
	if !upper {
		if kw, ok := keywords[s]; ok {
			return Token{Kind: kw, Text: s, Pos: pos}, nil
		}
		return Token{Kind: NAME, Text: s, Pos: pos}, nil
	}
	return Token{Kind: TYPE, Text: s, Pos: pos}, nil
}

func (lx *lexer) lexNumber(pos ast.Position) (Token, error) {
	var b strings.Builder
	if lx.peek() == '0' && (lx.at(1) == 'x' || lx.at(1) == 'X') {
		b.WriteRune(lx.adv())
		b.WriteRune(lx.adv())
		if !isHex(lx.peek()) {
			return Token{}, &Error{Pos: pos, Msg: "malformed hexadecimal literal"}
		}
		for isHex(lx.peek()) {
			b.WriteRune(lx.adv())
		}
		return Token{Kind: INT, Text: b.String(), Pos: pos}, nil
	}
	for unicode.IsDigit(lx.peek()) {
		b.WriteRune(lx.adv())
	}
	isFloat := false
	if lx.peek() == '.' && unicode.IsDigit(lx.at(1)) {
		isFloat = true
		b.WriteRune(lx.adv())
		for unicode.IsDigit(lx.peek()) {
			b.WriteRune(lx.adv())
		}
	}
	if lx.peek() == 'e' || lx.peek() == 'E' {
		isFloat = true
		b.WriteRune(lx.adv())
		if lx.peek() == '+' || lx.peek() == '-' {
			b.WriteRune(lx.adv())
		}
		if !unicode.IsDigit(lx.peek()) {
			return Token{}, &Error{Pos: pos, Msg: "malformed exponent in number"}
		}
		for unicode.IsDigit(lx.peek()) {
			b.WriteRune(lx.adv())
		}
	}
	if isFloat {
		return Token{Kind: FLOAT, Text: b.String(), Pos: pos}, nil
	}
	return Token{Kind: INT, Text: b.String(), Pos: pos}, nil
}

func (lx *lexer) lexSingle(pos ast.Position) (Token, error) {
	lx.adv() // opening '
	var b strings.Builder
	for {
		if lx.i >= len(lx.src) {
			return Token{}, &Error{Pos: pos, Msg: "unterminated single-quoted string"}
		}
		r := lx.adv()
		if r == '\\' {
			n := lx.peek()
			if n == '\'' || n == '\\' {
				b.WriteRune(lx.adv())
			} else {
				b.WriteRune('\\')
			}
			continue
		}
		if r == '\'' {
			break
		}
		b.WriteRune(r)
	}
	return Token{Kind: SQSTRING, Text: b.String(), Pos: pos}, nil
}

func (lx *lexer) lexDouble(pos ast.Position) (Token, error) {
	lx.adv() // opening "
	var b strings.Builder
	for {
		if lx.i >= len(lx.src) {
			return Token{}, &Error{Pos: pos, Msg: "unterminated double-quoted string"}
		}
		r := lx.peek()
		if r == '"' {
			lx.adv()
			break
		}
		if r == '\\' {
			b.WriteRune(lx.adv())
			if lx.i >= len(lx.src) {
				return Token{}, &Error{Pos: pos, Msg: "unterminated double-quoted string"}
			}
			b.WriteRune(lx.adv())
			continue
		}
		b.WriteRune(lx.adv())
	}
	return Token{Kind: DQSTRING, Text: b.String(), Pos: pos, Interp: true}, nil
}

func (lx *lexer) lexRegexp(pos ast.Position) (Token, error) {
	lx.adv() // opening /
	var b strings.Builder
	for {
		if lx.i >= len(lx.src) {
			return Token{}, &Error{Pos: pos, Msg: "unterminated regular expression"}
		}
		r := lx.peek()
		if r == '\n' {
			return Token{}, &Error{Pos: pos, Msg: "unterminated regular expression"}
		}
		if r == '\\' {
			b.WriteRune(lx.adv())
			b.WriteRune(lx.adv())
			continue
		}
		if r == '/' {
			lx.adv()
			break
		}
		b.WriteRune(lx.adv())
	}
	return Token{Kind: REGEXP, Text: b.String(), Pos: pos}, nil
}

func isHex(r rune) bool {
	return unicode.IsDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
