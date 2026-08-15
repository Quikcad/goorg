package organization

import (
	"go/ast"
	"go/token"

	"github.com/Quikcad/goorg/pkg/source/decl"
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

// member is one top-level declaration, classified.
type member struct {
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

func classifyGen(d *ast.GenDecl, enums map[string]bool, sections map[string]section) member {
	switch d.Tok {
	case token.CONST:
		return member{node: d, sect: sectionEnums, name: firstName(d)}
	case token.VAR:
		return member{node: d, sect: varSection(d, sections), name: firstName(d)}
	case token.TYPE:
		return classifyType(d, enums)
	default:
		// import declarations and anything else the parser produces
		return member{node: d, sect: sectionEnums, name: firstName(d)}
	}
}

func classifyType(d *ast.GenDecl, enums map[string]bool) member {
	spec, ok := firstTypeSpec(d)
	if !ok {
		return member{node: d, sect: sectionTypes, name: firstName(d)}
	}
	name := spec.Name.Name
	out := member{node: d, name: name, exported: spec.Name.IsExported(), owner: name}
	switch {
	case decl.IsInterface(spec):
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

func classifyFunc(d *ast.FuncDecl, local map[string]bool) member {
	out := member{
		node:     d,
		name:     d.Name.Name,
		exported: d.Name.IsExported(),
		isFunc:   true,
	}
	if d.Recv != nil {
		out.sect = sectionTypes
		out.isMethod = true
		out.owner = decl.ReceiverTypeName(d)
		return out
	}
	// A factory sits with the type it constructs rather than among the free
	// functions, which is what keeps a type and its constructor adjacent.
	if owner := decl.FactoryFor(d, local); owner != "" {
		out.sect = sectionTypes
		out.owner = owner
		return out
	}
	out.sect = sectionFunctions
	return out
}

// classify assigns every top-level declaration in a file to a section.
func classify(f *project.File) []member {
	enums := decl.EnumTypes(f.Syntax)
	local := decl.LocalTypes(f.Syntax)
	sections := localTypeSections(f.Syntax, enums)

	var out []member
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
			case decl.IsInterface(ts):
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
