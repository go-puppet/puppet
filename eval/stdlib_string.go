// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/go-pcore/pcore"
)

// registerStdlibString installs the string-manipulation function set.
func registerStdlibString(e *Evaluator) {
	e.funcs["lstrip"] = recursiveStrFn(func(s string) string { return strings.TrimLeft(s, " \t\n\r\f\v") })
	e.funcs["rstrip"] = recursiveStrFn(func(s string) string { return strings.TrimRight(s, " \t\n\r\f\v") })
	e.funcs["strip"] = recursiveStrFn(strings.TrimSpace)
	e.funcs["upcase"] = recursiveStrFn(strings.ToUpper)
	e.funcs["downcase"] = recursiveStrFn(strings.ToLower)
	e.funcs["capitalize"] = recursiveStrFn(capitalizeWord)
	e.funcs["swapcase"] = recursiveStrFn(swapCase)
	e.funcs["camelcase"] = recursiveStrFn(camelCase)
	e.funcs["chomp"] = recursiveStrFn(chomp)
	e.funcs["chop"] = recursiveStrFn(chop)
	e.funcs["squeeze"] = builtinSqueeze
	e.funcs["trim"] = recursiveStrFn(strings.TrimSpace)
	e.funcs["upcase_first"] = recursiveStrFn(upcaseFirst)
	e.funcs["start_with"] = builtinStartWith
	e.funcs["end_with"] = builtinEndWith
	e.funcs["str2bool"] = builtinStr2bool
	e.funcs["bool2str"] = builtinBool2str
	e.funcs["num2bool"] = builtinNum2bool
	e.funcs["bool2num"] = builtinBool2num
	e.funcs["str2num"] = builtinStr2num
	e.funcs["strlen"] = builtinStrlen
	e.funcs["uriescape"] = recursiveStrFn(uriEscape)
	e.funcs["shell_escape"] = builtinShellEscape
	e.funcs["versioncmp"] = builtinVersioncmp
	e.funcs["regsubst"] = builtinRegsubst
	e.funcs["match"] = builtinMatch
	e.funcs["stdlib::str2resource"] = builtinStrToResource
}

// recursiveStrFn wraps a string transform so that, like puppetlabs-stdlib, it
// also maps over arrays (element-wise) when given one.
func recursiveStrFn(f func(string) string) Function {
	var apply func(v Value) (Value, bool)
	apply = func(v Value) (Value, bool) {
		switch x := normalize(v).(type) {
		case string:
			return f(x), true
		case []any:
			out := make([]any, len(x))
			for i, el := range x {
				r, ok := apply(el)
				if !ok {
					return nil, false
				}
				out[i] = r
			}
			return out, true
		}
		return nil, false
	}
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) != 1 {
			return nil, &Error{Msg: "string function expects one argument"}
		}
		out, ok := apply(args[0])
		if !ok {
			return nil, &Error{Msg: "string function expects a String or Array of Strings"}
		}
		return out, nil
	}
}

func swapCase(s string) string {
	r := []rune(s)
	for i, c := range r {
		switch {
		case c >= 'a' && c <= 'z':
			r[i] = c - 32
		case c >= 'A' && c <= 'Z':
			r[i] = c + 32
		}
	}
	return string(r)
}

func camelCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		parts[i] = capitalizeWord(p)
	}
	return strings.Join(parts, "")
}

func chomp(s string) string {
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
}

func chop(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return s[:len(s)-2]
	}
	if s == "" {
		return s
	}
	r := []rune(s)
	return string(r[:len(r)-1])
}

func upcaseFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

func uriEscape(s string) string {
	const safe = "-_.!~*'()"
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.IndexByte(safe, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		const hex = "0123456789ABCDEF"
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

func builtinSqueeze(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("squeeze")
	}
	set := ""
	if len(args) == 2 {
		s, err := argStr(args, 1, "squeeze")
		if err != nil {
			return nil, err
		}
		set = s
	}
	s, err := argStr(args, 0, "squeeze")
	if err != nil {
		return nil, err
	}
	return squeeze(s, set), nil
}

func squeeze(s, set string) string {
	var b strings.Builder
	var prev rune
	first := true
	for _, c := range s {
		if !first && c == prev && (set == "" || strings.ContainsRune(set, c)) {
			continue
		}
		b.WriteRune(c)
		prev = c
		first = false
	}
	return b.String()
}

func builtinStartWith(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("start_with")
	}
	s, err := argStr(args, 0, "start_with")
	if err != nil {
		return nil, err
	}
	return anyPrefix(s, args[1]), nil
}

func builtinEndWith(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("end_with")
	}
	s, err := argStr(args, 0, "end_with")
	if err != nil {
		return nil, err
	}
	return anySuffix(s, args[1]), nil
}

func anyPrefix(s string, prefixes Value) bool {
	for _, p := range stringList(prefixes) {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func anySuffix(s string, suffixes Value) bool {
	for _, p := range stringList(suffixes) {
		if strings.HasSuffix(s, p) {
			return true
		}
	}
	return false
}

// stringList coerces a String or Array-of-Strings argument to a []string.
func stringList(v Value) []string {
	switch x := normalize(v).(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, stringify(e))
		}
		return out
	}
	return nil
}

func builtinStr2bool(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("str2bool")
	}
	if b, ok := normalize(args[0]).(bool); ok {
		return b, nil
	}
	s, err := argStr(args, 0, "str2bool")
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "y", "true", "yes":
		return true, nil
	case "", "0", "f", "n", "false", "no", "undef", "undefined":
		return false, nil
	}
	return nil, &Error{Msg: "str2bool(): unknown type of boolean given"}
}

func builtinBool2str(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 3 {
		return nil, wrongArgs("bool2str")
	}
	b, ok := normalize(args[0]).(bool)
	if !ok {
		return nil, &Error{Msg: "bool2str(): expects a Boolean"}
	}
	if len(args) == 3 {
		if b {
			return stringify(args[1]), nil
		}
		return stringify(args[2]), nil
	}
	return strconv.FormatBool(b), nil
}

func builtinNum2bool(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("num2bool")
	}
	switch x := normalize(args[0]).(type) {
	case int64:
		return x > 0, nil
	case float64:
		return x > 0, nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return nil, &Error{Msg: "num2bool(): unable to parse number " + x}
		}
		return f > 0, nil
	}
	return nil, &Error{Msg: "num2bool(): expects a Numeric or numeric String"}
}

func builtinBool2num(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("bool2num")
	}
	v := normalize(args[0])
	if b, ok := v.(bool); ok {
		if b {
			return int64(1), nil
		}
		return int64(0), nil
	}
	if s, ok := v.(string); ok {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "1", "t", "y", "true", "yes":
			return int64(1), nil
		default:
			return int64(0), nil
		}
	}
	return nil, &Error{Msg: "bool2num(): expects a Boolean or String"}
}

func builtinStr2num(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("str2num")
	}
	s, err := argStr(args, 0, "str2num")
	if err != nil {
		return nil, err
	}
	s = strings.TrimSpace(s)
	if i, err := strconv.ParseInt(s, 0, 64); err == nil {
		return i, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, &Error{Msg: "str2num(): cannot convert " + s}
	}
	return f, nil
}

func builtinStrlen(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("strlen")
	}
	s, err := argStr(args, 0, "strlen")
	if err != nil {
		return nil, err
	}
	return int64(len([]rune(s))), nil
}

func builtinShellEscape(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("shell_escape")
	}
	s := stringify(args[0])
	if s == "" {
		return "''", nil
	}
	safe := true
	for _, c := range []byte(s) {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' || c == '/' || c == ',' || c == ':' || c == '+' || c == '=') {
			safe = false
			break
		}
	}
	if safe {
		return s, nil
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'", nil
}

func builtinStrToResource(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("str2resource")
	}
	s, err := argStr(args, 0, "str2resource")
	if err != nil {
		return nil, err
	}
	open := strings.IndexByte(s, '[')
	if open < 0 || !strings.HasSuffix(s, "]") {
		return nil, &Error{Msg: "str2resource(): expected Type[title], got " + s}
	}
	typ := s[:open]
	title := s[open+1 : len(s)-1]
	title = strings.Trim(title, "'\"")
	return &ResourceRef{Type: typ, Title: title}, nil
}

// builtinVersioncmp compares two dotted version strings, returning -1, 0 or 1.
func builtinVersioncmp(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("versioncmp")
	}
	a, err := argStr(args, 0, "versioncmp")
	if err != nil {
		return nil, err
	}
	b, err := argStr(args, 1, "versioncmp")
	if err != nil {
		return nil, err
	}
	return int64(versionCompare(a, b)), nil
}

// versionCompare implements Puppet's Gem-style version comparison.
func versionCompare(a, b string) int {
	as := versionSegments(a)
	bs := versionSegments(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if c := compareSegment(x, y); c != 0 {
			return c
		}
	}
	return 0
}

func versionSegments(v string) []string {
	return regexp.MustCompile(`[.\-]`).Split(v, -1)
}

func compareSegment(x, y string) int {
	xi, xErr := strconv.Atoi(x)
	yi, yErr := strconv.Atoi(y)
	switch {
	case xErr == nil && yErr == nil:
		switch {
		case xi < yi:
			return -1
		case xi > yi:
			return 1
		default:
			return 0
		}
	case xErr == nil && yErr != nil:
		// numeric sorts after alpha (a pre-release like "1.0.rc" < "1.0.0")
		return 1
	case xErr != nil && yErr == nil:
		return -1
	default:
		return strings.Compare(x, y)
	}
}

// builtinRegsubst performs regex substitution on a String (or Array of Strings).
func builtinRegsubst(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 3 || len(args) > 5 {
		return nil, wrongArgs("regsubst")
	}
	pattern, err := regsubstPattern(args)
	if err != nil {
		return nil, err
	}
	repl, err := argStr(args, 2, "regsubst")
	if err != nil {
		return nil, err
	}
	repl = translateReplacement(repl)
	global := false
	if len(args) >= 4 {
		flags := stringList(args[3])
		for _, f := range flags {
			if strings.Contains(f, "G") || f == "global" {
				global = true
			}
		}
	}
	switch target := normalize(args[0]).(type) {
	case string:
		return regsubstOne(pattern, target, repl, global), nil
	case []any:
		out := make([]any, len(target))
		for i, el := range target {
			out[i] = regsubstOne(pattern, stringify(el), repl, global)
		}
		return out, nil
	}
	return nil, &Error{Msg: "regsubst(): first argument must be a String or Array"}
}

func regsubstPattern(args []Value) (*regexp.Regexp, error) {
	var src string
	switch p := normalize(args[1]).(type) {
	case *pcore.Regexp:
		src = p.Source()
	case string:
		src = p
	default:
		return nil, &Error{Msg: "regsubst(): pattern must be a String or Regexp"}
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, &Error{Msg: "regsubst(): invalid pattern: " + err.Error()}
	}
	return re, nil
}

// translateReplacement converts Puppet/Ruby `\1` backreferences to Go's `$1`.
func translateReplacement(repl string) string {
	var b strings.Builder
	rs := []rune(repl)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' && i+1 < len(rs) && rs[i+1] >= '0' && rs[i+1] <= '9' {
			b.WriteString("${")
			b.WriteRune(rs[i+1])
			b.WriteByte('}')
			i++
			continue
		}
		if rs[i] == '$' {
			b.WriteString("$$")
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

func regsubstOne(re *regexp.Regexp, target, repl string, global bool) string {
	if global {
		return re.ReplaceAllString(target, repl)
	}
	done := false
	return re.ReplaceAllStringFunc(target, func(m string) string {
		if done {
			return m
		}
		done = true
		sub := re.FindStringSubmatchIndex(m)
		return string(re.ExpandString(nil, repl, m, sub))
	})
}

// builtinMatch matches a String against a Regexp/String pattern (or an Array of
// them), returning the capture array or undef, per puppetlabs-stdlib match().
func builtinMatch(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("match")
	}
	if arr, ok := normalize(args[0]).([]any); ok {
		out := make([]any, len(arr))
		for i, el := range arr {
			m, err := matchOne(stringify(el), args[1])
			if err != nil {
				return nil, err
			}
			out[i] = m
		}
		return out, nil
	}
	s, err := argStr(args, 0, "match")
	if err != nil {
		return nil, err
	}
	return matchOne(s, args[1])
}

// matchPredicate builds a string predicate from a Regexp or a substring/String
// pattern, used by grep()/reject().
func matchPredicate(pattern Value) (func(string) bool, error) {
	switch p := normalize(pattern).(type) {
	case *pcore.Regexp:
		re, err := regexp.Compile(p.Source())
		if err != nil {
			return nil, &Error{Msg: err.Error()}
		}
		return re.MatchString, nil
	case string:
		re, err := regexp.Compile(p)
		if err != nil {
			return func(s string) bool { return strings.Contains(s, p) }, nil
		}
		return re.MatchString, nil
	}
	return nil, &Error{Msg: "expected a String or Regexp pattern"}
}

func matchOne(s string, pattern Value) (Value, error) {
	var src string
	switch p := normalize(pattern).(type) {
	case *pcore.Regexp:
		src = p.Source()
	case string:
		src = p
	default:
		return nil, &Error{Msg: "match(): pattern must be a String or Regexp"}
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, &Error{Msg: "match(): invalid pattern: " + err.Error()}
	}
	m := re.FindStringSubmatch(s)
	if m == nil {
		return pcore.Undef, nil
	}
	out := make([]any, len(m))
	for i, g := range m {
		out[i] = g
	}
	return out, nil
}
