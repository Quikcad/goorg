package pattern

import "github.com/Quikcad/goorg/pkg/lint/rule"

// Rules returns the pat/ family.
func Rules() []*rule.Rule {
	return []*rule.Rule{
		expandStructDefinition,
		factoryNaming,
	}
}
