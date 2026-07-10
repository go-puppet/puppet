// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"math"

	hocon "github.com/go-hocon/hocon"
	"github.com/go-pcore/pcore"
)

// registerStdlibHocon installs parsehocon, which parses a HOCON string into the
// Puppet data model, backed by the pure-Go github.com/go-hocon/hocon engine.
func registerStdlibHocon(e *Evaluator) {
	e.funcs["parsehocon"] = builtinParseHocon
	e.funcs["stdlib::parsehocon"] = builtinParseHocon
}

// builtinParseHocon implements parsehocon(hocon_string [, default]). On a parse
// error it returns the optional default when supplied, otherwise the error,
// matching puppetlabs-stdlib which re-raises Hocon::ConfigError::ConfigParseError.
func builtinParseHocon(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("parsehocon")
	}
	s, err := argStr(args, 0, "parsehocon")
	if err != nil {
		return nil, err
	}
	cfg, perr := hocon.Parse(s)
	if perr != nil {
		if len(args) == 2 {
			return args[1], nil
		}
		return nil, &Error{Msg: "parsehocon(): " + perr.Error()}
	}
	return hoconToValue(cfg.Root().Unwrap()), nil
}

// hoconToValue converts a hocon.Unwrap tree (which renders every number as a
// float64) into the Puppet data model, narrowing integral floats to Integer to
// match the Java-backed reference (`1` -> Integer, `1.5` -> Float).
func hoconToValue(v any) Value {
	switch x := v.(type) {
	case nil:
		return pcore.Undef
	case float64:
		if x == math.Trunc(x) && !math.IsInf(x, 0) && x >= math.MinInt64 && x <= math.MaxInt64 {
			return int64(x)
		}
		return x
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = hoconToValue(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = hoconToValue(e)
		}
		return out
	default:
		return x
	}
}
