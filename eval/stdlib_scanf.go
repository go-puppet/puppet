// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strconv"
	"strings"
)

// builtinScanf implements Puppet core scanf(data, format, [block]). It parses
// data according to a C/Ruby scanf format, returning an Array of the converted
// values. Conversion stops at the first directive that fails to match, so the
// result may be shorter than the number of directives (empty on total failure).
// An optional block receives the result Array and its return value is used.
func builtinScanf(_ *Context, args []Value, block *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("scanf")
	}
	data, err := argStr(args, 0, "scanf")
	if err != nil {
		return nil, err
	}
	format, err := argStr(args, 1, "scanf")
	if err != nil {
		return nil, err
	}
	result := scanfParse(data, format)
	if block != nil {
		return block.Call(result)
	}
	return result, nil
}

// scanfParse applies the format to data, returning the converted values.
func scanfParse(data, format string) []any {
	out := []any{}
	di := 0 // index into data
	fi := 0 // index into format
	for fi < len(format) {
		fc := format[fi]
		switch {
		case fc == '%':
			fi++
			if fi >= len(format) {
				return out
			}
			if format[fi] == '%' {
				di = skipSpaces(data, di)
				if di >= len(data) || data[di] != '%' {
					return out
				}
				di++
				fi++
				continue
			}
			// optional maximum field width
			width := 0
			for fi < len(format) && format[fi] >= '0' && format[fi] <= '9' {
				width = width*10 + int(format[fi]-'0')
				fi++
			}
			if fi >= len(format) {
				return out
			}
			verb := format[fi]
			fi++
			val, ndi, ok := scanOne(data, di, verb, width)
			if !ok {
				return out
			}
			out = append(out, val)
			di = ndi
		case fc == ' ' || fc == '\t' || fc == '\n':
			// whitespace in format matches any run of whitespace in data
			di = skipSpaces(data, di)
			fi++
		default:
			// literal must match
			if di >= len(data) || data[di] != fc {
				return out
			}
			di++
			fi++
		}
	}
	return out
}

func skipSpaces(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\f' || s[i] == '\v') {
		i++
	}
	return i
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}

// scanOne converts a single directive at data[i:], returning the value, the new
// index, and whether the conversion succeeded.
func scanOne(data string, i int, verb byte, width int) (any, int, bool) {
	switch verb {
	case 'c':
		// %c reads exactly one character (no whitespace skipping).
		if i >= len(data) {
			return nil, i, false
		}
		w := width
		if w == 0 {
			w = 1
		}
		if i+w > len(data) {
			w = len(data) - i
		}
		return data[i : i+w], i + w, true
	case 's':
		i = skipSpaces(data, i)
		start := i
		for i < len(data) && !isSpace(data[i]) {
			if width > 0 && i-start >= width {
				break
			}
			i++
		}
		if i == start {
			return nil, i, false
		}
		return data[start:i], i, true
	case 'd', 'u':
		return scanInt(data, i, width, 10, true)
	case 'x', 'X':
		return scanInt(data, i, width, 16, false)
	case 'o':
		return scanInt(data, i, width, 8, false)
	case 'i':
		return scanInt(data, i, width, 0, true)
	case 'f', 'e', 'g', 'E', 'G', 'a':
		return scanFloat(data, i, width)
	default:
		return nil, i, false
	}
}

// scanInt scans an integer in the given base (0 = C-style auto-detect via the
// 0x/0 prefixes). A leading sign is accepted when signed is true.
func scanInt(data string, i, width, base int, signed bool) (any, int, bool) {
	i = skipSpaces(data, i)
	start := i
	max := len(data)
	if width > 0 && start+width < max {
		max = start + width
	}
	j := i
	neg := false
	if signed && j < max && (data[j] == '+' || data[j] == '-') {
		neg = data[j] == '-'
		j++
	}
	// Resolve the effective base, consuming a 0x prefix for hex/auto.
	eff := base
	if (base == 16 || base == 0) && j+1 < max && data[j] == '0' && (data[j+1] == 'x' || data[j+1] == 'X') {
		j += 2
		eff = 16
	} else if base == 0 {
		if j < max && data[j] == '0' {
			eff = 8
		} else {
			eff = 10
		}
	}
	digitsStart := j
	for j < max && isDigitInBase(data[j], eff) {
		j++
	}
	if j == digitsStart {
		return nil, start, false
	}
	n, err := strconv.ParseInt(data[digitsStart:j], eff, 64)
	if err != nil {
		return nil, start, false
	}
	if neg {
		n = -n
	}
	return n, j, true
}

func isDigitInBase(b byte, base int) bool {
	var v int
	switch {
	case b >= '0' && b <= '9':
		v = int(b - '0')
	case b >= 'a' && b <= 'f':
		v = int(b-'a') + 10
	case b >= 'A' && b <= 'F':
		v = int(b-'A') + 10
	default:
		return false
	}
	return v < base
}

// scanFloat scans a floating-point number.
func scanFloat(data string, i, width int) (any, int, bool) {
	i = skipSpaces(data, i)
	start := i
	max := len(data)
	if width > 0 && start+width < max {
		max = start + width
	}
	j := i
	if j < max && (data[j] == '+' || data[j] == '-') {
		j++
	}
	seen := false
	for j < max && data[j] >= '0' && data[j] <= '9' {
		j++
		seen = true
	}
	if j < max && data[j] == '.' {
		j++
		for j < max && data[j] >= '0' && data[j] <= '9' {
			j++
			seen = true
		}
	}
	if !seen {
		return nil, start, false
	}
	// optional exponent
	if j < max && (data[j] == 'e' || data[j] == 'E') {
		k := j + 1
		if k < max && (data[k] == '+' || data[k] == '-') {
			k++
		}
		expStart := k
		for k < max && data[k] >= '0' && data[k] <= '9' {
			k++
		}
		if k > expStart {
			j = k
		}
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(data[start:j]), 64)
	if err != nil {
		return nil, start, false
	}
	return f, j, true
}
