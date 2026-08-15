package logic

import (
	"fmt"
	"go/ast"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// preferGuardClauseSettings is the configurable surface of the rule.
type preferGuardClauseSettings struct {
	// MinBodyStatements is how much wrapped code makes inverting worthwhile.
	MinBodyStatements int `yaml:"min_body_statements"`
	// CheckLoops applies the rule inside loop bodies, where continue is the
	// guard.
	CheckLoops bool `yaml:"check_loops"`
}

var preferGuardClause = &rule.Rule{
	ID:       "logic/prefer-guard-clause",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a wholly wrapped body should invert into a guard clause",
	Default:  diag.Error,
	Doc: `When a function body is wholly wrapped in a conditional, invert it into
a guard clause.

	func process(r *Record) error {        violation
		if r != nil {
			// 40 lines
		}
		return nil
	}

	func process(r *Record) error {        OK
		if r == nil {
			return nil
		}
		// 40 lines at the top level
	}

Rationale: a guard clause states the precondition and leaves; a wrapping
conditional states the precondition and then makes the reader carry it for forty
lines. The difference matters most at the bottom of a long function, where the
wrapped form requires scrolling back to recall which branch you are in. The
wrapped form also pushes every subsequent addition one level deeper, so it
compounds — this is how a function reaches five levels of indentation without
anyone deciding it should.

To fix: invert the condition, return or continue early, and unindent the body.

The rule fires only when the conditional is the sole statement of the body and
has no else, so there is exactly one way to invert it.

Configure in .goorg.yaml:

	settings:
	  logic/prefer-guard-clause:
	    min_body_statements: 3
	    check_loops: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := preferGuardClauseSettings{MinBodyStatements: 3, CheckLoops: true}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				body, kind := wrappableBody(n, s.CheckLoops)
				if body == nil {
					return true
				}
				wrapper, depth := soleConditional(body, s.MinBodyStatements)
				if wrapper == nil {
					return true
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(wrapper),
					Message: fmt.Sprintf("%s body is wholly wrapped in %s",
						kind, plural(depth, "conditional")),
					Help: "invert the condition and return early, then unindent the body",
				})
				return true
			})
		}
		return out
	},
}

// wrappableBody returns the body of a declaration the rule applies to.
func wrappableBody(n ast.Node, checkLoops bool) (*ast.BlockStmt, string) {
	switch node := n.(type) {
	case *ast.FuncDecl:
		return node.Body, "function"
	case *ast.FuncLit:
		return node.Body, "function"
	case *ast.ForStmt:
		if !checkLoops {
			return nil, ""
		}
		return node.Body, "loop"
	case *ast.RangeStmt:
		if !checkLoops {
			return nil, ""
		}
		return node.Body, "loop"
	default:
		return nil, ""
	}
}

// soleConditional returns the outermost `if` that wraps a whole body, together
// with how many such conditionals are nested.
//
// Nested wrappers collapse into one finding: `if a { if b { ... } }` is one
// inversion to make, not two, and reporting it twice would suggest otherwise.
func soleConditional(body *ast.BlockStmt, minStatements int) (*ast.IfStmt, int) {
	if body == nil {
		return nil, 0
	}
	var outermost *ast.IfStmt
	depth := 0
	block := body
	for {
		// The trailing statement of a function body is often a bare return,
		// which the inverted guard would supply anyway.
		stmts := significantStatements(block)
		if len(stmts) != 1 {
			break
		}
		wrapper, ok := stmts[0].(*ast.IfStmt)
		if !ok || wrapper.Else != nil || wrapper.Body == nil {
			break
		}
		if outermost == nil {
			outermost = wrapper
		}
		depth++
		block = wrapper.Body
	}
	if outermost == nil || len(block.List) < minStatements {
		return nil, 0
	}
	return outermost, depth
}

// significantStatements drops a trailing bare return, which a guard clause
// would reintroduce as its own early exit.
func significantStatements(block *ast.BlockStmt) []ast.Stmt {
	stmts := block.List
	if len(stmts) < 2 {
		return stmts
	}
	last, ok := stmts[len(stmts)-1].(*ast.ReturnStmt)
	if !ok || len(last.Results) > 1 {
		return stmts
	}
	return stmts[:len(stmts)-1]
}
