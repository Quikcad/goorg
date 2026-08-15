package pattern

import (
	"fmt"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// Rules returns the pat/ family.
func Rules() []*rule.Rule {
	return []*rule.Rule{
		expandStructDefinition,
		factoryNaming,
	}
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
