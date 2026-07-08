// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import "strings"

// Scope is a Puppet variable scope: a set of `$name => value` bindings with a
// lexical parent chain up to the top scope. Puppet variables are immutable once
// set within a scope.
type Scope struct {
	vars   map[string]Value
	parent *Scope
	top    *Scope
}

func newScope(parent *Scope) *Scope {
	s := &Scope{vars: map[string]Value{}, parent: parent}
	if parent == nil {
		s.top = s
	} else {
		s.top = parent.top
	}
	return s
}

// set binds name in this scope. It reports an error if name is already bound
// here (Puppet forbids reassigning a variable in the same scope).
func (s *Scope) set(name string, v Value) error {
	if _, exists := s.vars[name]; exists {
		return &Error{Msg: "cannot reassign variable $" + name}
	}
	s.vars[name] = v
	return nil
}

// setForce binds name unconditionally (used for injected parameters/facts).
func (s *Scope) setForce(name string, v Value) { s.vars[name] = v }

// lookup resolves a variable reference. A leading `::` (or any qualified
// `a::b`) resolves from the top scope; a bare name walks the local chain and
// then falls back to the top scope.
func (s *Scope) lookup(name string) (Value, bool) {
	if strings.HasPrefix(name, "::") {
		v, ok := s.top.vars[strings.TrimPrefix(name, "::")]
		return v, ok
	}
	if strings.Contains(name, "::") {
		v, ok := s.top.vars[name]
		return v, ok
	}
	for cur := s; cur != nil; cur = cur.parent {
		if v, ok := cur.vars[name]; ok {
			return v, ok
		}
	}
	return nil, false
}
