// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/ast"
)

func (e *Evaluator) evalUnary(x *ast.Unary, s *Scope) (Value, error) {
	v, err := e.eval(x.Operand, s)
	if err != nil {
		return nil, err
	}
	switch x.Op {
	case "!":
		return !truthy(v), nil
	case "-":
		switch n := normalize(v).(type) {
		case int64:
			return -n, nil
		case float64:
			return -n, nil
		}
		return nil, &Error{Pos: x.Pos(), Msg: "cannot negate " + typeName(v)}
	default: // "*"
		return nil, &Error{Pos: x.Pos(), Msg: "splat (*) is only valid in an argument or array"}
	}
}

func (e *Evaluator) evalBinary(x *ast.Binary, s *Scope) (Value, error) {
	// Short-circuit boolean operators.
	switch x.Op {
	case "and":
		l, err := e.eval(x.Left, s)
		if err != nil {
			return nil, err
		}
		if !truthy(l) {
			return false, nil
		}
		r, err := e.eval(x.Right, s)
		if err != nil {
			return nil, err
		}
		return truthy(r), nil
	case "or":
		l, err := e.eval(x.Left, s)
		if err != nil {
			return nil, err
		}
		if truthy(l) {
			return true, nil
		}
		r, err := e.eval(x.Right, s)
		if err != nil {
			return nil, err
		}
		return truthy(r), nil
	}

	l, err := e.eval(x.Left, s)
	if err != nil {
		return nil, err
	}
	r, err := e.eval(x.Right, s)
	if err != nil {
		return nil, err
	}
	switch x.Op {
	case "==":
		return equals(l, r), nil
	case "!=":
		return !equals(l, r), nil
	case "<", ">", "<=", ">=":
		return e.evalCompare(x.Op, l, r, x.Pos())
	case "=~":
		return e.evalMatch(l, r, s, x.Pos())
	case "!~":
		m, err := e.evalMatch(l, r, s, x.Pos())
		if err != nil {
			return nil, err
		}
		return !m.(bool), nil
	case "in":
		return e.evalIn(l, r), nil
	case "+", "-", "*", "/", "%", "<<":
		return e.evalArith(x.Op, l, r, x.Pos())
	}
	return nil, &Error{Pos: x.Pos(), Msg: "unknown operator " + x.Op}
}

func (e *Evaluator) evalCompare(op string, l, r Value, pos ast.Position) (Value, error) {
	c, err := compare(l, r)
	if err != nil {
		return nil, &Error{Pos: pos, Msg: err.Error()}
	}
	switch op {
	case "<":
		return c < 0, nil
	case ">":
		return c > 0, nil
	case "<=":
		return c <= 0, nil
	default: // ">="
		return c >= 0, nil
	}
}

func (e *Evaluator) evalMatch(l, r Value, s *Scope, pos ast.Position) (Value, error) {
	switch m := r.(type) {
	case *pcore.Regexp:
		str, ok := normalize(l).(string)
		if !ok {
			return nil, &Error{Pos: pos, Msg: "=~ requires a string on the left"}
		}
		return e.regexpMatch(m.Source(), str, s, pos)
	case string:
		// A string on the right of =~ is treated as a regexp pattern.
		str, ok := normalize(l).(string)
		if !ok {
			return nil, &Error{Pos: pos, Msg: "=~ requires a string on the left"}
		}
		return e.regexpMatch(m, str, s, pos)
	case pcore.Type:
		return pcore.IsInstance(m, normalize(l)), nil
	}
	return nil, &Error{Pos: pos, Msg: "=~ requires a Regexp, String or Type on the right"}
}

// regexpMatch compiles pattern, matches str, and (on success) binds the numbered
// capture variables $0..$n into scope s, matching Puppet's regex-match scoping.
func (e *Evaluator) regexpMatch(pattern, str string, s *Scope, pos ast.Position) (Value, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, &Error{Pos: pos, Msg: "invalid regular expression: " + err.Error()}
	}
	return matchAndCapture(re, str, s), nil
}

// matchAndCapture runs re against str and, when it matches, binds the numbered
// capture variables $0..$n into scope s (clearing them on a non-match).
func matchAndCapture(re *regexp.Regexp, str string, s *Scope) bool {
	groups := re.FindStringSubmatch(str)
	if s != nil {
		s.setMatch(groups)
	}
	return groups != nil
}

func (e *Evaluator) evalIn(l, r Value) Value {
	switch c := normalize(r).(type) {
	case []any:
		for _, el := range c {
			if equals(l, el) {
				return true
			}
		}
		return false
	case map[string]any:
		_, ok := c[stringify(l)]
		return ok
	case string:
		sub, ok := normalize(l).(string)
		return ok && strings.Contains(c, sub)
	}
	return false
}

func (e *Evaluator) evalArith(op string, l, r Value, pos ast.Position) (Value, error) {
	if op == "+" {
		if la, ok := normalize(l).([]any); ok {
			if ra, ok := normalize(r).([]any); ok {
				return append(append([]any{}, la...), ra...), nil
			}
		}
		if lm, ok := normalize(l).(map[string]any); ok {
			if rm, ok := normalize(r).(map[string]any); ok {
				return mergeHash(lm, rm), nil
			}
		}
	}
	if op == "<<" {
		if la, ok := normalize(l).([]any); ok {
			return append(append([]any{}, la...), r), nil
		}
		return nil, &Error{Pos: pos, Msg: "<< requires an array on the left"}
	}
	return numericArith(op, l, r, pos)
}

func mergeHash(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func numericArith(op string, l, r Value, pos ast.Position) (Value, error) {
	li, lIsInt := normalize(l).(int64)
	ri, rIsInt := normalize(r).(int64)
	if lIsInt && rIsInt {
		return intArith(op, li, ri, pos)
	}
	lf, lok := asFloat(l)
	rf, rok := asFloat(r)
	if !lok || !rok {
		return nil, &Error{Pos: pos, Msg: "arithmetic requires numeric operands"}
	}
	switch op {
	case "+":
		return lf + rf, nil
	case "-":
		return lf - rf, nil
	case "*":
		return lf * rf, nil
	case "/":
		if rf == 0 {
			return nil, &Error{Pos: pos, Msg: "division by zero"}
		}
		return lf / rf, nil
	default: // "%"
		return nil, &Error{Pos: pos, Msg: "modulo requires integer operands"}
	}
}

func intArith(op string, l, r int64, pos ast.Position) (Value, error) {
	switch op {
	case "+":
		return l + r, nil
	case "-":
		return l - r, nil
	case "*":
		return l * r, nil
	case "/":
		if r == 0 {
			return nil, &Error{Pos: pos, Msg: "division by zero"}
		}
		return l / r, nil
	default: // "%"
		if r == 0 {
			return nil, &Error{Pos: pos, Msg: "division by zero"}
		}
		return l % r, nil
	}
}

func (e *Evaluator) evalAssignment(x *ast.Assignment, s *Scope) (Value, error) {
	if x.Op != "=" {
		// The `+=` and `-=` append-assignment operators were removed in Puppet 4;
		// only plain `=` assignment is a valid Puppet expression.
		return nil, &Error{Pos: x.Pos(), Msg: "the " + x.Op + " operator is not supported in Puppet (removed in Puppet 4); use $x = $x + ... instead"}
	}
	v, err := e.eval(x.Value, s)
	if err != nil {
		return nil, err
	}
	vr, ok := x.Target.(*ast.Variable)
	if !ok {
		return nil, &Error{Pos: x.Pos(), Msg: "assignment target must be a variable"}
	}
	if err := s.set(vr.Name, v); err != nil {
		err.(*Error).Pos = x.Pos()
		return nil, err
	}
	return v, nil
}

func (e *Evaluator) evalAccess(x *ast.Access, s *Scope) (Value, error) {
	if isTypeExpr(x) {
		t, terr := e.evalType(x)
		if terr == nil {
			return t, nil
		}
		// A parameterized capitalized name that is not a known Pcore type is a
		// resource reference, e.g. `Service['nginx']`.
		if ref, ok, rerr := e.resourceRefFrom(x, s); ok {
			return ref, rerr
		}
		return nil, terr
	}
	operand, err := e.eval(x.Operand, s)
	if err != nil {
		return nil, err
	}
	keys := make([]Value, len(x.Keys))
	for i, k := range x.Keys {
		kv, err := e.eval(k, s)
		if err != nil {
			return nil, err
		}
		keys[i] = kv
	}
	return indexValue(operand, keys, x.Pos())
}

func indexValue(operand Value, keys []Value, pos ast.Position) (Value, error) {
	switch c := normalize(operand).(type) {
	case []any:
		return indexArray(c, keys, pos)
	case map[string]any:
		if v, ok := c[stringify(keys[0])]; ok {
			return v, nil
		}
		return pcore.Undef, nil
	case string:
		return indexString(c, keys, pos)
	}
	return nil, &Error{Pos: pos, Msg: "cannot index " + typeName(operand)}
}

func indexArray(a []any, keys []Value, pos ast.Position) (Value, error) {
	idx, err := intKey(keys[0], pos)
	if err != nil {
		return nil, err
	}
	if idx < 0 {
		idx += int64(len(a))
	}
	if len(keys) == 2 {
		count, err := intKey(keys[1], pos)
		if err != nil {
			return nil, err
		}
		return sliceArray(a, idx, count), nil
	}
	if idx < 0 || idx >= int64(len(a)) {
		return pcore.Undef, nil
	}
	return a[idx], nil
}

func sliceArray(a []any, start, count int64) []any {
	if start < 0 {
		start = 0
	}
	end := start + count
	if end > int64(len(a)) {
		end = int64(len(a))
	}
	if start > int64(len(a)) {
		start = int64(len(a))
	}
	return append([]any{}, a[start:end]...)
}

func indexString(str string, keys []Value, pos ast.Position) (Value, error) {
	rs := []rune(str)
	idx, err := intKey(keys[0], pos)
	if err != nil {
		return nil, err
	}
	if idx < 0 {
		idx += int64(len(rs))
	}
	if idx < 0 || idx >= int64(len(rs)) {
		return "", nil
	}
	return string(rs[idx]), nil
}

func intKey(v Value, pos ast.Position) (int64, error) {
	if i, ok := normalize(v).(int64); ok {
		return i, nil
	}
	return 0, &Error{Pos: pos, Msg: "index must be an integer"}
}

func (e *Evaluator) evalSelector(x *ast.Selector, s *Scope) (Value, error) {
	test, err := e.eval(x.Operand, s)
	if err != nil {
		return nil, err
	}
	var dflt *ast.SelectorEntry
	for i := range x.Entries {
		entry := &x.Entries[i]
		if _, ok := entry.Match.(*ast.Default); ok {
			dflt = entry
			continue
		}
		m, err := e.matches(test, entry.Match, s)
		if err != nil {
			return nil, err
		}
		if m {
			return e.eval(entry.Value, s)
		}
	}
	if dflt != nil {
		return e.eval(dflt.Value, s)
	}
	return nil, &Error{Pos: x.Pos(), Msg: "no matching selector entry and no default"}
}

func (e *Evaluator) evalIf(cond ast.Node, then, els []ast.Node, invert bool, s *Scope) (Value, error) {
	c, err := e.eval(cond, s)
	if err != nil {
		return nil, err
	}
	take := truthy(c)
	if invert {
		take = !take
	}
	if take {
		return e.evalBody(then, newScope(s))
	}
	return e.evalBody(els, newScope(s))
}

func (e *Evaluator) evalCase(x *ast.Case, s *Scope) (Value, error) {
	test, err := e.eval(x.Test, s)
	if err != nil {
		return nil, err
	}
	var dflt *ast.CaseOption
	for i := range x.Options {
		opt := &x.Options[i]
		for _, val := range opt.Values {
			if _, ok := val.(*ast.Default); ok {
				dflt = opt
				continue
			}
			m, err := e.matches(test, val, s)
			if err != nil {
				return nil, err
			}
			if m {
				return e.evalBody(opt.Body, newScope(s))
			}
		}
	}
	if dflt != nil {
		return e.evalBody(dflt.Body, newScope(s))
	}
	return pcore.Undef, nil
}

// matches reports whether test matches the match expression: type-instance for
// a Type, regexp match for a Regexp, else value equality.
func (e *Evaluator) matches(test Value, matchNode ast.Node, s *Scope) (bool, error) {
	m, err := e.eval(matchNode, s)
	if err != nil {
		return false, err
	}
	switch mv := m.(type) {
	case pcore.Type:
		return pcore.IsInstance(mv, normalize(test)), nil
	case *pcore.Regexp:
		if str, ok := normalize(test).(string); ok {
			// mv is already compiled, so recompiling its source cannot fail.
			re, _ := regexp.Compile(mv.Source())
			return matchAndCapture(re, str, s), nil
		}
		return false, nil
	}
	return equals(test, m), nil
}

// --- data-type expressions ------------------------------------------------

// isTypeExpr reports whether node denotes a parameterized data type
// (`Integer[1,10]`, `Optional[String]`, …).
func isTypeExpr(node ast.Node) bool {
	switch x := node.(type) {
	case *ast.QualifiedReference:
		return true
	case *ast.Access:
		return isTypeExpr(x.Operand)
	}
	return false
}

func (e *Evaluator) evalType(node ast.Node) (Value, error) {
	src, err := renderType(node)
	if err != nil {
		return nil, err
	}
	return pcore.Parse(src)
}

// renderType renders a data-type AST expression to its Pcore source form so it
// can be parsed by go-pcore.
func renderType(node ast.Node) (string, error) {
	switch x := node.(type) {
	case *ast.QualifiedReference:
		return x.Value, nil
	case *ast.Access:
		base, err := renderType(x.Operand)
		if err != nil {
			return "", err
		}
		parts := make([]string, len(x.Keys))
		for i, k := range x.Keys {
			p, err := renderTypeArg(k)
			if err != nil {
				return "", err
			}
			parts[i] = p
		}
		return base + "[" + strings.Join(parts, ", ") + "]", nil
	}
	return "", &Error{Pos: node.Pos(), Msg: "invalid type expression"}
}

func renderTypeArg(node ast.Node) (string, error) {
	switch x := node.(type) {
	case *ast.Integer:
		return strconv.FormatInt(x.Value, 10), nil
	case *ast.Float:
		return strconv.FormatFloat(x.Value, 'g', -1, 64), nil
	case *ast.String:
		return "'" + strings.ReplaceAll(x.Value, "'", "\\'") + "'", nil
	case *ast.Default:
		return "default", nil
	case *ast.Unary:
		if x.Op == "-" {
			inner, err := renderTypeArg(x.Operand)
			if err != nil {
				return "", err
			}
			return "-" + inner, nil
		}
	case *ast.QualifiedReference, *ast.Access:
		return renderType(x)
	}
	return "", &Error{Pos: node.Pos(), Msg: "invalid type argument"}
}
