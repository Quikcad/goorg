package organization

import "github.com/Quikcad/goorg/pkg/lint/rule"

// Rules returns the org/ family: ten syntax-tier rules and three type-tier ones.
func Rules() []*rule.Rule {
	return []*rule.Rule{
		consumerLocality,
		globalFileScoped,
		globalsSingletonOnly,
		interfaceMethodOrder,
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
