// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"github.com/go-pcore/pcore"
	"github.com/go-puppet/puppet/catalog"
	"github.com/go-puppet/puppet/parser"
)

// PlanExecutor is the orchestration seam a Bolt-like host (notably
// github.com/go-puppet-bolt/bolt) wires so that a `plan` written in the Puppet
// language can run tasks/commands/scripts and apply catalogs against targets.
//
// Values use the evaluator's generic model (strings, []any, map[string]any) so
// the interface has no dependency on a specific transport package. Targets are
// passed as their string specs; an executor resolves them through its own
// inventory. The returned Value becomes the plan function's result (typically a
// ResultSet rendered as an Array of per-target result Hashes).
type PlanExecutor interface {
	RunTask(task string, targets []string, params map[string]any) (Value, error)
	RunCommand(command string, targets []string) (Value, error)
	RunScript(script string, targets []string, args []any) (Value, error)
	GetTargets(spec Value) (Value, error)
	ApplyCatalog(targets []string, cat *catalog.Catalog) (Value, error)
}

// WithPlanExecutor wires a [PlanExecutor] for the plan functions.
func WithPlanExecutor(x PlanExecutor) Option { return func(e *Evaluator) { e.planExec = x } }

// returnSignal unwinds a function/plan/apply body when `return` is called.
type returnSignal struct{ value Value }

func (returnSignal) Error() string { return "return outside of a function, plan or apply block" }

// nextSignal / breakSignal implement the loop-control jump functions.
type nextSignal struct{ value Value }

func (nextSignal) Error() string { return "next() outside of an iteration" }

type breakSignal struct{ value Value }

func (breakSignal) Error() string { return "break() outside of an iteration" }

// registerPlan installs the plan keyword's function surface and the jump
// functions. These are registered unconditionally so that `return`/`next`/
// `break` work in ordinary functions too; the run_* / apply / get_targets
// functions require a [PlanExecutor].
func registerPlanFns(e *Evaluator) {
	e.funcs["return"] = func(_ *Context, args []Value, _ *Block) (Value, error) {
		return nil, returnSignal{value: firstOrUndef(args)}
	}
	e.funcs["next"] = func(_ *Context, args []Value, _ *Block) (Value, error) {
		return nil, nextSignal{value: firstOrUndef(args)}
	}
	e.funcs["break"] = func(_ *Context, args []Value, _ *Block) (Value, error) {
		return nil, breakSignal{value: firstOrUndef(args)}
	}
	e.funcs["run_task"] = builtinRunTask
	e.funcs["run_command"] = builtinRunCommand
	e.funcs["run_script"] = builtinRunScript
	e.funcs["get_targets"] = builtinGetTargets
	e.funcs["get_target"] = builtinGetTargets
	e.funcs["apply"] = builtinApply
}

func firstOrUndef(args []Value) Value {
	if len(args) > 0 {
		return args[0]
	}
	return pcore.Undef
}

// EvalPlan runs a previously-parsed `plan` by name with the given parameters and
// returns its result value. Definitions must have been registered first (via
// EvalProgram or by parsing a program that contains the plan). It is the entry
// point a Bolt host uses to run a `.pp` plan.
func (e *Evaluator) EvalPlan(name string, params map[string]any) (Value, error) {
	def, ok := e.plans[name]
	if !ok {
		return nil, &Error{Msg: "unknown plan " + name}
	}
	scope := newScope(e.top)
	if err := e.bindNamedParams(scope, def.Params, params, name); err != nil {
		return nil, err
	}
	v, err := e.evalBody(def.Body, scope)
	if rs, ok := err.(returnSignal); ok {
		return rs.value, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// EvalPlanString parses src, registers its definitions, and runs the named plan.
func EvalPlanString(src, plan string, params map[string]any, opts ...Option) (Value, []LogEntry, error) {
	prog, err := parser.Parse(src)
	if err != nil {
		return nil, nil, err
	}
	e := New(opts...)
	for _, n := range prog.Body {
		e.register(n)
	}
	v, err := e.EvalPlan(plan, params)
	return v, e.Logs(), err
}

func (e *Evaluator) requirePlanExecutor(fn string) error {
	if e.planExec == nil {
		return &Error{Msg: fn + "() requires a plan executor; run the plan through a Bolt host that wires eval.WithPlanExecutor"}
	}
	return nil
}

// targetSpecs coerces a targets argument (String / Array / Target hash) to the
// string specs a PlanExecutor consumes.
func targetSpecs(v Value) []string {
	switch x := normalize(v).(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, targetSpecs(e)...)
		}
		return out
	case map[string]any:
		if name, ok := x["uri"]; ok {
			return []string{stringify(name)}
		}
		if name, ok := x["name"]; ok {
			return []string{stringify(name)}
		}
	}
	return nil
}

func builtinRunTask(c *Context, args []Value, _ *Block) (Value, error) {
	if err := c.e.requirePlanExecutor("run_task"); err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, &Error{Msg: "run_task() expects a task name, targets and optional parameters"}
	}
	task, err := argStr(args, 0, "run_task")
	if err != nil {
		return nil, err
	}
	params := map[string]any{}
	if len(args) >= 3 {
		h, ok := normalize(args[2]).(map[string]any)
		if !ok {
			return nil, &Error{Msg: "run_task() parameters must be a Hash"}
		}
		params = h
	}
	return c.e.planExec.RunTask(task, targetSpecs(args[1]), params)
}

func builtinRunCommand(c *Context, args []Value, _ *Block) (Value, error) {
	if err := c.e.requirePlanExecutor("run_command"); err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, &Error{Msg: "run_command() expects a command and targets"}
	}
	command, err := argStr(args, 0, "run_command")
	if err != nil {
		return nil, err
	}
	return c.e.planExec.RunCommand(command, targetSpecs(args[1]))
}

func builtinRunScript(c *Context, args []Value, _ *Block) (Value, error) {
	if err := c.e.requirePlanExecutor("run_script"); err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, &Error{Msg: "run_script() expects a script and targets"}
	}
	script, err := argStr(args, 0, "run_script")
	if err != nil {
		return nil, err
	}
	var scriptArgs []any
	if len(args) >= 3 {
		if h, ok := normalize(args[2]).(map[string]any); ok {
			if a, ok := normalize(h["arguments"]).([]any); ok {
				scriptArgs = a
			}
		} else if a, ok := normalize(args[2]).([]any); ok {
			scriptArgs = a
		}
	}
	return c.e.planExec.RunScript(script, targetSpecs(args[1]), scriptArgs)
}

func builtinGetTargets(c *Context, args []Value, _ *Block) (Value, error) {
	if err := c.e.requirePlanExecutor("get_targets"); err != nil {
		return nil, err
	}
	if len(args) != 1 {
		return nil, wrongArgs("get_targets")
	}
	return c.e.planExec.GetTargets(args[0])
}

// builtinApply evaluates the apply block's body into a fresh catalog and hands
// it to the executor to apply against the given targets.
func builtinApply(c *Context, args []Value, block *Block) (Value, error) {
	if err := c.e.requirePlanExecutor("apply"); err != nil {
		return nil, err
	}
	if len(args) < 1 {
		return nil, &Error{Msg: "apply() expects targets and a block"}
	}
	if block == nil {
		return nil, &Error{Msg: "apply() requires a block of resources"}
	}
	cat, err := c.e.compileApplyBlock(block)
	if err != nil {
		return nil, err
	}
	return c.e.planExec.ApplyCatalog(targetSpecs(args[0]), cat)
}

// compileApplyBlock runs an apply block's body in a throwaway sub-evaluator so
// its resource declarations build an independent catalog.
func (e *Evaluator) compileApplyBlock(block *Block) (*catalog.Catalog, error) {
	sub := New(func(se *Evaluator) {
		se.facts = e.facts
		se.hiera = e.hiera
		se.nodeName = e.nodeName
	})
	// Share the parent's class/define definitions so the apply block can declare
	// them.
	sub.classes = e.classes
	sub.defines = e.defines
	sub.userFuncs = e.userFuncs
	sub.plans = e.plans
	scope := newScope(sub.top)
	for k, v := range block.scope.snapshot() {
		scope.setForce(k, v)
	}
	if _, err := sub.evalBody(block.node.Body, scope); err != nil {
		return nil, err
	}
	return sub.cat, nil
}
