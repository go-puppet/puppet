// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// registerStdlibSerialize installs stdlib::to_python and stdlib::to_ruby, which
// render a value as its Python or Ruby literal representation.
//
// Puppet stores Hashes as unordered maps in this implementation, so hash keys
// are emitted in sorted order; puppetlabs-stdlib preserves Ruby insertion order.
func registerStdlibSerialize(e *Evaluator) {
	e.funcs["to_python"] = builtinToPython
	e.funcs["to_ruby"] = builtinToRuby
}

func builtinToPython(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("to_python")
	}
	return toPython(args[0]), nil
}

func builtinToRuby(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("to_ruby")
	}
	return toRuby(args[0]), nil
}

func toPython(v Value) string {
	switch x := normalize(v).(type) {
	case bool:
		if x {
			return "True"
		}
		return "False"
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = toPython(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := sortedKeys(x)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = rubyStringInspect(k) + ": " + toPython(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		if isUndef(x) {
			return "None"
		}
		return rubyScalarInspect(x)
	}
}

func toRuby(v Value) string {
	switch x := normalize(v).(type) {
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = toRuby(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := sortedKeys(x)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = rubyStringInspect(k) + " => " + toRuby(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return rubyScalarInspect(x)
	}
}

// rubyScalarInspect renders a scalar the way Ruby's Object#inspect would:
// nil, true, false, quoted strings, and plain numbers.
func rubyScalarInspect(v Value) string {
	switch x := normalize(v).(type) {
	case string:
		return rubyStringInspect(x)
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return rubyFloatToS(x)
	default:
		if isUndef(x) {
			return "nil"
		}
		return stringify(x)
	}
}

// rubyFloatToS renders a float the way Ruby's Float#to_s does: with a mandatory
// decimal point (100.0, not 100) and a ".0" mantissa in exponent form.
func rubyFloatToS(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case math.IsNaN(f):
		return "NaN"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if e := strings.IndexAny(s, "eE"); e >= 0 {
		mant := s[:e]
		if !strings.Contains(mant, ".") {
			mant += ".0"
		}
		return mant + s[e:]
	}
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// rubyStringInspect renders a string the way Ruby's String#inspect does:
// double-quoted with backslash escapes for control and meta characters.
func rubyStringInspect(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case '\f':
			b.WriteString(`\f`)
		case '\v':
			b.WriteString(`\v`)
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case 0x1b:
			b.WriteString(`\e`)
		case '#':
			if i+1 < len(rs) && (rs[i+1] == '{' || rs[i+1] == '$' || rs[i+1] == '@') {
				b.WriteString(`\#`)
			} else {
				b.WriteByte('#')
			}
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
