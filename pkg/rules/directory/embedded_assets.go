package directory

import (
	"fmt"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/glob"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// embeddedAssetsSettings is the configurable surface of dir/embedded-assets.
type embeddedAssetsSettings struct {
	// Allow lists file names, or globs over base names, permitted beside .go
	// source despite not being Go.
	Allow []string `yaml:"allow"`
}

// permits reports whether a non-Go file may sit beside Go source.
func (s *embeddedAssetsSettings) permits(name string) bool {
	for _, pattern := range s.Allow {
		if pattern == name {
			return true
		}
		if glob.MatchPath(pattern, name) {
			return true
		}
	}
	return false
}

var embeddedAssets = &rule.Rule{
	ID:       "dir/embedded-assets",
	Category: rule.Directory,
	Tier:     rule.Syntax,
	Summary:  "non-Go files live in a subdirectory, never beside Go source",
	Default:  diag.Error,
	Doc: `Files that are not Go source live in a subdirectory of the package,
never alongside the .go files.

	pkg/billing/invoice/
	├── invoice.go                OK
	├── invoice_test.go           OK
	├── templates/                OK — an asset directory
	│   └── invoice.tmpl
	└── schema.json               violation — a non-Go file beside the source

//go:embed reaches into subdirectories without difficulty (//go:embed
templates, //go:embed templates/*), so this costs nothing at the call site. It
cannot reach outside the package directory, which is why the assets stay under
the package rather than moving to a shared tree.

An asset directory must contain no .go files — the moment it does, it is a
package and dir/max-package-depth applies to it.

Rationale: Go source and embedded assets are read for different reasons and
change on different schedules, but interleaved in one listing they compete for
the same attention. Worse, a bare listing gives no signal about which files are
compiled and which are data — a reader has to open the .json or .tmpl to
discover whether it is an input to the build, a fixture, or something nobody
deleted. A named subdirectory answers that in the path.

To fix: move the file into a subdirectory named for what it holds, and point
the //go:embed directive at that directory.

testdata/ is exempt by the Go toolchain's own convention. Configure the rest:

	settings:
	  dir/embedded-assets:
	    allow: [go.mod, go.sum, README.md, LICENSE, "*.md"]`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := embeddedAssetsSettings{
			Allow: []string{"go.mod", "go.sum", "go.work", "go.work.sum", "README.md", "LICENSE"},
		}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, d := range c.Project.Dirs {
			// Only a package directory can have assets "beside the source".
			if !d.HasGo {
				continue
			}
			for _, name := range d.NonGoFiles {
				if s.permits(name) {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: diag.Position{Path: joinPath(d.Rel, name)},
					Message:  fmt.Sprintf("%s sits beside Go source", name),
					Help:     "move it into a subdirectory and embed that directory instead",
				})
			}
		}
		return out
	},
}

func joinPath(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// settingsError converts a settings decode failure into a diagnostic pointing
// at the config file, so a typo in .goorg.yaml surfaces the same way a
// violation does instead of crashing the run.
func settingsError(err error) []diag.Diagnostic {
	return []diag.Diagnostic{{
		Position: diag.Position{Path: ".goorg.yaml", Line: 1},
		Message:  fmt.Sprintf("invalid rule settings: %v", strings.TrimSpace(err.Error())),
		Help:     "check the settings block for this rule against `goorg explain <rule>`",
	}}
}
