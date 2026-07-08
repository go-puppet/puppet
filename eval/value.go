// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-pcore/pcore"
)

// Value is a Puppet value. It uses the same representation as
// [github.com/go-pcore/pcore]: bool, int64, float64 and string for scalars,
// []any for arrays, map[string]any for hashes, pcore.Undef for undef, the
// pcore wrapper types for the rich values, and pcore.Type for data types.
type Value = any

// isUndef reports whether v is Puppet undef (a Go nil or the pcore.Undef
// singleton).
func isUndef(v Value) bool {
	if v == nil {
		return true
	}
	return v == pcore.Undef
}

// truthy applies Puppet truthiness: only undef and false are false.
func truthy(v Value) bool {
	if isUndef(v) {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

// equals reports Puppet value equality.
func equals(a, b Value) bool {
	a, b = normalize(a), normalize(b)
	switch x := a.(type) {
	case int64:
		switch y := b.(type) {
		case int64:
			return x == y
		case float64:
			return float64(x) == y
		}
		return false
	case float64:
		switch y := b.(type) {
		case int64:
			return x == float64(y)
		case float64:
			return x == y
		}
		return false
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equals(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, xv := range x {
			yv, present := y[k]
			if !present || !equals(xv, yv) {
				return false
			}
		}
		return true
	default:
		if isUndef(a) {
			return isUndef(b)
		}
		return a == b
	}
}

// normalize widens integer/float kinds and maps nil to pcore.Undef so value
// comparisons are consistent.
func normalize(v Value) Value {
	switch x := v.(type) {
	case nil:
		return pcore.Undef
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float32:
		return float64(x)
	default:
		return v
	}
}

// stringify renders v the way Puppet renders it inside an interpolated string.
func stringify(v Value) string {
	switch x := normalize(v).(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = inspect(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := sortedKeys(x)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = strconv.Quote(k) + " => " + inspect(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case pcore.Type:
		return x.String()
	default:
		if isUndef(x) {
			return ""
		}
		return fmt.Sprintf("%v", x)
	}
}

// inspect is like stringify but quotes bare strings, used inside collections.
func inspect(v Value) string {
	if s, ok := normalize(v).(string); ok {
		return strconv.Quote(s)
	}
	return stringify(v)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// asFloat returns v as a float64 if it is numeric.
func asFloat(v Value) (float64, bool) {
	switch x := normalize(v).(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// compare orders two values for the relational operators, returning -1, 0 or 1.
// It supports numeric and string comparison; other combinations are an error.
func compare(a, b Value) (int, error) {
	an, bn := normalize(a), normalize(b)
	if af, ok := asFloat(an); ok {
		if bf, ok := asFloat(bn); ok {
			switch {
			case af < bf:
				return -1, nil
			case af > bf:
				return 1, nil
			default:
				return 0, nil
			}
		}
	}
	if as, ok := an.(string); ok {
		if bs, ok := bn.(string); ok {
			return strings.Compare(strings.ToLower(as), strings.ToLower(bs)), nil
		}
	}
	return 0, fmt.Errorf("cannot compare %s and %s", typeName(a), typeName(b))
}

// typeName returns the Pcore type name of v (for error messages).
func typeName(v Value) string {
	return pcore.Infer(normalize(v)).Name()
}
