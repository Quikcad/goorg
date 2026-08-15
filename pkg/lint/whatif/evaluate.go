package whatif

import (
	"sort"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// Judge decides which proposed relocations are safe.
type Judge struct {
	// Rules are re-run against the mutated project. Only the ones whose
	// outcome depends on file membership take part; see rule.Placement.
	Rules []*rule.Rule
	// Severity resolves a rule's configured severity, so a rule the project
	// turned off cannot veto a move.
	Severity func(*rule.Rule) diag.Severity
	// Decoder supplies a rule's settings, so the comparison uses the project's
	// real limits rather than the shipped defaults.
	Decoder func(id string) func(any) error
}

// Evaluate applies each proposal in turn and keeps the ones that cost nothing.
//
// Each is judged against the original project, not against the accumulated
// result of earlier ones. Two independent moves cannot then interact through
// the order they happen to be evaluated in, which would make the output depend
// on map iteration.
func (j *Judge) Evaluate(p *project.Project, proposals []rule.Relocation) Result {
	proposals = sortProposals(proposals)
	kept, cyclic := dropCycles(proposals)

	var out Result
	out.Cyclic = cyclic
	baseline := j.violations(p)

	for _, proposal := range kept {
		mutated, err := Apply(p, proposal)
		if err != nil {
			// A move goorg cannot even model is not one it should recommend.
			out.Rejected++
			continue
		}
		if j.violations(mutated) > baseline {
			out.Rejected++
			continue
		}
		out.Accepted = append(out.Accepted, proposal.Finding)
	}
	diag.Sort(out.Accepted)
	return out
}

// violations counts the findings of every participating rule.
func (j *Judge) violations(p *project.Project) int {
	count := 0
	for _, r := range j.Rules {
		if !j.participates(r) {
			continue
		}
		ctx := rule.NewContext(p, j.decoderFor(r.ID))
		count += len(r.Check(ctx))
	}
	return count
}

// participates reports whether a rule's opinion bears on a relocation.
func (j *Judge) participates(r *rule.Rule) bool {
	switch {
	case r.Tier != rule.Syntax:
		// A type-tier rule would need the module reloaded and type-checked for
		// every candidate move, which costs more than the answer is worth.
		return false
	case r.Placement != rule.PlacementFile:
		// Where the declaration lands inside its new file is a separate and
		// always-available fix, so it must not veto the move.
		return false
	case j.Severity != nil && j.Severity(r) == diag.Off:
		// A rule the project switched off has no standing.
		return false
	default:
		return true
	}
}

func (j *Judge) decoderFor(id string) func(any) error {
	if j.Decoder == nil {
		return nil
	}
	return j.Decoder(id)
}

// Result is the outcome of evaluating a set of proposals.
type Result struct {
	// Accepted are the findings whose moves proved safe.
	Accepted []diag.Diagnostic
	// Rejected counts the proposals a move would have made things worse for.
	Rejected int
	// Cyclic counts the proposals dropped because two files each wanted to
	// send a declaration to the other.
	Cyclic int
}

// dropCycles removes proposals that would move declarations in both directions
// between the same pair of files.
//
// Each such move creates the other's finding, so acting on either produces the
// one goorg just reported — advice that walks the code in a circle. Neither is
// reported, and the count is surfaced so the silence is visible.
func dropCycles(proposals []rule.Relocation) ([]rule.Relocation, int) {
	edges := map[[2]string]bool{}
	for _, p := range proposals {
		edges[[2]string{p.From, p.To}] = true
	}

	var kept []rule.Relocation
	dropped := 0
	for _, p := range proposals {
		if edges[[2]string{p.To, p.From}] {
			dropped++
			continue
		}
		kept = append(kept, p)
	}
	return kept, dropped
}

// sortProposals gives the evaluation a deterministic order, so a run over an
// unchanged tree always reports the same thing.
func sortProposals(proposals []rule.Relocation) []rule.Relocation {
	out := append([]rule.Relocation(nil), proposals...)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.From != b.From:
			return a.From < b.From
		case a.Line != b.Line:
			return a.Line < b.Line
		default:
			return a.Name < b.Name
		}
	})
	return out
}
