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
