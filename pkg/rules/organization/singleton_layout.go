package organization

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// singletonLayoutSettings is the configurable surface of org/singleton-layout.
type singletonLayoutSettings struct {
	// InstanceFunc is the mandated name of the accessor.
	InstanceFunc string `yaml:"instance_func"`
	// ExclusiveFile requires the singleton to be the only thing in its file.
	ExclusiveFile bool `yaml:"exclusive_file"`
}

var singletonLayout = &rule.Rule{
	ID:        "org/singleton-layout",
	Category:  rule.Organization,
	Tier:      rule.Syntax,
	Placement: rule.PlacementOrder,
	Summary:   "a singleton file is laid out as state, accessor, then exported functions",
	Default:   diag.Error,
	Doc: `A file declaring a singleton is laid out as the singleton's variables,
then the unexported instance function, then the exported accessors.

	var registryOnce sync.Once
	var registry *Registry

	func instance() *Registry {
		registryOnce.Do(func() { registry = &Registry{...} })
		return registry
	}

	func Register(name string, h Handler) error { return instance().register(name, h) }
	func Lookup(name string) (Handler, bool)    { return instance().lookup(name) }

This is the one place an unexported function is required to precede exported
ones, and it overrides org/private-functions-last. The instance function is not
a helper — it is the accessor the rest of the file is built on, and every
exported function below it is a one-line delegation.

Rationale: a singleton is the one construct where package-level mutable state
is sanctioned, so it earns a fixed shape that makes the sanction visible.
Everything about the pattern that can go wrong — initialisation racing,
initialisation happening twice, some other file reaching past the accessor —
is prevented by the shape rather than by discipline. Putting the accessor
directly under the variables also means the whole initialisation story fits on
one screen: the state, the guard, and the single function permitted to build it.

To fix: order the file state, accessor, exported functions, and move anything
unrelated to its own file.

Configure in .goorg.yaml:

	settings:
	  org/singleton-layout:
	    instance_func: instance
	    exclusive_file: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := singletonLayoutSettings{InstanceFunc: defaultInstanceFunc, ExclusiveFile: true}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range c.Project.Files() {
			if f.IsTest {
				continue
			}
			sg := detectSingleton(f, s.InstanceFunc)
			if sg == nil {
				continue
			}
			out = append(out, checkSingletonOrder(c, f, sg, &s)...)
		}
		return out
	},
}

// checkSingletonOrder verifies the three-part shape of a singleton file.
func checkSingletonOrder(c *rule.Context, f *project.File, sg *singleton, s *singletonLayoutSettings) []diag.Diagnostic {
	var out []diag.Diagnostic

	if sg.accessor == nil {
		out = append(out, diag.Diagnostic{
			Position: c.Pos(sg.once),
			Message:  fmt.Sprintf("singleton has no %s function", s.InstanceFunc),
			Help:     fmt.Sprintf("add func %s() to construct it inside the sync.Once", s.InstanceFunc),
		})
		return out
	}

	accessorLine := c.Pos(sg.accessor).Line
	if onceLine := c.Pos(sg.once).Line; onceLine > accessorLine {
		out = append(out, diag.Diagnostic{
			Position: c.Pos(sg.once),
			Message:  "singleton state is declared after the accessor",
			Help:     "put the var declarations at the top of the file",
		})
	}
	for _, fn := range sg.exported {
		if c.Pos(fn).Line > accessorLine {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: c.Pos(fn),
			Message: fmt.Sprintf("exported %s appears before %s",
				fn.Name.Name, s.InstanceFunc),
			Help: fmt.Sprintf("move it below %s, which the rest of the file is built on", s.InstanceFunc),
		})
	}

	if s.ExclusiveFile {
		out = append(out, checkSingletonExclusive(c, f, sg)...)
	}
	return out
}

// checkSingletonExclusive reports declarations that are not part of the
// singleton, which belong in a file of their own.
func checkSingletonExclusive(c *rule.Context, f *project.File, sg *singleton) []diag.Diagnostic {
	belongs := map[ast.Decl]bool{sg.once: true}
	for _, d := range sg.state {
		belongs[d] = true
	}
	if sg.accessor != nil {
		belongs[ast.Decl(sg.accessor)] = true
	}
	for _, fn := range sg.exported {
		belongs[ast.Decl(fn)] = true
	}

	var out []diag.Diagnostic
	for _, node := range f.Syntax.Decls {
		if belongs[node] {
			continue
		}
		if gen, ok := node.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: c.Pos(node),
			Message:  "declaration is unrelated to the singleton in this file",
			Help:     "a singleton file holds its state, its accessor, and its exported functions and nothing else",
		})
	}
	return out
}
