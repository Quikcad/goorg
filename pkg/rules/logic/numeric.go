package logic

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

// signedWidth is the bit width of each sized integer type, used to tell a
// narrowing conversion from a widening one.
var signedWidth = map[string]int{
	"int8": 8, "int16": 16, "int32": 32, "int": 64, "int64": 64,
	"uint8": 8, "uint16": 16, "uint32": 32, "uint": 64, "uint64": 64,
	"byte": 8, "rune": 32,
}

var floatEquality = &rule.Rule{
	ID:       "logic/float-equality",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "floating-point values are compared with == or !=",
	Default:  diag.Error,
	Doc: `Floating-point values may not be compared with == or !=.

	if total == 0.3 {                violation
	if math.Abs(total-0.3) < epsilon {   OK

Rationale: 0.1 + 0.2 is not 0.3 in binary floating point, and no amount of
care at the comparison site changes that. The comparison is not merely
unreliable, it is unreliable in a way that depends on how the values were
computed, so it passes in a test with literal inputs and fails in production
with accumulated ones. That is the worst possible failure shape: correct-looking
code with a bug that only appears against real data.

To fix: compare against a tolerance, or hold the value as an integer of the
smallest unit — cents rather than dollars — where exactness is required.

Comparison against zero is still reported: it is exact for a value assigned zero
but not for one computed to it, and the distinction is invisible at the
comparison.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkFloatComparisons(pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

var lossyConversion = &rule.Rule{
	ID:       "logic/lossy-conversion",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "a narrowing numeric conversion has no range check",
	Default:  diag.Error,
	Doc: `A conversion to a narrower numeric type needs a range check first.

	var n int64 = readLength()
	buf := make([]byte, int32(n))    violation — silently truncates above 2^31

Rationale: Go's numeric conversions never fail. int32(n) on a value that does
not fit produces a different number and carries on, so an oversized length
becomes a small one, a positive becomes a negative, and the program continues
with data that is wrong rather than stopping. Nothing in the source marks the
moment it happened, so the symptom always appears somewhere else.

To fix: check the range before converting, or hold the value in a type wide
enough that it cannot overflow.

A conversion of an untyped constant is exempt: the compiler verifies those fit.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkNarrowing(pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

var unsignedUnderflow = &rule.Rule{
	ID:       "logic/unsigned-underflow",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "subtraction on an unsigned type can wrap to a huge value",
	Default:  diag.Warning,
	Doc: `Subtraction on an unsigned type wraps rather than going negative.

	var n uint = len(items)
	last := n - 1                    violation — 0 - 1 is 18446744073709551615

Rationale: uint(0) - 1 is not -1 and is not an error; it is the largest uint.
The result is then used as a length, an index or a loop bound, and the failure
surfaces as an enormous allocation or an out-of-range panic far from the
subtraction. Because the wrap is defined behaviour there is no diagnostic at any
point, and because it only triggers at zero it survives every test with
non-empty input.

To fix: guard the subtraction, or hold the value in a signed type where going
below zero is representable and visible.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkUnsignedSubtraction(pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

var integerDivisionToFloat = &rule.Rule{
	ID:       "logic/integer-division-to-float",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "integer division converted to a float truncates first",
	Default:  diag.Error,
	Doc: `Converting the result of an integer division to a float truncates before
converting.

	ratio := float64(hits / total)         violation — 7/10 is 0, so ratio is 0
	ratio := float64(hits) / float64(total)    OK

Rationale: the conversion is applied to the quotient, not the operands, so the
fraction is gone before the float ever exists. The result is not imprecise, it
is wrong — every ratio below one becomes zero. The expression reads as though
it produces a fraction, which is why this survives review, and it produces
plausible output (0, or 1) rather than an obvious error.

To fix: convert the operands, not the result.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkIntegerDivision(pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

// checkFloatComparisons reports == and != between floating-point values.
func checkFloatComparisons(pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	forEachExpr(pkg, func(file *ast.File, n ast.Node) {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
			return
		}
		if !isFloat(pkg.Info.TypeOf(bin.X)) || !isFloat(pkg.Info.TypeOf(bin.Y)) {
			return
		}
		out = append(out, diag.Diagnostic{
			Position: positionOf(pkg, bin.OpPos),
			Message:  "floating-point values compared for exact equality",
			Help:     "compare against a tolerance, or hold the value as an integer of the smallest unit",
		})
	})
	return out
}

// checkNarrowing reports conversions to a narrower integer type.
func checkNarrowing(pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	forEachExpr(pkg, func(file *ast.File, n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return
		}
		target, ok := call.Fun.(*ast.Ident)
		if !ok {
			return
		}
		if _, isType := pkg.Info.Uses[target].(*types.TypeName); !isType {
			return
		}
		// An untyped constant argument is checked by the compiler.
		if isConstant(pkg, call.Args[0]) {
			return
		}
		from := basicName(pkg.Info.TypeOf(call.Args[0]))
		to := target.Name
		if signedWidth[from] == 0 || signedWidth[to] == 0 || signedWidth[to] >= signedWidth[from] {
			return
		}
		out = append(out, diag.Diagnostic{
			Position: positionOf(pkg, call.Pos()),
			Message:  fmt.Sprintf("%s to %s truncates silently when the value does not fit", from, to),
			Help:     "check the range before converting, or use a wider type",
		})
	})
	return out
}

// checkUnsignedSubtraction reports subtraction on an unsigned operand.
func checkUnsignedSubtraction(pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	forEachExpr(pkg, func(file *ast.File, n ast.Node) {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok || bin.Op != token.SUB {
			return
		}
		if !isUnsigned(pkg.Info.TypeOf(bin.X)) {
			return
		}
		out = append(out, diag.Diagnostic{
			Position: positionOf(pkg, bin.OpPos),
			Message:  "subtraction on an unsigned value wraps instead of going negative",
			Help:     "guard the subtraction, or hold the value in a signed type",
		})
	})
	return out
}

// checkIntegerDivision reports float(a / b) where a and b are integers.
func checkIntegerDivision(pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	forEachExpr(pkg, func(file *ast.File, n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return
		}
		target, ok := call.Fun.(*ast.Ident)
		if !ok || (target.Name != "float32" && target.Name != "float64") {
			return
		}
		if _, isType := pkg.Info.Uses[target].(*types.TypeName); !isType {
			return
		}
		inner, ok := unparen(call.Args[0]).(*ast.BinaryExpr)
		if !ok || inner.Op != token.QUO {
			return
		}
		if !isInteger(pkg.Info.TypeOf(inner.X)) || !isInteger(pkg.Info.TypeOf(inner.Y)) {
			return
		}
		out = append(out, diag.Diagnostic{
			Position: positionOf(pkg, call.Pos()),
			Message:  "integer division is truncated before the conversion to float",
			Help:     "convert the operands, not the result",
		})
	})
	return out
}

// forEachExpr walks every non-test file of a package.
func forEachExpr(pkg *typed.Package, visit func(*ast.File, ast.Node)) {
	for i, file := range pkg.Syntax {
		if i < len(pkg.Files) && isTestFile(pkg.Files[i]) {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if n != nil {
				visit(file, n)
			}
			return true
		})
	}
}

func isConstant(pkg *typed.Package, expr ast.Expr) bool {
	tv, ok := pkg.Info.Types[expr]
	return ok && tv.Value != nil
}

func basicName(t types.Type) string {
	if t == nil {
		return ""
	}
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return ""
	}
	return basic.Name()
}

func isFloat(t types.Type) bool {
	return hasBasicInfo(t, types.IsFloat)
}

func isInteger(t types.Type) bool {
	return hasBasicInfo(t, types.IsInteger)
}

func isUnsigned(t types.Type) bool {
	return hasBasicInfo(t, types.IsUnsigned)
}

func hasBasicInfo(t types.Type, want types.BasicInfo) bool {
	if t == nil {
		return false
	}
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Info()&want != 0
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}
