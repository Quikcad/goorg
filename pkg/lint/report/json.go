package report

import (
	"encoding/json"
	"io"

	"github.com/Quikcad/goorg/pkg/lint/diag"
)

// jsonDocument is the wire shape. It is declared separately from the internal
// types so the JSON output is a stable contract rather than a mirror of
// whatever diag.Diagnostic happens to hold this release.
type jsonDocument struct {
	Version     int              `json:"version"`
	Diagnostics []jsonDiagnostic `json:"diagnostics"`
	Summary     jsonSummary      `json:"summary"`
}

type jsonDiagnostic struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Col      int    `json:"col,omitempty"`
	Message  string `json:"message"`
	Help     string `json:"help,omitempty"`
}

type jsonSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

func writeJSON(w io.Writer, ds []diag.Diagnostic) error {
	doc := jsonDocument{Version: 1, Diagnostics: make([]jsonDiagnostic, 0, len(ds))}
	for _, d := range ds {
		doc.Diagnostics = append(doc.Diagnostics, jsonDiagnostic{
			Rule:     d.RuleID,
			Severity: d.Severity.String(),
			Path:     d.Path,
			Line:     d.Line,
			Col:      d.Col,
			Message:  d.Message,
			Help:     d.Help,
		})
	}
	counts := diag.Summarize(ds)
	doc.Summary = jsonSummary{Errors: counts.Errors, Warnings: counts.Warnings}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
