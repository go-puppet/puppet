// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/ast"
)

// builtinCreateResources implements create_resources(type, resources, defaults):
// it declares one resource of the named type per key in the resources hash, each
// with the merged defaults + per-resource parameters.
func builtinCreateResources(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("create_resources")
	}
	typeName, err := argStr(args, 0, "create_resources")
	if err != nil {
		return nil, err
	}
	resources, err := argHash(args, 1, "create_resources")
	if err != nil {
		return nil, err
	}
	var defaults map[string]any
	if len(args) == 3 {
		d, err := argHash(args, 2, "create_resources")
		if err != nil {
			return nil, err
		}
		defaults = d
	}
	isClass := typeName == "class"
	for _, title := range sortedKeys(resources) {
		perParams, err := argHashAt(resources[title], "create_resources")
		if err != nil {
			return nil, err
		}
		params := applyDefaults(perParams, defaults)
		if isClass {
			if err := c.e.declareClass(title, params, ast.Position{}); err != nil {
				return nil, err
			}
			continue
		}
		if _, err := c.e.declareResource(typeName, title, params, ast.Regular, c.scope, ast.Position{}); err != nil {
			return nil, err
		}
	}
	return pcore.Undef, nil
}

func argHashAt(v Value, fn string) (map[string]any, error) {
	h, ok := normalize(v).(map[string]any)
	if !ok {
		return nil, &Error{Msg: fn + "(): each resource body must be a Hash of attributes"}
	}
	return h, nil
}

// builtinEnsureResource declares a resource only if it is not already in the
// catalog, matching puppetlabs-stdlib's ensure_resource().
func builtinEnsureResource(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("ensure_resource")
	}
	typeName, err := argStr(args, 0, "ensure_resource")
	if err != nil {
		return nil, err
	}
	var params map[string]any
	if len(args) == 3 {
		h, err := argHash(args, 2, "ensure_resource")
		if err != nil {
			return nil, err
		}
		params = h
	}
	for _, title := range titlesOf(args[1]) {
		ref := &ResourceRef{Type: capitalizeType(typeName), Title: title}
		if _, exists := c.e.cat.Get(ref.String()); exists {
			continue
		}
		if _, err := c.e.declareResource(typeName, title, params, ast.Regular, c.scope, ast.Position{}); err != nil {
			return nil, err
		}
	}
	return pcore.Undef, nil
}
