// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import "strings"

// This file implements the puppetlabs-stdlib shell/regexp escaping and
// tokenising functions in pure Go, byte-for-byte compatible with the Ruby
// reference implementations:
//
//   - shell_escape / stdlib::shell_escape  -> Ruby Shellwords.shellescape
//   - shell_join                           -> Ruby Shellwords.shelljoin
//   - shell_split                          -> Ruby Shellwords.shellsplit
//   - batch_escape / stdlib::batch_escape  -> Windows cmd.exe quoting
//   - powershell_escape / stdlib::...      -> PowerShell backtick quoting
//   - regexpescape                         -> Ruby Regexp.escape

// registerStdlibShellwords installs the escaping/tokenising helpers.
func registerStdlibShellwords(e *Evaluator) {
	e.funcs["shell_escape"] = builtinShellEscape
	e.funcs["stdlib::shell_escape"] = builtinShellEscape
	e.funcs["shell_join"] = builtinShellJoin
	e.funcs["shell_split"] = builtinShellSplit
	e.funcs["batch_escape"] = builtinBatchEscape
	e.funcs["stdlib::batch_escape"] = builtinBatchEscape
	e.funcs["powershell_escape"] = builtinPowershellEscape
	e.funcs["stdlib::powershell_escape"] = builtinPowershellEscape
	e.funcs["regexpescape"] = builtinRegexpEscape
}

// isShellSpace reports whether c is a Ruby \s whitespace byte.
func isShellSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '\f', '\v':
		return true
	}
	return false
}

// shellSafe reports whether an ASCII byte is left unescaped by
// Ruby's Shellwords.shellescape.
func shellSafe(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
		c == '_' || c == '-' || c == '.' || c == ',' || c == ':' || c == '+' ||
		c == '/' || c == '@'
}

// rubyShellEscape reproduces Ruby Shellwords.shellescape(str): each character
// outside the safe set is prefixed with a backslash, an empty string becomes
// ”, and a newline becomes a quoted literal newline ('\n').
func rubyShellEscape(s string) string {
	if s == "" {
		return "''"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteString("'\n'")
		case r < 128 && shellSafe(byte(r)):
			b.WriteByte(byte(r))
		default:
			b.WriteByte('\\')
			b.WriteRune(r)
		}
	}
	return b.String()
}

func builtinShellEscape(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("shell_escape")
	}
	return rubyShellEscape(stringify(args[0])), nil
}

func builtinShellJoin(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, &Error{Msg: "shell_join(): wrong number of arguments (given " + ordinal(len(args)-1) + ", expected 1)"}
	}
	arr, err := argArr(args, 0, "shell_join")
	if err != nil {
		return nil, err
	}
	parts := make([]string, len(arr))
	for i, e := range arr {
		parts[i] = rubyShellEscape(stringify(e))
	}
	return strings.Join(parts, " "), nil
}

func builtinShellSplit(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, &Error{Msg: "shell_split(): wrong number of arguments (given " + ordinal(len(args)-1) + ", expected 1)"}
	}
	toks, err := shellSplit(stringify(args[0]))
	if err != nil {
		return nil, err
	}
	out := make([]any, len(toks))
	for i, t := range toks {
		out[i] = t
	}
	return out, nil
}

// shellSplit reproduces Ruby Shellwords.shellsplit(line): it splits line into
// tokens the way a Bourne shell would, honouring single quotes, double quotes
// and backslash escapes, and errors on an unmatched quote.
func shellSplit(line string) ([]string, error) {
	var words []string
	field := make([]byte, 0, len(line))
	n := len(line)
	for i := 0; i < n; {
		for i < n && isShellSpace(line[i]) { // \s*
			i++
		}
		if i >= n {
			break
		}
		matched := false
		var piece []byte
		switch line[i] {
		case '\'': // '([^']*)'
			j := i + 1
			for j < n && line[j] != '\'' {
				j++
			}
			if j < n {
				piece = []byte(line[i+1 : j])
				i = j + 1
				matched = true
			}
		case '"': // "((?:[^"\\]|\\.)*)"
			j := i + 1
			var buf []byte
			closed := false
			for j < n {
				if line[j] == '"' {
					closed = true
					break
				}
				if line[j] == '\\' {
					if j+1 >= n {
						break // trailing backslash, no closing quote
					}
					buf = append(buf, line[j+1])
					j += 2
					continue
				}
				buf = append(buf, line[j])
				j++
			}
			if closed {
				piece = buf
				i = j + 1
				matched = true
			}
		}
		if !matched && line[i] == '\\' { // \\.?
			if i+1 < n {
				piece = []byte{line[i+1]}
				i += 2
			} else {
				piece = []byte{'\\'}
				i++
			}
			matched = true
		}
		if !matched {
			c := line[i]
			if !isShellSpace(c) && c != '\\' && c != '\'' && c != '"' { // [^\s\\'"]+
				j := i
				for j < n {
					d := line[j]
					if isShellSpace(d) || d == '\\' || d == '\'' || d == '"' {
						break
					}
					j++
				}
				piece = []byte(line[i:j])
				i = j
			} else {
				return nil, &Error{Msg: "shell_split(): Unmatched quote: " + line}
			}
		}
		field = append(field, piece...)
		if i >= n { // \z separator
			words = append(words, string(field))
			field = field[:0]
		} else if isShellSpace(line[i]) { // \s separator
			i++
			words = append(words, string(field))
			field = field[:0]
		}
	}
	return words, nil
}

func builtinBatchEscape(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, &Error{Msg: "batch_escape(): wrong number of arguments (given " + ordinal(len(args)-1) + ", expected 1)"}
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range stringify(args[0]) {
		switch r {
		case '"':
			b.WriteString(`""`)
		case '$', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

func builtinPowershellEscape(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, &Error{Msg: "powershell_escape(): wrong number of arguments (given " + ordinal(len(args)-1) + ", expected 1)"}
	}
	var b strings.Builder
	for _, r := range stringify(args[0]) {
		switch r {
		case ' ', '\'', '`', '|', '\n', '$':
			b.WriteByte('`')
			b.WriteRune(r)
		case '"':
			b.WriteString("\\`\"")
		default:
			b.WriteRune(r)
		}
	}
	return b.String(), nil
}

// rubyRegexpEscape reproduces Ruby Regexp.escape(str): metacharacters are
// backslash-escaped and the whitespace control characters are rendered as their
// escape sequences. All other characters (including multibyte runes) pass
// through unchanged.
func rubyRegexpEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 {
			switch byte(r) {
			case '[', ']', '{', '}', '(', ')', '|', '-', '*', '.', '\\', '?', '+', '^', '$', ' ', '#':
				b.WriteByte('\\')
				b.WriteByte(byte(r))
				continue
			case '\t':
				b.WriteString(`\t`)
				continue
			case '\n':
				b.WriteString(`\n`)
				continue
			case '\r':
				b.WriteString(`\r`)
				continue
			case '\f':
				b.WriteString(`\f`)
				continue
			case '\v':
				b.WriteString(`\v`)
				continue
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func builtinRegexpEscape(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, &Error{Msg: "regexpescape(): Wrong number of arguments given (0 for 1)"}
	}
	switch v := normalize(args[0]).(type) {
	case string:
		return rubyRegexpEscape(v), nil
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			if s, ok := normalize(e).(string); ok {
				out[i] = rubyRegexpEscape(s)
			} else {
				out[i] = e
			}
		}
		return out, nil
	default:
		return nil, &Error{Msg: "regexpescape(): Requires either array or string to work with"}
	}
}
