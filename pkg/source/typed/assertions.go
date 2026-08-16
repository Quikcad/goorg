package typed

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
)

// Assertion is a compile-time `var _ I = (*T)(nil)` pairing.
//
// It is the only place Go source states that a type is *meant* to implement an
// interface — satisfaction is otherwise structural and accidental. More than
// one rule needs to read that intent, and they have to agree on what counts:
// logic/interface-registry reports a missing assertion, and
// org/interface-method-order acts on the ones that exist. If the two disagreed,
// adding the assertion the first demands might not enable the second.
type Assertion struct {
	// Interface is the asserted interface type.
	Interface *types.Named
	// Concrete is the type claimed to implement it, with any pointer stripped.
	Concrete *types.Named
}

// Assertions returns the interface assertions a package declares, sorted so
// that callers iterating them produce stable output.
func Assertions(p *Package) []Assertion {
	if p.Info == nil {
		return nil
	}

	var out []Assertion
	for _, file := range p.Syntax {
		for _, vs := range blankVarSpecs(file) {
			out = append(out, assertionsOf(p, vs)...)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := name(out[i].Interface), name(out[j].Interface); a != b {
			return a < b
		}
		return name(out[i].Concrete) < name(out[j].Concrete)
	})
	return out
}

// assertionsOf reads the pairings out of one `var _ I = ...` spec.
func assertionsOf(p *Package, vs *ast.ValueSpec) []Assertion {
	iface := namedOf(p.Info.TypeOf(vs.Type))
	if iface == nil {
		return nil
	}
	if _, ok := iface.Underlying().(*types.Interface); !ok {
		return nil
	}

	var out []Assertion
	for _, value := range vs.Values {
		concrete := namedOf(p.Info.TypeOf(value))
		if concrete == nil {
			continue
		}
		out = append(out, Assertion{Interface: iface, Concrete: concrete})
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

// namedOf unwraps a type to the named type behind it, looking through a
// pointer, since `var _ I = (*T)(nil)` is the conventional spelling.
func namedOf(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, _ := t.(*types.Named)
	return named
}

func allBlank(names []*ast.Ident) bool {
	for _, n := range names {
		if n.Name != "_" {
			return false
		}
	}
	return len(names) > 0
}

func name(n *types.Named) string {
	if n == nil {
		return ""
	}
	return n.Obj().Name()
}
