package directory

import (
	"fmt"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// topLevelLayoutSettings is the configurable surface of dir/top-level-layout.
type topLevelLayoutSettings struct {
	// Roots are the top-level directories permitted to contain Go packages.
	Roots []string `yaml:"roots"`
	// AllowGoFilesAtRoot permits .go files in the repository root itself.
	AllowGoFilesAtRoot bool `yaml:"allow_go_files_at_root"`
}

var topLevelLayout = &rule.Rule{
	ID:       "dir/top-level-layout",
	Category: rule.Directory,
	Tier:     rule.Syntax,
	Summary:  "only the configured roots may contain Go packages",
	Default:  diag.Error,
	Doc: `At the repository root, only pkg/, cmd/ and internal/ may contain Go
packages. Other top-level directories are permitted without restriction as long
as they contain no .go files at any depth — docs/, deploy/, scripts/ are all
fine. This rule is about Go packages, not about directories in general.

	docs/architecture.md          OK — no Go
	scripts/release.sh            OK — no Go
	tools/generate/main.go        violation — a Go package outside a root
	main.go                       violation — a Go file at the repository root

Rationale: three roots, three meanings, no overlap. cmd/ is what the repository
produces, pkg/ is what other repositories may import, internal/ is what only
this one may use. Every Go file is therefore classified by its path alone —
"can I import this?" is answered by looking, never by asking. A fourth
top-level Go directory reopens that question for the whole repository.

To fix: move the package under one of the roots, choosing by who is allowed to
import it.

Configure in .goorg.yaml:

	settings:
	  dir/top-level-layout:
	    roots: [pkg, cmd, internal]
	    allow_go_files_at_root: false`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := topLevelLayoutSettings{Roots: defaultRoots}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		permitted := map[string]bool{}
		for _, r := range s.Roots {
			permitted[strings.Trim(strings.TrimSpace(r), "/")] = true
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Project.Packages {
			root := rootOf(pkg.Dir)
			switch {
			case root == "":
				if s.AllowGoFilesAtRoot {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: anchor(c, pkg),
					Message:  "Go files sit in the repository root",
					Help:     fmt.Sprintf("move them under one of: %s", strings.Join(s.Roots, ", ")),
				})
			case !permitted[root]:
				out = append(out, diag.Diagnostic{
					Position: anchor(c, pkg),
					Message: fmt.Sprintf("package %s is under %s/, which may not contain Go packages",
						pkg.Name, root),
					Help: fmt.Sprintf("move it under one of: %s", strings.Join(s.Roots, ", ")),
				})
			}
		}
		return out
	},
}
