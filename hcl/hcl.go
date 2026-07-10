// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package hcl

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	hcl2 "github.com/go-ruby-hcl2/hcl2"

	"github.com/go-puppet/puppet/ast"
)

// Parse parses a Terraform-style HCL2 manifest and produces the same
// [ast.Program] the Puppet (.pp) parser produces, so the result compiles to an
// identical catalog through the existing evaluator. See the package doc for the
// full HCL2↔Puppet mapping.
//
// Top-level body entries are emitted in a deterministic order: root-level
// attributes first (each becomes a variable [ast.Assignment]), then blocks in
// source order. HCL2's own body model keeps attributes and blocks in separate
// lists, so this ordering — not raw interleaving — is the contract.
func Parse(src string) (*ast.Program, error) {
	body, err := hcl2.Parse(src)
	if err != nil {
		return nil, err
	}
	var out []ast.Node
	for _, a := range body.Attributes {
		asn, err := attrAssignment(a)
		if err != nil {
			return nil, err
		}
		out = append(out, asn)
	}
	for _, blk := range body.Blocks {
		nodes, err := convertBlock(blk)
		if err != nil {
			return nil, err
		}
		out = append(out, nodes...)
	}
	return &ast.Program{Body: out}, nil
}

// unsupported reports a construct that HCL2 v0.1 does not translate. It returns
// a clear error rather than a silent or fake stub.
func unsupported(kind string) error {
	return fmt.Errorf("unsupported in HCL2 v0.1: %s", kind)
}

// attrAssignment turns a `name = expr` body entry into a variable assignment,
// the Puppet analogue of an HCL local/root attribute.
func attrAssignment(a *hcl2.Attribute) (ast.Node, error) {
	v, err := mapExpr(a.Expression())
	if err != nil {
		return nil, err
	}
	return &ast.Assignment{Op: "=", Target: &ast.Variable{Name: a.Name}, Value: v}, nil
}

// convertBlock translates one top-level HCL2 block into zero or more Puppet
// nodes. Only `resource` and `locals` are recognised in v0.1.
func convertBlock(blk *hcl2.Block) ([]ast.Node, error) {
	switch blk.Type {
	case "locals":
		return convertLocals(blk)
	case "resource":
		return convertResource(blk)
	default:
		return nil, unsupported(fmt.Sprintf("block type %q", blk.Type))
	}
}

// convertLocals turns `locals { k = expr … }` into one assignment per attribute.
func convertLocals(blk *hcl2.Block) ([]ast.Node, error) {
	if len(blk.Labels) != 0 {
		return nil, unsupported("labels on a locals block")
	}
	if len(blk.Body.Blocks) != 0 {
		return nil, unsupported("nested block inside locals")
	}
	var out []ast.Node
	for _, a := range blk.Body.Attributes {
		asn, err := attrAssignment(a)
		if err != nil {
			return nil, err
		}
		out = append(out, asn)
	}
	return out, nil
}

// convertResource turns `resource "TYPE" "TITLE" { attr = expr … }` into a
// single regular Puppet resource declaration.
func convertResource(blk *hcl2.Block) ([]ast.Node, error) {
	if len(blk.Labels) != 2 {
		return nil, unsupported("resource block needs exactly TYPE and TITLE labels")
	}
	if len(blk.Body.Blocks) != 0 {
		return nil, unsupported("nested block inside a resource")
	}
	var ops []ast.AttributeOp
	for _, a := range blk.Body.Attributes {
		v, err := mapExpr(a.Expression())
		if err != nil {
			return nil, err
		}
		ops = append(ops, ast.AttributeOp{Name: a.Name, Op: "=>", Value: v})
	}
	res := &ast.Resource{
		Type: &ast.QualifiedName{Value: blk.Labels[0]},
		Bodies: []ast.ResourceBody{{
			Title: &ast.String{Value: blk.Labels[1]},
			Ops:   ops,
		}},
		Form: ast.Regular,
	}
	return []ast.Node{res}, nil
}

// mapExpr maps an HCL2 expression node to its Puppet AST equivalent. Supported
// nodes are handled explicitly; the remaining sealed cases (VarExpr, CallExpr,
// CondExpr, ForTupleExpr, ForObjectExpr) fall through to a clear v0.2 error.
func mapExpr(e hcl2.Expr) (ast.Node, error) {
	switch x := e.(type) {
	case hcl2.LiteralExpr:
		return mapLiteral(x.Value), nil
	case hcl2.TemplateExpr:
		return mapTemplate(x.Raw, x.Heredoc)
	case hcl2.AttrExpr:
		return mapAttr(x)
	case hcl2.IndexExpr:
		return mapIndex(x)
	case hcl2.UnaryExpr:
		op, err := mapExpr(x.Operand)
		if err != nil {
			return nil, err
		}
		return &ast.Unary{Op: x.Op, Operand: op}, nil
	case hcl2.BinaryExpr:
		l, err := mapExpr(x.Left)
		if err != nil {
			return nil, err
		}
		r, err := mapExpr(x.Right)
		if err != nil {
			return nil, err
		}
		return &ast.Binary{Op: mapBinOp(x.Op), Left: l, Right: r}, nil
	case hcl2.TupleExpr:
		els, err := mapExprs(x.Items)
		if err != nil {
			return nil, err
		}
		return &ast.Array{Elements: els}, nil
	case hcl2.ObjectExpr:
		return mapObject(x)
	default:
		return nil, unsupported(fmt.Sprintf("%T", e))
	}
}

// mapExprs maps a slice of HCL2 expressions, propagating the first error.
func mapExprs(items []hcl2.Expr) ([]ast.Node, error) {
	var out []ast.Node
	for _, it := range items {
		n, err := mapExpr(it)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// mapObject maps an HCL2 object literal to a Puppet hash.
func mapObject(x hcl2.ObjectExpr) (ast.Node, error) {
	var entries []ast.KeyedEntry
	for i := range x.Keys {
		k, err := mapExpr(x.Keys[i])
		if err != nil {
			return nil, err
		}
		v, err := mapExpr(x.Vals[i])
		if err != nil {
			return nil, err
		}
		entries = append(entries, ast.KeyedEntry{Key: k, Value: v})
	}
	return &ast.Hash{Entries: entries}, nil
}

// mapLiteral maps a final HCL2 scalar value (int64/float64/bool/string, or nil
// for null) to the matching Puppet literal.
func mapLiteral(v hcl2.Value) ast.Node {
	switch t := v.(type) {
	case int64:
		return &ast.Integer{Value: t, Radix: 10}
	case float64:
		return &ast.Float{Value: t}
	case bool:
		return &ast.Boolean{Value: t}
	case string:
		return &ast.String{Value: t}
	default:
		return &ast.Undef{}
	}
}

// mapBinOp maps an HCL2 binary operator to Puppet's spelling: `&&`→and,
// `||`→or, everything else passes through unchanged.
func mapBinOp(op string) string {
	switch op {
	case "&&":
		return "and"
	case "||":
		return "or"
	default:
		return op
	}
}

// mapAttr maps an `Obj.Name` access. `local.X` becomes a variable; a
// `resource.TYPE.TITLE` traversal becomes a Puppet resource reference. Any
// other root is a v0.2 error.
func mapAttr(x hcl2.AttrExpr) (ast.Node, error) {
	if v, ok := x.Obj.(hcl2.VarExpr); ok && v.Name == "local" {
		return &ast.Variable{Name: x.Name}, nil
	}
	if inner, ok := x.Obj.(hcl2.AttrExpr); ok {
		if v, ok := inner.Obj.(hcl2.VarExpr); ok && v.Name == "resource" {
			return resourceRef(inner.Name, &ast.String{Value: x.Name}), nil
		}
	}
	return nil, unsupported("traversal root (only local.X and resource.TYPE.TITLE)")
}

// mapIndex maps `Coll[Idx]`. A `resource.TYPE["title"]` traversal becomes a
// resource reference; anything else becomes a generic Puppet access.
func mapIndex(x hcl2.IndexExpr) (ast.Node, error) {
	if attr, ok := x.Coll.(hcl2.AttrExpr); ok {
		if v, ok := attr.Obj.(hcl2.VarExpr); ok && v.Name == "resource" {
			idx, err := mapExpr(x.Idx)
			if err != nil {
				return nil, err
			}
			return resourceRef(attr.Name, idx), nil
		}
	}
	coll, err := mapExpr(x.Coll)
	if err != nil {
		return nil, err
	}
	idx, err := mapExpr(x.Idx)
	if err != nil {
		return nil, err
	}
	return &ast.Access{Operand: coll, Keys: []ast.Node{idx}}, nil
}

// resourceRef builds `Type[key]`, the Puppet reference to a declared resource.
// The HCL2 lower-case type is capitalised into a Puppet type reference.
func resourceRef(typ string, key ast.Node) ast.Node {
	return &ast.Access{
		Operand: &ast.QualifiedReference{Value: capitalize(typ)},
		Keys:    []ast.Node{key},
	}
}

// capitalize upper-cases the first rune (HCL type name → Puppet reference).
func capitalize(s string) string {
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// mapTemplate maps a quoted-string or heredoc template. With no interpolation
// it yields a static [ast.String]; otherwise an [ast.Concat] whose parts are
// literal-text [ast.String]s interleaved with the mapped `${…}` expressions.
// Backslash escapes are expanded for quoted strings and kept literal for
// heredocs.
func mapTemplate(raw string, heredoc bool) (ast.Node, error) {
	parts, lit, hadInterp, err := templateParts(raw, heredoc)
	if err != nil {
		return nil, err
	}
	if !hadInterp {
		return &ast.String{Value: lit}, nil
	}
	return &ast.Concat{Parts: parts}, nil
}

// templateParts scans a template body into Concat parts. It returns the parts,
// the fully-accumulated literal text (valid only when hadInterp is false), and
// whether any `${…}` interpolation was seen. Empty literal runs between/around
// interpolations are dropped, matching the Puppet parser.
func templateParts(raw string, heredoc bool) (parts []ast.Node, whole string, hadInterp bool, err error) {
	var lit strings.Builder
	var all strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			parts = append(parts, &ast.String{Value: lit.String()})
			lit.Reset()
		}
	}
	write := func(s string) { lit.WriteString(s); all.WriteString(s) }

	i := 0
	for i < len(raw) {
		switch {
		case strings.HasPrefix(raw[i:], "$${"):
			write("${")
			i += 3
		case strings.HasPrefix(raw[i:], "%%{"):
			write("%{")
			i += 3
		case strings.HasPrefix(raw[i:], "${"):
			hadInterp = true
			flush()
			end, ferr := findClose(raw, i+2)
			if ferr != nil {
				return nil, "", false, ferr
			}
			inner := raw[i+2 : end]
			node, perr := parseEmbedded(inner)
			if perr != nil {
				return nil, "", false, perr
			}
			parts = append(parts, node)
			i = end + 1
		case strings.HasPrefix(raw[i:], "%{"):
			return nil, "", false, unsupported("template directive %{…}")
		case !heredoc && raw[i] == '\\':
			r, w, derr := decodeEscape(raw, i)
			if derr != nil {
				return nil, "", false, derr
			}
			write(string(r))
			i += w
		default:
			_, w := utf8.DecodeRuneInString(raw[i:])
			write(raw[i : i+w])
			i += w
		}
	}
	flush()
	return parts, all.String(), hadInterp, nil
}

// findClose returns the index of the `}` that closes an interpolation opened at
// start (the byte after `${`), matching nested braces and skipping quoted
// strings inside. It errors on an unterminated interpolation (HCL2 keeps
// template bodies raw, so an unbalanced `${` reaches us via heredocs).
func findClose(raw string, start int) (int, error) {
	depth := 1
	i := start
	for i < len(raw) {
		switch raw[i] {
		case '"':
			i++
			for i < len(raw) {
				if raw[i] == '\\' {
					i += 2
					continue
				}
				if raw[i] == '"' {
					i++
					break
				}
				i++
			}
		case '{':
			depth++
			i++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
			i++
		default:
			i++
		}
	}
	return 0, unsupported("unterminated ${…} interpolation")
}

// parseEmbedded parses the body of a `${…}` interpolation into a Puppet node by
// re-parsing it through the HCL2 expression parser. Terraform whitespace-strip
// markers (`~`) are rejected in v0.1.
func parseEmbedded(inner string) (ast.Node, error) {
	if strings.HasPrefix(inner, "~") || strings.HasSuffix(inner, "~") {
		return nil, unsupported("template strip marker ~")
	}
	body, err := hcl2.Parse("__ = " + inner + "\n")
	if err != nil {
		return nil, err
	}
	return mapExpr(body.Attributes[0].Expression())
}

// decodeEscape decodes a backslash escape at s[i] (s[i]=='\\'), returning the
// decoded rune and the number of bytes consumed. It supports \n \t \r \" \\ and
// \uXXXX / \UXXXXXXXX unicode escapes. HCL2's tokenizer rejects an unterminated
// quoted string, so a quoted template body never ends in a lone backslash and
// s[i+1] is always in range here.
func decodeEscape(s string, i int) (rune, int, error) {
	switch s[i+1] {
	case 'n':
		return '\n', 2, nil
	case 't':
		return '\t', 2, nil
	case 'r':
		return '\r', 2, nil
	case '"':
		return '"', 2, nil
	case '\\':
		return '\\', 2, nil
	case 'u':
		return decodeHexEscape(s, i+2, 4)
	case 'U':
		return decodeHexEscape(s, i+2, 8)
	default:
		return 0, 0, unsupported(fmt.Sprintf("unknown escape \\%c", s[i+1]))
	}
}

// decodeHexEscape reads n hex digits at s[j:] and returns the code point plus
// the total bytes consumed by the whole escape (2 for the `\u`/`\U` prefix).
func decodeHexEscape(s string, j, n int) (rune, int, error) {
	if j+n > len(s) {
		return 0, 0, unsupported("truncated unicode escape")
	}
	var cp rune
	for k := 0; k < n; k++ {
		d, ok := hexVal(s[j+k])
		if !ok {
			return 0, 0, unsupported("invalid unicode escape digit")
		}
		cp = cp<<4 | rune(d)
	}
	return cp, 2 + n, nil
}

// hexVal returns the numeric value of a single hex digit.
func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	default:
		return 0, false
	}
}
