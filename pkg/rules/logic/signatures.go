package logic

import (
	"fmt"
	"go/ast"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// limitSettings is the shared shape of the three function budgets. Each rule
// decodes its own copy so the limits stay independently configurable.
type limitSettings struct {
	Max int `yaml:"max"`
}

// boolFieldSettings is the configurable surface of logic/boolean-field-count.
type boolFieldSettings struct {
	Max int `yaml:"max"`
	// ExemptSuffixes name the types whose booleans are independent switches
	// rather than a state machine.
	ExemptSuffixes []string `yaml:"exempt_suffixes"`
}

var maxFunctionLines = &rule.Rule{
	ID:       "logic/max-function-lines",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a function body stays within a configured line count",
	Default:  diag.Warning,
	Doc: `A function body may not exceed N lines.

Rationale: length is a blunt measure, which is its virtue — it needs no
judgement to apply and no argument to settle. A function past a screen has
stopped being readable as one idea, and the specific failure is that the reader
cannot see its beginning and end at once, so every claim about what it does has
to be reconstructed rather than seen.

To fix: extract the sections the function already has. A long function almost
always has comment-delimited parts, and those comments are the names of the
functions it should be split into.

This overlaps logic/max-nesting-depth, which measures the same problem more
precisely; length catches the flat-but-endless case that depth misses.

The default of 80 is roughly the standard library's p96 — see
docs/decisions.md D5 for how the budgets are calibrated.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := limitSettings{Max: 80}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.Max <= 0 {
			return nil
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				fn, ok := node.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				lines := c.Project.Position(fn.Body.Rbrace).Line - c.Project.Position(fn.Body.Lbrace).Line
				if lines <= s.Max {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(fn.Name),
					Message:  fmt.Sprintf("%s is %d lines, over the limit of %d", fn.Name.Name, lines, s.Max),
					Help:     "extract the sections it already has; its comments name them",
				})
			}
		}
		return out
	},
}

var maxFunctionParams = &rule.Rule{
	ID:       "logic/max-function-params",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a function takes at most a configured number of parameters",
	Default:  diag.Warning,
	Doc: `A function may take at most N parameters.

	func Fetch(ctx, id, name, kind, limit, offset, sort) error     violation

Rationale: past about four, a call site becomes a positional puzzle. The reader
sees Fetch(ctx, a, b, c, d, e) and cannot tell which argument is which without
opening the signature, and two parameters of the same type can be transposed
without any complaint from the compiler. That transposition is the actual
hazard: it produces a working program that does the wrong thing.

To fix: group the related parameters into a named struct, which makes every
argument labelled at the call site and makes transposition impossible.

A receiver does not count, and neither does a leading context.Context: both are
conventional and carry no ambiguity.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := limitSettings{Max: 5}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.Max <= 0 {
			return nil
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				fn, ok := node.(*ast.FuncDecl)
				if !ok {
					continue
				}
				count := countParams(fn)
				if count <= s.Max {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(fn.Name),
					Message: fmt.Sprintf("%s takes %s, over the limit of %d",
						fn.Name.Name, plural(count, "parameter"), s.Max),
					Help: "group the related ones into a named struct",
				})
			}
		}
		return out
	},
}

var maxReturnValues = &rule.Rule{
	ID:       "logic/max-return-values",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a function returns at most a configured number of values",
	Default:  diag.Warning,
	Doc: `A function may return at most N values, not counting a trailing error.

	func Parse(s string) (int, int, string, bool, error)     violation

Rationale: a caller writing a, b, c, d, err := Parse(s) has to name four values
in the right order with nothing to check them against. Unlike parameters, there
is no way to label them at the call site, so the ordering is pure convention —
and swapping two of the same type is silent. Returning many values is also how a
function admits it is doing several things.

To fix: return a named struct, which labels every field at the call site, or
split the function.

A trailing error is not counted: it is the one return Go has a universal
convention for, and no reader has ever mistaken its position.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := limitSettings{Max: 3}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		if s.Max <= 0 {
			return nil
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				fn, ok := node.(*ast.FuncDecl)
				if !ok {
					continue
				}
				count := countResults(fn)
				if count <= s.Max {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(fn.Name),
					Message: fmt.Sprintf("%s returns %s besides an error, over the limit of %d",
						fn.Name.Name, plural(count, "value"), s.Max),
					Help: "return a named struct so the call site labels each field",
				})
			}
		}
		return out
	},
}

var booleanFieldCount = &rule.Rule{
	ID:       "logic/boolean-field-count",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a struct carries at most a configured number of bool fields",
	Default:  diag.Warning,
	Doc: `A struct may declare at most N boolean fields.

	type Job struct {                violation — eight states, most invalid
		started  bool
		finished bool
		failed   bool
	}

	type Job struct {                OK
		state JobState
	}

Rationale: three booleans are eight states, and most of them are nonsense —
finished and failed both true, or failed without started. Nothing in the type
says which combinations are legal, so every method has to re-derive the answer
and each one may derive it differently. A named state type makes the legal set
explicit and unrepresentable-otherwise.

To fix: replace the flags with a state enum, or with separate types for the
states that carry different data.

Types whose names end in Settings, Options, Config or Flags are exempt. Their
booleans are independent switches — every combination is legal, which is the
opposite of the situation this rule exists to catch.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := boolFieldSettings{Max: 2, ExemptSuffixes: []string{"Settings", "Options", "Config", "Flags"}}
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
				out = append(out, boolFieldFindings(c, gen, &s)...)
			}
		}
		return out
	},
}

// boolFieldFindings reports structs with too many boolean fields.
func boolFieldFindings(c *rule.Context, gen *ast.GenDecl, s *boolFieldSettings) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, spec := range gen.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil || hasAnySuffix(ts.Name.Name, s.ExemptSuffixes) {
			continue
		}
		count := 0
		for _, field := range st.Fields.List {
			if id, ok := field.Type.(*ast.Ident); ok && id.Name == "bool" {
				count += max2(len(field.Names), 1)
			}
		}
		if count <= s.Max {
			continue
		}
		out = append(out, diag.Diagnostic{
			Position: c.Pos(ts.Name),
			Message: fmt.Sprintf("%s has %s, which encode %d states",
				ts.Name.Name, plural(count, "bool field"), 1<<count),
			Help: "replace the flags with a named state type",
		})
	}
	return out
}

// countParams counts a function's parameters, ignoring a leading context.
func countParams(fn *ast.FuncDecl) int {
	if fn.Type.Params == nil {
		return 0
	}
	count := 0
	for i, field := range fn.Type.Params.List {
		names := max2(len(field.Names), 1)
		if i == 0 && isContextType(field.Type) {
			continue
		}
		count += names
	}
	return count
}

// countResults counts a function's results, ignoring a trailing error.
func countResults(fn *ast.FuncDecl) int {
	if fn.Type.Results == nil {
		return 0
	}
	count := 0
	for _, field := range fn.Type.Results.List {
		if id, ok := field.Type.(*ast.Ident); ok && id.Name == "error" {
			continue
		}
		count += max2(len(field.Names), 1)
	}
	return count
}

func isContextType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Context" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "context"
}

// hasAnySuffix reports whether a name ends in one of the given suffixes.
func hasAnySuffix(name string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}
