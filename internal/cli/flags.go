package cli

import (
	"flag"
	"strings"
)

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
