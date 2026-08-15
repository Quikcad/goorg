package rule

import (
	"go/ast"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/source/project"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

// Context is what a rule is given to do its work.
type Context struct {
	// Project is the parsed source tree. Always present.
	Project *project.Project

	// Typed is the type-checked view. It is nil for syntax-tier rules, and
	// non-nil for type-tier ones — the runner only invokes a Types rule when
	// loading succeeded, so a type rule may read this without a nil check.
	Typed *typed.Program

	// decode applies this rule's settings block onto a destination struct. It
	// is nil when the project configures no settings for the rule. Keeping it
	// an opaque closure is what stops package rule from importing package
	// config, so adding a rule option never touches the config package.
	decode func(any) error
}

// NewContext builds a Context for a syntax-tier rule. decode may be nil, in
// which case Settings is a no-op and rules keep their zero-value defaults.
func NewContext(p *project.Project, decode func(any) error) *Context {
	return &Context{Project: p, decode: decode}
}

// NewTypedContext builds a Context for a type-tier rule.
func NewTypedContext(p *project.Project, t *typed.Program, decode func(any) error) *Context {
	return &Context{Project: p, Typed: t, decode: decode}
}

// Settings decodes this rule's configured settings into dst, which should point
// at a struct pre-populated with the rule's defaults. When the project
// configures nothing for the rule, dst is left untouched.
//
//goorg:ignore logic/any-should-be-generic — the decoder erases the type regardless
func (c *Context) Settings(dst any) error {
	if c.decode == nil {
		return nil
	}
	return c.decode(dst)
}

// Pos returns the root-relative position of an AST node.
func (c *Context) Pos(n ast.Node) diag.Position {
	if n == nil {
		return diag.Position{}
	}
	return c.Project.Position(n.Pos())
}

// FilePos points at the top of a file, for diagnostics about the file itself
// rather than about anything inside it.
func FilePos(f *project.File) diag.Position {
	return diag.Position{Path: f.Rel, Line: 1}
}

// DirPos points at a directory. Line stays 0 because directories have no lines;
// reporters render this as a bare path.
func DirPos(rel string) diag.Position {
	return diag.Position{Path: rel}
}
