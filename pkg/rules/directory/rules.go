package directory

import "github.com/Quikcad/goorg/pkg/lint/rule"

// Rules returns the dir/ family.
//
// Families are composed explicitly by internal/cli rather than registering
// themselves from init, so goorg carries no package-level mutable state and the
// active rule set is visible in one place.
func Rules() []*rule.Rule {
	return []*rule.Rule{
		domainLayout,
		domainNoGoFiles,
		embeddedAssets,
		maxEntries,
		maxPackageDepth,
		topLevelLayout,
	}
}
