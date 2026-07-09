// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"strings"
	"testing"

	"github.com/go-puppet/puppet/catalog"
)

// recExec is a recording PlanExecutor for tests.
type recExec struct {
	calls []string
}

func (r *recExec) RunTask(task string, targets []string, params map[string]any) (Value, error) {
	r.calls = append(r.calls, "task:"+task+":"+strings.Join(targets, ","))
	return map[string]any{"task": task, "ok": true}, nil
}
func (r *recExec) RunCommand(command string, targets []string) (Value, error) {
	r.calls = append(r.calls, "cmd:"+command)
	return "out:" + command, nil
}
func (r *recExec) RunScript(script string, targets []string, args []any) (Value, error) {
	r.calls = append(r.calls, "script:"+script)
	return int64(len(args)), nil
}
func (r *recExec) GetTargets(spec Value) (Value, error) {
	return []any{spec}, nil
}
func (r *recExec) ApplyCatalog(targets []string, cat *catalog.Catalog) (Value, error) {
	r.calls = append(r.calls, "apply")
	return int64(len(cat.Resources())), nil
}

func TestPlanBasic(t *testing.T) {
	x := &recExec{}
	src := `
plan deploy(String $app = 'web', String $nodes = 'g') {
  $ts = get_targets($nodes)
  run_command("systemctl restart ${app}", $ts)
  run_task('pkg::install', $ts, {'name' => $app})
  run_script('setup.sh', $ts, {'arguments' => ['--fast']})
  $n = apply($ts) {
    file { "/etc/${app}.conf": ensure => file }
    service { $app: ensure => running }
  }
  return "deployed ${app} (${n} resources)"
}
`
	v, _, err := EvalPlanString(src, "deploy", map[string]any{"app": "api"}, WithPlanExecutor(x))
	if err != nil {
		t.Fatal(err)
	}
	if v != "deployed api (2 resources)" {
		t.Errorf("plan result: %q", v)
	}
	joined := strings.Join(x.calls, "|")
	for _, want := range []string{"cmd:systemctl restart api", "task:pkg::install", "script:setup.sh", "apply"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing call %q in %q", want, joined)
		}
	}
}

func TestPlanNoReturn(t *testing.T) {
	x := &recExec{}
	v, _, err := EvalPlanString(`plan p() { notice('hi') }`, "p", nil, WithPlanExecutor(x))
	if err != nil {
		t.Fatal(err)
	}
	if !isUndef(v) {
		t.Errorf("expected undef, got %v", v)
	}
}

func TestPlanUnknown(t *testing.T) {
	_, _, err := EvalPlanString(`plan p() { }`, "nope", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown plan") {
		t.Errorf("got %v", err)
	}
}

func TestPlanParseError(t *testing.T) {
	_, _, err := EvalPlanString(`plan p( {`, "p", nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestPlanParamError(t *testing.T) {
	_, _, err := EvalPlanString(`plan p(Integer $n) { }`, "p", map[string]any{"n": "x"}, WithPlanExecutor(&recExec{}))
	if err == nil || !strings.Contains(err.Error(), "expects") {
		t.Errorf("got %v", err)
	}
}

func TestPlanBodyError(t *testing.T) {
	_, _, err := EvalPlanString(`plan p() { fail('boom') }`, "p", nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("got %v", err)
	}
}

func TestPlanFunctionsNeedExecutor(t *testing.T) {
	cases := []string{
		`run_task('t','n')`,
		`run_command('c','n')`,
		`run_script('s','n')`,
		`get_targets('n')`,
		`apply('n') { }`,
	}
	for _, expr := range cases {
		_, _, err := EvalPlanString("plan p() { "+expr+" }", "p", nil)
		if err == nil || !strings.Contains(err.Error(), "requires a plan executor") {
			t.Errorf("%s => %v", expr, err)
		}
	}
}

func TestPlanFunctionArgErrors(t *testing.T) {
	x := WithPlanExecutor(&recExec{})
	cases := []struct{ src, want string }{
		{`plan p() { run_task('t') }`, "expects a task name"},
		{`plan p() { run_task(5, 'n') }`, "must be a String"},
		{`plan p() { run_task('t','n',5) }`, "must be a Hash"},
		{`plan p() { run_command('c') }`, "expects a command"},
		{`plan p() { run_command(5,'n') }`, "must be a String"},
		{`plan p() { run_script('s') }`, "expects a script"},
		{`plan p() { run_script(5,'n') }`, "must be a String"},
		{`plan p() { get_targets('a','b') }`, "wrong number"},
		{`plan p() { apply() }`, "expects targets"},
		{`plan p() { apply('n') }`, "requires a block"},
	}
	for _, tc := range cases {
		_, _, err := EvalPlanString(tc.src, "p", nil, x)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s => %v, want %q", tc.src, err, tc.want)
		}
	}
}

func TestPlanRunScriptArrayArgs(t *testing.T) {
	x := &recExec{}
	// run_script accepts a bare array of arguments too.
	v, _, err := EvalPlanString(`plan p() { return run_script('s', 'n', ['a','b','c']) }`, "p", nil, WithPlanExecutor(x))
	if err != nil {
		t.Fatal(err)
	}
	if v != int64(3) {
		t.Errorf("got %v", v)
	}
}

func TestTargetSpecs(t *testing.T) {
	x := &recExec{}
	// Targets as an array, and as a Target-hash with uri/name.
	_, _, err := EvalPlanString(`plan p() {
	  run_command('c', ['a','b'])
	  run_command('c2', {'uri' => 'host1'})
	  run_command('c3', {'name' => 'host2'})
	}`, "p", nil, WithPlanExecutor(x))
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplyBlockError(t *testing.T) {
	_, _, err := EvalPlanString(`plan p() { apply('n') { nope() } }`, "p", nil, WithPlanExecutor(&recExec{}))
	if err == nil || !strings.Contains(err.Error(), "unknown function") {
		t.Errorf("got %v", err)
	}
}

func TestJumpFunctions(t *testing.T) {
	cases := []struct{ src, want string }{
		// return in a function
		{`function f() { return(7) 99 }
notice(f())`, "7"},
		{`function f() { return() }
notice("[${f()}]")`, "[]"},
		// next in map yields a value for that element
		{`notice([1,2,3].map |$x| { if $x == 2 { next(0) } $x })`, "[1, 0, 3]"},
		// break stops each; break value ignored for each's own return (each returns it)
		{`$r = [1,2,3,4].reduce(0) |$a,$x| { if $x > 2 { break($a) } $a + $x }
notice($r)`, "3"},
		// break in map returns the break value as the whole result
		{`notice([1,2,3].map |$x| { if $x == 2 { break(-1) } $x })`, "-1"},
		// break in filter
		{`notice([1,2,3].filter |$x| { if $x == 2 { break([9]) } true })`, "[9]"},
		// break in each returns the break value
		{`notice([1,2,3].each |$x| { if $x == 2 { break("stopped") } })`, "stopped"},
		// break in a hash filter
		{`notice({"a"=>1,"b"=>2}.filter |$k,$v| { if $k == "b" { break("x") } true })`, "x"},
	}
	for _, tc := range cases {
		if got := firstLog(t, tc.src); got != tc.want {
			t.Errorf("%s => %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestBreakBareForms(t *testing.T) {
	// bare return / next / break with no args (called as a statement without parens
	// is not valid; they are jump functions invoked with parens).
	if got := firstLog(t, `notice([1,2,3].map |$x| { if $x == 2 { next() } $x })`); got != "[1, , 3]" {
		t.Errorf("bare next got %q", got)
	}
}
