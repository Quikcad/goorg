package organization

import (
	"go/ast"
	"go/token"

	"github.com/Quikcad/goorg/pkg/source/project"
)

// defaultInstanceFunc is the mandated name of a singleton's accessor.
const defaultInstanceFunc = "instance"

// singleton describes a file that holds process-wide state.
//
// Recognising this shape is what resolves the one sanctioned conflict in the
// family: a singleton file requires the unexported accessor to appear *before*
// the exported ones, inverting org/private-functions-last. Because the shape is
// classified once here, that rule does not have to know singletons exist.
type singleton struct {
	// once is the sync.Once declaration guarding construction.
	once *ast.GenDecl
	// state are the package-level vars holding the instance.
	state []*ast.GenDecl
	// accessor is the unexported constructor, if one is declared.
	accessor *ast.FuncDecl
	// exported are the package-level functions that delegate to it.
	exported []*ast.FuncDecl
}

// detectSingleton classifies a file as a singleton file, or returns nil.
//
// The marker is a package-level sync.Once. That is deliberately narrow: a file
// with package state but no Once is not a singleton, it is a global, and
// org/globals-singleton-only reports it as one.
func detectSingleton(f *project.File, instanceName string) *singleton {
	if instanceName == "" {
		instanceName = defaultInstanceFunc
	}

	s := &singleton{}
	for _, node := range f.Syntax.Decls {
		switch d := node.(type) {
		case *ast.GenDecl:
			if d.Tok != token.VAR {
				continue
			}
			if declaresSyncOnce(d) {
				s.once = d
				continue
			}
			s.state = append(s.state, d)
		case *ast.FuncDecl:
			switch {
			case d.Recv != nil:
				continue
			case d.Name.Name == instanceName:
				s.accessor = d
			case d.Name.IsExported():
				s.exported = append(s.exported, d)
			}
		}
	}
	if s.once == nil {
		return nil
	}
	return s
}

// declaresSyncOnce reports whether a var declaration holds a sync.Once.
func declaresSyncOnce(d *ast.GenDecl) bool {
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if vs.Type != nil && isSelector(vs.Type, "sync", "Once") {
			return true
		}
		for _, v := range vs.Values {
			if isSelector(v, "sync", "Once") {
				return true
			}
		}
	}
	return false
}

// usesOnceDo reports whether a function body calls Do on something, which is
// the only sanctioned way to construct a singleton.
func usesOnceDo(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Do" {
			found = true
			return false
		}
		return true
	})
	return found
}

// assignsOutsideOnce reports the functions that assign to one of the named
// package-level variables anywhere other than inside a once-guarded closure.
func assignsOutsideOnce(f *project.File, names map[string]bool, accessor string) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, node := range f.Syntax.Decls {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// The accessor is where construction is supposed to happen.
		if fn.Recv == nil && fn.Name.Name == accessor {
			continue
		}
		if assignsAny(fn.Body, names) {
			out = append(out, fn)
		}
	}
	return out
}

func assignsAny(body *ast.BlockStmt, names map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && names[id.Name] {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// declaredNames returns every identifier a set of var declarations introduces.
func declaredNames(decls []*ast.GenDecl) map[string]bool {
	out := map[string]bool{}
	for _, d := range decls {
		for _, spec := range d.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				out[name.Name] = true
			}
		}
	}
	return out
}
