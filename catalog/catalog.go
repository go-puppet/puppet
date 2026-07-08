// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package catalog is the compiled-catalog model produced by the evaluator: a
// graph of resources with containment and relationship edges, serializable to
// the Puppet catalog JSON shape.
package catalog

import (
	"sort"
	"strconv"
	"strings"
)

// Resource is one catalog resource: a typed, titled bag of parameters plus its
// tags and virtual/exported flags.
type Resource struct {
	Type       string
	Title      string
	Parameters map[string]any
	Tags       []string
	Exported   bool
	Virtual    bool
}

// Ref returns the canonical Puppet reference `Type[Title]`.
func (r *Resource) Ref() string { return r.Type + "[" + r.Title + "]" }

// Edge is a directed relationship between two resource references.
type Edge struct {
	Source string
	Target string
}

// Catalog is a compiled catalog for one node.
type Catalog struct {
	Name        string
	Environment string
	Version     int64
	resources   []*Resource
	index       map[string]*Resource
	edges       []Edge
	edgeSet     map[Edge]bool
}

// New creates an empty catalog for the named node.
func New(name string) *Catalog {
	return &Catalog{
		Name:        name,
		Environment: "production",
		index:       map[string]*Resource{},
		edgeSet:     map[Edge]bool{},
	}
}

// Add inserts r, returning an error if a resource with the same reference is
// already present (Puppet forbids duplicate resource declarations).
func (c *Catalog) Add(r *Resource) error {
	ref := r.Ref()
	if _, dup := c.index[ref]; dup {
		return &DuplicateError{Ref: ref}
	}
	c.index[ref] = r
	c.resources = append(c.resources, r)
	return nil
}

// Get returns the resource for ref (`Type[Title]`) and whether it exists.
func (c *Catalog) Get(ref string) (*Resource, bool) {
	r, ok := c.index[ref]
	return r, ok
}

// Resources returns the resources in declaration order.
func (c *Catalog) Resources() []*Resource { return c.resources }

// Edges returns the edges in insertion order.
func (c *Catalog) Edges() []Edge { return c.edges }

// AddEdge records a directed edge source -> target, de-duplicating.
func (c *Catalog) AddEdge(source, target string) {
	e := Edge{Source: source, Target: target}
	if c.edgeSet[e] {
		return
	}
	c.edgeSet[e] = true
	c.edges = append(c.edges, e)
}

// DuplicateError reports a duplicate resource declaration.
type DuplicateError struct{ Ref string }

// Error implements error.
func (e *DuplicateError) Error() string {
	return "duplicate resource declaration: " + e.Ref
}

// JSON renders the catalog in the Puppet catalog JSON shape. It is written by
// hand (no reflection) so the field order is stable and deterministic.
func (c *Catalog) JSON() string {
	var b strings.Builder
	b.WriteByte('{')
	writeField(&b, "name", quote(c.Name))
	b.WriteByte(',')
	writeField(&b, "version", strconv.FormatInt(c.Version, 10))
	b.WriteByte(',')
	writeField(&b, "environment", quote(c.Environment))
	b.WriteByte(',')
	b.WriteString(`"resources":[`)
	for i, r := range c.resources {
		if i > 0 {
			b.WriteByte(',')
		}
		writeResource(&b, r)
	}
	b.WriteString(`],`)
	b.WriteString(`"edges":[`)
	for i, e := range c.edges {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('{')
		writeField(&b, "source", quote(e.Source))
		b.WriteByte(',')
		writeField(&b, "target", quote(e.Target))
		b.WriteByte('}')
	}
	b.WriteString("]}")
	return b.String()
}

func writeResource(b *strings.Builder, r *Resource) {
	b.WriteByte('{')
	writeField(b, "type", quote(r.Type))
	b.WriteByte(',')
	writeField(b, "title", quote(r.Title))
	b.WriteByte(',')
	writeField(b, "exported", strconv.FormatBool(r.Exported))
	b.WriteByte(',')
	b.WriteString(`"tags":[`)
	for i, t := range r.Tags {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(quote(t))
	}
	b.WriteString(`],`)
	b.WriteString(`"parameters":{`)
	keys := make([]string, 0, len(r.Parameters))
	for k := range r.Parameters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		writeField(b, k, jsonValue(r.Parameters[k]))
	}
	b.WriteString("}}")
}

func writeField(b *strings.Builder, key, rawValue string) {
	b.WriteString(quote(key))
	b.WriteByte(':')
	b.WriteString(rawValue)
}

// jsonValue renders a parameter value as JSON. It supports the scalar and
// collection shapes the evaluator produces; anything else falls back to its
// quoted Go string form.
func jsonValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case string:
		return quote(x)
	case []any:
		var b strings.Builder
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(jsonValue(e))
		}
		b.WriteByte(']')
		return b.String()
	case map[string]any:
		var b strings.Builder
		b.WriteByte('{')
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeField(&b, k, jsonValue(x[k]))
		}
		b.WriteByte('}')
		return b.String()
	default:
		return quote(stringify(x))
	}
}

func stringify(v any) string {
	if s, ok := v.(interface{ String() string }); ok {
		return s.String()
	}
	return ""
}

// quote renders s as a JSON string literal.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
