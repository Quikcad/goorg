package organization

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

// globalFileScopedSettings is the configurable surface of the rule.
type globalFileScopedSettings struct {
	// Exempt names the global kinds that are meant to be referenced
	// package-wide, reusing org/globals-singleton-only's classifiers.
	Exempt []string `yaml:"exempt"`
	// IgnoreTests skips _test.go files, which legitimately reach into a
	// package's internals.
	IgnoreTests bool `yaml:"ignore_tests"`
}

var globalFileScoped = &rule.Rule{
	ID:       "org/global-file-scoped",
	Category: rule.Organization,
	Tier:     rule.Types,
	Summary:  "a package-level variable is referenced only in the file that declares it",
	Default:  diag.Error,
	Doc: `A package-level variable may be referenced only within the file that
declares it.

	registry.go   var registry *Registry     declared here
	registry.go   func instance() ...        OK — same file
	lookup.go     registry.entries[name]     violation — reaches past the accessor

Go has no file-level visibility. This rule creates it by convention: internal/
restricts a package's reach, unexported restricts a package member's reach, and
this restricts a variable's reach to a single file.

Rationale: the accessor is the point. A singleton's instance function exists to
guarantee that initialisation happened exactly once before anyone reads the
value, and every reference that bypasses it and touches the variable directly is
a guarantee lost. Those references are also invisible — nothing in the
declaration says who reads it, so the set of writers can only be found by
searching the whole package. Confining a variable to one file makes its complete
usage auditable by reading that file, which is the same property internal/ gives
a package.

To fix: move the reference behind an accessor in the declaring file, or move the
declaration to the file that uses it.

The exemptions match org/globals-singleton-only exactly, and for the same
reason: this rule exists to force *mutable* state behind an accessor. A sentinel
error, an interface assertion, a compiled pattern, an embedded filesystem and a
definition table are none of them mutable, and all of them are meant to be read
from anywhere.

This rule is type-tier. A syntactic version would count a local variable that
shadows the global's name as a reference, and here a false positive accuses
correct code of a race.

Configure in .goorg.yaml:

	settings:
	  org/global-file-scoped:
	    exempt: [sentinel-errors, interface-assertions, compiled-patterns, lookup-tables]
	    ignore_tests: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := globalFileScopedSettings{
			// The same exemptions org/globals-singleton-only grants, and for
			// the same reason: this rule exists to force mutable state behind
			// an accessor, and none of these are mutable state. A definition
			// table or a compiled pattern is written once and read everywhere,
			// which is what it is for.
			Exempt: []string{
				exemptSentinelErrors, exemptInterfaceAsserts,
				exemptCompiledPatterns, exemptLookupTables, exemptEmbeddedFilesystem,
			},
			IgnoreTests: true,
		}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkGlobalScope(pkg, &s)...)
		}
		diag.Sort(out)
		return out
	},
}

// checkGlobalScope reports package-level variables read from another file.
func checkGlobalScope(pkg *typed.Package, s *globalFileScopedSettings) []diag.Diagnostic {
	globals := packageVars(pkg, s)
	if len(globals) == 0 {
		return nil
	}

	// strays[obj] is the set of files that reference obj from elsewhere.
	strays := map[types.Object]map[string]bool{}
	for ident, obj := range pkg.Info.Uses {
		home, tracked := globals[obj]
		if !tracked {
			continue
		}
		where := pkg.FileOf(ident.Pos())
		if where == home || where == "" {
			continue
		}
		if strays[obj] == nil {
			strays[obj] = map[string]bool{}
		}
		strays[obj][where] = true
	}

	var out []diag.Diagnostic
	for obj, files := range strays {
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		out = append(out, diag.Diagnostic{
			Position: diag.Position{
				Path: pkg.FileOf(obj.Pos()),
				Line: pkg.Fset.Position(obj.Pos()).Line,
				Col:  pkg.Fset.Position(obj.Pos()).Column,
			},
			Message: fmt.Sprintf("package variable %s is referenced from %s",
				obj.Name(), joinFiles(names)),
			Help: "expose it through an accessor in this file, or move the declaration",
		})
	}
	return out
}

// packageVars returns the package-level variables the rule tracks, mapped to
// the file that declares each one.
func packageVars(pkg *typed.Package, s *globalFileScopedSettings) map[types.Object]string {
	exempt := map[string]bool{}
	for _, kind := range s.Exempt {
		exempt[kind] = true
	}

	out := map[types.Object]string{}
	for i, file := range pkg.Syntax {
		if s.IgnoreTests && i < len(pkg.Files) && isTestFile(pkg.Files[i]) {
			continue
		}
		for _, node := range file.Decls {
			gen, ok := node.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if kind := classifyGlobal(gen, vs); kind != "" && exempt[kind] {
					continue
				}
				for _, name := range vs.Names {
					obj := pkg.Info.Defs[name]
					if obj == nil || name.Name == "_" {
						continue
					}
					out[obj] = pkg.FileOf(name.Pos())
				}
			}
		}
	}
	return out
}

func isTestFile(path string) bool {
	return len(path) > 8 && path[len(path)-8:] == "_test.go"
}

func joinFiles(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return fmt.Sprintf("%s and %d more", names[0], len(names)-1)
}
