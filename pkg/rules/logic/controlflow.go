package logic

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

var negatedCondition = &rule.Rule{
	ID:       "logic/negated-condition",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "an if/else on a negated condition should be flipped",
	Default:  diag.Warning,
	Doc: `When an if has an else, its condition is written positively.

	if !ok {                         violation
		handleFailure()
	} else {
		handleSuccess()
	}

	if ok {                          OK
		handleSuccess()
	} else {
		handleFailure()
	}

Rationale: with an else branch, a negated condition makes the reader hold "not
this" while reading the first block and then invert it again for the second.
The negation buys nothing — the branches simply swap — so it is pure cost. It
also reads worst in exactly the case it appears most: !ok, where the successful
path is the one the reader came to find and is now second.

To fix: drop the negation and swap the branches.

An if with no else is untouched; there a negated guard is often the clearest
form, and logic/prefer-guard-clause actively wants it.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, f := range files(c) {
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				stmt, ok := n.(*ast.IfStmt)
				if !ok || stmt.Else == nil {
					return true
				}
				// else-if is a chain, not a negation to flip.
				if _, chained := stmt.Else.(*ast.IfStmt); chained {
					return true
				}
				if !isNegation(stmt.Cond) {
					return true
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(stmt.Cond),
					Message:  "condition is negated although the if has an else",
					Help:     "drop the negation and swap the branches",
				})
				return true
			})
		}
		return out
	},
}

var singleCaseSwitch = &rule.Rule{
	ID:       "logic/single-case-switch",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a switch with one case is an if, or a missing case",
	Default:  diag.Warning,
	Doc: `A switch with a single case and no default is either an if written the
long way, or a switch somebody forgot to finish.

	switch s {                       violation
	case Draft:
		draft()
	}

Rationale: the two readings have opposite fixes, and neither is what the code
says. If one case is genuinely all there is, an if states that plainly and
costs three fewer lines. If it is not — and a switch on an enum with one case
usually is not — the missing branches are silently doing nothing, which is the
same failure a non-exhaustive switch has and is just as invisible.

To fix: collapse it to an if, or add the cases that are missing.

A type switch is exempt: a single-case type switch is the idiomatic way to test
one dynamic type and bind the result.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, f := range files(c) {
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				stmt, ok := n.(*ast.SwitchStmt)
				if !ok || stmt.Body == nil || len(stmt.Body.List) != 1 {
					return true
				}
				clause, ok := stmt.Body.List[0].(*ast.CaseClause)
				if !ok || clause.List == nil {
					// A lone default is not a single case.
					return true
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(stmt),
					Message:  "switch has a single case and no default",
					Help:     "collapse it to an if, or add the cases that are missing",
				})
				return true
			})
		}
		return out
	},
}

var identicalBranches = &rule.Rule{
	ID:       "logic/identical-branches",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "two branches of one conditional have identical bodies",
	Default:  diag.Error,
	Doc: `The branches of an if/else may not have identical bodies.

	if fast {                        violation
		render(x)
	} else {
		render(x)
	}

Rationale: either the condition does not matter, in which case the branch is
noise concealing that fact, or one branch was meant to differ and the edit was
never made. Both are bugs, and the second is the dangerous one: the code reads
as though it handles two cases and silently handles one. Copy-paste between
branches is how it happens, which is why the bodies are usually long enough that
nobody notices they match.

To fix: delete the conditional if the branches really are the same, or write the
branch that was meant to differ.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, f := range files(c) {
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				stmt, ok := n.(*ast.IfStmt)
				if !ok || stmt.Else == nil {
					return true
				}
				other, ok := stmt.Else.(*ast.BlockStmt)
				if !ok || len(stmt.Body.List) == 0 {
					return true
				}
				if render(f.Syntax, stmt.Body) != render(f.Syntax, other) {
					return true
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(stmt),
					Message:  "both branches of the conditional are identical",
					Help:     "delete the conditional, or write the branch that was meant to differ",
				})
				return true
			})
		}
		return out
	},
}

var emptyBranch = &rule.Rule{
	ID:       "logic/empty-branch",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "an empty branch needs a comment saying why",
	Default:  diag.Warning,
	Doc: `A branch with an empty body carries a comment explaining the omission.

	if err != nil {                  violation — deliberate, or forgotten?
	}

	if err != nil {                  OK
		// The cache is advisory; a write failure is not worth reporting.
	}

Rationale: an empty branch is indistinguishable from an unfinished one. The
reader cannot tell whether doing nothing is the decision or the bug, and the
cost of guessing wrong is high in exactly the place empty branches appear most —
error handling. A comment converts an omission into a decision somebody can
disagree with.

To fix: say why nothing happens, or delete the branch.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, f := range files(c) {
			commented := commentedLines(c, f.Syntax)
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				block, ok := emptyBlockOf(n)
				if !ok {
					return true
				}
				open := c.Project.Position(block.Lbrace).Line
				close := c.Project.Position(block.Rbrace).Line
				if hasCommentBetween(commented, open, close) {
					return true
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(block),
					Message:  "branch body is empty with no explanation",
					Help:     "say why nothing happens here, or delete the branch",
				})
				return true
			})
		}
		return out
	},
}

// nestingSettings is the configurable surface of logic/max-nesting-depth.
type nestingSettings struct {
	Max int `yaml:"max"`
}

// ifChainSettings is the configurable surface of logic/if-chain-to-switch.
type ifChainSettings struct {
	// MinBranches is how long a chain must be before a switch is clearer.
	MinBranches int `yaml:"min_branches"`
}

var maxNestingDepth = &rule.Rule{
	ID:       "logic/max-nesting-depth",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "block nesting stays within a configured depth",
	Default:  diag.Error,
	Doc: `Blocks may not nest beyond N levels inside a function.

	for _, x := range xs {           depth 1
		if x.ok {                    depth 2
			for _, y := range x.ys { depth 3
				if y.ok {            depth 4 — over a limit of 3
				}
			}
		}
	}

Rationale: nesting depth is the single best predictor of a function nobody
wants to touch. Each level adds a condition the reader must keep true in their
head for everything below it, and they compound rather than add — four levels
is sixteen paths to hold, not four. Depth is also what makes a function
untestable, because reaching the innermost branch requires satisfying every
condition above it at once.

To fix: invert the outer conditions into guard clauses, or extract the inner
block into a named function. logic/prefer-guard-clause reports the specific case
where the whole body is wrapped.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := nestingSettings{Max: 3}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.Max <= 0 {
			return nil
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				fn, ok := node.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				if deepest, at := maxDepth(fn.Body, 0); deepest > s.Max {
					out = append(out, diag.Diagnostic{
						Position: c.Pos(at),
						Message: fmt.Sprintf("%s nests %d levels deep, over the limit of %d",
							fn.Name.Name, deepest, s.Max),
						Help: "invert the outer conditions into guards, or extract the inner block",
					})
				}
			}
		}
		return out
	},
}

var ifChainToSwitch = &rule.Rule{
	ID:       "logic/if-chain-to-switch",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a long else-if chain on one operand should be a switch",
	Default:  diag.Warning,
	Doc: `Three or more else-if branches comparing the same operand are a switch.

	if s == Draft {                  violation
	} else if s == Sent {
	} else if s == Paid {
	}

	switch s {                       OK
	case Draft:
	case Sent:
	case Paid:
	}

Rationale: a switch states its shape — one operand, a set of cases — where a
chain only implies it, and the reader has to check each condition to confirm
that they really do all test the same thing. The switch form is also the one
exhaustiveness checking can read, so converting is what makes a missing case
detectable at all rather than a branch that silently never runs.

To fix: rewrite the chain as a switch on the shared operand.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := ifChainSettings{MinBranches: 3}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				stmt, ok := n.(*ast.IfStmt)
				if !ok {
					return true
				}
				operand, branches := chainOperand(f.Syntax, stmt)
				if operand == "" || branches < s.MinBranches {
					return true
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(stmt),
					Message: fmt.Sprintf("%s branches all compare %s",
						plural(branches, "if"), operand),
					Help: "rewrite the chain as a switch on that operand",
				})
				// The chain is one finding, not one per branch.
				return false
			})
		}
		return out
	},
}

// maxDepth returns the deepest nesting inside a block and the node at it.
func maxDepth(block *ast.BlockStmt, depth int) (int, ast.Node) {
	deepest, at := depth, ast.Node(block)
	for _, stmt := range block.List {
		inner, node := nestedBlock(stmt)
		if inner == nil {
			continue
		}
		d, n := maxDepth(inner, depth+1)
		if d > deepest {
			deepest, at = d, n
			if n == nil {
				at = node
			}
		}
	}
	return deepest, at
}

// nestedBlock returns the block a statement introduces, if it introduces one.
func nestedBlock(stmt ast.Stmt) (*ast.BlockStmt, ast.Node) {
	switch s := stmt.(type) {
	case *ast.IfStmt:
		return s.Body, s
	case *ast.ForStmt:
		return s.Body, s
	case *ast.RangeStmt:
		return s.Body, s
	case *ast.BlockStmt:
		return s, s
	default:
		return nil, nil
	}
}

// chainOperand returns the operand every branch of an else-if chain compares,
// and how many branches there are.
func chainOperand(file *ast.File, stmt *ast.IfStmt) (string, int) {
	operand := comparedOperand(file, stmt.Cond)
	if operand == "" {
		return "", 0
	}
	branches := 1
	for next := stmt.Else; next != nil; {
		chained, ok := next.(*ast.IfStmt)
		if !ok {
			break
		}
		if comparedOperand(file, chained.Cond) != operand {
			return "", 0
		}
		branches++
		next = chained.Else
	}
	return operand, branches
}

// comparedOperand returns the left side of an equality comparison, rendered.
func comparedOperand(file *ast.File, cond ast.Expr) string {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return ""
	}
	return render(file, bin.X)
}

func isNegation(cond ast.Expr) bool {
	unary, ok := cond.(*ast.UnaryExpr)
	return ok && unary.Op == token.NOT
}

// emptyBlockOf returns the empty body of a branching statement.
func emptyBlockOf(n ast.Node) (*ast.BlockStmt, bool) {
	var block *ast.BlockStmt
	switch s := n.(type) {
	case *ast.IfStmt:
		block = s.Body
	case *ast.ForStmt:
		block = s.Body
	case *ast.RangeStmt:
		block = s.Body
	default:
		return nil, false
	}
	if block == nil || len(block.List) != 0 {
		return nil, false
	}
	return block, true
}

func commentedLines(c *rule.Context, file *ast.File) map[int]bool {
	out := map[int]bool{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			out[c.Project.Position(comment.Pos()).Line] = true
		}
	}
	return out
}

func hasCommentBetween(lines map[int]bool, from, to int) bool {
	for line := from; line <= to; line++ {
		if lines[line] {
			return true
		}
	}
	return false
}

// render prints a node back to source, so two subtrees can be compared for
// structural equality without writing a recursive comparison for every node.
func render(file *ast.File, n ast.Node) string {
	var b strings.Builder
	// A fresh FileSet renders positions relatively, so two identical subtrees
	// at different offsets produce the same text.
	if err := printer.Fprint(&b, token.NewFileSet(), n); err != nil {
		return ""
	}
	return b.String()
}
