package logic

import (
	"fmt"
	"go/types"
	"sort"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

// interfaceRegistrySettings is the configurable surface of the rule.
type interfaceRegistrySettings struct {
	// Interfaces are checked against every named type in the project.
	Interfaces []string `yaml:"interfaces"`
	// ReportNearMiss reports a type that has a method of the right name but
	// the wrong signature.
	ReportNearMiss bool `yaml:"report_near_miss"`
	// ReportMissingAssertion reports a type that implements without saying so.
	ReportMissingAssertion bool `yaml:"report_missing_assertion"`
	// AssertionsForExportedOnly limits the assertion check to exported types.
	AssertionsForExportedOnly bool `yaml:"assertions_for_exported_only"`
}

var interfaceRegistry = &rule.Rule{
	ID:       "logic/interface-registry",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "types are checked against a registry of interfaces",
	Default:  diag.Error,
	Doc: `Every named type is checked against a registry of interfaces, and two
situations are reported.

Near miss — the type has a method matching a registry method by name but not by
signature, so it silently fails to implement the interface:

	func (s Status) String() (string, error)     not a fmt.Stringer

Nothing about that is a compile error. fmt.Printf("%v", status) prints the
underlying value and the author never learns why.

Missing assertion — the type satisfies an interface structurally, but nothing
pins that:

	var _ fmt.Stringer = Status(0)              the assertion that pins it

Rationale: interfaces in Go are satisfied implicitly, which is the language's
best feature and its worst failure mode. Implicit satisfaction means nothing in
the source says "this type is meant to be a Stringer", so nothing breaks when it
stops being one. A renamed method or a changed signature turns an
implementation into an ordinary method, and every call site that relied on it
falls back to a default that usually still compiles and often still runs. An
explicit assertion converts that silent behavioural regression into a build
failure at the point of the change.

To fix: for a near miss, correct the signature. For a missing assertion, add
var _ Iface = (*T)(nil) beside the type.

Registry packages are loaded on demand, so an interface works even in a module
that does not yet import it — which is exactly the case where a type that should
implement it does not.

Configure in .goorg.yaml:

	settings:
	  logic/interface-registry:
	    interfaces: [fmt.Stringer, error, io.Reader, io.Writer, io.Closer]
	    report_near_miss: true
	    report_missing_assertion: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := interfaceRegistrySettings{
			Interfaces: []string{
				"fmt.Stringer", "error",
				"encoding.TextMarshaler", "encoding.TextUnmarshaler",
				"io.Reader", "io.Writer", "io.Closer", "sort.Interface",
			},
			ReportNearMiss:            true,
			ReportMissingAssertion:    true,
			AssertionsForExportedOnly: true,
		}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		c.Typed.Import(importPaths(s.Interfaces)...)
		registry := resolveInterfaces(c.Typed, s.Interfaces)
		if len(registry) == 0 {
			return nil
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkPackageInterfaces(pkg, registry, &s)...)
		}
		diag.Sort(out)
		return out
	},
}

// importPaths extracts the package paths a registry needs loaded.
func importPaths(names []string) []string {
	var out []string
	for _, name := range names {
		if path, _, ok := strings.Cut(name, "."); ok {
			out = append(out, path)
		}
	}
	return out
}

// resolveInterfaces turns registry names into interface types, dropping any that
// could not be loaded.
func resolveInterfaces(program *typed.Program, names []string) map[string]*types.Interface {
	out := map[string]*types.Interface{}
	for _, name := range names {
		obj := program.Lookup(name)
		if obj == nil {
			continue
		}
		iface, ok := obj.Type().Underlying().(*types.Interface)
		if !ok || iface.NumMethods() == 0 {
			continue
		}
		out[name] = iface
	}
	return out
}

// checkPackageInterfaces checks every named type in a package.
func checkPackageInterfaces(pkg *typed.Package, registry map[string]*types.Interface, s *interfaceRegistrySettings) []diag.Diagnostic {
	declared := assertionsIn(pkg)
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []diag.Diagnostic
	for _, named := range namedTypes(pkg) {
		for _, ifaceName := range names {
			iface := registry[ifaceName]
			switch {
			case implements(named, iface):
				if !s.ReportMissingAssertion {
					continue
				}
				if s.AssertionsForExportedOnly && !named.Obj().Exported() {
					continue
				}
				if declared[assertion{iface: ifaceName, named: named.Obj().Name()}] {
					continue
				}
				out = append(out, finding(pkg, named,
					fmt.Sprintf("%s implements %s but nothing records it", named.Obj().Name(), ifaceName),
					fmt.Sprintf("add `var _ %s = (*%s)(nil)` so a signature change breaks the build",
						shortName(ifaceName), named.Obj().Name())))
			case s.ReportNearMiss:
				if method, want := nearMiss(named, iface); method != "" {
					out = append(out, finding(pkg, named,
						fmt.Sprintf("%s.%s has the wrong signature for %s", named.Obj().Name(), method, ifaceName),
						fmt.Sprintf("the interface wants %s; as written the type silently does not implement it", want)))
				}
			}
		}
	}
	return out
}

// implements reports whether a type or its pointer satisfies an interface.
func implements(named *types.Named, iface *types.Interface) bool {
	return types.Implements(named, iface) || types.Implements(types.NewPointer(named), iface)
}

// nearMiss returns the method whose name matches an interface method but whose
// signature does not, together with the signature the interface wants.
//
// This is the half of the rule that catches real bugs: a String method
// returning (string, error) is not a Stringer, and nothing says so.
func nearMiss(named *types.Named, iface *types.Interface) (method, want string) {
	have := methodSet(named)
	for i := range iface.NumMethods() {
		m := iface.Method(i)
		got, ok := have[m.Name()]
		if !ok {
			continue
		}
		if types.Identical(got.Type(), m.Type()) {
			continue
		}
		return m.Name(), types.TypeString(m.Type(), relativeTo(named))
	}
	return "", ""
}

// methodSet returns the methods of a type and its pointer, by name.
func methodSet(named *types.Named) map[string]*types.Func {
	out := map[string]*types.Func{}
	for _, t := range []types.Type{named, types.NewPointer(named)} {
		ms := types.NewMethodSet(t)
		for i := range ms.Len() {
			fn, ok := ms.At(i).Obj().(*types.Func)
			if ok {
				out[fn.Name()] = fn
			}
		}
	}
	return out
}

// namedTypes returns the named types a package declares, sorted by name.
func namedTypes(pkg *typed.Package) []*types.Named {
	if pkg.Types == nil {
		return nil
	}
	scope := pkg.Types.Scope()
	var out []*types.Named
	for _, name := range scope.Names() {
		obj, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := obj.Type().(*types.Named)
		if !ok || named.Obj().Pkg() != pkg.Types {
			continue
		}
		// An interface cannot fail to implement itself in a useful way.
		if _, isIface := named.Underlying().(*types.Interface); isIface {
			continue
		}
		out = append(out, named)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Obj().Name() < out[j].Obj().Name() })
	return out
}

func finding(pkg *typed.Package, named *types.Named, message, help string) diag.Diagnostic {
	return diag.Diagnostic{
		Position: positionOf(pkg, named.Obj().Pos()),
		Message:  message,
		Help:     help,
	}
}

// qualifiedName renders a type as pkg.Name, matching the registry's spelling.
func qualifiedName(t types.Type) string {
	named, ok := t.(*types.Named)
	if !ok {
		return types.TypeString(t, nil)
	}
	if named.Obj().Pkg() == nil {
		return named.Obj().Name()
	}
	return named.Obj().Pkg().Name() + "." + named.Obj().Name()
}

func underlyingNamed(t types.Type) string {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return ""
	}
	return named.Obj().Name()
}

func relativeTo(named *types.Named) types.Qualifier {
	if named.Obj().Pkg() == nil {
		return nil
	}
	return types.RelativeTo(named.Obj().Pkg())
}

func shortName(qualified string) string {
	if _, name, ok := strings.Cut(qualified, "."); ok {
		return strings.SplitN(qualified, ".", 2)[0] + "." + name
	}
	return qualified
}
