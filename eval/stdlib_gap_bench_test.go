// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"crypto/sha256"
	"math/big"
	"testing"
	"time"
)

// Benchmarks for the non-trivial functions added to close the stdlib gap. The
// reference-comparison methodology (against MRI puppet on a Tart VM) is
// documented in BENCHMARKS.md.

func BenchmarkSHA256(b *testing.B) {
	fn := digestFn(sha256.New, "sha256")
	args := []Value{"the quick brown fox jumps over the lazy dog"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := fn(nil, args, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPwHashSHA512(b *testing.B) {
	// SHA-512-crypt runs 5000 hash rounds; this is intentionally the dominant
	// cost and mirrors the reference crypt(3) work factor.
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		shaCrypt("correct horse battery staple", "abc123", true)
	}
}

func BenchmarkPwHashBcrypt(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := bcryptCrypt("password", "2b", "10$cgT08pfGUo9SUIIvXrIJ1u"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFqdnRand(b *testing.B) {
	b.ReportAllocs()
	seed := md5Seed("host.example.com:30:")
	for i := 0; i < b.N; i++ {
		deterministicRandInt(seed, 30)
	}
}

func BenchmarkStrftime(b *testing.B) {
	tm := time.Unix(1471954394, 0).UTC()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		strftime(tm, "%Y-%m-%dT%H:%M:%S%z (%A, week %V)")
	}
}

func BenchmarkMTSeed(b *testing.B) {
	seed := big.NewInt(0x1234567890abcdef)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		newMTFromBig(seed)
	}
}
