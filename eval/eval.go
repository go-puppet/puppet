// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package eval evaluates a parsed Puppet manifest ([ast.Program]) into a
// [catalog.Catalog]. It implements Puppet scoping, expression semantics,
// conditionals, iteration, class/defined-type instantiation, resource
// declaration and relationship chaining, and a built-in function set. The type
// system is delegated to [github.com/go-pcore/pcore], data binding to
// [github.com/go-hiera/hiera], and facts to an injectable provider (backed by
// [github.com/go-facter/facter] by default).
//
// A function/type registry seam ([Evaluator.RegisterFunction]) lets a host —
// notably go-ruby-puppet — contribute Ruby-defined custom functions.
package eval

import (
	"fmt"
	"strings"

	"github.com/go-hiera/hiera"
	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/catalog"
	"github.com/go-puppet/puppet/parser"
)

// Error is an evaluation error, optionally carrying a source position.
type Error struct {
	Pos ast.Position
	Msg string
}

// Error implements error.
func (e *Error) Error() string {
	if e.Pos.Line == 0 {
		return "evaluation error: " + e.Msg
	}
	return fmt.Sprintf("evaluation error at %s: %s", e.Pos, e.Msg)
}

// LogEntry is one message emitted by a logging function (notice/info/…).
type LogEntry struct {
	Level   string
	Message string
}

// FactsProvider supplies top-scope facts. A bare fact name ("os") or a dotted
// path ("os.family") resolves through Fact; Facts returns the whole tree for
// `$facts`.
type FactsProvider interface {
	Fact(name string) (Value, bool)
	Facts() map[string]any
}

// Function is a callable contributed to the evaluator. args are the already
// evaluated positional arguments; block is the optional trailing lambda (nil
// when absent).
type Function func(c *Context, args []Value, block *Block) (Value, error)

// Context is handed to every [Function]. It exposes the evaluator services a
// function needs without widening the public surface.
type Context struct {
	e     *Evaluator
	scope *Scope
}

// Log records a message at the given level.
func (c *Context) Log(level, msg string) { c.e.logs = append(c.e.logs, LogEntry{level, msg}) }

// Catalog returns the catalog being built.
func (c *Context) Catalog() *catalog.Catalog { return c.e.cat }

// Lookup resolves key through the configured Hiera, if any.
func (c *Context) Lookup(key string) (Value, bool, error) { return c.e.lookup(key, c.scope) }

// Evaluator holds the state of one compilation.
type Evaluator struct {
	top          *Scope
	cat          *catalog.Catalog
	funcs        map[string]Function
	classes      map[string]*ast.ClassDefinition
	defines      map[string]*ast.DefineDefinition
	userFuncs    map[string]*ast.FunctionDefinition
	nodes        []*ast.NodeDefinition
	included     map[string]bool
	logs         []LogEntry
	hiera        *hiera.Hiera
	facts        FactsProvider
	exported     ExportedStore
	templates    TemplateLoader
	erb          ERBRenderer
	eppStack     []*strings.Builder
	nodeName     string
	curContainer string
}

// lookup resolves key through the configured Hiera, feeding the current scope's
// variables and facts in as the interpolation scope. It reports (undef, false)
// when no Hiera is configured or the key is absent.
func (e *Evaluator) lookup(key string, s *Scope) (Value, bool, error) {
	if e.hiera == nil {
		return pcore.Undef, false, nil
	}
	v, ok, err := e.hiera.Lookup(key, nil)
	if err != nil {
		return nil, false, &Error{Msg: err.Error()}
	}
	if !ok {
		return pcore.Undef, false, nil
	}
	return toValue(v), true, nil
}

// Option configures a new [Evaluator].
type Option func(*Evaluator)

// WithFacts sets the facts provider (default: none — `$facts` is undef).
func WithFacts(f FactsProvider) Option { return func(e *Evaluator) { e.facts = f } }

// WithHiera wires a Hiera engine to back lookup().
func WithHiera(h *hiera.Hiera) Option { return func(e *Evaluator) { e.hiera = h } }

// WithNodeName sets the compiling node's name (default "default").
func WithNodeName(name string) Option { return func(e *Evaluator) { e.nodeName = name } }

// WithExportedStore wires a backing store for exported resources (`@@`), so
// they can be collected on other nodes with `<<| |>>`. Without one, exported
// resources are only collectable within the same compilation.
func WithExportedStore(store ExportedStore) Option {
	return func(e *Evaluator) { e.exported = store }
}

// New builds an [Evaluator] with the built-in functions registered.
func New(opts ...Option) *Evaluator {
	e := &Evaluator{
		funcs:     map[string]Function{},
		classes:   map[string]*ast.ClassDefinition{},
		defines:   map[string]*ast.DefineDefinition{},
		userFuncs: map[string]*ast.FunctionDefinition{},
		included:  map[string]bool{},
		nodeName:  "default",
	}
	for _, o := range opts {
		o(e)
	}
	e.top = newScope(nil)
	e.cat = catalog.New(e.nodeName)
	registerBuiltins(e)
	registerStdlib(e)
	registerEPPRenderers(e)
	registerTemplateFns(e)
	e.installFacts()
	return e
}

// RegisterFunction adds or replaces a named function. This is the seam a host
// (e.g. go-ruby-puppet) uses to contribute custom functions.
func (e *Evaluator) RegisterFunction(name string, fn Function) { e.funcs[name] = fn }

// Logs returns the messages emitted during evaluation.
func (e *Evaluator) Logs() []LogEntry { return e.logs }

// installFacts binds `$facts` and each top-level fact into the top scope.
func (e *Evaluator) installFacts() {
	if e.facts == nil {
		return
	}
	e.top.setForce("facts", toValue(e.facts.Facts()))
	for k, v := range e.facts.Facts() {
		e.top.setForce(k, toValue(v))
	}
}

// EvalString parses and evaluates src, returning the resulting catalog.
func EvalString(src string, opts ...Option) (*catalog.Catalog, []LogEntry, error) {
	prog, err := parser.Parse(src)
	if err != nil {
		return nil, nil, err
	}
	e := New(opts...)
	cat, err := e.EvalProgram(prog)
	return cat, e.Logs(), err
}

// EvalProgram evaluates a whole program: definitions are registered first
// (hoisted) so forward references resolve, then top-level statements run in
// order.
func (e *Evaluator) EvalProgram(prog *ast.Program) (*catalog.Catalog, error) {
	for _, n := range prog.Body {
		e.register(n)
	}
	for _, n := range prog.Body {
		if isDefinition(n) {
			continue
		}
		if _, err := e.eval(n, e.top); err != nil {
			return nil, err
		}
	}
	if err := e.applyMatchingNode(); err != nil {
		return nil, err
	}
	return e.cat, nil
}

// eval evaluates one expression node in scope.
func (e *Evaluator) eval(n ast.Node, s *Scope) (Value, error) {
	switch x := n.(type) {
	case *ast.Undef:
		return pcore.Undef, nil
	case *ast.Default:
		return pcore.Default, nil
	case *ast.Boolean:
		return x.Value, nil
	case *ast.Integer:
		return x.Value, nil
	case *ast.Float:
		return x.Value, nil
	case *ast.String:
		return x.Value, nil
	case *ast.Concat:
		return e.evalConcat(x, s)
	case *ast.Heredoc:
		return e.eval(x.Text, s)
	case *ast.Regexp:
		return pcore.NewRegexp(x.Value)
	case *ast.QualifiedName:
		return x.Value, nil
	case *ast.QualifiedReference:
		return pcore.Parse(x.Value)
	case *ast.Variable:
		if v, ok := s.lookup(x.Name); ok {
			return v, nil
		}
		return pcore.Undef, nil
	case *ast.Array:
		return e.evalArray(x, s)
	case *ast.Hash:
		return e.evalHash(x, s)
	case *ast.Access:
		return e.evalAccess(x, s)
	case *ast.Unary:
		return e.evalUnary(x, s)
	case *ast.Binary:
		return e.evalBinary(x, s)
	case *ast.Assignment:
		return e.evalAssignment(x, s)
	case *ast.Selector:
		return e.evalSelector(x, s)
	case *ast.If:
		return e.evalIf(x.Cond, x.Then, x.Else, false, s)
	case *ast.Unless:
		return e.evalIf(x.Cond, x.Then, x.Else, true, s)
	case *ast.Case:
		return e.evalCase(x, s)
	case *ast.Call:
		return e.evalCall(x, s)
	case *ast.MethodCall:
		return e.evalMethodCall(x, s)
	case *ast.Resource:
		return e.evalResource(x, s)
	case *ast.Relationship:
		return e.evalRelationship(x, s)
	case *ast.ResourceDefaults:
		return e.evalResourceDefaults(x, s)
	case *ast.ResourceOverride:
		return e.evalResourceOverride(x, s)
	case *ast.Collector:
		return e.evalCollector(x, s)
	}
	return nil, &Error{Pos: n.Pos(), Msg: fmt.Sprintf("cannot evaluate %T", n)}
}

// evalBody evaluates a sequence of statements, returning the last value.
func (e *Evaluator) evalBody(body []ast.Node, s *Scope) (Value, error) {
	var last Value = pcore.Undef
	for _, n := range body {
		v, err := e.eval(n, s)
		if err != nil {
			return nil, err
		}
		last = v
	}
	return last, nil
}

func (e *Evaluator) evalConcat(x *ast.Concat, s *Scope) (Value, error) {
	var b []byte
	for _, part := range x.Parts {
		v, err := e.eval(part, s)
		if err != nil {
			return nil, err
		}
		b = append(b, stringify(v)...)
	}
	return string(b), nil
}

func (e *Evaluator) evalArray(x *ast.Array, s *Scope) (Value, error) {
	out := make([]any, 0, len(x.Elements))
	for _, el := range x.Elements {
		if u, ok := el.(*ast.Unary); ok && u.Op == "*" {
			v, err := e.eval(u.Operand, s)
			if err != nil {
				return nil, err
			}
			if arr, ok := v.([]any); ok {
				out = append(out, arr...)
				continue
			}
			out = append(out, v)
			continue
		}
		v, err := e.eval(el, s)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (e *Evaluator) evalHash(x *ast.Hash, s *Scope) (Value, error) {
	out := map[string]any{}
	for _, entry := range x.Entries {
		k, err := e.eval(entry.Key, s)
		if err != nil {
			return nil, err
		}
		v, err := e.eval(entry.Value, s)
		if err != nil {
			return nil, err
		}
		out[stringify(k)] = v
	}
	return out, nil
}

// toValue converts a decoded facts/hiera value (which may use []any /
// map[string]any and native scalar kinds) into the evaluator's value model.
func toValue(v any) Value {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			out[k] = toValue(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = toValue(e)
		}
		return out
	default:
		return normalize(v)
	}
}
