package directory

import (
	"sort"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// Layout modes for dir/domain-layout.
const (
	// ModeDomains requires every package to belong to a domain:
	// <root>/<domain>/<package>.
	ModeDomains = "domains"
	// ModePackages forbids the domain layer: <root>/<package>.
	ModePackages = "packages"
	// ModeAny accepts either shape and disables the rule for that root.
	ModeAny = "any"
)

// defaultRoots are the top-level directories permitted to contain Go packages.
var defaultRoots = []string{"pkg", "cmd", "internal"}

// rootOf returns the top-level segment of a root-relative path, or "" for the
// project root itself.
func rootOf(rel string) string {
	if rel == "." || rel == "" {
		return ""
	}
	root, _, _ := strings.Cut(rel, "/")
	return root
}

// depthUnder returns how many path segments a directory sits below a root.
// A direct child of the root is depth 1. It returns 0 when rel is the root
// itself, and -1 when rel is not under it at all.
func depthUnder(root, rel string) int {
	if rel == root {
		return 0
	}
	if !strings.HasPrefix(rel, root+"/") {
		return -1
	}
	return strings.Count(strings.TrimPrefix(rel, root+"/"), "/") + 1
}

// packagesUnder returns the packages beneath a root, sorted by directory.
func packagesUnder(p *project.Project, root string) []*project.Package {
	var out []*project.Package
	for _, pkg := range p.Packages {
		if depthUnder(root, pkg.Dir) > 0 {
			out = append(out, pkg)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out
}

// actsAsDomain reports whether a directory has packages beneath it, which is
// what distinguishes a domain from an ordinary package.
//
// The distinction decides which of two rules owns a depth-1 directory that
// holds Go files. With child packages it is a domain polluted with code, and
// dir/domain-has-no-go-files reports it; without them it is a package that
// never got a domain, and dir/domain-layout reports it. Without this split both
// rules fire on the same directory and the user sees one defect twice.
func actsAsDomain(p *project.Project, dir string) bool {
	for _, pkg := range p.Packages {
		if pkg.Dir != dir && strings.HasPrefix(pkg.Dir, dir+"/") {
			return true
		}
	}
	return false
}

// anchor returns the position a package-level finding should point at: the
// package clause of its first non-test file, falling back to the directory.
func anchor(c *rule.Context, pkg *project.Package) diag.Position {
	files := pkg.NonTestFiles()
	if len(files) == 0 {
		files = pkg.Files
	}
	if len(files) == 0 {
		return rule.DirPos(pkg.Dir)
	}
	return c.Pos(files[0].Syntax.Name)
}
