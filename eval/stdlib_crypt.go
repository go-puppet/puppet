// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"regexp"
	"strconv"

	"github.com/go-pcore/pcore"
	"golang.org/x/crypto/blowfish"
)

// This file implements pw_hash in pure Go. Puppet's pw_hash builds a crypt(3)
// salt of the form "$<prefix>$<salt>" and relies on the platform crypt(3) to do
// the hashing. To stay CGO-free and portable we implement the crypt algorithms
// directly: MD5-crypt ($1$), SHA-256-crypt ($5$) and SHA-512-crypt ($6$) per
// Ulrich Drepper's specification, and bcrypt ($2a$/$2b$/$2x$/$2y$) via the
// pure-Go blowfish primitive. Outputs were validated against `openssl passwd`
// and the canonical OpenBSD bcrypt test vectors.

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
		return bcryptCrypt(password, ht.prefix, salt)
	}
	switch ht.prefix {
	case "1":
		return md5Crypt(password, salt), nil
	case "5":
		return shaCrypt(password, salt, false), nil
	default: // "6"
		return shaCrypt(password, salt, true), nil
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

// --- crypt(3) base64 ------------------------------------------------------

const cryptB64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// b64From24 emits n crypt-base64 chars from the 24-bit value b2<<16|b1<<8|b0.
func b64From24(b2, b1, b0 byte, n int, out *[]byte) {
	w := uint(b2)<<16 | uint(b1)<<8 | uint(b0)
	for ; n > 0; n-- {
		*out = append(*out, cryptB64[w&0x3f])
		w >>= 6
	}
}

// --- MD5-crypt ($1$) ------------------------------------------------------

func md5Crypt(password, salt string) string {
	const magic = "$1$"
	pw := []byte(password)
	s := []byte(salt)

	b := md5.New()
	b.Write(pw)
	b.Write(s)
	b.Write(pw)
	alt := b.Sum(nil)

	a := md5.New()
	a.Write(pw)
	a.Write([]byte(magic))
	a.Write(s)
	for i := len(pw); i > 0; i -= 16 {
		if i > 16 {
			a.Write(alt[:16])
		} else {
			a.Write(alt[:i])
		}
	}
	for i := len(pw); i > 0; i >>= 1 {
		if i&1 == 1 {
			a.Write([]byte{0})
		} else {
			a.Write(pw[:1])
		}
	}
	sum := a.Sum(nil)

	for i := 0; i < 1000; i++ {
		c := md5.New()
		if i&1 == 1 {
			c.Write(pw)
		} else {
			c.Write(sum)
		}
		if i%3 != 0 {
			c.Write(s)
		}
		if i%7 != 0 {
			c.Write(pw)
		}
		if i&1 == 1 {
			c.Write(sum)
		} else {
			c.Write(pw)
		}
		sum = c.Sum(nil)
	}

	out := make([]byte, 0, 34)
	out = append(out, magic...)
	out = append(out, s...)
	out = append(out, '$')
	b64From24(sum[0], sum[6], sum[12], 4, &out)
	b64From24(sum[1], sum[7], sum[13], 4, &out)
	b64From24(sum[2], sum[8], sum[14], 4, &out)
	b64From24(sum[3], sum[9], sum[15], 4, &out)
	b64From24(sum[4], sum[10], sum[5], 4, &out)
	b64From24(0, 0, sum[11], 2, &out)
	return string(out)
}

// --- SHA-crypt ($5$/$6$) --------------------------------------------------

func shaCrypt(password, salt string, is512 bool) string {
	var newHash func() hash.Hash
	var magic string
	var hlen int
	if is512 {
		newHash, magic, hlen = sha512.New, "$6$", 64
	} else {
		newHash, magic, hlen = sha256.New, "$5$", 32
	}

	pw := []byte(password)
	s := []byte(salt)
	if len(s) > 16 {
		s = s[:16]
	}

	b := newHash()
	b.Write(pw)
	b.Write(s)
	b.Write(pw)
	sumB := b.Sum(nil)

	a := newHash()
	a.Write(pw)
	a.Write(s)
	for i := len(pw); i > 0; i -= hlen {
		if i > hlen {
			a.Write(sumB)
		} else {
			a.Write(sumB[:i])
		}
	}
	for i := len(pw); i > 0; i >>= 1 {
		if i&1 == 1 {
			a.Write(sumB)
		} else {
			a.Write(pw)
		}
	}
	sumA := a.Sum(nil)

	dp := newHash()
	for range pw {
		dp.Write(pw)
	}
	p := cryptSeq(dp.Sum(nil), len(pw), hlen)

	ds := newHash()
	for i := 0; i < 16+int(sumA[0]); i++ {
		ds.Write(s)
	}
	sBytes := cryptSeq(ds.Sum(nil), len(s), hlen)

	cur := sumA
	for i := 0; i < 5000; i++ {
		c := newHash()
		if i&1 == 1 {
			c.Write(p)
		} else {
			c.Write(cur)
		}
		if i%3 != 0 {
			c.Write(sBytes)
		}
		if i%7 != 0 {
			c.Write(p)
		}
		if i&1 == 1 {
			c.Write(cur)
		} else {
			c.Write(p)
		}
		cur = c.Sum(nil)
	}

	out := make([]byte, 0, 106)
	out = append(out, magic...)
	out = append(out, s...)
	out = append(out, '$')
	if is512 {
		out = shaEncode512(cur, out)
	} else {
		out = shaEncode256(cur, out)
	}
	return string(out)
}

// cryptSeq builds a length-n byte sequence by repeating the hlen-byte digest.
func cryptSeq(digest []byte, n, hlen int) []byte {
	out := make([]byte, 0, n)
	for i := n; i > 0; i -= hlen {
		if i > hlen {
			out = append(out, digest...)
		} else {
			out = append(out, digest[:i]...)
		}
	}
	return out
}

func shaEncode256(c, out []byte) []byte {
	perm := [][3]int{
		{0, 10, 20}, {21, 1, 11}, {12, 22, 2}, {3, 13, 23},
		{24, 4, 14}, {15, 25, 5}, {6, 16, 26}, {27, 7, 17},
		{18, 28, 8}, {9, 19, 29},
	}
	for _, p := range perm {
		b64From24(c[p[0]], c[p[1]], c[p[2]], 4, &out)
	}
	b64From24(0, c[31], c[30], 3, &out)
	return out
}

func shaEncode512(c, out []byte) []byte {
	perm := [][3]int{
		{0, 21, 42}, {22, 43, 1}, {44, 2, 23}, {3, 24, 45},
		{25, 46, 4}, {47, 5, 26}, {6, 27, 48}, {28, 49, 7},
		{50, 8, 29}, {9, 30, 51}, {31, 52, 10}, {53, 11, 32},
		{12, 33, 54}, {34, 55, 13}, {56, 14, 35}, {15, 36, 57},
		{37, 58, 16}, {59, 17, 38}, {18, 39, 60}, {40, 61, 19},
		{62, 20, 41},
	}
	for _, p := range perm {
		b64From24(c[p[0]], c[p[1]], c[p[2]], 4, &out)
	}
	b64From24(0, 0, c[63], 2, &out)
	return out
}

// --- bcrypt ($2a$/$2b$/$2x$/$2y$) -----------------------------------------

const bcryptB64 = "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

var magicCipherData = []byte("OrpheanBeholderScryDoubt")

func bcryptB64Decode(s string) []byte {
	var rev [256]byte
	for i := range rev {
		rev[i] = 0xff
	}
	for i := 0; i < len(bcryptB64); i++ {
		rev[bcryptB64[i]] = byte(i)
	}
	var out []byte
	var buf uint
	bits := 0
	for i := 0; i < len(s); i++ {
		v := rev[s[i]]
		if v == 0xff {
			continue
		}
		buf = buf<<6 | uint(v)
		bits += 6
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(buf>>uint(bits)))
		}
	}
	return out
}

func bcryptB64Encode(src []byte) []byte {
	var out []byte
	var buf uint
	bits := 0
	for _, b := range src {
		buf = buf<<8 | uint(b)
		bits += 8
		for bits >= 6 {
			bits -= 6
			out = append(out, bcryptB64[(buf>>uint(bits))&0x3f])
		}
	}
	if bits > 0 {
		out = append(out, bcryptB64[(buf<<uint(6-bits))&0x3f])
	}
	return out
}

// bcryptCrypt reproduces crypt(3) bcrypt. salt is "<cost>$<22-char base64>".
func bcryptCrypt(password, prefix, salt string) (Value, error) {
	cost, err := strconv.Atoi(salt[:2])
	if err != nil {
		return nil, &Error{Msg: "pw_hash(): invalid bcrypt cost"}
	}
	salt22 := salt[3:25]
	// A 22-character bcrypt-base64 salt always decodes to exactly 16 bytes.
	csalt := bcryptB64Decode(salt22)[:16]

	// key is password + NUL and is therefore never empty, so NewSaltedCipher
	// (which only errors on a zero-length key) cannot fail here.
	key := append([]byte(password), 0)
	c, _ := blowfish.NewSaltedCipher(key, csalt)
	rounds := uint64(1) << uint(cost)
	for i := uint64(0); i < rounds; i++ {
		blowfish.ExpandKey(key, c)
		blowfish.ExpandKey(csalt, c)
	}
	cipherData := make([]byte, len(magicCipherData))
	copy(cipherData, magicCipherData)
	for i := 0; i < 24; i += 8 {
		for j := 0; j < 64; j++ {
			c.Encrypt(cipherData[i:i+8], cipherData[i:i+8])
		}
	}
	hsh := bcryptB64Encode(cipherData[:23])
	cs := strconv.Itoa(cost)
	if cost < 10 {
		cs = "0" + cs
	}
	return "$" + prefix + "$" + cs + "$" + salt22 + string(hsh), nil
}
