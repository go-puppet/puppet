// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package hcl is a Terraform-style HCL2 front-end for Puppet. [Parse] reads an
// HCL2 (HashiCorp Configuration Language v2) manifest and produces the very
// same [github.com/go-puppet/puppet/ast.Program] the native Puppet (.pp) parser
// produces, so an HCL2 manifest compiles to an identical catalog through the
// existing evaluator. The HCL2 grammar itself is parsed by the pure-Go
// [github.com/go-ruby-hcl2/hcl2] package; this package only translates its
// read-only expression AST into Puppet's model.
//
// # Mapping (v0.1, Terraform-style)
//
// Blocks:
//
//	resource "TYPE" "TITLE" {   →  Puppet resource:  TYPE { 'TITLE':
//	  attr = expr                                       attr => expr,
//	}                                                 }
//
//	locals {                    →  one assignment per attribute:
//	  k = expr                       $k = expr
//	}
//
// A root-level attribute (`k = expr` at the top of the document) maps to the
// same variable assignment as a local. `require`/`before`/`notify`/`subscribe`
// need no special handling: they are ordinary attributes whose values are
// resource references, so Puppet relationships fall out naturally.
//
// Expressions (mapExpr):
//
//	number / bool / null      →  Integer(radix 10) / Boolean / Undef
//	fractional number         →  Float
//	"text"  (no interpolation)→  String
//	"a ${x} b"                →  Concat[ String("a "), <x>, String(" b") ]
//	local.X                   →  $X
//	resource.TYPE.TITLE       →  Type['TITLE']   (Access on a QualifiedReference)
//	resource.TYPE["title"]    →  Type['title']
//	coll[idx]                 →  Access{coll, [idx]}
//	[a, b]                    →  Array
//	{ k = v }                 →  Hash{ k => v }
//	a && b / a || b           →  (a and b) / (a or b)
//	other binary / unary      →  Binary / Unary, operator passed through
//
// Quoted-string escapes (\n \t \r \" \\ \uXXXX \UXXXXXXXX) are expanded;
// heredoc bodies keep backslashes literal. `$${` and `%%{` are literal `${`/`%{`.
//
// # Not yet supported (v0.2)
//
// These return a clear "unsupported in HCL2 v0.1: …" error rather than a fake
// stub: function calls, the `a ? b : c` conditional, `for` tuple/object
// comprehensions, `%{…}` template directives, `~` whitespace-strip markers,
// `class`/`define`/`node` and other unknown block types, and traversal roots
// other than local.X and resource.TYPE.TITLE.
//
// # Example
//
//	prog, err := hcl.Parse(`
//	  locals {
//	    mode = "0644"
//	  }
//	  resource "file" "app" {
//	    ensure  = "present"
//	    mode    = local.mode
//	    require = resource.package.app
//	  }
//	`)
//
// yields the same [ast.Program] as the Puppet manifest:
//
//	$mode = '0644'
//	file { 'app':
//	  ensure  => 'present',
//	  mode    => $mode,
//	  require => Package['app'],
//	}
package hcl
