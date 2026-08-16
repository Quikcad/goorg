package typed

import (
	"go/ast"
	"go/types"
)

// InterfaceMethodOrder returns each interface's method names in the order they
// were written.
//
// It reads the syntax rather than the type, because types.Interface.Method
// returns methods sorted by identifier — the type checker has no notion of the
// order an interface was written in, and that order is exactly what this
// answers. Embedded interfaces contribute nothing: their methods arrive from
// elsewhere and impose no position here.
func InterfaceMethodOrder(p *Package) map[*types.Named][]string {
	return methodOrders(p, interfaceMethods)
}

// MethodOrder returns each named type's methods in the order they were
// declared, across every file of the package.
//
// Go requires a method to be declared in its type's package, so one package is
// the whole picture. Files are visited in sorted order, so a type whose methods
// are split across files — which org/type-cohesion forbids — still produces a
// stable answer rather than one that depends on the loader.
func MethodOrder(p *Package) map[*types.Named][]string {
	if p.Info == nil {
		return nil
	}

	out := map[*types.Named][]string{}
	for _, file := range p.sortedSyntax() {
		for _, node := range file.Decls {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			owner := namedOf(p.Info.TypeOf(fn.Recv.List[0].Type))
			if owner == nil {
				continue
			}
			out[owner] = append(out[owner], fn.Name.Name)
		}
	}
	return out
}

// methodOrders applies a per-type-spec extractor across a package's syntax.
func methodOrders(p *Package, extract func(*ast.TypeSpec) []string) map[*types.Named][]string {
	if p.Info == nil {
		return nil
	}

	out := map[*types.Named][]string{}
	for _, file := range p.sortedSyntax() {
		for _, spec := range typeSpecs(file) {
			named := namedOf(p.Info.TypeOf(spec.Name))
			if named == nil {
				continue
			}
			if names := extract(spec); len(names) > 0 {
				out[named] = names
			}
		}
	}
	return out
}

// interfaceMethods returns the explicitly declared method names of an
// interface, in source order.
func interfaceMethods(spec *ast.TypeSpec) []string {
	iface, ok := spec.Type.(*ast.InterfaceType)
	if !ok || iface.Methods == nil {
		return nil
	}
	var out []string
	for _, field := range iface.Methods.List {
		// An embedded interface or a type-union element has no name, and its
		// methods are not positioned by this declaration.
		for _, name := range field.Names {
			out = append(out, name.Name)
		}
	}
	return out
}

func typeSpecs(file *ast.File) []*ast.TypeSpec {
	var out []*ast.TypeSpec
	for _, node := range file.Decls {
		gen, ok := node.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				out = append(out, ts)
			}
		}
	}
	return out
}
