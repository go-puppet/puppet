// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parser

import (
	"strconv"

	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/lexer"
)

// parsePrimary parses an atom: a literal, name, variable, grouping, collection
// literal, conditional, or a decorated resource.
func (p *parser) parsePrimary(c *cursor) (ast.Node, error) {
	t := p.cur(c)
	switch t.Kind {
	case lexer.INT:
		p.advance(c)
		v, radix, err := parseIntLiteral(t.Text)
		if err != nil {
			return nil, &Error{Pos: t.Pos, Msg: err.Error()}
		}
		return &ast.Integer{Base: base(t.Pos), Value: v, Radix: radix}, nil
	case lexer.FLOAT:
		p.advance(c)
		v, err := strconv.ParseFloat(t.Text, 64)
		if err != nil {
			return nil, &Error{Pos: t.Pos, Msg: "invalid float literal"}
		}
		return &ast.Float{Base: base(t.Pos), Value: v}, nil
	case lexer.SQSTRING:
		p.advance(c)
		return &ast.String{Base: base(t.Pos), Value: t.Text}, nil
	case lexer.DQSTRING:
		p.advance(c)
		return interpolate(t.Text, t.Pos)
	case lexer.HEREDOC:
		p.advance(c)
		var text ast.Node
		var err error
		if t.Interp {
			text, err = interpolate(t.Text, t.Pos)
		} else {
			text = &ast.String{Base: base(t.Pos), Value: t.Text}
		}
		if err != nil {
			return nil, err
		}
		return &ast.Heredoc{Base: base(t.Pos), Syntax: t.Syntax, Text: text}, nil
	case lexer.REGEXP:
		p.advance(c)
		return &ast.Regexp{Base: base(t.Pos), Value: t.Text}, nil
	case lexer.KwTrue:
		p.advance(c)
		return &ast.Boolean{Base: base(t.Pos), Value: true}, nil
	case lexer.KwFalse:
		p.advance(c)
		return &ast.Boolean{Base: base(t.Pos), Value: false}, nil
	case lexer.KwUndef:
		p.advance(c)
		return &ast.Undef{Base: base(t.Pos)}, nil
	case lexer.KwDefault:
		p.advance(c)
		return &ast.Default{Base: base(t.Pos)}, nil
	case lexer.VARIABLE:
		p.advance(c)
		return &ast.Variable{Base: base(t.Pos), Name: t.Text}, nil
	case lexer.NAME:
		p.advance(c)
		return &ast.QualifiedName{Base: base(t.Pos), Value: t.Text}, nil
	case lexer.TYPE:
		p.advance(c)
		return &ast.QualifiedReference{Base: base(t.Pos), Value: t.Text}, nil
	case lexer.LParen:
		p.advance(c)
		e, err := p.parseExpr(c)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(c, lexer.RParen); err != nil {
			return nil, err
		}
		return e, nil
	case lexer.LBrack:
		return p.parseArray(c)
	case lexer.LBrace:
		return p.parseHash(c)
	case lexer.KwIf:
		return p.parseIf(c)
	case lexer.KwUnless:
		return p.parseUnless(c)
	case lexer.KwCase:
		return p.parseCase(c)
	case lexer.At, lexer.AtAt:
		return p.parseVirtualResource(c)
	}
	return nil, p.errAt(c, "unexpected %s", t.Kind)
}

func (p *parser) parseArray(c *cursor) (ast.Node, error) {
	pos := p.advance(c).Pos // [
	elems, err := p.parseExprList(c, lexer.RBrack)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(c, lexer.RBrack); err != nil {
		return nil, err
	}
	return &ast.Array{Base: base(pos), Elements: elems}, nil
}

func (p *parser) parseHash(c *cursor) (ast.Node, error) {
	pos := p.advance(c).Pos // {
	var entries []ast.KeyedEntry
	for p.kind(c) != lexer.RBrace {
		k, err := p.parseExpr(c)
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
		entries = append(entries, ast.KeyedEntry{Key: k, Value: v})
		if !p.accept(c, lexer.Comma) {
			break
		}
	}
	if _, err := p.expect(c, lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.Hash{Base: base(pos), Entries: entries}, nil
}

// maybeLambda parses a trailing `|params| { body }` block if present.
func (p *parser) maybeLambda(c *cursor) (*ast.Lambda, error) {
	if p.kind(c) != lexer.Pipe {
		return nil, nil
	}
	pos := p.advance(c).Pos // |
	params, err := p.parseParams(c, lexer.Pipe)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(c, lexer.Pipe); err != nil {
		return nil, err
	}
	body, err := p.parseBlock(c)
	if err != nil {
		return nil, err
	}
	return &ast.Lambda{Base: base(pos), Params: params, Body: body}, nil
}

// parseParams parses a comma-separated parameter list up to (but not
// consuming) close.
func (p *parser) parseParams(c *cursor, close lexer.Kind) ([]ast.Parameter, error) {
	var params []ast.Parameter
	for p.kind(c) != close && p.kind(c) != lexer.EOF {
		param, err := p.parseParam(c)
		if err != nil {
			return nil, err
		}
		params = append(params, param)
		if !p.accept(c, lexer.Comma) {
			break
		}
	}
	return params, nil
}

func (p *parser) parseParam(c *cursor) (ast.Parameter, error) {
	var param ast.Parameter
	param.CapturesRest = p.accept(c, lexer.Star)
	if p.kind(c) == lexer.TYPE {
		t, err := p.parseDataType(c)
		if err != nil {
			return param, err
		}
		param.Type = t
	}
	v, err := p.expect(c, lexer.VARIABLE)
	if err != nil {
		return param, err
	}
	param.Name = v.Text
	if p.accept(c, lexer.Equals) {
		def, err := p.parseExpr(c)
		if err != nil {
			return param, err
		}
		param.Default = def
	}
	return param, nil
}

// parseCollector parses `Type <| query |>` / `Type <<| query |>>`.
func (p *parser) parseCollector(c *cursor, typ ast.Node) (ast.Node, error) {
	exported := p.kind(c) == lexer.LLCollect
	pos := p.advance(c).Pos
	end := lexer.RCollect
	if exported {
		end = lexer.RRCollect
	}
	var query ast.Node
	if p.kind(c) != end {
		q, err := p.parseExpr(c)
		if err != nil {
			return nil, err
		}
		query = q
	}
	if _, err := p.expect(c, end); err != nil {
		return nil, err
	}
	return &ast.Collector{Base: base(pos), Type: typ, Query: query, Exported: exported}, nil
}

// parseIntLiteral converts a lexed integer literal, reporting the radix (8, 10
// or 16) as written.
func parseIntLiteral(text string) (int64, int, error) {
	radix := 10
	switch {
	case len(text) > 1 && (text[1] == 'x' || text[1] == 'X'):
		radix = 16
	case len(text) > 1 && text[0] == '0':
		radix = 8
	}
	v, err := strconv.ParseInt(text, 0, 64)
	if err != nil {
		return 0, 0, err
	}
	return v, radix, nil
}
