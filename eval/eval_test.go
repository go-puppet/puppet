// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-hiera/hiera"
)

// lastLog evaluates src and returns the message of the last emitted log entry.
func lastLog(t *testing.T, src string, opts ...Option) string {
	t.Helper()
	_, logs, err := EvalString(src, opts...)
	if err != nil {
		t.Fatalf("EvalString(%q) error: %v", src, err)
	}
	if len(logs) == 0 {
		t.Fatalf("EvalString(%q) produced no log", src)
	}
	return logs[len(logs)-1].Message
}

func evalErr(t *testing.T, src string, opts ...Option) string {
	t.Helper()
	_, _, err := EvalString(src, opts...)
	if err == nil {
		t.Fatalf("EvalString(%q): expected error, got nil", src)
	}
	return err.Error()
}

func TestExpressions(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(1 + 2)`, "3"},
		{`notice(2.5 * 2)`, "5"},
		{`notice(7 % 3)`, "1"},
		{`notice(10 / 4)`, "2"},
		{`notice(10.0 / 4)`, "2.5"},
		{`notice(1 + 2.0)`, "3"},
		{`notice(9 - 4)`, "5"},
		{`notice([1,2] + [3])`, "[1, 2, 3]"},
		{`notice(({'a'=>1} + {'b'=>2})['b'])`, "2"},
		{`notice([1] << 2)`, "[1, 2]"},
		{`notice(-5)`, "-5"},
		{`notice(-2.5)`, "-2.5"},
		{`notice(!true)`, "false"},
		{`notice(1 < 2)`, "true"},
		{`notice("b" > "a")`, "true"},
		{`notice(1 == 1.0)`, "true"},
		{`notice(3 != 2)`, "true"},
		{`notice(2 <= 2)`, "true"},
		{`notice(3 >= 4)`, "false"},
		{`notice(true and false)`, "false"},
		{`notice(false or true)`, "true"},
		{`notice(false and true)`, "false"},
		{`notice(true or false)`, "true"},
		{`notice('foo' =~ /o+/)`, "true"},
		{`notice('foo' !~ /z/)`, "true"},
		{`notice(5 =~ Integer)`, "true"},
		{`notice(2 in [1,2,3])`, "true"},
		{`notice('x' in {'x'=>1})`, "true"},
		{`notice('ell' in 'hello')`, "true"},
		{`notice(9 in [1,2])`, "false"},
		{`notice([10,20,30][1])`, "20"},
		{`notice([10,20,30][-1])`, "30"},
		{`notice([10,20,30][5])`, ""},
		{`notice([1,2,3,4][1,2])`, "[2, 3]"},
		{`notice({'a'=>1}['a'])`, "1"},
		{`notice({'a'=>1}['z'])`, ""},
		{`notice('hello'[1])`, "e"},
		{`notice('hello'[-1])`, "o"},
		{`notice('hello'[9])`, ""},
		{`notice($undefined)`, ""},
		{`$x = 5
notice($x)`, "5"},
		{`notice(2 ? {1=>'a', 2=>'b', default=>'c'})`, "b"},
		{`notice(9 ? {1=>'a', default=>'z'})`, "z"},
		{`if 1<2 { notice('y') } else { notice('n') }`, "y"},
		{`if 1>2 { notice('y') } else { notice('n') }`, "n"},
		{`unless false { notice('u') }`, "u"},
		{`unless true { notice('a') } else { notice('b') }`, "b"},
		{`case 2 { 1: {notice('a')} 2,3: {notice('b')} default:{notice('c')} }`, "b"},
		{`case 5 { Integer: {notice('int')} }`, "int"},
		{`case 'foo' { /o/: {notice('m')} }`, "m"},
		{`case 9 { default: {notice('d')} }`, "d"},
		// heredoc
		{"$h = @(END)\n  hi there\n  | END\nnotice($h)", "hi there\n"},
		// interpolation
		{`notice("a${1+1}b")`, "a2b"},
		{`$w = 'world'
notice("hi ${w}")`, "hi world"},
	}
	for _, tc := range cases {
		if got := lastLog(t, tc.src); got != tc.want {
			t.Errorf("%q\n  got  %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

func TestBuiltins(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(upcase('abc'))`, "ABC"},
		{`notice(downcase('ABC'))`, "abc"},
		{`notice(capitalize('hELLo'))`, "Hello"},
		{`notice(capitalize(''))`, ""},
		{`notice(strip('  x  '))`, "x"},
		{`notice(length('héllo'))`, "5"},
		{`notice(size([1,2]))`, "2"},
		{`notice(size({'a'=>1,'b'=>2}))`, "2"},
		{`notice(empty(''))`, "true"},
		{`notice(empty([1]))`, "false"},
		{`notice(empty(undef))`, "true"},
		{`notice(split('a,b,c', ',')[1])`, "b"},
		{`notice(join(split('a1b2c', /[0-9]/), '-'))`, "a-b-c"},
		{`notice(join([1,2,3], '-'))`, "1-2-3"},
		{`notice(join([1,2]))`, "12"},
		{`notice(sprintf('%s=%d', 'x', 5))`, "x=5"},
		{`notice(keys({'b'=>2,'a'=>1}))`, `["a", "b"]`},
		{`notice(values({'b'=>2,'a'=>1}))`, "[1, 2]"},
		{`notice(merge({'a'=>1},{'a'=>2,'b'=>3})['a'])`, "2"},
		{`notice(reverse([1,2,3]))`, "[3, 2, 1]"},
		{`notice(reverse('abc'))`, "cba"},
		{`notice(abs(-5))`, "5"},
		{`notice(abs(-2.5))`, "2.5"},
		{`notice(abs(4))`, "4"},
		{`notice(min(3,1,2))`, "1"},
		{`notice(max(3,1,2))`, "3"},
		{`notice(assert_type(Integer, 5))`, "5"},
		{`notice(Integer('42') + 1)`, "43"},
		{`notice(Integer(4.9))`, "4"},
		{`notice(Float('1.5'))`, "1.5"},
		{`notice(Float(3))`, "3"},
		{`notice(String(5))`, "5"},
		{`notice(Boolean('yes'))`, "true"},
		{`notice(Boolean('false'))`, "false"},
		{`notice('abc'.upcase)`, "ABC"},
		{`notice([3,1,2].reverse)`, "[2, 1, 3]"},
		// iteration
		{`notice([1,2,3].map |$x| { $x*$x })`, "[1, 4, 9]"},
		{`notice([1,2,3,4].filter |$x| { $x % 2 == 0 })`, "[2, 4]"},
		{`notice([1,2,3].reduce |$a,$b| { $a+$b })`, "6"},
		{`notice([1,2,3].reduce(10) |$a,$b| { $a+$b })`, "16"},
		{`notice([].reduce |$a,$b| { $a })`, ""},
		{`notice(with(5) |$x| { $x+1 })`, "6"},
		{`notice([1,2,3,4].slice(2))`, "[[1, 2], [3, 4]]"},
		{`notice([].slice(2))`, "[]"},
		{`$r = [1,2,3].each |$x| { notice($x) }
notice($r)`, "[1, 2, 3]"},
		{`['x','y'].each |$i,$v| { notice("${i}:${v}") }`, "1:y"},
		{`{'a'=>1,'b'=>2}.each |$k,$v| { notice("${k}=${v}") }`, "b=2"},
		{`notice({'a'=>1,'b'=>2}.filter |$k,$v| { $v > 1 })`, `{"b" => 2}`},
		{`notice({'a'=>1}.map |$pair| { $pair[0] })`, `["a"]`},
		{`{'a'=>1}.each |$pair| { notice($pair) }`, `["a", 1]`},
		{`[1,2,3,4].slice(2) |$c| { notice($c) }`, "[3, 4]"},
		{`notice(type(5))`, "Integer[5, 5]"},
	}
	for _, tc := range cases {
		if got := lastLog(t, tc.src); got != tc.want {
			t.Errorf("%q\n  got  %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

func TestUserFunction(t *testing.T) {
	if got := lastLog(t, `function double($x) { $x * 2 }
notice(double(21))`); got != "42" {
		t.Errorf("got %q", got)
	}
	if got := lastLog(t, `function typed(Integer $x) >> Integer { $x + 1 }
notice(typed(4))`); got != "5" {
		t.Errorf("got %q", got)
	}
}

func TestLoggingLevels(t *testing.T) {
	_, logs, err := EvalString(`notice('n'); info('i'); warning('w'); err('e'); debug('d'); alert('a'); crit('c'); emerg('m')`)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 8 {
		t.Fatalf("want 8 logs, got %d", len(logs))
	}
	if logs[0].Level != "notice" || logs[3].Level != "err" {
		t.Errorf("levels wrong: %+v", logs)
	}
}

func TestCatalog(t *testing.T) {
	cat, _, err := EvalString(`
file { '/tmp/a':
  ensure => present,
  mode   => '0644',
  tag    => ['x', 'y'],
}
@user { 'bob': }
@@nagios_service { 'svc': }
package { ['vim', 'git']: ensure => installed }
notify { 'a': } notify { 'b': }
Package['vim'] -> Service['nginx']
Service['nginx'] ~> Notify['a']
`)
	if err != nil {
		t.Fatal(err)
	}
	js := cat.JSON()
	for _, want := range []string{
		`"type":"File","title":"/tmp/a"`,
		`"tags":["x","y"]`,
		`"type":"User","title":"bob"`,
		`"type":"Nagios_service","title":"svc","exported":true`,
		`"type":"Package","title":"vim"`,
		`"type":"Package","title":"git"`,
		`{"source":"Package[vim]","target":"Service[nginx]"}`,
		`{"source":"Service[nginx]","target":"Notify[a]"}`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("catalog missing %s\n%s", want, js)
		}
	}
	// virtual flag
	if r, ok := cat.Get("User[bob]"); !ok || !r.Virtual {
		t.Error("user bob should be virtual")
	}
}

func TestClassesAndDefines(t *testing.T) {
	cat, logs, err := EvalString(`
class base { notice('base') }
class web(String $vhost = 'localhost') inherits base {
  file { "/etc/${vhost}": ensure => file }
}
define site($root) {
  file { $root: ensure => directory }
  notice("site ${title} at ${root}")
}
include web
site { 'blog': root => '/srv/blog' }
class { 'web': }
`)
	if err != nil {
		t.Fatal(err)
	}
	js := cat.JSON()
	for _, want := range []string{
		`"type":"Class","title":"Base"`,
		`"type":"Class","title":"Web"`,
		`"type":"File","title":"/etc/localhost"`,
		`"type":"Site","title":"blog"`,
		`"type":"File","title":"/srv/blog"`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("missing %s\n%s", want, js)
		}
	}
	var found bool
	for _, l := range logs {
		if l.Message == "site blog at /srv/blog" {
			found = true
		}
	}
	if !found {
		t.Errorf("define log missing: %+v", logs)
	}
}

func TestNodes(t *testing.T) {
	cat, _, err := EvalString(`
node 'web1', /^db/ { include role_web }
node default { include role_default }
class role_web { }
class role_default { }
`, WithNodeName("web1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Get("Class[Role_web]"); !ok {
		t.Error("role_web not declared for web1")
	}

	cat2, _, err := EvalString(`
node /^db/ { include dbrole }
node default { include defrole }
class dbrole {} class defrole {}
`, WithNodeName("db99"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat2.Get("Class[Dbrole]"); !ok {
		t.Error("dbrole not matched by regexp")
	}

	cat3, _, err := EvalString(`node default { include only }
class only {}`, WithNodeName("whatever"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat3.Get("Class[Only]"); !ok {
		t.Error("default node not applied")
	}
}

func TestFacts(t *testing.T) {
	facts := MapFacts{
		"os":       map[string]any{"family": "Debian", "name": "Ubuntu"},
		"hostname": "web1",
	}
	if got := lastLog(t, `notice($facts['os']['family'])`, WithFacts(facts)); got != "Debian" {
		t.Errorf("facts hash got %q", got)
	}
	if got := lastLog(t, `notice($hostname)`, WithFacts(facts)); got != "web1" {
		t.Errorf("top fact got %q", got)
	}
}

func TestHiera(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hiera.yaml"), `version: 5
defaults:
  datadir: data
  data_hash: yaml_data
hierarchy:
  - name: Common
    path: common.yaml
`)
	os.MkdirAll(filepath.Join(dir, "data"), 0o755)
	writeFile(t, filepath.Join(dir, "data", "common.yaml"), `message: from hiera
web::port: 8080
`)
	h, err := hiera.Load(filepath.Join(dir, "hiera.yaml"), hiera.MapScope{})
	if err != nil {
		t.Fatalf("hiera.Load: %v", err)
	}
	if got := lastLog(t, `notice(lookup('message'))`, WithHiera(h)); got != "from hiera" {
		t.Errorf("lookup got %q", got)
	}
	if got := lastLog(t, `notice(lookup('absent', 'fallback'))`, WithHiera(h)); got != "fallback" {
		t.Errorf("lookup default got %q", got)
	}
	// automatic class parameter data binding
	cat, logs, err := EvalString(`class web(Integer $port) { notice("port ${port}") }
include web`, WithHiera(h))
	if err != nil {
		t.Fatalf("class auto-bind: %v", err)
	}
	_ = cat
	if logs[len(logs)-1].Message != "port 8080" {
		t.Errorf("auto-bound port wrong: %+v", logs)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFacterProvider(t *testing.T) {
	p := FacterFacts()
	if p.Facts() == nil {
		t.Error("Facts() nil")
	}
	// kernel is present on every supported OS.
	if _, ok := p.Fact("kernel"); !ok {
		t.Error("expected a 'kernel' fact")
	}
	if _, ok := p.Fact("no_such_fact_xyz"); ok {
		t.Error("unexpected fact")
	}
}
