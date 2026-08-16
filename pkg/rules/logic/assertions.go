package logic

import "github.com/Quikcad/goorg/pkg/source/typed"

// assertion records a compile-time `var _ I = T(...)` pairing, which is what
// logic/interface-registry looks for before reporting a missing one.
type assertion struct {
	iface string
	named string
}

// assertionsIn collects the `var _ I = T(...)` pairings a package declares.
//
// The scan itself lives in pkg/source/typed so that this rule and
// org/interface-method-order cannot disagree about what an assertion is —
// adding the one this rule demands has to enable the other.
func assertionsIn(pkg *typed.Package) map[assertion]bool {
	out := map[assertion]bool{}
	for _, a := range typed.Assertions(pkg) {
		out[assertion{iface: qualifiedName(a.Interface), named: a.Concrete.Obj().Name()}] = true
	}
	return out
}
