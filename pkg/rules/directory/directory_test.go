package directory

import (
	"strings"
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/lint/ruletest"
)

// conforming is a tree that satisfies every rule in the family. Each rule
// asserts it stays silent here, which is what stops a rule from being written
// so broadly that it fires on correct code.
var conforming = map[string]string{
	"go.mod":                         "module example.com/ok\n\ngo 1.25.0\n",
	"README.md":                      "# ok\n",
	"docs/design.md":                 "# design\n",
	"cmd/lint/tool/main.go":          "package main\n\nfunc main() {}\n",
	"pkg/billing/invoice/invoice.go": "package invoice\n",
	"pkg/billing/ledger/ledger.go":   "package ledger\n",
	"internal/cli/cli.go":            "package cli\n",
	"internal/cli/run.go":            "package cli\n",
}

func TestMaxEntries(t *testing.T) {
	files := map[string]string{"go.mod": "module x\n"}
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		files["pkg/big/wide/"+name+".go"] = "package wide\n"
	}

	t.Run("over the limit", func(t *testing.T) {
		got := ruletest.RunWith(t, maxEntries, files, maxEntriesSettings{Limit: 3})
		ruletest.Assert(t, got, []string{"directory holds 6 entries, over the limit of 3"})
	})

	t.Run("under the limit is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, maxEntries, files), nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, maxEntries, conforming), nil)
	})
}

// TestMaxEntriesOverrideSpecificity pins that the longest literal prefix wins,
// so a narrow override beats a broad one regardless of map order.
func TestMaxEntriesOverrideSpecificity(t *testing.T) {
	s := maxEntriesSettings{
		Limit:     20,
		Overrides: map[string]int{"**": 5, "docs/**": 50, "docs/api/**": 2},
	}
	for range 20 {
		if got := s.limitFor("docs/api/v1"); got != 2 {
			t.Fatalf("limitFor(docs/api/v1) = %d, want 2", got)
		}
		if got := s.limitFor("docs/guide"); got != 50 {
			t.Fatalf("limitFor(docs/guide) = %d, want 50", got)
		}
		if got := s.limitFor("pkg/lint"); got != 5 {
			t.Fatalf("limitFor(pkg/lint) = %d, want 5", got)
		}
	}
}

func TestTopLevelLayout(t *testing.T) {
	t.Run("go package outside a root", func(t *testing.T) {
		got := ruletest.Run(t, topLevelLayout, map[string]string{
			"go.mod":                "module x\n",
			"tools/generate/gen.go": "package generate\n",
		})
		ruletest.Assert(t, got, []string{"is under tools/, which may not contain Go packages"})
	})

	t.Run("go files at the repository root", func(t *testing.T) {
		got := ruletest.Run(t, topLevelLayout, map[string]string{
			"go.mod":  "module x\n",
			"main.go": "package main\n\nfunc main() {}\n",
		})
		ruletest.Assert(t, got, []string{"Go files sit in the repository root"})
	})

	t.Run("non-go directories are unconstrained", func(t *testing.T) {
		got := ruletest.Run(t, topLevelLayout, map[string]string{
			"go.mod":              "module x\n",
			"docs/a.md":           "# a\n",
			"scripts/release.sh":  "#!/bin/sh\n",
			"deploy/k8s/app.yaml": "kind: Deployment\n",
			"pkg/a/b/b.go":        "package b\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, topLevelLayout, conforming), nil)
	})
}

func TestDomainLayout(t *testing.T) {
	t.Run("package directly under a root", func(t *testing.T) {
		got := ruletest.Run(t, domainLayout, map[string]string{
			"go.mod":              "module x\n",
			"pkg/invoice/inv.go":  "package invoice\n",
			"pkg/b/ledger/led.go": "package ledger\n",
		})
		ruletest.Assert(t, got, []string{"package invoice sits directly under pkg/"})
	})

	t.Run("internal defaults to any", func(t *testing.T) {
		got := ruletest.Run(t, domainLayout, map[string]string{
			"go.mod":              "module x\n",
			"internal/cli/cli.go": "package cli\n",
		})
		ruletest.Assert(t, got, nil)
	})

	// Deeper nesting is dir/max-package-depth's job. This rule sets the
	// minimum only, so the two never report the same directory twice.
	t.Run("deeper nesting is not this rule's finding", func(t *testing.T) {
		got := ruletest.Run(t, domainLayout, map[string]string{
			"go.mod":                       "module x\n",
			"pkg/billing/invoice/pdf/p.go": "package pdf\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("unknown mode is a settings error", func(t *testing.T) {
		got := ruletest.RunWith(t, domainLayout, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n",
		}, domainLayoutSettings{Pkg: "sideways"})
		ruletest.Assert(t, got, []string{"invalid rule settings"})
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, domainLayout, conforming), nil)
	})
}

func TestMaxPackageDepth(t *testing.T) {
	t.Run("subpackage", func(t *testing.T) {
		got := ruletest.Run(t, maxPackageDepth, map[string]string{
			"go.mod":                       "module x\n",
			"pkg/billing/invoice/pdf/p.go": "package pdf\n",
		})
		ruletest.Assert(t, got, []string{"package pdf is 3 levels under pkg/, over the limit of 2"})
	})

	t.Run("subdomain", func(t *testing.T) {
		got := ruletest.Run(t, maxPackageDepth, map[string]string{
			"go.mod":                      "module x\n",
			"pkg/billing/eu/invoice/i.go": "package invoice\n",
		})
		ruletest.Assert(t, got, []string{"package invoice is 3 levels under pkg/, over the limit of 2"})
	})

	// This is the interaction that makes dir/embedded-assets viable: an asset
	// directory below a package must not be counted as a package itself.
	t.Run("asset directory below a package is exempt", func(t *testing.T) {
		got := ruletest.Run(t, maxPackageDepth, map[string]string{
			"go.mod":                                 "module x\n",
			"pkg/billing/invoice/invoice.go":         "package invoice\n",
			"pkg/billing/invoice/templates/inv.tmpl": "hello\n",
			"pkg/billing/invoice/templates/rec.tmpl": "hello\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("a go file turns an asset directory into a violation", func(t *testing.T) {
		got := ruletest.Run(t, maxPackageDepth, map[string]string{
			"go.mod":                         "module x\n",
			"pkg/billing/invoice/invoice.go": "package invoice\n",
			"pkg/billing/invoice/templates/render.go": "package templates\n",
		})
		ruletest.Assert(t, got, []string{"package templates is 3 levels under pkg/"})
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, maxPackageDepth, conforming), nil)
	})
}

func TestDomainHasNoGoFiles(t *testing.T) {
	t.Run("go file in a domain directory", func(t *testing.T) {
		got := ruletest.Run(t, domainNoGoFiles, map[string]string{
			"go.mod":                         "module x\n",
			"pkg/billing/types.go":           "package billing\n",
			"pkg/billing/invoice/invoice.go": "package invoice\n",
		})
		ruletest.Assert(t, got, []string{"domain directory pkg/billing contains 1 Go file"})
	})

	t.Run("internal is not a domain root", func(t *testing.T) {
		got := ruletest.Run(t, domainNoGoFiles, map[string]string{
			"go.mod":              "module x\n",
			"internal/cli/cli.go": "package cli\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, domainNoGoFiles, conforming), nil)
	})
}

func TestEmbeddedAssets(t *testing.T) {
	t.Run("asset beside go source", func(t *testing.T) {
		got := ruletest.Run(t, embeddedAssets, map[string]string{
			"go.mod":                          "module x\n",
			"pkg/billing/invoice/invoice.go":  "package invoice\n",
			"pkg/billing/invoice/schema.json": "{}\n",
		})
		ruletest.Assert(t, got, []string{"schema.json sits beside Go source"})
	})

	t.Run("asset in a subdirectory is correct", func(t *testing.T) {
		got := ruletest.Run(t, embeddedAssets, map[string]string{
			"go.mod":                                 "module x\n",
			"pkg/billing/invoice/invoice.go":         "package invoice\n",
			"pkg/billing/invoice/schema/schema.json": "{}\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("allowed names are exempt", func(t *testing.T) {
		got := ruletest.Run(t, embeddedAssets, map[string]string{
			"go.mod":                         "module x\n",
			"pkg/billing/invoice/invoice.go": "package invoice\n",
			"pkg/billing/invoice/README.md":  "# invoice\n",
		})
		ruletest.Assert(t, got, nil)
	})

	// A directory with no Go source is an asset directory, not a package, so
	// its contents are none of this rule's business.
	t.Run("directory without go source is unconstrained", func(t *testing.T) {
		got := ruletest.Run(t, embeddedAssets, map[string]string{
			"go.mod":              "module x\n",
			"deploy/k8s/app.yaml": "kind: Deployment\n",
			"deploy/k8s/svc.yaml": "kind: Service\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, embeddedAssets, conforming), nil)
	})
}

// TestNoDirectoryIsReportedTwice guards the ownership split between
// dir/domain-layout and dir/domain-has-no-go-files.
//
// A depth-1 directory holding Go files is exactly one defect: a domain polluted
// with code, or a package that never got a domain. Both rules once fired on
// both cases, so a single mistake produced two findings and two contradictory
// pieces of advice.
func TestNoDirectoryIsReportedTwice(t *testing.T) {
	violating := map[string]string{
		"go.mod": "module x\n",
		// A package that never got a domain: no packages beneath it.
		"pkg/invoice/inv.go": "package invoice\n",
		// A domain polluted with code: it has a package beneath it.
		"pkg/billing/types.go":           "package billing\n",
		"pkg/billing/invoice/invoice.go": "package invoice\n",
		// A binary directly under cmd/, likewise with nothing beneath it.
		"cmd/tool/main.go": "package main\n\nfunc main() {}\n",
	}

	seen := map[string][]string{}
	for _, r := range Rules() {
		for _, finding := range ruletest.Run(t, r, violating) {
			path, _, _ := strings.Cut(finding, ":")
			seen[path] = append(seen[path], r.ID)
		}
	}
	for path, ids := range seen {
		if len(ids) > 1 {
			t.Errorf("%s reported by %d rules (%s); each defect must have one owner",
				path, len(ids), strings.Join(ids, ", "))
		}
	}

	// The split must still report every defect, not silence one of them.
	byRule := map[string]int{}
	for _, ids := range seen {
		for _, id := range ids {
			byRule[id]++
		}
	}
	if byRule["dir/domain-layout"] != 2 {
		t.Errorf("dir/domain-layout fired %d times, want 2", byRule["dir/domain-layout"])
	}
	if byRule["dir/domain-has-no-go-files"] != 1 {
		t.Errorf("dir/domain-has-no-go-files fired %d times, want 1", byRule["dir/domain-has-no-go-files"])
	}
}

// TestFamilyIsWellFormed holds every rule to the documentation and determinism
// standards, and to the gofmt conformance check.
func TestFamilyIsWellFormed(t *testing.T) {
	rules := Rules()
	if len(rules) != 6 {
		t.Fatalf("family has %d rules, want 6", len(rules))
	}
	for _, r := range rules {
		t.Run(r.ID, func(t *testing.T) {
			ruletest.AssertWellFormed(t, r)
			if r.Tier != rule.Syntax {
				t.Errorf("tier = %v, want syntax", r.Tier)
			}
			ruletest.AssertDeterministic(t, r, conforming)
			ruletest.AssertGofmtStable(t, r, conforming)
		})
	}
}

// TestFamilyBuildsAsASet catches a malformed or duplicate ID, which NewSet
// rejects.
func TestFamilyBuildsAsASet(t *testing.T) {
	if _, err := rule.NewSet(Rules()); err != nil {
		t.Fatalf("Rules() does not form a valid set: %v", err)
	}
}

// TestConformingTreeIsSilentForEveryRule is the whole-family version of the
// per-rule silence checks: no rule may fire on a tree that follows the standard.
func TestConformingTreeIsSilentForEveryRule(t *testing.T) {
	for _, r := range Rules() {
		if got := ruletest.Run(t, r, conforming); len(got) != 0 {
			t.Errorf("%s fired on a conforming tree: %v", r.ID, got)
		}
	}
}
