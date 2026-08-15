package cli

import (
	"fmt"
	"runtime"

	"github.com/Quikcad/goorg/internal/buildinfo"
)

func runVersion(env *Env, args []string) int {
	fs := newFlagSet(env, "version")
	short := fs.Bool("short", false, "print only the version string")
	fs.Usage = func() {
		fmt.Fprint(env.Stderr, "Usage: goorg version [-short]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return ExitError
	}

	if *short {
		fmt.Fprintln(env.Stdout, buildinfo.Version())
		return ExitOK
	}

	set, err := buildRuleSet()
	if err != nil {
		fmt.Fprintf(env.Stderr, "goorg: %v\n", err)
		return ExitError
	}

	fmt.Fprintf(env.Stdout, "goorg %s\n", buildinfo.Version())
	fmt.Fprintf(env.Stdout, "  go:    %s\n", runtime.Version())
	fmt.Fprintf(env.Stdout, "  os:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(env.Stdout, "  rules: %d\n", set.Len())
	return ExitOK
}
