package rule

import "github.com/Quikcad/goorg/pkg/lint/diag"

// Relocation is a finding a rule will only stand behind if moving the
// declaration would not break something else.
//
// A rule emits one instead of a diagnostic when its advice is "put this
// somewhere else". The engine decides whether the move is safe by making it and
// looking, rather than by the rule guessing — see pkg/lint/whatif.
type Relocation struct {
	// Name is the declaration being moved, for the message.
	Name string
	// From and To are root-relative file paths.
	From string
	To   string
	// Line is where the declaration starts in From, which is how the engine
	// finds it again.
	Line int
	// Finding is emitted if and only if the move turns out to be safe.
	Finding diag.Diagnostic
}
