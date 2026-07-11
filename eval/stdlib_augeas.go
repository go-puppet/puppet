// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strconv"
	"strings"

	"github.com/go-augeas/augeas"
	"github.com/go-pcore/pcore"
)

// registerStdlibAugeas installs validate_augeas, backed by the pure-Go
// github.com/go-augeas/augeas lens engine.
func registerStdlibAugeas(e *Evaluator) {
	e.funcs["validate_augeas"] = builtinValidateAugeas
}

// builtinValidateAugeas implements
// validate_augeas(content, lens[, tests[, message]]): it parses content with the
// named Augeas lens (e.g. "Sudoers.lns") and fails compilation if the lens
// cannot parse it. An optional third argument lists Augeas path expressions
// (relative to `$file`) that must NOT match; a fourth overrides the error
// message. It mirrors puppetlabs-stdlib's validate_augeas.
func builtinValidateAugeas(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 4 {
		return nil, &Error{Msg: "validate_augeas(): wrong number of arguments (must be 2, 3, or 4)"}
	}
	content, err := argStr(args, 0, "validate_augeas")
	if err != nil {
		return nil, err
	}
	lensName, err := argStr(args, 1, "validate_augeas")
	if err != nil {
		return nil, err
	}
	var tests []any
	if len(args) >= 3 && !isUndef(args[2]) {
		t, terr := argArr(args, 2, "validate_augeas")
		if terr != nil {
			return nil, terr
		}
		tests = t
	}
	msg := "validate_augeas(): Failed to validate content against " + strconv.Quote(lensName)
	if len(args) == 4 {
		m, merr := argStr(args, 3, "validate_augeas")
		if merr != nil {
			return nil, merr
		}
		msg = m
	}

	module, binding, ok := strings.Cut(lensName, ".")
	if !ok {
		return nil, &Error{Msg: "validate_augeas(): invalid lens name " + strconv.Quote(lensName)}
	}
	lens, lerr := augeas.NewEngine().Lens(module, binding)
	if lerr != nil {
		return nil, &Error{Msg: "validate_augeas(): unknown lens " + strconv.Quote(lensName) + ": " + lerr.Error()}
	}

	aug := augeas.New()
	const path = "/files/validate_augeas"
	if serr := aug.TextStore(lens, path, content); serr != nil {
		return nil, &Error{Msg: msg + " with error: " + serr.Error()}
	}

	// path was just created by TextStore, so this cannot fail.
	_, _ = aug.DefineVariable("file", path)
	for _, t := range tests {
		expr, ok := normalize(t).(string)
		if !ok {
			return nil, &Error{Msg: "validate_augeas(): each test path must be a String"}
		}
		if len(aug.Match(expr)) != 0 {
			return nil, &Error{Msg: msg + " testing path " + expr}
		}
	}
	return pcore.Undef, nil
}
