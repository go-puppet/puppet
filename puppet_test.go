// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package puppet

import (
	"testing"

	"github.com/go-puppet/puppet/ast"
)

func TestParse(t *testing.T) {
	prog, err := Parse(`$x = 1 + 2`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if got := ast.Sexpr(prog); got != `(program [(= $x (+ 1 2))])` {
		t.Errorf("got %s", got)
	}
	if _, err := Parse(`$x = `); err == nil {
		t.Error("expected error")
	}
}

func TestParseExpression(t *testing.T) {
	n, err := ParseExpression(`[1, 2, 3]`)
	if err != nil {
		t.Fatalf("ParseExpression error: %v", err)
	}
	if got := ast.Sexpr(n); got != `(array 1 2 3)` {
		t.Errorf("got %s", got)
	}
	if _, err := ParseExpression(`)`); err == nil {
		t.Error("expected error")
	}
}
