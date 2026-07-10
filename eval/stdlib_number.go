// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"math"
	"strconv"
	"strings"
)

// registerStdlibNumber installs the numeric function set.
func registerStdlibNumber(e *Evaluator) {
	e.funcs["ceiling"] = numFn1(math.Ceil, "ceiling")
	e.funcs["floor"] = numFn1(math.Floor, "floor")
	e.funcs["round"] = builtinRound
	e.funcs["sqrt"] = floatFn1(math.Sqrt, "sqrt")
	e.funcs["clamp"] = builtinClamp
	e.funcs["sum"] = builtinSum
	e.funcs["to_bytes"] = builtinToBytes
	e.funcs["pw_hash"] = builtinPwHash
}

// numFn1 wraps a float→float transform that returns an Integer result (ceiling,
// floor).
func numFn1(f func(float64) float64, name string) Function {
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) != 1 {
			return nil, wrongArgs(name)
		}
		v, ok := asFloatArg(args[0])
		if !ok {
			return nil, &Error{Msg: name + "(): expects a Numeric"}
		}
		return int64(f(v)), nil
	}
}

// floatFn1 wraps a float→float transform returning a Float (sqrt).
func floatFn1(f func(float64) float64, name string) Function {
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) != 1 {
			return nil, wrongArgs(name)
		}
		v, ok := asFloatArg(args[0])
		if !ok {
			return nil, &Error{Msg: name + "(): expects a Numeric"}
		}
		return f(v), nil
	}
}

// asFloatArg accepts a Numeric or numeric String.
func asFloatArg(v Value) (float64, bool) {
	if f, ok := asFloat(v); ok {
		return f, true
	}
	if s, ok := normalize(v).(string); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func builtinRound(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("round")
	}
	if i, ok := normalize(args[0]).(int64); ok {
		return i, nil
	}
	f, ok := asFloatArg(args[0])
	if !ok {
		return nil, &Error{Msg: "round(): expects a Numeric"}
	}
	return int64(math.Round(f)), nil
}

func builtinClamp(_ *Context, args []Value, _ *Block) (Value, error) {
	vals := args
	if len(args) == 1 {
		a, err := argArr(args, 0, "clamp")
		if err != nil {
			return nil, err
		}
		vals = a
	}
	if len(vals) != 3 {
		return nil, &Error{Msg: "clamp(): expects three values (or an Array of three)"}
	}
	sorted := append([]any{}, vals...)
	var cerr error
	sortThree(sorted, &cerr)
	if cerr != nil {
		return nil, cerr
	}
	return sorted[1], nil
}

func sortThree(a []any, errOut *error) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			c, err := compare(a[i], a[j])
			if err != nil {
				*errOut = &Error{Msg: err.Error()}
				return
			}
			if c > 0 {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func builtinSum(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("sum")
	}
	a, err := argArr(args, 0, "sum")
	if err != nil {
		return nil, err
	}
	allInt := true
	var fi int64
	var ff float64
	for _, e := range a {
		switch n := normalize(e).(type) {
		case int64:
			fi += n
			ff += float64(n)
		case float64:
			allInt = false
			ff += n
		default:
			return nil, &Error{Msg: "sum(): all elements must be Numeric"}
		}
	}
	if allInt {
		return fi, nil
	}
	return ff, nil
}

func builtinToBytes(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("to_bytes")
	}
	if f, ok := asFloat(args[0]); ok {
		return int64(f), nil
	}
	s, err := argStr(args, 0, "to_bytes")
	if err != nil {
		return nil, err
	}
	return parseBytes(s)
}

func parseBytes(s string) (Value, error) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '.' || s[i] == '-' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	numStr := s[:i]
	unit := strings.TrimSpace(s[i:])
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil, &Error{Msg: "to_bytes(): cannot parse number in " + s}
	}
	mult := map[string]float64{
		"": 1, "b": 1,
		"k": 1 << 10, "kb": 1 << 10,
		"m": 1 << 20, "mb": 1 << 20,
		"g": 1 << 30, "gb": 1 << 30,
		"t": 1 << 40, "tb": 1 << 40,
		"p": 1 << 50, "pb": 1 << 50,
		"e": 1 << 60, "eb": 1 << 60,
	}
	m, ok := mult[strings.ToLower(unit)]
	if !ok {
		return nil, &Error{Msg: "to_bytes(): unknown unit '" + unit + "'"}
	}
	return int64(num * m), nil
}
