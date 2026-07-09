// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"
)

// TestArityErrors exercises the wrong-argument-count branch of every stdlib
// function that guards its arity, so those error returns are covered.
func TestArityErrors(t *testing.T) {
	calls := []string{
		`unique()`, `concat()`, `member(1)`, `index()`, `delete(1)`, `delete_at(1)`,
		`delete_undef_values()`, `compact()`, `difference([1])`, `intersection([1])`,
		`union()`, `range(1)`, `prefix()`, `suffix()`, `reject([1])`, `values_at([1])`,
		`zip([1])`, `has_key({})`, `join_keys_to_values({})`, `hash([1], 2)`,
		`deep_merge({})`, `dig(5)`, `get({})`, `delete_values(5)`, `delete_regex(5)`,
		`convert_to(5)`, `tree_each([1])`, `ceiling()`, `floor()`, `round()`, `sqrt()`,
		`sum()`, `to_bytes()`, `basename()`, `dirname()`, `extname()`, `type_of()`,
		`lstrip()`, `squeeze()`, `str2bool()`, `bool2str()`, `num2bool()`, `bool2num()`,
		`str2num()`, `strlen()`, `shell_escape()`, `start_with("a")`, `end_with("a")`,
		`versioncmp("1")`, `regsubst("a")`, `match("a")`, `str2resource()`, `getvar()`,
		`getparam(File['x'])`, `create_resources()`, `ensure_resource()`, `defined()`,
		`count(1,2,3)`, `sort(1,2)`, `flatten()`, `keys()`, `size(1,2)`, `type(1,2)`,
		`parsejson()`, `parseyaml()`, `to_json()`, `to_json_pretty()`, `to_yaml()`,
		`loadjson()`, `loadyaml()`, `validate_re(5)`, `validate_legacy(5)`,
	}
	for _, c := range calls {
		err := evalErr(t, "notice("+c+")")
		if err == "" {
			t.Errorf("%s: expected an error", c)
		}
	}
}

func TestEdgeBranches(t *testing.T) {
	cases := []struct{ expr, want string }{
		// index with a block that never matches -> undef (empty render)
		{`index([1,2,3]) |$x| { $x > 9 }`, ""},
		// prefix / suffix over a hash
		{`prefix({"a"=>1}, "p_")`, `{"p_a" => 1}`},
		{`suffix({"a"=>1}, "_s")`, `{"a_s" => 1}`},
		// grep / reject predicate errors surface
		// unique on already-unique
		{`unique([1,2])`, "[1, 2]"},
		// difference/intersection identity
		{`intersection([1,1,2],[1,2])`, "[1, 2]"},
		// range single-letter with default step
		{`range("a","a")`, `["a"]`},
		// dig into an array with out-of-range index
		{`dig([1,2], [5])`, ""},
		// get with a numeric-looking segment into array
		{`get([10,20], "1")`, "20"},
		// convert_to Array on undef
		{`convert_to(undef, Array)`, "[]"},
		// any2array with a single scalar arg already covered; test empty hash
		{`any2array({})`, "[]"},
		// zip with second array longer is truncated to first
		{`zip([1],[2,3])`, "[[1, 2]]"},
	}
	for _, tc := range cases {
		if got := evalOut(t, tc.expr); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestGrepRejectPredicateErrors(t *testing.T) {
	if err := evalErr(t, `notice(grep(["a"], 5))`); !strings.Contains(err, "expected a String or Regexp") {
		t.Errorf("grep pattern err: %q", err)
	}
	if err := evalErr(t, `notice(reject(["a"], 5))`); !strings.Contains(err, "expected a String or Regexp") {
		t.Errorf("reject pattern err: %q", err)
	}
}

func TestYamlSpecialTypes(t *testing.T) {
	// Symbol -> string
	if got := evalOut(t, `parseyaml(":sym")`); got != "sym" {
		t.Errorf("yaml symbol got %q", got)
	}
	// big integer that fits in int64 after being parsed as *big.Int
	if got := evalOut(t, `parseyaml("100000000000000000000")`); got == "" {
		t.Errorf("yaml bigint got %q", got)
	}
	// nested list/hash round-trip through yamlToValue
	if got := evalOut(t, `parseyaml("a:\n  - 1\n  - 2\n")`); got != `{"a" => [1, 2]}` {
		t.Errorf("yaml nested got %q", got)
	}
}

func TestToJSONYAMLNested(t *testing.T) {
	// Rich value (a Type) rendered via stringify fallback in JSON.
	if got := evalOut(t, `to_json({"t" => Integer})`); got != `{"t":"Integer"}` {
		t.Errorf("to_json type got %q", got)
	}
	// to_yaml over nested arrays/hashes and undef.
	if got := evalOut(t, `to_yaml([1, {"a"=>undef}])`); !strings.Contains(got, "a:") {
		t.Errorf("to_yaml nested got %q", got)
	}
	// valueToJSON over a nested array inside a hash.
	if got := evalOut(t, `to_json({"a" => [1, {"b" => 2}]})`); got != `{"a":[1,{"b":2}]}` {
		t.Errorf("to_json deep got %q", got)
	}
}
