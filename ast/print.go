// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package ast

import (
	"fmt"
	"strconv"
	"strings"
)

// String renders a node as a compact, parenthesized s-expression. The form is
// stable and lossless enough to assert parser output against in tests; it is
// not Puppet source. A nil node renders as "nil".
func Sexpr(n Node) string {
	var b strings.Builder
	write(&b, n)
	return b.String()
}

func write(b *strings.Builder, n Node) {
	switch x := n.(type) {
	case nil:
		b.WriteString("nil")
	case *Program:
		b.WriteString("(program")
		writeBody(b, x.Body)
		b.WriteByte(')')
	case *Undef:
		b.WriteString("undef")
	case *Default:
		b.WriteString("default")
	case *Boolean:
		b.WriteString(strconv.FormatBool(x.Value))
	case *Integer:
		if x.Radix == 16 {
			fmt.Fprintf(b, "0x%x", x.Value)
		} else if x.Radix == 8 {
			fmt.Fprintf(b, "0%o", x.Value)
		} else {
			b.WriteString(strconv.FormatInt(x.Value, 10))
		}
	case *Float:
		b.WriteString(strconv.FormatFloat(x.Value, 'g', -1, 64))
	case *String:
		b.WriteString(strconv.Quote(x.Value))
	case *Regexp:
		b.WriteByte('/')
		b.WriteString(x.Value)
		b.WriteByte('/')
	case *Concat:
		b.WriteString("(concat")
		writeSeq(b, x.Parts)
		b.WriteByte(')')
	case *Heredoc:
		fmt.Fprintf(b, "(heredoc %q ", x.Syntax)
		write(b, x.Text)
		b.WriteByte(')')
	case *QualifiedName:
		b.WriteString(x.Value)
	case *QualifiedReference:
		b.WriteString(x.Value)
	case *Variable:
		b.WriteByte('$')
		b.WriteString(x.Name)
	case *Array:
		b.WriteString("(array")
		writeSeq(b, x.Elements)
		b.WriteByte(')')
	case *Hash:
		b.WriteString("(hash")
		for _, e := range x.Entries {
			b.WriteString(" (")
			write(b, e.Key)
			b.WriteString(" => ")
			write(b, e.Value)
			b.WriteByte(')')
		}
		b.WriteByte(')')
	case *Access:
		b.WriteString("(access ")
		write(b, x.Operand)
		writeSeq(b, x.Keys)
		b.WriteByte(')')
	case *Unary:
		fmt.Fprintf(b, "(%s ", x.Op)
		write(b, x.Operand)
		b.WriteByte(')')
	case *Binary:
		fmt.Fprintf(b, "(%s ", x.Op)
		write(b, x.Left)
		b.WriteByte(' ')
		write(b, x.Right)
		b.WriteByte(')')
	case *Assignment:
		fmt.Fprintf(b, "(%s ", x.Op)
		write(b, x.Target)
		b.WriteByte(' ')
		write(b, x.Value)
		b.WriteByte(')')
	case *Selector:
		b.WriteString("(? ")
		write(b, x.Operand)
		for _, e := range x.Entries {
			b.WriteString(" (")
			write(b, e.Match)
			b.WriteString(" => ")
			write(b, e.Value)
			b.WriteByte(')')
		}
		b.WriteByte(')')
	case *If:
		b.WriteString("(if ")
		write(b, x.Cond)
		writeBody(b, x.Then)
		if x.Else != nil {
			b.WriteString(" else")
			writeBody(b, x.Else)
		}
		b.WriteByte(')')
	case *Unless:
		b.WriteString("(unless ")
		write(b, x.Cond)
		writeBody(b, x.Then)
		if x.Else != nil {
			b.WriteString(" else")
			writeBody(b, x.Else)
		}
		b.WriteByte(')')
	case *Case:
		b.WriteString("(case ")
		write(b, x.Test)
		for _, o := range x.Options {
			b.WriteString(" (when")
			writeSeq(b, o.Values)
			b.WriteString(" :")
			writeBody(b, o.Body)
			b.WriteByte(')')
		}
		b.WriteByte(')')
	case *Call:
		b.WriteString("(call ")
		write(b, x.Functor)
		writeSeq(b, x.Args)
		writeLambda(b, x.Lambda)
		b.WriteByte(')')
	case *MethodCall:
		b.WriteString("(. ")
		write(b, x.Receiver)
		b.WriteByte(' ')
		b.WriteString(x.Method)
		writeSeq(b, x.Args)
		writeLambda(b, x.Lambda)
		b.WriteByte(')')
	case *Lambda:
		b.WriteString("(lambda")
		writeParams(b, x.Params)
		writeBody(b, x.Body)
		b.WriteByte(')')
	case *Resource:
		fmt.Fprintf(b, "(resource%s ", formTag(x.Form))
		write(b, x.Type)
		for _, body := range x.Bodies {
			b.WriteString(" (body ")
			write(b, body.Title)
			writeOps(b, body.Ops)
			b.WriteByte(')')
		}
		b.WriteByte(')')
	case *ResourceDefaults:
		b.WriteString("(defaults ")
		write(b, x.Type)
		writeOps(b, x.Ops)
		b.WriteByte(')')
	case *ResourceOverride:
		b.WriteString("(override ")
		write(b, x.Resource)
		writeOps(b, x.Ops)
		b.WriteByte(')')
	case *Collector:
		if x.Exported {
			b.WriteString("(collect-exported ")
		} else {
			b.WriteString("(collect ")
		}
		write(b, x.Type)
		if x.Query != nil {
			b.WriteByte(' ')
			write(b, x.Query)
		}
		b.WriteByte(')')
	case *ClassDefinition:
		fmt.Fprintf(b, "(class %s", x.Name)
		writeParams(b, x.Params)
		if x.Parent != "" {
			fmt.Fprintf(b, " inherits %s", x.Parent)
		}
		writeBody(b, x.Body)
		b.WriteByte(')')
	case *DefineDefinition:
		fmt.Fprintf(b, "(define %s", x.Name)
		writeParams(b, x.Params)
		writeBody(b, x.Body)
		b.WriteByte(')')
	case *NodeDefinition:
		b.WriteString("(node")
		writeSeq(b, x.Matches)
		writeBody(b, x.Body)
		b.WriteByte(')')
	case *FunctionDefinition:
		fmt.Fprintf(b, "(function %s", x.Name)
		writeParams(b, x.Params)
		if x.ReturnType != nil {
			b.WriteString(" >> ")
			write(b, x.ReturnType)
		}
		writeBody(b, x.Body)
		b.WriteByte(')')
	case *Relationship:
		fmt.Fprintf(b, "(%s ", x.Op)
		write(b, x.Left)
		b.WriteByte(' ')
		write(b, x.Right)
		b.WriteByte(')')
	default:
		fmt.Fprintf(b, "(?%T)", n)
	}
}

func writeSeq(b *strings.Builder, ns []Node) {
	for _, n := range ns {
		b.WriteByte(' ')
		write(b, n)
	}
}

func writeBody(b *strings.Builder, ns []Node) {
	b.WriteString(" [")
	for i, n := range ns {
		if i > 0 {
			b.WriteByte(' ')
		}
		write(b, n)
	}
	b.WriteByte(']')
}

func writeLambda(b *strings.Builder, l *Lambda) {
	if l == nil {
		return
	}
	b.WriteString(" (lambda")
	writeParams(b, l.Params)
	writeBody(b, l.Body)
	b.WriteByte(')')
}

func writeParams(b *strings.Builder, ps []Parameter) {
	if len(ps) == 0 {
		return
	}
	b.WriteString(" (params")
	for _, p := range ps {
		b.WriteString(" (")
		if p.CapturesRest {
			b.WriteByte('*')
		}
		if p.Type != nil {
			write(b, p.Type)
			b.WriteByte(' ')
		}
		b.WriteByte('$')
		b.WriteString(p.Name)
		if p.Default != nil {
			b.WriteString(" = ")
			write(b, p.Default)
		}
		b.WriteByte(')')
	}
	b.WriteByte(')')
}

func writeOps(b *strings.Builder, ops []AttributeOp) {
	for _, op := range ops {
		b.WriteString(" (")
		if op.Splat {
			b.WriteByte('*')
		} else {
			b.WriteString(op.Name)
		}
		b.WriteByte(' ')
		b.WriteString(op.Op)
		b.WriteByte(' ')
		write(b, op.Value)
		b.WriteByte(')')
	}
}

func formTag(f ResourceForm) string {
	switch f {
	case Virtual:
		return "@"
	case Exported:
		return "@@"
	default:
		return ""
	}
}
