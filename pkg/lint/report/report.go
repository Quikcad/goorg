// Package report renders diagnostics in the formats CI and humans need.
//
// Three formats for three consumers: text for a person reading a terminal,
// github for the Actions annotation protocol that puts findings inline on a
// pull request, and json for anything that consumes goorg programmatically.
package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
)

// Format identifies an output renderer.
type Format string

const (
	// Text is the default human-readable format.
	Text Format = "text"
	// GitHub emits GitHub Actions workflow commands, which the runner turns
	// into inline annotations on the pull request.
	GitHub Format = "github"
	// JSON emits a machine-readable document.
	JSON Format = "json"
)

// Options controls rendering.
type Options struct {
	// Color enables ANSI styling in the text format.
	Color bool
	// ShowHelp includes each diagnostic's remediation line in the text format.
	ShowHelp bool
}

// ParseFormat validates a --format value.
func ParseFormat(s string) (Format, error) {
	want := strings.ToLower(strings.TrimSpace(s))
	names := make([]string, 0, len(Formats()))
	for _, f := range Formats() {
		if string(f) == want {
			return f, nil
		}
		names = append(names, string(f))
	}
	return "", fmt.Errorf("unknown format %q (want one of: %s)", s, strings.Join(names, ", "))
}

// Formats lists every supported format, for help text and validation.
func Formats() []Format {
	return []Format{Text, GitHub, JSON}
}

// Write renders diagnostics in the requested format.
func Write(w io.Writer, f Format, ds []diag.Diagnostic, opts Options) error {
	switch f {
	case Text:
		return writeText(w, ds, opts)
	case GitHub:
		return writeGitHub(w, ds)
	case JSON:
		return writeJSON(w, ds)
	default:
		return fmt.Errorf("unknown format %q", f)
	}
}
