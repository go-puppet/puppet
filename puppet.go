// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package puppet is a pure-Go (cgo-free) implementation of the Puppet language.
//
// This root package is a thin façade over the pipeline sub-packages:
//
//   - [github.com/go-puppet/puppet/lexer] — the tokenizer;
//   - [github.com/go-puppet/puppet/parser] — the recursive-descent parser;
//   - [github.com/go-puppet/puppet/ast] — the Puppet::Pops-style syntax model.
//
// [Parse] compiles a manifest into an [ast.Program]; [ParseExpression] parses a
// single expression. The evaluator and catalog compiler build on this model.
//
// Everything is pure Go: no cgo, so it cross-compiles to and runs on every
// 64-bit Go target, and links into a static binary by default.
package puppet

import (
	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/parser"
)

// Parse parses a complete Puppet manifest into its [ast.Program].
func Parse(src string) (*ast.Program, error) {
	return parser.Parse(src)
}

// ParseExpression parses a single Puppet expression into an [ast.Node].
func ParseExpression(src string) (ast.Node, error) {
	return parser.ParseExpression(src)
}
