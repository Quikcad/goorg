package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/Quikcad/goorg/pkg/lint/config"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

func runRules(env *Env, args []string) int {
	fs := newFlagSet(env, "rules")
	configPath := fs.String("config", "", "path to a config file (default: .goorg.yaml discovered at the project root)")
	root := fs.String("root", "", "project root whose config decides the severities shown")
	category := fs.String("category", "", "show only one category: dir, org, logic, or pat")
	fs.Usage = func() {
		fmt.Fprint(env.Stderr, `Usage: goorg rules [flags]

Lists every rule with the severity it would run at in this project, so you can
see the effect of a config change without running a check.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return ExitError
	}

	set, cfg, code := loadFor(env, *root, *configPath)
	if code != ExitOK {
		return code
	}

	want := strings.ToLower(strings.TrimSpace(*category))
	if want != "" && !validCategory(want) {
		fmt.Fprintf(env.Stderr, "goorg: unknown category %q (want dir, org, logic, or pat)\n", *category)
		return ExitError
	}

	if set.Len() == 0 {
		fmt.Fprintln(env.Stdout, "no rules are registered in this build")
		return ExitOK
	}

	overridden := 0
	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	for _, cat := range rule.Categories() {
		if want != "" && string(cat) != want {
			continue
		}
		matching := set.InCategory(cat)
		if len(matching) == 0 {
			continue
		}
		fmt.Fprintf(tw, "\n%s\n", cat.Describe())
		for _, r := range matching {
			sev := cfg.Severity(r)
			marker := " "
			if sev != r.Default {
				// Flag rules the project has moved off their shipped default,
				// so a surprising result is traceable to the config.
				marker = "*"
				overridden++
			}
			fmt.Fprintf(tw, "  %s%s\t%s\t%s\t%s\n", marker, r.ID, sev, r.Tier, r.Summary)
		}
	}
	fmt.Fprint(tw, "\n")
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return ExitError
	}

	if overridden > 0 {
		fmt.Fprintf(env.Stdout, "* %s overridden by %s\n",
			plural(overridden, "rule"), configLabel(cfg))
	}
	fmt.Fprintln(env.Stdout, "run `goorg explain <rule>` for the rationale and how to fix a rule")
	return ExitOK
}

func runExplain(env *Env, args []string) int {
	fs := newFlagSet(env, "explain")
	configPath := fs.String("config", "", "path to a config file (default: .goorg.yaml discovered at the project root)")
	root := fs.String("root", "", "project root whose config decides the severity shown")
	fs.Usage = func() {
		fmt.Fprint(env.Stderr, `Usage: goorg explain <rule-id>

Prints a rule's rationale, examples, and configuration options.

  goorg explain dir/domain-layout

Flags:
`)
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return ExitError
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "goorg: explain takes exactly one rule ID")
		fs.Usage()
		return ExitError
	}

	set, cfg, code := loadFor(env, *root, *configPath)
	if code != ExitOK {
		return code
	}

	id := fs.Arg(0)
	r := set.Get(id)
	if r == nil {
		fmt.Fprintf(env.Stderr, "goorg: no rule %q\n", id)
		if near := nearestRules(set, id); len(near) > 0 {
			fmt.Fprintf(env.Stderr, "did you mean: %s\n", strings.Join(near, ", "))
		}
		return ExitError
	}

	fmt.Fprintf(env.Stdout, "%s\n%s\n\n", r.ID, strings.Repeat("=", len(r.ID)))
	fmt.Fprintf(env.Stdout, "category:  %s\n", r.Category.Describe())
	fmt.Fprintf(env.Stdout, "tier:      %s\n", r.Tier)
	fmt.Fprintf(env.Stdout, "default:   %s\n", r.Default)
	fmt.Fprintf(env.Stdout, "in effect: %s (%s)\n\n", cfg.Severity(r), configLabel(cfg))
	fmt.Fprintf(env.Stdout, "%s\n\n%s\n", r.Summary, strings.TrimSpace(r.Doc))
	return ExitOK
}

// loadFor builds the rule set and loads the config for the commands that only
// report on them. It returns ExitOK on success.
func loadFor(env *Env, rootFlag, configPath string) (*rule.Set, *config.Config, int) {
	set, err := buildRuleSet()
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return nil, nil, ExitError
	}
	dir := rootFlag
	if dir == "" {
		dir = "."
	}
	root, err := resolveRoot(dir)
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return nil, nil, ExitError
	}
	cfg, err := loadConfig(root, configPath, set)
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return nil, nil, ExitError
	}
	return set, cfg, ExitOK
}

func configLabel(cfg *config.Config) string {
	if cfg.Path == "" {
		return "shipped defaults (no config file found)"
	}
	return cfg.Path
}

func validCategory(name string) bool {
	for _, c := range rule.Categories() {
		if string(c) == name {
			return true
		}
	}
	return false
}

// nearestRules suggests rules sharing a substring with a mistyped ID.
func nearestRules(set *rule.Set, id string) []string {
	needle := strings.ToLower(id)
	if i := strings.Index(needle, "/"); i >= 0 {
		needle = needle[i+1:]
	}
	if needle == "" {
		return nil
	}
	var out []string
	for _, candidate := range set.IDs() {
		if strings.Contains(candidate, needle) {
			out = append(out, candidate)
		}
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}
