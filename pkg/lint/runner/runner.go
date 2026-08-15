// Package runner executes a rule set against a loaded project.
//
// It is the only place that knows how a rule's declared default, the project's
// configuration, and a rule's raw findings combine into the final diagnostic
// list. Keeping that in one place is what lets rules stay ignorant of
// configuration.
package runner

import (
	"github.com/Quikcad/goorg/pkg/lint/config"
	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/lint/suppress"
	"github.com/Quikcad/goorg/pkg/lint/whatif"
	"github.com/Quikcad/goorg/pkg/source/project"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

// Options controls a run.
type Options struct {
	// Tiers are the rule tiers available. A rule whose tier is absent is
	// counted as deferred rather than silently passing.
	Tiers map[rule.Tier]bool
	// Typed is the type-checked program, required when Tiers permits
	// rule.Types. A Types rule is never invoked without it.
	Typed *typed.Program
	// WhatIf enables the relocation pass. With it off, a rule's proposals are
	// reported as findings unexamined, which is the narrow form phase 5
	// shipped.
	WhatIf bool
}

// Result is the outcome of a run.
type Result struct {
	// Diagnostics are sorted by position and rule ID.
	Diagnostics []diag.Diagnostic
	// Counts summarizes Diagnostics by severity.
	Counts diag.Counts
	// Ran is the number of rules that were enabled and executed.
	Ran int
	// Skipped is the number of rules turned off by configuration.
	Skipped int
	// Deferred is the number of enabled rules held back because they need a
	// tier that is not available in this run.
	Deferred int
	// Suppressed is the number of findings silenced by inline directives.
	Suppressed int
	// Relocations counts proposed moves the what-if pass rejected because they
	// would have made something else worse.
	Relocations int
}

// Failed reports whether the run found anything at error severity.
func (r Result) Failed() bool {
	return r.Counts.Errors > 0
}

// Run executes every enabled rule and returns the combined diagnostics.
//
// Parse errors from loading are included unconditionally: a file goorg could
// not read is a gap in coverage, and passing silently on it would make the tool
// untrustworthy in exactly the situation where it matters.
func Run(p *project.Project, cfg *config.Config, set *rule.Set, opts Options) Result {
	res := Result{Diagnostics: append([]diag.Diagnostic(nil), p.ParseErrors...)}

	directives, invalid := suppress.Scan(p)
	res.Diagnostics = append(res.Diagnostics, invalid...)

	for _, r := range set.All() {
		severity := cfg.Severity(r)
		if severity == diag.Off {
			res.Skipped++
			continue
		}
		if opts.Tiers != nil && !opts.Tiers[r.Tier] {
			res.Deferred++
			continue
		}
		// A Types rule without a type-checked program would silently check
		// nothing, which is the failure mode D1 exists to prevent.
		if r.Tier == rule.Types && opts.Typed == nil {
			res.Deferred++
			continue
		}
		res.Ran++

		ctx := contextFor(r, p, opts.Typed, cfg.DecoderFor(r.ID))
		if opts.WhatIf {
			ctx.EnableWhatIf()
		}
		found := r.Check(ctx)
		found = append(found, res.judgeProposals(p, cfg, set, ctx, opts)...)
		for _, d := range found {
			// Rules report what is wrong and where; the runner is what decides
			// whether that is an error or a warning.
			d.RuleID = r.ID
			d.Severity = severity
			res.Diagnostics = append(res.Diagnostics, d)
		}
	}

	before := len(res.Diagnostics)
	res.Diagnostics = directives.Apply(res.Diagnostics)
	res.Suppressed = before - len(res.Diagnostics)
	res.Diagnostics = append(res.Diagnostics, directives.Stale()...)

	diag.Sort(res.Diagnostics)
	res.Counts = diag.Summarize(res.Diagnostics)
	return res
}

// judgeProposals turns a rule's proposed relocations into findings, keeping
// only the moves that would not make something else worse.
func (r *Result) judgeProposals(p *project.Project, cfg *config.Config, set *rule.Set, ctx *rule.Context, opts Options) []diag.Diagnostic {
	proposals := ctx.Proposals()
	if len(proposals) == 0 {
		return nil
	}
	if !opts.WhatIf {
		// Without the pass, a proposal is reported as the rule intended it.
		out := make([]diag.Diagnostic, 0, len(proposals))
		for _, proposal := range proposals {
			out = append(out, proposal.Finding)
		}
		return out
	}

	judge := &whatif.Judge{
		Rules:    set.All(),
		Severity: cfg.Severity,
		Decoder:  cfg.DecoderFor,
	}
	result := judge.Evaluate(p, proposals)
	r.Relocations += result.Rejected + result.Cyclic
	return result.Accepted
}

// contextFor builds the context a rule's tier requires.
func contextFor(r *rule.Rule, p *project.Project, t *typed.Program, decode func(any) error) *rule.Context {
	if r.Tier == rule.Types {
		return rule.NewTypedContext(p, t, decode)
	}
	return rule.NewContext(p, decode)
}
