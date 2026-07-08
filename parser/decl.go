// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parser

import (
	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/lexer"
)

// --- conditionals ---------------------------------------------------------

// parseIf parses `if`/`elsif` (entered on either keyword) with an optional
// `elsif`/`else` tail.
func (p *parser) parseIf(c *cursor) (ast.Node, error) {
	pos := p.advance(c).Pos // if | elsif
	cond, err := p.parseExpr(c)
	if err != nil {
		return nil, err
	}
	then, err := p.parseBlock(c)
	if err != nil {
		return nil, err
	}
	var els []ast.Node
	switch p.kind(c) {
	case lexer.KwElsif:
		nested, err := p.parseIf(c)
		if err != nil {
			return nil, err
		}
		els = []ast.Node{nested}
	case lexer.KwElse:
		p.advance(c)
		els, err = p.parseBlock(c)
		if err != nil {
			return nil, err
		}
	}
	return &ast.If{Base: base(pos), Cond: cond, Then: then, Else: els}, nil
}

func (p *parser) parseUnless(c *cursor) (ast.Node, error) {
	pos := p.advance(c).Pos
	cond, err := p.parseExpr(c)
	if err != nil {
		return nil, err
	}
	then, err := p.parseBlock(c)
	if err != nil {
		return nil, err
	}
	var els []ast.Node
	if p.accept(c, lexer.KwElse) {
		els, err = p.parseBlock(c)
		if err != nil {
			return nil, err
		}
	}
	return &ast.Unless{Base: base(pos), Cond: cond, Then: then, Else: els}, nil
}

func (p *parser) parseCase(c *cursor) (ast.Node, error) {
	pos := p.advance(c).Pos
	test, err := p.parseExpr(c)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(c, lexer.LBrace); err != nil {
		return nil, err
	}
	var options []ast.CaseOption
	for p.kind(c) != lexer.RBrace && p.kind(c) != lexer.EOF {
		values, err := p.parseExprList(c, lexer.Colon)
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			return nil, p.errAt(c, "case option requires at least one match value")
		}
		if _, err := p.expect(c, lexer.Colon); err != nil {
			return nil, err
		}
		body, err := p.parseBlock(c)
		if err != nil {
			return nil, err
		}
		options = append(options, ast.CaseOption{Values: values, Body: body})
	}
	if _, err := p.expect(c, lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.Case{Base: base(pos), Test: test, Options: options}, nil
}

// --- resources ------------------------------------------------------------

// tryResourceLike interprets a `{` following a primary as a resource
// declaration, resource defaults, or a resource override, depending on the
// primary's shape. It reports ok=false when the primary cannot introduce one.
func (p *parser) tryResourceLike(c *cursor, e ast.Node) (ast.Node, bool, error) {
	switch x := e.(type) {
	case *ast.QualifiedName:
		n, err := p.parseResourceDecl(c, e, ast.Regular)
		return n, true, err
	case *ast.QualifiedReference:
		n, err := p.parseResourceDefaults(c, e)
		return n, true, err
	case *ast.Access:
		if _, ok := x.Operand.(*ast.QualifiedReference); ok {
			n, err := p.parseResourceOverride(c, e)
			return n, true, err
		}
	}
	return nil, false, nil
}

func (p *parser) parseResourceDecl(c *cursor, typ ast.Node, form ast.ResourceForm) (ast.Node, error) {
	pos := p.advance(c).Pos // {
	var bodies []ast.ResourceBody
	for p.kind(c) != lexer.RBrace {
		title, err := p.parseExpr(c)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(c, lexer.Colon); err != nil {
			return nil, err
		}
		ops, err := p.parseAttributeOps(c)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, ast.ResourceBody{Title: title, Ops: ops})
		if !p.accept(c, lexer.Semi) {
			break
		}
	}
	if _, err := p.expect(c, lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.Resource{Base: base(pos), Type: typ, Bodies: bodies, Form: form}, nil
}

func (p *parser) parseResourceDefaults(c *cursor, typ ast.Node) (ast.Node, error) {
	pos := p.advance(c).Pos // {
	ops, err := p.parseAttributeOps(c)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(c, lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.ResourceDefaults{Base: base(pos), Type: typ, Ops: ops}, nil
}

func (p *parser) parseResourceOverride(c *cursor, ref ast.Node) (ast.Node, error) {
	pos := p.advance(c).Pos // {
	ops, err := p.parseAttributeOps(c)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(c, lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.ResourceOverride{Base: base(pos), Resource: ref, Ops: ops}, nil
}

// parseAttributeOps parses comma-separated attribute operations, stopping at
// `;` or `}`.
func (p *parser) parseAttributeOps(c *cursor) ([]ast.AttributeOp, error) {
	var ops []ast.AttributeOp
	for {
		if k := p.kind(c); k == lexer.RBrace || k == lexer.Semi {
			return ops, nil
		}
		var op ast.AttributeOp
		if p.accept(c, lexer.Star) {
			op.Splat = true
			if _, err := p.expect(c, lexer.FArrow); err != nil {
				return nil, err
			}
			op.Op = "=>"
		} else {
			name, err := p.expect(c, lexer.NAME)
			if err != nil {
				return nil, err
			}
			op.Name = name.Text
			switch p.kind(c) {
			case lexer.FArrow:
				op.Op = "=>"
			case lexer.PArrow:
				op.Op = "+>"
			default:
				return nil, p.errAt(c, "expected => or +>, got %s", p.cur(c).Kind)
			}
			p.advance(c)
		}
		val, err := p.parseExpr(c)
		if err != nil {
			return nil, err
		}
		op.Value = val
		ops = append(ops, op)
		if !p.accept(c, lexer.Comma) {
			return ops, nil
		}
	}
}

// parseVirtualResource parses `@resource {...}` (virtual) or `@@resource {...}`
// (exported).
func (p *parser) parseVirtualResource(c *cursor) (ast.Node, error) {
	form := ast.Virtual
	if p.kind(c) == lexer.AtAt {
		form = ast.Exported
	}
	p.advance(c)
	name, err := p.expect(c, lexer.NAME)
	if err != nil {
		return nil, err
	}
	typ := &ast.QualifiedName{Base: base(name.Pos), Value: name.Text}
	if p.kind(c) != lexer.LBrace {
		return nil, p.errAt(c, "expected '{' after virtual/exported resource type")
	}
	return p.parseResourceDecl(c, typ, form)
}

// --- definitions ----------------------------------------------------------

func (p *parser) parseClass(c *cursor) (ast.Node, error) {
	pos := p.advance(c).Pos // class
	if p.kind(c) == lexer.LBrace {
		typ := &ast.QualifiedName{Base: base(pos), Value: "class"}
		return p.parseResourceDecl(c, typ, ast.Regular)
	}
	name, err := p.defName(c)
	if err != nil {
		return nil, err
	}
	params, err := p.parseParenParams(c)
	if err != nil {
		return nil, err
	}
	parent := ""
	if p.accept(c, lexer.KwInherits) {
		pn, err := p.defName(c)
		if err != nil {
			return nil, err
		}
		parent = pn
	}
	body, err := p.parseBlock(c)
	if err != nil {
		return nil, err
	}
	return &ast.ClassDefinition{Base: base(pos), Name: name, Params: params, Parent: parent, Body: body}, nil
}

func (p *parser) parseDefine(c *cursor) (ast.Node, error) {
	pos := p.advance(c).Pos
	name, err := p.defName(c)
	if err != nil {
		return nil, err
	}
	params, err := p.parseParenParams(c)
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock(c)
	if err != nil {
		return nil, err
	}
	return &ast.DefineDefinition{Base: base(pos), Name: name, Params: params, Body: body}, nil
}

func (p *parser) parseNode(c *cursor) (ast.Node, error) {
	pos := p.advance(c).Pos
	matches, err := p.parseExprList(c, lexer.LBrace)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, p.errAt(c, "node definition requires at least one matcher")
	}
	body, err := p.parseBlock(c)
	if err != nil {
		return nil, err
	}
	return &ast.NodeDefinition{Base: base(pos), Matches: matches, Body: body}, nil
}

func (p *parser) parseFunctionDef(c *cursor) (ast.Node, error) {
	pos := p.advance(c).Pos
	name, err := p.defName(c)
	if err != nil {
		return nil, err
	}
	params, err := p.parseParenParams(c)
	if err != nil {
		return nil, err
	}
	var rtype ast.Node
	if p.kind(c) == lexer.Gt && p.at(c, 1).Kind == lexer.Gt {
		p.advance(c)
		p.advance(c)
		rtype, err = p.parseDataType(c)
		if err != nil {
			return nil, err
		}
	}
	body, err := p.parseBlock(c)
	if err != nil {
		return nil, err
	}
	return &ast.FunctionDefinition{Base: base(pos), Name: name, Params: params, ReturnType: rtype, Body: body}, nil
}

// parseDataType parses a data-type expression: a [ast.QualifiedReference]
// followed by any number of `[...]` parameter lists (e.g. `Optional[String]`,
// `Hash[String, Integer]`). Unlike the general postfix parser it never treats
// a following `{` as a resource, so it is safe before a `$name` or a `{ body }`.
func (p *parser) parseDataType(c *cursor) (ast.Node, error) {
	e, err := p.parsePrimary(c)
	if err != nil {
		return nil, err
	}
	for p.kind(c) == lexer.LBrack {
		e, err = p.parseAccess(c, e)
		if err != nil {
			return nil, err
		}
	}
	return e, nil
}

// defName reads a definition name (a bareword, possibly namespaced).
func (p *parser) defName(c *cursor) (string, error) {
	t, err := p.expect(c, lexer.NAME)
	if err != nil {
		return "", err
	}
	return t.Text, nil
}

// parseParenParams parses an optional `( params )` clause.
func (p *parser) parseParenParams(c *cursor) ([]ast.Parameter, error) {
	if !p.accept(c, lexer.LParen) {
		return nil, nil
	}
	params, err := p.parseParams(c, lexer.RParen)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(c, lexer.RParen); err != nil {
		return nil, err
	}
	return params, nil
}
