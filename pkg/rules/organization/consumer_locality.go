package organization

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/decl"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

// consumerLocalitySettings is the configurable surface of the rule.
type consumerLocalitySettings struct {
	CheckFunctions bool `yaml:"check_functions"`
	CheckEnums     bool `yaml:"check_enums"`
	// MaxTargetDeclarations bounds how full the destination file may be before
	// the rule stays quiet. See the note on the what-if problem below.
	MaxTargetDeclarations int  `yaml:"max_target_declarations"`
	IgnoreTests           bool `yaml:"ignore_tests"`
	// SkipHelperFiles leaves files that export nothing alone.
	SkipHelperFiles bool `yaml:"skip_helper_files"`
}

var consumerLocality = &rule.Rule{
	ID:       "org/consumer-locality",
	Category: rule.Organization,
	Tier:     rule.Types,
	Summary:  "a declaration used from only one other file belongs in that file",
	Default:  diag.Warning,
	Doc: `A declaration whose only consumers within the package live in one other
file belongs in that file.

	lexer.go    calls validate() three times     the only consumer
	parser.go   func validate(...) error         declared here — violation

Rationale: code should live next to the code that needs it. A helper in a
distant file is a helper nobody knows exists, so the next person who needs it
writes a second one — and now there are two subtly different implementations,
which is how a package accumulates three functions that all normalise a name.
Locality also makes deletion safe: beside its only caller, a function's
redundancy is obvious when the caller goes; three files away it survives forever
because nobody can prove it is unused.

To fix: move the declaration into the file that uses it.

A file that exports nothing is left alone entirely: it is a helper file by
construction, and grouping implementation by topic is what it is for.

Only unexported declarations are considered. An exported one's real consumers
live in other packages, which a package-scoped analysis cannot see, so a single
internal use would be a meaningless signal — and acting on it would scatter a
package's own API across files to sit beside one incidental caller.

Deliberately silent when there is no single right answer: no consumers at all
(that is dead code, a different rule), consumers in two or more files (it is
genuinely shared), or consumers only outside the package. Methods are never
relocated — org/type-cohesion puts them with their type.

This ships as the narrow form. The specification allows a declaration to move
unless the move would break another rule, which needs the engine to evaluate a
tree that does not exist yet. Until that exists, the rule stays quiet when the
destination file already holds max_target_declarations or more top-level
declarations, which approximates "has room" without this rule reading another
rule's configuration. The approximation is conservative: it misses real
findings rather than proposing a move that creates a new violation.

Configure in .goorg.yaml:

	settings:
	  org/consumer-locality:
	    check_functions: true
	    check_enums: true
	    max_target_declarations: 12`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := consumerLocalitySettings{
			CheckFunctions:        true,
			CheckEnums:            true,
			MaxTargetDeclarations: 12,
			IgnoreTests:           true,
			SkipHelperFiles:       true,
		}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkLocality(pkg, &s)...)
		}
		diag.Sort(out)
		return out
	},
}

// checkLocality reports declarations whose sole consuming file is another one.
func checkLocality(pkg *typed.Package, s *consumerLocalitySettings) []diag.Diagnostic {
	homes := relocatable(pkg, s)
	if len(homes) == 0 {
		return nil
	}
	sizes := declarationCounts(pkg)

	// consumers[obj] is the set of files referencing obj.
	consumers := map[types.Object]map[string]bool{}
	for ident, obj := range pkg.Info.Uses {
		if _, tracked := homes[obj]; !tracked {
			continue
		}
		where := pkg.FileOf(ident.Pos())
		if where == "" || (s.IgnoreTests && isTestFile(where)) {
			continue
		}
		if consumers[obj] == nil {
			consumers[obj] = map[string]bool{}
		}
		consumers[obj][where] = true
	}

	var out []diag.Diagnostic
	for obj, files := range consumers {
		// Exactly one consuming file, and it is not where the declaration
		// lives. Anything else has no single right answer.
		if len(files) != 1 {
			continue
		}
		target := onlyKey(files)
		home := homes[obj]
		if target == home {
			continue
		}
		if sizes[target] >= s.MaxTargetDeclarations {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: diag.Position{
				Path: home,
				Line: pkg.Fset.Position(obj.Pos()).Line,
				Col:  pkg.Fset.Position(obj.Pos()).Column,
			},
			Message: fmt.Sprintf("%s is used only from %s", obj.Name(), target),
			Help:    fmt.Sprintf("move it into %s, beside its only consumer", target),
		})
	}
	return out
}

// relocatable returns the declarations the rule may propose moving, mapped to
// the file that declares each.
func relocatable(pkg *typed.Package, s *consumerLocalitySettings) map[types.Object]string {
	out := map[types.Object]string{}
	for i, file := range pkg.Syntax {
		if s.IgnoreTests && i < len(pkg.Files) && isTestFile(pkg.Files[i]) {
			continue
		}
		// A file that exports nothing is a helper file by construction, and
		// grouping implementation by topic is what such a file is for. This is
		// the same judgement org/max-private-functions makes with
		// when_file_has_exports.
		if s.SkipHelperFiles && !exportsAnything(file) {
			continue
		}
		locals := decl.LocalTypes(file)
		for _, node := range file.Decls {
			switch d := node.(type) {
			case *ast.FuncDecl:
				// A method's home is its type, never its caller.
				if !s.CheckFunctions || d.Recv != nil || d.Name.Name == "init" {
					continue
				}
				// So is a factory's: org/type-cohesion outranks this rule, and
				// proposing that newPackage leave package.go would create the
				// violation it forbids.
				if decl.FactoryFor(d, locals) != "" {
					continue
				}
				// An exported declaration's real consumers are in other
				// packages, which this package's type information cannot see.
				// One internal use therefore says nothing about where it
				// belongs.
				if d.Name.IsExported() {
					continue
				}
				if obj := pkg.Info.Defs[d.Name]; obj != nil {
					out[obj] = pkg.FileOf(d.Name.Pos())
				}
			case *ast.GenDecl:
				if !s.CheckEnums || d.Tok.String() != "const" {
					continue
				}
				collectConstNames(pkg, d, out)
			}
		}
	}
	return out
}

func collectConstNames(pkg *typed.Package, d *ast.GenDecl, out map[types.Object]string) {
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range vs.Names {
			if name.Name == "_" || name.IsExported() {
				continue
			}
			if obj := pkg.Info.Defs[name]; obj != nil {
				out[obj] = pkg.FileOf(name.Pos())
			}
		}
	}
}

// declarationCounts returns how many top-level declarations each file holds,
// which is the proxy for whether it has room to take another.
func declarationCounts(pkg *typed.Package) map[string]int {
	out := map[string]int{}
	for i, file := range pkg.Syntax {
		if i >= len(pkg.Files) {
			continue
		}
		out[pkg.Files[i]] = len(file.Decls)
	}
	return out
}

func onlyKey(set map[string]bool) string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// exportsAnything reports whether a file declares anything exported.
func exportsAnything(file *ast.File) bool {
	for _, node := range file.Decls {
		switch d := node.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.IsExported() {
				return true
			}
		case *ast.GenDecl:
			if genDeclExports(d) {
				return true
			}
		}
	}
	return false
}

func genDeclExports(d *ast.GenDecl) bool {
	for _, spec := range d.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			if sp.Name.IsExported() {
				return true
			}
		case *ast.ValueSpec:
			for _, name := range sp.Names {
				if name.IsExported() {
					return true
				}
			}
		}
	}
	return false
}
