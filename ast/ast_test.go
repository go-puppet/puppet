// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package ast

import "testing"

// unknownNode is an out-of-model node used to exercise the renderer's default
// arm.
type unknownNode struct{ Base }

func TestPositionString(t *testing.T) {
	p := Position{Offset: 5, Line: 3, Column: 7}
	if p.String() != "3:7" {
		t.Errorf("Position.String() = %q", p.String())
	}
}

func TestBaseMarkers(t *testing.T) {
	b := Base{P: Position{Line: 2, Column: 4}}
	if b.Pos().Line != 2 || b.Pos().Column != 4 {
		t.Errorf("Pos() = %+v", b.Pos())
	}
	b.node() // exercise the marker method
}

func TestSexpr(t *testing.T) {
	str := &String{Value: "s"}
	v := &Variable{Name: "x"}
	one := &Integer{Value: 1}
	cases := []struct {
		node Node
		want string
	}{
		{nil, "nil"},
		{&unknownNode{}, "(?*ast.unknownNode)"},
		{&Program{Body: []Node{one}}, "(program [1])"},
		{&Undef{}, "undef"},
		{&Default{}, "default"},
		{&Boolean{Value: true}, "true"},
		{&Boolean{Value: false}, "false"},
		{&Integer{Value: 10}, "10"},
		{&Integer{Value: 255, Radix: 16}, "0xff"},
		{&Integer{Value: 493, Radix: 8}, "0755"},
		{&Float{Value: 1.5}, "1.5"},
		{str, `"s"`},
		{&Regexp{Value: "a.b"}, "/a.b/"},
		{&Concat{Parts: []Node{str, v}}, `(concat "s" $x)`},
		{&Heredoc{Syntax: "json", Text: str}, `(heredoc "json" "s")`},
		{&QualifiedName{Value: "file"}, "file"},
		{&QualifiedReference{Value: "File"}, "File"},
		{v, "$x"},
		{&Array{Elements: []Node{one, str}}, `(array 1 "s")`},
		{&Hash{Entries: []KeyedEntry{{Key: str, Value: one}}}, `(hash ("s" => 1))`},
		{&Access{Operand: v, Keys: []Node{one}}, "(access $x 1)"},
		{&Unary{Op: "-", Operand: one}, "(- 1)"},
		{&Binary{Op: "+", Left: one, Right: one}, "(+ 1 1)"},
		{&Assignment{Op: "=", Target: v, Value: one}, "(= $x 1)"},
		{&Selector{Operand: v, Entries: []SelectorEntry{{Match: str, Value: one}}}, `(? $x ("s" => 1))`},
		{&If{Cond: v, Then: []Node{one}}, "(if $x [1])"},
		{&If{Cond: v, Then: []Node{one}, Else: []Node{str}}, `(if $x [1] else ["s"])`},
		{&Unless{Cond: v, Then: []Node{one}}, "(unless $x [1])"},
		{&Unless{Cond: v, Then: []Node{one}, Else: []Node{str}}, `(unless $x [1] else ["s"])`},
		{&Case{Test: v, Options: []CaseOption{{Values: []Node{one}, Body: []Node{str}}}}, `(case $x (when 1 : ["s"]))`},
		{&Call{Functor: &QualifiedName{Value: "f"}, Args: []Node{one}}, "(call f 1)"},
		{&Call{Functor: &QualifiedName{Value: "f"}, Lambda: &Lambda{Body: []Node{one}}}, "(call f (lambda [1]))"},
		{&MethodCall{Receiver: v, Method: "m", Args: []Node{one}}, "(. $x m 1)"},
		{&Lambda{Params: []Parameter{{Name: "a"}}, Body: []Node{one}}, "(lambda (params ($a)) [1])"},
		{&Lambda{Params: []Parameter{{Name: "r", CapturesRest: true, Type: &QualifiedReference{Value: "Any"}, Default: one}}, Body: nil}, "(lambda (params (*Any $r = 1)) [])"},
		{&Resource{Type: &QualifiedName{Value: "file"}, Bodies: []ResourceBody{{Title: str, Ops: []AttributeOp{{Name: "ensure", Op: "=>", Value: one}}}}}, `(resource file (body "s" (ensure => 1)))`},
		{&Resource{Type: &QualifiedName{Value: "file"}, Form: Virtual, Bodies: []ResourceBody{{Title: str}}}, `(resource@ file (body "s"))`},
		{&Resource{Type: &QualifiedName{Value: "file"}, Form: Exported, Bodies: []ResourceBody{{Title: str}}}, `(resource@@ file (body "s"))`},
		{&ResourceDefaults{Type: &QualifiedReference{Value: "File"}, Ops: []AttributeOp{{Splat: true, Op: "=>", Value: v}}}, "(defaults File (* => $x))"},
		{&ResourceOverride{Resource: &Access{Operand: &QualifiedReference{Value: "File"}, Keys: []Node{str}}, Ops: []AttributeOp{{Name: "mode", Op: "+>", Value: one}}}, `(override (access File "s") (mode +> 1))`},
		{&Collector{Type: &QualifiedReference{Value: "File"}}, "(collect File)"},
		{&Collector{Type: &QualifiedReference{Value: "File"}, Query: v}, "(collect File $x)"},
		{&Collector{Type: &QualifiedReference{Value: "F"}, Exported: true}, "(collect-exported F)"},
		{&ClassDefinition{Name: "c", Body: []Node{one}}, "(class c [1])"},
		{&ClassDefinition{Name: "c", Params: []Parameter{{Name: "a"}}, Parent: "base", Body: nil}, "(class c (params ($a)) inherits base [])"},
		{&DefineDefinition{Name: "d", Body: nil}, "(define d [])"},
		{&NodeDefinition{Matches: []Node{str}, Body: nil}, `(node "s" [])`},
		{&FunctionDefinition{Name: "f", Body: []Node{one}}, "(function f [1])"},
		{&FunctionDefinition{Name: "f", ReturnType: &QualifiedReference{Value: "Integer"}, Body: nil}, "(function f >> Integer [])"},
		{&Relationship{Op: "->", Left: v, Right: str}, `(-> $x "s")`},
	}
	for _, tc := range cases {
		if got := Sexpr(tc.node); got != tc.want {
			t.Errorf("Sexpr\n  got  %s\n  want %s", got, tc.want)
		}
	}
}
