package diag

import (
	"fmt"
	"strings"
)

// Severity controls whether a rule runs and whether its findings fail a build.
// Rules carry a default; configuration can raise or lower it.
type Severity int

const (
	// Off suppresses the rule entirely. It is not run at all.
	Off Severity = iota
	// Warning reports the finding without failing the build.
	Warning
	// Error reports the finding and fails the build.
	Error
)

// String returns the canonical spelling used in configuration and output.
func (s Severity) String() string {
	switch s {
	case Off:
		return "off"
	case Warning:
		return "warning"
	case Error:
		return "error"
	default:
		return fmt.Sprintf("severity(%d)", int(s))
	}
}

// ParseSeverity accepts the spellings permitted in .goorg.yaml. It is lenient
// about the obvious aliases so that a config file reads naturally, but it
// rejects anything it does not recognise rather than guessing — a severity
// silently defaulting to Off is a rule that silently stops running.
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off", "false", "none", "ignore":
		return Off, nil
	case "warn", "warning":
		return Warning, nil
	case "error", "err", "true", "on":
		return Error, nil
	default:
		return Off, fmt.Errorf("unknown severity %q (want one of: off, warning, error)", s)
	}
}
