package logic

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// maxConditionOperandsSettings is the configurable surface of the rule.
type maxConditionOperandsSettings struct {
	// Max is the number of operands a boolean expression may combine.
	Max int `yaml:"max"`
	// RequireParensOnMixed reports && and || mixed at the same level without
	// parentheses, regardless of the count.
	RequireParensOnMixed bool `yaml:"require_parens_on_mixed"`
	// IncludeLoops applies the rule to for conditions as well as if.
	IncludeLoops bool `yaml:"include_loops"`
}

var maxConditionOperands = &rule.Rule{
	ID:       "logic/max-condition-operands",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a condition combines at most a configured number of operands",
	Default:  diag.Error,
	Doc: `A condition may combine at most N operands with && and ||.

	if u != nil && u.Active && !u.Banned && (u.Role == Admin || u.Role == Owner) && u.MFA {
	                                                                             violation

	if u.CanAdminister() {                                                       OK

Rationale: boolean expressions are read by evaluating them, and people cannot
evaluate six terms without losing track of one. The failure is not that the
condition is hard to read; it is that it is hard to read
incorrectly-but-plausibly — the reader believes they understood it, and the term
they dropped is the one that mattered.

Naming a predicate replaces the evaluation with a lookup, and gives the
condition a place to be unit-tested.

To fix: extract a named predicate, usually a method on the type the condition
interrogates.

require_parens_on_mixed is called out separately because Go's precedence is
correct but not obvious: && binds tighter than ||, so a || b && c reads to many
people as (a || b) && c. The parentheses cost nothing.

Configure in .goorg.yaml:

	settings:
	  logic/max-condition-operands:
	    max: 4
	    require_parens_on_mixed: true
	    include_loops: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := maxConditionOperandsSettings{Max: 4, RequireParensOnMixed: true, IncludeLoops: true}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				cond, kind := conditionOf(n, s.IncludeLoops)
				if cond == nil {
					return true
				}
				if count := countOperands(cond); s.Max > 0 && count > s.Max {
					out = append(out, diag.Diagnostic{
						Position: c.Pos(cond),
						Message: fmt.Sprintf("%s condition combines %s, over the limit of %d",
							kind, plural(count, "operand"), s.Max),
						Help: "extract a named predicate",
					})
				}
				if s.RequireParensOnMixed && mixesOperators(cond) {
					out = append(out, diag.Diagnostic{
						Position: c.Pos(cond),
						Message:  "condition mixes && and || without parentheses",
						Help:     "parenthesise the intended grouping; && binds tighter than ||",
					})
				}
				return true
			})
		}
		return out
	},
}

// conditionOf returns the boolean condition of a statement, and a label for it.
func conditionOf(n ast.Node, includeLoops bool) (ast.Expr, string) {
	switch stmt := n.(type) {
	case *ast.IfStmt:
		return stmt.Cond, "if"
	case *ast.ForStmt:
		if !includeLoops || stmt.Cond == nil {
			return nil, ""
		}
		return stmt.Cond, "for"
	default:
		return nil, ""
	}
}

// countOperands counts the leaves of the boolean expression tree joined by &&
// and ||. Anything else, including a call or a comparison, is one operand.
func countOperands(e ast.Expr) int {
	switch expr := e.(type) {
	case *ast.BinaryExpr:
		if expr.Op == token.LAND || expr.Op == token.LOR {
			return countOperands(expr.X) + countOperands(expr.Y)
		}
		return 1
	case *ast.ParenExpr:
		return countOperands(expr.X)
	default:
		return 1
	}
}

// mixesOperators reports whether && and || appear at the same level of the
// expression tree without parentheses between them.
func mixesOperators(e ast.Expr) bool {
	top, ok := e.(*ast.BinaryExpr)
	if !ok || (top.Op != token.LAND && top.Op != token.LOR) {
		return false
	}
	other := token.LAND
	if top.Op == token.LAND {
		other = token.LOR
	}
	return hasBareOperator(top.X, other) || hasBareOperator(top.Y, other)
}

// hasBareOperator reports whether op appears in an expression without an
// intervening ParenExpr, which is what makes the precedence implicit.
func hasBareOperator(e ast.Expr, op token.Token) bool {
	bin, ok := e.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if bin.Op == op {
		return true
	}
	if bin.Op != token.LAND && bin.Op != token.LOR {
		return false
	}
	return hasBareOperator(bin.X, op) || hasBareOperator(bin.Y, op)
}
