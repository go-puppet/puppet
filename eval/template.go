// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"github.com/go-puppet/puppet/ast"
)

// ERBRenderer renders an ERB template (the legacy Ruby-based template format
// used by template()/inline_template()). ERB templates may contain arbitrary
// Ruby, which a pure-Go evaluator cannot execute; a host that embeds a Ruby
// runtime (e.g. go-ruby-puppet/rbgo, which can reuse github.com/go-ruby-erb to
// compile the template to Ruby and then run it) wires an implementation via
// [WithERBRenderer].
//
// src is the raw template text. vars is the set of top-scope variables and
// facts the template may reference (as @instance variables / scope lookups in
// Ruby ERB). The result is the rendered string.
type ERBRenderer interface {
	RenderERB(src string, vars map[string]any) (string, error)
}

// WithERBRenderer wires an ERB renderer for template()/inline_template().
func WithERBRenderer(r ERBRenderer) Option {
	return func(e *Evaluator) { e.erb = r }
}

// registerTemplateFns installs epp(), inline_epp(), template() and
// inline_template().
func registerTemplateFns(e *Evaluator) {
	e.funcs["inline_epp"] = builtinInlineEpp
	e.funcs["epp"] = builtinEpp
	e.funcs["inline_template"] = builtinInlineTemplate
	e.funcs["template"] = builtinTemplate
}

// templateParams extracts the optional trailing parameter hash of an
// epp()/inline_epp() call.
func templateParams(args []Value, idx int) (map[string]any, error) {
	if len(args) <= idx {
		return nil, nil
	}
	h, ok := normalize(args[idx]).(map[string]any)
	if !ok {
		return nil, &Error{Msg: "template parameters must be a Hash"}
	}
	return h, nil
}

func builtinInlineEpp(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, &Error{Msg: "inline_epp() expects a template String and an optional parameters Hash"}
	}
	tmpl, ok := normalize(args[0]).(string)
	if !ok {
		return nil, &Error{Msg: "inline_epp() first argument must be a String"}
	}
	params, err := templateParams(args, 1)
	if err != nil {
		return nil, err
	}
	return c.e.renderEPP(tmpl, params, pcorePos())
}

func builtinEpp(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, &Error{Msg: "epp() expects a template name and an optional parameters Hash"}
	}
	name, ok := normalize(args[0]).(string)
	if !ok {
		return nil, &Error{Msg: "epp() first argument must be a String"}
	}
	src, err := c.e.loadTemplate(name)
	if err != nil {
		return nil, err
	}
	params, err := templateParams(args, 1)
	if err != nil {
		return nil, err
	}
	return c.e.renderEPP(src, params, pcorePos())
}

func builtinInlineTemplate(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, &Error{Msg: "inline_template() expects at least one template String"}
	}
	return c.e.renderERBAll(args)
}

func builtinTemplate(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, &Error{Msg: "template() expects at least one template name"}
	}
	loaded := make([]Value, len(args))
	for i, a := range args {
		name, ok := normalize(a).(string)
		if !ok {
			return nil, &Error{Msg: "template() arguments must be Strings"}
		}
		src, err := c.e.loadTemplate(name)
		if err != nil {
			return nil, err
		}
		loaded[i] = src
	}
	return c.e.renderERBAll(loaded)
}

// renderERBAll renders and concatenates one or more ERB template sources
// through the configured [ERBRenderer].
func (e *Evaluator) renderERBAll(srcs []Value) (Value, error) {
	if e.erb == nil {
		return nil, &Error{Msg: "ERB templates require an ERB renderer; wire one with eval.WithERBRenderer " +
			"(a pure-Go Puppet evaluator cannot execute the Ruby in an ERB template). EPP templates (epp/inline_epp) are supported natively."}
	}
	vars := e.templateVars()
	var out string
	for _, s := range srcs {
		src, ok := normalize(s).(string)
		if !ok {
			return nil, &Error{Msg: "template source must be a String"}
		}
		r, err := e.erb.RenderERB(src, vars)
		if err != nil {
			return nil, &Error{Msg: err.Error()}
		}
		out += r
	}
	return out, nil
}

// templateVars snapshots the top-scope variables (globals and facts) that a
// template may reference.
func (e *Evaluator) templateVars() map[string]any {
	vars := map[string]any{}
	for k, v := range e.top.vars {
		vars[k] = v
	}
	return vars
}

// loadTemplate resolves a template name through the configured loader.
func (e *Evaluator) loadTemplate(name string) (string, error) {
	if e.templates == nil {
		return "", &Error{Msg: "cannot load template " + name + ": no template loader configured (use eval.WithTemplateLoader)"}
	}
	src, err := e.templates.Load(name)
	if err != nil {
		return "", &Error{Msg: err.Error()}
	}
	return src, nil
}

// pcorePos is a placeholder position for errors raised inside builtins, where no
// AST node is available; the dispatcher re-stamps the call site.
func pcorePos() ast.Position { return ast.Position{} }
