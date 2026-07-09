// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"sort"
	"strings"

	"github.com/go-pcore/pcore"
)

// registerStdlibArray installs the array-manipulation function set.
func registerStdlibArray(e *Evaluator) {
	e.funcs["flatten"] = builtinFlatten
	e.funcs["unique"] = builtinUnique
	e.funcs["concat"] = builtinConcat
	e.funcs["member"] = builtinMember
	e.funcs["count"] = builtinCount
	e.funcs["index"] = builtinIndex
	e.funcs["delete"] = builtinDelete
	e.funcs["delete_at"] = builtinDeleteAt
	e.funcs["delete_undef_values"] = builtinDeleteUndef
	e.funcs["compact"] = builtinDeleteUndef
	e.funcs["difference"] = builtinDifference
	e.funcs["intersection"] = builtinIntersection
	e.funcs["union"] = builtinUnion
	e.funcs["range"] = builtinRange
	e.funcs["sort"] = builtinSort
	e.funcs["prefix"] = builtinPrefix
	e.funcs["suffix"] = builtinSuffix
	e.funcs["grep"] = builtinGrep
	e.funcs["reject"] = builtinReject
	e.funcs["values_at"] = builtinValuesAt
	e.funcs["zip"] = builtinZip
	e.funcs["any"] = builtinAny
	e.funcs["all"] = builtinAll
	e.funcs["has_key"] = builtinHasKey
	e.funcs["join_keys_to_values"] = builtinJoinKeysToValues
	e.funcs["hash"] = builtinArrayToHash
	e.funcs["pick"] = builtinPick
	e.funcs["pick_default"] = builtinPickDefault
}

func builtinFlatten(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("flatten")
	}
	a, err := argArr(args, 0, "flatten")
	if err != nil {
		return nil, err
	}
	return flattenArray(a), nil
}

func flattenArray(a []any) []any {
	out := []any{}
	for _, e := range a {
		if sub, ok := normalize(e).([]any); ok {
			out = append(out, flattenArray(sub)...)
			continue
		}
		out = append(out, e)
	}
	return out
}

func builtinUnique(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("unique")
	}
	switch x := normalize(args[0]).(type) {
	case string:
		seen := map[rune]bool{}
		var b strings.Builder
		for _, r := range x {
			if !seen[r] {
				seen[r] = true
				b.WriteRune(r)
			}
		}
		return b.String(), nil
	case []any:
		return uniqueValues(x), nil
	}
	return nil, &Error{Msg: "unique(): expects a String or Array"}
}

func uniqueValues(a []any) []any {
	out := []any{}
	for _, e := range a {
		dup := false
		for _, k := range out {
			if equals(e, k) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, e)
		}
	}
	return out
}

func builtinConcat(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, wrongArgs("concat")
	}
	out, err := argArr(args, 0, "concat")
	if err != nil {
		return nil, err
	}
	result := append([]any{}, out...)
	for _, a := range args[1:] {
		if arr, ok := normalize(a).([]any); ok {
			result = append(result, arr...)
			continue
		}
		result = append(result, a)
	}
	return result, nil
}

func builtinMember(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("member")
	}
	a, err := argArr(args, 0, "member")
	if err != nil {
		return nil, err
	}
	wanted := args[1]
	// stdlib member accepts a single value or an array of values (all must be
	// present).
	if wl, ok := normalize(wanted).([]any); ok {
		for _, w := range wl {
			if !containsValue(a, w) {
				return false, nil
			}
		}
		return true, nil
	}
	return containsValue(a, wanted), nil
}

func containsValue(a []any, v Value) bool {
	for _, e := range a {
		if equals(e, v) {
			return true
		}
	}
	return false
}

func builtinCount(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("count")
	}
	a, err := argArr(args, 0, "count")
	if err != nil {
		return nil, err
	}
	if len(args) == 1 {
		n := int64(0)
		for _, e := range a {
			if !isUndef(e) {
				n++
			}
		}
		return n, nil
	}
	n := int64(0)
	for _, e := range a {
		if equals(e, args[1]) {
			n++
		}
	}
	return n, nil
}

func builtinIndex(c *Context, args []Value, block *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("index")
	}
	a, err := argArr(args, 0, "index")
	if err != nil {
		return nil, err
	}
	if block != nil {
		for i, e := range a {
			r, err := block.Call(e)
			if err != nil {
				return nil, err
			}
			if truthy(r) {
				return int64(i), nil
			}
		}
		return pcore.Undef, nil
	}
	for i, e := range a {
		if equals(e, args[1]) {
			return int64(i), nil
		}
	}
	return pcore.Undef, nil
}

func builtinDelete(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("delete")
	}
	toDelete := valueSet(args[1])
	switch x := normalize(args[0]).(type) {
	case []any:
		out := []any{}
		for _, e := range x {
			if !toDelete(e) {
				out = append(out, e)
			}
		}
		return out, nil
	case map[string]any:
		out := cloneHash(x)
		for k := range out {
			if toDelete(k) {
				delete(out, k)
			}
		}
		return out, nil
	case string:
		res := x
		for _, d := range stringList(args[1]) {
			res = strings.ReplaceAll(res, d, "")
		}
		return res, nil
	}
	return nil, &Error{Msg: "delete(): first argument must be Array, Hash or String"}
}

// valueSet returns a predicate matching a single value or any of an array.
func valueSet(v Value) func(Value) bool {
	if arr, ok := normalize(v).([]any); ok {
		return func(e Value) bool { return containsValue(arr, e) }
	}
	return func(e Value) bool { return equals(e, v) }
}

func builtinDeleteAt(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("delete_at")
	}
	a, err := argArr(args, 0, "delete_at")
	if err != nil {
		return nil, err
	}
	idx, ok := normalize(args[1]).(int64)
	if !ok {
		return nil, &Error{Msg: "delete_at(): index must be an Integer"}
	}
	if idx < 0 {
		idx += int64(len(a))
	}
	if idx < 0 || idx >= int64(len(a)) {
		return append([]any{}, a...), nil
	}
	out := append([]any{}, a[:idx]...)
	return append(out, a[idx+1:]...), nil
}

func builtinDeleteUndef(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("delete_undef_values")
	}
	switch x := normalize(args[0]).(type) {
	case []any:
		out := []any{}
		for _, e := range x {
			if !isUndef(e) {
				out = append(out, e)
			}
		}
		return out, nil
	case map[string]any:
		out := map[string]any{}
		for k, v := range x {
			if !isUndef(v) {
				out[k] = v
			}
		}
		return out, nil
	}
	return nil, &Error{Msg: "delete_undef_values(): expects an Array or Hash"}
}

func builtinDifference(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("difference")
	}
	a, err := argArr(args, 0, "difference")
	if err != nil {
		return nil, err
	}
	b, err := argArr(args, 1, "difference")
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, e := range a {
		if !containsValue(b, e) {
			out = append(out, e)
		}
	}
	return out, nil
}

func builtinIntersection(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("intersection")
	}
	a, err := argArr(args, 0, "intersection")
	if err != nil {
		return nil, err
	}
	b, err := argArr(args, 1, "intersection")
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, e := range a {
		if containsValue(b, e) && !containsValue(out, e) {
			out = append(out, e)
		}
	}
	return out, nil
}

func builtinUnion(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, wrongArgs("union")
	}
	out := []any{}
	for i := range args {
		a, err := argArr(args, i, "union")
		if err != nil {
			return nil, err
		}
		for _, e := range a {
			if !containsValue(out, e) {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func builtinRange(_ *Context, args []Value, block *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("range")
	}
	out, err := rangeValues(args)
	if err != nil {
		return nil, err
	}
	if block != nil {
		for _, e := range out {
			if _, err := block.Call(e); err != nil {
				return nil, err
			}
		}
		return pcore.Undef, nil
	}
	return out, nil
}

func rangeValues(args []Value) ([]any, error) {
	step := int64(1)
	if len(args) == 3 {
		s, ok := normalize(args[2]).(int64)
		if !ok || s == 0 {
			return nil, &Error{Msg: "range(): step must be a non-zero Integer"}
		}
		step = s
	}
	lo, loInt := normalize(args[0]).(int64)
	hi, hiInt := normalize(args[1]).(int64)
	if loInt && hiInt {
		out := []any{}
		if step > 0 {
			for i := lo; i <= hi; i += step {
				out = append(out, i)
			}
		} else {
			for i := lo; i >= hi; i += step {
				out = append(out, i)
			}
		}
		return out, nil
	}
	// String range (e.g. 'a'..'c') using single-character bounds.
	ls, lok := normalize(args[0]).(string)
	hs, hok := normalize(args[1]).(string)
	if lok && hok && len(ls) == 1 && len(hs) == 1 {
		out := []any{}
		for c := ls[0]; c <= hs[0]; c += byte(step) {
			out = append(out, string(c))
		}
		return out, nil
	}
	return nil, &Error{Msg: "range(): unsupported bounds"}
}

func builtinSort(_ *Context, args []Value, block *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("sort")
	}
	if s, ok := normalize(args[0]).(string); ok {
		rs := []rune(s)
		sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
		return string(rs), nil
	}
	a, err := argArr(args, 0, "sort")
	if err != nil {
		return nil, err
	}
	out := append([]any{}, a...)
	var sortErr error
	sort.SliceStable(out, func(i, j int) bool {
		if block != nil {
			r, e := block.Call(out[i], out[j])
			if e != nil {
				sortErr = e
				return false
			}
			c, _ := normalize(r).(int64)
			return c < 0
		}
		c, e := compare(out[i], out[j])
		if e != nil {
			sortErr = e
			return false
		}
		return c < 0
	})
	if sortErr != nil {
		return nil, sortErr
	}
	return out, nil
}

func builtinPrefix(_ *Context, args []Value, _ *Block) (Value, error) {
	return affix(args, "prefix", true)
}

func builtinSuffix(_ *Context, args []Value, _ *Block) (Value, error) {
	return affix(args, "suffix", false)
}

func affix(args []Value, fn string, prefix bool) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs(fn)
	}
	fix := ""
	if len(args) == 2 {
		s, err := argStr(args, 1, fn)
		if err != nil {
			return nil, err
		}
		fix = s
	}
	if h, ok := normalize(args[0]).(map[string]any); ok {
		out := map[string]any{}
		for k, v := range h {
			if prefix {
				out[fix+k] = v
			} else {
				out[k+fix] = v
			}
		}
		return out, nil
	}
	a, err := argArr(args, 0, fn)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(a))
	for i, e := range a {
		if prefix {
			out[i] = fix + stringify(e)
		} else {
			out[i] = stringify(e) + fix
		}
	}
	return out, nil
}

func builtinGrep(_ *Context, args []Value, block *Block) (Value, error) {
	if block != nil {
		return builtinFilter(nil, args, block)
	}
	if len(args) != 2 {
		return nil, wrongArgs("grep")
	}
	a, err := argArr(args, 0, "grep")
	if err != nil {
		return nil, err
	}
	m, err := matchPredicate(args[1])
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, e := range a {
		if m(stringify(e)) {
			out = append(out, e)
		}
	}
	return out, nil
}

func builtinReject(_ *Context, args []Value, block *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("reject")
	}
	a, err := argArr(args, 0, "reject")
	if err != nil {
		return nil, err
	}
	m, err := matchPredicate(args[1])
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, e := range a {
		if !m(stringify(e)) {
			out = append(out, e)
		}
	}
	return out, nil
}

func builtinValuesAt(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("values_at")
	}
	a, err := argArr(args, 0, "values_at")
	if err != nil {
		return nil, err
	}
	var idxs []Value
	if arr, ok := normalize(args[1]).([]any); ok {
		idxs = arr
	} else {
		idxs = []Value{args[1]}
	}
	out := []any{}
	for _, iv := range idxs {
		i, ok := normalize(iv).(int64)
		if !ok {
			return nil, &Error{Msg: "values_at(): indices must be Integers"}
		}
		if i < 0 {
			i += int64(len(a))
		}
		if i >= 0 && i < int64(len(a)) {
			out = append(out, a[i])
		}
	}
	return out, nil
}

func builtinZip(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("zip")
	}
	a, err := argArr(args, 0, "zip")
	if err != nil {
		return nil, err
	}
	b, err := argArr(args, 1, "zip")
	if err != nil {
		return nil, err
	}
	out := make([]any, len(a))
	for i := range a {
		pair := []any{a[i]}
		if i < len(b) {
			pair = append(pair, b[i])
		} else {
			pair = append(pair, pcore.Undef)
		}
		out[i] = pair
	}
	if len(args) == 3 && truthy(args[2]) {
		return flattenArray(out), nil
	}
	return out, nil
}

func builtinAny(_ *Context, args []Value, block *Block) (Value, error) {
	if block == nil {
		return nil, &Error{Msg: "any() requires a block"}
	}
	if len(args) != 1 {
		return nil, wrongArgs("any")
	}
	found := false
	isHash := isHashValue(args[0])
	err := iterPairs(args[0], func(k, v Value) error {
		r, e := callPair(block, k, v, isHash)
		if e != nil {
			return e
		}
		if truthy(r) {
			found = true
		}
		return nil
	})
	return found, err
}

func builtinAll(_ *Context, args []Value, block *Block) (Value, error) {
	if block == nil {
		return nil, &Error{Msg: "all() requires a block"}
	}
	if len(args) != 1 {
		return nil, wrongArgs("all")
	}
	all := true
	isHash := isHashValue(args[0])
	err := iterPairs(args[0], func(k, v Value) error {
		r, e := callPair(block, k, v, isHash)
		if e != nil {
			return e
		}
		if !truthy(r) {
			all = false
		}
		return nil
	})
	return all, err
}

func builtinHasKey(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("has_key")
	}
	h, err := argHash(args, 0, "has_key")
	if err != nil {
		return nil, err
	}
	_, ok := h[stringify(args[1])]
	return ok, nil
}

func builtinJoinKeysToValues(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("join_keys_to_values")
	}
	h, err := argHash(args, 0, "join_keys_to_values")
	if err != nil {
		return nil, err
	}
	sep, err := argStr(args, 1, "join_keys_to_values")
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, k := range sortedKeys(h) {
		v := h[k]
		if isUndef(v) {
			out = append(out, k+sep)
			continue
		}
		if arr, ok := normalize(v).([]any); ok {
			for _, el := range arr {
				out = append(out, k+sep+stringify(el))
			}
			continue
		}
		out = append(out, k+sep+stringify(v))
	}
	return out, nil
}

func builtinArrayToHash(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("hash")
	}
	a, err := argArr(args, 0, "hash")
	if err != nil {
		return nil, err
	}
	flat := a
	// Accept either [[k,v],...] pairs or a flat [k,v,k,v] list.
	if len(a) > 0 {
		if _, ok := normalize(a[0]).([]any); ok {
			flat = []any{}
			for _, pair := range a {
				p, _ := normalize(pair).([]any)
				flat = append(flat, p...)
			}
		}
	}
	if len(flat)%2 != 0 {
		return nil, &Error{Msg: "hash(): expects an even number of elements"}
	}
	out := map[string]any{}
	for i := 0; i < len(flat); i += 2 {
		out[stringify(flat[i])] = flat[i+1]
	}
	return out, nil
}

func builtinPick(_ *Context, args []Value, _ *Block) (Value, error) {
	for _, a := range args {
		if !isUndef(a) && a != "" {
			return a, nil
		}
	}
	return nil, &Error{Msg: "pick(): must receive at least one non-undef, non-empty value"}
}

func builtinPickDefault(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) == 0 {
		return nil, &Error{Msg: "pick_default(): must receive at least one argument"}
	}
	for _, a := range args {
		if !isUndef(a) && a != "" {
			return a, nil
		}
	}
	return args[len(args)-1], nil
}
