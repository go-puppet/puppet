// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import "strings"

// isNumeric reports whether name is a run of ASCII digits (a match variable).
func isNumeric(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return true
}

// Scope is a Puppet variable scope: a set of `$name => value` bindings with a
// lexical parent chain up to the top scope. Puppet variables are immutable once
// set within a scope.
type Scope struct {
	vars     map[string]Value
	parent   *Scope
	top      *Scope
	match    []string                  // numbered match variables ($0..$n) from the last =~ here
	defaults map[string]map[string]any // resource defaults declared in this scope, by capitalized type
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

// snapshot returns all variables visible from this scope up the parent chain,
// with nearer bindings winning. Used to seed a sub-evaluator (e.g. an apply
// block) with the caller's locals.
func (s *Scope) snapshot() map[string]Value {
	out := map[string]Value{}
	var chain []*Scope
	for cur := s; cur != nil; cur = cur.parent {
		chain = append(chain, cur)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		for k, v := range chain[i].vars {
			out[k] = v
		}
	}
	return out
}

// setMatch records the numbered match variables ($0..$n) produced by a `=~`
// evaluated in this scope. groups is the FindStringSubmatch result (or nil to
// clear them on a failed match).
func (s *Scope) setMatch(groups []string) { s.match = groups }

// setDefaults records resource defaults for a capitalized type in this scope.
// Later declarations in the same or a nested scope inherit them.
func (s *Scope) setDefaults(capType string, attrs map[string]any) {
	if s.defaults == nil {
		s.defaults = map[string]map[string]any{}
	}
	existing := s.defaults[capType]
	if existing == nil {
		existing = map[string]any{}
		s.defaults[capType] = existing
	}
	for k, v := range attrs {
		existing[k] = v
	}
}

// lookupDefaults collects the effective resource defaults for capType by walking
// the scope chain, with the nearest scope winning on conflicts.
func (s *Scope) lookupDefaults(capType string) map[string]any {
	var chain []map[string]any
	for cur := s; cur != nil; cur = cur.parent {
		if cur.defaults != nil {
			if d, ok := cur.defaults[capType]; ok {
				chain = append(chain, d)
			}
		}
	}
	if len(chain) == 0 {
		return nil
	}
	out := map[string]any{}
	// Apply from the outermost inward so nearer scopes override.
	for i := len(chain) - 1; i >= 0; i-- {
		for k, v := range chain[i] {
			out[k] = v
		}
	}
	return out
}

// lookup resolves a variable reference. A leading `::` (or any qualified
// `a::b`) resolves from the top scope; a bare name walks the local chain and
// then falls back to the top scope.
func (s *Scope) lookup(name string) (Value, bool) {
	if isNumeric(name) {
		idx := 0
		for i := 0; i < len(name); i++ {
			idx = idx*10 + int(name[i]-'0')
		}
		for cur := s; cur != nil; cur = cur.parent {
			if cur.match != nil {
				if idx < len(cur.match) {
					return cur.match[idx], true
				}
				return nil, false
			}
		}
		return nil, false
	}
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
