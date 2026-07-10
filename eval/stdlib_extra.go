// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/netip"
	"sort"
	"strings"

	"github.com/go-pcore/pcore"
)

// registerStdlibExtra installs the remaining pure puppetlabs-stdlib functions:
// is_a, enclose_ipv6, stdlib::ip_in_range, stdlib::nested_values,
// stdlib::xml_encode, stdlib::has_function, validate_x509_rsa_key_pair and
// stdlib::sort_by.
func registerStdlibExtra(e *Evaluator) {
	e.funcs["is_a"] = builtinIsA
	e.funcs["enclose_ipv6"] = builtinEncloseIPv6
	e.funcs["stdlib::ip_in_range"] = builtinIPInRange
	e.funcs["stdlib::nested_values"] = builtinNestedValues
	e.funcs["stdlib::xml_encode"] = builtinXMLEncode
	e.funcs["stdlib::has_function"] = builtinHasFunction
	e.funcs["validate_x509_rsa_key_pair"] = builtinValidateX509RSAKeyPair
	e.funcs["sort_by"] = builtinSortBy
	e.funcs["stdlib::sort_by"] = builtinSortBy
}

// builtinIsA implements is_a(value, Type): true when value is an instance of the
// given Pcore type, equivalent to the `=~` type check.
func builtinIsA(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("is_a")
	}
	t, ok := args[1].(pcore.Type)
	if !ok {
		return nil, &Error{Msg: "is_a(): second argument must be a Type"}
	}
	return pcore.IsInstance(t, normalize(args[0])), nil
}

// builtinEncloseIPv6 implements enclose_ipv6(value): wraps every IPv6 address in
// square brackets, leaving IPv4 addresses and "*" untouched, and returns the
// unique list.
func builtinEncloseIPv6(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, &Error{Msg: "enclose_ipv6(): Wrong number of arguments given " + ordinal(len(args)-1) + " for 1"}
	}
	var input []any
	switch v := normalize(args[0]).(type) {
	case string:
		input = []any{v}
	case []any:
		input = v
	default:
		return nil, &Error{Msg: "enclose_ipv6(): Wrong argument type given " + typeName(args[0]) + " expected String or Array"}
	}
	seen := map[string]bool{}
	out := []any{}
	for _, raw := range input {
		if isUndef(raw) {
			continue
		}
		val := stringify(raw)
		if val != "*" {
			addr, err := netip.ParseAddr(val)
			if err != nil {
				return nil, &Error{Msg: "enclose_ipv6(): Wrong argument given " + val + " is not an ip address."}
			}
			if addr.Is6() && !addr.Is4In6() {
				val = "[" + addr.String() + "]"
			}
		}
		if !seen[val] {
			seen[val] = true
			out = append(out, val)
		}
	}
	return out, nil
}

// builtinIPInRange implements stdlib::ip_in_range(ip, range): reports whether ip
// falls within any of the given CIDR range(s).
func builtinIPInRange(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, &Error{Msg: "stdlib::ip_in_range(): expects 2 arguments, got " + ordinal(len(args)-1)}
	}
	ipStr, err := argStr(args, 0, "stdlib::ip_in_range")
	if err != nil {
		return nil, err
	}
	ip, perr := netip.ParseAddr(ipStr)
	if perr != nil {
		return nil, &Error{Msg: "stdlib::ip_in_range(): invalid IP address " + ipStr}
	}
	var ranges []string
	switch v := normalize(args[1]).(type) {
	case string:
		ranges = []string{v}
	case []any:
		for _, e := range v {
			ranges = append(ranges, stringify(e))
		}
	default:
		return nil, &Error{Msg: "stdlib::ip_in_range(): parameter 'range' expects a value of type String or Array, got " + typeName(args[1])}
	}
	for _, r := range ranges {
		if cidrContains(r, ip) {
			return true, nil
		}
	}
	return false, nil
}

// cidrContains reports whether ip is inside the CIDR (or bare address) r.
func cidrContains(r string, ip netip.Addr) bool {
	if strings.Contains(r, "/") {
		p, err := netip.ParsePrefix(r)
		if err != nil {
			return false
		}
		return p.Contains(ip)
	}
	a, err := netip.ParseAddr(r)
	if err != nil {
		return false
	}
	return a == ip
}

// builtinNestedValues implements stdlib::nested_values(hash): the flattened list
// of all values in a (possibly nested) hash.
func builtinNestedValues(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("stdlib::nested_values")
	}
	h, ok := normalize(args[0]).(map[string]any)
	if !ok {
		return nil, &Error{Msg: "stdlib::nested_values(): argument must be a Hash"}
	}
	out := []any{}
	nestedValues(h, &out)
	return out, nil
}

func nestedValues(h map[string]any, out *[]any) {
	for _, k := range sortedKeys(h) {
		if child, ok := normalize(h[k]).(map[string]any); ok {
			nestedValues(child, out)
		} else {
			*out = append(*out, h[k])
		}
	}
}

// builtinXMLEncode implements stdlib::xml_encode(str [, type]): encodes str for
// safe inclusion in XML text ('text', the default) or an attribute value
// ('attr').
func builtinXMLEncode(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("stdlib::xml_encode")
	}
	s, err := argStr(args, 0, "stdlib::xml_encode")
	if err != nil {
		return nil, err
	}
	kind := "text"
	if len(args) == 2 {
		kind, err = argStr(args, 1, "stdlib::xml_encode")
		if err != nil {
			return nil, err
		}
	}
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	switch kind {
	case "text":
		return s, nil
	case "attr":
		s = strings.ReplaceAll(s, "\"", "&quot;")
		return "\"" + s + "\"", nil
	default:
		return nil, &Error{Msg: "stdlib::xml_encode(): type must be 'text' or 'attr'"}
	}
}

// builtinHasFunction implements stdlib::has_function(name): reports whether a
// function of that name is available to the evaluator.
func builtinHasFunction(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("stdlib::has_function")
	}
	name, err := argStr(args, 0, "stdlib::has_function")
	if err != nil {
		return nil, err
	}
	_, ok := c.e.funcs[name]
	return ok, nil
}

// builtinValidateX509RSAKeyPair implements validate_x509_rsa_key_pair(cert, key):
// it fails compilation unless the PEM certificate's signature verifies against
// the supplied PEM RSA private key.
func builtinValidateX509RSAKeyPair(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, &Error{Msg: "validate_x509_rsa_key_pair(): wrong number of arguments (" + ordinal(len(args)-1) + "; must be 2)"}
	}
	certPEM, err := argStr(args, 0, "validate_x509_rsa_key_pair")
	if err != nil {
		return nil, err
	}
	keyPEM, err := argStr(args, 1, "validate_x509_rsa_key_pair")
	if err != nil {
		return nil, err
	}
	cert, cerr := parseCertPEM(certPEM)
	if cerr != nil {
		return nil, &Error{Msg: "validate_x509_rsa_key_pair(): Not a valid x509 certificate: " + cerr.Error()}
	}
	key, kerr := parseRSAKeyPEM(keyPEM)
	if kerr != nil {
		return nil, &Error{Msg: "validate_x509_rsa_key_pair(): Not a valid RSA key: " + kerr.Error()}
	}
	probe := &x509.Certificate{PublicKey: &key.PublicKey}
	if verr := probe.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); verr != nil {
		return nil, &Error{Msg: "validate_x509_rsa_key_pair(): Certificate signature does not match supplied key"}
	}
	return pcore.Undef, nil
}

func parseCertPEM(s string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, &Error{Msg: "no PEM block"}
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseRSAKeyPEM(s string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, &Error{Msg: "no PEM block"}
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, &Error{Msg: "not an RSA key"}
	}
	return rk, nil
}

// builtinSortBy implements stdlib::sort_by(collection) |$e| { ... }: an ordered
// copy of an Array, String or Hash sorted by the key each element maps to.
func builtinSortBy(_ *Context, args []Value, block *Block) (Value, error) {
	if len(args) != 1 {
		return nil, wrongArgs("stdlib::sort_by")
	}
	if block == nil {
		return nil, &Error{Msg: "stdlib::sort_by(): expects a block"}
	}
	switch coll := normalize(args[0]).(type) {
	case []any:
		out := append([]any{}, coll...)
		return sortByKey(out, func(i int) (Value, error) { return block.Call(out[i]) })
	case string:
		runes := []rune(coll)
		elems := make([]any, len(runes))
		for i, r := range runes {
			elems[i] = string(r)
		}
		sorted, err := sortByKey(elems, func(i int) (Value, error) { return block.Call(elems[i]) })
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, e := range sorted.([]any) {
			b.WriteString(e.(string))
		}
		return b.String(), nil
	case map[string]any:
		// Our Hashes are unordered, so a sorted copy has the same entries; we
		// still invoke the block per entry to surface any error it raises.
		for _, k := range sortedKeys(coll) {
			var err error
			if block.Arity() >= 2 {
				_, err = block.Call(k, coll[k])
			} else {
				_, err = block.Call([]any{k, coll[k]})
			}
			if err != nil {
				return nil, err
			}
		}
		return cloneHash(coll), nil
	default:
		return nil, &Error{Msg: "stdlib::sort_by(): parameter 'ary' expects an Array value, got " + typeName(args[0])}
	}
}

// sortByKey returns elems stably ordered by the key produced by keyFn(i).
func sortByKey(elems []any, keyFn func(i int) (Value, error)) (Value, error) {
	keys := make([]Value, len(elems))
	for i := range elems {
		k, err := keyFn(i)
		if err != nil {
			return nil, err
		}
		keys[i] = k
	}
	idx := make([]int, len(elems))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return spaceship(keys[idx[a]], keys[idx[b]]) < 0
	})
	out := make([]any, len(elems))
	for i, j := range idx {
		out[i] = elems[j]
	}
	return out, nil
}

// spaceship orders two values like Ruby's <=>: numerically for numbers and by
// byte order (case-sensitive) for strings.
func spaceship(a, b Value) int {
	an, bn := normalize(a), normalize(b)
	if af, ok := asFloat(an); ok {
		if bf, ok := asFloat(bn); ok {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			default:
				return 0
			}
		}
	}
	as, aok := an.(string)
	bs, bok := bn.(string)
	if aok && bok {
		return strings.Compare(as, bs)
	}
	return strings.Compare(stringify(an), stringify(bn))
}
