// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"bytes"
	"encoding/base64"
	"math/big"
	"strconv"
	"strings"
)

// registerStdlibEncode installs assorted string-encoding functions from
// puppetlabs-stdlib (base64, convert_base, dos2unix, unix2dos) and Puppet core
// (scanf).
func registerStdlibEncode(e *Evaluator) {
	e.funcs["base64"] = builtinBase64
	e.funcs["convert_base"] = builtinConvertBase
	e.funcs["dos2unix"] = strFn1(func(s string) string {
		return strings.ReplaceAll(s, "\r\n", "\n")
	}, "dos2unix")
	e.funcs["unix2dos"] = strFn1(unix2dos, "unix2dos")
	e.funcs["scanf"] = builtinScanf
}

// strFn1 wraps a string->string transform taking one String argument.
func strFn1(f func(string) string, name string) Function {
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) != 1 {
			return nil, wrongArgs(name)
		}
		s, err := argStr(args, 0, name)
		if err != nil {
			return nil, err
		}
		return f(s), nil
	}
}

// unix2dos converts LF (and existing CRLF) line endings to CRLF, matching
// stdlib's gsub(/\r?\n/, "\r\n").
func unix2dos(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			b.WriteString("\r\n")
			i++
			continue
		}
		if s[i] == '\n' {
			b.WriteString("\r\n")
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// builtinBase64 implements base64(action, string, [method]). action is
// 'encode'|'decode'; method is 'default'|'strict'|'urlsafe'.
func builtinBase64(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, &Error{Msg: "base64(): expects (action, string, [method])"}
	}
	action, err := argStr(args, 0, "base64")
	if err != nil {
		return nil, err
	}
	if action != "encode" && action != "decode" {
		return nil, &Error{Msg: "base64(): first argument must be 'encode' or 'decode', got " + strconv.Quote(action)}
	}
	input, err := argStr(args, 1, "base64")
	if err != nil {
		return nil, err
	}
	method := "default"
	if len(args) == 3 {
		method, err = argStr(args, 2, "base64")
		if err != nil {
			return nil, err
		}
	}
	switch method {
	case "default", "strict", "urlsafe":
	default:
		return nil, &Error{Msg: "base64(): method must be 'default', 'strict' or 'urlsafe', got " + strconv.Quote(method)}
	}

	if action == "encode" {
		switch method {
		case "strict":
			return base64.StdEncoding.EncodeToString([]byte(input)), nil
		case "urlsafe":
			return base64.URLEncoding.EncodeToString([]byte(input)), nil
		default:
			return wrap64(base64.StdEncoding.EncodeToString([]byte(input))), nil
		}
	}
	// decode
	switch method {
	case "strict":
		out, derr := base64.StdEncoding.DecodeString(input)
		if derr != nil {
			return nil, &Error{Msg: "base64(): " + derr.Error()}
		}
		return string(out), nil
	case "urlsafe":
		out, derr := base64.URLEncoding.DecodeString(input)
		if derr != nil {
			return nil, &Error{Msg: "base64(): " + derr.Error()}
		}
		return string(out), nil
	default:
		// Ruby's Base64.decode64 ignores embedded newlines.
		dec := base64.NewDecoder(base64.StdEncoding, strings.NewReader(input))
		var buf bytes.Buffer
		if _, derr := buf.ReadFrom(dec); derr != nil {
			return nil, &Error{Msg: "base64(): " + derr.Error()}
		}
		return buf.String(), nil
	}
}

// wrap64 inserts a newline every 60 characters and appends a trailing newline,
// matching Ruby's Base64.encode64.
func wrap64(s string) string {
	var b strings.Builder
	for len(s) > 60 {
		b.WriteString(s[:60])
		b.WriteByte('\n')
		s = s[60:]
	}
	b.WriteString(s)
	b.WriteByte('\n')
	return b.String()
}

// builtinConvertBase implements convert_base(number, base): render the base-10
// number in the given target base (2..36).
func builtinConvertBase(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("convert_base")
	}
	n := new(big.Int)
	switch v := normalize(args[0]).(type) {
	case int64:
		n.SetInt64(v)
	case string:
		if _, ok := n.SetString(v, 10); !ok {
			return nil, &Error{Msg: "convert_base(): first argument must be an integer or its base-10 string"}
		}
	default:
		return nil, &Error{Msg: "convert_base(): first argument must be an integer or its base-10 string"}
	}
	base, err := baseArg(args[1])
	if err != nil {
		return nil, err
	}
	return n.Text(base), nil
}

// baseArg extracts a radix (2..36) from an Integer or numeric String.
func baseArg(v Value) (int, error) {
	var base int
	switch b := normalize(v).(type) {
	case int64:
		base = int(b)
	case string:
		n, err := strconv.Atoi(b)
		if err != nil {
			return 0, &Error{Msg: "convert_base(): second argument must be a base between 2 and 36"}
		}
		base = n
	default:
		return 0, &Error{Msg: "convert_base(): second argument must be a base between 2 and 36"}
	}
	if base < 2 || base > 36 {
		return 0, &Error{Msg: "convert_base(): second argument must be a base between 2 and 36"}
	}
	return base, nil
}
