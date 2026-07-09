// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"github.com/go-pcore/pcore"
)

// registerStdlibHash installs the hash-manipulation function set.
func registerStdlibHash(e *Evaluator) {
	e.funcs["deep_merge"] = builtinDeepMerge
	e.funcs["dig"] = builtinDig
	e.funcs["get"] = builtinGet
	e.funcs["delete_values"] = builtinDeleteValues
	e.funcs["delete_regex"] = builtinDeleteRegex
	e.funcs["stdlib::merge"] = builtinMerge
	e.funcs["convert_to"] = builtinConvertTo
	e.funcs["tree_each"] = builtinTreeEach
}

func builtinDeepMerge(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 {
		return nil, &Error{Msg: "deep_merge(): expects at least two Hashes"}
	}
	out := map[string]any{}
	for i := range args {
		if isUndef(args[i]) {
			continue
		}
		h, err := argHash(args, i, "deep_merge")
		if err != nil {
			return nil, err
		}
		out = deepMergeHash(out, h)
	}
	return out, nil
}

func deepMergeHash(a, b map[string]any) map[string]any {
	out := cloneHash(a)
	for k, v := range b {
		if existing, ok := out[k]; ok {
			em, eok := normalize(existing).(map[string]any)
			vm, vok := normalize(v).(map[string]any)
			if eok && vok {
				out[k] = deepMergeHash(em, vm)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// builtinDig navigates a nested structure by a path array, returning undef when
// any step is missing.
func builtinDig(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("dig")
	}
	path, err := argArr(args, 1, "dig")
	if err != nil {
		return nil, err
	}
	return digPath(args[0], path), nil
}

func digPath(root Value, path []any) Value {
	cur := root
	for _, key := range path {
		if isUndef(cur) {
			return pcore.Undef
		}
		switch c := normalize(cur).(type) {
		case map[string]any:
			v, ok := c[stringify(key)]
			if !ok {
				return pcore.Undef
			}
			cur = v
		case []any:
			i, ok := normalize(key).(int64)
			if !ok || i < 0 || i >= int64(len(c)) {
				return pcore.Undef
			}
			cur = c[i]
		default:
			return pcore.Undef
		}
	}
	return cur
}

// builtinGet is like dig but takes a dotted string path and an optional default.
func builtinGet(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("get")
	}
	pathStr, err := argStr(args, 1, "get")
	if err != nil {
		return nil, err
	}
	path := dottedPath(pathStr)
	v := digPath(args[0], path)
	if isUndef(v) && len(args) == 3 {
		return args[2], nil
	}
	return v, nil
}

func dottedPath(s string) []any {
	if s == "" {
		return nil
	}
	var out []any
	cur := ""
	flush := func() {
		if cur == "" {
			return
		}
		if isNumeric(cur) {
			n := int64(0)
			for i := 0; i < len(cur); i++ {
				n = n*10 + int64(cur[i]-'0')
			}
			out = append(out, n)
		} else {
			out = append(out, cur)
		}
		cur = ""
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			flush()
			continue
		}
		cur += string(s[i])
	}
	flush()
	return out
}

func builtinDeleteValues(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("delete_values")
	}
	h, err := argHash(args, 0, "delete_values")
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for k, v := range h {
		if !equals(v, args[1]) {
			out[k] = v
		}
	}
	return out, nil
}

func builtinDeleteRegex(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("delete_regex")
	}
	m, err := matchPredicate(args[1])
	if err != nil {
		return nil, err
	}
	switch x := normalize(args[0]).(type) {
	case []any:
		out := []any{}
		for _, e := range x {
			if !m(stringify(e)) {
				out = append(out, e)
			}
		}
		return out, nil
	case map[string]any:
		out := map[string]any{}
		for k, v := range x {
			if !m(k) {
				out[k] = v
			}
		}
		return out, nil
	}
	return nil, &Error{Msg: "delete_regex(): expects an Array or Hash"}
}

// builtinConvertTo converts a value to a target type (String, Array, Hash).
func builtinConvertTo(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("convert_to")
	}
	t, ok := args[1].(pcore.Type)
	if !ok {
		return nil, &Error{Msg: "convert_to(): second argument must be a Type"}
	}
	switch t.Name() {
	case "String":
		return stringify(args[0]), nil
	case "Array":
		return toArray(args[0]), nil
	case "Hash":
		if h, ok := normalize(args[0]).(map[string]any); ok {
			return h, nil
		}
		return nil, &Error{Msg: "convert_to(): cannot convert to Hash"}
	case "Boolean":
		return builtinAny2Bool(nil, args[:1], nil)
	}
	return nil, &Error{Msg: "convert_to(): unsupported target type " + t.Name()}
}

func toArray(v Value) []any {
	switch x := normalize(v).(type) {
	case []any:
		return x
	case map[string]any:
		out := []any{}
		for _, k := range sortedKeys(x) {
			out = append(out, []any{k, x[k]})
		}
		return out
	default:
		if isUndef(v) {
			return []any{}
		}
		return []any{v}
	}
}

// builtinTreeEach walks a nested structure, invoking the block with each value.
func builtinTreeEach(_ *Context, args []Value, block *Block) (Value, error) {
	if block == nil {
		return nil, &Error{Msg: "tree_each() requires a block"}
	}
	if len(args) != 1 {
		return nil, wrongArgs("tree_each")
	}
	if err := treeWalk(args[0], block); err != nil {
		return nil, err
	}
	return args[0], nil
}

func treeWalk(v Value, block *Block) error {
	switch x := normalize(v).(type) {
	case []any:
		for _, e := range x {
			if err := treeWalk(e, block); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, k := range sortedKeys(x) {
			if err := treeWalk(x[k], block); err != nil {
				return err
			}
		}
	default:
		if _, err := block.Call(v); err != nil {
			return err
		}
	}
	return nil
}
