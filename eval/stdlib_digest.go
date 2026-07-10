// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"hash/crc32"
	"strconv"
)

// registerStdlibDigest installs the cryptographic digest functions. These match
// Puppet core `md5`/`sha1` and puppetlabs-stdlib `stdlib::sha256`, each of which
// returns the lowercase hex digest of its single String argument.
func registerStdlibDigest(e *Evaluator) {
	e.funcs["md5"] = digestFn(md5.New, "md5")
	e.funcs["sha1"] = digestFn(sha1.New, "sha1")
	e.funcs["sha256"] = digestFn(sha256.New, "sha256")
	e.funcs["sha512"] = digestFn(sha512.New, "sha512")
	// puppetlabs-stdlib namespaces sha256 as stdlib::sha256.
	e.funcs["stdlib::sha256"] = e.funcs["sha256"]
	e.funcs["stdlib::crc32"] = builtinCRC32
	e.funcs["crc32"] = builtinCRC32
	e.funcs["fqdn_uuid"] = builtinFqdnUUID
}

// builtinCRC32 implements stdlib::crc32(value): the lowercase, unpadded
// hexadecimal CRC-32 (IEEE) of the value's Ruby string form, matching
// Zlib.crc32(value.to_s).to_s(16).
func builtinCRC32(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, &Error{Msg: "stdlib::crc32(): wrong number of arguments (given " + ordinal(len(args)-1) + ", expected 1)"}
	}
	sum := crc32.ChecksumIEEE([]byte(scalarToRubyString(args[0])))
	return strconv.FormatUint(uint64(sum), 16), nil
}

// scalarToRubyString renders a scalar as Ruby's Object#to_s would: integers and
// booleans plainly, floats with a mandatory decimal point (100.0, not 100), and
// strings verbatim.
func scalarToRubyString(v Value) string {
	switch x := normalize(v).(type) {
	case float64:
		return rubyFloatToS(x)
	default:
		return stringify(x)
	}
}

// uuidNamespaceDNS is the RFC 4122 DNS namespace UUID (6ba7b810-9dad-11d1-80b4-00c04fd430c8).
var uuidNamespaceDNS = []byte{
	0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1,
	0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
}

// builtinFqdnUUID implements fqdn_uuid(string): an RFC 4122 version 5 (SHA-1,
// DNS namespace) UUID for the given name.
func builtinFqdnUUID(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) == 0 {
		return nil, &Error{Msg: "fqdn_uuid(): No arguments given"}
	}
	if len(args) != 1 {
		return nil, &Error{Msg: "fqdn_uuid(): Too many arguments given (" + strconv.Itoa(len(args)) + ")"}
	}
	name, err := argStr(args, 0, "fqdn_uuid")
	if err != nil {
		return nil, err
	}
	h := sha1.New()
	h.Write(uuidNamespaceDNS)
	h.Write([]byte(name))
	b := h.Sum(nil)[:16]
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // DCE 1.1 variant
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16]), nil
}

// digestFn builds a function computing the lowercase hex digest of a String.
func digestFn(newHash func() hash.Hash, name string) Function {
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) != 1 {
			return nil, wrongArgs(name)
		}
		s, err := argStr(args, 0, name)
		if err != nil {
			return nil, err
		}
		h := newHash()
		h.Write([]byte(s))
		return hex.EncodeToString(h.Sum(nil)), nil
	}
}
