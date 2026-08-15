package rule

import "fmt"

// Tier records what a rule needs in order to run.
//
// The split is the central architectural decision of the project: syntax rules
// work on a tree that does not compile, type rules do not. Every rule declares
// its tier from the first commit, even while only Syntax exists, because
// retrofitting the field onto a populated rule set costs far more than carrying
// it unused. See docs/decisions.md D1.
type Tier int

const (
	// Syntax rules need only go/parser. They are cheap and always run.
	Syntax Tier = iota
	// Types rules need go/types, so they run only when the module loads.
	Types
)

// String returns the spelling used in output and documentation.
func (t Tier) String() string {
	switch t {
	case Syntax:
		return "syntax"
	case Types:
		return "types"
	default:
		return fmt.Sprintf("tier(%d)", int(t))
	}
}
