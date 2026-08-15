package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
)

// escapeData escapes a workflow command message body.
var escapeData = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace

// escapeProperty escapes a workflow command property value, which additionally
// may not contain the separators.
var escapeProperty = strings.NewReplacer(
	"%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C",
).Replace

// writeGitHub emits workflow commands in the form
//
//	::error file=path,line=1,col=1,title=goorg rule/id::message
//
// GitHub renders these as inline annotations on the diff. Properties are
// comma-separated and the message runs to end of line, so both need escaping —
// an unescaped newline would silently truncate the annotation.
func writeGitHub(w io.Writer, ds []diag.Diagnostic) error {
	for _, d := range ds {
		level := "warning"
		if d.Severity == diag.Error {
			level = "error"
		}

		props := []string{"file=" + escapeProperty(d.Path)}
		if d.Line > 0 {
			props = append(props, fmt.Sprintf("line=%d", d.Line))
			if d.Col > 0 {
				props = append(props, fmt.Sprintf("col=%d", d.Col))
			}
		}
		props = append(props, "title="+escapeProperty("goorg "+d.RuleID))

		msg := d.Message
		if d.Help != "" {
			msg += "\n\n" + d.Help
		}
		_, err := fmt.Fprintf(w, "::%s %s::%s\n", level, strings.Join(props, ","), escapeData(msg))
		if err != nil {
			return err
		}
	}
	return nil
}
