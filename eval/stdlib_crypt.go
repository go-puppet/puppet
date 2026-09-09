// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"regexp"
	"strconv"

	"github.com/go-encryptions/unixcrypt"
	"github.com/go-pcore/pcore"
)

// This file implements pw_hash. Puppet's pw_hash builds a crypt(3) salt of
// the form "$<prefix>$<salt>" and relies on the platform crypt(3) to do the
// hashing; to stay CGO-free and portable, the actual algorithms (MD5-crypt,
// SHA-256/512-crypt, bcrypt) live in github.com/go-encryptions/unixcrypt —
// extracted from an earlier version of this file so the implementation is
// shared, not duplicated, with other consumers that need crypt(3)-format
// password hashing (currently also go-ansible/template's password_hash
// filter). This file keeps only the Puppet-specific glue: pw_hash's own
// argument validation and error messages, and its salt-format conventions.
// Puppet's own pw_hash() has no rounds parameter, so SHA-256/512-crypt are
// always called with rounds=0 (unixcrypt's own "unspecified", matching this
// file's prior behavior exactly: the crypt(3) spec's own default of 5000,
// no rounds= prefix ever emitted).

type pwHashType struct {
	prefix string
	salt   *regexp.Regexp // non-nil for bcrypt variants
}

var pwHashTypes = map[string]pwHashType{
	"md5":      {prefix: "1"},
	"sha-256":  {prefix: "5"},
	"sha-512":  {prefix: "6"},
	"bcrypt":   {prefix: "2b", salt: bcryptSaltRe},
	"bcrypt-a": {prefix: "2a", salt: bcryptSaltRe},
	"bcrypt-x": {prefix: "2x", salt: bcryptSaltRe},
	"bcrypt-y": {prefix: "2y", salt: bcryptSaltRe},
}

var (
	bcryptSaltRe    = regexp.MustCompile(`^(0[4-9]|[12][0-9]|3[01])\$[./A-Za-z0-9]{22}`)
	plainSaltRe     = regexp.MustCompile(`^[a-zA-Z0-9./]+$`)
	pwHashTypeNames = "md5, sha-256, sha-512, bcrypt, bcrypt-a, bcrypt-x, bcrypt-y"
)

// builtinPwHash implements pw_hash(password, type, salt).
func builtinPwHash(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 3 {
		return nil, &Error{Msg: "pw_hash(): wrong number of arguments (" + strconv.Itoa(len(args)) + " for 3)"}
	}
	// First argument may be a String or Undef.
	var password string
	switch p := normalize(args[0]).(type) {
	case string:
		password = p
	default:
		if !isUndef(p) {
			return nil, &Error{Msg: "pw_hash(): first argument must be a string"}
		}
	}
	typeName, err := argStr(args, 1, "pw_hash")
	if err != nil {
		return nil, &Error{Msg: "pw_hash(): second argument must be a string"}
	}
	ht, ok := pwHashTypes[toLowerASCII(typeName)]
	if !ok {
		return nil, &Error{Msg: "pw_hash(): " + typeName + " is not a valid hash type"}
	}
	salt, err := argStr(args, 2, "pw_hash")
	if err != nil {
		return nil, &Error{Msg: "pw_hash(): third argument must be a string"}
	}
	if salt == "" {
		return nil, &Error{Msg: "pw_hash(): third argument must not be empty"}
	}
	if ht.salt != nil {
		if !ht.salt.MatchString(salt) {
			return nil, &Error{Msg: "pw_hash(): characters in salt must match " + ht.salt.String()}
		}
	} else if !plainSaltRe.MatchString(salt) {
		return nil, &Error{Msg: "pw_hash(): characters in salt must be in the set [a-zA-Z0-9./]"}
	}
	// An empty (or undef) password hashes to nothing, matching upstream.
	if password == "" {
		return pcore.Undef, nil
	}

	if ht.salt != nil {
		// bcryptSaltRe already validated salt's shape (cost 04-31, a
		// 22-char base64 salt) to exactly the constraints
		// BcryptFromMCF re-checks, so its error is provably
		// unreachable here — the same "validated upstream, so this
		// specific error can't fire" reasoning this file already
		// applied to blowfish.NewSaltedCipher before this function's
		// algorithms moved into unixcrypt.
		hashed, _ := unixcrypt.BcryptFromMCF(password, ht.prefix, salt)
		return hashed, nil
	}
	switch ht.prefix {
	case "1":
		return unixcrypt.MD5Crypt(password, salt), nil
	case "5":
		return unixcrypt.SHA256Crypt(password, salt, 0), nil
	default: // "6"
		return unixcrypt.SHA512Crypt(password, salt, 0), nil
	}
}

// toLowerASCII lowercases ASCII letters (hash type names are ASCII).
func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
