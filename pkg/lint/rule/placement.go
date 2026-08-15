package rule

import "fmt"

// Placement records what a rule's outcome depends on.
//
// It exists for the what-if pass, which has to tell a real objection to moving
// a declaration from a cosmetic one. A budget that the destination file would
// blow is a reason not to move the code; the moved declaration landing in the
// wrong part of its new file is not, because reordering it there is a separate
// and always-available fix.
type Placement int

//goorg:ignore logic/enum-zero-value-unnamed — participating is the deliberate default, not an unset value
const (
	// PlacementFile means the rule's outcome depends on which file a
	// declaration lives in. These rules decide whether a relocation is safe,
	// and it is the zero value so a rule participates unless it opts out.
	PlacementFile Placement = iota
	// PlacementOrder means the outcome depends only on where in a file a
	// declaration sits. These rules are excluded from the what-if comparison.
	PlacementOrder
)

var _ fmt.Stringer = PlacementFile

// String returns the spelling used in documentation.
func (p Placement) String() string {
	switch p {
	case PlacementFile:
		return "file"
	case PlacementOrder:
		return "order"
	default:
		return fmt.Sprintf("placement(%d)", int(p))
	}
}
