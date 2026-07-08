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
prog, _ := puppet.Parse(`
  class nginx (String $vhost = 'localhost', Integer[1,65535] $port = 80) {
    package { 'nginx': ensure => installed }
    -> service { 'nginx': ensure => running, enable => true }
  }
`)
// prog is an *ast.Program you can walk, transform, or hand to the evaluator.
```

## Status

**Milestone 1 — lexer + parser + AST — complete**, at **100 % test coverage**
(enforced as a CI gate, including every parse-error path), `gofmt` + `go vet`
clean, and green across all **six 64-bit Go targets** (amd64, arm64, riscv64,
loong64, ppc64le, s390x). The evaluator and catalog compiler (Milestone 2) build
on this model.

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

## Packages

| Package | Role |
|---------|------|
| `github.com/go-puppet/puppet` | façade: `Parse`, `ParseExpression` |
| `…/lexer` | tokenizer (`Lex`) |
| `…/parser` | recursive-descent parser (`Parse`, `ParseExpression`) |
| `…/ast` | Puppet::Pops-style node model + `Sexpr` renderer |

## Roadmap

- **v0.1 (this release):** lexer, parser, AST.
- **v0.2:** evaluator (scopes, class/define instantiation, iteration, built-in
  functions, `lookup()` via go-hiera), catalog compiler (resource graph +
  containment/relationship edges, Puppet catalog JSON), and a
  function/type **registry seam** that **go-ruby-puppet** populates with
  Ruby-defined custom functions and types.
- **Staged (clearly not yet implemented):** EPP/ERB templates, exported
  resources / PuppetDB, the full stdlib module, and the plan/apply language.

## Principles

- **Pure Go, zero cgo.** Cross-compiles anywhere; static binary by default.
- **Faithful to Puppet 8 / Pops.** Node kinds, grammar and precedence track the
  Puppet specification.
- **No reinvention.** Types → go-pcore, data → go-hiera, facts → go-facter.
- **100 % test coverage**, enforced in CI, including every parse-error branch,
  on all six 64-bit Go arches.

BSD-3-Clause.
