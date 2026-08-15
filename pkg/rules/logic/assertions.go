package logic

import (
	"go/ast"
	"go/token"

	"github.com/Quikcad/goorg/pkg/source/typed"
)

// assertion records a compile-time `var _ I = T(...)` pairing, which is what
// logic/interface-registry looks for before reporting a missing one.
type assertion struct {
	iface string
	named string
}

// assertionsIn collects the `var _ I = T(...)` pairings a package declares.
func assertionsIn(pkg *typed.Package) map[assertion]bool {
	out := map[assertion]bool{}
	for _, file := range pkg.Syntax {
		for _, vs := range blankVarSpecs(file) {
			recordAssertion(pkg, vs, out)
		}
	}
	return out
}

// blankVarSpecs returns the `var _ T = ...` specs of a file.
func blankVarSpecs(file *ast.File) []*ast.ValueSpec {
	var out []*ast.ValueSpec
	for _, node := range file.Decls {
		gen, ok := node.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if ok && vs.Type != nil && allBlank(vs.Names) {
				out = append(out, vs)
			}
		}
	}
	return out
}

func recordAssertion(pkg *typed.Package, vs *ast.ValueSpec, out map[assertion]bool) {
	ifaceType := pkg.Info.TypeOf(vs.Type)
	if ifaceType == nil {
		return
	}
	iface := qualifiedName(ifaceType)
	for _, value := range vs.Values {
		valueType := pkg.Info.TypeOf(value)
		if valueType == nil {
			continue
		}
		if named := underlyingNamed(valueType); named != "" {
			out[assertion{iface: iface, named: named}] = true
		}
	}
}

func allBlank(names []*ast.Ident) bool {
	for _, n := range names {
		if n.Name != "_" {
			return false
		}
	}
	return len(names) > 0
}
