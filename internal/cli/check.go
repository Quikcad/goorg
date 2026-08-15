package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/config"
	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/report"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/lint/runner"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// checkFlags is the flag surface of `goorg check`.
type checkFlags struct {
	configPath  string
	root        string
	format      string
	color       string
	failOn      string
	maxWarnings int
	brief       bool
	syntaxOnly  bool
	noWhatIf    bool
}

func (f *checkFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&f.configPath, "config", "", "path to a config file (default: .goorg.yaml discovered at the project root)")
	fs.StringVar(&f.root, "root", "", "project root to check (default: nearest ancestor with go.mod or .goorg.yaml)")
	fs.StringVar(&f.format, "format", "auto", "output format: auto, text, github, json")
	fs.StringVar(&f.color, "color", "auto", "colorize text output: auto, always, never")
	fs.StringVar(&f.failOn, "fail-on", "error", "lowest severity that fails the run: error, warning")
	fs.IntVar(&f.maxWarnings, "max-warnings", -1, "fail if warnings exceed this count (-1 disables the check)")
	fs.BoolVar(&f.brief, "brief", false, "omit the help line under each finding in text output")
	fs.BoolVar(&f.syntaxOnly, "syntax-only", false, "run only syntax-tier rules; skip rules that need type information")
	fs.BoolVar(&f.noWhatIf, "no-what-if", false, "report proposed relocations without checking whether the move would break another rule")
}

// checkRun is everything resolved from flags and configuration before rules run.
type checkRun struct {
	root   string
	proj   *project.Project
	cfg    *config.Config
	set    *rule.Set
	tiers  tierPlan
	only   []string
	format report.Format
	failOn diag.Severity
}

// resolveCheck turns flags and configuration into a runnable plan.
func resolveCheck(env *Env, f *checkFlags, args []string) (*checkRun, int) {
	failOn, err := diag.ParseSeverity(f.failOn)
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: -fail-on: %v\n", err)
		return nil, ExitError
	}
	if failOn == diag.Off {
		fmt.Fprintln(env.Stderr, "goorg: -fail-on must be error or warning")
		return nil, ExitError
	}

	format, err := resolveFormat(env, f.format)
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: -format: %v\n", err)
		return nil, ExitError
	}

	set, err := buildRuleSet()
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return nil, ExitError
	}

	rootArg := f.root
	if rootArg == "" {
		rootArg = firstPathArg(args)
	}
	root, err := resolveRoot(rootArg)
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return nil, ExitError
	}

	cfg, err := loadConfig(root, f.configPath, set)
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return nil, ExitError
	}

	only, err := normalizePaths(root, args)
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return nil, ExitError
	}

	proj, err := project.Load(root, project.Options{Exclude: cfg.Excluder()})
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return nil, ExitError
	}

	return &checkRun{
		root: root, proj: proj, cfg: cfg, set: set,
		tiers:  planTiers(root, cfg, set, f.syntaxOnly),
		only:   only,
		format: format,
		failOn: failOn,
	}, ExitOK
}

// run executes the rules and renders the result.
func (p *checkRun) run(env *Env, f *checkFlags) int {
	res := runner.Run(p.proj, p.cfg, p.set, runner.Options{Tiers: p.tiers.tiers, Typed: p.tiers.program, WhatIf: !f.noWhatIf})
	res.Diagnostics = append(res.Diagnostics, p.tiers.gaps...)
	if len(p.only) > 0 {
		res.Diagnostics = filterPaths(res.Diagnostics, p.only)
	}
	diag.Sort(res.Diagnostics)
	res.Counts = diag.Summarize(res.Diagnostics)

	opts := report.Options{Color: useColor(env, f.color), ShowHelp: !f.brief}
	if err := report.Write(env.Stdout, p.format, res.Diagnostics, opts); err != nil {
		fmt.Fprintf(env.Stderr, "goorg: write report: %v\n", err)
		return ExitError
	}

	switch {
	case p.tiers.failed:
		// A type tier that was wanted and could not run is goorg failing to do
		// its job, not the project failing a check.
		return ExitError
	case res.Counts.Errors > 0:
		return ExitFindings
	case p.failOn == diag.Warning && res.Counts.Warnings > 0:
		return ExitFindings
	case f.maxWarnings >= 0 && res.Counts.Warnings > f.maxWarnings:
		fmt.Fprintf(env.Stderr, "goorg: %s exceeds -max-warnings=%d\n",
			plural(res.Counts.Warnings, "warning"), f.maxWarnings)
		return ExitFindings
	default:
		return ExitOK
	}
}

func runCheck(env *Env, args []string) int {
	fs := newFlagSet(env, "check")
	var f checkFlags
	f.bind(fs)
	fs.Usage = func() {
		fmt.Fprint(env.Stderr, `Usage: goorg check [flags] [paths...]

Checks a Go project against the rules configured in .goorg.yaml.

Paths accept Go's package pattern syntax; "./..." and "." are equivalent
because goorg is always recursive. When paths are given, findings outside
them are suppressed, but the whole project is still loaded — a rule can only
judge a directory in context.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return ExitError
	}

	plan, code := resolveCheck(env, &f, fs.Args())
	if code != ExitOK {
		return code
	}
	return plan.run(env, &f)
}

// filterPaths keeps only diagnostics under one of the requested paths.
func filterPaths(ds []diag.Diagnostic, only []string) []diag.Diagnostic {
	out := ds[:0:0]
	for _, d := range ds {
		for _, p := range only {
			if d.Path == p || strings.HasPrefix(d.Path, p+"/") {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// firstPathArg returns the first positional argument, defaulting to the current
// directory. It only decides where to start looking for the project root.
func firstPathArg(args []string) string {
	if len(args) == 0 {
		return "."
	}
	p := strings.TrimSuffix(strings.TrimSuffix(args[0], "..."), "/")
	if p == "" {
		return "."
	}
	return p
}

// resolveFormat maps the "auto" format onto the environment. Detecting GitHub
// Actions means annotations appear on the pull request without anyone having to
// remember a flag, which is the whole point of the format.
func resolveFormat(env *Env, name string) (report.Format, error) {
	if strings.ToLower(strings.TrimSpace(name)) != "auto" {
		return report.ParseFormat(name)
	}
	if env.getenv("GITHUB_ACTIONS") == "true" {
		return report.GitHub, nil
	}
	return report.Text, nil
}

// useColor decides whether to emit ANSI styling, honoring the NO_COLOR
// convention (https://no-color.org) under --color=auto.
func useColor(env *Env, mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "always", "true", "yes":
		return true
	case "never", "false", "no":
		return false
	default:
		if env.getenv("NO_COLOR") != "" {
			return false
		}
		return env.StdoutIsTerminal
	}
}
