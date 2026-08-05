<p align="center"><img src="https://raw.githubusercontent.com/go-puppet/brand/main/social/go-puppet.png" alt="go-puppet/puppet" width="640"></p>

<h1 align="center">go-puppet / puppet</h1>
<p align="center"><strong>The Puppet language in pure Go — lexer, parser and Puppet::Pops AST, no cgo.</strong></p>

<p align="center">
  <a href="https://github.com/go-puppet/puppet/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/go-puppet/puppet/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://pkg.go.dev/github.com/go-puppet/puppet"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/go-puppet/puppet.svg"></a>
  <a href="https://go-puppet.github.io/docs/"><img alt="Docs" src="https://img.shields.io/badge/docs-mkdocs--material-FBBF24?style=flat-square"></a>
  <a href="https://github.com/go-puppet/puppet/blob/main/LICENSE"><img alt="License: BSD-3-Clause" src="https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square"></a>
  <img alt="Go 1.26.4+" src="https://img.shields.io/badge/go-1.26.4%2B-00ADD8?style=flat-square&logo=go&logoColor=white">
  <img alt="Coverage 100%" src="https://img.shields.io/badge/coverage-100%25-1a7f37?style=flat-square">
</p>

---

`go-puppet/puppet` is a **pure-Go (no cgo) implementation of the [Puppet](https://www.puppet.com/docs/puppet/latest/puppet_language.html)
language**. It parses Puppet 8 manifests into a faithful
[`Puppet::Pops`](https://github.com/puppetlabs/puppet/tree/main/lib/puppet/pops)-style
abstract-syntax model that an evaluator and catalog compiler build on. The type
system is delegated to **[go-pcore](https://github.com/go-pcore/pcore)**, data
binding to **[go-hiera](https://github.com/go-hiera/hiera)**, and facts to
**[go-facter](https://github.com/go-facter/facter)** — this repository never
reimplements them.

```go
cat, logs, _ := eval.EvalString(`
  class nginx (String $vhost = 'localhost', Integer[1,65535] $port = 80) {
    package { 'nginx': ensure => installed }
    -> service { 'nginx': ensure => running, require => Package['nginx'] }
  }
  include nginx
`)
fmt.Println(cat.JSON()) // Puppet catalog JSON: resources + containment/ordering edges
_ = logs                // notice/info/warning/err messages
```

Need a Terraform-style front-end instead? `hcl.Parse` reads an HCL2 manifest and
produces the *same* `ast.Program`, so it compiles to an identical catalog through
the same evaluator (see [The HCL2 front-end](#the-hcl2-front-end)).

## Status

**The lexer, parser, AST, evaluator and catalog compiler are complete** — plus
EPP/ERB templates, resource defaults/overrides/collectors, exported resources,
the plan/apply language, an extensive stdlib and the Terraform-style
[HCL2 front-end](#the-hcl2-front-end). All at **100 % test coverage** (enforced as
a CI gate, including every parse- and eval-error path), `gofmt` + `go vet` clean,
and green across all **six 64-bit Go targets** (amd64, arm64, riscv64, loong64,
ppc64le, s390x). The type system is delegated to **go-pcore**, data binding to
**go-hiera**, and facts to an injectable provider backed by **go-facter**.

## What it parses

The lexer and parser cover the Puppet 8 grammar:

| Area | Constructs |
|------|-----------|
| **Literals** | integers (dec/hex/octal), floats, single/double-quoted strings, `undef`, `default`, booleans, regexps, arrays, hashes |
| **Interpolation** | `"$var"`, `"${expr}"` (full expressions, method calls, function calls), `\uXXXX` / `\u{…}` escapes |
| **Heredocs** | `@(TAG)` / `@("TAG")`, `:syntax` tags, `|` indent margins, `-` trailing-newline chomp |
| **Data types** | `Integer[1,10]`, `Optional[String]`, `Struct[…]`, … (parsed as AST; evaluated via go-pcore) |
| **Operators** | arithmetic, comparison, `and`/`or`/`!`, `=~`/`!~`, `in`, `<<`, `+=`/`-=`, full precedence ladder |
| **Control flow** | `if`/`elsif`/`else`, `unless`, `case`, selectors `? { }` |
| **Resources** | declarations, defaults `Type { }`, overrides `Type[t] { }`, collectors `<| |>` / `<<| |>>`, virtual `@` / exported `@@` |
| **Definitions** | `class` (with `inherits`), `define`, `node`, `function` (with `>> ReturnType`) |
| **Calls** | function calls, statement-style calls (`include x`), method chains `.`, lambdas `\|params\| { }` |
| **Relationships** | `->`, `~>`, `<-`, `<~` chaining |

## What it evaluates

The evaluator compiles a manifest to a catalog:

| Area | Support |
|------|---------|
| **Scopes** | top / node / local scopes, immutable variable binding, `$::top`-scope and `$facts` |
| **Expressions** | arithmetic, comparison, boolean short-circuit, `=~`/`!~` (Regexp & Type), `in`, indexing/slicing, selectors, `if`/`unless`/`case` (value/Regexp/Type match) |
| **Data types** | data-type expressions evaluated through **go-pcore** (`assert_type`, `type`, `=~ Integer[1,10]`, typed parameters) |
| **Classes & defines** | `include`/`require`/`contain`, `class { }`, `inherits`, defined-type instantiation with `$title`/`$name`, automatic Hiera parameter data-binding |
| **Iteration** | `each`, `map`, `filter`, `reduce`, `with`, `slice` over arrays and hashes |
| **Built-in functions** | logging (`notice`/`info`/`warning`/`err`/`debug`/…), `fail`, `lookup` (via **go-hiera**), `assert_type`, `type`, and an extensive stdlib (string/array/hash/numeric, digests, encoding, path, time, TOML/JSON/PSON, `validate_*`, `pw_hash`, `shellwords`, …) |
| **Templates** | `epp`/`inline_epp` (EPP) and `template`/`inline_template` (ERB), through an injectable template loader |
| **Resource forms** | declarations, defaults `Type { }`, overrides `Type[t] { }`, virtual/collectors `<\| \|>`, exported `@@` + `<<\| \|>>` via an injectable exported-resource store |
| **Plans** | Bolt-style plan/apply language (`EvalPlanString`, `apply { }`) through an injectable plan executor |
| **Regex capture** | `$1`…`$n` match variables after `=~` |
| **Catalog** | resource graph with containment + relationship + metaparameter (`require`/`before`/`notify`/`subscribe`) edges, serialized to Puppet catalog JSON |

### The registry seam

`Evaluator.RegisterFunction(name, fn)` adds or overrides a function at runtime.
This is the seam **go-ruby-puppet** plugs into to contribute Ruby-defined custom
functions and types without this repository depending on the Ruby VM.

## Packages

| Package | Role |
|---------|------|
| `github.com/go-puppet/puppet` | façade: `Parse`, `ParseExpression` |
| `…/lexer` | tokenizer (`Lex`) |
| `…/parser` | recursive-descent parser (`Parse`, `ParseExpression`) |
| `…/ast` | Puppet::Pops-style node model + `Sexpr` renderer |
| `…/eval` | evaluator (`EvalString`, `EvalPlanString`, `Evaluator`, `RegisterFunction`, `WithFacts`/`WithHiera`/`WithNodeName`/`WithTemplateLoader`/`WithExportedStore`/`WithPlanExecutor`) |
| `…/catalog` | catalog model + Puppet catalog JSON |
| `…/hcl` | Terraform-style HCL2 front-end (`Parse`) → the same `ast.Program` |

## The HCL2 front-end

`hcl.Parse` reads a Terraform-style **HCL2** manifest and produces the very same
`ast.Program` the native Puppet (`.pp`) parser produces, so an HCL2 manifest
compiles to an **identical catalog** through the existing evaluator. The HCL2
grammar itself is parsed by the pure-Go
[`go-ruby-hcl2/hcl2`](https://github.com/go-ruby-hcl2/hcl2); this package only
translates its read-only expression AST into Puppet's model.

```go
prog, _ := hcl.Parse(`
  locals { mode = "0644" }
  resource "file" "app" {
    ensure  = "present"
    mode    = local.mode
    require = resource.package.app
  }
`)
// same *ast.Program as:  $mode = '0644'
//                        file { 'app': ensure => 'present', mode => $mode,
//                                      require => Package['app'] }
```

The v0.1 mapping covers `resource`/`locals` blocks, root-level assignments,
literals, templates, attribute/index traversal, unary/binary operators, and
resource-reference relationships. Constructs not yet mapped (function calls, the
`a ? b : c` conditional, `for` comprehensions, `%{…}` template directives and
unknown block types) return a clear `unsupported in HCL2 v0.1: …` error rather
than a fake stub.

## Roadmap

- **Shipped:** lexer, parser, AST, evaluator, catalog compiler, iteration,
  Hiera-backed `lookup()`, facts, and the function/type registry seam; **plus**
  EPP/ERB templates, resource **defaults**/**overrides**/**collectors**, exported
  resources, the plan/apply language, an extensive stdlib, `$1` regex-match
  capture, and the Terraform-style HCL2 front-end.
- **Still in progress (returns a clear error today — no fake stubs):** Pcore
  **type constructors** beyond the scalar core (`Timestamp()`, `SemVer()`, …), and
  the HCL2 front-end's v0.2 expression set (function calls, `a ? b : c`, `for`
  comprehensions, `%{…}` template directives, additional block types).

## Principles

- **Pure Go, zero cgo.** Cross-compiles anywhere; static binary by default.
- **Faithful to Puppet 8 / Pops.** Node kinds, grammar and precedence track the
  Puppet specification.
- **No reinvention.** Types → go-pcore, data → go-hiera, facts → go-facter.
- **100 % test coverage**, enforced in CI, including every parse- and eval-error
  branch, on all six 64-bit Go arches.

BSD-3-Clause.
