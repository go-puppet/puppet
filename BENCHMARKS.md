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

### Status

- Go benchmarks: **shipped and green** (numbers above).
- MRI reference: methodology documented; run in a Tart VM with Ruby 2.7–3.2 +
  `puppet` gem (the dev host's Ruby 4.0.5 is unsupported by the gem). On every
  microbenchmark the Go implementation is expected to win by a wide margin
  because it avoids the Ruby interpreter boot and runs the compiler as native
  code; the numbers above are the reproducible Go baseline to compare against.
