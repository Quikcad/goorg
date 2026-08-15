package organization

import (
	"fmt"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// privateFunctionsLastSettings is the configurable surface of the rule.
type privateFunctionsLastSettings struct {
	// IncludeMethods applies the same split within each type's method run.
	IncludeMethods bool `yaml:"include_methods"`
}

var privateFunctionsLast = &rule.Rule{
	ID:       "org/private-functions-last",
	Category: rule.Organization,
	Tier:     rule.Syntax,
	Summary:  "unexported functions come after every exported one",
	Default:  diag.Error,
	Doc: `Unexported functions come after every exported function in the file.

	func Parse(s string) (*Doc, error)     OK — exported first
	func Render(d *Doc) string

	func normalize(s string) string        OK — unexported last
	func validate(d *Doc) error

Rationale: a file's exported functions are its purpose; its unexported
functions are how it achieves that. A reader arriving from another package is
looking for the former and will read past the latter, so putting helpers first
taxes every reader to save the author nothing. The split also makes the file's
public surface countable at a glance, which is what org/max-public-functions
measures.

To fix: move the exported function above the unexported ones.

Factories are exempt: a factory belongs beside the type it builds, and
org/member-order places it there. Forcing an exported factory above an
unexported one would pull it away from its own type.

Singleton files are exempt, and are the only sanctioned inversion: the
unexported instance accessor is required to appear *before* the exported
functions that delegate to it, because it is not a helper — it is what the rest
of the file is built on. Test files are exempt entirely.

Configure in .goorg.yaml:

	settings:
	  org/private-functions-last:
	    include_methods: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := privateFunctionsLastSettings{IncludeMethods: true}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range orderedFiles(c) {
			out = append(out, checkExportedFirst(c, f, false)...)
			if s.IncludeMethods {
				out = append(out, checkExportedFirst(c, f, true)...)
			}
		}
		return out
	},
}

// checkExportedFirst reports an exported declaration that follows an
// unexported one, either among free functions or within each type's methods.
func checkExportedFirst(c *rule.Context, f *fileDecls, methods bool) []diag.Diagnostic {
	// Methods are grouped by receiver: the split applies within each type's
	// run, not across the file, so two types never constrain each other.
	seenUnexported := map[string]string{}

	var out []diag.Diagnostic
	for _, d := range f.decls {
		if !d.isFunc || d.isMethod != methods {
			continue
		}
		// A factory sits with the type it builds, which outranks this rule —
		// otherwise an unexported factory for an early type would force every
		// later exported factory to move away from its own type.
		if !d.isMethod && d.sect != sectionFunctions {
			continue
		}
		group := ""
		if methods {
			group = d.owner
		}
		if !d.exported {
			if _, ok := seenUnexported[group]; !ok {
				seenUnexported[group] = d.name
			}
			continue
		}
		first, ok := seenUnexported[group]
		if !ok {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: c.Pos(d.node),
			Message:  fmt.Sprintf("exported %s appears after unexported %s", describe(d), first),
			Help:     "move it above the unexported declarations",
		})
	}
	return out
}
