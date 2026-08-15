// Package cli implements goorg's command-line interface.
//
// Every command takes already-parsed arguments and explicit output writers, and
// returns an exit code rather than calling os.Exit. That keeps the whole CLI
// testable in-process: a test runs a command against a temporary directory and
// asserts on both streams and the code.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/config"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// Exit codes. These are contractual: CI configurations branch on them, so they
// may not be reassigned or collapsed.
const (
	// ExitOK means no findings at or above the failure threshold.
	ExitOK = 0
	// ExitFindings means the check ran and found violations.
	ExitFindings = 1
	// ExitError means goorg could not complete the run — bad flags, bad
	// config, unreadable tree. Distinct from ExitFindings so CI can tell "your
	// code has problems" from "the linter is broken".
	ExitError = 2
)

// Env carries the process environment a command needs, so tests can supply
// their own.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	// Getenv reads an environment variable. It may be nil, in which case
	// os.Getenv is used.
	Getenv func(string) string
	// StdoutIsTerminal controls whether color is enabled under --color=auto.
	StdoutIsTerminal bool
}

// Main is the entry point. It returns an exit code instead of exiting so that
// callers — including tests — stay in control.
func Main(env *Env, args []string) int {
	if env.Stdout == nil {
		env.Stdout = io.Discard
	}
	if env.Stderr == nil {
		env.Stderr = io.Discard
	}

	// A bare `goorg` checks the current tree. That is the overwhelmingly
	// common invocation and should not require a subcommand.
	if len(args) == 0 {
		return runCheck(env, nil)
	}

	switch args[0] {
	case "-h", "--help", "help":
		usage(env.Stdout)
		return ExitOK
	case "-v", "--version":
		return runVersion(env, nil)
	}

	for _, cmd := range commands() {
		if cmd.name == args[0] {
			return cmd.run(env, args[1:])
		}
	}

	// An unrecognized first argument that looks like a path is almost
	// certainly `goorg ./...`, so treat it as a check rather than an error.
	if !strings.HasPrefix(args[0], "-") && looksLikePath(args[0]) {
		return runCheck(env, args)
	}

	fmt.Fprintf(env.Stderr, "goorg: unknown command %q\n\n", args[0])
	usage(env.Stderr)
	return ExitError
}

func (e *Env) getenv(key string) string {
	if e.Getenv == nil {
		return os.Getenv(key)
	}
	return e.Getenv(key)
}

// command is one subcommand.
type command struct {
	name    string
	summary string
	run     func(env *Env, args []string) int
}

func commands() []*command {
	return []*command{
		{"check", "check a project against the configured rules", runCheck},
		{"rules", "list every rule and its configured severity", runRules},
		{"explain", "print the full documentation for a rule", runExplain},
		{"init", "write a starter .goorg.yaml", runInit},
		{"version", "print the goorg version", runVersion},
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `goorg — enforce Go project structure and house style in CI

Usage:
  goorg [command] [flags] [paths...]

Commands:
`)
	for _, cmd := range commands() {
		fmt.Fprintf(w, "  %-9s %s\n", cmd.name, cmd.summary)
	}
	fmt.Fprint(w, `
Running goorg with no command checks the current directory.

Suppress a finding in source with a reason:
  //goorg:ignore <rule-id> — <why>

Exit codes:
  0  no findings
  1  findings reported
  2  goorg could not run (bad flags, bad config, unreadable tree)

Run "goorg <command> -h" for the flags a command accepts.
`)
}

// newFlagSet builds a flag set that reports errors through the command's own
// stderr and never calls os.Exit.
func newFlagSet(env *Env, name string) *flag.FlagSet {
	fs := flag.NewFlagSet("goorg "+name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	return fs
}

// parseFlags parses args, accepting flags in any position.
//
// The standard flag package stops at the first non-flag argument, so
// `goorg check ./... -format=json` would silently treat -format as a path and
// run with the default format. A linter that quietly ignores the flag you gave
// it is worse than one that rejects it, so arguments are permuted into
// flags-first order before parsing.
func parseFlags(fs *flag.FlagSet, args []string) error {
	return fs.Parse(permuteArgs(fs, args))
}

// permuteArgs moves every flag ahead of every operand, preserving relative
// order within each group. Everything after a bare "--" is an operand.
func permuteArgs(fs *flag.FlagSet, args []string) []string {
	// A boolean flag never consumes the following argument, so its value must
	// not be swallowed as one.
	isBool := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			isBool[f.Name] = true
		}
	})

	var flags, operands []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			operands = append(operands, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			operands = append(operands, arg)
			continue
		}

		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			continue // -name=value carries its own value
		}
		if !isBool[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, operands...)
}

// resolveRoot finds the project root: the nearest ancestor of dir holding a
// go.mod or a goorg config. Falling back to dir itself means goorg still works
// in a directory that is not a module, which matters when checking one
// subdirectory of a monorepo.
func resolveRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}
	for cur := abs; ; {
		if hasProjectMarker(cur) {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		cur = parent
	}
}

func hasProjectMarker(dir string) bool {
	names := append([]string{"go.mod"}, config.FileNames...)
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// loadConfig resolves the configuration for a root, honoring an explicit
// --config path when one is given.
func loadConfig(root, explicit string, set *rule.Set) (*config.Config, error) {
	if explicit != "" {
		return config.Load(explicit, set)
	}
	found, err := config.Discover(root)
	if err != nil {
		return nil, err
	}
	if found == "" {
		return config.Default(), nil
	}
	return config.Load(found, set)
}

// normalizePaths turns Go-style package patterns into directories relative to
// root. goorg is always recursive, so `./...` and `.` mean the same thing;
// accepting both is about matching muscle memory, not semantics.
func normalizePaths(root string, args []string) ([]string, error) {
	var out []string
	for _, arg := range args {
		p := strings.TrimSuffix(strings.TrimSuffix(filepath.ToSlash(arg), "..."), "/")
		if p == "" || p == "." {
			// The whole tree: no filtering needed, so drop every other path.
			return nil, nil
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", arg, err)
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, fmt.Errorf("resolve %s against project root %s: %w", arg, root, err)
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			return nil, fmt.Errorf("path %s is outside the project root %s", arg, root)
		}
		if rel == "." {
			return nil, nil
		}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}

func looksLikePath(s string) bool {
	return s == "." || strings.ContainsAny(s, "/\\") || strings.HasSuffix(s, "...")
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
