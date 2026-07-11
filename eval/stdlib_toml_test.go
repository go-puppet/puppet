// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"

	"github.com/go-pcore/pcore"
)

// TestToTOMLGolden checks stdlib::to_toml against byte-for-byte golden output
// captured from puppetlabs-stdlib's PuppetX::Stdlib::TomlDumper (Ruby).
func TestToTOMLGolden(t *testing.T) {
	cases := []struct{ expr, want string }{
		{
			`stdlib::to_toml({"a" => "hello", "b" => 2, "f" => 1.5, "g" => false, "neg" => -3, "z" => true})`,
			"a = \"hello\"\nb = 2\nf = 1.5\ng = false\nneg = -3\nz = true\n",
		},
		{
			`stdlib::to_toml({"a.b" => "x", "normal_key-1" => "y", "with space" => "z"})`,
			"\"a.b\" = \"x\"\nnormal_key-1 = \"y\"\n\"with space\" = \"z\"\n",
		},
		{
			`stdlib::to_toml({"s" => "line1\nline2\ttab\"quote\\back"})`,
			"s = \"line1\\nline2\\ttab\\\"quote\\\\back\"\n",
		},
		{
			`stdlib::to_toml({"s" => "a#{b}c and #@x and #\$y"})`,
			"s = \"a#{b}c and #@x and #$y\"\n",
		},
		{
			`stdlib::to_toml({"s" => "café €"})`,
			"s = \"café €\"\n",
		},
		{
			`stdlib::to_toml({"database" => {"server" => "1.2.3.4", "ports" => "x"}, "title" => "cfg"})`,
			"title = \"cfg\"\n[database]\nports = \"x\"\nserver = \"1.2.3.4\"\n",
		},
		{
			`stdlib::to_toml({"a" => {"b" => {"c" => "d"}}})`,
			"[a.b]\nc = \"d\"\n",
		},
		{
			`stdlib::to_toml({"products" => [{"name" => "Hammer", "sku" => 738}, {"name" => "Nail", "sku" => 284}]})`,
			"[[products]]\nname = \"Hammer\"\nsku = 738\n[[products]]\nname = \"Nail\"\nsku = 284\n",
		},
		{
			`stdlib::to_toml({"a" => {}})`,
			"[a]\n",
		},
		{
			`stdlib::to_toml({"empty" => [], "list" => ["a", "b"], "nested_arr" => [[1, 2], [3]], "nums" => [1, 2, 3]})`,
			"empty = []\nlist = [\"a\", \"b\"]\nnested_arr = [[1, 2], [3]]\nnums = [1, 2, 3]\n",
		},
		{
			`stdlib::to_toml({"a" => 1.0, "b" => 100.0, "c" => -3.5, "d" => 0.25})`,
			"a = 1.0\nb = 100.0\nc = -3.5\nd = 0.25\n",
		},
		{
			`stdlib::to_toml({"name" => "top", "servers" => {"alpha" => {"ip" => "10.0.0.1"}, "beta" => {"ip" => "10.0.0.2"}}})`,
			"name = \"top\"\n[servers.alpha]\nip = \"10.0.0.1\"\n[servers.beta]\nip = \"10.0.0.2\"\n",
		},
		{
			`stdlib::to_toml({"list" => ["a\nb", "c#{d}"]})`,
			"list = [\"a\\nb\", \"c\\#{d}\"]\n",
		},
		{
			`stdlib::to_toml({"a" => undef})`,
			"a = nil\n",
		},
		// The unnamespaced alias behaves identically.
		{
			`to_toml({"a" => 1})`,
			"a = 1\n",
		},
	}
	for _, c := range cases {
		if got := evalOut(t, c.expr); got != c.want {
			t.Errorf("%s\n got=%q\nwant=%q", c.expr, got, c.want)
		}
	}
}

// TestToTOMLSensitive verifies the rewrap_sensitive_data integration: a hash
// containing a Sensitive value yields a Sensitive TOML string, unwrappable back
// to the clear TOML.
func TestToTOMLSensitive(t *testing.T) {
	if got := evalOut(t, `stdlib::to_toml({"pw" => Sensitive("secret")})`); got != "Sensitive [value redacted]" {
		t.Fatalf("sensitive to_toml render = %q, want redacted", got)
	}
	if got := evalOut(t, `unwrap(stdlib::to_toml({"pw" => Sensitive("secret")}))`); got != "pw = \"secret\"\n" {
		t.Fatalf("unwrapped sensitive to_toml = %q", got)
	}
}

// TestToTOMLErrors covers the argument-validation branches.
func TestToTOMLErrors(t *testing.T) {
	if e := evalErr(t, `notice(stdlib::to_toml())`); !strings.Contains(e, "wrong number of arguments") {
		t.Errorf("arity error = %q", e)
	}
	if e := evalErr(t, `notice(stdlib::to_toml("x"))`); !strings.Contains(e, "must be a Hash") {
		t.Errorf("type error = %q", e)
	}
}

// TestRubyInspectString exercises every escape branch of the Ruby String#inspect
// port against golden output captured from Ruby, in both the top-level (guard
// stripped) and array-element (guard kept) forms.
func TestRubyInspectString(t *testing.T) {
	in := "bell\abs\btab\tnl\nvt\vff\fcr\resc\x1bq\"bs\\hash#{y}plain#z"
	wantTop := `"bell\abs\btab\tnl\nvt\vff\fcr\resc\eq\"bs\\hash#{y}plain#z"`
	wantArr := `"bell\abs\btab\tnl\nvt\vff\fcr\resc\eq\"bs\\hash\#{y}plain#z"`
	if got := tomlValue(in); got != wantTop {
		t.Errorf("tomlValue top-level:\n got=%q\nwant=%q", got, wantTop)
	}
	if got := rubyInspect(in); got != wantArr {
		t.Errorf("rubyInspect array-element:\n got=%q\nwant=%q", got, wantArr)
	}
	// Control characters without a named escape use \uXXXX (uppercase hex).
	if got := tomlValue("\x00\x01\x7f"); got != "\"\\u0000\\u0001\\u007F\"" {
		t.Errorf("control chars = %q", got)
	}
}

// TestRubyInspectValues covers rubyInspect's non-string branches, including the
// undef ("nil") and fallthrough cases not reachable through to_toml manifests.
func TestRubyInspectValues(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{true, "true"},
		{int64(7), "7"},
		{1.0, "1.0"},
		{1.5, "1.5"},
		{[]any{int64(1), "a"}, `[1, "a"]`},
		{map[string]any{"k": int64(1)}, `{"k"=>1}`},
		{pcore.Undef, "nil"},
	}
	for _, c := range cases {
		if got := rubyInspect(c.v); got != c.want {
			t.Errorf("rubyInspect(%#v) = %q, want %q", c.v, got, c.want)
		}
	}
	// Fallthrough: a Pcore Type is rendered via stringify.
	typ, err := pcore.Parse("Integer")
	if err != nil {
		t.Fatal(err)
	}
	if got := rubyInspect(typ); got != "Integer" {
		t.Errorf("rubyInspect(Type) = %q", got)
	}
}
