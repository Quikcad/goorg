package decl

import "go/ast"

// BaseTypeName unwraps a type expression down to its identifier, looking
// through pointers and generic instantiation. It returns "" for a type that
// has no single name, such as a slice or a map.
func BaseTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return BaseTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return BaseTypeName(t.X)
	case *ast.IndexListExpr:
		return BaseTypeName(t.X)
	default:
		return ""
	}
}

// ReceiverTypeName returns the type a method is declared on, or "".
func ReceiverTypeName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) != 1 {
		return ""
	}
	return BaseTypeName(d.Recv.List[0].Type)
}

// FactoryFor returns the local type a function constructs, or "".
//
// A factory is a function whose first non-error result is a type declared in
// the same file. org/member-order uses this to keep a factory beside its type,
// and pat/factory-naming uses it to decide which prefix the function needs.
func FactoryFor(d *ast.FuncDecl, local map[string]bool) string {
	if d.Recv != nil || d.Type.Results == nil {
		return ""
	}
	for _, field := range d.Type.Results.List {
		name := BaseTypeName(field.Type)
		if name == "" || name == "error" {
			continue
		}
		if local[name] {
			return name
		}
		return ""
	}
	return ""
}

// IsSelector reports whether an expression is exactly pkg.Name.
func IsSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg
}
