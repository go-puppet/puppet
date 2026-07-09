// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

func TestFinalEdges(t *testing.T) {
	// dirname of root
	if got := evalOut(t, `dirname("/")`); got != "/" {
		t.Errorf("dirname(/) got %q", got)
	}
	// qualified variable lookup that misses the top scope
	if got := evalOut(t, `"[${nonexistent::qualified}]"`); got != "[]" {
		t.Errorf("qualified miss got %q", got)
	}
	// a top-scoped Class reference decapitalises its leading empty segment
	if got := firstLog(t, `notice(defined(Class['::nope']))`); got != "false" {
		t.Errorf("defined ::class got %q", got)
	}
}

func TestFinalErrors(t *testing.T) {
	loader := mapLoader{"t.epp": "x"}
	cases := []struct {
		src  string
		opts []Option
		want string
	}{
		{`notice(tree_each({"a"=>{"b"=>1}}) |$v| { nope() })`, nil, "unknown function"},
		{`notice(clamp(5))`, nil, "must be an Array"},
		{`notice(to_bytes([]))`, nil, "must be a String"},
		{`notice(any2bool(1,2))`, nil, "wrong number"},
		{`create_resources('class', {'nosuchclass' => {}})`, nil, "cannot find class"},
		{`file { '/d': }
create_resources('file', {'/d' => {}})`, nil, "duplicate resource"},
		{`define needs(String $x) { }
ensure_resource('needs', 'inst')`, nil, "missing value for parameter"},
		{`notice(epp('t.epp', 5))`, []Option{WithTemplateLoader(loader)}, "must be a Hash"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src, tc.opts...); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}
