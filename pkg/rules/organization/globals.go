package organization

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/decl"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// globalKind names a category of package-level variable that the globals rules
// treat as something other than mutable state.
type globalKind string

const (
	exemptSentinelErrors     globalKind = "sentinel-errors"
	exemptInterfaceAsserts   globalKind = "interface-assertions"
	exemptCompiledPatterns   globalKind = "compiled-patterns"
	exemptLookupTables       globalKind = "lookup-tables"
	exemptEmbeddedFilesystem globalKind = "embedded-filesystems"
)

// compileOnce are constructors whose whole purpose is to do expensive work once
// at initialisation. Go offers no other place to put the result.
var compileOnce = map[string][]string{
	"regexp":   {"MustCompile", "MustCompilePOSIX"},
	"strings":  {"NewReplacer"},
	"template": {"Must"},
}

// globalsSettings is the configurable surface of org/globals-singleton-only.
type globalsSettings struct {
	// Allow names the exempt forms. The list is the rule: Go offers no
	// immutable composite constant, so several of these have no alternative
	// spelling and a rule without them would fire on unavoidable code.
	Allow []string `yaml:"allow"`
	// RequireUnexportedTables keeps exempt tables out of the public API, where
	// another package could mutate them.
	RequireUnexportedTables bool `yaml:"require_unexported_tables"`
}

func (s *globalsSettings) allows(kind globalKind) bool {
	for _, a := range s.Allow {
		if globalKind(strings.TrimSpace(a)) == kind {
			return true
		}
	}
	return false
}

var globalsSingletonOnly = &rule.Rule{
	ID:       "org/globals-singleton-only",
	Category: rule.Organization,
	Tier:     rule.Syntax,
	Summary:  "package-level variables are permitted only as singleton state",
	Default:  diag.Error,
	Doc: `A package-level var is permitted only as the backing state of a
singleton. Everything else that wants to be a global is a constant, a field on
a type, or a parameter.

Rationale: a package-level variable is state with no owner and no lifetime.
Anything in the package can read it, anything can write it, nothing coordinates
the two, and tests that touch it cannot run in parallel or in isolation. The
cost does not show up until the package has grown enough that no single person
knows every writer. The singleton exception exists because process-wide state is
occasionally real — a connection pool, a metrics registry, a cache. Confining
globals to that one pattern means every global has an accessor, a sync.Once and
a known initialisation point, instead of being a variable someone assigned to
in an init function.

To fix: make it a constant if it never changes, a struct field if it belongs to
something, or a proper singleton if it is genuinely process-wide.

The exemption list is the whole design, not a convenience. Go has no immutable
composite constant, so a lookup table, a compiled regexp and a sentinel error
can only be package-level vars, and //go:embed cannot target anything else. A
rule without these exemptions fires on idiomatic, correct, unavoidable code and
is switched off within a day.

Exempt by default:

	sentinel-errors        var ErrFoo = errors.New(...)
	interface-assertions   var _ Iface = (*T)(nil)
	compiled-patterns      var re = regexp.MustCompile(...)
	lookup-tables          var valid = map[Status]bool{...}
	                       var rule = &Rule{...}          — a definition table
	embedded-filesystems   //go:embed assets
	                       var assets embed.FS

Configure in .goorg.yaml:

	settings:
	  org/globals-singleton-only:
	    allow: [sentinel-errors, interface-assertions, compiled-patterns, lookup-tables]
	    require_unexported_tables: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := globalsSettings{
			Allow: []string{
				string(exemptSentinelErrors), string(exemptInterfaceAsserts),
				string(exemptCompiledPatterns), string(exemptLookupTables),
				string(exemptEmbeddedFilesystem),
			},
			RequireUnexportedTables: true,
		}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range c.Project.Files() {
			if f.IsTest {
				continue
			}
			// A singleton's own state is the sanctioned case.
			if detectSingleton(f, defaultInstanceFunc) != nil {
				continue
			}
			out = append(out, checkGlobals(c, f, &s)...)
		}
		return out
	},
}

// classifyGlobal names the exemption a declaration qualifies for, or "".
func classifyGlobal(gen *ast.GenDecl, vs *ast.ValueSpec) globalKind {
	if allBlank(vs.Names) {
		return exemptInterfaceAsserts
	}
	if hasEmbedDirective(gen) || (vs.Type != nil && decl.IsSelector(vs.Type, "embed", "FS")) {
		return exemptEmbeddedFilesystem
	}

	if vs.Type != nil {
		if id, ok := vs.Type.(*ast.Ident); ok && id.Name == "error" {
			return exemptSentinelErrors
		}
		if isCompositeType(vs.Type) {
			return exemptLookupTables
		}
	}
	for _, v := range vs.Values {
		switch {
		case isCall(v, "errors", "New"), isCall(v, "fmt", "Errorf"):
			return exemptSentinelErrors
		case isCompileOnce(v):
			return exemptCompiledPatterns
		case isMustCall(v):
			return exemptCompiledPatterns
		}
		if isCompositeLit(v) {
			return exemptLookupTables
		}
	}
	return ""
}

// checkGlobalSpec judges one package-level variable declaration.
func checkGlobalSpec(c *rule.Context, gen *ast.GenDecl, vs *ast.ValueSpec, s *globalsSettings) *diag.Diagnostic {
	kind := classifyGlobal(gen, vs)
	if kind == "" || !s.allows(kind) {
		return &diag.Diagnostic{
			Position: c.Pos(vs),
			Message:  fmt.Sprintf("package-level variable %s is not singleton state", vs.Names[0].Name),
			Help:     "make it a constant, a struct field, or a singleton guarded by sync.Once",
		}
	}
	// An exempt table is still API if it is exported, and any importer can
	// reassign an element of it.
	if kind != exemptLookupTables || !s.RequireUnexportedTables || !anyExported(vs) {
		return nil
	}

	return &diag.Diagnostic{
		Position: c.Pos(vs),
		Message:  fmt.Sprintf("exported lookup table %s can be mutated by any importer", vs.Names[0].Name),
		Help:     "make it unexported and expose a lookup function",
	}
}

// checkGlobals reports package-level vars that no exemption covers.
func checkGlobals(c *rule.Context, f *project.File, s *globalsSettings) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, node := range f.Syntax.Decls {
		gen, ok := node.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if d := checkGlobalSpec(c, gen, vs, s); d != nil {
				out = append(out, *d)
			}
		}
	}
	return out
}

// isCompositeLit recognises a composite literal, including the addressed form.
//
// &T{...} parses as a UnaryExpr wrapping the literal, so matching only
// *ast.CompositeLit misses every package-level definition table written as a
// pointer — which is most of them.
func isCompositeLit(expr ast.Expr) bool {
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		expr = unary.X
	}
	_, ok := expr.(*ast.CompositeLit)
	return ok
}

func isCompositeType(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.MapType, *ast.ArrayType:
		return true
	default:
		return false
	}
}

// isCompileOnce reports whether an expression is such a constructor, or a
// method called directly on one — strings.NewReplacer(...).Replace is a single
// compile-once value however it is spelled.
func isCompileOnce(expr ast.Expr) bool {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return isCompileOnce(sel.X)
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	for _, name := range compileOnce[pkg.Name] {
		if sel.Sel.Name == name {
			return true
		}
	}
	return false
}

// isMustCall recognises the Must* convention, which by construction runs once
// at initialisation and panics rather than returning an error.
func isMustCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return strings.HasPrefix(fn.Sel.Name, "Must")
	case *ast.Ident:
		return strings.HasPrefix(fn.Name, "Must")
	default:
		return false
	}
}

func isCall(expr ast.Expr, pkg, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	return decl.IsSelector(call.Fun, pkg, name)
}

func hasEmbedDirective(gen *ast.GenDecl) bool {
	if gen.Doc == nil {
		return false
	}
	for _, comment := range gen.Doc.List {
		if strings.HasPrefix(comment.Text, "//go:embed") {
			return true
		}
	}
	return false
}

func allBlank(names []*ast.Ident) bool {
	for _, n := range names {
		if n.Name != "_" {
			return false
		}
	}
	return len(names) > 0
}

func anyExported(vs *ast.ValueSpec) bool {
	for _, n := range vs.Names {
		if n.IsExported() {
			return true
		}
	}
	return false
}
