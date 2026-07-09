// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"path"
	"strings"
)

// registerStdlibPath installs the path-manipulation function set.
func registerStdlibPath(e *Evaluator) {
	e.funcs["basename"] = builtinBasename
	e.funcs["dirname"] = builtinDirname
	e.funcs["extname"] = builtinExtname
}

func builtinBasename(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("basename")
	}
	p, err := argStr(args, 0, "basename")
	if err != nil {
		return nil, err
	}
	base := path.Base(strings.TrimRight(p, "/"))
	if base == "." || base == "/" {
		base = ""
	}
	if len(args) == 2 {
		suffix, err := argStr(args, 1, "basename")
		if err != nil {
			return nil, err
		}
		base = strings.TrimSuffix(base, suffix)
	}
	return base, nil
}

func builtinDirname(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("dirname")
	}
	p, err := argStr(args, 0, "dirname")
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimRight(p, "/")
	if !strings.Contains(trimmed, "/") {
		if strings.HasPrefix(p, "/") {
			return "/", nil
		}
		return ".", nil
	}
	return path.Dir(trimmed), nil
}

func builtinExtname(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("extname")
	}
	p, err := argStr(args, 0, "extname")
	if err != nil {
		return nil, err
	}
	base := path.Base(p)
	// A leading-dot filename ("./.bashrc" -> ".bashrc") has no extension.
	if strings.HasPrefix(base, ".") {
		base = base[1:]
	}
	dot := strings.LastIndexByte(base, '.')
	if dot <= 0 {
		return "", nil
	}
	return base[dot:], nil
}
