package organization

import "github.com/Quikcad/goorg/pkg/lint/rule"

// Rules returns the org/ family: ten syntax-tier rules and two type-tier ones.
func Rules() []*rule.Rule {
	return []*rule.Rule{
		consumerLocality,
		globalFileScoped,
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
