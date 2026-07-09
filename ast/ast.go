// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package ast defines the abstract-syntax model for the Puppet language — the
// Go analogue of Puppet's Puppet::Pops::Model. Every node produced by the
// parser implements [Node]; concrete node kinds are plain data structs so an
// evaluator can dispatch on them with a type switch. Positions are carried by
// an embedded [Base] so error messages can point at source.
package ast

import "fmt"

// Position is a location in a source file: a 0-based byte offset plus the
// 1-based line and column it falls on.
type Position struct {
	Offset int
	Line   int
	Column int
}

// String renders the position as line:column.
func (p Position) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Column) }

// Node is implemented by every AST node.
type Node interface {
	// Pos returns the node's source position.
	Pos() Position
	node()
}

// Base carries a source position; every concrete node embeds it.
type Base struct{ P Position }

// Pos returns the embedded position.
func (b Base) Pos() Position { return b.P }
func (b Base) node()         {}

// Program is the root: the ordered top-level body of a manifest.
type Program struct {
	Base
	Body []Node
}

// --- literals -------------------------------------------------------------

// Undef is the literal `undef`.
type Undef struct{ Base }

// Default is the literal `default`.
type Default struct{ Base }

// Boolean is a `true`/`false` literal.
type Boolean struct {
	Base
	Value bool
}

// Integer is an integer literal; Radix records 8, 10 or 16 as written.
type Integer struct {
	Base
	Value int64
	Radix int
}

// Float is a floating-point literal.
type Float struct {
	Base
	Value float64
}

// String is a single-quoted (non-interpolating) string literal, or a fully
// static double-quoted string with no interpolation.
type String struct {
	Base
	Value string
}

// Regexp is a `/.../ ` regular-expression literal (source without slashes).
type Regexp struct {
	Base
	Value string
}

// Concat is an interpolating string: an ordered list of parts, each either a
// [String] (literal text) or an arbitrary embedded expression from `${...}` or
// `$var`.
type Concat struct {
	Base
	Parts []Node
}

// Heredoc is a heredoc string. Syntax is the optional tag (e.g. "json"); Text
// is a [String] or [Concat] holding the (already dedented) body.
type Heredoc struct {
	Base
	Syntax string
	Text   Node
}

// --- names, references, variables -----------------------------------------

// QualifiedName is a bareword (lower-case) name, e.g. a resource type,
// function name or a case/selector value such as `present`.
type QualifiedName struct {
	Base
	Value string
}

// QualifiedReference is a type reference (starts upper-case), e.g. `Integer`
// or `Foo::Bar`.
type QualifiedReference struct {
	Base
	Value string
}

// Variable is `$name` (name has no leading `$`; `::` top-scope kept).
type Variable struct {
	Base
	Name string
}

// --- collections ----------------------------------------------------------

// Array is a `[...]` literal list.
type Array struct {
	Base
	Elements []Node
}

// Hash is a `{...}` literal hash.
type Hash struct {
	Base
	Entries []KeyedEntry
}

// KeyedEntry is one `key => value` pair of a [Hash].
type KeyedEntry struct {
	Key   Node
	Value Node
}

// --- operators ------------------------------------------------------------

// Access is `operand[key, ...]` — element access and, on a
// [QualifiedReference], a parameterized data type such as `Integer[1,10]`.
type Access struct {
	Base
	Operand Node
	Keys    []Node
}

// Unary is a prefix `-x`, `!x` or splat `*x`.
type Unary struct {
	Base
	Op      string
	Operand Node
}

// Binary is an infix operation: arithmetic (`+ - * / %`), shift/append (`<<`),
// comparison (`== != < > <= >=`), match (`=~ !~`), membership (`in`) and
// boolean (`and or`).
type Binary struct {
	Base
	Op          string
	Left, Right Node
}

// Assignment is `target op value` with op `=`, `+=` or `-=`.
type Assignment struct {
	Base
	Op     string
	Target Node
	Value  Node
}

// --- conditionals ---------------------------------------------------------

// Selector is `operand ? { match => value, ... }`.
type Selector struct {
	Base
	Operand Node
	Entries []SelectorEntry
}

// SelectorEntry is one arm of a [Selector].
type SelectorEntry struct {
	Match Node
	Value Node
}

// If is `if/elsif/else`. Else holds either a body, a single nested [If]
// (elsif) or nil.
type If struct {
	Base
	Cond Node
	Then []Node
	Else []Node
}

// Unless is `unless cond { } else { }`.
type Unless struct {
	Base
	Cond Node
	Then []Node
	Else []Node
}

// Case is `case test { values: { body } ... }`.
type Case struct {
	Base
	Test    Node
	Options []CaseOption
}

// CaseOption is one `values : { body }` arm of a [Case].
type CaseOption struct {
	Values []Node
	Body   []Node
}

// --- calls & lambdas ------------------------------------------------------

// Call is a function call. Functor is the callee ([QualifiedName] for a normal
// call). RVal marks a parenthesized (value) call vs a statement-style call
// (`include foo`). Lambda is an optional trailing block.
type Call struct {
	Base
	Functor Node
	Args    []Node
	Lambda  *Lambda
	RVal    bool
}

// MethodCall is `receiver.method(args) |block|` (a `.`-chained call). A bare
// `$x.foo` is a MethodCall with no args.
type MethodCall struct {
	Base
	Receiver Node
	Method   string
	Args     []Node
	Lambda   *Lambda
}

// Lambda is a `|params| { body }` block.
type Lambda struct {
	Base
	Params []Parameter
	Body   []Node
}

// Parameter is one formal parameter: optional data-type, `$name`, optional
// default, and CapturesRest for `*$rest`.
type Parameter struct {
	Type         Node
	Name         string
	Default      Node
	CapturesRest bool
}

// --- resources ------------------------------------------------------------

// ResourceForm distinguishes regular, virtual (`@`) and exported (`@@`)
// resource declarations.
type ResourceForm int

const (
	// Regular is an ordinary resource declaration.
	Regular ResourceForm = iota
	// Virtual is an `@`-prefixed virtual resource.
	Virtual
	// Exported is an `@@`-prefixed exported resource.
	Exported
)

// Resource is a resource declaration `type { title: attr => val ; ... }`.
type Resource struct {
	Base
	Type   Node
	Bodies []ResourceBody
	Form   ResourceForm
}

// ResourceBody is one `title: attributes` clause of a [Resource].
type ResourceBody struct {
	Title Node
	Ops   []AttributeOp
}

// AttributeOp is one attribute operation inside a resource body. Op is `=>`
// or `+>`. Splat marks `* => hash`.
type AttributeOp struct {
	Name  string
	Op    string
	Value Node
	Splat bool
}

// ResourceDefaults is `Type { attr => val }` — defaults for a resource type.
type ResourceDefaults struct {
	Base
	Type Node
	Ops  []AttributeOp
}

// ResourceOverride is `Type[title] { attr => val }` — an override.
type ResourceOverride struct {
	Base
	Resource Node
	Ops      []AttributeOp
}

// Collector is a resource collector: `Type <| query |>` (virtual) or
// `Type <<| query |>>` (exported). Query may be nil for the empty query.
type Collector struct {
	Base
	Type     Node
	Query    Node
	Exported bool
}

// --- definitions ----------------------------------------------------------

// ClassDefinition is `class name (params) inherits parent { body }`.
type ClassDefinition struct {
	Base
	Name   string
	Params []Parameter
	Parent string
	Body   []Node
}

// DefineDefinition is `define name (params) { body }`.
type DefineDefinition struct {
	Base
	Name   string
	Params []Parameter
	Body   []Node
}

// NodeDefinition is `node matches { body }`. Matches are name/regexp/default
// literals.
type NodeDefinition struct {
	Base
	Matches []Node
	Body    []Node
}

// FunctionDefinition is `function name(params) >> ReturnType { body }`.
type FunctionDefinition struct {
	Base
	Name       string
	Params     []Parameter
	ReturnType Node
	Body       []Node
}

// PlanDefinition is a Bolt `plan name(params) { body }`. A plan is like a
// function whose body runs orchestration (run_task/apply/…) rather than
// producing a catalog.
type PlanDefinition struct {
	Base
	Name   string
	Params []Parameter
	Body   []Node
}

// --- relationships --------------------------------------------------------

// Relationship is a chaining operator between two references: `->`, `~>`,
// `<-` or `<~`.
type Relationship struct {
	Base
	Op          string
	Left, Right Node
}
