package pattern

import (
	"fmt"
	"go/ast"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// expandStructSettings is the configurable surface of the rule.
type expandStructSettings struct {
	// IncludeAnonymous applies the rule to struct types that are not named:
	// table-test row types, struct-typed variables and parameters.
	IncludeAnonymous bool `yaml:"include_anonymous"`
	// OneFieldPerLine additionally forbids `X, Y int` on a shared line.
	OneFieldPerLine bool `yaml:"one_field_per_line"`
}

var expandStructDefinition = &rule.Rule{
	ID:       "pat/expand-struct-definition",
	Category: rule.Pattern,
	Tier:     rule.Syntax,
	Summary:  "a struct with fields is written across multiple lines",
	Default:  diag.Error,
	Doc: `A struct type with at least one field is written across multiple lines,
one field per line.

	type Point struct{ X, Y int }     violation

	type Point struct {               OK
		X int
		Y int
	}

gofmt does not do this. It preserves whichever form the author wrote, so a
one-line struct survives formatting indefinitely — which is exactly why the rule
has room to exist rather than duplicating the formatter.

The empty struct is always exempt. struct{} is a unit value, not a record, and
map[string]struct{}, chan struct{} and struct{}{} are load-bearing Go idiom.

Rationale: a one-line struct is a struct that has not been budgeted for. Every
field added later either extends the line or forces a whole-declaration rewrite,
so the line grows instead. The multi-line form also makes adding a field exactly
a one-line diff, which is what makes struct changes reviewable — in the
collapsed form, adding a field rewrites the declaration and review loses the
ability to see what actually changed.

To fix: put each field on its own line.

one_field_per_line extends the same reasoning to grouped fields: X, Y int saves
a line and costs the ability to document, tag, or change the type of X without
touching Y.

Configure in .goorg.yaml:

	settings:
	  pat/expand-struct-definition:
	    include_anonymous: true
	    one_field_per_line: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := expandStructSettings{IncludeAnonymous: true, OneFieldPerLine: true}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range c.Project.Files() {
			named := namedStructs(f.Syntax)
			ast.Inspect(f.Syntax, func(n ast.Node) bool {
				st, ok := n.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				// The empty struct is a unit value; there is nothing to expand.
				if st.Fields.NumFields() == 0 {
					return true
				}
				if !s.IncludeAnonymous && !named[st] {
					return true
				}
				out = append(out, checkStruct(c, st, &s)...)
				return true
			})
		}
		return out
	},
}

// checkStruct reports a struct written on one line, or with grouped fields.
func checkStruct(c *rule.Context, st *ast.StructType, s *expandStructSettings) []diag.Diagnostic {
	open := c.Project.Position(st.Fields.Opening).Line
	if open == c.Project.Position(st.Fields.Closing).Line {
		return []diag.Diagnostic{{
			Position: c.Pos(st),
			Message:  fmt.Sprintf("struct with %s is written on one line", plural(st.Fields.NumFields(), "field")),
			Help:     "put each field on its own line",
		}}
	}
	if !s.OneFieldPerLine {
		return nil
	}

	var out []diag.Diagnostic
	for _, field := range st.Fields.List {
		if len(field.Names) < 2 {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: c.Pos(field),
			Message:  fmt.Sprintf("%s share one declaration", plural(len(field.Names), "field")),
			Help:     "give each field its own line, so it can be documented and changed alone",
		})
	}
	return out
}

// namedStructs returns the struct types that are the body of a type
// declaration, as opposed to anonymous ones inside expressions.
func namedStructs(f *ast.File) map[*ast.StructType]bool {
	out := map[*ast.StructType]bool{}
	for _, node := range f.Decls {
		gen, ok := node.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				out[st] = true
			}
		}
	}
	return out
}
