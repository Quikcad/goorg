package organization

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// memberOrderSettings is the configurable surface of org/member-order.
type memberOrderSettings struct {
	// Grouping keeps each type with its own factory and methods (`per-type`),
	// or accepts any arrangement within the type section (`by-kind`).
	Grouping string `yaml:"grouping"`
	// SeparateVarBlocks requires each package-level var its own declaration.
	SeparateVarBlocks bool `yaml:"separate_var_blocks"`
}

var memberOrder = &rule.Rule{
	ID:        "org/member-order",
	Category:  rule.Organization,
	Tier:      rule.Syntax,
	Placement: rule.PlacementOrder,
	Summary:   "declarations appear in the canonical file order",
	Default:   diag.Error,
	Doc: `Declarations appear in one order: enums, package variables, interfaces,
then each type followed by its factory and its methods, then functions.

	type Status int          // 1. enums — the type and its constants together
	const ( ... )
	var ErrNotFound = ...    // 2. package variables, one block each
	type Handler interface   // 3. interfaces
	type Registry struct     // 4. types, each with its factory and methods
	func NewRegistry() ...
	func (r *Registry) ...
	func Normalize(...)      // 5. functions

Rationale: a file read top to bottom should introduce things before it uses
them. Enums are the vocabulary the rest of the file is written in, types are the
nouns, methods are what those nouns do, and free functions are what is left.
Reading in that order means never meeting a name whose meaning is defined two
hundred lines further down. The order also gives every future addition an
obvious home — without one, new code goes at the bottom regardless of what it
is, and the file becomes a chronological log of what was added when.

separate_var_blocks is the clause that costs something. A grouped var (...)
block reads as a single unit, which is the problem: it invites unrelated globals
to accumulate under one header where each new entry is a one-line diff nobody
questions.

To fix: move the declaration into its section. A type's factory and methods must
stay contiguous with it.

Singleton files are exempt — org/singleton-layout defines their order instead,
and it deliberately inverts this one. Test files are exempt entirely.

Configure in .goorg.yaml:

	settings:
	  org/member-order:
	    grouping: per-type
	    separate_var_blocks: true`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := memberOrderSettings{Grouping: "per-type", SeparateVarBlocks: true}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range orderedFiles(c) {
			out = append(out, checkSectionOrder(c, f)...)
			if s.Grouping == "per-type" {
				out = append(out, checkTypeContiguity(c, f)...)
			}
			if s.SeparateVarBlocks {
				out = append(out, checkVarBlocks(c, f)...)
			}
		}
		return out
	},
}

// checkSectionOrder reports declarations that appear before a section they
// should follow.
func checkSectionOrder(c *rule.Context, f *fileDecls) []diag.Diagnostic {
	var out []diag.Diagnostic
	highest := sectionEnums
	for _, d := range f.decls {
		if d.sect >= highest {
			highest = d.sect
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: c.Pos(d.node),
			Message: fmt.Sprintf("%s appears after the %s section; it belongs in %s",
				describe(d), highest, d.sect),
			Help: "move it up to its section: enums, vars, interfaces, types, functions",
		})
	}
	return out
}

// checkTypeContiguity reports a type whose declarations are interrupted by
// another type's.
func checkTypeContiguity(c *rule.Context, f *fileDecls) []diag.Diagnostic {
	var out []diag.Diagnostic
	closed := map[string]bool{}
	current := ""
	for _, d := range f.decls {
		if d.sect != sectionTypes || d.owner == "" {
			continue
		}
		if d.owner == current {
			continue
		}
		if closed[d.owner] {
			out = append(out, diag.Diagnostic{
				Position: c.Pos(d.node),
				Message: fmt.Sprintf("%s is separated from the rest of %s by %s",
					describe(d), d.owner, current),
				Help: "keep a type, its factory and its methods contiguous",
			})
			continue
		}
		if current != "" {
			closed[current] = true
		}
		current = d.owner
	}
	return out
}

// checkVarBlocks reports grouped package-level var declarations.
func checkVarBlocks(c *rule.Context, f *fileDecls) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, d := range f.decls {
		gen, ok := d.node.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR || len(gen.Specs) < 2 {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: c.Pos(gen),
			Message:  fmt.Sprintf("var block declares %s", plural(len(gen.Specs), "variable")),
			Help:     "give each package-level variable its own var declaration",
		})
	}
	return out
}

func describe(d member) string {
	switch {
	case d.isMethod:
		return fmt.Sprintf("method %s.%s", d.owner, d.name)
	case d.isFunction() && d.owner != "":
		return fmt.Sprintf("factory %s", d.name)
	case d.isFunction():
		return fmt.Sprintf("func %s", d.name)
	case d.name != "":
		return fmt.Sprintf("%s %s", d.sect, d.name)
	default:
		return d.sect.String()
	}
}
