// Command goorg checks that a Go project follows the house standard for
// directory layout, file organization, code shape, and naming.
//
// It is built for CI: it reports findings, sets a meaningful exit code, and
// does nothing else. It never rewrites source. Run `goorg -h` for usage.
package main

import (
	"os"

	"github.com/Quikcad/goorg/internal/cli"
)

func main() {
	env := &cli.Env{
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		StdoutIsTerminal: isTerminal(os.Stdout),
	}
	os.Exit(cli.Main(env, os.Args[1:]))
}

// isTerminal reports whether f is attached to a character device, which decides
// whether colorized output would be read by a human or captured into a log.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
