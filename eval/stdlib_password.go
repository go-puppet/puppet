// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"strconv"

	"golang.org/x/crypto/pbkdf2"
)

// This file implements the puppetlabs-stdlib password-hashing helpers used to
// pre-compute macOS user password attributes, in pure Go:
//
//   - str2saltedsha512  -> salted-SHA512 hash (macOS 10.7)
//   - str2saltedpbkdf2  -> salted PBKDF2-HMAC-SHA512 hash (macOS 10.8+)

// registerStdlibPassword installs the salted password-hash functions.
func registerStdlibPassword(e *Evaluator) {
	e.funcs["str2saltedsha512"] = builtinStr2SaltedSHA512
	e.funcs["str2saltedpbkdf2"] = builtinStr2SaltedPBKDF2
}

// saltedSHA512Seed returns the 4-byte random seed prepended to the SHA512
// digest. It is a variable so tests can pin a deterministic seed.
var saltedSHA512Seed = randomSaltedSeed

// randomSaltedSeed mirrors Ruby's `Array(rand((2**31)-1)).pack('L')`: a random
// unsigned 31-bit integer packed little-endian into four bytes.
func randomSaltedSeed() []byte {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	n := binary.LittleEndian.Uint32(buf[:]) % ((1 << 31) - 1)
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, n)
	return out
}

// builtinStr2SaltedSHA512 implements str2saltedsha512(password): the hex of a
// 4-byte random seed followed by SHA512(seed + password).
func builtinStr2SaltedSHA512(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, &Error{Msg: "str2saltedsha512(): Wrong number of arguments passed (" + strconv.Itoa(len(args)) + " but we require 1)"}
	}
	password, err := argStr(args, 0, "str2saltedsha512")
	if err != nil {
		return nil, &Error{Msg: "str2saltedsha512(): Requires a String argument"}
	}
	seed := saltedSHA512Seed()
	sum := sha512.Sum512(append(append([]byte{}, seed...), []byte(password)...))
	return hex.EncodeToString(seed) + hex.EncodeToString(sum[:]), nil
}

// builtinStr2SaltedPBKDF2 implements str2saltedpbkdf2(password, salt, iterations):
// a Hash with the PBKDF2-HMAC-SHA512 password hash, the salt, and the iteration
// count, matching macOS 10.8+ user password attributes.
func builtinStr2SaltedPBKDF2(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 3 {
		return nil, &Error{Msg: "str2saltedpbkdf2(): wrong number of arguments (" + strconv.Itoa(len(args)) + " for 3)"}
	}
	password, ok := normalize(args[0]).(string)
	if !ok {
		return nil, &Error{Msg: "str2saltedpbkdf2(): first argument must be a string"}
	}
	salt, ok := normalize(args[1]).(string)
	if !ok {
		return nil, &Error{Msg: "str2saltedpbkdf2(): second argument must be a string"}
	}
	if len(salt) < 8 {
		return nil, &Error{Msg: "str2saltedpbkdf2(): second argument must be at least 8 bytes long"}
	}
	iterations, ok := normalize(args[2]).(int64)
	if !ok {
		return nil, &Error{Msg: "str2saltedpbkdf2(): third argument must be an integer"}
	}
	if iterations <= 40_000 || iterations >= 70_000 {
		return nil, &Error{Msg: "str2saltedpbkdf2(): third argument must be between 40,000 and 70,000"}
	}
	hashBytes := pbkdf2.Key([]byte(password), []byte(salt), int(iterations), 128, sha512.New)
	return map[string]any{
		"password_hex": hex.EncodeToString(hashBytes),
		"salt_hex":     hex.EncodeToString([]byte(salt)),
		"iterations":   iterations,
	}, nil
}
