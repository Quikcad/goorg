package logic

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

var exportedEmbeddedMutex = &rule.Rule{
	ID:       "logic/exported-embedded-mutex",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "an exported struct embedding a mutex leaks Lock into its API",
	Default:  diag.Error,
	Doc: `An exported struct may not embed sync.Mutex or sync.RWMutex.

	type Cache struct {              violation — Cache.Lock is now public API
		sync.Mutex
		entries map[string]string
	}

	type Cache struct {              OK
		mu      sync.Mutex
		entries map[string]string
	}

Rationale: embedding promotes Lock and Unlock into the struct's method set, so
any importer can lock the type's mutex. That is not a hypothetical: it lets a
caller hold the lock across a call back into the type and deadlock it, or unlock
a mutex the type is relying on. The type's invariant becomes something its own
package cannot enforce, and it is now part of the API — removing the embedding
later is a breaking change.

To fix: name the field. mu sync.Mutex costs three characters and keeps the lock
inside the type that owns it.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkEmbeddedMutex(pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

var contextInStruct = &rule.Rule{
	ID:       "logic/context-in-struct",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "a context.Context stored in a struct outlives its request",
	Default:  diag.Error,
	Doc: `A struct may not have a context.Context field.

	type Worker struct {             violation
		ctx context.Context
	}

	func (w *Worker) Run(ctx context.Context) error   OK

Rationale: a Context carries a deadline and a cancellation signal that belong to
one call, and storing it ties that lifetime to the object's instead. A Worker
built during startup holds the startup context forever, so cancellation never
reaches its work; a Worker built per request and reused holds a context that is
already cancelled, so its work fails for no visible reason. Both failures are
about lifetime, which is exactly what the field hides.

To fix: pass the context as the first parameter of each method that needs one.
That is also what makes the cancellation path readable from the signature.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkContextFields(pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

var unusedTypeParameter = &rule.Rule{
	ID:       "logic/unused-type-parameter",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "a type parameter used once is not doing generic work",
	Default:  diag.Warning,
	Doc: `A type parameter must appear more than once in a signature.

	func Log[T any](v T)             violation — T relates nothing to anything
	func Log(v any)                  the same function, said plainly

	func First[T any](xs []T) T      OK — T ties the result to the input

Rationale: the purpose of a type parameter is to relate two positions — an
argument to a result, or two arguments to each other. Appearing once, it relates
nothing, and the function accepts exactly what any accepts while looking more
constrained than it is. It also costs the caller: generic instantiation at every
call site for a guarantee the signature does not actually make.

To fix: use any if the type genuinely does not matter, or tie the parameter to a
result so it earns its place.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			out = append(out, checkTypeParams(pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

// panicSettings is the configurable surface of logic/panic-outside-main.
type panicSettings struct {
	// AllowIn are path prefixes where panicking is the program's own choice.
	AllowIn []string `yaml:"allow_in"`
}

var panicOutsideMain = &rule.Rule{
	ID:       "logic/panic-outside-main",
	Category: rule.Logic,
	Tier:     rule.Types,
	Summary:  "a library may not panic",
	Default:  diag.Error,
	Doc: `panic may only be called from a main package.

	func Parse(s string) Doc {       violation — takes the caller's process down
		if s == "" {
			panic("empty input")
		}
	}

Rationale: panicking is a decision about the whole process, and a library is not
entitled to make it. The caller may have a perfectly good response to bad input
— reject the request, fall back, retry — and a panic removes every one of those
options. It also cannot be handled at the call site without recover, which
propagates the mistake outward rather than containing it.

To fix: return an error. If the condition is genuinely impossible, it is an
invariant the type should make unrepresentable.

Exempt: main packages, which are the program and may decide to stop; test files;
and Must-prefixed constructors, whose panicking is the documented contract.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := panicSettings{AllowIn: []string{"cmd/"}}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, pkg := range c.Typed.Sound() {
			if pkg.Name == "main" || allowedPath(pkg.Dir, s.AllowIn) {
				continue
			}
			out = append(out, checkPanics(pkg)...)
		}
		diag.Sort(out)
		return out
	},
}

// checkEmbeddedMutex reports exported structs embedding a lock.
func checkEmbeddedMutex(pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	forEachTypeSpec(pkg, func(ts *ast.TypeSpec, st *ast.StructType) {
		if !ts.Name.IsExported() {
			return
		}
		for _, field := range st.Fields.List {
			if len(field.Names) != 0 || !isLockType(pkg.Info.TypeOf(field.Type)) {
				continue
			}
			out = append(out, diag.Diagnostic{
				Position: positionOf(pkg, field.Pos()),
				Message:  fmt.Sprintf("%s embeds a mutex, promoting Lock into its public API", ts.Name.Name),
				Help:     "name the field: mu sync.Mutex",
			})
		}
	})
	return out
}

// checkContextFields reports struct fields holding a context.
func checkContextFields(pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	forEachTypeSpec(pkg, func(ts *ast.TypeSpec, st *ast.StructType) {
		for _, field := range st.Fields.List {
			if !isContextValue(pkg.Info.TypeOf(field.Type)) {
				continue
			}
			out = append(out, diag.Diagnostic{
				Position: positionOf(pkg, field.Pos()),
				Message:  fmt.Sprintf("%s stores a context, tying its lifetime to the struct", ts.Name.Name),
				Help:     "pass the context as the first parameter of each method that needs one",
			})
		}
	})
	return out
}

// checkPanics reports calls to the builtin panic.
func checkPanics(pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	for i, file := range pkg.Syntax {
		if i < len(pkg.Files) && isTestFile(pkg.Files[i]) {
			continue
		}
		for _, node := range file.Decls {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Body == nil || strings.HasPrefix(fn.Name.Name, "Must") {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok || id.Name != "panic" {
					return true
				}
				if _, isBuiltin := pkg.Info.Uses[id].(*types.Builtin); !isBuiltin {
					return true
				}
				out = append(out, diag.Diagnostic{
					Position: positionOf(pkg, call.Pos()),
					Message:  fmt.Sprintf("%s panics, which takes the caller's process down", fn.Name.Name),
					Help:     "return an error and let the caller decide",
				})
				return true
			})
		}
	}
	return out
}

// checkTypeParams reports type parameters appearing only once.
func checkTypeParams(pkg *typed.Package) []diag.Diagnostic {
	var out []diag.Diagnostic
	for i, file := range pkg.Syntax {
		if i < len(pkg.Files) && isTestFile(pkg.Files[i]) {
			continue
		}
		for _, node := range file.Decls {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Type.TypeParams == nil {
				continue
			}
			out = append(out, unusedParamsOf(pkg, fn)...)
		}
	}
	return out
}

// unusedParamsOf reports the type parameters of one function that appear once.
func unusedParamsOf(pkg *typed.Package, fn *ast.FuncDecl) []diag.Diagnostic {
	counts := countTypeParamUses(fn)
	var out []diag.Diagnostic
	for _, name := range typeParamNames(fn) {
		if counts[name.Name] > 1 {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: positionOf(pkg, name.Pos()),
			Message:  fmt.Sprintf("type parameter %s appears once, so it relates nothing", name.Name),
			Help:     "use any if the type does not matter, or tie it to a result",
		})
	}
	return out
}

// typeParamNames flattens a function's type parameter list.
func typeParamNames(fn *ast.FuncDecl) []*ast.Ident {
	var out []*ast.Ident
	for _, field := range fn.Type.TypeParams.List {
		out = append(out, field.Names...)
	}
	return out
}

// countTypeParamUses counts each type parameter's appearances in a signature.
func countTypeParamUses(fn *ast.FuncDecl) map[string]int {
	declared := map[string]int{}
	for _, field := range fn.Type.TypeParams.List {
		for _, name := range field.Names {
			declared[name.Name] = 0
		}
	}
	for _, list := range []*ast.FieldList{fn.Type.Params, fn.Type.Results} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			ast.Inspect(field.Type, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					if _, tracked := declared[id.Name]; tracked {
						declared[id.Name]++
					}
				}
				return true
			})
		}
	}
	return declared
}

// forEachTypeSpec visits every struct declaration in a package's non-test files.
func forEachTypeSpec(pkg *typed.Package, visit func(*ast.TypeSpec, *ast.StructType)) {
	for i, file := range pkg.Syntax {
		if i < len(pkg.Files) && isTestFile(pkg.Files[i]) {
			continue
		}
		for _, node := range file.Decls {
			gen, ok := node.(*ast.GenDecl)
			if !ok {
				continue
			}
			visitStructSpecs(gen, visit)
		}
	}
}

// visitStructSpecs calls visit for each struct declared in one type block.
func visitStructSpecs(gen *ast.GenDecl, visit func(*ast.TypeSpec, *ast.StructType)) {
	for _, spec := range gen.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		if st, ok := ts.Type.(*ast.StructType); ok && st.Fields != nil {
			visit(ts, st)
		}
	}
}

func isLockType(t types.Type) bool {
	name := qualifiedName(t)
	return name == "sync.Mutex" || name == "sync.RWMutex"
}

func isContextValue(t types.Type) bool {
	return qualifiedName(t) == "context.Context"
}

func allowedPath(dir string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(dir, prefix) {
			return true
		}
	}
	return false
}
