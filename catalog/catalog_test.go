// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"strings"
	"testing"
)

func TestAddAndGet(t *testing.T) {
	c := New("node1")
	r := &Resource{Type: "File", Title: "/tmp/x", Parameters: map[string]any{}}
	if err := c.Add(r); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got, ok := c.Get("File[/tmp/x]"); !ok || got != r {
		t.Errorf("Get failed: %v %v", got, ok)
	}
	if _, ok := c.Get("File[nope]"); ok {
		t.Error("unexpected Get hit")
	}
	if len(c.Resources()) != 1 {
		t.Errorf("Resources len = %d", len(c.Resources()))
	}
	// duplicate
	err := c.Add(&Resource{Type: "File", Title: "/tmp/x"})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if de, ok := err.(*DuplicateError); !ok || de.Ref != "File[/tmp/x]" || !strings.Contains(de.Error(), "duplicate") {
		t.Errorf("bad dup error: %v", err)
	}
}

func TestEdges(t *testing.T) {
	c := New("n")
	c.AddEdge("A[a]", "B[b]")
	c.AddEdge("A[a]", "B[b]") // dedup
	c.AddEdge("B[b]", "C[c]")
	if len(c.Edges()) != 2 {
		t.Errorf("edges = %v", c.Edges())
	}
}

func TestJSON(t *testing.T) {
	c := New("web1")
	c.Version = 7
	c.Add(&Resource{
		Type:     "File",
		Title:    "/tmp/x",
		Exported: true,
		Tags:     []string{"a", "b"},
		Parameters: map[string]any{
			"ensure":  "present",
			"count":   int64(3),
			"ratio":   1.5,
			"on":      true,
			"empty":   nil,
			"list":    []any{int64(1), "two"},
			"nested":  map[string]any{"k": "v", "k2": "v2"},
			"n2":      2,
			"rune_ct": '\n',
		},
	})
	c.Add(&Resource{Type: "Service", Title: "nginx", Parameters: map[string]any{}})
	c.AddEdge("Class[main]", "File[/tmp/x]")
	c.AddEdge("File[/tmp/x]", "Service[nginx]")
	js := c.JSON()
	for _, want := range []string{
		`"name":"web1"`, `"version":7`, `"environment":"production"`,
		`"type":"File"`, `"title":"/tmp/x"`, `"exported":true`,
		`"tags":["a","b"]`, `"ensure":"present"`, `"count":3`, `"ratio":1.5`,
		`"on":true`, `"empty":null`, `"list":[1,"two"]`, `"nested":{"k":"v","k2":"v2"}`, `"n2":2`,
		`{"source":"Class[main]","target":"File[/tmp/x]"},{"source":"File[/tmp/x]","target":"Service[nginx]"}`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("JSON missing %s\n%s", want, js)
		}
	}
}

func TestJSONQuoteEscaping(t *testing.T) {
	c := New("n\"x")
	c.Add(&Resource{Type: "File", Title: "a\\b\nc\t\"d\"", Parameters: map[string]any{}})
	js := c.JSON()
	if !strings.Contains(js, `\"`) || !strings.Contains(js, `\\`) || !strings.Contains(js, `\n`) || !strings.Contains(js, `\t`) {
		t.Errorf("escaping wrong: %s", js)
	}
}

// stringerParam exercises the jsonValue default arm (a value with String()).
type stringerParam struct{}

func (stringerParam) String() string { return "S" }

func TestJSONDefaultAndCR(t *testing.T) {
	c := New("n")
	c.Add(&Resource{Type: "T", Title: "t", Parameters: map[string]any{"x": stringerParam{}, "cr": "a\rb"}})
	js := c.JSON()
	if !strings.Contains(js, `"x":"S"`) {
		t.Errorf("default jsonValue arm: %s", js)
	}
	if !strings.Contains(js, `\r`) {
		t.Errorf("carriage return escape: %s", js)
	}
	// stringify fallback for a non-Stringer default value.
	if got := jsonValue(struct{}{}); got != `""` {
		t.Errorf("non-stringer jsonValue = %s", got)
	}
}

func TestResourceRef(t *testing.T) {
	r := &Resource{Type: "File", Title: "/x"}
	if r.Ref() != "File[/x]" {
		t.Errorf("Ref = %s", r.Ref())
	}
}
