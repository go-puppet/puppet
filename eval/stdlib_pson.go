// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"encoding/json"
	"strings"
)

// registerStdlibPson installs parsepson, which parses PSON (Puppet Structured
// Object Notation) into the value model.
func registerStdlibPson(e *Evaluator) {
	e.funcs["parsepson"] = builtinParsePSON
}

// builtinParsePSON implements parsepson(String[, default]). PSON is Puppet's
// JSON variant (it additionally permits raw binary bytes inside strings); its
// grammar is otherwise JSON, so a standard JSON decode reproduces PSON.load's
// result for every text input. On a parse failure the optional default is
// returned, or the error is raised when no default was given.
func builtinParsePSON(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("parsepson")
	}
	s, err := argStr(args, 0, "parsepson")
	if err != nil {
		return nil, err
	}
	v, perr := parsePSONString(s)
	if perr != nil {
		if len(args) == 2 {
			return args[1], nil
		}
		return nil, perr
	}
	return v, nil
}

// parsePSONString decodes one PSON document into the value model, reusing the
// shared JSON-to-value lowering so integers, floats, nulls, arrays and objects
// match parsejson exactly.
func parsePSONString(s string) (Value, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, &Error{Msg: "parsepson(): " + err.Error()}
	}
	return jsonToValue(raw), nil
}
