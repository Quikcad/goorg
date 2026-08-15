// Package cli implements goorg's command-line interface.
//
// Every command takes already-parsed arguments and explicit output writers, and
// returns an exit code rather than calling os.Exit. That keeps the whole CLI
// testable in-process: a test runs a command against a temporary directory and
// asserts on both streams and the code.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Exit codes. These are contractual: CI configurations branch on them, so they
// may not be reassigned or collapsed.
//
//goorg:ignore logic/iota-candidate — the literal values are the published contract, not an ordering
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

func looksLikePath(s string) bool {
	return s == "." || strings.ContainsAny(s, "/\\") || strings.HasSuffix(s, "...")
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
