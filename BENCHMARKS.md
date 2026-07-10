<!--
Copyright (c) 2026, the go-puppet/puppet authors
SPDX-License-Identifier: BSD-3-Clause
-->

# Performance: go-puppet vs. reference MRI Puppet

The bar for this project is **at least as fast as the reference implementation**
(MRI Ruby Puppet). This document records the Go-side benchmarks that ship with
the code and the reproducible methodology for comparing them against the
reference `puppet` gem.

## Go benchmarks (shipped)

The hot paths are covered by Go `Benchmark*` functions in
[`eval/bench_test.go`](eval/bench_test.go), all driven on the same
representative real-world manifest (a parameterised `webserver` class: typed
parameters, a resource default, `each` iteration with interpolation, a selector,
a defined-type-style body and relationship chaining) plus an EPP template and a
stdlib-heavy manifest.

Run them with:

```sh
go test ./eval/ -run '^$' -bench . -benchmem
```

### Baseline (Apple M-series, `arm64-darwin`, Go 1.26.4)

| Benchmark                | Time/op | Allocs/op | What it measures                         |
|--------------------------|--------:|----------:|------------------------------------------|
| `BenchmarkParse`         | ~25 µs  | 249       | lex + parse the manifest to the AST      |
| `BenchmarkCompileCatalog`| ~42 µs  | 584       | parse **and** evaluate to a catalog      |
| `BenchmarkEvalOnly`      | ~15 µs  | 335       | evaluate a pre-parsed AST                |
| `BenchmarkEPPRender`     | ~26 µs  | 362       | compile + render an EPP template         |
| `BenchmarkStdlibFunctions`| ~14 µs | 279       | map/filter/merge/regsubst/flatten/…      |

So an end-to-end compile of a real module class is on the order of **40
microseconds**. (Numbers vary by host; regenerate the table with the command
above on the machine under test.)

### stdlib digest / crypt / random / time functions

The non-trivial functions added to complete the stdlib surface (cryptographic
digests, `pw_hash` crypt algorithms, deterministic seeded random, `strftime`)
have their own `Benchmark*` functions in
[`eval/stdlib_gap_bench_test.go`](eval/stdlib_gap_bench_test.go).

| Benchmark              | Time/op  | Allocs/op | What it measures                              |
|------------------------|---------:|----------:|-----------------------------------------------|
| `BenchmarkSHA256`      | ~250 ns  | 4         | `sha256()` hex digest of a short string       |
| `BenchmarkFqdnRand`    | ~7 µs    | 7         | MD5 seed → MRI-compatible MT19937 → `rand`    |
| `BenchmarkMTSeed`      | ~7 µs    | 5         | seed one MT19937 (init-by-array)              |
| `BenchmarkStrftime`    | ~0.4 µs  | 9         | expand a multi-directive `strftime` format    |
| `BenchmarkPwHashSHA512`| ~1.1 ms  | ~10 000   | SHA-512-crypt (**5000** hash rounds, `$6$`)   |
| `BenchmarkPwHashBcrypt`| ~50 ms   | 5         | bcrypt at cost 10 (2¹⁰ Blowfish key rounds)   |

The `pw_hash` timings are **deliberately dominated by the algorithm's work
factor** (crypt-SHA's 5000 rounds; bcrypt's exponential cost) — these mirror the
reference crypt(3)/OpenBSD costs exactly and must not be "optimised" away, since
the cost *is* the security property. The digest, random and `strftime`
benchmarks measure pure Go overhead and carry no such intrinsic floor.

**Reference methodology (MRI puppet, Tart VM).** In the same Debian Tart VM used
for the compiler benchmarks (Ruby 2.7–3.2 + `puppet` gem), time the equivalent
functions to compare like-for-like:

```sh
# digests / random / strftime (functions dispatched through the evaluator):
ruby -rpuppet -rbenchmark -e '
  Puppet.initialize_settings
  scope = Puppet::Parser::Scope.new(Puppet::Parser::Compiler.new(
    Puppet::Node.new("bench", :facts => Puppet::Node::Facts.new("bench",
      "networking" => {"fqdn" => "host.example.com"}))))
  n = 100_000
  puts "sha256:  %.1f ns/op" % (Benchmark.realtime { n.times { scope.call_function("sha256", ["x"]) } }/n*1e9)
  puts "fqdn_rand: %.1f ns/op" % (Benchmark.realtime { n.times { scope.call_function("fqdn_rand", [30]) } }/n*1e9)
'
# pw_hash correctness/parity is verified byte-for-byte against `openssl passwd`
# (-6/-5/-1) and the canonical OpenBSD bcrypt vectors in the differential tests,
# so its cost is fixed by the algorithm, not the implementation.
```

Because MRI's `sha256`/`fqdn_rand` call into C (`Digest`, `Random`) while
go-puppet stays pure Go, the honest comparison is on the *dispatch + algorithm*
path shown above; record `MRI_ns_per_op / Go_ns_per_op` alongside the table.

## Reference comparison (MRI Puppet)

The reference is the Ruby `puppet` gem's parser + compiler. The puppet gem
supports MRI Ruby 2.7–3.2 and does **not** run on the Ruby 4.x currently on the
dev host, so the reference measurement is taken in a Tart VM with a compatible
Ruby. This keeps the comparison honest and reproducible rather than guessed.

### Harness

Provision (per the fleet convention, use a Debian Tart VM):

```sh
tart clone debian puppet-bench && tart run puppet-bench &
# inside the VM:
sudo apt-get update && sudo apt-get install -y ruby ruby-dev build-essential
sudo gem install puppet -v '~> 8.0'   # MRI-compatible Ruby (rbenv 3.2 if needed)
```

Use the **same** manifest for both sides. Save the `webserver` manifest from
`benchManifest` in `eval/bench_test.go` as `webserver.pp`.

**Reference (MRI) — parse+compile timing.** Puppet has a large fixed
interpreter/boot cost, so measure the compiler in-process to compare the
*compile* work rather than process startup:

```sh
# parse-only, N iterations, wall-clock (Ruby):
ruby -rpuppet -rbenchmark -e '
  Puppet.initialize_settings
  src = File.read("webserver.pp")
  n = 1000
  t = Benchmark.realtime { n.times { Puppet::Pops::Parser::EvaluatingParser.new.parse_string(src) } }
  puts "parse: %.1f us/op" % (t/n*1e6)
'
```

For a full catalog compile the supported path is `puppet catalog compile` /
`puppet apply --catalog` on an agent-configured node; the per-run process boot
(~1–2 s) dominates, which is itself the story: a long-lived go-puppet process
compiles the same manifest in tens of microseconds.

### Recording results

Append the VM's Ruby/puppet versions and the measured `us/op` next to the Go
numbers, and compute the ratio `MRI_us_per_op / Go_us_per_op`. The commit that
adds real reference numbers should update the table above with a third column.

## Measured results — real hardware (2026-07-10)

The Go `Parse` benchmark and the MRI reference (Puppet 8.10.0's
`Puppet::Pops::Parser::EvaluatingParser#parse_string`) were run on the **same
host** over the **identical** `webserver.pp` manifest (the `benchManifest`
above), in-process under `Benchmark.realtime` after a warm-up — so the number is
the *parse* work, not Ruby interpreter boot. Measured on two real, non-x86
arches.

| Arch | Host | CPU | Go | Ruby (MRI) | puppet |
|------|------|-----|----|-----------|--------|
| `s390x` | LinuxONE | IBM z15 (8561), 2 vCPU | go1.26.4 | 3.2.3 | 8.10.0 |
| `riscv64` | cfarm95 (GCC farm) | SpacemiT X60 (rv64gcv), 8 core | go1.26.4 | 3.3.8 | 8.10.0 |

| Arch | Go `Parse` | MRI `EvaluatingParser#parse_string` | ratio (MRI ÷ Go) |
|------|-----------:|------------------------------------:|-----------------:|
| `s390x`   | 62 µs  | 1 214 µs  | **19.5× faster** |
| `riscv64` | 599 µs | 23 418 µs | **39.1× faster** |

For context, on the same hosts the Go end-to-end catalog compile
(`BenchmarkCompileCatalog`) is 107 µs (`s390x`) / 1 480 µs (`riscv64`). The
reference full-catalog compile has no comparable in-process single call — the
supported path (`puppet apply` / `puppet catalog compile`) pays a ~1–2 s process
boot per run — so the **parse** number above is the honest apples-to-apples
figure. On both real architectures the pure-Go parser is well over an order of
magnitude faster than the reference, so the "≥ reference" rule is satisfied.

### Status

- Go benchmarks: **shipped and green** (numbers above).
- MRI reference: **measured on real `s390x` and `riscv64` hardware** (table
  above), Puppet 8.10.0 on Ruby 3.2.3 / 3.3.8. The Go implementation wins by
  19–39× on parse because it runs the compiler as native code with no per-call
  Pops AST/object allocation.
