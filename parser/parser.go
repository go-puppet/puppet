// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package parser turns Puppet source into an [ast] tree (the Puppet::Pops
// model). It is a hand-written recursive-descent parser over the token stream
// produced by [lexer], covering the Puppet 8 expression grammar — literals and
// interpolation, data-type expressions, the full operator precedence ladder,
// selectors, conditionals, resource declarations/defaults/overrides/collectors,
// class/define/node/function definitions, function calls with lambdas, and
// relationship chaining.
package parser

import (
	"fmt"

	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/lexer"
)

// Error is a parse error with a source position.
type Error struct {
	Pos ast.Position
	Msg string
}

// Error implements error.
func (e *Error) Error() string {
	return fmt.Sprintf("parse error at %s: %s", e.Pos, e.Msg)
}

type parser struct {
	toks []lexer.Token
}

// state is the parser cursor; kept separate so it is trivially copyable for
// speculative lookahead (used to disambiguate resource bodies).
type cursor struct{ i int }

// Parse parses a complete Puppet manifest and returns its [ast.Program].
func Parse(src string) (*ast.Program, error) {
	toks, err := lexer.Lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	c := &cursor{}
	body, err := p.parseStatements(c, lexer.EOF)
	if err != nil {
		return nil, err
	}
	return &ast.Program{Body: body}, nil
}

// ParseExpression parses a single Puppet expression (used for `${...}`
// interpolation and by callers that need one expression rather than a program).
func ParseExpression(src string) (ast.Node, error) {
	toks, err := lexer.Lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	c := &cursor{}
	n, err := p.parseExpr(c)
	if err != nil {
		return nil, err
	}
	if p.kind(c) != lexer.EOF {
		return nil, p.errAt(c, "unexpected %s after expression", p.cur(c).Kind)
	}
	return n, nil
}

// --- cursor helpers -------------------------------------------------------

func (p *parser) cur(c *cursor) lexer.Token { return p.toks[c.i] }
func (p *parser) kind(c *cursor) lexer.Kind { return p.toks[c.i].Kind }

func (p *parser) at(c *cursor, n int) lexer.Token {
	j := c.i + n
	if j >= len(p.toks) {
		return p.toks[len(p.toks)-1] // EOF
	}
	return p.toks[j]
}

func (p *parser) advance(c *cursor) lexer.Token {
	t := p.toks[c.i]
	if c.i < len(p.toks)-1 {
		c.i++
	}
	return t
}

func (p *parser) accept(c *cursor, k lexer.Kind) bool {
	if p.kind(c) == k {
		p.advance(c)
		return true
	}
	return false
}

func (p *parser) expect(c *cursor, k lexer.Kind) (lexer.Token, error) {
	if p.kind(c) != k {
		return lexer.Token{}, p.errAt(c, "expected %s, got %s", k, p.cur(c).Kind)
	}
	return p.advance(c), nil
}

func (p *parser) errAt(c *cursor, format string, args ...any) error {
	return &Error{Pos: p.cur(c).Pos, Msg: fmt.Sprintf(format, args...)}
}

// --- statements & blocks --------------------------------------------------

// parseStatements parses a sequence of statements terminated by end (RBrace or
// EOF), consuming optional `;` separators.
func (p *parser) parseStatements(c *cursor, end lexer.Kind) ([]ast.Node, error) {
	var out []ast.Node
	for {
		for p.accept(c, lexer.Semi) {
		}
		if k := p.kind(c); k == end || k == lexer.EOF {
			return out, nil
		}
		n, err := p.parseStatement(c)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
}

// parseBlock parses `{ statements }`.
func (p *parser) parseBlock(c *cursor) ([]ast.Node, error) {
	if _, err := p.expect(c, lexer.LBrace); err != nil {
		return nil, err
	}
	body, err := p.parseStatements(c, lexer.RBrace)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(c, lexer.RBrace); err != nil {
		return nil, err
	}
	return body, nil
}

// parseStatement parses one top-level/block element.
func (p *parser) parseStatement(c *cursor) (ast.Node, error) {
	switch p.kind(c) {
	case lexer.KwClass:
		return p.parseClass(c)
	case lexer.KwDefine:
		return p.parseDefine(c)
	case lexer.KwNode:
		return p.parseNode(c)
	case lexer.KwFunction:
		return p.parseFunctionDef(c)
	}
	e, err := p.parseExpr(c)
	if err != nil {
		return nil, err
	}
	// Statement-style function call: `include foo, bar`, `notice "x"`.
	if qn, ok := e.(*ast.QualifiedName); ok && startsArgument(p.kind(c)) {
		args, err := p.parseExprList(c, lexer.Semi)
		if err != nil {
			return nil, err
		}
		lambda, err := p.maybeLambda(c)
		if err != nil {
			return nil, err
		}
		return &ast.Call{Base: base(qn.Pos()), Functor: qn, Args: args, Lambda: lambda}, nil
	}
	return e, nil
}

// startsArgument reports whether k can begin an expression argument for a
// statement-style function call.
func startsArgument(k lexer.Kind) bool {
	switch k {
	case lexer.NAME, lexer.TYPE, lexer.VARIABLE, lexer.INT, lexer.FLOAT,
		lexer.SQSTRING, lexer.DQSTRING, lexer.HEREDOC, lexer.REGEXP,
		lexer.LBrack, lexer.LParen, lexer.KwTrue, lexer.KwFalse,
		lexer.KwUndef, lexer.KwDefault, lexer.Minus, lexer.Not:
		return true
	}
	return false
}

// --- expression precedence ladder -----------------------------------------

func (p *parser) parseExpr(c *cursor) (ast.Node, error) { return p.parseRelationship(c) }

func (p *parser) parseRelationship(c *cursor) (ast.Node, error) {
	left, err := p.parseAssignment(c)
	if err != nil {
		return nil, err
	}
	for {
		var op string
		switch p.kind(c) {
		case lexer.Arrow:
			op = "->"
		case lexer.TildeArrow:
			op = "~>"
		case lexer.LArrow:
			op = "<-"
		case lexer.LTilde:
			op = "<~"
		default:
			return left, nil
		}
		pos := p.advance(c).Pos
		right, err := p.parseAssignment(c)
		if err != nil {
			return nil, err
		}
		left = &ast.Relationship{Base: base(pos), Op: op, Left: left, Right: right}
	}
}

func (p *parser) parseAssignment(c *cursor) (ast.Node, error) {
	left, err := p.parseSelector(c)
	if err != nil {
		return nil, err
	}
	var op string
	switch p.kind(c) {
	case lexer.Equals:
		op = "="
	case lexer.PlusEq:
		op = "+="
	case lexer.MinusEq:
		op = "-="
	default:
		return left, nil
	}
	pos := p.advance(c).Pos
	right, err := p.parseAssignment(c) // right-associative
	if err != nil {
		return nil, err
	}
	return &ast.Assignment{Base: base(pos), Op: op, Target: left, Value: right}, nil
}

func (p *parser) parseSelector(c *cursor) (ast.Node, error) {
	e, err := p.parseOr(c)
	if err != nil {
		return nil, err
	}
	if p.kind(c) != lexer.Query {
		return e, nil
	}
	pos := p.advance(c).Pos
	if _, err := p.expect(c, lexer.LBrace); err != nil {
		return nil, err
	}
	var entries []ast.SelectorEntry
	for p.kind(c) != lexer.RBrace {
		m, err := p.parseExpr(c)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(c, lexer.FArrow); err != nil {
			return nil, err
		}
		v, err := p.parseExpr(c)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ast.SelectorEntry{Match: m, Value: v})
		if !p.accept(c, lexer.Comma) {
			break
		}
	}
	if _, err := p.expect(c, lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.Selector{Base: base(pos), Operand: e, Entries: entries}, nil
}

// binaryTier parses a left-associative binary tier: sub, then repeated
// (op sub) while the current token maps to an operator.
func (p *parser) binaryTier(c *cursor, sub func(*cursor) (ast.Node, error), ops map[lexer.Kind]string) (ast.Node, error) {
	left, err := sub(c)
	if err != nil {
		return nil, err
	}
	for {
		op, ok := ops[p.kind(c)]
		if !ok {
			return left, nil
		}
		pos := p.advance(c).Pos
		right, err := sub(c)
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{Base: base(pos), Op: op, Left: left, Right: right}
	}
}

func (p *parser) parseOr(c *cursor) (ast.Node, error) {
	return p.binaryTier(c, p.parseAnd, map[lexer.Kind]string{lexer.KwOr: "or"})
}
func (p *parser) parseAnd(c *cursor) (ast.Node, error) {
	return p.binaryTier(c, p.parseEquality, map[lexer.Kind]string{lexer.KwAnd: "and"})
}
func (p *parser) parseEquality(c *cursor) (ast.Node, error) {
	return p.binaryTier(c, p.parseRelational, map[lexer.Kind]string{lexer.IsEq: "==", lexer.NotEq: "!="})
}
func (p *parser) parseRelational(c *cursor) (ast.Node, error) {
	return p.binaryTier(c, p.parseShift, map[lexer.Kind]string{
		lexer.Lt: "<", lexer.Gt: ">", lexer.Le: "<=", lexer.Ge: ">="})
}
func (p *parser) parseShift(c *cursor) (ast.Node, error) {
	return p.binaryTier(c, p.parseAdditive, map[lexer.Kind]string{lexer.LShift: "<<"})
}
func (p *parser) parseAdditive(c *cursor) (ast.Node, error) {
	return p.binaryTier(c, p.parseMultiplicative, map[lexer.Kind]string{lexer.Plus: "+", lexer.Minus: "-"})
}
func (p *parser) parseMultiplicative(c *cursor) (ast.Node, error) {
	return p.binaryTier(c, p.parseMatch, map[lexer.Kind]string{lexer.Star: "*", lexer.Slash: "/", lexer.Mod: "%"})
}
func (p *parser) parseMatch(c *cursor) (ast.Node, error) {
	return p.binaryTier(c, p.parseIn, map[lexer.Kind]string{lexer.Match: "=~", lexer.NotMatch: "!~"})
}
func (p *parser) parseIn(c *cursor) (ast.Node, error) {
	return p.binaryTier(c, p.parseUnary, map[lexer.Kind]string{lexer.KwIn: "in"})
}

func (p *parser) parseUnary(c *cursor) (ast.Node, error) {
	var op string
	switch p.kind(c) {
	case lexer.Minus:
		op = "-"
	case lexer.Not:
		op = "!"
	case lexer.Star:
		op = "*"
	default:
		return p.parsePostfix(c)
	}
	pos := p.advance(c).Pos
	operand, err := p.parseUnary(c)
	if err != nil {
		return nil, err
	}
	return &ast.Unary{Base: base(pos), Op: op, Operand: operand}, nil
}

// parsePostfix parses a primary followed by any chain of postfix operators:
// indexing `[...]`, method calls `.m(...)`, calls `(...)`, resource bodies
// `{...}`, overrides `Type[t]{...}` and collectors `<| |>`.
func (p *parser) parsePostfix(c *cursor) (ast.Node, error) {
	e, err := p.parsePrimary(c)
	if err != nil {
		return nil, err
	}
	for {
		switch p.kind(c) {
		case lexer.LBrack:
			// A `[` with preceding whitespace starts a new array/statement
			// rather than indexing the previous expression (Puppet's
			// whitespace rule); indexing a QualifiedReference (a data type or
			// resource reference) is exempt, as `Integer [1,2]` is still a type.
			if p.cur(c).Spaced {
				if _, isRef := e.(*ast.QualifiedReference); !isRef {
					return e, nil
				}
			}
			e, err = p.parseAccess(c, e)
		case lexer.Dot:
			e, err = p.parseMethodCall(c, e)
		case lexer.LParen:
			e, err = p.parseCall(c, e)
		case lexer.LBrace:
			if e2, ok, err2 := p.tryResourceLike(c, e); ok {
				e, err = e2, err2
			} else {
				return e, nil
			}
		case lexer.LCollect, lexer.LLCollect:
			if _, ok := e.(*ast.QualifiedReference); ok {
				e, err = p.parseCollector(c, e)
			} else {
				return e, nil
			}
		default:
			return e, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (p *parser) parseAccess(c *cursor, operand ast.Node) (ast.Node, error) {
	pos := p.advance(c).Pos // [
	keys, err := p.parseExprList(c, lexer.RBrack)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, p.errAt(c, "empty [] access")
	}
	if _, err := p.expect(c, lexer.RBrack); err != nil {
		return nil, err
	}
	return &ast.Access{Base: base(pos), Operand: operand, Keys: keys}, nil
}

func (p *parser) parseMethodCall(c *cursor, recv ast.Node) (ast.Node, error) {
	pos := p.advance(c).Pos // .
	name, err := p.methodName(c)
	if err != nil {
		return nil, err
	}
	var args []ast.Node
	if p.kind(c) == lexer.LParen {
		p.advance(c)
		args, err = p.parseExprList(c, lexer.RParen)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(c, lexer.RParen); err != nil {
			return nil, err
		}
	}
	lambda, err := p.maybeLambda(c)
	if err != nil {
		return nil, err
	}
	return &ast.MethodCall{Base: base(pos), Receiver: recv, Method: name, Args: args, Lambda: lambda}, nil
}

func (p *parser) methodName(c *cursor) (string, error) {
	switch p.kind(c) {
	case lexer.NAME, lexer.TYPE:
		return p.advance(c).Text, nil
	}
	return "", p.errAt(c, "expected method name after '.', got %s", p.cur(c).Kind)
}

func (p *parser) parseCall(c *cursor, functor ast.Node) (ast.Node, error) {
	switch functor.(type) {
	case *ast.QualifiedName, *ast.QualifiedReference:
	default:
		return nil, p.errAt(c, "cannot call this expression")
	}
	pos := p.advance(c).Pos // (
	args, err := p.parseExprList(c, lexer.RParen)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(c, lexer.RParen); err != nil {
		return nil, err
	}
	lambda, err := p.maybeLambda(c)
	if err != nil {
		return nil, err
	}
	return &ast.Call{Base: base(pos), Functor: functor, Args: args, Lambda: lambda, RVal: true}, nil
}

// parseExprList parses a comma-separated expression list up to (but not
// consuming) end, tolerating a trailing comma.
func (p *parser) parseExprList(c *cursor, end lexer.Kind) ([]ast.Node, error) {
	var out []ast.Node
	for p.kind(c) != end && p.kind(c) != lexer.EOF {
		n, err := p.parseExpr(c)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
		if !p.accept(c, lexer.Comma) {
			break
		}
	}
	return out, nil
}

func base(pos ast.Position) ast.Base { return ast.Base{P: pos} }
