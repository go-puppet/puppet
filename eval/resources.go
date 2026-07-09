// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/catalog"
)

// applyDefaults returns params with any keys present in defaults but missing
// from params filled in. An explicit attribute always wins over a default.
func applyDefaults(params, defaults map[string]any) map[string]any {
	if len(defaults) == 0 {
		return params
	}
	out := map[string]any{}
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range params {
		out[k] = v
	}
	return out
}

// evalResourceDefaults records `Type { attr => val }` defaults in the current
// scope, applying to resources of that type declared in this or nested scopes.
func (e *Evaluator) evalResourceDefaults(x *ast.ResourceDefaults, s *Scope) (Value, error) {
	ref, ok := x.Type.(*ast.QualifiedReference)
	if !ok {
		return nil, &Error{Pos: x.Pos(), Msg: "resource defaults require a type reference"}
	}
	attrs, err := e.evalAttributeOps(x.Ops, s)
	if err != nil {
		return nil, err
	}
	s.setDefaults(capitalizeType(ref.Value), attrs)
	return pcore.Undef, nil
}

// evalResourceOverride applies `Type[title] { attr => val }` to already-declared
// resources, overriding parameters and adding relationship edges.
func (e *Evaluator) evalResourceOverride(x *ast.ResourceOverride, s *Scope) (Value, error) {
	target, err := e.eval(x.Resource, s)
	if err != nil {
		return nil, err
	}
	refs := collectRefs(target)
	if len(refs) == 0 {
		return nil, &Error{Pos: x.Pos(), Msg: "resource override target must be a resource reference"}
	}
	attrs, err := e.evalAttributeOps(x.Ops, s)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		res, ok := e.cat.Get(ref)
		if !ok {
			return nil, &Error{Pos: x.Pos(), Msg: "cannot override resource " + ref + ": it is not in the catalog"}
		}
		e.applyOverride(res, ref, x.Ops, attrs)
	}
	return pcore.Undef, nil
}

// applyOverride writes the override attributes onto res, honouring `+>`
// (append) operations and routing relationship metaparameters to edges.
func (e *Evaluator) applyOverride(res *catalog.Resource, ref string, ops []ast.AttributeOp, attrs map[string]any) {
	appendOps := map[string]bool{}
	for _, op := range ops {
		if op.Op == "+>" {
			appendOps[op.Name] = true
		}
	}
	for k, v := range attrs {
		if dir, ok := metaparams[k]; ok {
			for _, other := range collectRefs(v) {
				if dir == "forward" {
					e.cat.AddEdge(ref, other)
				} else {
					e.cat.AddEdge(other, ref)
				}
			}
			continue
		}
		if k == "tag" {
			res.Tags = append(res.Tags, tagStrings(v)...)
			continue
		}
		if appendOps[k] {
			res.Parameters[k] = appendValue(res.Parameters[k], v)
			continue
		}
		res.Parameters[k] = v
	}
}

// appendValue implements the `+>` attribute-append operation: array/hash merge,
// otherwise replacement.
func appendValue(existing, add Value) Value {
	if ea, ok := normalize(existing).([]any); ok {
		if aa, ok := normalize(add).([]any); ok {
			return append(append([]any{}, ea...), aa...)
		}
		return append(append([]any{}, ea...), add)
	}
	if em, ok := normalize(existing).(map[string]any); ok {
		if am, ok := normalize(add).(map[string]any); ok {
			return mergeHash(em, am)
		}
	}
	return add
}

// evalCollector realizes virtual (`<| |>`) or exported (`<<| |>>`) resources
// whose attributes match the query, returning references to those realized.
func (e *Evaluator) evalCollector(x *ast.Collector, s *Scope) (Value, error) {
	ref, ok := x.Type.(*ast.QualifiedReference)
	if !ok {
		return nil, &Error{Pos: x.Pos(), Msg: "resource collector requires a type reference"}
	}
	capType := capitalizeType(ref.Value)
	var realized []any

	if x.Exported {
		imported, err := e.importExported(capType, x.Query, s)
		if err != nil {
			return nil, err
		}
		realized = append(realized, imported...)
	}

	for _, res := range e.cat.Resources() {
		if res.Type != capType {
			continue
		}
		if x.Exported {
			if !res.Exported {
				continue
			}
		} else if !res.Virtual {
			continue
		}
		match, err := e.matchQuery(x.Query, res, s)
		if err != nil {
			return nil, err
		}
		if !match {
			continue
		}
		res.Virtual = false
		res.Exported = false
		e.cat.AddEdge(e.container(), res.Ref())
		realized = append(realized, &ResourceRef{Type: res.Type, Title: res.Title})
	}
	return realized, nil
}

// importExported pulls matching exported resources from the configured store
// into the catalog and returns references to them.
func (e *Evaluator) importExported(capType string, query ast.Node, s *Scope) ([]any, error) {
	if e.exported == nil {
		return nil, nil
	}
	res, err := e.exported.CollectExported(capType, e.nodeName)
	if err != nil {
		return nil, &Error{Msg: err.Error()}
	}
	var out []any
	for _, r := range res {
		match, err := e.matchQuery(query, r, s)
		if err != nil {
			return nil, err
		}
		if !match {
			continue
		}
		if _, dup := e.cat.Get(r.Ref()); dup {
			continue
		}
		clone := &catalog.Resource{
			Type:       r.Type,
			Title:      r.Title,
			Parameters: r.Parameters,
			Tags:       r.Tags,
		}
		// The duplicate check above guarantees Add succeeds here.
		_ = e.cat.Add(clone)
		e.cat.AddEdge(e.container(), clone.Ref())
		out = append(out, &ResourceRef{Type: clone.Type, Title: clone.Title})
	}
	return out, nil
}

// matchQuery evaluates a collector query against a resource. A nil query matches
// everything. Supported forms: `attr == value`, `attr != value`, and boolean
// `and`/`or` combinations; `title` and `tag` are special attribute names.
func (e *Evaluator) matchQuery(query ast.Node, res *catalog.Resource, s *Scope) (bool, error) {
	if query == nil {
		return true, nil
	}
	bin, ok := query.(*ast.Binary)
	if !ok {
		return false, &Error{Pos: query.Pos(), Msg: "unsupported collector query expression"}
	}
	switch bin.Op {
	case "and":
		l, err := e.matchQuery(bin.Left, res, s)
		if err != nil || !l {
			return false, err
		}
		return e.matchQuery(bin.Right, res, s)
	case "or":
		l, err := e.matchQuery(bin.Left, res, s)
		if err != nil || l {
			return l, err
		}
		return e.matchQuery(bin.Right, res, s)
	case "==", "!=":
		return e.matchQueryComparison(bin, res, s)
	}
	return false, &Error{Pos: query.Pos(), Msg: "unsupported collector query operator " + bin.Op}
}

func (e *Evaluator) matchQueryComparison(bin *ast.Binary, res *catalog.Resource, s *Scope) (bool, error) {
	field, ok := bin.Left.(*ast.QualifiedName)
	if !ok {
		return false, &Error{Pos: bin.Pos(), Msg: "collector query attribute must be a bare name"}
	}
	want, err := e.eval(bin.Right, s)
	if err != nil {
		return false, err
	}
	eq := e.queryFieldEquals(field.Value, want, res)
	if bin.Op == "!=" {
		return !eq, nil
	}
	return eq, nil
}

func (e *Evaluator) queryFieldEquals(field string, want Value, res *catalog.Resource) bool {
	switch field {
	case "title":
		return stringify(want) == res.Title
	case "tag":
		for _, t := range res.Tags {
			if t == stringify(want) {
				return true
			}
		}
		return false
	default:
		v, ok := res.Parameters[field]
		if !ok {
			return false
		}
		return equals(v, want)
	}
}
