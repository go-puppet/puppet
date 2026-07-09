// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"github.com/go-pcore/pcore"
	yaml "github.com/go-ruby-yaml/yaml"
)

// registerStdlibData installs the JSON/YAML (de)serialization functions.
func registerStdlibData(e *Evaluator) {
	e.funcs["parsejson"] = builtinParseJSON
	e.funcs["parseyaml"] = builtinParseYAML
	e.funcs["to_json"] = builtinToJSON(false)
	e.funcs["to_json_pretty"] = builtinToJSON(true)
	e.funcs["to_yaml"] = builtinToYAML
	e.funcs["loadjson"] = builtinLoadJSON
	e.funcs["loadyaml"] = builtinLoadYAML
}

func builtinParseJSON(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("parsejson")
	}
	s, err := argStr(args, 0, "parsejson")
	if err != nil {
		return nil, err
	}
	v, perr := parseJSONString(s)
	if perr != nil {
		if len(args) == 2 {
			return args[1], nil
		}
		return nil, perr
	}
	return v, nil
}

func parseJSONString(s string) (Value, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, &Error{Msg: "parsejson(): " + err.Error()}
	}
	return jsonToValue(raw), nil
}

func jsonToValue(v any) Value {
	switch x := v.(type) {
	case nil:
		return pcore.Undef
	case json.Number:
		if i, err := strconv.ParseInt(string(x), 10, 64); err == nil {
			return i
		}
		f, _ := strconv.ParseFloat(string(x), 64)
		return f
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			out[k] = jsonToValue(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = jsonToValue(e)
		}
		return out
	default:
		return x
	}
}

func builtinLoadJSON(c *Context, args []Value, _ *Block) (Value, error) {
	return loadThen(c, args, "loadjson", parseJSONString)
}

func builtinLoadYAML(c *Context, args []Value, _ *Block) (Value, error) {
	return loadThen(c, args, "loadyaml", parseYAMLString)
}

// loadThen loads a template-named file via the loader and parses it, returning
// the optional default on any error.
func loadThen(c *Context, args []Value, fn string, parse func(string) (Value, error)) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs(fn)
	}
	name, err := argStr(args, 0, fn)
	if err != nil {
		return nil, err
	}
	src, lerr := c.e.loadTemplate(name)
	if lerr == nil {
		if v, perr := parse(src); perr == nil {
			return v, nil
		}
	}
	if len(args) == 2 {
		return args[1], nil
	}
	if lerr != nil {
		return nil, lerr
	}
	return parse(src)
}

func builtinParseYAML(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("parseyaml")
	}
	s, err := argStr(args, 0, "parseyaml")
	if err != nil {
		return nil, err
	}
	v, perr := parseYAMLString(s)
	if perr != nil {
		if len(args) == 2 {
			return args[1], nil
		}
		return nil, perr
	}
	return v, nil
}

func parseYAMLString(s string) (Value, error) {
	raw, err := yaml.Load(s)
	if err != nil {
		return nil, &Error{Msg: "parseyaml(): " + err.Error()}
	}
	return yamlToValue(raw), nil
}

// yamlToValue converts a go-ruby-yaml value into the evaluator's value model.
func yamlToValue(v any) Value {
	switch x := v.(type) {
	case nil:
		return pcore.Undef
	case *yaml.Map:
		out := map[string]any{}
		for _, p := range x.Pairs() {
			out[stringify(yamlToValue(p.Key))] = yamlToValue(p.Val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = yamlToValue(e)
		}
		return out
	case yaml.Symbol:
		return string(x)
	case *big.Int:
		if x.IsInt64() {
			return x.Int64()
		}
		return x.String()
	case *big.Float:
		f, _ := x.Float64()
		return f
	case int:
		return int64(x)
	default:
		return normalize(x)
	}
}

func builtinToJSON(pretty bool) Function {
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) != 1 {
			if pretty {
				return nil, wrongArgs("to_json_pretty")
			}
			return nil, wrongArgs("to_json")
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if pretty {
			enc.SetIndent("", "  ")
		}
		if err := enc.Encode(valueToJSON(args[0])); err != nil {
			return nil, &Error{Msg: "to_json(): " + err.Error()}
		}
		return strings.TrimRight(buf.String(), "\n"), nil
	}
}

// valueToJSON lowers the evaluator's values to types encoding/json handles,
// mapping undef to nil and rendering rich Pcore values via their String form.
func valueToJSON(v Value) any {
	switch x := normalize(v).(type) {
	case nil:
		return nil
	case bool, int64, float64, string:
		return x
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = valueToJSON(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = valueToJSON(e)
		}
		return out
	default:
		if isUndef(x) {
			return nil
		}
		return stringify(x)
	}
}

func builtinToYAML(_ *Context, args []Value, _ *Block) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, wrongArgs("to_yaml")
	}
	out, err := yaml.Dump(valueToYAML(args[0]))
	if err != nil {
		return nil, &Error{Msg: "to_yaml(): " + err.Error()}
	}
	return out, nil
}

// valueToYAML lowers evaluator values for go-ruby-yaml's Dump. Hashes become
// plain Go maps (Dump emits those in sorted-key order, which is deterministic).
func valueToYAML(v Value) any {
	switch x := normalize(v).(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = valueToYAML(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for _, k := range sortedKeys(x) {
			out[k] = valueToYAML(x[k])
		}
		return out
	default:
		if isUndef(x) {
			return nil
		}
		return x
	}
}
