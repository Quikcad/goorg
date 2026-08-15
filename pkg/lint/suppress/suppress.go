// Package suppress implements goorg's inline suppression directive.
//
//	//goorg:ignore org/max-private-functions — parser state machine, splitting hurts
//	func lex(s string) []token { ... }
//
// The reason is mandatory. A directive without one is reported rather than
// honoured, because a suppression nobody can evaluate later is worse than the
// finding it hides. See docs/decisions.md D4.
package suppress

import (
	"fmt"
	"go/ast"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// Meta-diagnostic IDs. The goorg/ namespace is reserved for the tool's own
// diagnostics and is not a configurable rule family.
const (
	InvalidRule = "goorg/invalid-suppression"
	StaleRule   = "goorg/stale-suppression"
)

// directivePrefix is the comment text that introduces a suppression.
const directivePrefix = "//goorg:ignore"

// reasonSeparators are stripped from the front of a reason so that the em dash
// in the documented form is optional in practice.
var reasonSeparators = []string{"—", "--", "–", ":", "-"}

// Directive is one parsed suppression comment.
type Directive struct {
	// RuleID is the rule this directive silences.
	RuleID string
	// Reason is the mandatory justification.
	Reason string
	// Path is the root-relative file the directive appears in.
	Path string
	// Line is the line the directive itself is on.
	Line int
	// Start and End bound the lines it covers, inclusive.
	Start int
	End   int

	used bool
}

// Covers reports whether a diagnostic falls inside this directive's range.
func (d *Directive) Covers(x diag.Diagnostic) bool {
	if x.Path != d.Path || x.RuleID != d.RuleID {
		return false
	}
	// A whole-file or whole-directory finding has no line, so it can only be
	// suppressed by a directive that is itself file-scoped.
	if x.Line == 0 {
		return d.Start == 0
	}
	return x.Line >= d.Start && x.Line <= d.End
}

// parse splits a directive comment into its rule ID and reason.
func parse(text, path string, line int) (*Directive, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(text, directivePrefix))
	if rest == "" {
		return nil, fmt.Errorf("suppression names no rule")
	}

	id, reason, _ := strings.Cut(rest, " ")
	reason = strings.TrimSpace(reason)
	for _, sep := range reasonSeparators {
		if trimmed, ok := strings.CutPrefix(reason, sep); ok {
			reason = strings.TrimSpace(trimmed)
			break
		}
	}
	if reason == "" {
		return nil, fmt.Errorf("suppression of %s gives no reason", id)
	}
	return &Directive{RuleID: id, Reason: reason, Path: path, Line: line}, nil
}

// Set holds every directive found in a project.
type Set struct {
	byPath map[string][]*Directive
}

// Scan reads suppression directives from every file, returning the set plus
// diagnostics for directives that are malformed.
func Scan(p *project.Project) (*Set, []diag.Diagnostic) {
	s := &Set{byPath: map[string][]*Directive{}}
	var problems []diag.Diagnostic

	for _, f := range p.Files() {
		for _, c := range directiveComments(f) {
			line := p.Position(c.Pos()).Line
			d, err := parse(strings.TrimSpace(c.Text), f.Rel, line)
			if err != nil {
				problems = append(problems, diag.Diagnostic{
					Position: diag.Position{Path: f.Rel, Line: line},
					RuleID:   InvalidRule,
					Severity: diag.Error,
					Message:  err.Error(),
					Help:     "write //goorg:ignore <rule-id> — <reason>",
				})
				continue
			}
			d.Start, d.End = coverage(p, f, line)
			s.byPath[f.Rel] = append(s.byPath[f.Rel], d)
		}
	}
	diag.Sort(problems)
	return s, problems
}

// Apply removes suppressed diagnostics, marking the directives that fired.
func (s *Set) Apply(ds []diag.Diagnostic) []diag.Diagnostic {
	out := ds[:0:0]
	for _, d := range ds {
		if s.suppress(d) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// Stale returns a diagnostic for every directive that suppressed nothing.
//
// A directive left behind after the code it excused was fixed is a lie about
// the code, and the only way to find one is to notice it never fired.
func (s *Set) Stale() []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, directives := range s.byPath {
		for _, d := range directives {
			if d.used {
				continue
			}
			out = append(out, diag.Diagnostic{
				Position: diag.Position{Path: d.Path, Line: d.Line},
				RuleID:   StaleRule,
				Severity: diag.Warning,
				Message:  fmt.Sprintf("suppression of %s is unused", d.RuleID),
				Help:     "the finding it silenced is gone; delete the directive",
			})
		}
	}
	diag.Sort(out)
	return out
}

// Len returns the number of directives in the set.
func (s *Set) Len() int {
	n := 0
	for _, directives := range s.byPath {
		n += len(directives)
	}
	return n
}

func (s *Set) suppress(x diag.Diagnostic) bool {
	// A suppression cannot silence goorg's own meta-diagnostics: a broken
	// directive that suppresses the report of its own brokenness would be
	// unfixable.
	if strings.HasPrefix(x.RuleID, "goorg/") {
		return false
	}
	for _, d := range s.byPath[x.Path] {
		if d.Covers(x) {
			d.used = true
			return true
		}
	}
	return false
}

// directiveComments returns the suppression comments in a file.
func directiveComments(f *project.File) []*ast.Comment {
	var out []*ast.Comment
	for _, group := range f.Syntax.Comments {
		for _, c := range group.List {
			if strings.HasPrefix(strings.TrimSpace(c.Text), directivePrefix) {
				out = append(out, c)
			}
		}
	}
	return out
}

// coverage decides which lines a directive protects.
//
// A trailing directive covers only its own line. A standalone one covers the
// declaration or statement immediately below it, which is found by taking the
// widest AST node starting on the next line — the widest, because the intent is
// to cover the whole declaration rather than the identifier that shares its
// start position.
func coverage(p *project.Project, f *project.File, line int) (start, end int) {
	if isTrailing(f, line) {
		return line, line
	}

	target := line + 1
	widest := 0
	ast.Inspect(f.Syntax, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if p.Position(n.Pos()).Line != target {
			return true
		}
		if e := p.Position(n.End()).Line; e > widest {
			widest = e
		}
		return true
	})
	if widest == 0 {
		// Nothing starts on the next line; the directive covers only it, and
		// will be reported as stale if that line yields no finding.
		return target, target
	}
	return target, widest
}

// isTrailing reports whether code precedes the directive on its own line.
func isTrailing(f *project.File, line int) bool {
	text := f.LineText(line)
	idx := strings.Index(text, directivePrefix)
	if idx < 0 {
		return false
	}
	return strings.TrimSpace(text[:idx]) != ""
}
