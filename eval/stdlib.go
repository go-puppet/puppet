// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"github.com/go-pcore/pcore"
)

// registerStdlib installs the Puppet-core and puppetlabs-stdlib function set on
// top of the small bootstrap registered by registerBuiltins. Functions are
// grouped by category into sibling files. A host may still override any of them
// via [Evaluator.RegisterFunction].
func registerStdlib(e *Evaluator) {
	registerStdlibString(e)
	registerStdlibArray(e)
	registerStdlibHash(e)
	registerStdlibNumber(e)
	registerStdlibPath(e)
	registerStdlibType(e)
	registerStdlibData(e)
	registerStdlibMisc(e)
}

// --- small argument helpers -----------------------------------------------

// argStr returns args[i] as a string, or an error naming the function.
func argStr(args []Value, i int, fn string) (string, error) {
	s, ok := normalize(args[i]).(string)
	if !ok {
		return "", &Error{Msg: fn + "(): argument " + ordinal(i) + " must be a String"}
	}
	return s, nil
}

// argArr returns args[i] as an array.
func argArr(args []Value, i int, fn string) ([]any, error) {
	a, ok := normalize(args[i]).([]any)
	if !ok {
		return nil, &Error{Msg: fn + "(): argument " + ordinal(i) + " must be an Array"}
	}
	return a, nil
}

// argHash returns args[i] as a hash.
func argHash(args []Value, i int, fn string) (map[string]any, error) {
	h, ok := normalize(args[i]).(map[string]any)
	if !ok {
		return nil, &Error{Msg: fn + "(): argument " + ordinal(i) + " must be a Hash"}
	}
	return h, nil
}

func ordinal(i int) string {
	switch i {
	case 0:
		return "1"
	case 1:
		return "2"
	case 2:
		return "3"
	default:
		return "n"
	}
}

// wrongArgs builds a standard arity error.
func wrongArgs(fn string) error { return &Error{Msg: fn + "(): wrong number of arguments"} }

// cloneHash returns a shallow copy of m.
func cloneHash(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// registerStdlibMisc installs functions that do not belong to a single value
// category: reflection, resource generation, and the regex/version helpers real
// manifests depend on.
func registerStdlibMisc(e *Evaluator) {
	e.funcs["defined"] = builtinDefined
	e.funcs["getvar"] = builtinGetvar
	e.funcs["getparam"] = builtinGetparam
	e.funcs["create_resources"] = builtinCreateResources
	e.funcs["ensure_resource"] = builtinEnsureResource
	e.funcs["assert_private"] = func(_ *Context, _ []Value, _ *Block) (Value, error) { return pcore.Undef, nil }
}

// builtinDefined implements defined($name) for classes, resource types,
// resource references, and `'$var'` variable checks.
func builtinDefined(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) == 0 {
		return nil, wrongArgs("defined")
	}
	for _, a := range args {
		if c.e.isDefined(a) {
			return true, nil
		}
	}
	return false, nil
}

func (e *Evaluator) isDefined(a Value) bool {
	switch x := normalize(a).(type) {
	case *ResourceRef:
		if x.Type == "Class" {
			return e.included[classKey(x.Title)]
		}
		_, ok := e.cat.Get(x.String())
		return ok
	case pcore.Type:
		return true
	case string:
		if len(x) > 1 && x[0] == '$' {
			_, ok := e.top.lookup(x[1:])
			return ok
		}
		if _, ok := e.classes[x]; ok {
			return true
		}
		if _, ok := e.defines[x]; ok {
			return true
		}
		if _, ok := e.funcs[x]; ok {
			return true
		}
		_, ok := e.userFuncs[x]
		return ok
	}
	return false
}

// classKey lowercases a class title for lookup in the included set.
func classKey(title string) string { return lowerFirstSegments(title) }

func builtinGetvar(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, wrongArgs("getvar")
	}
	name, err := argStr(args, 0, "getvar")
	if err != nil {
		return nil, err
	}
	v, ok := c.scope.lookup(name)
	if !ok {
		if len(args) >= 2 {
			return args[1], nil
		}
		return pcore.Undef, nil
	}
	return v, nil
}

func builtinGetparam(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("getparam")
	}
	refs := collectRefs(args[0])
	if len(refs) == 0 {
		return pcore.Undef, nil
	}
	param, err := argStr(args, 1, "getparam")
	if err != nil {
		return nil, err
	}
	res, ok := c.e.cat.Get(refs[0])
	if !ok {
		return pcore.Undef, nil
	}
	if v, ok := res.Parameters[param]; ok {
		return v, nil
	}
	return pcore.Undef, nil
}
