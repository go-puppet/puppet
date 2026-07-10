// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	mrand "math/rand"
	"strings"
)

// This file implements the deterministic, seeded random functions from Puppet
// core (fqdn_rand) and puppetlabs-stdlib (seeded_rand, seeded_rand_string,
// fqdn_rotate, fqdn_rand_string, shuffle).
//
// Puppet derives every deterministic value from Ruby's Random, which is a
// Mersenne-Twister (MT19937) seeded via init-by-array from a (typically 128-bit)
// integer digest of the input. To be byte-for-byte compatible we reproduce
// MRI's MT19937 exactly: init_genrand / init_by_array seeding, the tempered
// genrand_int32 output, and the rejection sampling of Random#rand(max)
// ("limited_rand"). The results were validated against MRI Ruby.

// registerStdlibRandom installs the seeded random function set.
func registerStdlibRandom(e *Evaluator) {
	e.funcs["fqdn_rand"] = builtinFqdnRand
	e.funcs["seeded_rand"] = builtinSeededRand
	e.funcs["seeded_rand_string"] = builtinSeededRandString
	e.funcs["fqdn_rotate"] = builtinFqdnRotate
	e.funcs["fqdn_rand_string"] = builtinFqdnRandString
	e.funcs["shuffle"] = builtinShuffle
	// puppetlabs-stdlib namespaced aliases.
	e.funcs["stdlib::seeded_rand"] = builtinSeededRand
	e.funcs["stdlib::seeded_rand_string"] = builtinSeededRandString
	e.funcs["stdlib::fqdn_rotate"] = builtinFqdnRotate
	e.funcs["stdlib::fqdn_rand_string"] = builtinFqdnRandString
}

// --- MRI-compatible Mersenne Twister -------------------------------------

const mtN = 624

type mtState struct {
	state [mtN]uint32
	pos   int
}

func (m *mtState) initGenrand(s uint32) {
	m.state[0] = s
	for j := 1; j < mtN; j++ {
		m.state[j] = 1812433253*(m.state[j-1]^(m.state[j-1]>>30)) + uint32(j)
	}
	m.pos = mtN
}

func (m *mtState) initByArray(key []uint32) {
	m.initGenrand(19650218)
	i, j := 1, 0
	k := mtN
	if len(key) > k {
		k = len(key)
	}
	for ; k > 0; k-- {
		m.state[i] = (m.state[i] ^ ((m.state[i-1] ^ (m.state[i-1] >> 30)) * 1664525)) + key[j] + uint32(j)
		i++
		j++
		if i >= mtN {
			m.state[0] = m.state[mtN-1]
			i = 1
		}
		if j >= len(key) {
			j = 0
		}
	}
	for k = mtN - 1; k > 0; k-- {
		m.state[i] = (m.state[i] ^ ((m.state[i-1] ^ (m.state[i-1] >> 30)) * 1566083941)) - uint32(i)
		i++
		if i >= mtN {
			m.state[0] = m.state[mtN-1]
			i = 1
		}
	}
	m.state[0] = 0x80000000
	m.pos = mtN
}

func (m *mtState) genrand() uint32 {
	const mm = 397
	const matrixA = 0x9908b0df
	const upper = 0x80000000
	const lower = 0x7fffffff
	if m.pos >= mtN {
		var kk int
		for kk = 0; kk < mtN-mm; kk++ {
			y := (m.state[kk] & upper) | (m.state[kk+1] & lower)
			m.state[kk] = m.state[kk+mm] ^ (y >> 1) ^ ((y & 1) * matrixA)
		}
		for ; kk < mtN-1; kk++ {
			y := (m.state[kk] & upper) | (m.state[kk+1] & lower)
			m.state[kk] = m.state[kk+(mm-mtN)] ^ (y >> 1) ^ ((y & 1) * matrixA)
		}
		y := (m.state[mtN-1] & upper) | (m.state[0] & lower)
		m.state[mtN-1] = m.state[mm-1] ^ (y >> 1) ^ ((y & 1) * matrixA)
		m.pos = 0
	}
	y := m.state[m.pos]
	m.pos++
	y ^= y >> 11
	y ^= (y << 7) & 0x9d2c5680
	y ^= (y << 15) & 0xefc60000
	y ^= y >> 18
	return y
}

// newMTFromBig seeds an MT the way MRI's rand_init does: the seed integer is
// packed into 32-bit words, least-significant word first, then fed to
// init_by_array (or init_genrand when a single word).
func newMTFromBig(seed *big.Int) *mtState {
	m := &mtState{}
	s := new(big.Int).Abs(seed)
	var key []uint32
	if s.Sign() == 0 {
		key = []uint32{0}
	} else {
		tmp := new(big.Int).Set(s)
		mask := big.NewInt(0xffffffff)
		for tmp.Sign() > 0 {
			w := new(big.Int).And(tmp, mask)
			key = append(key, uint32(w.Uint64()))
			tmp.Rsh(tmp, 32)
		}
	}
	// MRI's rand_init chooses init_genrand vs init_by_array on the untrimmed
	// word count, then drops a most-significant word equal to 1 before the
	// array seeding (so a two-word seed like 2**32 still uses init_by_array).
	if len(key) <= 1 {
		m.initGenrand(key[0])
	} else {
		if key[len(key)-1] == 1 {
			key = key[:len(key)-1]
		}
		m.initByArray(key)
	}
	return m
}

func makeMask(x uint64) uint64 {
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16
	x |= x >> 32
	return x
}

// limitedRand returns a value in [0, limit] using MRI's rejection sampling.
func (m *mtState) limitedRand(limit uint64) uint64 {
	if limit == 0 {
		return 0
	}
	mask := makeMask(limit)
	for {
		var val uint64
		for i := 1; i >= 0; i-- {
			if (mask>>(uint(i)*32))&0xffffffff != 0 {
				r := uint64(m.genrand()) & (mask >> (uint(i) * 32))
				val |= r << (uint(i) * 32)
			}
		}
		if val <= limit {
			return val
		}
	}
}

// randInt returns a value in [0, max) matching Ruby's Random#rand(max) for a
// positive integer max.
func (m *mtState) randInt(max uint64) uint64 { return m.limitedRand(max - 1) }

// deterministicRandInt reproduces Puppet::Util.deterministic_rand_int: seed a
// fresh MT with the (big-integer) seed and draw one rand(max).
func deterministicRandInt(seed *big.Int, max uint64) uint64 {
	return newMTFromBig(seed).randInt(max)
}

// md5Seed returns the MD5 hex digest of s reinterpreted as a big integer
// (Ruby's Digest::MD5.hexdigest(s).hex).
func md5Seed(s string) *big.Int {
	sum := md5.Sum([]byte(s))
	n := new(big.Int)
	n.SetString(hex.EncodeToString(sum[:]), 16)
	return n
}

// sha256Seed returns the SHA-256 hex digest of s as a big integer.
func sha256Seed(s string) *big.Int {
	sum := sha256.Sum256([]byte(s))
	n := new(big.Int)
	n.SetString(hex.EncodeToString(sum[:]), 16)
	return n
}

// --- fqdn helper ----------------------------------------------------------

// fqdnFact resolves $facts['networking']['fqdn'] from the current scope,
// returning "" when unavailable (Ruby nil.to_s).
func fqdnFact(c *Context) string {
	facts, ok := c.scope.lookup("facts")
	if !ok {
		return ""
	}
	m, ok := normalize(facts).(map[string]any)
	if !ok {
		return ""
	}
	net, ok := m["networking"].(map[string]any)
	if !ok {
		return ""
	}
	if f, ok := net["fqdn"].(string); ok {
		return f
	}
	return ""
}

// --- functions ------------------------------------------------------------

// builtinFqdnRand implements Puppet core fqdn_rand(max, [seed], [downcase]).
func builtinFqdnRand(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 3 {
		return nil, wrongArgs("fqdn_rand")
	}
	max, ok := asInt64(args[0])
	if !ok || max < 1 {
		return nil, &Error{Msg: "fqdn_rand(): first argument must be a positive Integer"}
	}
	initialSeed := ""
	if len(args) >= 2 {
		initialSeed = stringify(args[1])
	}
	fqdn := fqdnFact(c)
	if len(args) >= 3 && truthy(args[2]) {
		fqdn = strings.ToLower(fqdn)
	}
	joined := fqdn + ":" + stringify(max) + ":" + initialSeed
	return int64(deterministicRandInt(md5Seed(joined), uint64(max))), nil
}

// builtinSeededRand implements stdlib seeded_rand(max, seed).
func builtinSeededRand(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 2 {
		return nil, wrongArgs("seeded_rand")
	}
	max, ok := asInt64(args[0])
	if !ok || max < 1 {
		return nil, &Error{Msg: "seeded_rand(): first argument must be an Integer >= 1"}
	}
	seed, err := argStr(args, 1, "seeded_rand")
	if err != nil {
		return nil, err
	}
	return int64(deterministicRandInt(md5Seed(seed), uint64(max))), nil
}

const seededRandCharset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// builtinSeededRandString implements stdlib seeded_rand_string(length, seed, [charset]).
func builtinSeededRandString(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, wrongArgs("seeded_rand_string")
	}
	length, ok := asInt64(args[0])
	if !ok || length < 1 {
		return nil, &Error{Msg: "seeded_rand_string(): length must be an Integer >= 1"}
	}
	seed, err := argStr(args, 1, "seeded_rand_string")
	if err != nil {
		return nil, err
	}
	charset := []rune(seededRandCharset)
	if len(args) == 3 {
		cs, err := argStr(args, 2, "seeded_rand_string")
		if err != nil {
			return nil, err
		}
		if len([]rune(cs)) < 2 {
			return nil, &Error{Msg: "seeded_rand_string(): charset must be at least 2 characters"}
		}
		charset = []rune(cs)
	}
	m := newMTFromBig(sha256Seed(seed))
	out := make([]rune, length)
	for i := int64(0); i < length; i++ {
		out[i] = charset[m.randInt(uint64(len(charset)))]
	}
	return string(out), nil
}

// builtinFqdnRotate implements stdlib fqdn_rotate(input, *seeds) for Arrays and
// Strings: a deterministic left-rotation seeded off the fqdn and seeds.
func builtinFqdnRotate(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, wrongArgs("fqdn_rotate")
	}
	seeds := make([]string, 0, len(args)-1)
	for _, s := range args[1:] {
		seeds = append(seeds, stringify(s))
	}
	joined := fqdnFact(c) + ":" + strings.Join(seeds, ":")
	switch in := normalize(args[0]).(type) {
	case []any:
		if len(in) <= 1 {
			return in, nil
		}
		out := append([]any(nil), in...)
		offset := deterministicRandInt(md5Seed(joined), uint64(len(out)))
		rotateLeft(out, int(offset))
		return out, nil
	case string:
		runes := []rune(in)
		if len(runes) <= 1 {
			return in, nil
		}
		arr := make([]any, len(runes))
		for i, r := range runes {
			arr[i] = string(r)
		}
		offset := deterministicRandInt(md5Seed(joined), uint64(len(arr)))
		rotateLeft(arr, int(offset))
		var b strings.Builder
		for _, r := range arr {
			b.WriteString(r.(string))
		}
		return b.String(), nil
	default:
		return nil, &Error{Msg: "fqdn_rotate(): first argument must be an Array or String"}
	}
}

// rotateLeft rotates s left by n positions in place (n may exceed len). It is
// only called with a non-empty slice.
func rotateLeft(s []any, n int) {
	n %= len(s)
	tmp := append(append([]any(nil), s[n:]...), s[:n]...)
	copy(s, tmp)
}

const fqdnRandStringCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// builtinFqdnRandString implements stdlib fqdn_rand_string(length, [charset], *seed).
func builtinFqdnRandString(c *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 {
		return nil, wrongArgs("fqdn_rand_string")
	}
	length, ok := asInt64(args[0])
	if !ok || length < 1 {
		return nil, &Error{Msg: "fqdn_rand_string(): length must be an Integer >= 1"}
	}
	charset := []rune(fqdnRandStringCharset)
	if len(args) >= 2 {
		cs := stringify(args[1])
		if cs != "" {
			charset = []rune(cs)
		}
	}
	seeds := make([]string, 0)
	if len(args) > 2 {
		for _, s := range args[2:] {
			seeds = append(seeds, stringify(s))
		}
	}
	fqdn := fqdnFact(c)
	size := uint64(len(charset))
	out := make([]rune, length)
	for i := int64(0); i < length; i++ {
		perSeed := append(append([]string(nil), seeds...), stringify(int64(i+1)))
		ss := strings.Join(perSeed, ":")
		joined := fqdn + ":" + stringify(int64(size)) + ":" + ss
		idx := deterministicRandInt(md5Seed(joined), size)
		out[i] = charset[idx]
	}
	return string(out), nil
}

// builtinShuffle implements stdlib shuffle(input): a non-deterministic
// Fisher-Yates shuffle of an Array or String (matches upstream, which uses the
// global unseeded PRNG).
func builtinShuffle(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) != 1 {
		return nil, &Error{Msg: "shuffle(): expects one Array or String argument"}
	}
	switch in := normalize(args[0]).(type) {
	case []any:
		out := append([]any(nil), in...)
		fisherYates(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		return out, nil
	case string:
		runes := []rune(in)
		fisherYates(len(runes), func(i, j int) { runes[i], runes[j] = runes[j], runes[i] })
		return string(runes), nil
	default:
		return nil, &Error{Msg: "shuffle(): requires either an Array or a String"}
	}
}

func fisherYates(n int, swap func(i, j int)) {
	for i := 0; i < n; i++ {
		j := i + mrand.Intn(n-i)
		swap(i, j)
	}
}
