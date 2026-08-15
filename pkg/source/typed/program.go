// Package typed loads a Go project with full type information.
//
// This is the second of goorg's two loaders. pkg/source/project parses and
// nothing more, so it works on a tree that does not compile; this one runs the
// type checker, so it does not. Rules declare which they need through their
// Tier, and the CLI only reaches for this loader when a type-tier rule is
// actually enabled. See docs/decisions.md D1.
package typed

import (
	"fmt"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// loadMode is everything the type-tier rules need and nothing more. Each extra
// bit costs real time on a large module.
const loadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedSyntax |
	packages.NeedTypes |
	packages.NeedTypesInfo |
	packages.NeedDeps |
	packages.NeedImports

// Program is a type-checked view of a project.
type Program struct {
	// Root is the absolute path the packages were loaded from.
	Root string
	// Packages holds every package that type-checked, keyed by root-relative
	// directory.
	Packages map[string]*Package

	// external holds packages loaded on demand to resolve names the project
	// does not itself import.
	external map[string]*types.Package
}

// Load type-checks every package under root.
//
// It returns an error when the module cannot be loaded at all, and reports
// per-package failures through Package.Errors. The distinction matters: a
// module that will not load is a run goorg cannot make, whereas one broken
// package is a gap in coverage the caller should still hear about.
func Load(root string) (*Program, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}

	cfg := &packages.Config{Mode: loadMode, Dir: abs, Tests: false}
	loaded, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("no packages found under %s", root)
	}

	p := &Program{Root: abs, Packages: map[string]*Package{}, external: map[string]*types.Package{}}
	for _, pkg := range loaded {
		converted := newPackage(abs, pkg)
		if converted == nil {
			continue
		}
		p.Packages[converted.Dir] = converted
	}
	if len(p.Packages) == 0 {
		return nil, fmt.Errorf("no packages under %s could be type-checked", root)
	}
	return p, nil
}

// Broken returns the packages that did not type-check cleanly, sorted by
// directory. A type rule cannot be trusted on these, so the caller reports them
// as skipped coverage rather than passing silently.
func (p *Program) Broken() []*Package {
	var out []*Package
	for _, pkg := range p.Packages {
		if len(pkg.Errors) > 0 {
			out = append(out, pkg)
		}
	}
	sortPackages(out)
	return out
}

// Sound returns the packages that type-checked cleanly, sorted by directory.
func (p *Program) Sound() []*Package {
	var out []*Package
	for _, pkg := range p.Packages {
		if len(pkg.Errors) == 0 && pkg.Types != nil {
			out = append(out, pkg)
		}
	}
	sortPackages(out)
	return out
}

// Import loads packages by import path so their names can be resolved, even
// when the project under test never imports them.
//
// Without this, checking a type against fmt.Stringer would only work in modules
// that already import fmt — and a type that should be a Stringer and is not is
// exactly the case where nothing imports fmt yet. Paths that fail to load are
// skipped rather than reported: an unresolvable interface simply is not checked.
func (p *Program) Import(paths ...string) {
	var missing []string
	for _, path := range paths {
		if path == "" || p.external[path] != nil {
			continue
		}
		missing = append(missing, path)
	}
	if len(missing) == 0 {
		return
	}

	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: p.Root}
	loaded, err := packages.Load(cfg, missing...)
	if err != nil {
		return
	}
	for _, pkg := range loaded {
		if pkg.Types != nil {
			p.external[pkg.PkgPath] = pkg.Types
		}
	}
}

// Lookup resolves a qualified name such as "fmt.Stringer".
//
// It searches packages loaded through Import first, then the transitive imports
// of the project's own packages, then the universe scope for predeclared names
// such as error.
func (p *Program) Lookup(qualified string) types.Object {
	path, name, ok := strings.Cut(qualified, ".")
	if !ok {
		return types.Universe.Lookup(qualified)
	}
	if pkg := p.external[path]; pkg != nil {
		if obj := pkg.Scope().Lookup(name); obj != nil {
			return obj
		}
	}
	for _, pkg := range p.Packages {
		if pkg.Types == nil {
			continue
		}
		if obj := lookupIn(pkg.Types, path, name); obj != nil {
			return obj
		}
	}
	return nil
}

// lookupIn searches a package and its imports for path.name.
func lookupIn(pkg *types.Package, path, name string) types.Object {
	if pkg.Name() == path || pkg.Path() == path {
		if obj := pkg.Scope().Lookup(name); obj != nil {
			return obj
		}
	}
	for _, imported := range pkg.Imports() {
		if imported.Name() != path && imported.Path() != path {
			continue
		}
		if obj := imported.Scope().Lookup(name); obj != nil {
			return obj
		}
	}
	return nil
}

func sortPackages(pkgs []*Package) {
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Dir < pkgs[j].Dir })
}
