package typed

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

// Package is one type-checked package.
type Package struct {
	// Dir is the slash-separated directory relative to the project root.
	Dir string
	// Name is the package clause.
	Name string
	// Types is the type-checker's view, nil when checking failed.
	Types *types.Package
	// Info holds the identifier resolution the rules read.
	Info *types.Info
	// Fset positions every file.
	Fset *token.FileSet
	// Syntax holds the parsed files, aligned with Files.
	Syntax []*ast.File
	// Files holds root-relative paths, aligned with Syntax.
	Files []string
	// Errors are the type-checking failures, if any.
	Errors []string

	// root is the absolute project root, so FileOf can return a path that
	// matches the ones diagnostics carry.
	root string
}

// FileOf returns the root-relative path of the file containing a position.
//
// go/packages reports absolute paths; every diagnostic goorg emits is
// root-relative, so the two must be reconciled here rather than at each call
// site.
func (p *Package) FileOf(pos token.Pos) string {
	if !pos.IsValid() || p.Fset == nil {
		return ""
	}
	name := p.Fset.Position(pos).Filename
	if p.root == "" {
		return filepath.ToSlash(name)
	}
	rel, err := filepath.Rel(p.root, name)
	if err != nil {
		return filepath.ToSlash(name)
	}
	return filepath.ToSlash(rel)
}

// newPackage converts a go/packages result, or returns nil for one with no
// usable content.
func newPackage(root string, pkg *packages.Package) *Package {
	if pkg == nil || len(pkg.GoFiles) == 0 {
		return nil
	}

	out := &Package{
		root:   root,
		Name:   pkg.Name,
		Types:  pkg.Types,
		Info:   pkg.TypesInfo,
		Fset:   pkg.Fset,
		Syntax: pkg.Syntax,
	}
	for _, err := range pkg.Errors {
		out.Errors = append(out.Errors, err.Error())
	}
	for _, file := range pkg.GoFiles {
		rel, relErr := filepath.Rel(root, file)
		if relErr != nil {
			rel = file
		}
		out.Files = append(out.Files, filepath.ToSlash(rel))
	}
	if len(out.Files) > 0 {
		out.Dir = filepath.ToSlash(filepath.Dir(out.Files[0]))
	}
	return out
}
