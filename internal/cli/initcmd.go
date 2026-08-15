package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/config"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

func runInit(env *Env, args []string) int {
	fs := newFlagSet(env, "init")
	root := fs.String("root", ".", "directory to write the config file into")
	force := fs.Bool("force", false, "overwrite an existing config file")
	stdout := fs.Bool("stdout", false, "print the config to stdout instead of writing a file")
	fs.Usage = func() {
		fmt.Fprint(env.Stderr, `Usage: goorg init [flags]

Writes a starter .goorg.yaml listing every rule at its shipped default, with
the settings blocks documented inline.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return ExitError
	}

	set, err := buildRuleSet()
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return ExitError
	}
	content := starterConfig(set)

	if *stdout {
		fmt.Fprint(env.Stdout, content)
		return ExitOK
	}

	path := filepath.Join(*root, config.FileNames[0])
	if !*force {
		switch _, err := os.Stat(path); {
		case err == nil:
			fmt.Fprintf(env.Stderr, "goorg: %s already exists; pass -force to overwrite\n", path)
			return ExitError
		case !errors.Is(err, os.ErrNotExist):
			fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
			return ExitError
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintf(env.Stderr, "goorg: write %s: %v\n", path, err)
		return ExitError
	}

	fmt.Fprintf(env.Stdout, "wrote %s\n", path)
	fmt.Fprintln(env.Stdout, "run `goorg check ./...` to see where you stand")
	return ExitOK
}

// starterConfig is generated from the live rule set, so a newly added rule
// appears in `goorg init` without anyone remembering to update a template.
func starterConfig(set *rule.Set) string {
	var b strings.Builder
	fmt.Fprintf(&b, `# goorg configuration — https://github.com/Quikcad/goorg#configuration
#
# Every rule is listed below at its shipped default. Severities are:
#   error     reported, and fails the run
#   warning   reported, does not fail the run
#   off       not run at all
#
# A key may be an exact rule ID or a glob. The most specific key wins, so
# "dir/*: warning" can still be overridden by "dir/domain-layout: error".
#
# Suppress a single finding in source instead, with a reason:
#   //goorg:ignore <rule-id> — <why>
version: %d

rules:
`, config.SchemaVersion)

	if set.Len() == 0 {
		b.WriteString("  # No rules are registered in this build.\n")
	}

	// Align severities into a column so the file stays readable as it grows.
	width := 0
	for _, r := range set.All() {
		if n := len(r.ID); n > width {
			width = n
		}
	}
	for _, cat := range rule.Categories() {
		matching := set.InCategory(cat)
		if len(matching) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n  # %s\n", cat.Describe())
		for _, r := range matching {
			fmt.Fprintf(&b, "  %-*s %-8s # %s\n", width+1, r.ID+":", r.Default, r.Summary)
		}
	}

	b.WriteString(`
# Per-rule options. Run "goorg explain <rule>" to see what each one accepts.
settings: {}

# Paths goorg must not read, as globs over root-relative paths. "**" matches any
# number of directories, and a pattern with no "/" applies at any depth.
# "**/testdata/**" is always excluded and does not need listing here.
exclude:
  - "**/*.pb.go"
  - "**/*_gen.go"
`)
	return b.String()
}
