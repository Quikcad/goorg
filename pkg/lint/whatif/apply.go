// Package whatif answers whether moving a declaration would break something.
//
// A rule that wants to say "put this somewhere else" cannot know the answer:
// the destination file has budgets, and the declaration has a type it may need
// to stay beside. Rather than have the rule read five other rules' settings and
// go stale when a sixth is added, the engine makes the move on an in-memory
// copy and re-runs the rules that care. The proposal survives only if nothing
// got worse.
//
// See docs/file-organization.md, "The what-if problem".
package whatif

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// Apply returns a copy of the project with one declaration moved.
//
// The move is textual: the declaration's source, including its doc comment, is
// cut from one file and appended to the other, and both are reparsed. Working
// on source rather than on the syntax tree keeps every position honest, which
// matters because the rules being re-run report positions.
//
// The declaration is appended rather than inserted at its correct place in the
// destination. Placement within the new file is deliberately not modelled —
// see Evaluate, which excludes the rules that would care.
func Apply(p *project.Project, r rule.Relocation) (*project.Project, error) {
	from := p.FileAt(r.From)
	to := p.FileAt(r.To)
	if from == nil || to == nil {
		return nil, fmt.Errorf("relocation refers to a file goorg did not load")
	}

	block, remainder, err := cut(p, from, r.Line)
	if err != nil {
		return nil, err
	}
	merged := append(append([]byte(nil), to.Src...), append([]byte("\n"), block...)...)

	return reparse(p, map[string][]byte{
		from.Rel: remainder,
		to.Rel:   merged,
	})
}

// cut extracts the declaration beginning on a line, returning it and what is
// left of the file.
func cut(p *project.Project, f *project.File, line int) (block, remainder []byte, err error) {
	for _, decl := range f.Syntax.Decls {
		start := declStart(decl)
		if p.Position(start).Line != line && p.Position(decl.Pos()).Line != line {
			continue
		}
		lo := p.Fset.Position(start).Offset
		hi := p.Fset.Position(decl.End()).Offset
		if lo < 0 || hi > len(f.Src) || lo >= hi {
			return nil, nil, fmt.Errorf("declaration at %s:%d has no usable extent", f.Rel, line)
		}
		block = append([]byte(nil), f.Src[lo:hi]...)
		remainder = append(append([]byte(nil), f.Src[:lo]...), f.Src[hi:]...)
		return block, remainder, nil
	}
	return nil, nil, fmt.Errorf("no declaration begins at %s:%d", f.Rel, line)
}

// declStart returns the position a declaration begins at, including the doc
// comment, which is part of what moves with it.
func declStart(decl ast.Decl) token.Pos {
	switch d := decl.(type) {
	case *ast.GenDecl:
		if d.Doc != nil {
			return d.Doc.Pos()
		}
	case *ast.FuncDecl:
		if d.Doc != nil {
			return d.Doc.Pos()
		}
	}
	return decl.Pos()
}

// reparse rebuilds the affected packages of a project from mutated source.
//
// Everything else — the directory tree, the untouched packages — is carried
// over, so the copy stays a faithful model of the project rather than a
// stripped-down one.
func reparse(p *project.Project, changed map[string][]byte) (*project.Project, error) {
	out := &project.Project{
		Root:        p.Root,
		Module:      p.Module,
		Fset:        token.NewFileSet(),
		Dirs:        p.Dirs,
		ParseErrors: p.ParseErrors,
	}

	for _, pkg := range p.Packages {
		rebuilt := &project.Package{Dir: pkg.Dir, Name: pkg.Name}
		for _, f := range pkg.Files {
			src, mutated := changed[f.Rel]
			if !mutated {
				src = f.Src
			}
			replacement, err := parseInto(out.Fset, p.Root, f, src)
			if err != nil {
				return nil, err
			}
			// A file emptied of every declaration still holds its package
			// clause, so it stays part of the package.
			rebuilt.Files = append(rebuilt.Files, replacement)
		}
		out.Packages = append(out.Packages, rebuilt)
	}
	return out, nil
}

// parseInto reparses one file's source against a fresh file set.
func parseInto(fset *token.FileSet, root string, f *project.File, src []byte) (*project.File, error) {
	syntax, err := parser.ParseFile(fset, root+"/"+f.Rel, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("reparse %s after the move: %w", f.Rel, err)
	}
	return &project.File{
		Rel:    f.Rel,
		Name:   f.Name,
		Dir:    f.Dir,
		Syntax: syntax,
		Src:    src,
		IsTest: f.IsTest,
		Lines:  strings.Count(string(src), "\n") + 1,
	}, nil
}
