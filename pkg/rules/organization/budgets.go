package organization

import (
	"fmt"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// budgetSettings is shared by the three file budgets. Each rule decodes its own
// copy, so the limits stay independently configurable.
type budgetSettings struct {
	Limit int `yaml:"limit"`
	// CountMethods must stay false by default. A type with more methods than
	// the file's budget cannot satisfy both this and org/type-cohesion, and
	// method count is a property of the type — capped by
	// logic/max-object-members — not of the file.
	CountMethods bool `yaml:"count_methods"`
}

// privateBudgetSettings adds the conditional that makes the private-function
// budget bearable.
type privateBudgetSettings struct {
	Limit              int  `yaml:"limit"`
	CountMethods       bool `yaml:"count_methods"`
	WhenFileHasExports bool `yaml:"when_file_has_exports"`
}

var maxFunctionsPerFile = &rule.Rule{
	ID:       "org/max-functions-per-file",
	Category: rule.Organization,
	Tier:     rule.Syntax,
	Summary:  "a file declares at most a configured number of functions",
	Default:  diag.Error,
	Doc: `A file declares at most N functions. Methods are counted against the
owning type's budget, not the file's.

Rationale: function count is the closest available proxy for how many distinct
things a file does. It is a better measure than line count for this purpose —
one long function is a logic/ problem, whereas twenty short ones is an org/
problem, and only the second means the file has stopped being about one thing.

To fix: split the file along the responsibilities it has accumulated. The split
must not create a subpackage; dir/max-package-depth forbids that.

count_methods defaults to false and should stay there. A type with more methods
than this budget cannot satisfy both this rule and org/type-cohesion, which
requires a type's methods to live in one file. Method count belongs to the type
and is capped by logic/max-object-members instead.

Test files are exempt, per docs/decisions.md D6.

Configure in .goorg.yaml:

	settings:
	  org/max-functions-per-file:
	    limit: 15
	    count_methods: false`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := budgetSettings{Limit: 15}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.Limit <= 0 {
			return nil
		}

		var out []diag.Diagnostic
		for _, f := range nonTestFiles(c) {
			exported, unexported := countFuncs(f, s.CountMethods)
			total := exported + unexported
			if total <= s.Limit {
				continue
			}
			out = append(out, diag.Diagnostic{
				Position: rule.FilePos(f),
				Message:  fmt.Sprintf("file declares %s, over the limit of %d", plural(total, "function"), s.Limit),
				Help:     "split the file along the responsibilities it has accumulated",
			})
		}
		return out
	},
}

var maxPublicFunctions = &rule.Rule{
	ID:       "org/max-public-functions",
	Category: rule.Organization,
	Tier:     rule.Syntax,
	Summary:  "a file declares at most a configured number of exported functions",
	Default:  diag.Error,
	Doc: `A file declares at most N exported functions.

Rationale: exported functions are the package's API surface, and surface
concentrated in one file is surface nobody owns. A file with a dozen exported
functions is either a package pretending to be a file, or a grab-bag — and it
is the file every subsequent addition gets appended to, because it is already
the one that "has the API in it". A tight cap forces the question *which
package does this belong to* while the answer is still cheap.

To fix: move a cohesive subset of the API into its own file, or its own
package.

Test files are exempt, per docs/decisions.md D6.

Configure in .goorg.yaml:

	settings:
	  org/max-public-functions:
	    limit: 6`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := budgetSettings{Limit: 6}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.Limit <= 0 {
			return nil
		}

		var out []diag.Diagnostic
		for _, f := range nonTestFiles(c) {
			exported, _ := countFuncs(f, s.CountMethods)
			if exported <= s.Limit {
				continue
			}
			out = append(out, diag.Diagnostic{
				Position: rule.FilePos(f),
				Message: fmt.Sprintf("file exports %s, over the limit of %d",
					plural(exported, "function"), s.Limit),
				Help: "move a cohesive subset of the API into its own file or package",
			})
		}
		return out
	},
}

var maxPrivateFunctions = &rule.Rule{
	ID:       "org/max-private-functions",
	Category: rule.Organization,
	Tier:     rule.Syntax,
	Summary:  "a file with exports declares few unexported functions",
	Default:  diag.Error,
	Doc: `A file that declares at least one exported function may declare at most
N unexported functions. A file of only unexported functions is unconstrained —
that is a helper file, and helper files are allowed to be helper files.

Rationale: unexported functions piling up beneath an exported one are the
visible residue of a file doing too much. Each is a step the exported function
needs but does not name, and past a small number the file has an implementation
of its own that is no longer readable as "the API plus a little glue".

To fix: keeping the cap severe forces a choice between two good outcomes. If the
helpers were really one cohesive thing, they become a type with methods. If they
were unrelated, they belong in different files near the code that uses them —
which is what org/consumer-locality will independently say.

The shipped limit of 5 is calibrated against real Go rather than chosen for
severity; see docs/decisions.md D5. It still flags roughly a fifth of standard
library files, because the standard library freely mixes public API and private
implementation in one file, which is the habit this rule exists to break.

Test files are exempt, per docs/decisions.md D6.

Configure in .goorg.yaml:

	settings:
	  org/max-private-functions:
	    limit: 5
	    when_file_has_exports: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := privateBudgetSettings{Limit: 5, WhenFileHasExports: true}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.Limit <= 0 {
			return nil
		}

		var out []diag.Diagnostic
		for _, f := range nonTestFiles(c) {
			exported, unexported := countFuncs(f, s.CountMethods)
			if s.WhenFileHasExports && exported == 0 {
				continue
			}
			if unexported <= s.Limit {
				continue
			}
			out = append(out, diag.Diagnostic{
				Position: rule.FilePos(f),
				Message: fmt.Sprintf("file declares %s beside its exported API, over the limit of %d",
					plural(unexported, "unexported function"), s.Limit),
				Help: "group cohesive helpers into a type, or move unrelated ones beside the code that uses them",
			})
		}
		return out
	},
}
