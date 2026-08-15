// Package decl answers questions about Go declarations that more than one rule
// family needs to ask.
//
// It exists because the same classifiers kept being needed twice:
// org/member-order and logic/iota-candidate both have to recognise an enum,
// org/type-cohesion and pat/factory-naming both have to recognise a factory.
// Duplicating them would let the two copies drift, and two rules disagreeing
// about what a factory *is* produces contradictory findings on the same code.
package decl

import (
	"go/ast"
	"go/token"
)

// EnumTypes returns the named types that have a const block declaring values of
// that type in the same file.
//
// This is the definition of "enum" shared by org/member-order, which places an
// enum's type declaration beside its constants, and logic/iota-candidate, which
// reports a hand-numbered run that should use iota.
func EnumTypes(f *ast.File) map[string]bool {
	out := map[string]bool{}
	for _, node := range f.Decls {
		d, ok := node.(*ast.GenDecl)
		if !ok || d.Tok != token.CONST {
			continue
		}
		for _, spec := range d.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if id, ok := vs.Type.(*ast.Ident); ok {
				out[id.Name] = true
			}
		}
	}
	return out
}

// LocalTypes returns the type names declared in a file.
func LocalTypes(f *ast.File) map[string]bool {
	out := map[string]bool{}
	for _, node := range f.Decls {
		d, ok := node.(*ast.GenDecl)
		if !ok || d.Tok != token.TYPE {
			continue
		}
		for _, spec := range d.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				out[ts.Name.Name] = true
			}
		}
	}
	return out
}

// IsInterface reports whether a type declaration declares an interface.
func IsInterface(spec *ast.TypeSpec) bool {
	_, ok := spec.Type.(*ast.InterfaceType)
	return ok
}
