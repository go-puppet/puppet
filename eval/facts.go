// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strconv"
	"strings"

	"github.com/go-facter/facter"
)

// MapFacts is a deterministic [FactsProvider] backed by a nested map. A dotted
// fact name digs through nested map[string]any / []any values, so
// {"os": {"family": "Debian"}} resolves "os.family".
type MapFacts map[string]any

// Fact resolves a (possibly dotted) fact name.
func (m MapFacts) Fact(name string) (Value, bool) {
	v, ok := dig(map[string]any(m), strings.Split(name, "."))
	if !ok {
		return nil, false
	}
	return toValue(v), true
}

// Facts returns the whole fact tree.
func (m MapFacts) Facts() map[string]any { return map[string]any(m) }

func dig(v any, segs []string) (any, bool) {
	cur := v
	for _, seg := range segs {
		switch c := cur.(type) {
		case map[string]any:
			nv, ok := c[seg]
			if !ok {
				return nil, false
			}
			cur = nv
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// facterProvider adapts a go-facter Collection to [FactsProvider].
type facterProvider struct{ c *facter.Collection }

// FacterFacts returns a [FactsProvider] backed by go-facter's live host
// inventory. Pass it via [WithFacts].
func FacterFacts() FactsProvider { return &facterProvider{c: facter.New()} }

// Fact resolves a (possibly dotted) fact through go-facter.
func (f *facterProvider) Fact(name string) (Value, bool) {
	v, ok := f.c.Value(name)
	if !ok {
		return nil, false
	}
	return toValue(v), true
}

// Facts returns the full fact hash.
func (f *facterProvider) Facts() map[string]any { return f.c.ToHash() }
