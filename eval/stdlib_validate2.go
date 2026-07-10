// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"regexp"

	"github.com/go-pcore/pcore"
)

// This file implements the puppetlabs-stdlib syntactic validators that fail
// compilation when a value is malformed: validate_domain_name (Stdlib::Fqdn or
// Stdlib::Dns::Zone) and validate_email_address (Stdlib::Email). The accepting
// patterns are copied verbatim from the module's type aliases.

var (
	fqdnRe  = regexp.MustCompile(`\A(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])\z`)
	zoneRe  = regexp.MustCompile(`\A((([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])\.)+|\.)\z`)
	emailRe = regexp.MustCompile(`\A[a-zA-Z0-9.!#$%&'*+\/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*\z`)
)

// registerStdlibValidate2 installs the domain-name and e-mail validators under
// both their bare and stdlib::-namespaced names.
func registerStdlibValidate2(e *Evaluator) {
	dom := domainValidator("validate_domain_name", func(s string) bool {
		return fqdnRe.MatchString(s) || zoneRe.MatchString(s)
	})
	email := domainValidator("validate_email_address", emailRe.MatchString)
	e.funcs["validate_domain_name"] = dom
	e.funcs["stdlib::validate_domain_name"] = dom
	e.funcs["validate_email_address"] = email
	e.funcs["stdlib::validate_email_address"] = email
}

// domainValidator builds a repeated-argument validator that raises on the first
// value that is not a String or does not match ok.
func domainValidator(fn string, ok func(string) bool) Function {
	return func(_ *Context, args []Value, _ *Block) (Value, error) {
		if len(args) == 0 {
			return nil, &Error{Msg: fn + "(): Wrong number of arguments need at least one"}
		}
		for _, a := range args {
			s, isStr := normalize(a).(string)
			if !isStr {
				return nil, &Error{Msg: fn + "(): got " + typeName(a)}
			}
			if !ok(s) {
				return nil, &Error{Msg: fn + "(): got '" + s + "'"}
			}
		}
		return pcore.Undef, nil
	}
}
