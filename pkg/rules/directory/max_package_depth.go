package directory

import (
	"fmt"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// maxPackageDepthSettings maps each root to its maximum package depth.
type maxPackageDepthSettings struct {
	Pkg      int `yaml:"pkg"`
	Cmd      int `yaml:"cmd"`
	Internal int `yaml:"internal"`
}

// limits returns the configured depth per root.
func (s *maxPackageDepthSettings) limits() map[string]int {
	return map[string]int{"pkg": s.Pkg, "cmd": s.Cmd, "internal": s.Internal}
}

var maxPackageDepth = &rule.Rule{
	ID:       "dir/max-package-depth",
	Category: rule.Directory,
	Tier:     rule.Syntax,
	Summary:  "no subdomains and no subpackages",
	Default:  diag.Error,
	Doc: `A Go package may not nest below the depth its root allows. Depth counts
from the root, whose immediate children are depth 1:

	pkg/billing/                     depth 1   domain
	pkg/billing/invoice/             depth 2   package
	pkg/billing/invoice/pdf/         depth 3   too deep — a subpackage
	pkg/billing/eu/invoice/          depth 3   too deep — a subdomain

Only package directories count. An asset directory may sit below a package
without violating this rule, because it holds no Go code:

	pkg/billing/invoice/templates/invoice.tmpl     OK
	pkg/billing/invoice/templates/render.go        violation — now a package

Rationale: nesting implies a hierarchy Go does not have. pkg/billing/invoice/pdf
reads as though pdf were part of invoice and somehow more private than it, but
Go has exactly one visibility boundary below the module — internal/ — and
nesting is not it. pdf is equally importable from anywhere either way, so the
hierarchy communicates a restriction that does not exist. Subdomains have the
same problem one level up, and they compound: once pkg/billing/eu/ exists, the
next team adds pkg/billing/eu/vat/, and the path to a package stops being
predictable.

To fix: ask whether this is a distinct responsibility. If it is, make it a
sibling package in the same domain. If it is not, it belongs in the file it
came from.

This rule sets the maximum; dir/domain-layout sets the minimum, so the two
never report the same directory.

Configure in .goorg.yaml:

	settings:
	  dir/max-package-depth:
	    pkg: 2
	    cmd: 2
	    internal: 2`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := maxPackageDepthSettings{Pkg: 2, Cmd: 2, Internal: 2}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for root, limit := range s.limits() {
			if limit <= 0 {
				continue
			}
			for _, pkg := range packagesUnder(c.Project, root) {
				depth := depthUnder(root, pkg.Dir)
				if depth <= limit {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: anchor(c, pkg),
					Message: fmt.Sprintf("package %s is %d levels under %s/, over the limit of %d",
						pkg.Name, depth, root, limit),
					Help: "make it a sibling package in the same domain, or fold it into the package it came from",
				})
			}
		}
		diag.Sort(out)
		return out
	},
}
