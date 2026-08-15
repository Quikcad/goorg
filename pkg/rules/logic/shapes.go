package logic

import (
	"fmt"
	"go/ast"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

var pointerToSliceOrMap = &rule.Rule{
	ID:       "logic/pointer-to-slice-or-map",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a pointer to a slice or map is almost always a mistake",
	Default:  diag.Warning,
	Doc: `A signature may not take or return *[]T or *map[K]V.

	func Append(xs *[]int, x int)    violation
	func Append(xs []int, x int) []int   OK

Rationale: slices and maps already hold a pointer to their backing store, so a
map passed by value shares its contents and a slice shares its elements. Taking
a pointer signals that the author believed otherwise, and the code around it is
usually written on that mistaken belief. For a map the pointer buys nothing at
all. For a slice it buys exactly one thing — reassigning the caller's header —
which Go's convention expresses by returning the new slice instead, the way
append does.

To fix: take the slice or map by value. If the length must change, return the
new slice.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				fn, ok := node.(*ast.FuncDecl)
				if !ok {
					continue
				}
				for _, field := range signatureFields(fn) {
					kind := pointerToReference(field.Type)
					if kind == "" {
						continue
					}
					out = append(out, diag.Diagnostic{
						Position: c.Pos(field.Type),
						Message:  fmt.Sprintf("%s already holds a reference; the pointer adds nothing", kind),
						Help:     "take it by value, and return the new slice if the length changes",
					})
				}
			}
		}
		return out
	},
}

var emptyInterfaceField = &rule.Rule{
	ID:       "logic/empty-interface-field",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a struct field typed any erases what it holds",
	Default:  diag.Warning,
	Doc: `A struct field may not be typed any.

	type Event struct {              violation
		Payload any
	}

Rationale: an any field has the same erasure problem as an any parameter, and
outlives it. A parameter's dynamic type is asserted back within one function; a
field's is asserted wherever the struct travels, which is everywhere, by code
that has no way to know what was put in. The set of types the field can hold is
real but written nowhere, so it can only be learned by finding every writer.

To fix: name the types the field can hold — a type parameter if there is one, an
interface with methods if there are several, a sum-like set of concrete fields
if the cases are fixed.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, f := range files(c) {
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				st, ok := n.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				for _, field := range st.Fields.List {
					if !isBareAny(field.Type) {
						continue
					}
					out = append(out, diag.Diagnostic{
						Position: c.Pos(field),
						Message:  "struct field is typed any, erasing what it holds",
						Help:     "name the types it can hold, or use a type parameter",
					})
				}
				return true
			})
		}
		return out
	},
}

var booleanParameter = &rule.Rule{
	ID:       "logic/boolean-parameter",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a bool parameter on an exported function is unreadable at the call site",
	// Opt-in. The objection is real but the shape is common and not wrong, and
	// on a real codebase this fired 71 times — too often to be the default
	// position. See docs/decisions.md D7.
	Default: diag.Off,
	Doc: `An exported function may not take a bare bool parameter.

	Fetch(id, true)                  violation — true what?
	FetchWithRetry(id)               OK
	Fetch(id, Options{Retry: true})  OK

Rationale: at the call site a boolean argument is a literal with no name
attached. The reader has to open the signature to learn what true means, and
half the time guesses instead. Two booleans are worse than twice as bad, since
they can also be transposed silently. The parameter is also a fork in the
function's behaviour that its name does not mention.

To fix: split into two functions named for what they do, or take an options
struct where the field name labels the value at the call site.

Unexported functions are exempt: their call sites are in the same package as the
signature, so the lookup is cheap.

This rule ships off. Enable it with

	rules:
	  logic/boolean-parameter: warning

when the team has decided it wants the convention. The objection is real, but a
bare bool parameter is common enough that firing by default reads as noise
rather than as advice.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				fn, ok := node.(*ast.FuncDecl)
				if !ok || !fn.Name.IsExported() || fn.Type.Params == nil {
					continue
				}
				for _, field := range fn.Type.Params.List {
					id, ok := field.Type.(*ast.Ident)
					if !ok || id.Name != "bool" {
						continue
					}
					out = append(out, diag.Diagnostic{
						Position: c.Pos(field),
						Message:  fmt.Sprintf("%s takes a bare bool, which the call site cannot label", fn.Name.Name),
						Help:     "split the function, or take an options struct",
					})
				}
			}
		}
		return out
	},
}

// interfaceSizeSettings is the configurable surface of logic/interface-size.
type interfaceSizeSettings struct {
	Max int `yaml:"max"`
}

var interfaceSize = &rule.Rule{
	ID:       "logic/interface-size",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "an interface declares at most a configured number of methods",
	Default:  diag.Warning,
	Doc: `An interface may declare at most N methods.

	type Store interface {           violation at a limit of 3
		Get(id string) ([]byte, error)
		Put(id string, b []byte) error
		Delete(id string) error
		List() ([]string, error)
		Close() error
	}

Rationale: an interface's size is a tax on everyone who implements it, and the
people who pay it most are the ones writing test doubles. A five-method
interface means a fake with five methods, four of which the test does not care
about — so the fake gets written once, shared, and quietly becomes a second
implementation nobody maintains. Small interfaces are also what make a
dependency honest: a function that needs Get should ask for a Getter, not for
the whole store.

To fix: split the interface along the way its methods are actually consumed. The
consumer usually needs one or two.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := interfaceSizeSettings{Max: 4}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.Max <= 0 {
			return nil
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				gen, ok := node.(*ast.GenDecl)
				if !ok {
					continue
				}
				out = append(out, oversizedInterfaces(c, gen, s.Max)...)
			}
		}
		return out
	},
}

// oversizedInterfaces reports interfaces declaring too many methods.
func oversizedInterfaces(c *rule.Context, gen *ast.GenDecl, max int) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, spec := range gen.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		iface, ok := ts.Type.(*ast.InterfaceType)
		if !ok || iface.Methods == nil {
			continue
		}
		count := 0
		for _, field := range iface.Methods.List {
			// An embedded interface is one entry, not its expansion.
			count += max2(len(field.Names), 1)
		}
		if count <= max {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: c.Pos(ts.Name),
			Message: fmt.Sprintf("interface %s declares %s, over the limit of %d",
				ts.Name.Name, plural(count, "method"), max),
			Help: "split it along the way its methods are actually consumed",
		})
	}
	return out
}

// signatureFields returns every parameter and result field of a function.
func signatureFields(fn *ast.FuncDecl) []*ast.Field {
	var out []*ast.Field
	if fn.Type.Params != nil {
		out = append(out, fn.Type.Params.List...)
	}
	if fn.Type.Results != nil {
		out = append(out, fn.Type.Results.List...)
	}
	return out
}

// pointerToReference names the reference kind behind a pointer, or "".
func pointerToReference(expr ast.Expr) string {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return ""
	}
	switch t := star.X.(type) {
	case *ast.MapType:
		return "a map"
	case *ast.ArrayType:
		// A pointer to a fixed-size array is legitimate; only slices are.
		if t.Len == nil {
			return "a slice"
		}
	}
	return ""
}

// isBareAny reports whether an expression is `any` or `interface{}`.
func isBareAny(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == "any"
	case *ast.InterfaceType:
		return t.Methods == nil || len(t.Methods.List) == 0
	default:
		return false
	}
}
