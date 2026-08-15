package organization

import "github.com/Quikcad/goorg/pkg/lint/rule"

// Rules returns the org/ family.
//
// Two further rules are specified but need type information and land in
// phase 5: org/consumer-locality and org/global-file-scoped.
func Rules() []*rule.Rule {
	return []*rule.Rule{
		globalsSingletonOnly,
		interfaceOwnFile,
		maxFunctionsPerFile,
		maxPrivateFunctions,
		maxPublicFunctions,
		memberOrder,
		privateFunctionsLast,
		singletonInstanceFunc,
		singletonLayout,
		typeCohesion,
	}
}
