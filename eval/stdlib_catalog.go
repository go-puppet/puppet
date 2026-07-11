// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"

	"github.com/go-pcore/pcore"
)

// registerStdlibCatalog installs the catalog- and fact-aware stdlib functions
// that are nonetheless pure with respect to the host: deprecation logging,
// resource-attribute queries, and bulk resource declaration.
func registerStdlibCatalog(e *Evaluator) {
	e.funcs["deprecation"] = builtinDeprecation
	e.funcs["defined_with_params"] = builtinDefinedWithParams
	e.funcs["ensure_resources"] = builtinEnsureResources
	e.funcs["ensure_packages"] = builtinEnsurePackages
	e.funcs["stdlib::ensure_packages"] = builtinEnsurePackages
	e.funcs["os_version_gte"] = builtinOSVersionGTE
	e.funcs["stdlib::os_version_gte"] = builtinOSVersionGTE
}

// builtinDeprecation implements deprecation(key, message[, use_strict_setting]):
// it logs a warning at most once per key. go-puppet has no `strict` setting, so
// the optional third argument is validated but does not alter behaviour.
func builtinDeprecation(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("deprecation")
	}
	key, err := argStr(args, 0, "deprecation")
	if err != nil {
		return nil, err
	}
	message, err := argStr(args, 1, "deprecation")
	if err != nil {
		return nil, err
	}
	if len(args) == 3 {
		if _, ok := normalize(args[2]).(bool); !ok {
			return nil, &Error{Msg: "deprecation(): third argument must be a Boolean"}
		}
	}
	if c.e.deprecated == nil {
		c.e.deprecated = map[string]bool{}
	}
	if !c.e.deprecated[key] {
		c.e.deprecated[key] = true
		c.Log("warning", message)
	}
	return pcore.Undef, nil
}

// builtinDefinedWithParams implements defined_with_params(reference[, params]):
// it returns true when a resource matching reference is already in the catalog
// and every requested parameter matches (an undef expectation matches an absent
// or undef attribute, as in puppetlabs-stdlib).
func builtinDefinedWithParams(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("defined_with_params")
	}
	ref, ok := resourceRefString(args[0])
	if !ok {
		return nil, &Error{Msg: "defined_with_params(): first argument must be a resource reference"}
	}
	var params map[string]any
	if len(args) == 2 && !isUndef(args[1]) {
		h, err := argHash(args, 1, "defined_with_params")
		if err != nil {
			return nil, err
		}
		params = h
	}
	res, exists := c.e.cat.Get(ref)
	if !exists {
		return false, nil
	}
	for k, want := range params {
		got, present := res.Parameters[k]
		if isUndef(want) && (!present || isUndef(got)) {
			continue
		}
		if !present || !equals(got, want) {
			return false, nil
		}
	}
	return true, nil
}

// resourceRefString canonicalises a resource reference given either as a
// ResourceRef or a "Type[title]" string into the catalog key form.
func resourceRefString(v Value) (string, bool) {
	switch x := normalize(v).(type) {
	case *ResourceRef:
		return x.String(), true
	case string:
		if i := strings.IndexByte(x, '['); i > 0 && strings.HasSuffix(x, "]") {
			return capitalizeType(x[:i]) + "[" + x[i+1:len(x)-1] + "]", true
		}
	}
	return "", false
}

// builtinEnsureResources implements ensure_resources(type, resources[, params]):
// like create_resources but idempotent, declaring each resource only when it is
// not already in the catalog. Per-resource attributes override the shared
// defaults.
func builtinEnsureResources(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("ensure_resources")
	}
	typeName, err := argStr(args, 0, "ensure_resources")
	if err != nil {
		return nil, err
	}
	resources, err := argHash(args, 1, "ensure_resources")
	if err != nil {
		return nil, &Error{Msg: "ensure_resources(): Requires second argument to be a Hash"}
	}
	var params map[string]any
	if len(args) == 3 && !isUndef(args[2]) {
		h, herr := argHash(args, 2, "ensure_resources")
		if herr != nil {
			return nil, herr
		}
		params = h
	}
	for _, title := range sortedKeys(resources) {
		merged := params
		if per, ok := normalize(resources[title]).(map[string]any); ok {
			merged = mergeHashes(params, per)
		}
		if _, rerr := builtinEnsureResource(c, []Value{typeName, title, orEmptyHash(merged)}, nil); rerr != nil {
			return nil, rerr
		}
	}
	return pcore.Undef, nil
}

// builtinEnsurePackages implements stdlib::ensure_packages(packages[, defaults]):
// it declares a Package resource per name (idempotently, via ensure_resource),
// merging the shared defaults with any per-package attributes. It accepts a
// single package name, an array of names, or a hash of name => attributes.
func builtinEnsurePackages(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("stdlib::ensure_packages")
	}
	var defaults map[string]any
	if len(args) == 2 && !isUndef(args[1]) {
		h, err := argHash(args, 1, "stdlib::ensure_packages")
		if err != nil {
			return nil, err
		}
		defaults = h
	}
	switch pkgs := normalize(args[0]).(type) {
	case map[string]any:
		for _, name := range sortedKeys(pkgs) {
			attrs, _ := normalize(pkgs[name]).(map[string]any)
			if err := ensureOnePackage(c, name, mergeHashes(defaults, attrs)); err != nil {
				return nil, err
			}
		}
	case []any:
		for _, p := range pkgs {
			name, ok := normalize(p).(string)
			if !ok {
				return nil, &Error{Msg: "stdlib::ensure_packages(): package names must be Strings"}
			}
			if err := ensureOnePackage(c, name, defaults); err != nil {
				return nil, err
			}
		}
	case string:
		if err := ensureOnePackage(c, pkgs, defaults); err != nil {
			return nil, err
		}
	default:
		return nil, &Error{Msg: "stdlib::ensure_packages(): expects a String, Array, or Hash of packages"}
	}
	return pcore.Undef, nil
}

// ensureOnePackage declares one Package with the merged attributes, applying the
// upstream default-ensure rule: an unspecified (or present/installed) ensure
// becomes "present" when the package is already declared with ensure => present,
// otherwise "installed".
func ensureOnePackage(c *Context, name string, defaults map[string]any) error {
	attrs := mergeHashes(map[string]any{"ensure": "installed"}, defaults)
	if ens, _ := normalize(attrs["ensure"]).(string); ens == "present" || ens == "installed" {
		attrs["ensure"] = defaultEnsure(c, name)
	}
	_, err := builtinEnsureResource(c, []Value{"package", name, attrs}, nil)
	return err
}

// defaultEnsure returns "present" if Package[name] is already declared with
// ensure => present in the catalog, else "installed".
func defaultEnsure(c *Context, name string) string {
	v, _ := builtinDefinedWithParams(c, []Value{"Package[" + name + "]", map[string]any{"ensure": "present"}}, nil)
	if b, _ := v.(bool); b {
		return "present"
	}
	return "installed"
}

// builtinOSVersionGTE implements stdlib::os_version_gte(os, version): it reports
// whether the compiling node's `$facts['os']['name']` equals os and its
// `$facts['os']['release']['major']` is version-wise >= version.
func builtinOSVersionGTE(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("stdlib::os_version_gte")
	}
	os, err := argStr(args, 0, "stdlib::os_version_gte")
	if err != nil {
		return nil, err
	}
	version, err := argStr(args, 1, "stdlib::os_version_gte")
	if err != nil {
		return nil, err
	}
	name, nok := digFactString(c, "os", "name")
	major, mok := digFactString(c, "os", "release", "major")
	if !nok || !mok {
		return nil, &Error{Msg: "stdlib::os_version_gte(): the os.name and os.release.major facts are required"}
	}
	return name == os && versionCompare(major, version) >= 0, nil
}

// digFactString resolves a dotted `$facts` path to a string value.
func digFactString(c *Context, segs ...string) (string, bool) {
	v, ok := c.scope.lookup("facts")
	if !ok {
		return "", false
	}
	cur, ok := dig(normalize(v), segs)
	if !ok {
		return "", false
	}
	s, ok := normalize(cur).(string)
	return s, ok
}

// mergeHashes returns a new hash of base overlaid with over (over wins). A nil
// input is treated as empty.
func mergeHashes(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// orEmptyHash returns h, or an empty hash when h is nil, so ensure_resource
// always receives a hash argument.
func orEmptyHash(h map[string]any) map[string]any {
	if h == nil {
		return map[string]any{}
	}
	return h
}
