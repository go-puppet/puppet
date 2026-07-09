// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"

	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/ast"
	"github.com/go-puppet/puppet/catalog"
)

// ResourceRef is a Puppet resource reference value, e.g. `File['/tmp/x']`.
type ResourceRef struct {
	Type  string
	Title string
}

// String renders the canonical `Type[Title]` form.
func (r *ResourceRef) String() string { return r.Type + "[" + r.Title + "]" }

// register records a definition so forward references resolve during
// evaluation. Non-definitions are ignored.
func (e *Evaluator) register(n ast.Node) {
	switch x := n.(type) {
	case *ast.ClassDefinition:
		e.classes[x.Name] = x
	case *ast.DefineDefinition:
		e.defines[x.Name] = x
	case *ast.FunctionDefinition:
		e.userFuncs[x.Name] = x
	case *ast.NodeDefinition:
		e.nodes = append(e.nodes, x)
	}
}

func isDefinition(n ast.Node) bool {
	switch n.(type) {
	case *ast.ClassDefinition, *ast.DefineDefinition, *ast.FunctionDefinition, *ast.NodeDefinition:
		return true
	}
	return false
}

// container returns the current containment reference (default the main class).
func (e *Evaluator) container() string {
	if e.curContainer == "" {
		return "Class[main]"
	}
	return e.curContainer
}

func (e *Evaluator) evalResource(x *ast.Resource, s *Scope) (Value, error) {
	typeName := ""
	if qn, ok := x.Type.(*ast.QualifiedName); ok {
		typeName = qn.Value
	} else {
		return nil, &Error{Pos: x.Pos(), Msg: "resource type must be a name"}
	}
	if typeName == "class" {
		return e.declareClassResource(x, s)
	}
	var refs []any
	for _, body := range x.Bodies {
		titleVal, err := e.eval(body.Title, s)
		if err != nil {
			return nil, err
		}
		params, err := e.evalAttributeOps(body.Ops, s)
		if err != nil {
			return nil, err
		}
		for _, title := range titlesOf(titleVal) {
			ref, err := e.declareResource(typeName, title, params, x.Form, s, x.Pos())
			if err != nil {
				return nil, err
			}
			refs = append(refs, ref)
		}
	}
	if len(refs) == 1 {
		return refs[0], nil
	}
	return refs, nil
}

// titlesOf expands a resource title value into individual string titles.
func titlesOf(v Value) []string {
	if arr, ok := normalize(v).([]any); ok {
		out := make([]string, len(arr))
		for i, t := range arr {
			out[i] = stringify(t)
		}
		return out
	}
	return []string{stringify(v)}
}

func (e *Evaluator) evalAttributeOps(ops []ast.AttributeOp, s *Scope) (map[string]any, error) {
	params := map[string]any{}
	for _, op := range ops {
		v, err := e.eval(op.Value, s)
		if err != nil {
			return nil, err
		}
		if op.Splat {
			h, ok := normalize(v).(map[string]any)
			if !ok {
				return nil, &Error{Pos: op.Value.Pos(), Msg: "* => requires a hash"}
			}
			for k, hv := range h {
				params[k] = hv
			}
			continue
		}
		params[op.Name] = v
	}
	return params, nil
}

// declareResource adds a resource to the catalog (or instantiates a defined
// type), wiring containment and metaparameter edges, and returns its reference.
func (e *Evaluator) declareResource(typeName, title string, params map[string]any, form ast.ResourceForm, s *Scope, pos ast.Position) (*ResourceRef, error) {
	params = applyDefaults(params, s.lookupDefaults(capitalizeType(typeName)))
	if def, ok := e.defines[typeName]; ok {
		return e.instantiateDefine(def, title, params, form, pos)
	}
	capType := capitalizeType(typeName)
	ref := &ResourceRef{Type: capType, Title: title}
	res := &catalog.Resource{
		Type:       capType,
		Title:      title,
		Parameters: map[string]any{},
		Virtual:    form == ast.Virtual,
		Exported:   form == ast.Exported,
	}
	e.assignParams(res, ref.String(), params)
	if err := e.cat.Add(res); err != nil {
		return nil, &Error{Pos: pos, Msg: err.Error()}
	}
	e.cat.AddEdge(e.container(), ref.String())
	if err := e.storeExported(res); err != nil {
		return nil, &Error{Pos: pos, Msg: err.Error()}
	}
	return ref, nil
}

// storeExported records an exported resource in the configured store, if any.
func (e *Evaluator) storeExported(res *catalog.Resource) error {
	if !res.Exported || e.exported == nil {
		return nil
	}
	return e.exported.StoreExported(e.nodeName, res)
}

// metaparams are the relationship metaparameters handled as edges rather than
// stored parameters.
var metaparams = map[string]string{
	"before":    "forward",
	"notify":    "forward",
	"require":   "reverse",
	"subscribe": "reverse",
}

// assignParams populates a resource's parameters, converting relationship
// metaparameters into catalog edges.
func (e *Evaluator) assignParams(res *catalog.Resource, self string, params map[string]any) {
	for k, v := range params {
		if dir, ok := metaparams[k]; ok {
			for _, other := range collectRefs(v) {
				if dir == "forward" {
					e.cat.AddEdge(self, other)
				} else {
					e.cat.AddEdge(other, self)
				}
			}
			continue
		}
		if k == "tag" {
			res.Tags = append(res.Tags, tagStrings(v)...)
			continue
		}
		res.Parameters[k] = v
	}
}

func tagStrings(v Value) []string {
	if arr, ok := normalize(v).([]any); ok {
		out := make([]string, len(arr))
		for i, t := range arr {
			out[i] = stringify(t)
		}
		return out
	}
	return []string{stringify(v)}
}

func (e *Evaluator) instantiateDefine(def *ast.DefineDefinition, title string, params map[string]any, form ast.ResourceForm, pos ast.Position) (*ResourceRef, error) {
	capType := capitalizeType(def.Name)
	ref := &ResourceRef{Type: capType, Title: title}
	res := &catalog.Resource{
		Type:       capType,
		Title:      title,
		Parameters: map[string]any{},
		Virtual:    form == ast.Virtual,
		Exported:   form == ast.Exported,
	}
	if err := e.cat.Add(res); err != nil {
		return nil, &Error{Pos: pos, Msg: err.Error()}
	}
	e.cat.AddEdge(e.container(), ref.String())

	ds := newScope(e.top)
	ds.setForce("title", title)
	ds.setForce("name", title)
	if err := e.bindNamedParams(ds, def.Params, params, def.Name); err != nil {
		return nil, positioned(err, pos)
	}
	prev := e.curContainer
	e.curContainer = ref.String()
	_, err := e.evalBody(def.Body, ds)
	e.curContainer = prev
	if err != nil {
		return nil, err
	}
	return ref, nil
}

// bindNamedParams binds class/define parameters from a name→value map, applying
// Hiera automatic data binding, defaults, and type checks.
func (e *Evaluator) bindNamedParams(s *Scope, params []ast.Parameter, provided map[string]any, ns string) error {
	for _, p := range params {
		var val Value
		switch {
		case provided != nil && hasKey(provided, p.Name):
			val = provided[p.Name]
		case e.hieraParam(ns, p.Name, &val):
			// bound from hiera
		case p.Default != nil:
			v, err := e.eval(p.Default, s)
			if err != nil {
				return err
			}
			val = v
		default:
			return &Error{Msg: "missing value for parameter $" + p.Name + " of " + ns}
		}
		if err := e.checkParamType(p, val); err != nil {
			return err
		}
		s.setForce(p.Name, val)
	}
	return nil
}

func hasKey(m map[string]any, k string) bool { _, ok := m[k]; return ok }

// hieraParam attempts an automatic data binding of `ns::name`, storing the
// result in *out and reporting whether it was found.
func (e *Evaluator) hieraParam(ns, name string, out *Value) bool {
	if e.hiera == nil {
		return false
	}
	v, ok, err := e.hiera.Lookup(ns+"::"+name, nil)
	if err != nil || !ok {
		return false
	}
	*out = toValue(v)
	return true
}

// --- classes --------------------------------------------------------------

func (e *Evaluator) declareClassResource(x *ast.Resource, s *Scope) (Value, error) {
	for _, body := range x.Bodies {
		titleVal, err := e.eval(body.Title, s)
		if err != nil {
			return nil, err
		}
		params, err := e.evalAttributeOps(body.Ops, s)
		if err != nil {
			return nil, err
		}
		if err := e.declareClass(stringify(titleVal), params, x.Pos()); err != nil {
			return nil, err
		}
	}
	return pcore.Undef, nil
}

// declareClass realizes a class into the catalog exactly once.
func (e *Evaluator) declareClass(name string, provided map[string]any, pos ast.Position) error {
	if e.included[name] {
		return nil
	}
	def, ok := e.classes[name]
	if !ok {
		return &Error{Pos: pos, Msg: "cannot find class " + name}
	}
	e.included[name] = true

	if def.Parent != "" {
		if err := e.declareClass(def.Parent, nil, pos); err != nil {
			return err
		}
	}
	capName := capitalizeType(name)
	ref := &ResourceRef{Type: "Class", Title: capName}
	res := &catalog.Resource{Type: "Class", Title: capName, Parameters: map[string]any{}}
	if err := e.cat.Add(res); err != nil {
		return &Error{Pos: pos, Msg: err.Error()}
	}
	e.cat.AddEdge(e.container(), ref.String())

	cs := newScope(e.top)
	cs.setForce("title", name)
	cs.setForce("name", name)
	if err := e.bindNamedParams(cs, def.Params, provided, name); err != nil {
		return positioned(err, pos)
	}
	prev := e.curContainer
	e.curContainer = ref.String()
	_, err := e.evalBody(def.Body, cs)
	e.curContainer = prev
	return err
}

// --- relationships --------------------------------------------------------

func (e *Evaluator) evalRelationship(x *ast.Relationship, s *Scope) (Value, error) {
	l, err := e.eval(x.Left, s)
	if err != nil {
		return nil, err
	}
	r, err := e.eval(x.Right, s)
	if err != nil {
		return nil, err
	}
	ls := collectRefs(l)
	rs := collectRefs(r)
	if len(ls) == 0 || len(rs) == 0 {
		return nil, &Error{Pos: x.Pos(), Msg: "relationship operands must be resource references"}
	}
	for _, a := range ls {
		for _, b := range rs {
			switch x.Op {
			case "->", "~>":
				e.cat.AddEdge(a, b)
			default: // "<-", "<~"
				e.cat.AddEdge(b, a)
			}
		}
	}
	return r, nil
}

// resourceRefFrom builds a resource reference (or array of them) from an
// `Type[title, ...]` access whose base is a plain capitalized name. It reports
// ok=false when the operand is not a bare [ast.QualifiedReference].
func (e *Evaluator) resourceRefFrom(x *ast.Access, s *Scope) (Value, bool, error) {
	base, ok := x.Operand.(*ast.QualifiedReference)
	if !ok {
		return nil, false, nil
	}
	refs := make([]any, len(x.Keys))
	for i, k := range x.Keys {
		title, err := e.eval(k, s)
		if err != nil {
			return nil, true, err
		}
		refs[i] = &ResourceRef{Type: base.Value, Title: stringify(title)}
	}
	if len(refs) == 1 {
		return refs[0], true, nil
	}
	return refs, true, nil
}

// collectRefs flattens a value into resource-reference strings.
func collectRefs(v Value) []string {
	switch x := normalize(v).(type) {
	case *ResourceRef:
		return []string{x.String()}
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, collectRefs(e)...)
		}
		return out
	}
	return nil
}

// --- node matching --------------------------------------------------------

func (e *Evaluator) applyMatchingNode() error {
	if len(e.nodes) == 0 {
		return nil
	}
	var dflt *ast.NodeDefinition
	for _, nd := range e.nodes {
		matched, isDefault, err := e.nodeMatches(nd)
		if err != nil {
			return err
		}
		if isDefault {
			dflt = nd
			continue
		}
		if matched {
			return e.evalNodeBody(nd)
		}
	}
	if dflt != nil {
		return e.evalNodeBody(dflt)
	}
	return &Error{Msg: "no node definition matches " + e.nodeName}
}

func (e *Evaluator) nodeMatches(nd *ast.NodeDefinition) (matched, isDefault bool, err error) {
	for _, m := range nd.Matches {
		if _, ok := m.(*ast.Default); ok {
			isDefault = true
			continue
		}
		v, err := e.eval(m, e.top)
		if err != nil {
			return false, false, err
		}
		switch mv := v.(type) {
		case *pcore.Regexp:
			if mv.MatchString(e.nodeName) {
				return true, false, nil
			}
		default:
			if stringify(mv) == e.nodeName {
				return true, false, nil
			}
		}
	}
	return false, isDefault, nil
}

func (e *Evaluator) evalNodeBody(nd *ast.NodeDefinition) error {
	_, err := e.evalBody(nd.Body, newScope(e.top))
	return err
}

// capitalizeType capitalizes each `::` segment of a resource/class type name,
// matching Puppet's reference form (`nagios_service` -> `Nagios_service`,
// `foo::bar` -> `Foo::Bar`).
func capitalizeType(name string) string {
	segs := strings.Split(name, "::")
	for i, seg := range segs {
		if seg == "" {
			continue
		}
		r := []rune(seg)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		segs[i] = string(r)
	}
	return strings.Join(segs, "::")
}
