package organization

import (
	"fmt"
	"go/ast"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// instanceFuncSettings is the configurable surface of the rule.
type instanceFuncSettings struct {
	Name            string `yaml:"name"`
	RequireSyncOnce bool   `yaml:"require_sync_once"`
	// ForbidInit reports a singleton constructed in an init function.
	ForbidInit bool `yaml:"forbid_init"`
	// ForbidExternalAssignment reports assignment outside the once-guarded
	// closure.
	ForbidExternalAssignment bool `yaml:"forbid_external_assignment"`
}

var singletonInstanceFunc = &rule.Rule{
	ID:       "org/singleton-instance-func",
	Category: rule.Organization,
	Tier:     rule.Syntax,
	Summary:  "a singleton is built by an unexported instance function guarded by sync.Once",
	Default:  diag.Error,
	Doc: `A singleton is constructed by an unexported function named instance,
and that function guards construction with sync.Once.

	func instance() *Registry {                                   OK
		registryOnce.Do(func() { registry = &Registry{...} })
		return registry
	}

	func init() { registry = &Registry{...} }                     violation
	func instance() *Registry {                                   violation
		if registry == nil { registry = &Registry{...} }
		return registry
	}

Rationale: the unguarded lazy form is a data race that will not reproduce in
testing and will not be caught in review, because it looks exactly like correct
code. The init form is not racy but is worse in another way: it fixes
construction order across the whole package graph, runs whether or not the
singleton is ever used, and gives no place to return an error. sync.Once is the
one spelling with none of those problems, and mandating a single accessor name
means every singleton in a codebase can be enumerated by grepping one
identifier.

To fix: move construction into a sync.Once closure inside the accessor, and
make every other reference go through the accessor rather than the variable.

Configure in .goorg.yaml:

	settings:
	  org/singleton-instance-func:
	    name: instance
	    require_sync_once: true
	    forbid_init: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := instanceFuncSettings{
			Name:                     defaultInstanceFunc,
			RequireSyncOnce:          true,
			ForbidInit:               true,
			ForbidExternalAssignment: true,
		}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range c.Project.Files() {
			if f.IsTest {
				continue
			}
			sg := detectSingleton(f, s.Name)
			if sg == nil {
				continue
			}
			if sg.accessor == nil {
				// org/singleton-layout already reports the missing accessor;
				// repeating it here would be the same defect twice.
				continue
			}
			if s.RequireSyncOnce && !usesOnceDo(sg.accessor) {
				out = append(out, diag.Diagnostic{
					Position: c.Pos(sg.accessor),
					Message:  fmt.Sprintf("%s does not guard construction with sync.Once", s.Name),
					Help:     "wrap construction in once.Do; a nil check races under concurrent first use",
				})
			}
			if s.ForbidExternalAssignment {
				names := declaredNames(sg.state)
				for _, fn := range assignsOutsideOnce(f, names, s.Name) {
					out = append(out, diag.Diagnostic{
						Position: c.Pos(fn),
						Message: fmt.Sprintf("%s assigns the singleton outside %s",
							funcLabel(fn), s.Name),
						Help: "construct it only inside the once-guarded closure",
					})
				}
			}
			if s.ForbidInit {
				out = append(out, checkNoInitConstruction(c, f, sg)...)
			}
		}
		return out
	},
}

// checkNoInitConstruction reports a singleton built in an init function.
func checkNoInitConstruction(c *rule.Context, f *project.File, sg *singleton) []diag.Diagnostic {
	names := declaredNames(sg.state)
	var out []diag.Diagnostic
	for _, node := range f.Syntax.Decls {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "init" || fn.Body == nil {
			continue
		}
		if !assignsAny(fn.Body, names) {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: c.Pos(fn),
			Message:  "singleton is constructed in an init function",
			Help:     "construct it lazily in the accessor, guarded by sync.Once",
		})
	}
	return out
}

func funcLabel(fn *ast.FuncDecl) string {
	if fn.Recv != nil {
		return fmt.Sprintf("method %s", fn.Name.Name)
	}
	return fmt.Sprintf("func %s", fn.Name.Name)
}
