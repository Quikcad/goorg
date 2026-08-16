package logic

import (
	"fmt"
	"go/ast"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// sectionSpacingSettings is the configurable surface of the rule. Each limit
// disables its own check when set to 0, so a project can adopt one boundary
// without the other.
type sectionSpacingSettings struct {
	// MinGuards is how long the leading run of guard clauses has to be before
	// the body needs setting off from it.
	MinGuards int `yaml:"min_guards"`
	// MinResultLines is how many lines the trailing return has to span before
	// it needs setting off from the work above it.
	MinResultLines int `yaml:"min_result_lines"`
	// MinBodyStatements is how much work has to precede that return before
	// separating it buys anything.
	MinBodyStatements int `yaml:"min_body_statements"`
}

var sectionSpacing = &rule.Rule{
	ID:       "logic/section-spacing",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	// The outcome travels with the declaration, so this rule has no opinion
	// about which file the declaration lives in and must not veto a move.
	Placement: rule.PlacementOrder,
	Summary:   "a function's guard prologue and its result are set off by a blank line",
	Default:   diag.Warning,
	Doc: `A run of guard clauses, and a multi-line result, are separated from the
body by a blank line.

	func MakeBSpline(...) (BSpline, error) {   violation
		if degree < 1 {
			return BSpline{}, errDegree
		}
		if len(ctrl) < degree+1 {
			return BSpline{}, errControl
		}
		w, rational, err := normalizeWeights(weights, len(ctrl))
		if err != nil {
			return BSpline{}, err
		}
		return BSpline{
			ctrl:   ctrl,
			degree: degree,
		}, nil
	}

	func MakeBSpline(...) (BSpline, error) {   OK
		if degree < 1 {
			return BSpline{}, errDegree
		}
		if len(ctrl) < degree+1 {
			return BSpline{}, errControl
		}

		w, rational, err := normalizeWeights(weights, len(ctrl))
		if err != nil {
			return BSpline{}, err
		}

		return BSpline{
			ctrl:   ctrl,
			degree: degree,
		}, nil
	}

Rationale: a function that validates, works, and returns is three things, and
the reader is looking for exactly one of them. Run together, finding the work
means scanning every guard to check it is a guard, because at a glance a
rejected precondition and the real computation are the same shape — an if with a
return in it. Two blank lines cost nothing and answer the question before it is
asked. This is the one piece of vertical structure gofmt has no opinion on: it
never inserts a blank line between statements and never removes one, so where
the sections fall is left entirely to the author, which is why it drifts.

To fix: put a blank line after the last guard, and before the returned result.

A guard clause is an if with no else whose body ends in return, break, continue,
goto or panic. A comment between two statements counts as a boundary, since it
divides the sections at least as clearly as a blank line does.

Configure in .goorg.yaml:

	settings:
	  logic/section-spacing:
	    min_guards: 2
	    min_result_lines: 3
	    min_body_statements: 3`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := sectionSpacingSettings{MinGuards: 2, MinResultLines: 3, MinBodyStatements: 3}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				body := functionBody(n)
				if body == nil {
					return true
				}
				for _, b := range missingBoundaries(c, body, s) {
					out = append(out, diag.Diagnostic{
						Position: c.Pos(body.List[b.at]),
						Message:  b.message,
						Help:     b.help,
					})
				}
				return true
			})
		}
		return out
	},
}

// boundary is a place in a function body where a section changes, identified by
// the index of the statement that opens the new section.
type boundary struct {
	at      int
	message string
	help    string
}

// missingBoundaries returns the section boundaries a body needs but does not
// draw.
//
// Both checks contribute to one set keyed by statement index. A function whose
// guards run straight into a multi-line return has one missing blank line, and
// keying on the index is what makes that one finding rather than two reports of
// the same gap.
func missingBoundaries(c *rule.Context, body *ast.BlockStmt, s sectionSpacingSettings) []boundary {
	var out []boundary
	seen := map[int]bool{}
	// Appended in statement order, so the output does not depend on map
	// iteration; seen is membership only.
	add := func(b boundary) {
		if seen[b.at] || separated(c, body, b.at) {
			return
		}
		seen[b.at] = true
		out = append(out, b)
	}

	if n := guardPrologue(c, body, s); n > 0 {
		add(boundary{
			at:      n,
			message: fmt.Sprintf("the body follows %s with no blank line", plural(n, "guard clause")),
			help:    "insert a blank line after the last guard",
		})
	}
	if at := trailingResult(c, body, s); at > 0 {
		add(boundary{
			at:      at,
			message: "the returned result follows the body with no blank line",
			help:    "insert a blank line before the return",
		})
	}
	return out
}

// separated reports whether a section boundary is drawn before statement i.
//
// gofmt leaves nothing but blank lines and comments between two statements, so
// a line gap wider than one means something divides them. A comment is not a
// blank line, but it separates the sections at least as clearly, and demanding
// a blank line as well would be goorg arguing with an author who has already
// made the boundary explicit.
func separated(c *rule.Context, body *ast.BlockStmt, i int) bool {
	if i <= 0 || i >= len(body.List) {
		return true
	}
	above := c.Project.Position(body.List[i-1].End()).Line
	at := c.Project.Position(body.List[i].Pos()).Line
	return at-above > 1
}

// guardPrologue returns the length of the leading run of guard clauses, which
// is also the index of the first statement after it. It returns 0 when there is
// no run worth setting off.
func guardPrologue(c *rule.Context, body *ast.BlockStmt, s sectionSpacingSettings) int {
	if s.MinGuards <= 0 {
		return 0
	}
	n := 0
	for _, stmt := range body.List {
		if !isGuardClause(stmt) {
			break
		}
		n++
	}
	// A function that is nothing but guards has no body to set them off from.
	if n < s.MinGuards || n >= len(body.List) {
		return 0
	}
	// Neither has one whose guards are followed by a lone one-line statement:
	// the `return nil` closing a run of validations is the result, and whether
	// that wants separating is the result check's call, not this one's.
	if len(body.List)-n == 1 && spanOf(c, body.List[n]) < 2 {
		return 0
	}
	return n
}

// trailingResult returns the index of a multi-line trailing return that has
// enough work above it to be worth separating, or 0.
func trailingResult(c *rule.Context, body *ast.BlockStmt, s sectionSpacingSettings) int {
	if s.MinResultLines <= 0 || len(body.List) <= s.MinBodyStatements {
		return 0
	}
	at := len(body.List) - 1
	result, ok := body.List[at].(*ast.ReturnStmt)
	if !ok {
		return 0
	}
	if spanOf(c, result) < s.MinResultLines {
		return 0
	}
	return at
}

// spanOf returns how many lines a statement occupies.
func spanOf(c *rule.Context, stmt ast.Stmt) int {
	return c.Project.Position(stmt.End()).Line - c.Project.Position(stmt.Pos()).Line + 1
}

// isGuardClause reports whether a statement is a conditional early exit.
func isGuardClause(stmt ast.Stmt) bool {
	cond, ok := stmt.(*ast.IfStmt)
	if !ok || cond.Else != nil || cond.Body == nil {
		return false
	}
	return exitsEarly(cond.Body.List)
}

// exitsEarly reports whether a block leaves rather than falling through.
//
// Only the forms the parser can confirm count. os.Exit and log.Fatal leave too,
// but the syntax tier cannot tell either from any other method with that name,
// and reading one wrongly would move where the rule thinks the prologue ends.
func exitsEarly(list []ast.Stmt) bool {
	if len(list) == 0 {
		return false
	}
	switch last := list[len(list)-1].(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return true
	case *ast.ExprStmt:
		call, ok := last.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		name, ok := call.Fun.(*ast.Ident)
		return ok && name.Name == "panic"
	default:
		return false
	}
}

// functionBody returns the body of a function declaration or literal, or nil.
func functionBody(n ast.Node) *ast.BlockStmt {
	switch fn := n.(type) {
	case *ast.FuncDecl:
		return fn.Body
	case *ast.FuncLit:
		return fn.Body
	default:
		return nil
	}
}
