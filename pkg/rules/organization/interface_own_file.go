package organization

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// Modes for org/interface-own-file.
const (
	// ModeOwnFile allows one interface per file and no other type.
	ModeOwnFile = "own-file"
	// ModeSeparate lets interfaces share a file with each other but not with
	// struct declarations.
	ModeSeparate = "separate"
	// ModeOff disables the rule.
	ModeOff = "off"
)

// interfaceOwnFileSettings is the configurable surface of the rule.
type interfaceOwnFileSettings struct {
	Mode string `yaml:"mode"`
	// MinMethods exempts small interfaces, which are often best declared
	// beside the consumer that needs them.
	MinMethods int `yaml:"min_methods"`
}

var interfaceOwnFile = &rule.Rule{
	ID:       "org/interface-own-file",
	Category: rule.Organization,
	Tier:     rule.Syntax,
	Summary:  "an interface declaration gets a file of its own",
	Default:  diag.Warning,
	Doc: `An interface declaration gets its own file, as a struct does.

	own-file    one interface per file, and no other type in it
	separate    interfaces may share a file with each other, but not with structs
	off         the rule does not run

Rationale: an interface is a contract, and a contract with a file to itself is
one that can be read, reviewed and diffed without an implementation beside it.
Keeping it separate from any struct also removes the strongest visual cue that
the interface exists *for* that struct — which is the habit that produces
producer-side interfaces with exactly one implementation.

To fix: move the interface into a file named for it, or move the other types
out.

min_methods exempts small interfaces. A single-method interface declared next to
the consumer that needs it is good Go, and forcing it into a file of its own
would fight the more important convention of declaring interfaces where they
are used.

Test files are exempt, per docs/decisions.md D6.

Configure in .goorg.yaml:

	settings:
	  org/interface-own-file:
	    mode: own-file
	    min_methods: 1`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := interfaceOwnFileSettings{Mode: ModeOwnFile, MinMethods: 1}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}
		mode := strings.ToLower(strings.TrimSpace(s.Mode))
		switch mode {
		case ModeOff:
			return nil
		case ModeOwnFile, ModeSeparate:
		default:
			return settingsError(fmt.Errorf("unknown mode %q (want own-file, separate, or off)", s.Mode))
		}

		var out []diag.Diagnostic
		for _, f := range nonTestFiles(c) {
			interfaces, others := splitTypeDecls(f.Syntax, s.MinMethods)
			if len(interfaces) == 0 {
				continue
			}
			switch {
			case len(others) > 0:
				out = append(out, diag.Diagnostic{
					Position: c.Pos(interfaces[0]),
					Message: fmt.Sprintf("interface %s shares a file with %s",
						interfaces[0].Name.Name, plural(len(others), "other type")),
					Help: "give the interface a file of its own",
				})
			case mode == ModeOwnFile && len(interfaces) > 1:
				out = append(out, diag.Diagnostic{
					Position: c.Pos(interfaces[1]),
					Message: fmt.Sprintf("file declares %s",
						plural(len(interfaces), "interface")),
					Help: "one interface per file",
				})
			}
		}
		return out
	},
}

// splitTypeDecls separates a file's interface declarations from its other
// types, ignoring interfaces below the method threshold.
func splitTypeDecls(f *ast.File, minMethods int) (interfaces, others []*ast.TypeSpec) {
	for _, node := range f.Decls {
		gen, ok := node.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			iface, isIface := ts.Type.(*ast.InterfaceType)
			if !isIface {
				others = append(others, ts)
				continue
			}
			if iface.Methods.NumFields() < minMethods {
				continue
			}
			interfaces = append(interfaces, ts)
		}
	}
	return interfaces, others
}
