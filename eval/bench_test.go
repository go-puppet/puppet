// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"testing"

	"github.com/go-puppet/puppet/parser"
)

// benchManifest is a representative real-world manifest: a parameterised class
// with data types, a defined type, iteration, conditionals, string
// interpolation, a resource default, a selector and relationship chaining — the
// shape of an ordinary Puppet module class.
const benchManifest = `
class webserver (
  String            $package    = 'nginx',
  Integer[1,65535]  $port       = 80,
  Array[String]     $vhosts     = ['a.example', 'b.example'],
  Enum['running','stopped'] $ensure = 'running',
) {
  File { owner => 'root', group => 'root', mode => '0644' }

  package { $package:
    ensure => installed,
  }

  $vhosts.each |$i, $vhost| {
    file { "/etc/nginx/sites/${vhost}.conf":
      ensure  => file,
      content => "server { listen ${port}; server_name ${vhost}; }",
      require => Package[$package],
    }
  }

  $svc = $ensure ? {
    'running' => 'running',
    default   => 'stopped',
  }

  service { $package:
    ensure    => $svc,
    subscribe => File["/etc/nginx/sites/${vhosts[0]}.conf"],
  }

  Package[$package] -> Service[$package]
}

include webserver
`

func BenchmarkParse(b *testing.B) {
	src := benchManifest
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parser.Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompileCatalog(b *testing.B) {
	src := benchManifest
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := EvalString(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvalOnly(b *testing.B) {
	prog, err := parser.Parse(benchManifest)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := New()
		if _, err := e.EvalProgram(prog); err != nil {
			b.Fatal(err)
		}
	}
}

const benchEPP = `<%- | String $title, Array[String] $items, Integer $port | -%>
# <%= $title %> (port <%= $port %>)
<% $items.each |$item| { -%>
  - <%= capitalize($item) %>
<% } -%>
<% if $port == 80 { -%>
default-http
<% } -%>
`

func BenchmarkEPPRender(b *testing.B) {
	src := `notice(inline_epp($t, {'title' => 'Report', 'items' => ['one','two','three'], 'port' => 80}))`
	prog, err := parser.Parse(src)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := New()
		e.top.setForce("t", benchEPP)
		if _, err := e.EvalProgram(prog); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStdlibFunctions(b *testing.B) {
	src := `
$a = [1,2,3,4,5,6,7,8,9,10]
$h = {'one' => 1, 'two' => 2, 'three' => 3}
$x1 = $a.map |$n| { $n * 2 }.filter |$n| { $n > 5 }
$x2 = merge($h, {'four' => 4})
$x3 = regsubst('a.b.c.d', '\.', '/', 'G')
$x4 = join(keys($h), ',')
$x5 = flatten([[1,2],[3,[4,5]]])
$x6 = versioncmp('1.2.10', '1.2.9')
`
	prog, err := parser.Parse(src)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := New()
		if _, err := e.EvalProgram(prog); err != nil {
			b.Fatal(err)
		}
	}
}
