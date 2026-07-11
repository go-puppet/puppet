// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

// Sensitive is the value-model wrapper for Puppet's Sensitive data type. It
// carries an arbitrary value while redacting it in every rendered form (to_s,
// inspect, interpolation) so secrets do not leak into logs or catalogs. The
// wrapped value is retrievable only through [Sensitive.Unwrap] (or the `unwrap`
// function). It is used by resource-api sensitive-parameter handling and by
// stdlib::rewrap_sensitive_data.
type Sensitive struct{ value Value }

// NewSensitive wraps v as a Sensitive value. Wrapping mirrors Puppet's
// Sensitive.new: any value (including an already-Sensitive one) may be wrapped.
func NewSensitive(v Value) *Sensitive { return &Sensitive{value: v} }

// Unwrap returns the wrapped value.
func (s *Sensitive) Unwrap() Value { return s.value }

// String renders the redacted form. Because [stringify] and [inspect] fall
// through to fmt "%v" for unknown types, this is what appears wherever a
// Sensitive is stringified.
func (s *Sensitive) String() string { return "Sensitive [value redacted]" }

// registerStdlibSensitive installs the Sensitive-related functions: the core
// `unwrap` and puppetlabs-stdlib's `stdlib::rewrap_sensitive_data`.
func registerStdlibSensitive(e *Evaluator) {
	e.funcs["unwrap"] = builtinUnwrap
	e.funcs["stdlib::rewrap_sensitive_data"] = builtinRewrapSensitiveData
}

// builtinUnwrap implements the core `unwrap` function: it yields the value held
// by a Sensitive. With a lambda it passes the unwrapped value to the block and
// returns the block's result (so the clear value never escapes into a
// variable). A non-Sensitive argument is returned unchanged, matching Puppet's
// leniency for already-clear data.
func builtinUnwrap(_ *Context, args []Value, block *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("unwrap")
	}
	v := args[0]
	if s, ok := v.(*Sensitive); ok {
		v = s.Unwrap()
	}
	if block != nil {
		return block.Call(v)
	}
	return v, nil
}

// builtinRewrapSensitiveData implements stdlib::rewrap_sensitive_data: it deeply
// unwraps every Sensitive found within data, optionally runs a lambda on the
// unwrapped structure, and re-wraps the result as Sensitive if (and only if) any
// Sensitive value was present. This is what lets stdlib::to_toml and friends
// transparently accept sensitive parameters.
func builtinRewrapSensitiveData(_ *Context, args []Value, block *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("stdlib::rewrap_sensitive_data")
	}
	found := false
	unwrapped := deepUnwrap(args[0], &found)
	result := unwrapped
	if block != nil {
		v, err := block.Call(unwrapped)
		if err != nil {
			return nil, err
		}
		result = v
	}
	if found {
		return NewSensitive(result), nil
	}
	return result, nil
}

// deepUnwrap recursively replaces every Sensitive within v by its wrapped value,
// recording in found whether at least one was seen. Hash keys are always plain
// strings in this value model, so only values need unwrapping.
func deepUnwrap(v Value, found *bool) Value {
	switch x := v.(type) {
	case *Sensitive:
		*found = true
		return deepUnwrap(x.Unwrap(), found)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = deepUnwrap(val, found)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = deepUnwrap(e, found)
		}
		return out
	default:
		return v
	}
}
