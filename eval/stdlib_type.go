// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"github.com/go-pcore/pcore"
)

// registerStdlibType installs the (mostly legacy) type-inspection function set.
func registerStdlibType(e *Evaluator) {
	e.funcs["is_string"] = isTypeFn(func(v Value) bool { _, ok := v.(string); return ok })
	e.funcs["is_integer"] = isTypeFn(func(v Value) bool { _, ok := v.(int64); return ok })
	e.funcs["is_float"] = isTypeFn(func(v Value) bool { _, ok := v.(float64); return ok })
	e.funcs["is_numeric"] = isTypeFn(func(v Value) bool { _, ok := asFloat(v); return ok })
	e.funcs["is_bool"] = isTypeFn(func(v Value) bool { _, ok := v.(bool); return ok })
	e.funcs["is_array"] = isTypeFn(func(v Value) bool { _, ok := v.([]any); return ok })
	e.funcs["is_hash"] = isTypeFn(func(v Value) bool { _, ok := v.(map[string]any); return ok })
	e.funcs["type_of"] = builtinTypeOf
	e.funcs["any2array"] = builtinAny2Array
	e.funcs["any2bool"] = builtinAny2Bool
	e.funcs["validate_array"] = validateFn(func(v Value) bool { _, ok := v.([]any); return ok }, "Array")
	e.funcs["validate_hash"] = validateFn(func(v Value) bool { _, ok := v.(map[string]any); return ok }, "Hash")
	e.funcs["validate_string"] = validateFn(func(v Value) bool { _, ok := v.(string); return ok }, "String")
	e.funcs["validate_bool"] = validateFn(func(v Value) bool { _, ok := v.(bool); return ok }, "Boolean")
	e.funcs["validate_integer"] = validateFn(func(v Value) bool { _, ok := v.(int64); return ok }, "Integer")
	e.funcs["validate_numeric"] = validateFn(func(v Value) bool { _, ok := asFloat(v); return ok }, "Numeric")
	e.funcs["validate_re"] = builtinValidateRe
	e.funcs["validate_legacy"] = builtinValidateLegacy
}

func isTypeFn(pred func(Value) bool) Function {
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) != 1 {
			return nil, &Error{Msg: "is_* type test expects one argument"}
		}
		return pred(normalize(args[0])), nil
	}
}

func builtinTypeOf(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("type_of")
	}
	return pcore.Infer(normalize(args[0])), nil
}

func builtinAny2Array(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) == 0 {
		return []any{}, nil
	}
	if len(args) == 1 {
		switch x := normalize(args[0]).(type) {
		case []any:
			return x, nil
		case map[string]any:
			out := []any{}
			for _, k := range sortedKeys(x) {
				out = append(out, k, x[k])
			}
			return out, nil
		default:
			if isUndef(args[0]) {
				return []any{}, nil
			}
			return []any{args[0]}, nil
		}
	}
	return append([]any{}, args...), nil
}

func builtinAny2Bool(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("any2bool")
	}
	switch x := normalize(args[0]).(type) {
	case bool:
		return x, nil
	case string:
		return builtinStr2bool(nil, args, nil)
	case int64:
		return x != 0, nil
	case float64:
		return x != 0, nil
	}
	if isUndef(args[0]) {
		return false, nil
	}
	return true, nil
}

func validateFn(pred func(Value) bool, name string) Function {
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) == 0 {
			return nil, &Error{Msg: "validate_" + name + "(): expects at least one argument"}
		}
		for _, a := range args {
			if !pred(normalize(a)) {
				return nil, &Error{Msg: "validate_" + name + "(): a value is not a " + name}
			}
		}
		return pcore.Undef, nil
	}
}

func builtinValidateRe(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("validate_re")
	}
	subject := stringify(args[0])
	for _, p := range stringList(args[1]) {
		// stringList yields Strings, for which matchPredicate never errors.
		m, _ := matchPredicate(p)
		if m(subject) {
			return pcore.Undef, nil
		}
	}
	msg := "validate_re(): " + subject + " does not match the supplied pattern(s)"
	if len(args) == 3 {
		msg = stringify(args[2])
	}
	return nil, &Error{Msg: msg}
}

// builtinValidateLegacy accepts (value, TypeName, deprecated_validator, ...) and
// validates value against the Pcore type in the second argument.
func builtinValidateLegacy(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 {
		return nil, wrongArgs("validate_legacy")
	}
	var t pcore.Type
	switch tv := args[0].(type) {
	case pcore.Type:
		t = tv
	case string:
		parsed, err := pcore.Parse(tv)
		if err != nil {
			return nil, &Error{Msg: "validate_legacy(): invalid type " + tv}
		}
		t = parsed
	default:
		return nil, &Error{Msg: "validate_legacy(): first argument must be a Type"}
	}
	if !pcore.IsInstance(t, normalize(args[len(args)-1])) {
		return nil, &Error{Msg: "validate_legacy(): value is not an instance of " + t.String()}
	}
	return pcore.Undef, nil
}
