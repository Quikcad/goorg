package logic

import (
	"fmt"
	"go/ast"
	"sort"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/decl"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// maxObjectMembersSettings is the configurable surface of the rule.
type maxObjectMembersSettings struct {
	Fields  int `yaml:"fields"`
	Methods int `yaml:"methods"`
	// Combined caps fields and methods against one budget. 0 disables it.
	Combined int `yaml:"combined"`
	// CountEmbedded counts an embedded type as one member rather than
	// expanding its own member set.
	CountEmbedded bool `yaml:"count_embedded"`
}

// typeSize is the member tally for one named type across a package.
type typeSize struct {
	name    string
	pos     diag.Position
	fields  int
	methods int
}

var maxObjectMembers = &rule.Rule{
	ID:       "logic/max-object-members",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a type declares at most a configured number of members",
	Default:  diag.Error,
	Doc: `A type may declare at most N members. A member is a struct field or a
method with that type as receiver, counted across the whole package.

	type Server struct {          19 fields — this is three types wearing one name
		addr, cert, key                   string
		readTimeout, writeTimeout         time.Duration
		maxConns, maxIdle, maxHeaderBytes int
		...
	}

	type Server struct {          the seams were already there
		listen ListenConfig
		limits Limits
		obs    Observability
	}

Rationale: a type's member count is the most reliable proxy for how many
responsibilities it has. Fields are the state something owns, and when a type
owns nineteen pieces of state no single method touches most of them — the type
is really several types that happen to share a struct literal. The count also
bounds what a reader must hold in their head to reason about any one method:
every field is potentially mutated by every method, so the invariant surface
grows with the product of the two.

To fix: split the type along the grouping its fields already suggest. The split
must not become a subpackage — dir/max-package-depth forbids that — so the new
type is a sibling in the same package.

Methods are counted here rather than against the file, which is what lets
org/type-cohesion require a type's methods to share one file without fighting
org/max-functions-per-file.

Configure in .goorg.yaml:

	settings:
	  logic/max-object-members:
	    fields: 12
	    methods: 15
	    count_embedded: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := maxObjectMembersSettings{Fields: 12, Methods: 15, CountEmbedded: true}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Project.Packages {
			out = append(out, checkPackageTypes(c, pkg, &s)...)
		}
		return out
	},
}

// checkPackageTypes tallies every type in a package and reports the oversized.
func checkPackageTypes(c *rule.Context, pkg *project.Package, s *maxObjectMembersSettings) []diag.Diagnostic {
	sizes := map[string]*typeSize{}
	size := func(name string) *typeSize {
		t, ok := sizes[name]
		if !ok {
			t = &typeSize{name: name}
			sizes[name] = t
		}
		return t
	}

	for _, f := range pkg.Files {
		if f.IsTest {
			continue
		}
		for _, node := range f.Syntax.Decls {
			switch d := node.(type) {
			case *ast.GenDecl:
				collectFields(c, d, size, s.CountEmbedded)
			case *ast.FuncDecl:
				if name := decl.ReceiverTypeName(d); name != "" {
					size(name).methods++
				}
			}
		}
	}

	names := make([]string, 0, len(sizes))
	for name := range sizes {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []diag.Diagnostic
	for _, name := range names {
		t := sizes[name]
		if t.pos.Path == "" {
			// A method on a type declared in another file of a package goorg
			// did not load; there is nothing to point at.
			continue
		}
		switch {
		case s.Fields > 0 && t.fields > s.Fields:
			out = append(out, diag.Diagnostic{
				Position: t.pos,
				Message: fmt.Sprintf("type %s has %s, over the limit of %d",
					name, plural(t.fields, "field"), s.Fields),
				Help: "split the type along the grouping its fields already suggest",
			})
		case s.Methods > 0 && t.methods > s.Methods:
			out = append(out, diag.Diagnostic{
				Position: t.pos,
				Message: fmt.Sprintf("type %s has %s, over the limit of %d",
					name, plural(t.methods, "method"), s.Methods),
				Help: "split the responsibilities into separate types",
			})
		case s.Combined > 0 && t.fields+t.methods > s.Combined:
			out = append(out, diag.Diagnostic{
				Position: t.pos,
				Message: fmt.Sprintf("type %s has %d members, over the combined limit of %d",
					name, t.fields+t.methods, s.Combined),
				Help: "split the responsibilities into separate types",
			})
		}
	}
	return out
}

// declaredFields counts a struct's fields, expanding grouped names. An
// embedded type counts as one member rather than as its expansion, which keeps
// the count about what this type declares.
func declaredFields(st *ast.StructType, countEmbedded bool) int {
	count := 0
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			if countEmbedded {
				count++
			}
			continue
		}
		count += len(field.Names)
	}
	return count
}

// collectFields records the declared field count of every struct in a type
// declaration.
func collectFields(c *rule.Context, d *ast.GenDecl, size func(string) *typeSize, countEmbedded bool) {
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		t := size(ts.Name.Name)
		t.pos = c.Pos(ts.Name)
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			continue
		}
		t.fields += declaredFields(st, countEmbedded)
	}
}
