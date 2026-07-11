// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strconv"

	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/ast"
)

// Block is a Puppet lambda captured with its defining scope, invocable by a
// [Function] during iteration.
type Block struct {
	node  *ast.Lambda
	scope *Scope
	e     *Evaluator
}

// Arity returns the number of declared block parameters.
func (b *Block) Arity() int { return len(b.node.Params) }

// Call invokes the block with positional arguments. A `next()` inside the block
// unwinds to here and yields its value as the block's result; a `break()`
// propagates so the surrounding iterator can stop.
func (b *Block) Call(args ...Value) (Value, error) {
	ls := newScope(b.scope)
	if err := b.e.bindParams(ls, b.node.Params, args); err != nil {
		return nil, err
	}
	v, err := b.e.evalBody(b.node.Body, ls)
	if ns, ok := err.(nextSignal); ok {
		return ns.value, nil
	}
	return v, err
}

func (e *Evaluator) evalCall(x *ast.Call, s *Scope) (Value, error) {
	args, err := e.evalArgs(x.Args, s)
	if err != nil {
		return nil, err
	}
	block := e.blockOf(x.Lambda, s)
	switch fn := x.Functor.(type) {
	case *ast.QualifiedName:
		return e.dispatch(fn.Value, args, block, s, x.Pos())
	case *ast.QualifiedReference:
		return e.typeCast(fn.Value, args, x.Pos())
	}
	return nil, &Error{Pos: x.Pos(), Msg: "invalid call target"}
}

func (e *Evaluator) evalMethodCall(x *ast.MethodCall, s *Scope) (Value, error) {
	recv, err := e.eval(x.Receiver, s)
	if err != nil {
		return nil, err
	}
	args, err := e.evalArgs(x.Args, s)
	if err != nil {
		return nil, err
	}
	block := e.blockOf(x.Lambda, s)
	all := append([]Value{recv}, args...)
	return e.dispatch(x.Method, all, block, s, x.Pos())
}

// evalArgs evaluates a call's argument list, expanding a leading `*` splat of
// an array into individual arguments.
func (e *Evaluator) evalArgs(nodes []ast.Node, s *Scope) ([]Value, error) {
	var args []Value
	for _, a := range nodes {
		if u, ok := a.(*ast.Unary); ok && u.Op == "*" {
			v, err := e.eval(u.Operand, s)
			if err != nil {
				return nil, err
			}
			if arr, ok := normalize(v).([]any); ok {
				args = append(args, arr...)
				continue
			}
			args = append(args, v)
			continue
		}
		v, err := e.eval(a, s)
		if err != nil {
			return nil, err
		}
		args = append(args, v)
	}
	return args, nil
}

func (e *Evaluator) blockOf(l *ast.Lambda, s *Scope) *Block {
	if l == nil {
		return nil
	}
	return &Block{node: l, scope: s, e: e}
}

// dispatch resolves and invokes a named function: built-in/registered first,
// then user-defined `function` definitions.
func (e *Evaluator) dispatch(name string, args []Value, block *Block, s *Scope, pos ast.Position) (Value, error) {
	if fn, ok := e.funcs[name]; ok {
		v, err := fn(&Context{e: e, scope: s}, args, block)
		if err != nil {
			return nil, positioned(err, pos)
		}
		return v, nil
	}
	if def, ok := e.userFuncs[name]; ok {
		return e.callUserFunction(def, args, pos)
	}
	return nil, &Error{Pos: pos, Msg: "unknown function " + name}
}

func (e *Evaluator) callUserFunction(def *ast.FunctionDefinition, args []Value, pos ast.Position) (Value, error) {
	fs := newScope(e.top)
	if err := e.bindParams(fs, def.Params, args); err != nil {
		return nil, positioned(err, pos)
	}
	v, err := e.evalBody(def.Body, fs)
	if rs, ok := err.(returnSignal); ok {
		v, err = rs.value, nil
	}
	if err != nil {
		return nil, err
	}
	if def.ReturnType != nil {
		t, terr := e.evalType(def.ReturnType)
		if terr != nil {
			return nil, terr
		}
		if !pcore.IsInstance(t.(pcore.Type), normalize(v)) {
			return nil, &Error{Pos: pos, Msg: "function " + def.Name + " returned a value not matching " + t.(pcore.Type).String()}
		}
	}
	return v, nil
}

// bindParams binds a parameter list against positional arguments in scope s,
// applying defaults, type checks, and a final `*rest` capture.
func (e *Evaluator) bindParams(s *Scope, params []ast.Parameter, args []Value) error {
	i := 0
	for _, p := range params {
		if p.CapturesRest {
			rest := append([]any{}, args[i:]...)
			i = len(args)
			s.setForce(p.Name, rest)
			continue
		}
		var val Value
		switch {
		case i < len(args):
			val = args[i]
			i++
		case p.Default != nil:
			v, err := e.eval(p.Default, s)
			if err != nil {
				return err
			}
			val = v
		default:
			return &Error{Msg: "missing value for parameter $" + p.Name}
		}
		if err := e.checkParamType(p, val); err != nil {
			return err
		}
		s.setForce(p.Name, val)
	}
	if i < len(args) {
		return &Error{Msg: "too many arguments"}
	}
	return nil
}

func (e *Evaluator) checkParamType(p ast.Parameter, val Value) error {
	if p.Type == nil {
		return nil
	}
	t, err := e.evalType(p.Type)
	if err != nil {
		return err
	}
	if !pcore.IsInstance(t.(pcore.Type), normalize(val)) {
		return &Error{Msg: "parameter $" + p.Name + " expects " + t.(pcore.Type).String() + ", got " + typeName(val)}
	}
	return nil
}

// typeCast implements the type-constructor call form: `Integer("5")`,
// `String(5)`, `Float("1.5")`, `Boolean(0)`, `Array(x)`, `Hash(pairs)`.
func (e *Evaluator) typeCast(name string, args []Value, pos ast.Position) (Value, error) {
	switch name {
	case "Array":
		return typeCastArray(args, pos)
	case "Hash":
		return typeCastHash(args, pos)
	}
	if len(args) != 1 {
		return nil, &Error{Pos: pos, Msg: name + "() expects one argument"}
	}
	v := normalize(args[0])
	switch name {
	case "Sensitive":
		return NewSensitive(v), nil
	case "Integer":
		return toInt(v, pos)
	case "Float":
		if f, ok := asFloat(v); ok {
			return f, nil
		}
		if str, ok := v.(string); ok {
			f, err := strconv.ParseFloat(str, 64)
			if err != nil {
				return nil, &Error{Pos: pos, Msg: "cannot convert to Float: " + str}
			}
			return f, nil
		}
		return nil, &Error{Pos: pos, Msg: "cannot convert " + typeName(v) + " to Float"}
	case "String":
		return stringify(v), nil
	case "Boolean":
		return builtinAny2Bool(nil, args[:1], nil)
	case "Numeric":
		if f, ok := asFloatArg(v); ok {
			if i, ok2 := v.(int64); ok2 {
				return i, nil
			}
			return f, nil
		}
		return nil, &Error{Pos: pos, Msg: "cannot convert " + typeName(v) + " to Numeric"}
	}
	return nil, &Error{Pos: pos, Msg: "type constructor " + name + "() is not supported for " + typeName(v)}
}

// typeCastArray implements Array(x[, wrap]): non-arrays become a single-element
// array (or, for a hash, its [key,value] pairs) unless wrap forces wrapping.
func typeCastArray(args []Value, pos ast.Position) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, &Error{Pos: pos, Msg: "Array() expects one or two arguments"}
	}
	wrap := len(args) == 2 && truthy(args[1])
	if wrap {
		return []any{args[0]}, nil
	}
	return toArray(args[0]), nil
}

// typeCastHash implements Hash(x): an array of [k,v] pairs or a flat [k,v,...]
// list becomes a hash; a hash is returned unchanged.
func typeCastHash(args []Value, pos ast.Position) (Value, error) {
	if len(args) != 1 {
		return nil, &Error{Pos: pos, Msg: "Hash() expects one argument"}
	}
	if h, ok := normalize(args[0]).(map[string]any); ok {
		return h, nil
	}
	if _, ok := normalize(args[0]).([]any); ok {
		return builtinArrayToHash(nil, args, nil)
	}
	return nil, &Error{Pos: pos, Msg: "cannot convert " + typeName(args[0]) + " to Hash"}
}

func toInt(v Value, pos ast.Position) (Value, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case float64:
		return int64(x), nil
	case string:
		n, err := strconv.ParseInt(x, 0, 64)
		if err != nil {
			return nil, &Error{Pos: pos, Msg: "cannot convert to Integer: " + x}
		}
		return n, nil
	}
	return nil, &Error{Pos: pos, Msg: "cannot convert " + typeName(v) + " to Integer"}
}

// positioned attaches pos to an *Error that has none.
func positioned(err error, pos ast.Position) error {
	if ee, ok := err.(*Error); ok && ee.Pos.Line == 0 {
		ee.Pos = pos
	}
	return err
}
