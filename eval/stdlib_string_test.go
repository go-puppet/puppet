// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

// evalOut evaluates `notice(<expr>)` and returns the rendered message.
func evalOut(t *testing.T, expr string, opts ...Option) string {
	t.Helper()
	return firstLog(t, "notice("+expr+")", opts...)
}

func TestStdlibStringFns(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`lstrip("  hi ")`, "hi "},
		{`rstrip(" hi  ")`, " hi"},
		{`strip("  hi  ")`, "hi"},
		{`upcase("aB")`, "AB"},
		{`upcase(["a","b"])`, `["A", "B"]`},
		{`downcase("Ab")`, "ab"},
		{`capitalize("hello")`, "Hello"},
		{`swapcase("Hello")`, "hELLO"},
		{`camelcase("foo_bar")`, "FooBar"},
		{`chomp("line\n")`, "line"},
		{`chop("abc")`, "ab"},
		{`chop("ab\r\n")`, "ab"},
		{`trim("  x  ")`, "x"},
		{`upcase_first("foo")`, "Foo"},
		{`squeeze("aaabbbccc")`, "abc"},
		{`squeeze("aaabbb", "a")`, "abbb"},
		{`str2num("42")`, "42"},
		{`str2num("4.5")`, "4.5"},
		{`strlen("hello")`, "5"},
		{`bool2num("true")`, "1"},
		{`bool2num("no")`, "0"},
		{`bool2num(true)`, "1"},
		{`bool2num(false)`, "0"},
		{`num2bool(5)`, "true"},
		{`num2bool(0)`, "false"},
		{`num2bool(2.5)`, "true"},
		{`num2bool("3")`, "true"},
		{`str2bool("yes")`, "true"},
		{`str2bool("f")`, "false"},
		{`str2bool(true)`, "true"},
		{`bool2str(true)`, "true"},
		{`bool2str(false, "on", "off")`, "off"},
		{`bool2str(true, "on", "off")`, "on"},
		{`uriescape("a b/c")`, "a%20b%2Fc"},
		{`shell_escape("plain")`, "plain"},
		{`shell_escape("a b")`, `a\ b`},
		{`shell_escape("")`, "''"},
		{`start_with("hello", "he")`, "true"},
		{`start_with("hello", ["x","he"])`, "true"},
		{`start_with("hello", "no")`, "false"},
		{`end_with("hello", "lo")`, "true"},
		{`end_with("hello", ["no","lo"])`, "true"},
		{`versioncmp("1.2.10", "1.2.9")`, "1"},
		{`versioncmp("1.0", "1.0")`, "0"},
		{`versioncmp("2.0", "2.0.1")`, "-1"},
		{`versioncmp("2.0", "10.0")`, "-1"},
		{`regsubst("a.b.c", "\\.", "-", "G")`, "a-b-c"},
		{`regsubst("a.b.c", "\\.", "-")`, "a-b.c"},
		{`regsubst("John Smith", "(\\w+) (\\w+)", "\\2, \\1")`, "Smith, John"},
		{`regsubst(["a.b","c.d"], "\\.", "_", "G")`, `["a_b", "c_d"]`},
		{`match("abc123", "([a-z]+)(\\d+)")`, `["abc123", "abc", "123"]`},
		{`match("xyz", "\\d+")`, ""},
		{`match(["a1","b2"], "([a-z])(\\d)")`, `[["a1", "a", "1"], ["b2", "b", "2"]]`},
		{`str2resource("File[/tmp/x]")`, "File[/tmp/x]"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestStdlibStringErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(lstrip())`, "one argument"},
		{`notice(lstrip(5))`, "String or Array"},
		{`notice(upcase([5]))`, "String or Array"},
		{`notice(squeeze())`, "wrong number"},
		{`notice(squeeze(5))`, "must be a String"},
		{`notice(squeeze("a", 5))`, "must be a String"},
		{`notice(str2bool())`, "wrong number"},
		{`notice(str2bool(5))`, "must be a String"},
		{`notice(str2bool("maybe"))`, "unknown type of boolean"},
		{`notice(bool2str())`, "wrong number"},
		{`notice(bool2str(5))`, "expects a Boolean"},
		{`notice(num2bool())`, "wrong number"},
		{`notice(num2bool("x"))`, "unable to parse"},
		{`notice(num2bool([]))`, "expects a Numeric"},
		{`notice(bool2num())`, "wrong number"},
		{`notice(bool2num([]))`, "expects a Boolean or String"},
		{`notice(str2num())`, "wrong number"},
		{`notice(str2num(5))`, "must be a String"},
		{`notice(str2num("xx"))`, "cannot convert"},
		{`notice(strlen())`, "wrong number"},
		{`notice(strlen(5))`, "must be a String"},
		{`notice(shell_escape())`, "wrong number"},
		{`notice(start_with("a"))`, "wrong number"},
		{`notice(start_with(5, "a"))`, "must be a String"},
		{`notice(end_with("a"))`, "wrong number"},
		{`notice(end_with(5, "a"))`, "must be a String"},
		{`notice(versioncmp("1"))`, "wrong number"},
		{`notice(versioncmp(5, "1"))`, "must be a String"},
		{`notice(versioncmp("1", 5))`, "must be a String"},
		{`notice(regsubst("a"))`, "wrong number"},
		{`notice(regsubst("a", 5, "b"))`, "must be a String or Regexp"},
		{`notice(regsubst("a", "(", "b"))`, "invalid pattern"},
		{`notice(regsubst("a", "x", 5))`, "must be a String"},
		{`notice(regsubst(5, "x", "y"))`, "String or Array"},
		{`notice(match("a"))`, "wrong number"},
		{`notice(match(5, "x"))`, "must be a String"},
		{`notice(match("a", 5))`, "must be a String or Regexp"},
		{`notice(match("a", "("))`, "invalid pattern"},
		{`notice(str2resource())`, "wrong number"},
		{`notice(str2resource(5))`, "must be a String"},
		{`notice(str2resource("nope"))`, "expected Type[title]"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestStdlibRegsubstRegexpArg(t *testing.T) {
	if got := evalOut(t, `regsubst("a1b2", /\d/, "#", "G")`); got != "a#b#" {
		t.Errorf("got %q", got)
	}
	if got := evalOut(t, `match("a1", /([a-z])(\d)/)`); got != `["a1", "a", "1"]` {
		t.Errorf("got %q", got)
	}
}

func TestStdlibDataFns(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`parsejson("{\"a\":1,\"b\":[2,3]}")`, `{"a" => 1, "b" => [2, 3]}`},
		{`parsejson("[1,2.5,true,null,\"s\"]")`, `[1, 2.5, true, , "s"]`},
		{`parsejson("nope", "def")`, "def"},
		{`to_json({"a"=>1,"b"=>[2,3]})`, `{"a":1,"b":[2,3]}`},
		{`to_json([1,"x",true,undef])`, `[1,"x",true,null]`},
		{`to_json_pretty({"a"=>1})`, "{\n  \"a\": 1\n}"},
		{`parseyaml("a: 1\nb: two\n")`, `{"a" => 1, "b" => "two"}`},
		{`parseyaml("- 1\n- 2\n")`, "[1, 2]"},
		{`parseyaml("\t- x", "fallback")`, "fallback"},
		{`to_yaml({"k"=>"v"})`, "---\nk: v"},
	}
	for _, tc := range cases {
		got := evalOut(t, tc.expr)
		if strings.TrimSpace(got) != strings.TrimSpace(tc.want) {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestStdlibDataErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(parsejson())`, "wrong number"},
		{`notice(parsejson(5))`, "must be a String"},
		{`notice(parsejson("{bad"))`, "parsejson"},
		{`notice(parseyaml())`, "wrong number"},
		{`notice(parseyaml(5))`, "must be a String"},
		{`notice(to_json())`, "wrong number"},
		{`notice(to_json_pretty())`, "wrong number"},
		{`notice(to_yaml())`, "wrong number"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestStdlibLoadJSONYAML(t *testing.T) {
	loader := mapLoader{
		"d/a.json":   `{"k": 1}`,
		"d/a.yaml":   "k: 2\n",
		"d/bad.json": "{oops",
	}
	if got := evalOut(t, `loadjson("d/a.json")`, WithTemplateLoader(loader)); got != `{"k" => 1}` {
		t.Errorf("loadjson got %q", got)
	}
	if got := evalOut(t, `loadyaml("d/a.yaml")`, WithTemplateLoader(loader)); got != `{"k" => 2}` {
		t.Errorf("loadyaml got %q", got)
	}
	if got := evalOut(t, `loadjson("d/missing.json", "def")`, WithTemplateLoader(loader)); got != "def" {
		t.Errorf("loadjson default got %q", got)
	}
	if got := evalOut(t, `loadjson("d/bad.json", "def")`, WithTemplateLoader(loader)); got != "def" {
		t.Errorf("loadjson bad-parse default got %q", got)
	}
	// no default, missing loader -> error
	if err := evalErr(t, `notice(loadjson("x"))`); !strings.Contains(err, "no template loader") {
		t.Errorf("loadjson no-loader got %q", err)
	}
	// no default, parse error -> error
	if err := evalErr(t, `notice(loadjson("d/bad.json"))`, WithTemplateLoader(loader)); !strings.Contains(err, "parsejson") {
		t.Errorf("loadjson bad-parse got %q", err)
	}
	if err := evalErr(t, `notice(loadjson())`); !strings.Contains(err, "wrong number") {
		t.Errorf("loadjson arity got %q", err)
	}
	if err := evalErr(t, `notice(loadjson(5))`, WithTemplateLoader(loader)); !strings.Contains(err, "must be a String") {
		t.Errorf("loadjson type got %q", err)
	}
}
