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

// anyGenericSettings is the configurable surface of the rule.
type anyGenericSettings struct {
	CheckParams  bool `yaml:"check_params"`
	CheckResults bool `yaml:"check_results"`
	// IgnoreReflective skips functions whose body inspects the dynamic type,
	// which is what genuine heterogeneity looks like.
	IgnoreReflective bool `yaml:"ignore_reflective"`
	// ExportedOnly limits the rule to the API surface, where the erasure is
	// inflicted on callers rather than absorbed internally.
	ExportedOnly bool `yaml:"exported_only"`
}

var anyShouldBeGeneric = &rule.Rule{
	ID:       "logic/any-should-be-generic",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "any that only carries a value should be a type parameter",
	Default:  diag.Warning,
	Doc: `A function using any where a type parameter would preserve type
information should use a type parameter.

	func First(items []any) any       violation — the caller asserts the result back

	func First[T any](items []T) T    OK

Rationale: any erases the caller's type and makes them assert it back, moving a
compile-time guarantee to a runtime panic. The cases that read as "this must be
any" are usually the ones where a single type parameter threads the type through
untouched — the function never inspects the value, it only moves it.

To fix: introduce a type parameter and replace any with it.

Genuinely heterogeneous cases exist — fmt.Println, decoding into an unknown
shape — which is why the rule only fires when the body never inspects the
dynamic type. A type assertion, a type switch, or any use of reflect means the
function really does need the erased form, and the rule stays quiet.

This is a judgement call that will sometimes be wrong, so it ships as a warning.

Configure in .goorg.yaml:

	settings:
	  logic/any-should-be-generic:
	    check_params: true
	    check_results: true
	    ignore_reflective: true
	    exported_only: false`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := anyGenericSettings{
			CheckParams:      true,
			CheckResults:     true,
			IgnoreReflective: true,
			ExportedOnly:     false,
		}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkAnyUsage(pkg, &s)...)
		}
		diag.Sort(out)
		return out
	},
}

// checkAnyUsage reports functions whose any could be a type parameter.
func checkAnyUsage(pkg *typed.Package, s *anyGenericSettings) []diag.Diagnostic {
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
			// A function that is already generic has made the choice.
			if fn.Type.TypeParams != nil {
				continue
			}
			count := countAny(pkg, fn, s)
			if count == 0 {
				continue
			}
			if s.IgnoreReflective && inspectsDynamicType(fn) {
				continue
			}
			out = append(out, diag.Diagnostic{
				Position: positionOf(pkg, fn.Name.Pos()),
				Message: fmt.Sprintf("%s takes or returns %s without inspecting the value",
					fn.Name.Name, plural(count, "any")),
				Help: "introduce a type parameter so the caller keeps its type",
			})
		}
	}
	return out
}

// countAny counts the any-typed parameters and results of a function.
func countAny(pkg *typed.Package, fn *ast.FuncDecl, s *anyGenericSettings) int {
	count := 0
	if s.CheckParams && fn.Type.Params != nil {
		count += countAnyFields(pkg, fn.Type.Params.List)
	}
	if s.CheckResults && fn.Type.Results != nil {
		count += countAnyFields(pkg, fn.Type.Results.List)
	}
	return count
}

func countAnyFields(pkg *typed.Package, fields []*ast.Field) int {
	count := 0
	for _, field := range fields {
		if !isEmptyInterface(pkg.Info.TypeOf(field.Type)) {
			continue
		}
		if len(field.Names) == 0 {
			count++
			continue
		}
		count += len(field.Names)
	}
	return count
}

// isEmptyInterface reports whether a type is `any` or `interface{}`.
func isEmptyInterface(t types.Type) bool {
	if t == nil {
		return false
	}
	iface, ok := t.Underlying().(*types.Interface)
	return ok && iface.NumMethods() == 0 && iface.IsMethodSet()
}

// inspectsDynamicType reports whether a body examines what an any actually
// holds, which is what genuine heterogeneity looks like.
func inspectsDynamicType(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.TypeAssertExpr, *ast.TypeSwitchStmt:
			found = true
			return false
		case *ast.SelectorExpr:
			if id, ok := node.X.(*ast.Ident); ok && id.Name == "reflect" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func positionOf(pkg *typed.Package, pos token.Pos) diag.Position {
	p := pkg.Fset.Position(pos)
	return diag.Position{Path: pkg.FileOf(pos), Line: p.Line, Col: p.Column}
}
