// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import "github.com/go-pcore/pcore"

// requireBlock returns the block or an error if it is absent.
func requireBlock(block *Block, name string) (*Block, error) {
	if block == nil {
		return nil, &Error{Msg: name + "() requires a block"}
	}
	return block, nil
}

// iterPairs yields index/element pairs for arrays and key/value pairs for
// hashes, invoking fn for each. It errors on non-iterable receivers.
func iterPairs(v Value, fn func(a, b Value) error) error {
	switch c := normalize(v).(type) {
	case []any:
		for i, el := range c {
			if err := fn(int64(i), el); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for _, k := range sortedKeys(c) {
			if err := fn(k, c[k]); err != nil {
				return err
			}
		}
		return nil
	}
	return &Error{Msg: "value is not iterable"}
}

// callPair invokes block with either a single value (1-parameter block) — for
// arrays the element, for hashes a [key, value] pair — or two values.
func callPair(block *Block, a, b Value, isHash bool) (Value, error) {
	if block.Arity() <= 1 {
		if isHash {
			return block.Call([]any{a, b})
		}
		return block.Call(b)
	}
	return block.Call(a, b)
}

func builtinEach(_ *Context, args []Value, block *Block) (Value, error) {
	if err := need(args, 1, "each"); err != nil {
		return nil, err
	}
	b, err := requireBlock(block, "each")
	if err != nil {
		return nil, err
	}
	isHash := isHashValue(args[0])
	err = iterPairs(args[0], func(k, v Value) error {
		_, e := callPair(b, k, v, isHash)
		return e
	})
	if err != nil {
		return nil, err
	}
	return args[0], nil
}

func builtinMap(_ *Context, args []Value, block *Block) (Value, error) {
	if err := need(args, 1, "map"); err != nil {
		return nil, err
	}
	b, err := requireBlock(block, "map")
	if err != nil {
		return nil, err
	}
	isHash := isHashValue(args[0])
	out := []any{}
	err = iterPairs(args[0], func(k, v Value) error {
		r, e := callPair(b, k, v, isHash)
		if e != nil {
			return e
		}
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func builtinFilter(_ *Context, args []Value, block *Block) (Value, error) {
	if err := need(args, 1, "filter"); err != nil {
		return nil, err
	}
	b, err := requireBlock(block, "filter")
	if err != nil {
		return nil, err
	}
	if m, ok := normalize(args[0]).(map[string]any); ok {
		out := map[string]any{}
		for _, k := range sortedKeys(m) {
			keep, e := callPair(b, k, m[k], true)
			if e != nil {
				return nil, e
			}
			if truthy(keep) {
				out[k] = m[k]
			}
		}
		return out, nil
	}
	out := []any{}
	err = iterPairs(args[0], func(k, v Value) error {
		keep, e := callPair(b, k, v, false)
		if e != nil {
			return e
		}
		if truthy(keep) {
			out = append(out, v)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func builtinReduce(_ *Context, args []Value, block *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, &Error{Msg: "reduce() expects a collection and an optional seed"}
	}
	b, err := requireBlock(block, "reduce")
	if err != nil {
		return nil, err
	}
	items, err := elementsOf(args[0])
	if err != nil {
		return nil, err
	}
	var memo Value
	start := 0
	if len(args) == 2 {
		memo = args[1]
	} else {
		if len(items) == 0 {
			return pcore.Undef, nil
		}
		memo = items[0]
		start = 1
	}
	for _, it := range items[start:] {
		memo, err = b.Call(memo, it)
		if err != nil {
			return nil, err
		}
	}
	return memo, nil
}

func builtinWith(_ *Context, args []Value, block *Block) (Value, error) {
	b, err := requireBlock(block, "with")
	if err != nil {
		return nil, err
	}
	return b.Call(args...)
}

func builtinSlice(_ *Context, args []Value, block *Block) (Value, error) {
	if err := need(args, 2, "slice"); err != nil {
		return nil, err
	}
	items, err := elementsOf(args[0])
	if err != nil {
		return nil, err
	}
	n, ok := normalize(args[1]).(int64)
	if !ok || n <= 0 {
		return nil, &Error{Msg: "slice() chunk size must be a positive Integer"}
	}
	var chunks []any
	for i := 0; i < len(items); i += int(n) {
		end := i + int(n)
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, append([]any{}, items[i:end]...))
	}
	if block == nil {
		if chunks == nil {
			return []any{}, nil
		}
		return chunks, nil
	}
	for _, ch := range chunks {
		if _, err := block.Call(ch); err != nil {
			return nil, err
		}
	}
	return args[0], nil
}

// elementsOf returns the ordered elements of an array or the values of a hash
// (by sorted key) for the fold/slice iterators.
func elementsOf(v Value) ([]Value, error) {
	switch c := normalize(v).(type) {
	case []any:
		return c, nil
	case map[string]any:
		out := make([]Value, 0, len(c))
		for _, k := range sortedKeys(c) {
			out = append(out, []any{k, c[k]})
		}
		return out, nil
	}
	return nil, &Error{Msg: "value is not iterable"}
}

func isHashValue(v Value) bool {
	_, ok := normalize(v).(map[string]any)
	return ok
}
