package pattern

import (
	"fmt"
	"go/ast"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/decl"
)

// Scopes for pat/factory-naming.
const (
	// scopePrefixed checks only functions already named New* or Make*.
	scopePrefixed = "prefixed"
	// scopeAllFactories additionally requires every function returning a local
	// type to carry one of the prefixes.
	scopeAllFactories = "all-factories"
)

// factoryNamingSettings is the configurable surface of the rule.
type factoryNamingSettings struct {
	ValuePrefix   string `yaml:"value_prefix"`
	PointerPrefix string `yaml:"pointer_prefix"`
	// Scope is `prefixed` or `all-factories`.
	Scope string `yaml:"scope"`
	// AllowNames are exempt under all-factories, being established Go idiom.
	AllowNames []string `yaml:"allow_names"`
}

var factoryNaming = &rule.Rule{
	ID:       "pat/factory-naming",
	Category: rule.Pattern,
	Tier:     rule.Syntax,
	Summary:  "Make returns a value, New returns a pointer",
	Default:  diag.Error,
	Doc: `A factory returning a value type is named Make. A factory returning a
pointer is named New.

	func MakeConfig() Config      OK
	func NewConfig() *Config      OK

	func NewConfig() Config       violation — returns a value, named New
	func MakeConfig() *Config     violation — returns a pointer, named Make

The bare forms New and Make are equally acceptable, and preferred where the
package name already carries the type: billing.New() reads better than
billing.NewBilling(). A trailing error is ignored when classifying, so
func NewClient() (*Client, error) is a pointer factory.

Rationale: whether a constructor hands back a value or a pointer determines
everything a caller does next — whether assignment copies or aliases, whether a
nil check is required, whether the zero value is meaningful, whether the result
is safe to share across goroutines. Today that answer lives in the signature,
which means reading it, and at a call site like cfg := pkg.NewConfig() the
signature is not on screen. Encoding it in the prefix moves the answer to the
call site.

To fix: rename to match what the function returns, or change what it returns.

Scope is the design fork. The shipped default, prefixed, checks only functions
already named New* or Make*, so it cannot produce a false positive on idiomatic
code — it only judges names the author already chose. all-factories additionally
demands the prefix on every function returning a local type, which flags Parse,
Open, Dial, FromString and MustCompile: all established Go idiom, all correct as
written. The allow_names list makes that survivable but will never be complete.

Result types imported from another package are skipped. Deciding whether such a
type is an interface — which callers cannot take the address of, and which
therefore wants New — needs type information this tier does not have, and
guessing would misname every constructor returning an error-like interface.

Configure in .goorg.yaml:

	settings:
	  pat/factory-naming:
	    value_prefix: Make
	    pointer_prefix: New
	    scope: prefixed`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := factoryNamingSettings{
			ValuePrefix:   "Make",
			PointerPrefix: "New",
			Scope:         scopePrefixed,
			AllowNames:    []string{"Parse", "Open", "Dial", "Must", "From", "Load", "Decode", "Unmarshal"},
		}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		switch s.Scope {
		case scopePrefixed, scopeAllFactories:
		default:
			return settingsError(fmt.Errorf("unknown scope %q (want prefixed or all-factories)", s.Scope))
		}

		var out []diag.Diagnostic
		for _, f := range c.Project.Files() {
			if f.IsTest {
				continue
			}
			local := decl.LocalTypes(f.Syntax)
			for _, node := range f.Syntax.Decls {
				fn, ok := node.(*ast.FuncDecl)
				if !ok || fn.Recv != nil {
					continue
				}
				if d := checkFactory(c, fn, local, &s); d != nil {
					out = append(out, *d)
				}
			}
		}
		return out
	},
}

// checkFactory reports a factory whose prefix disagrees with what it returns.
func checkFactory(c *rule.Context, fn *ast.FuncDecl, local map[string]bool, s *factoryNamingSettings) *diag.Diagnostic {
	name := fn.Name.Name
	hasValue := hasPrefix(name, s.ValuePrefix)
	hasPointer := hasPrefix(name, s.PointerPrefix)

	result, ok := firstResult(fn)
	if !ok {
		if hasValue || hasPointer {
			return &diag.Diagnostic{
				Position: c.Pos(fn.Name),
				Message:  fmt.Sprintf("%s is named as a factory but returns nothing", name),
				Help:     "rename it, or return the value it builds",
			}
		}
		return nil
	}

	// A result type declared elsewhere cannot be classified from syntax: an
	// imported interface looks exactly like an imported struct.
	if !local[decl.BaseTypeName(result)] {
		return nil
	}

	_, isPointer := result.(*ast.StarExpr)
	switch {
	case !hasValue && !hasPointer:
		if s.Scope != scopeAllFactories || allowed(name, s.AllowNames) {
			return nil
		}
		want := s.ValuePrefix
		if isPointer {
			want = s.PointerPrefix
		}
		return &diag.Diagnostic{
			Position: c.Pos(fn.Name),
			Message:  fmt.Sprintf("%s builds %s but carries no factory prefix", name, decl.BaseTypeName(result)),
			Help:     fmt.Sprintf("name it %s%s", want, decl.BaseTypeName(result)),
		}
	case hasPointer && !isPointer:
		return &diag.Diagnostic{
			Position: c.Pos(fn.Name),
			Message:  fmt.Sprintf("%s returns a value, so it should be named %s...", name, s.ValuePrefix),
			Help:     fmt.Sprintf("rename it to %s, or return a pointer", s.ValuePrefix+strings.TrimPrefix(name, s.PointerPrefix)),
		}
	case hasValue && isPointer:
		return &diag.Diagnostic{
			Position: c.Pos(fn.Name),
			Message:  fmt.Sprintf("%s returns a pointer, so it should be named %s...", name, s.PointerPrefix),
			Help:     fmt.Sprintf("rename it to %s, or return a value", s.PointerPrefix+strings.TrimPrefix(name, s.ValuePrefix)),
		}
	default:
		return nil
	}
}

// firstResult returns the first non-error result of a function.
func firstResult(fn *ast.FuncDecl) (ast.Expr, bool) {
	if fn.Type.Results == nil {
		return nil, false
	}
	for _, field := range fn.Type.Results.List {
		if id, ok := field.Type.(*ast.Ident); ok && id.Name == "error" {
			continue
		}
		return field.Type, true
	}
	return nil, false
}

// hasPrefix reports whether a name carries a factory prefix at a word boundary,
// so that Newton is not mistaken for a New factory.
func hasPrefix(name, prefix string) bool {
	if prefix == "" || !strings.HasPrefix(name, prefix) {
		return false
	}
	rest := name[len(prefix):]
	if rest == "" {
		return true
	}
	return rest[0] >= 'A' && rest[0] <= 'Z'
}

func allowed(name string, names []string) bool {
	for _, n := range names {
		if hasPrefix(name, n) || name == n {
			return true
		}
	}
	return false
}
