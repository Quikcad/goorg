package logic

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

// numericWidth orders the numeric types within a family, so that narrowing can
// be told from widening. A type absent from this table is never proposed.
var numericWidth = map[string]int{
	"float32": 1, "float64": 2,
	"int8": 1, "int16": 2, "int32": 3, "int": 4, "int64": 4,
	"uint8": 1, "uint16": 2, "uint32": 3, "uint": 4, "uint64": 4,
}

// numericFamily groups types that can sensibly be swapped for one another. A
// value is only ever compared within its own family.
var numericFamily = map[string]string{
	"float32": "float", "float64": "float",
	"int8": "signed", "int16": "signed", "int32": "signed", "int": "signed", "int64": "signed",
	"uint8": "unsigned", "uint16": "unsigned", "uint32": "unsigned", "uint": "unsigned", "uint64": "unsigned",
}

// numericTypeSettings is the configurable surface of the rule.
type numericTypeSettings struct {
	// MinSavings is how many conversions a change must remove before the rule
	// says anything.
	MinSavings int `yaml:"min_savings"`
	// AllowNarrowing permits proposing a narrower type. It must stay false:
	// fewer conversions must never argue for silently losing precision.
	AllowNarrowing bool `yaml:"allow_narrowing"`
	// ExportedOnly limits the rule to exported signatures, which are API.
	ExportedOnly bool `yaml:"exported_only"`
}

// conversionTally counts what a value is converted to at its use sites.
type conversionTally struct {
	name    string
	have    string
	pos     diag.Position
	targets map[string]int
}

var idealNumericType = &rule.Rule{
	ID:       "logic/ideal-numeric-type",
	Category: rule.Logic,
	Tier:     rule.Types,
	// A heuristic that cannot see precision requirements must never fail a
	// build. This ships as a warning and should stay one.
	Default: diag.Warning,
	Summary: "a numeric parameter should be the type its uses already speak",
	Doc: `Choose the numeric type that minimises conversions at the value's use
sites.

	func scale(v float64) {              violation — every use converts
		tex.SetScale(float32(v))
		mesh.Resize(float32(v), 1.0)
		buf.PutFloat32(float32(v))
	}

	func scale(v float32) { ... }        OK — zero conversions

Rationale: a numeric type declared once and converted at every use is a type
that was chosen by habit rather than by the code that consumes it. Each
conversion is noise at the call site, and each is a place where a narrowing
conversion can silently lose precision or overflow without any diagnostic.
Counting conversions turns "what type should this be?" from a matter of taste
into a measurement: the right type is the one the surrounding code already
speaks.

To fix: declare the parameter as the type its uses convert it to, and delete the
conversions.

Conversion count is a proxy, not an answer. Precision requirements, memory
layout in large arrays, and the demands of an external API are all invisible to
it. allow_narrowing is false and should stay false — "fewer conversions" must
never be allowed to argue for silently losing precision, so the rule only ever
proposes a type at least as wide as the one declared. It ships as a warning for
the same reason.

Configure in .goorg.yaml:

	settings:
	  logic/ideal-numeric-type:
	    min_savings: 2
	    allow_narrowing: false`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := numericTypeSettings{MinSavings: 2, AllowNarrowing: false}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.MinSavings < 1 {
			s.MinSavings = 1
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkNumericTypes(pkg, &s)...)
		}
		diag.Sort(out)
		return out
	},
}

// checkNumericTypes reports parameters whose uses all convert them away.
func checkNumericTypes(pkg *typed.Package, s *numericTypeSettings) []diag.Diagnostic {
	var out []diag.Diagnostic
	for i, file := range pkg.Syntax {
		if i < len(pkg.Files) && isTestFile(pkg.Files[i]) {
			continue
		}
		for _, node := range file.Decls {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if s.ExportedOnly && !fn.Name.IsExported() {
				continue
			}
			out = append(out, checkFuncParams(pkg, fn, s)...)
		}
	}
	return out
}

// checkFuncParams tallies conversions of each numeric parameter in one body.
func checkFuncParams(pkg *typed.Package, fn *ast.FuncDecl, s *numericTypeSettings) []diag.Diagnostic {
	tallies := numericParams(pkg, fn)
	if len(tallies) == 0 {
		return nil
	}
	countConversions(pkg, fn.Body, tallies)

	names := make([]string, 0, len(tallies))
	for name := range tallies {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []diag.Diagnostic
	for _, name := range names {
		t := tallies[name]
		want, savings := bestTarget(t, s)
		if want == "" || savings < s.MinSavings {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: t.pos,
			Message: fmt.Sprintf("%s is %s but every use converts it to %s",
				name, t.have, want),
			Help: fmt.Sprintf("declare it as %s and delete the %s", want, plural(savings, "conversion")),
		})
	}
	return out
}

// numericParams returns the tally slots for a function's numeric parameters.
func numericParams(pkg *typed.Package, fn *ast.FuncDecl) map[string]*conversionTally {
	if fn.Type.Params == nil {
		return nil
	}
	out := map[string]*conversionTally{}
	for _, field := range fn.Type.Params.List {
		t := pkg.Info.TypeOf(field.Type)
		basic, ok := t.(*types.Basic)
		if !ok || numericFamily[basic.Name()] == "" {
			continue
		}
		for _, name := range field.Names {
			if name.Name == "_" {
				continue
			}
			out[name.Name] = &conversionTally{
				name:    name.Name,
				have:    basic.Name(),
				pos:     positionOf(pkg, name.Pos()),
				targets: map[string]int{},
			}
		}
	}
	return out
}

// countConversions records every T(param) conversion in a body.
func countConversions(pkg *typed.Package, body *ast.BlockStmt, tallies map[string]*conversionTally) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		target, ok := call.Fun.(*ast.Ident)
		if !ok || numericFamily[target.Name] == "" {
			return true
		}
		// A call to an identifier naming a type is a conversion; one naming a
		// function is not, and only the type checker can tell them apart.
		if _, isType := pkg.Info.Uses[target].(*types.TypeName); !isType {
			return true
		}
		arg, ok := call.Args[0].(*ast.Ident)
		if !ok {
			return true
		}
		if t, tracked := tallies[arg.Name]; tracked {
			t.targets[target.Name]++
		}
		return true
	})
}

// bestTarget returns the type most conversions ask for, and how many would go
// away, or "" when no proposal is safe.
func bestTarget(t *conversionTally, s *numericTypeSettings) (string, int) {
	best, bestCount := "", 0
	targets := make([]string, 0, len(t.targets))
	for name := range t.targets {
		targets = append(targets, name)
	}
	sort.Strings(targets)

	for _, name := range targets {
		count := t.targets[name]
		switch {
		case name == t.have:
			continue
		case numericFamily[name] != numericFamily[t.have]:
			// Crossing families changes what the value means, not just how it
			// is stored.
			continue
		case !s.AllowNarrowing && numericWidth[name] < numericWidth[t.have]:
			// The whole point: fewer conversions must never argue for
			// silently losing precision.
			continue
		case count > bestCount:
			best, bestCount = name, count
		}
	}
	return best, bestCount
}
