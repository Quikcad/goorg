package organization

import (
	"fmt"
	"go/types"
	"sort"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

// interfaceMethodOrderSettings is the configurable surface of the rule.
type interfaceMethodOrderSettings struct {
	// MinMethods is how many methods an interface needs before its order says
	// anything. One method imposes no order at all.
	MinMethods int `yaml:"min_methods"`
	// IncludeImplicit also checks interfaces a type satisfies without saying
	// so. Off by default: structural satisfaction is often accidental, and a
	// type that happens to fit an unrelated interface owes it no ordering.
	IncludeImplicit bool `yaml:"include_implicit"`
	// Contiguous additionally requires the interface's methods to be declared
	// as an uninterrupted run, with the type's other methods before or after.
	Contiguous bool `yaml:"contiguous"`
}

// methodMatch pairs a type with an interface it is checked against.
type methodMatch struct {
	concrete *types.Named
	iface    *types.Named
	order    []string
}

var interfaceMethodOrder = &rule.Rule{
	ID:        "org/interface-method-order",
	Category:  rule.Organization,
	Tier:      rule.Types,
	Placement: rule.PlacementOrder,
	Summary:   "methods are declared in the order the interface declares them",
	Default:   diag.Warning,
	Doc: `A type that implements an interface declares those methods in the order
the interface declares them.

	type Reader interface {          the interface sets the order
		Read(p []byte) (int, error)
		Close() error
	}

	var _ Reader = (*File)(nil)

	func (f *File) Read(p []byte) (int, error)    OK — Read then Close
	func (f *File) Close() error

	func (f *File) Close() error                  violation — Close before Read
	func (f *File) Read(p []byte) (int, error)

Rationale: an interface is a contract read as a list, and every implementation
is a copy of that list written out longhand. When the copies are in different
orders, the reader comparing an implementation against the contract — or two
implementations against each other — has to search rather than scan, and does
it once per method. The cost is small each time and paid on every reading, by
whoever is least familiar with the code.

The order also carries meaning the names do not. An interface usually lists its
methods in the sequence a caller uses them: open, read, close. An
implementation that scrambles that ordering discards the one piece of
documentation the interface provided for free.

To fix: reorder the methods to match the interface. Methods the interface does
not mention may sit anywhere.

Only interfaces declared in the module are checked, because the order an
interface was written in is not recorded in its type — it has to be read from
the source, and the source of a dependency is not loaded. Only interfaces with
at least two methods constrain anything.

By default the rule acts on interfaces a type is *declared* to implement, with
a compile-time assertion:

	var _ Reader = (*File)(nil)

which is the same assertion logic/interface-registry asks for. Structural
satisfaction alone is often accidental, so include_implicit is off; turning it
on means a type that happens to fit an unrelated interface owes it an ordering
too.

A type asserted against two interfaces whose orders disagree cannot satisfy
both, and will be reported for one of them. That is a real conflict in the
design rather than a false positive: the two contracts disagree about the
sequence, and something has to give.

Configure in .goorg.yaml:

	settings:
	  org/interface-method-order:
	    min_methods: 2
	    include_implicit: false
	    contiguous: false`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := interfaceMethodOrderSettings{MinMethods: 2}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.MinMethods < 2 {
			s.MinMethods = 2
		}

		orders := interfaceOrders(c.Typed)
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkMethodOrder(pkg, orders, &s)...)
		}
		diag.Sort(out)
		return out
	},
}

// interfaceOrders collects the declared method order of every interface in the
// module, across packages, since a type may implement one declared elsewhere.
func interfaceOrders(program *typed.Program) map[*types.Named][]string {
	out := map[*types.Named][]string{}
	for _, pkg := range program.Sound() {
		for named, order := range typed.InterfaceMethodOrder(pkg) {
			out[named] = order
		}
	}
	return out
}

// checkMethodOrder reports types whose methods disagree with an interface they
// implement.
func checkMethodOrder(pkg *typed.Package, orders map[*types.Named][]string, s *interfaceMethodOrderSettings) []diag.Diagnostic {
	declared := typed.MethodOrder(pkg)
	if len(declared) == 0 {
		return nil
	}

	var out []diag.Diagnostic
	for _, match := range matchesFor(pkg, orders, s) {
		found, ok := declared[match.concrete]
		if !ok {
			continue
		}
		if d := compareOrder(pkg, match, found, s); d != nil {
			out = append(out, *d)
		}
	}
	return out
}

// compareOrder checks one type against one interface.
func compareOrder(pkg *typed.Package, match methodMatch, declared []string, s *interfaceMethodOrderSettings) *diag.Diagnostic {
	wanted := map[string]bool{}
	for _, name := range match.order {
		wanted[name] = true
	}

	// The subsequence of the type's methods that the interface names, in the
	// order the type declares them.
	var got []string
	firstAt, lastAt, index := -1, -1, 0
	for _, name := range declared {
		if !wanted[name] {
			index++
			continue
		}
		if firstAt < 0 {
			firstAt = index
		}
		lastAt = index
		got = append(got, name)
		index++
	}
	// A type only partially implementing the interface is not this rule's
	// business; the compiler or logic/interface-registry will say so.
	if len(got) != len(match.order) {
		return nil
	}

	if !equalOrder(got, match.order) {
		return &diag.Diagnostic{
			Position: methodPosition(pkg, match.concrete, got[0]),
			Message: fmt.Sprintf("%s declares %s methods as %s; the interface declares them %s",
				match.concrete.Obj().Name(), match.iface.Obj().Name(),
				strings.Join(got, ", "), strings.Join(match.order, ", ")),
			Help: fmt.Sprintf("reorder the methods to match %s", match.iface.Obj().Name()),
		}
	}
	if s.Contiguous && lastAt-firstAt+1 != len(got) {
		return &diag.Diagnostic{
			Position: methodPosition(pkg, match.concrete, got[0]),
			Message: fmt.Sprintf("%s interleaves other methods among the %s ones",
				match.concrete.Obj().Name(), match.iface.Obj().Name()),
			Help: "keep the interface's methods together, and put the rest before or after",
		}
	}
	return nil
}

// matchesFor returns the type-and-interface pairs worth checking in a package.
func matchesFor(pkg *typed.Package, orders map[*types.Named][]string, s *interfaceMethodOrderSettings) []methodMatch {
	var out []methodMatch
	for _, a := range typed.Assertions(pkg) {
		if order, ok := orders[a.Interface]; ok && len(order) >= s.MinMethods {
			out = append(out, methodMatch{concrete: a.Concrete, iface: a.Interface, order: order})
		}
	}
	if s.IncludeImplicit {
		out = append(out, implicitMatches(pkg, orders, s)...)
	}

	sort.Slice(out, func(i, j int) bool {
		if a, b := out[i].concrete.Obj().Name(), out[j].concrete.Obj().Name(); a != b {
			return a < b
		}
		return out[i].iface.Obj().Name() < out[j].iface.Obj().Name()
	})
	return out
}

// implicitMatches finds types that satisfy an interface without declaring it.
func implicitMatches(pkg *typed.Package, orders map[*types.Named][]string, s *interfaceMethodOrderSettings) []methodMatch {
	var out []methodMatch
	for _, named := range packageTypes(pkg) {
		for iface, order := range orders {
			if len(order) < s.MinMethods || iface == named {
				continue
			}
			underlying, ok := iface.Underlying().(*types.Interface)
			if !ok {
				continue
			}
			if !types.Implements(named, underlying) && !types.Implements(types.NewPointer(named), underlying) {
				continue
			}
			out = append(out, methodMatch{concrete: named, iface: iface, order: order})
		}
	}
	return out
}

// packageTypes returns the named non-interface types a package declares.
func packageTypes(pkg *typed.Package) []*types.Named {
	if pkg.Types == nil {
		return nil
	}
	var out []*types.Named
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		obj, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			continue
		}
		if _, isIface := named.Underlying().(*types.Interface); isIface {
			continue
		}
		out = append(out, named)
	}
	return out
}

// methodPosition locates a named method of a type.
func methodPosition(pkg *typed.Package, named *types.Named, method string) diag.Position {
	for i := range named.NumMethods() {
		m := named.Method(i)
		if m.Name() != method {
			continue
		}
		p := pkg.Fset.Position(m.Pos())
		return diag.Position{Path: pkg.FileOf(m.Pos()), Line: p.Line, Col: p.Column}
	}
	p := pkg.Fset.Position(named.Obj().Pos())
	return diag.Position{Path: pkg.FileOf(named.Obj().Pos()), Line: p.Line, Col: p.Column}
}

func equalOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
