// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/ast"
)

// registerBuiltins installs the core Puppet function set. Custom functions
// (e.g. from go-ruby-puppet) are added later via [Evaluator.RegisterFunction]
// and can override any of these.
func registerBuiltins(e *Evaluator) {
	for _, level := range []string{"notice", "info", "warning", "err", "debug", "alert", "crit", "emerg"} {
		lvl := level
		e.funcs[lvl] = func(c *Context, args []Value, _ *Block) (Value, error) {
			c.Log(lvl, concatArgs(args))
			return pcore.Undef, nil
		}
	}
	e.funcs["fail"] = func(_ *Context, args []Value, _ *Block) (Value, error) {
		return nil, &Error{Msg: concatArgs(args)}
	}
	for _, kw := range []string{"include", "require", "contain"} {
		e.funcs[kw] = builtinInclude
	}
	e.funcs["lookup"] = builtinLookup
	e.funcs["assert_type"] = builtinAssertType
	e.funcs["type"] = builtinType
	e.funcs["length"] = builtinSize
	e.funcs["size"] = builtinSize
	e.funcs["empty"] = builtinEmpty
	// upcase/downcase/capitalize/strip are registered (with Array support) by
	// registerStdlibString.
	e.funcs["split"] = builtinSplit
	e.funcs["join"] = builtinJoin
	e.funcs["sprintf"] = builtinSprintf
	e.funcs["keys"] = builtinKeys
	e.funcs["values"] = builtinValues
	e.funcs["merge"] = builtinMerge
	e.funcs["reverse"] = builtinReverse
	e.funcs["abs"] = builtinAbs
	e.funcs["min"] = builtinMinMax(false)
	e.funcs["max"] = builtinMinMax(true)
	e.funcs["each"] = builtinEach
	e.funcs["map"] = builtinMap
	e.funcs["filter"] = builtinFilter
	e.funcs["reduce"] = builtinReduce
	e.funcs["with"] = builtinWith
	e.funcs["slice"] = builtinSlice
}

// concatArgs renders and concatenates arguments the way notice()/fail() do.
func concatArgs(args []Value) string {
	var b strings.Builder
	for _, a := range args {
		b.WriteString(stringify(a))
	}
	return b.String()
}

func need(args []Value, n int, name string) error {
	if len(args) != n {
		return &Error{Msg: fmt.Sprintf("%s() expects %d argument(s), got %d", name, n, len(args))}
	}
	return nil
}

func builtinInclude(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) == 0 {
		return nil, &Error{Msg: "include/require/contain expects at least one class name"}
	}
	for _, a := range args {
		for _, name := range classNames(a) {
			if err := c.e.declareClass(name, nil, ast.Position{}); err != nil {
				return nil, err
			}
		}
	}
	return pcore.Undef, nil
}

// classNames extracts class names from an include argument (a string, an array
// of strings, or a Class[...] reference).
func classNames(v Value) []string {
	switch x := normalize(v).(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, classNames(e)...)
		}
		return out
	case *ResourceRef:
		if x.Type == "Class" {
			return []string{strings.ToLower(x.Title)}
		}
	}
	return nil
}

func builtinLookup(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, &Error{Msg: "lookup() expects a key"}
	}
	key, ok := normalize(args[0]).(string)
	if !ok {
		return nil, &Error{Msg: "lookup() key must be a string"}
	}
	v, found, err := c.Lookup(key)
	if err != nil {
		return nil, err
	}
	if found {
		return v, nil
	}
	if len(args) >= 2 {
		return args[1], nil // default value
	}
	return nil, &Error{Msg: "lookup() did not find a value for '" + key + "' and no default was given"}
}

func builtinAssertType(_ *Context, args []Value, _ *Block) (Value, error) {
	if err := need(args, 2, "assert_type"); err != nil {
		return nil, err
	}
	t, ok := args[0].(pcore.Type)
	if !ok {
		return nil, &Error{Msg: "assert_type() first argument must be a Type"}
	}
	if !pcore.IsInstance(t, normalize(args[1])) {
		return nil, &Error{Msg: "assert_type(): value is not an instance of " + t.String()}
	}
	return args[1], nil
}

func builtinType(_ *Context, args []Value, _ *Block) (Value, error) {
	if err := need(args, 1, "type"); err != nil {
		return nil, err
	}
	return pcore.Infer(normalize(args[0])), nil
}

func builtinSize(_ *Context, args []Value, _ *Block) (Value, error) {
	if err := need(args, 1, "size"); err != nil {
		return nil, err
	}
	switch x := normalize(args[0]).(type) {
	case string:
		return int64(len([]rune(x))), nil
	case []any:
		return int64(len(x)), nil
	case map[string]any:
		return int64(len(x)), nil
	}
	return nil, &Error{Msg: "size() expects a String, Array or Hash"}
}

func builtinEmpty(_ *Context, args []Value, _ *Block) (Value, error) {
	if err := need(args, 1, "empty"); err != nil {
		return nil, err
	}
	switch x := normalize(args[0]).(type) {
	case string:
		return len(x) == 0, nil
	case []any:
		return len(x) == 0, nil
	case map[string]any:
		return len(x) == 0, nil
	}
	if isUndef(args[0]) {
		return true, nil
	}
	return nil, &Error{Msg: "empty() expects a String, Array, Hash or Undef"}
}

func capitalizeWord(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + strings.ToLower(string(r[1:]))
}

func builtinSplit(_ *Context, args []Value, _ *Block) (Value, error) {
	if err := need(args, 2, "split"); err != nil {
		return nil, err
	}
	s, ok := normalize(args[0]).(string)
	if !ok {
		return nil, &Error{Msg: "split() first argument must be a String"}
	}
	var parts []string
	switch sep := args[1].(type) {
	case string:
		parts = strings.Split(s, sep)
	case *pcore.Regexp:
		// The pattern already compiled when the Regexp value was created.
		rx := regexp.MustCompile(sep.Source())
		parts = rx.Split(s, -1)
	default:
		return nil, &Error{Msg: "split() separator must be a String or Regexp"}
	}
	return toAnySlice(parts), nil
}

func builtinJoin(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, &Error{Msg: "join() expects an Array and an optional separator"}
	}
	arr, ok := normalize(args[0]).([]any)
	if !ok {
		return nil, &Error{Msg: "join() first argument must be an Array"}
	}
	sep := ""
	if len(args) == 2 {
		sep = stringify(args[1])
	}
	parts := make([]string, len(arr))
	for i, e := range arr {
		parts[i] = stringify(e)
	}
	return strings.Join(parts, sep), nil
}

func builtinSprintf(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, &Error{Msg: "sprintf() expects a format string"}
	}
	format, ok := normalize(args[0]).(string)
	if !ok {
		return nil, &Error{Msg: "sprintf() format must be a String"}
	}
	rest := make([]any, len(args)-1)
	for i, a := range args[1:] {
		rest[i] = normalize(a)
	}
	return fmt.Sprintf(format, rest...), nil
}

func builtinKeys(_ *Context, args []Value, _ *Block) (Value, error) {
	if err := need(args, 1, "keys"); err != nil {
		return nil, err
	}
	m, ok := normalize(args[0]).(map[string]any)
	if !ok {
		return nil, &Error{Msg: "keys() expects a Hash"}
	}
	out := []any{}
	for _, k := range sortedKeys(m) {
		out = append(out, k)
	}
	return out, nil
}

func builtinValues(_ *Context, args []Value, _ *Block) (Value, error) {
	if err := need(args, 1, "values"); err != nil {
		return nil, err
	}
	m, ok := normalize(args[0]).(map[string]any)
	if !ok {
		return nil, &Error{Msg: "values() expects a Hash"}
	}
	out := []any{}
	for _, k := range sortedKeys(m) {
		out = append(out, m[k])
	}
	return out, nil
}

func builtinMerge(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 {
		return nil, &Error{Msg: "merge() expects at least two Hashes"}
	}
	out := map[string]any{}
	for _, a := range args {
		m, ok := normalize(a).(map[string]any)
		if !ok {
			return nil, &Error{Msg: "merge() arguments must be Hashes"}
		}
		for k, v := range m {
			out[k] = v
		}
	}
	return out, nil
}

func builtinReverse(_ *Context, args []Value, _ *Block) (Value, error) {
	if err := need(args, 1, "reverse"); err != nil {
		return nil, err
	}
	switch x := normalize(args[0]).(type) {
	case string:
		r := []rune(x)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		return string(r), nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[len(x)-1-i] = e
		}
		return out, nil
	}
	return nil, &Error{Msg: "reverse() expects a String or Array"}
}

func builtinAbs(_ *Context, args []Value, _ *Block) (Value, error) {
	if err := need(args, 1, "abs"); err != nil {
		return nil, err
	}
	switch x := normalize(args[0]).(type) {
	case int64:
		if x < 0 {
			return -x, nil
		}
		return x, nil
	case float64:
		return math.Abs(x), nil
	}
	return nil, &Error{Msg: "abs() expects a Numeric"}
}

func builtinMinMax(wantMax bool) Function {
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) == 0 {
			return nil, &Error{Msg: "min()/max() expects at least one argument"}
		}
		best := args[0]
		for _, a := range args[1:] {
			c, err := compare(a, best)
			if err != nil {
				return nil, &Error{Msg: err.Error()}
			}
			if (wantMax && c > 0) || (!wantMax && c < 0) {
				best = a
			}
		}
		return best, nil
	}
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
