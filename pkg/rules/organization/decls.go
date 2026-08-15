package organization

import (
	"go/ast"
	"go/token"

	"github.com/Quikcad/goorg/pkg/source/project"
)

// section is a position in the canonical file order. The zero value is the
// first section, so declarations sort naturally by section value.
type section int

const (
	// sectionEnums holds const blocks and the named types they belong to.
	sectionEnums section = iota
	// sectionVars holds package-level variables.
	sectionVars
	// sectionInterfaces holds interface declarations.
	sectionInterfaces
	// sectionTypes holds structs and other named types, each followed by its
	// factory and its methods.
	sectionTypes
	// sectionFunctions holds everything else.
	sectionFunctions
)

// sectionNames are the labels used in configuration and in findings.
var sectionNames = map[section]string{
	sectionEnums:      "enums",
	sectionVars:       "vars",
	sectionInterfaces: "interfaces",
	sectionTypes:      "structs",
	sectionFunctions:  "functions",
}

func (s section) String() string {
	if name, ok := sectionNames[s]; ok {
		return name
	}
	return "unknown"
}

// varSection places a package-level var.
//
// Normally that is the vars section, but a var whose initializer references a
// type declared later in the same file cannot be introduced before it. The
// canonical order exists so a file reads top to bottom without meeting an
// undefined name, and hoisting such a var above its type would defeat exactly
// that. So the var sinks to the section of the latest local type it depends on.
func varSection(d *ast.GenDecl, sections map[string]section) section {
	sect := sectionVars
	ast.Inspect(d, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if ref, declared := sections[id.Name]; declared && ref > sect {
			sect = ref
		}
		return true
	})
	return sect
}

// decl is one top-level declaration, classified.
type decl struct {
	node ast.Decl
	sect section
	// owner is the type a method or factory belongs to, or "" for anything
	// else. It is what org/member-order uses to check that a type's
	// declarations stay contiguous.
	owner string
	// name is the declared identifier, for messages.
	name string
	// exported reports whether name is exported. Meaningless for var and const
	// blocks, which may declare several names at once.
	exported bool
	// isFunc distinguishes functions and methods from everything else.
	isFunc bool
	// isMethod reports whether the declaration has a receiver.
	isMethod bool
}

func classifyGen(d *ast.GenDecl, enums map[string]bool, sections map[string]section) decl {
	switch d.Tok {
	case token.CONST:
		return decl{node: d, sect: sectionEnums, name: firstName(d)}
	case token.VAR:
		return decl{node: d, sect: varSection(d, sections), name: firstName(d)}
	case token.TYPE:
		return classifyType(d, enums)
	default:
		// import declarations and anything else the parser produces
		return decl{node: d, sect: sectionEnums, name: firstName(d)}
	}
}

func classifyType(d *ast.GenDecl, enums map[string]bool) decl {
	spec, ok := firstTypeSpec(d)
	if !ok {
		return decl{node: d, sect: sectionTypes, name: firstName(d)}
	}
	name := spec.Name.Name
	out := decl{node: d, name: name, exported: spec.Name.IsExported(), owner: name}
	switch {
	case isInterface(spec):
		out.sect = sectionInterfaces
	case enums[name]:
		// An enum's type declaration belongs with the constants that give it
		// meaning, not with the structs.
		out.sect = sectionEnums
	default:
		out.sect = sectionTypes
	}
	return out
}

func classifyFunc(d *ast.FuncDecl, local map[string]bool) decl {
	out := decl{
		node:     d,
		name:     d.Name.Name,
		exported: d.Name.IsExported(),
		isFunc:   true,
	}
	if d.Recv != nil {
		out.sect = sectionTypes
		out.isMethod = true
		out.owner = receiverTypeName(d)
		return out
	}
	// A factory sits with the type it constructs rather than among the free
	// functions, which is what keeps a type and its constructor adjacent.
	if owner := factoryFor(d, local); owner != "" {
		out.sect = sectionTypes
		out.owner = owner
		return out
	}
	out.sect = sectionFunctions
	return out
}

// classify assigns every top-level declaration in a file to a section.
func classify(f *project.File) []decl {
	enums := enumTypes(f.Syntax)
	local := localTypes(f.Syntax)
	sections := localTypeSections(f.Syntax, enums)

	var out []decl
	for _, node := range f.Syntax.Decls {
		switch d := node.(type) {
		case *ast.GenDecl:
			out = append(out, classifyGen(d, enums, sections))
		case *ast.FuncDecl:
			out = append(out, classifyFunc(d, local))
		}
	}
	return out
}

// localTypeSections maps each type declared in a file to its section.
func localTypeSections(f *ast.File, enums map[string]bool) map[string]section {
	out := map[string]section{}
	for _, node := range f.Decls {
		d, ok := node.(*ast.GenDecl)
		if !ok || d.Tok != token.TYPE {
			continue
		}
		for _, spec := range d.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			switch {
			case isInterface(ts):
				out[ts.Name.Name] = sectionInterfaces
			case enums[ts.Name.Name]:
				out[ts.Name.Name] = sectionEnums
			default:
				out[ts.Name.Name] = sectionTypes
			}
		}
	}
	return out
}

// enumTypes returns the named types that have a const block declaring values of
// that type in the same file.
func enumTypes(f *ast.File) map[string]bool {
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

// localTypes returns the type names declared in a file.
func localTypes(f *ast.File) map[string]bool {
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

// factoryFor returns the local type a function constructs, or "".
//
// A factory is a function whose first non-error result is a type declared in
// the same file, which is the same classifier pat/factory-naming uses.
func factoryFor(d *ast.FuncDecl, local map[string]bool) string {
	if d.Type.Results == nil {
		return ""
	}
	for _, field := range d.Type.Results.List {
		name := baseTypeName(field.Type)
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

// baseTypeName unwraps a type expression down to its identifier, looking
// through pointers and generic instantiation.
func baseTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return baseTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return baseTypeName(t.X)
	case *ast.IndexListExpr:
		return baseTypeName(t.X)
	default:
		return ""
	}
}

// receiverTypeName returns the type a method is declared on.
func receiverTypeName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) != 1 {
		return ""
	}
	return baseTypeName(d.Recv.List[0].Type)
}

func isInterface(spec *ast.TypeSpec) bool {
	_, ok := spec.Type.(*ast.InterfaceType)
	return ok
}

func firstTypeSpec(d *ast.GenDecl) (*ast.TypeSpec, bool) {
	if len(d.Specs) != 1 {
		return nil, false
	}
	ts, ok := d.Specs[0].(*ast.TypeSpec)
	return ts, ok
}

func firstName(d *ast.GenDecl) string {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			if len(s.Names) > 0 {
				return s.Names[0].Name
			}
		case *ast.TypeSpec:
			return s.Name.Name
		}
	}
	return ""
}

// isSelector reports whether an expression is pkg.Name.
func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg
}
