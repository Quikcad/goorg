package organization

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// typeCohesionSettings is the configurable surface of org/type-cohesion.
type typeCohesionSettings struct {
	// IncludeFactories requires a type's factory to sit with it too.
	IncludeFactories bool `yaml:"include_factories"`
	// IgnoreBuildConstrained skips files carrying a build constraint, which
	// legitimately split a type's methods by platform.
	IgnoreBuildConstrained bool `yaml:"ignore_build_constrained"`
	IgnoreTests            bool `yaml:"ignore_tests"`
}

// typeHome records where a type and its associated declarations were found.
type typeHome struct {
	name    string
	declIn  string
	members map[string][]string
}

var typeCohesion = &rule.Rule{
	ID:       "org/type-cohesion",
	Category: rule.Organization,
	Tier:     rule.Syntax,
	Summary:  "a type, its factory and its methods live in one file",
	Default:  diag.Error,
	Doc: `A type, its factory, and all of its methods live in one file.

	registry.go       type Registry struct { ... }
	registry_ops.go   func (r *Registry) Register(...)      violation
	registry_new.go   func NewRegistry() *Registry          violation

Rationale: a type and its methods are one unit of meaning. Split across files
there is no single place that answers "what can this do?", and the answer has
to be assembled by grep — which reliably misses the method in the file nobody
thought to look in. The split also breaks review: a change to an invariant
touches the struct in one file and the methods that maintain it in three
others, and no reviewer sees the whole change at once.

To fix: move the methods next to the type they belong to.

This rule outranks the file budgets: a method's home is its type, and it is
never relocated to satisfy a count. That is why the budgets exclude methods.

Build-constrained files are exempt, since splitting a type's methods by platform
is what build tags are for. Test files are exempt, per docs/decisions.md D6.

Configure in .goorg.yaml:

	settings:
	  org/type-cohesion:
	    include_factories: true
	    ignore_build_constrained: true
	    ignore_tests: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := typeCohesionSettings{
			IncludeFactories:       true,
			IgnoreBuildConstrained: true,
			IgnoreTests:            true,
		}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Project.Packages {
			out = append(out, checkPackageCohesion(c, pkg, &s)...)
		}
		return out
	},
}

// checkPackageCohesion reports types whose declarations span files.
func checkPackageCohesion(c *rule.Context, pkg *project.Package, s *typeCohesionSettings) []diag.Diagnostic {
	homes := map[string]*typeHome{}
	home := func(name string) *typeHome {
		h, ok := homes[name]
		if !ok {
			h = &typeHome{name: name, members: map[string][]string{}}
			homes[name] = h
		}
		return h
	}

	for _, f := range pkg.Files {
		if s.IgnoreTests && f.IsTest {
			continue
		}
		if s.IgnoreBuildConstrained && hasBuildConstraint(f) {
			continue
		}
		for _, d := range classify(f) {
			if d.owner == "" {
				continue
			}
			switch {
			case d.isMethod:
				h := home(d.owner)
				h.members[f.Name] = append(h.members[f.Name], "method "+d.name)
			case d.isFunc:
				if !s.IncludeFactories {
					continue
				}
				h := home(d.owner)
				h.members[f.Name] = append(h.members[f.Name], "factory "+d.name)
			default:
				home(d.owner).declIn = f.Name
			}
		}
	}

	var out []diag.Diagnostic
	names := make([]string, 0, len(homes))
	for name := range homes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		h := homes[name]
		// A type declared outside this package — an alias target, or a
		// receiver on an imported type — has no home here to be split from.
		if h.declIn == "" {
			continue
		}
		var strays []string
		for file := range h.members {
			if file != h.declIn {
				strays = append(strays, file)
			}
		}
		if len(strays) == 0 {
			continue
		}
		sort.Strings(strays)
		out = append(out, diag.Diagnostic{
			Position: diag.Position{Path: pkg.Dir + "/" + strays[0]},
			Message: fmt.Sprintf("%s of type %s live away from its declaration in %s",
				plural(countStrays(h, strays), "member"), name, h.declIn),
			Help: fmt.Sprintf("move them into %s, beside the type they belong to", h.declIn),
		})
	}
	return out
}

func countStrays(h *typeHome, files []string) int {
	n := 0
	for _, f := range files {
		n += len(h.members[f])
	}
	return n
}

// hasBuildConstraint reports whether a file carries a //go:build line.
func hasBuildConstraint(f *project.File) bool {
	for _, group := range f.Syntax.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:build") {
				return true
			}
		}
		// Constraints must precede the package clause; anything later is an
		// ordinary comment.
		if group.End() > f.Syntax.Package {
			break
		}
	}
	return false
}
