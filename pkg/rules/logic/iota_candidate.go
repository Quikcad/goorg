package logic

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// iotaCandidateSettings is the configurable surface of the rule.
type iotaCandidateSettings struct {
	// MinConstants is the shortest run worth reporting.
	MinConstants int `yaml:"min_constants"`
	// AllowOffsets also reports runs that start elsewhere or step by a
	// constant amount, which iota expresses as iota+1 or iota*8.
	AllowOffsets bool `yaml:"allow_offsets"`
	// RequireNamedType additionally reports a run of untyped constants.
	RequireNamedType bool `yaml:"require_named_type"`
}

var iotaCandidate = &rule.Rule{
	ID:       "logic/iota-candidate",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a run of consecutive integer constants should use iota",
	Default:  diag.Error,
	Doc: `A run of related constants with consecutive literal values should use
iota.

	const (                              violation
		StatusPending  = 0
		StatusActive   = 1
		StatusArchived = 2
	)

	type Status int                      OK
	const (
		StatusPending Status = iota
		StatusActive
		StatusArchived
	)

Rationale: hand-numbered constants have to be renumbered by hand. Inserting a
value in the middle means editing every line below it, and the one that gets
missed produces two constants with the same value — which is not a compile
error, and which turns a switch into a silently unreachable branch.

iota also creates the pressure to declare a named type, and a named type is what
makes the enum checkable at all: func SetStatus(s Status) cannot be handed a
stray int, and Status can carry a String method.

To fix: give the run a named type and replace the literals with iota.

Configure in .goorg.yaml:

	settings:
	  logic/iota-candidate:
	    min_constants: 3
	    allow_offsets: true
	    require_named_type: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := iotaCandidateSettings{MinConstants: 3, AllowOffsets: true, RequireNamedType: true}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.MinConstants < 2 {
			s.MinConstants = 2
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				d, ok := node.(*ast.GenDecl)
				if !ok || d.Tok != token.CONST || len(d.Specs) < s.MinConstants {
					continue
				}
				if finding := checkConstBlock(c, d, &s); finding != nil {
					out = append(out, *finding)
				}
			}
		}
		return out
	},
}

// checkConstBlock reports a const block whose values form an arithmetic
// sequence written out by hand.
func checkConstBlock(c *rule.Context, d *ast.GenDecl, s *iotaCandidateSettings) *diag.Diagnostic {
	values, typed, ok := constRun(d)
	if !ok || len(values) < s.MinConstants {
		return nil
	}
	step, ok := arithmeticStep(values)
	if !ok {
		return nil
	}
	if !s.AllowOffsets && (values[0] != 0 || step != 1) {
		return nil
	}

	message := fmt.Sprintf("%s are numbered by hand and should use iota", plural(len(values), "constant"))
	help := "replace the literals with iota"
	if s.RequireNamedType && !typed {
		message += ", with a named type"
		help = "declare a named type for the run and replace the literals with iota"
	}
	return &diag.Diagnostic{
		Position: c.Pos(d),
		Message:  message,
		Help:     help,
	}
}

// constRun extracts the integer values of a const block, reporting whether
// every spec carries one and whether the run has a named type.
func constRun(d *ast.GenDecl) (values []int64, typed bool, ok bool) {
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Values) != 1 || len(vs.Names) != 1 {
			return nil, false, false
		}
		if vs.Type != nil {
			typed = true
		}
		n, ok := intLiteral(vs.Values[0])
		if !ok {
			return nil, false, false
		}
		values = append(values, n)
	}
	return values, typed, len(values) > 0
}

// arithmeticStep returns the common difference of a sequence, if it has one.
// A step of zero is rejected: repeated values are a bug, not an enum.
func arithmeticStep(values []int64) (int64, bool) {
	if len(values) < 2 {
		return 0, false
	}
	step := values[1] - values[0]
	if step == 0 {
		return 0, false
	}
	for i := 2; i < len(values); i++ {
		if values[i]-values[i-1] != step {
			return 0, false
		}
	}
	return step, true
}

// intLiteral reads a signed integer literal.
func intLiteral(e ast.Expr) (int64, bool) {
	if unary, ok := e.(*ast.UnaryExpr); ok && unary.Op == token.SUB {
		n, ok := intLiteral(unary.X)
		return -n, ok
	}
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	n, err := strconv.ParseInt(lit.Value, 0, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
