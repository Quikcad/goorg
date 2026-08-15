package directory

import (
	"fmt"
	"path"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// domainLayoutSettings maps each root to its layout mode.
type domainLayoutSettings struct {
	Pkg      string `yaml:"pkg"`
	Cmd      string `yaml:"cmd"`
	Internal string `yaml:"internal"`
}

// modes returns the configured mode per root.
func (s *domainLayoutSettings) modes() map[string]string {
	return map[string]string{"pkg": s.Pkg, "cmd": s.Cmd, "internal": s.Internal}
}

var domainLayout = &rule.Rule{
	ID:       "dir/domain-layout",
	Category: rule.Directory,
	Tier:     rule.Syntax,
	Summary:  "packages sit at the nesting depth their root's mode requires",
	Default:  diag.Error,
	Doc: `Go packages under a root sit at <root>/<domain>/<package>, and the
domain layer is either mandatory or forbidden — never mixed.

	domains    every package belongs to a domain   pkg/billing/invoice/
	packages   no domain layer                     pkg/invoice/
	any        either shape; the rule is off for that root

Under "domains":

	pkg/billing/invoice/invoice.go     OK
	pkg/billing/ledger/ledger.go       OK — second package, same domain
	pkg/invoice/invoice.go             violation — no domain layer

Rationale: a domain is the unit of ownership. When packages sit directly under
pkg/, the root becomes a flat list that grows monotonically and expresses no
relationships — nothing says that invoice, ledger and dunning are one system
while imageproxy is not. Grouping by domain makes the seams in the system
visible in the file tree, and makes "who owns this?" answerable from the path.
Allowing both shapes at once is worse than either alone, which is why this is a
mode and not a pair of toggles.

This rule enforces the **minimum** nesting only. Anything deeper is
dir/max-package-depth's job, so the two never report the same directory twice.
That also means "packages" mode is enforced by setting max-package-depth to 1
for that root; this rule alone cannot forbid a domain layer.

To fix: move the package into a domain named for the area of the system it
belongs to, or set the root's mode to match how the repository is really laid
out.

Configure in .goorg.yaml:

	settings:
	  dir/domain-layout:
	    pkg: domains
	    cmd: domains
	    internal: any`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := domainLayoutSettings{Pkg: modeDomains, Cmd: modeDomains, Internal: modeAny}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for root, mode := range s.modes() {
			required, err := requiredDepth(mode)
			if err != nil {
				return settingsError(fmt.Errorf("dir/domain-layout: %s: %w", root, err))
			}
			if required == 0 {
				continue
			}
			for _, pkg := range packagesUnder(c.Project, root) {
				depth := depthUnder(root, pkg.Dir)
				if depth >= required {
					continue
				}
				// A directory with packages beneath it is a domain that has
				// picked up stray code, which dir/domain-has-no-go-files owns.
				if actsAsDomain(c.Project, pkg.Dir) {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: anchor(c, pkg),
					Message: fmt.Sprintf("package %s sits directly under %s/; it must belong to a domain",
						pkg.Name, root),
					Help: fmt.Sprintf("move it to %s/<domain>/%s/", root, path.Base(pkg.Dir)),
				})
			}
		}
		diag.Sort(out)
		return out
	},
}

// requiredDepth converts a mode into the minimum depth a package must sit at,
// or 0 when the rule does not apply to that root.
func requiredDepth(mode string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case modeDomains:
		return 2, nil
	case modePackages, modeAny, "":
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown mode %q (want domains, packages, or any)", mode)
	}
}
