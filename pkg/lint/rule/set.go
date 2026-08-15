package rule

import (
	"fmt"
	"regexp"
	"sort"
)

// idPattern constrains rule IDs to "<category>/<kebab-case>". It is compiled
// once at package level, which org/globals-singleton-only exempts precisely
// because Go offers no other spelling for it.
var idPattern = regexp.MustCompile(`^(dir|org|logic|pat)/[a-z0-9]+(-[a-z0-9]+)*$`)

// Set is an immutable collection of rules, keyed by ID.
type Set struct {
	ordered []*Rule
	byID    map[string]*Rule
}

// NewSet validates and collects rules from one or more families.
//
// It returns an error rather than panicking on a malformed or duplicate ID:
// both are programmer errors, but a linter that panics in someone's CI is worse
// than one that explains itself and exits 2.
func NewSet(groups ...[]*Rule) (*Set, error) {
	s := &Set{byID: map[string]*Rule{}}
	for _, group := range groups {
		for _, r := range group {
			if err := s.add(r); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(s.ordered, func(i, j int) bool { return s.ordered[i].ID < s.ordered[j].ID })
	return s, nil
}

// All returns every rule, sorted by ID.
func (s *Set) All() []*Rule {
	return s.ordered
}

// Get returns the rule with the given ID, or nil.
func (s *Set) Get(id string) *Rule {
	return s.byID[id]
}

// IDs returns every rule ID, sorted.
func (s *Set) IDs() []string {
	out := make([]string, 0, len(s.ordered))
	for _, r := range s.ordered {
		out = append(out, r.ID)
	}
	return out
}

// InCategory returns the rules in one category, sorted by ID.
func (s *Set) InCategory(c Category) []*Rule {
	var out []*Rule
	for _, r := range s.ordered {
		if r.Category == c {
			out = append(out, r)
		}
	}
	return out
}

// Len returns the number of rules in the set.
func (s *Set) Len() int {
	return len(s.ordered)
}

func (s *Set) add(r *Rule) error {
	switch {
	case r == nil:
		return fmt.Errorf("nil rule")
	case !idPattern.MatchString(r.ID):
		return fmt.Errorf("malformed rule ID %q (want <dir|org|logic|pat>/<kebab-case>)", r.ID)
	case r.ID[:len(r.Category)+1] != string(r.Category)+"/":
		return fmt.Errorf("rule %q has category %q, which does not match its ID prefix", r.ID, r.Category)
	case r.Check == nil:
		return fmt.Errorf("rule %q has no Check function", r.ID)
	}
	if _, dup := s.byID[r.ID]; dup {
		return fmt.Errorf("rule %q registered twice", r.ID)
	}
	s.byID[r.ID] = r
	s.ordered = append(s.ordered, r)
	return nil
}
