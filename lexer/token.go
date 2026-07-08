// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package lexer turns Puppet source into a token stream. It is independent of
// the AST: it recognizes the Puppet lexical grammar — names, type references,
// variables, numbers, single/double-quoted strings, heredocs, regular
// expressions, keywords and the full operator/punctuation set — and leaves
// string interpolation for the parser to expand from the raw token text.
package lexer

import "github.com/go-puppet/puppet/ast"

// Kind enumerates the lexical token kinds.
type Kind int

// Token kinds.
const (
	EOF Kind = iota
	NAME
	TYPE
	VARIABLE
	INT
	FLOAT
	SQSTRING // single-quoted string, fully unescaped
	DQSTRING // double-quoted string, raw inner text (parser expands)
	HEREDOC  // heredoc body (raw; parser expands if Interp)
	REGEXP

	// keywords
	KwTrue
	KwFalse
	KwUndef
	KwDefault
	KwIf
	KwElsif
	KwElse
	KwUnless
	KwCase
	KwAnd
	KwOr
	KwIn
	KwClass
	KwDefine
	KwInherits
	KwNode
	KwFunction
	KwType

	// punctuation
	LBrace
	RBrace
	LBrack
	RBrack
	LParen
	RParen
	Comma
	Semi
	Colon
	Dot
	Query

	// arrows / relationships
	FArrow     // =>
	PArrow     // +>
	Arrow      // ->
	TildeArrow // ~>
	LArrow     // <-
	LTilde     // <~

	// assignment
	Equals
	PlusEq
	MinusEq

	// comparison / match
	IsEq
	NotEq
	Match
	NotMatch
	Lt
	Gt
	Le
	Ge

	// arithmetic / boolean
	Plus
	Minus
	Star
	Slash
	Mod
	LShift
	Not

	// resource decorations / collectors
	At
	AtAt
	Pipe
	LCollect  // <|
	RCollect  // |>
	LLCollect // <<|
	RRCollect // |>>
)

var kindName = [...]string{
	EOF: "EOF", NAME: "NAME", TYPE: "TYPE", VARIABLE: "VARIABLE",
	INT: "INT", FLOAT: "FLOAT", SQSTRING: "SQSTRING", DQSTRING: "DQSTRING",
	HEREDOC: "HEREDOC", REGEXP: "REGEXP",
	KwTrue: "true", KwFalse: "false", KwUndef: "undef", KwDefault: "default",
	KwIf: "if", KwElsif: "elsif", KwElse: "else", KwUnless: "unless",
	KwCase: "case", KwAnd: "and", KwOr: "or", KwIn: "in",
	KwClass: "class", KwDefine: "define", KwInherits: "inherits",
	KwNode: "node", KwFunction: "function", KwType: "type",
	LBrace: "{", RBrace: "}", LBrack: "[", RBrack: "]",
	LParen: "(", RParen: ")", Comma: ",", Semi: ";", Colon: ":",
	Dot: ".", Query: "?",
	FArrow: "=>", PArrow: "+>", Arrow: "->", TildeArrow: "~>",
	LArrow: "<-", LTilde: "<~",
	Equals: "=", PlusEq: "+=", MinusEq: "-=",
	IsEq: "==", NotEq: "!=", Match: "=~", NotMatch: "!~",
	Lt: "<", Gt: ">", Le: "<=", Ge: ">=",
	Plus: "+", Minus: "-", Star: "*", Slash: "/", Mod: "%",
	LShift: "<<", Not: "!",
	At: "@", AtAt: "@@", Pipe: "|",
	LCollect: "<|", RCollect: "|>", LLCollect: "<<|", RRCollect: "|>>",
}

// String returns the token kind's name.
func (k Kind) String() string { return kindName[k] }

// Token is one lexical token.
type Token struct {
	Kind   Kind
	Text   string       // literal/operator text, or raw string content
	Pos    ast.Position // source position of the token start
	Interp bool         // DQSTRING/HEREDOC: interpolation is enabled
	Syntax string       // HEREDOC: optional syntax tag (e.g. "json")
}

var keywords = map[string]Kind{
	"true": KwTrue, "false": KwFalse, "undef": KwUndef, "default": KwDefault,
	"if": KwIf, "elsif": KwElsif, "else": KwElse, "unless": KwUnless,
	"case": KwCase, "and": KwAnd, "or": KwOr, "in": KwIn,
	"class": KwClass, "define": KwDefine, "inherits": KwInherits,
	"node": KwNode, "function": KwFunction,
}
