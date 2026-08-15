package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
)

// ANSI styles, applied only when Options.Color is set.
//
//goorg:ignore logic/stringly-typed-enum — a palette used together, not a set of alternatives
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

func writeText(w io.Writer, ds []diag.Diagnostic, opts Options) error {
	paint := func(style, s string) string {
		if !opts.Color {
			return s
		}
		return style + s + ansiReset
	}

	for _, d := range ds {
		label, style := "warning", ansiYellow
		if d.Severity == diag.Error {
			label, style = "error", ansiRed
		}
		_, err := fmt.Fprintf(w, "%s: %s: %s %s\n",
			paint(ansiBold, d.Position.String()),
			paint(style, label),
			d.Message,
			paint(ansiCyan, "["+d.RuleID+"]"),
		)
		if err != nil {
			return err
		}
		if opts.ShowHelp && d.Help != "" {
			if _, err := fmt.Fprintf(w, "  %s\n", paint(ansiDim, "help: "+d.Help)); err != nil {
				return err
			}
		}
	}

	counts := diag.Summarize(ds)
	if counts.Total() == 0 {
		_, err := fmt.Fprintln(w, paint(ansiBold, "no findings"))
		return err
	}

	summary := fmt.Sprintf("%s, %s", plural(counts.Errors, "error"), plural(counts.Warnings, "warning"))
	if _, err := fmt.Fprintf(w, "\n%s\n", paint(ansiBold, summary)); err != nil {
		return err
	}
	// Naming the rules involved turns "fix these 40 findings" into "fix or
	// reconsider these 3 rules", which is the decision a team actually makes.
	breakdown := ruleBreakdown(ds)
	if breakdown == "" {
		return nil
	}
	_, err := fmt.Fprintf(w, "%s\n", paint(ansiDim, breakdown))
	return err
}

// ruleBreakdown lists the rules that fired, most frequent first.
func ruleBreakdown(ds []diag.Diagnostic) string {
	counts := map[string]int{}
	for _, d := range ds {
		counts[d.RuleID]++
	}
	if len(counts) == 0 {
		return ""
	}
	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if counts[ids[i]] != counts[ids[j]] {
			return counts[ids[i]] > counts[ids[j]]
		}
		return ids[i] < ids[j]
	})
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%s (%d)", id, counts[id]))
	}
	return "rules: " + strings.Join(parts, ", ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
