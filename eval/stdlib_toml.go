// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// registerStdlibToml installs the TOML encoder. stdlib::to_toml is the canonical
// name; to_toml is the (deprecated) unnamespaced alias.
func registerStdlibToml(e *Evaluator) {
	e.funcs["stdlib::to_toml"] = builtinToTOML
	e.funcs["to_toml"] = builtinToTOML
}

// builtinToTOML implements stdlib::to_toml(Hash): it renders a hash as TOML.
// Following the upstream function, the data is first passed through
// rewrap_sensitive_data so that a structure containing Sensitive values yields a
// Sensitive TOML string; otherwise a plain String is returned.
func builtinToTOML(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("stdlib::to_toml")
	}
	h, err := argHash(args, 0, "stdlib::to_toml")
	if err != nil {
		return nil, err
	}
	// Mirror rewrap_sensitive_data: dump the unwrapped structure, then re-wrap
	// the resulting string as Sensitive iff any Sensitive value was present.
	found := false
	unwrapped := deepUnwrap(h, &found).(map[string]any)
	toml := newTOMLDumper(unwrapped).str
	if found {
		return NewSensitive(toml), nil
	}
	return toml, nil
}

// tomlDumper ports puppetlabs-stdlib's PuppetX::Stdlib::TomlDumper (itself a
// copy of toml-rb v2.0.1's dumper) so the output is byte-for-byte faithful:
// simple key/value pairs first (sorted), then nested tables, then table arrays.
type tomlDumper struct{ str string }

// newTOMLDumper renders h to TOML.
func newTOMLDumper(h map[string]any) *tomlDumper {
	d := &tomlDumper{}
	d.visit(h, nil, false)
	return d
}

func (d *tomlDumper) visit(h map[string]any, prefix []string, extraBrackets bool) {
	simple, nested, tableArray := sortTOMLPairs(h)
	if len(prefix) > 0 && (len(simple) > 0 || len(h) == 0) {
		d.printPrefix(prefix, extraBrackets)
	}
	d.dumpPairs(simple, nested, tableArray, prefix)
}

// tomlPair is one key/value entry retaining the (sorted) key.
type tomlPair struct {
	key string
	val any
}

// sortTOMLPairs splits a hash's entries, in sorted-key order, into simple
// scalars/arrays, nested hashes, and arrays-of-hashes (table arrays).
func sortTOMLPairs(h map[string]any) (simple, nested, tableArray []tomlPair) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := normalize(h[k])
		p := tomlPair{key: k, val: v}
		switch x := v.(type) {
		case map[string]any:
			nested = append(nested, p)
		case []any:
			if len(x) > 0 {
				if _, ok := normalize(x[0]).(map[string]any); ok {
					tableArray = append(tableArray, p)
					continue
				}
			}
			simple = append(simple, p)
		default:
			simple = append(simple, p)
		}
	}
	return simple, nested, tableArray
}

func (d *tomlDumper) dumpPairs(simple, nested, tableArray []tomlPair, prefix []string) {
	for _, p := range simple {
		d.str += tomlKey(p.key) + " = " + tomlValue(p.val) + "\n"
	}
	for _, p := range nested {
		d.visit(p.val.(map[string]any), append(prefixCopy(prefix), tomlKey(p.key)), false)
	}
	for _, p := range tableArray {
		aux := append(prefixCopy(prefix), tomlKey(p.key))
		for _, child := range p.val.([]any) {
			d.printPrefix(aux, true)
			cs, cn, cta := sortTOMLPairs(child.(map[string]any))
			d.dumpPairs(cs, cn, cta, aux)
		}
	}
}

func (d *tomlDumper) printPrefix(prefix []string, extraBrackets bool) {
	joined := strings.Join(prefix, ".")
	if extraBrackets {
		joined = "[" + joined + "]"
	}
	d.str += "[" + joined + "]\n"
}

// tomlKey renders a hash key: bare when it matches [a-zA-Z0-9_-]*, otherwise a
// quoted key with embedded double quotes escaped.
func tomlKey(key string) string {
	if isBareTOMLKey(key) {
		return key
	}
	return `"` + strings.ReplaceAll(key, `"`, `\"`) + `"`
}

func isBareTOMLKey(key string) bool {
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// tomlValue renders a scalar or (non-table) array TOML value. A top-level String
// uses Ruby String#inspect with the interpolation-guard backslash removed
// (matching the upstream `gsub(/\\(#[$@{])/, '\1')`); array elements use plain
// Ruby inspect via rubyInspect.
func tomlValue(v any) string {
	if s, ok := v.(string); ok {
		return unguardHash(rubyInspectString(s))
	}
	return rubyInspect(v)
}

// unguardHash reverses Ruby's `#{`/`#$`/`#@` escaping, dropping the backslash
// that String#inspect inserts before those interpolation sequences.
func unguardHash(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+2 < len(s) && s[i+1] == '#' &&
			(s[i+2] == '{' || s[i+2] == '$' || s[i+2] == '@') {
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// rubyInspect mirrors Ruby's Object#inspect for the value kinds that appear
// inside a TOML array, so nested arrays render exactly as the upstream dumper's
// `obj.inspect` does.
func rubyInspect(v any) string {
	switch x := normalize(v).(type) {
	case string:
		return rubyInspectString(x)
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return rubyFloatInspect(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = rubyInspect(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := sortedKeys(x)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = rubyInspectString(k) + "=>" + rubyInspect(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		if isUndef(x) {
			return "nil"
		}
		return stringify(x)
	}
}

// rubyFloatInspect renders a float the way Ruby's Float#inspect does: the
// shortest round-tripping decimal, always carrying a decimal point.
func rubyFloatInspect(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// rubyInspectString reproduces Ruby's String#inspect: a double-quoted string
// with C-style escapes for the standard control characters, `\uXXXX` for other
// control bytes and DEL, `\#` before an interpolation sequence, and every other
// (valid UTF-8) rune emitted literally.
func rubyInspectString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	runes := []rune(s)
	for i, r := range runes {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\v':
			b.WriteString(`\v`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		case '\x1b':
			b.WriteString(`\e`)
		case '#':
			if i+1 < len(runes) && (runes[i+1] == '{' || runes[i+1] == '$' || runes[i+1] == '@') {
				b.WriteString(`\#`)
			} else {
				b.WriteByte('#')
			}
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// prefixCopy returns a fresh copy of prefix so appends never alias a shared
// backing array across sibling tables.
func prefixCopy(prefix []string) []string {
	out := make([]string, len(prefix))
	copy(out, prefix)
	return out
}
