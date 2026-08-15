package logic

import (
	"fmt"
	"go/ast"
	"go/types"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

var interfaceAtConsumer = &rule.Rule{
	ID:       "logic/interface-at-consumer",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "an interface declared beside its only implementation belongs at the consumer",
	Default:  diag.Warning,
	Doc: `An interface declared in the same package as its only implementation is
declared in the wrong place.

	package store                    violation
	type Store interface { Get(string) ([]byte, error) }
	type diskStore struct{}          the only implementation

	package handler                  OK — the consumer states what it needs
	type getter interface { Get(string) ([]byte, error) }

Rationale: an interface is a statement about what a caller needs, and the caller
is the only party who knows that. Declared beside its implementation it becomes
a statement about what the implementation offers, which is what the concrete
type already says — so it adds a layer without adding information. It also
forces every consumer through one abstraction chosen by someone who could not
see their requirements, and it makes the implementation's package the one that
must change when a consumer needs something narrower.

To fix: delete the interface and return the concrete type. Let each consumer
declare the small interface it actually needs.

Reported only when the declaring package holds an implementation and no other
package in the module does, which is the case where the indirection is
demonstrably buying nothing.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkProducerInterfaces(c.Typed, pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

var constraintTooWide = &rule.Rule{
	ID:       "logic/constraint-too-wide",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "a type parameter narrowed at runtime should be narrowed in its constraint",
	Default:  diag.Warning,
	Doc: `A type parameter constrained by any, whose body then asserts or switches
on the value's dynamic type, has moved a compile-time contract to runtime.

	func Render[T any](v T) string {         violation
		if s, ok := any(v).(fmt.Stringer); ok {
			return s.String()
		}
		return "?"
	}

	func Render[T fmt.Stringer](v T) string {    OK
		return v.String()
	}

Rationale: the assertion is the constraint, written in the wrong place. As a
constraint it is checked at the call site, names the requirement in the
signature, and produces a comprehensible error when a caller does not meet it.
As an assertion it is checked at run time, is invisible from the signature, and
its failure path is a silent fallback that callers discover in production.

To fix: move the asserted interface into the constraint.

Detection is limited to this shape. The general question — whether a wider
constraint than the body needs was chosen — cannot be answered from the code,
because a constraint the body does not exercise may still be the contract the
author intends.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkWideConstraints(pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

// stringerSettings is the configurable surface of logic/enum-missing-string.
type stringerSettings struct {
	// MinConstants is how many values a type needs before String pays off.
	MinConstants int `yaml:"min_constants"`
}

var enumMissingString = &rule.Rule{
	ID:       "logic/enum-missing-string",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "an enum type has no String method",
	Default:  diag.Warning,
	Doc: `A named type with a run of constants declares a String method.

	type Status int                  violation — logs print 0, 1, 2
	const (
		StatusDraft Status = iota
		StatusSent
	)

	func (s Status) String() string { ... }      OK

Rationale: without String, every log line, error message and test failure
involving the value prints an integer. The reader then has to find the const
block and count to work out which state it was — at exactly the moment they are
debugging and least able to afford it. The cost is paid on every read of every
diagnostic the value ever appears in, and it is paid by whoever is least
equipped to pay it.

To fix: add a String method covering every constant, and a default branch that
reports the numeric value so an unrecognised one is still identifiable.

A string-backed enum is exempt: it already prints its own value, so a String
method would only repeat it.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := stringerSettings{MinConstants: 3}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkEnumStringers(pkg, &s)...)
		}
		diag.Sort(out)
		return out
	},
}

// checkEnumStringers reports enum types with no String method.
func checkEnumStringers(pkg *typed.Package, s *stringerSettings) []diag.Diagnostic {
	counts := constantsPerType(pkg)

	var out []diag.Diagnostic
	for _, named := range namedTypes(pkg) {
		if counts[named.Obj().Name()] < s.MinConstants {
			continue
		}
		if _, has := methodSet(named)["String"]; has {
			continue
		}
		// A string-backed enum already prints its own name; String would only
		// repeat the value.
		if basicName(named) == "string" {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: positionOf(pkg, named.Obj().Pos()),
			Message: fmt.Sprintf("enum %s has %s and no String method",
				named.Obj().Name(), plural(counts[named.Obj().Name()], "constant")),
			Help: "add a String method so logs and test failures name the value",
		})
	}
	return out
}

// constantsPerType counts the declared constants of each named type.
func constantsPerType(pkg *typed.Package) map[string]int {
	out := map[string]int{}
	if pkg.Types == nil {
		return out
	}
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		konst, ok := scope.Lookup(name).(*types.Const)
		if !ok {
			continue
		}
		if named, ok := konst.Type().(*types.Named); ok {
			out[named.Obj().Name()]++
		}
	}
	return out
}

// checkProducerInterfaces reports interfaces declared beside their only
// implementation.
func checkProducerInterfaces(program *typed.Program, pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	if pkg.Types == nil {
		return nil
	}
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
		iface, ok := named.Underlying().(*types.Interface)
		if !ok || iface.NumMethods() == 0 {
			continue
		}
		local, elsewhere := countImplementations(program, pkg, iface)
		if local == 0 || elsewhere > 0 {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: positionOf(pkg, obj.Pos()),
			Message: fmt.Sprintf("interface %s is declared beside its only implementation",
				obj.Name()),
			Help: "return the concrete type; let each consumer declare the interface it needs",
		})
	}
	return out
}

// countImplementations counts the types satisfying an interface, inside the
// declaring package and outside it.
func countImplementations(program *typed.Program, home *typed.Package, iface *types.Interface) (local, elsewhere int) {
	for _, pkg := range program.Sound() {
		for _, named := range namedTypes(pkg) {
			if !implements(named, iface) {
				continue
			}
			if pkg == home {
				local++
				continue
			}
			elsewhere++
		}
	}
	return local, elsewhere
}

// checkWideConstraints reports an any-constrained type parameter whose value is
// asserted to an interface inside the body.
func checkWideConstraints(pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	for i, file := range pkg.Syntax {
		if i < len(pkg.Files) && isTestFile(pkg.Files[i]) {
			continue
		}
		for _, node := range file.Decls {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Type.TypeParams == nil {
				continue
			}
			wide := anyConstrained(fn)
			if len(wide) == 0 {
				continue
			}
			for _, asserted := range assertedInterfaces(pkg, fn) {
				out = append(out, diag.Diagnostic{
					Position: positionOf(pkg, fn.Name.Pos()),
					Message: fmt.Sprintf("%s constrains a type parameter to any, then asserts %s at run time",
						fn.Name.Name, asserted),
					Help: fmt.Sprintf("constrain the parameter to %s instead", asserted),
				})
				break
			}
		}
	}
	return out
}

// anyConstrained returns the type parameters constrained by any.
func anyConstrained(fn *ast.FuncDecl) []string {
	var out []string
	for _, field := range fn.Type.TypeParams.List {
		if !isBareAny(field.Type) {
			continue
		}
		for _, name := range field.Names {
			out = append(out, name.Name)
		}
	}
	return out
}

// assertedInterfaces returns the interface types a body asserts to.
func assertedInterfaces(pkg *typed.Package, fn *ast.FuncDecl) []string {
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assert, ok := n.(*ast.TypeAssertExpr)
		if !ok || assert.Type == nil {
			return true
		}
		t := pkg.Info.TypeOf(assert.Type)
		if t == nil {
			return true
		}
		if _, isIface := t.Underlying().(*types.Interface); !isIface {
			return true
		}
		out = append(out, qualifiedName(t))
		return true
	})
	return out
}
