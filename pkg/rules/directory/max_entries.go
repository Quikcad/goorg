package directory

import (
	"fmt"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/glob"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// maxEntriesSettings is the configurable surface of dir/max-entries.
type maxEntriesSettings struct {
	// Limit is the maximum number of immediate children a directory may hold.
	Limit int `yaml:"limit"`
	// Overrides maps a path glob to a different limit. The most specific
	// matching pattern wins.
	Overrides map[string]int `yaml:"overrides"`
}

// limitFor resolves the limit that applies to a directory.
func (s *maxEntriesSettings) limitFor(rel string) int {
	best, bestSpecificity := s.Limit, -1
	for pattern, limit := range s.Overrides {
		if !glob.MatchPath(pattern, rel) {
			continue
		}
		if spec := glob.Specificity(pattern); spec > bestSpecificity {
			best, bestSpecificity = limit, spec
		}
	}
	return best
}

var maxEntries = &rule.Rule{
	ID:       "dir/max-entries",
	Category: rule.Directory,
	Tier:     rule.Syntax,
	Summary:  "a directory holds at most a configured number of entries",
	Default:  diag.Warning,
	Doc: `A directory may contain at most N entries, counting files and immediate
subdirectories together. The count is of immediate children only, not
recursive, so a subdirectory counts as one entry however much it holds.

Rationale: a directory is the unit people scan. Past a couple of dozen entries
nobody reads the listing — they grep and hope, which means new code lands
wherever the last file did rather than where it belongs. The limit is a forcing
function: when a package outgrows it, the fix is to split along the seam that
already exists, and the limit makes you find that seam while it is still
obvious.

To fix: split the directory into purpose-named siblings. Note that the split
must not create a subpackage — dir/max-package-depth forbids that — so the
right move is a sibling package in the same domain, not a nested one.

Moving embedded assets into a subdirectory, as dir/embedded-assets requires,
collapses many files into a single entry and often resolves this on its own.

Configure in .goorg.yaml:

	settings:
	  dir/max-entries:
	    limit: 20
	    overrides:
	      "docs/**": 50`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := maxEntriesSettings{Limit: 20}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, d := range c.Project.Dirs {
			limit := s.limitFor(d.Rel)
			if limit <= 0 || d.Entries <= limit {
				continue
			}
			out = append(out, diag.Diagnostic{
				Position: rule.DirPos(d.Rel),
				Message:  fmt.Sprintf("directory holds %d entries, over the limit of %d", d.Entries, limit),
				Help:     "split it into purpose-named sibling directories, or move assets into a subdirectory",
			})
		}
		return out
	},
}
