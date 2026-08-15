package directory

import (
	"fmt"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// domainNoGoFilesSettings names the roots whose depth-1 directories are domains.
//
// The specification describes this rule as having no configuration, and the
// check itself is absolute — not one Go file. But the rule still has to know
// *which* directories are domains, and that depends on which roots use domain
// layout. Under `packages` mode a depth-1 directory is an ordinary package and
// must be left alone, so the roots list is a necessity rather than a knob.
type domainNoGoFilesSettings struct {
	Roots []string `yaml:"roots"`
}

var domainNoGoFiles = &rule.Rule{
	ID:       "dir/domain-has-no-go-files",
	Category: rule.Directory,
	Tier:     rule.Syntax,
	Summary:  "a domain directory contains no Go files whatsoever",
	Default:  diag.Error,
	Doc: `A domain directory contains no .go files. Not one. Not doc.go, not a
shared types.go, not a single helper.

	pkg/billing/
	├── invoice/          OK — a package
	├── ledger/           OK — a package
	└── types.go          violation — a Go file in a domain directory

Rationale: the moment a domain directory holds Go code it becomes a package,
and a package sitting above its siblings is where "shared" types accumulate.
Every package in the domain then imports it, it acquires a dependency on each
of them in turn, and the domain has an import cycle waiting to happen and a
file nobody can change safely.

Keeping the layer empty of code means a domain is purely a grouping. It has no
API, so nothing can depend on it, so it cannot rot.

To fix: if the type is genuinely shared across the domain's packages, give it
its own package within that domain, named for what it is rather than for the
fact that several things use it.

The rule has no threshold — one file defeats it. It applies only to roots laid
out with domains:

	settings:
	  dir/domain-has-no-go-files:
	    roots: [pkg, cmd]`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := domainNoGoFilesSettings{Roots: []string{"pkg", "cmd"}}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, root := range s.Roots {
			root = strings.Trim(strings.TrimSpace(root), "/")
			for _, pkg := range packagesUnder(c.Project, root) {
				if depthUnder(root, pkg.Dir) != 1 {
					continue
				}
				// Without packages beneath it this is not a domain at all, just
				// a package missing its domain — dir/domain-layout's finding.
				if !actsAsDomain(c.Project, pkg.Dir) {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: anchor(c, pkg),
					Message: fmt.Sprintf("domain directory %s contains %s",
						pkg.Dir, plural(len(pkg.Files), "Go file")),
					Help: "move the code into a package within the domain, named for what it provides",
				})
			}
		}
		diag.Sort(out)
		return out
	},
}
