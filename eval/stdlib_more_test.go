// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

func TestStdlibArrayFns(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`flatten([1,[2,[3,4]],5])`, "[1, 2, 3, 4, 5]"},
		{`unique([1,1,2,3,3,2])`, "[1, 2, 3]"},
		{`unique("aabbcc")`, "abc"},
		{`concat([1,2],[3,4],5)`, "[1, 2, 3, 4, 5]"},
		{`member([1,2,3], 2)`, "true"},
		{`member([1,2,3], [1,2])`, "true"},
		{`member([1,2,3], [1,9])`, "false"},
		{`member([1,2,3], 9)`, "false"},
		{`count([1,undef,2,undef])`, "2"},
		{`count([1,2,2,3], 2)`, "2"},
		{`index([10,20,30], 20)`, "1"},
		{`index(["a","b"], "z")`, ""},
		{`delete([1,2,3,2], 2)`, "[1, 3]"},
		{`delete({"a"=>1,"b"=>2}, "a")`, `{"b" => 2}`},
		{`delete("foobar", "o")`, "fbar"},
		{`delete([1,2,3], [1,3])`, "[2]"},
		{`delete_at([1,2,3], 1)`, "[1, 3]"},
		{`delete_at([1,2,3], -1)`, "[1, 2]"},
		{`delete_at([1,2,3], 9)`, "[1, 2, 3]"},
		{`delete_undef_values([1,undef,2])`, "[1, 2]"},
		{`compact([1,undef,2])`, "[1, 2]"},
		{`delete_undef_values({"a"=>1,"b"=>undef})`, `{"a" => 1}`},
		{`difference([1,2,3],[2])`, "[1, 3]"},
		{`intersection([1,2,3],[2,3,4])`, "[2, 3]"},
		{`union([1,2],[2,3],[3,4])`, "[1, 2, 3, 4]"},
		{`range(1,5)`, "[1, 2, 3, 4, 5]"},
		{`range(1,10,2)`, "[1, 3, 5, 7, 9]"},
		{`range(5,1,-2)`, "[5, 3, 1]"},
		{`range("a","c")`, `["a", "b", "c"]`},
		{`sort([3,1,2])`, "[1, 2, 3]"},
		{`sort("dbca")`, "abcd"},
		{`prefix(["a","b"], "x")`, `["xa", "xb"]`},
		{`suffix(["a","b"], "z")`, `["az", "bz"]`},
		{`grep(["aaa","bbb","abc"], "a")`, `["aaa", "abc"]`},
		{`reject(["aaa","bbb","abc"], "b")`, `["aaa"]`},
		{`values_at([10,20,30,40], [0,2])`, "[10, 30]"},
		{`values_at([10,20,30], 1)`, "[20]"},
		{`zip([1,2],[3,4])`, "[[1, 3], [2, 4]]"},
		{`zip([1,2],[3])`, "[[1, 3], [2, ]]"},
		{`zip([1],[2],true)`, "[1, 2]"},
		{`has_key({"a"=>1}, "a")`, "true"},
		{`has_key({"a"=>1}, "b")`, "false"},
		{`join_keys_to_values({"a"=>1,"b"=>2}, "=")`, `["a=1", "b=2"]`},
		{`hash([["a",1],["b",2]])`, `{"a" => 1, "b" => 2}`},
		{`hash(["a",1,"b",2])`, `{"a" => 1, "b" => 2}`},
		{`pick(undef, "", "x", "y")`, "x"},
		{`pick_default(undef, "", "d")`, "d"},
		{`pick_default("a", "b")`, "a"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestStdlibArrayBlockFns(t *testing.T) {
	if got := firstLog(t, `notice(index([1,2,3,4]) |$x| { $x > 2 })`); got != "2" {
		t.Errorf("index block got %q", got)
	}
	if got := firstLog(t, `notice(any([1,2,3]) |$x| { $x == 2 })`); got != "true" {
		t.Errorf("any got %q", got)
	}
	if got := firstLog(t, `notice(all([2,4,6]) |$x| { $x % 2 == 0 })`); got != "true" {
		t.Errorf("all got %q", got)
	}
	if got := firstLog(t, `notice(all([2,3]) |$x| { $x % 2 == 0 })`); got != "false" {
		t.Errorf("all-false got %q", got)
	}
	if got := firstLog(t, `notice(grep([1,2,3,4]) |$x| { $x > 2 })`); got != "[3, 4]" {
		t.Errorf("grep block got %q", got)
	}
	// range with block returns undef but iterates
	firstLog(t, `range(1,2) |$x| { notice($x) }`)
}

func TestStdlibArrayErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(flatten())`, "wrong number"},
		{`notice(flatten(5))`, "must be an Array"},
		{`notice(unique(5))`, "String or Array"},
		{`notice(concat(5))`, "must be an Array"},
		{`notice(member(5,1))`, "must be an Array"},
		{`notice(count(5))`, "must be an Array"},
		{`notice(index(5,1))`, "must be an Array"},
		{`notice(delete(5,1))`, "Array, Hash or String"},
		{`notice(delete_at(5,1))`, "must be an Array"},
		{`notice(delete_at([1],"x"))`, "index must be an Integer"},
		{`notice(delete_undef_values(5))`, "Array or Hash"},
		{`notice(difference(5,[1]))`, "must be an Array"},
		{`notice(difference([1],5))`, "must be an Array"},
		{`notice(intersection(5,[1]))`, "must be an Array"},
		{`notice(intersection([1],5))`, "must be an Array"},
		{`notice(union(5))`, "must be an Array"},
		{`notice(range(1))`, "wrong number"},
		{`notice(range(1,2,0))`, "non-zero"},
		{`notice(range([],[]))`, "unsupported bounds"},
		{`notice(sort(5,6))`, "wrong number"},
		{`notice(prefix(5))`, "must be an Array"},
		{`notice(prefix([1], 5))`, "must be a String"},
		{`notice(grep([1]))`, "wrong number"},
		{`notice(reject([1], 5))`, "expected a String or Regexp"},
		{`notice(reject(5, "x"))`, "must be an Array"},
		{`notice(values_at(5,[0]))`, "must be an Array"},
		{`notice(values_at([1],["x"]))`, "must be Integers"},
		{`notice(zip(5,[1]))`, "must be an Array"},
		{`notice(zip([1],5))`, "must be an Array"},
		{`notice(has_key(5,"a"))`, "must be a Hash"},
		{`notice(join_keys_to_values(5,"="))`, "must be a Hash"},
		{`notice(join_keys_to_values({},5))`, "must be a String"},
		{`notice(hash([1]))`, "even number"},
		{`notice(hash(5))`, "must be an Array"},
		{`notice(pick(undef, ""))`, "at least one"},
		{`notice(pick_default())`, "at least one"},
		{`notice(any([1]))`, "requires a block"},
		{`notice(all([1]))`, "requires a block"},
		{`notice(count(1,2,3))`, "wrong number"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestStdlibJoinKeysUndefAndArray(t *testing.T) {
	if got := evalOut(t, `join_keys_to_values({"a"=>undef}, ":")`); got != `["a:"]` {
		t.Errorf("undef value got %q", got)
	}
	if got := evalOut(t, `join_keys_to_values({"a"=>[1,2]}, ":")`); got != `["a:1", "a:2"]` {
		t.Errorf("array value got %q", got)
	}
}

func TestStdlibHashFns(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`deep_merge({"a"=>{"x"=>1}}, {"a"=>{"y"=>2}})`, `{"a" => {"x" => 1, "y" => 2}}`},
		{`deep_merge({"a"=>1}, undef, {"b"=>2})`, `{"a" => 1, "b" => 2}`},
		{`deep_merge({"a"=>1}, {"a"=>2})`, `{"a" => 2}`},
		{`dig({"a"=>{"b"=>[10,20]}}, ["a","b",1])`, "20"},
		{`dig({"a"=>1}, ["a","b"])`, ""},
		{`dig({"a"=>1}, ["z"])`, ""},
		{`get({"a"=>{"b"=>5}}, "a.b")`, "5"},
		{`get({"a"=>[1,2]}, "a.1")`, "2"},
		{`get({"a"=>1}, "z", "def")`, "def"},
		{`get({"a"=>1}, "")`, `{"a" => 1}`},
		{`delete_values({"a"=>1,"b"=>2,"c"=>1}, 1)`, `{"b" => 2}`},
		{`delete_regex(["foo","bar","baz"], "^ba")`, `["foo"]`},
		{`delete_regex({"foo"=>1,"bar"=>2}, "^ba")`, `{"foo" => 1}`},
		{`convert_to(5, String)`, "5"},
		{`convert_to("x", Array)`, `["x"]`},
		{`convert_to({"a"=>1}, Array)`, `[["a", 1]]`},
		{`convert_to(0, Boolean)`, "false"},
		{`convert_to({"a"=>1}, Hash)`, `{"a" => 1}`},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestStdlibTreeEach(t *testing.T) {
	got := logsOf(t, `tree_each([1,{"a"=>2},[3]]) |$v| { notice($v) }`)
	joined := strings.Join(got, ",")
	if joined != "1,2,3" {
		t.Errorf("tree_each got %q", joined)
	}
}

func TestStdlibHashErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(deep_merge({"a"=>1}))`, "at least two"},
		{`notice(deep_merge({"a"=>1}, 5))`, "must be a Hash"},
		{`notice(dig(5))`, "wrong number"},
		{`notice(dig({}, 5))`, "must be an Array"},
		{`notice(get({}))`, "wrong number"},
		{`notice(get({}, 5))`, "must be a String"},
		{`notice(delete_values(5,1))`, "must be a Hash"},
		{`notice(delete_regex(5,"x"))`, "Array or Hash"},
		{`notice(delete_regex([], 5))`, "expected a String or Regexp"},
		{`notice(convert_to(5))`, "wrong number"},
		{`notice(convert_to(5, "x"))`, "must be a Type"},
		{`notice(convert_to("x", Hash))`, "cannot convert to Hash"},
		{`notice(convert_to(5, Integer))`, "unsupported target type"},
		{`notice(tree_each([1]))`, "requires a block"},
		{`notice(tree_each(1,2) |$x| {})`, "wrong number"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestStdlibNumberFns(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`ceiling(4.2)`, "5"},
		{`ceiling(4)`, "4"},
		{`floor(4.8)`, "4"},
		{`round(4.5)`, "5"},
		{`round(4.4)`, "4"},
		{`round(7)`, "7"},
		{`sqrt(9.0)`, "3"},
		{`ceiling("4.2")`, "5"},
		{`clamp(5, 1, 10)`, "5"},
		{`clamp(0, 3, 10)`, "3"},
		{`clamp(99, 3, 10)`, "10"},
		{`clamp([5,1,10])`, "5"},
		{`sum([1,2,3])`, "6"},
		{`sum([1,2.5])`, "3.5"},
		{`to_bytes("1kb")`, "1024"},
		{`to_bytes("2 MB")`, "2097152"},
		{`to_bytes("5")`, "5"},
		{`to_bytes(1024)`, "1024"},
		{`to_bytes("1.5k")`, "1536"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestStdlibNumberErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(ceiling())`, "wrong number"},
		{`notice(ceiling("x"))`, "expects a Numeric"},
		{`notice(sqrt())`, "wrong number"},
		{`notice(sqrt("x"))`, "expects a Numeric"},
		{`notice(round())`, "wrong number"},
		{`notice(round("x"))`, "expects a Numeric"},
		{`notice(clamp(1,2))`, "three values"},
		{`notice(clamp([1,"a",3]))`, "cannot compare"},
		{`notice(sum())`, "wrong number"},
		{`notice(sum(5))`, "must be an Array"},
		{`notice(sum([1,"a"]))`, "must be Numeric"},
		{`notice(to_bytes())`, "wrong number"},
		{`notice(to_bytes("xx"))`, "cannot parse number"},
		{`notice(to_bytes("1zz"))`, "unknown unit"},
		{`notice(pw_hash("x","sha512","salt"))`, "not a valid hash type"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestStdlibPathFns(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`basename("/a/b/c.txt")`, "c.txt"},
		{`basename("/a/b/c.txt", ".txt")`, "c"},
		{`basename("/a/b/")`, "b"},
		{`basename("/")`, ""},
		{`dirname("/a/b/c.txt")`, "/a/b"},
		{`dirname("/a/b/")`, "/a"},
		{`dirname("file")`, "."},
		{`dirname("/file")`, "/"},
		{`extname("/a/b/c.txt")`, ".txt"},
		{`extname("noext")`, ""},
		{`extname("/a/.bashrc")`, ""},
		{`extname("a.b.c")`, ".c"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestStdlibPathErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`notice(basename())`, "wrong number"},
		{`notice(basename(5))`, "must be a String"},
		{`notice(basename("/a", 5))`, "must be a String"},
		{`notice(dirname())`, "wrong number"},
		{`notice(dirname(5))`, "must be a String"},
		{`notice(extname())`, "wrong number"},
		{`notice(extname(5))`, "must be a String"},
	}
	for _, tc := range cases {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}

func TestStdlibTypeFns(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`is_string("x")`, "true"},
		{`is_string(5)`, "false"},
		{`is_integer(5)`, "true"},
		{`is_integer("5")`, "false"},
		{`is_float(1.5)`, "true"},
		{`is_numeric(5)`, "true"},
		{`is_numeric("x")`, "false"},
		{`is_bool(true)`, "true"},
		{`is_array([1])`, "true"},
		{`is_hash({"a"=>1})`, "true"},
		{`type_of(5)`, "Integer[5, 5]"},
		{`any2array(undef)`, "[]"},
		{`any2array([1,2])`, "[1, 2]"},
		{`any2array({"a"=>1})`, `["a", 1]`},
		{`any2array("x")`, `["x"]`},
		{`any2array(1,2,3)`, "[1, 2, 3]"},
		{`any2array()`, "[]"},
		{`any2bool(true)`, "true"},
		{`any2bool("yes")`, "true"},
		{`any2bool(0)`, "false"},
		{`any2bool(3)`, "true"},
		{`any2bool(1.5)`, "true"},
		{`any2bool(undef)`, "false"},
		{`any2bool([1])`, "true"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestStdlibValidate(t *testing.T) {
	// Passing validations return undef (no error).
	oks := []string{
		`validate_array([1])`,
		`validate_hash({"a"=>1})`,
		`validate_string("x")`,
		`validate_bool(true)`,
		`validate_integer(5)`,
		`validate_numeric(1.5)`,
		`validate_re("abc", "b")`,
		`validate_re("abc", ["z","a"])`,
		`validate_legacy(Integer, "deprecated", 5)`,
		`validate_legacy("Integer", "deprecated", 5)`,
	}
	for _, src := range oks {
		if _, _, err := EvalString("notice(" + src + ")"); err != nil {
			t.Errorf("%s should pass, got %v", src, err)
		}
	}
	errs := []struct{ src, want string }{
		{`notice(validate_array(5))`, "not a Array"},
		{`notice(validate_array())`, "at least one"},
		{`notice(validate_hash(5))`, "not a Hash"},
		{`notice(validate_string(5))`, "not a String"},
		{`notice(validate_bool(5))`, "not a Boolean"},
		{`notice(validate_integer("x"))`, "not a Integer"},
		{`notice(validate_numeric("x"))`, "not a Numeric"},
		{`notice(validate_re(5))`, "wrong number"},
		{`notice(validate_re("abc", "z"))`, "does not match"},
		{`notice(validate_re("abc", "z", "custom msg"))`, "custom msg"},
		{`notice(validate_re("abc", "("))`, "does not match"},
		{`notice(validate_legacy(5))`, "wrong number"},
		{`notice(validate_legacy(5, "d", 1))`, "must be a Type"},
		{`notice(validate_legacy("Bogus[", "d", 1))`, "invalid type"},
		{`notice(validate_legacy(Integer, "d", "x"))`, "not an instance"},
		{`notice(is_string())`, "one argument"},
	}
	for _, tc := range errs {
		if err := evalErr(t, tc.src); !strings.Contains(err, tc.want) {
			t.Errorf("%s => %q, want %q", tc.src, err, tc.want)
		}
	}
}
