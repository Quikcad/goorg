// Command measure computes the distributions goorg's budget rules need
// defaults for, over an arbitrary Go source corpus.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type series struct {
	name   string
	values []int
}

func (s *series) add(v int) { s.values = append(s.values, v) }

func (s *series) report() string {
	if len(s.values) == 0 {
		return fmt.Sprintf("%-28s (no data)", s.name)
	}
	sort.Ints(s.values)
	p := func(q float64) int {
		i := int(q * float64(len(s.values)-1))
		return s.values[i]
	}
	sum := 0
	for _, v := range s.values {
		sum += v
	}

	return fmt.Sprintf("%-28s n=%-6d mean=%-6.1f p50=%-4d p75=%-4d p90=%-4d p95=%-4d p99=%-4d max=%d",
		s.name, len(s.values), float64(sum)/float64(len(s.values)),
		p(0.50), p(0.75), p(0.90), p(0.95), p(0.99), s.values[len(s.values)-1])
}

// rate is the shape a spacing convention needs instead of a percentile. A
// budget asks "how big do people let this get"; a convention asks "how often do
// people already do it", and the answer sets the adoption cost directly.
type rate struct {
	name       string
	total      int
	conforming int
}

func (r *rate) add(ok bool) {
	r.total++
	if ok {
		r.conforming++
	}
}

func (r *rate) report() string {
	if r.total == 0 {
		return fmt.Sprintf("%-28s (no data)", r.name)
	}
	return fmt.Sprintf("%-28s n=%-6d already spaced=%-6d (%.1f%%)  would flag=%d",
		r.name, r.total, r.conforming,
		100*float64(r.conforming)/float64(r.total), r.total-r.conforming)
}

//goorg:ignore logic/max-function-lines — a measurement script; splitting it would hide the metric list
func main() {
	root := os.Args[1]

	funcsPerFile := &series{name: "functions/file"}
	exportedPerFile := &series{name: "exported functions/file"}
	privateWithExports := &series{name: "unexported fns/file w/ exports"}
	methodsPerFile := &series{name: "methods/file"}
	linesPerFile := &series{name: "lines/file"}
	fieldsPerStruct := &series{name: "fields/struct"}
	methodsPerType := &series{name: "methods/type"}
	dirEntries := &series{name: "entries/directory"}
	condOperands := &series{name: "operands/condition"}
	varBlocks := &series{name: "package vars/file"}
	funcLines := &series{name: "lines/function"}
	nesting := &series{name: "nesting depth/function"}
	params := &series{name: "params/function"}
	guardBoundary := &rate{name: "guard prologue -> body"}
	resultBoundary := &rate{name: "body -> multi-line result"}

	var generated int
	fset := token.NewFileSet()
	// methods[dir][typeName] counts methods per type within a package.
	methods := map[string]map[string]int{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			entries, rerr := os.ReadDir(path)
			if rerr == nil {
				n := 0
				for _, e := range entries {
					if strings.HasPrefix(e.Name(), ".") {
						continue
					}
					n++
				}
				dirEntries.add(n)
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		// Generated files are not written by hand and must not influence a
		// budget meant for code people maintain.
		if isGenerated(src) {
			generated++
			return nil
		}
		f, perr := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if perr != nil {
			return nil
		}

		linesPerFile.add(strings.Count(string(src), "\n") + 1)

		dir := filepath.Dir(path)
		if methods[dir] == nil {
			methods[dir] = map[string]int{}
		}

		var fns, exported, unexported, meths, vars int
		for _, decl := range f.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Body != nil {
					open := fset.Position(decl.Body.Lbrace).Line
					shut := fset.Position(decl.Body.Rbrace).Line
					funcLines.add(shut - open)
					nesting.add(blockDepth(decl.Body, 0))
					params.add(decl.Type.Params.NumFields())
				}
				if decl.Recv != nil {
					meths++
					if tn := receiverTypeName(decl.Recv.List[0].Type); tn != "" {
						methods[dir][tn]++
					}
					continue
				}
				fns++
				if decl.Name.IsExported() {
					exported++
				} else {
					unexported++
				}
			case *ast.GenDecl:
				if decl.Tok == token.VAR {
					vars += len(decl.Specs)
				}
			}
		}
		funcsPerFile.add(fns)
		exportedPerFile.add(exported)
		methodsPerFile.add(meths)
		varBlocks.add(vars)
		if exported > 0 {
			privateWithExports.add(unexported)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				scanSections(fset, n.Body, guardBoundary, resultBoundary)
			case *ast.FuncLit:
				scanSections(fset, n.Body, guardBoundary, resultBoundary)
			case *ast.StructType:
				count := 0
				for _, field := range n.Fields.List {
					if len(field.Names) == 0 {
						count++ // embedded
						continue
					}
					count += len(field.Names)
				}
				if count > 0 {
					fieldsPerStruct.add(count)
				}
			case *ast.IfStmt:
				if n.Cond != nil {
					condOperands.add(countOperands(n.Cond))
				}
			case *ast.ForStmt:
				if n.Cond != nil {
					condOperands.add(countOperands(n.Cond))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for _, byType := range methods {
		for _, n := range byType {
			methodsPerType.add(n)
		}
	}

	fmt.Printf("corpus: %s (%d generated files skipped)\n\n", root, generated)
	for _, s := range []*series{
		funcsPerFile, exportedPerFile, privateWithExports, methodsPerFile,
		linesPerFile, fieldsPerStruct, methodsPerType, dirEntries,
		condOperands, varBlocks, funcLines, nesting, params,
	} {
		fmt.Println(s.report())
	}
	fmt.Println()
	for _, r := range []*rate{guardBoundary, resultBoundary} {
		fmt.Println(r.report())
	}
}

// scanSections counts the section boundaries logic/section-spacing asks for and
// how many the corpus already draws.
//
// The classification is restated here rather than imported: the rule's helpers
// are unexported, and the tool deliberately depends on nothing but go/ast so a
// measurement stays reproducible against a goorg that has moved on. Keep the
// two definitions in step — see pkg/rules/logic/section_spacing.go.
func scanSections(fset *token.FileSet, body *ast.BlockStmt, guards, results *rate) {
	if body == nil || len(body.List) == 0 {
		return
	}
	line := func(p token.Pos) int { return fset.Position(p).Line }
	spaced := func(i int) bool { return line(body.List[i].Pos())-line(body.List[i-1].End()) > 1 }
	span := func(s ast.Stmt) int { return line(s.End()) - line(s.Pos()) + 1 }

	n := 0
	for _, stmt := range body.List {
		if !isGuardClause(stmt) {
			break
		}
		n++
	}
	counted := -1
	if n >= 2 && n < len(body.List) && !(len(body.List)-n == 1 && span(body.List[n]) < 2) {
		guards.add(spaced(n))
		counted = n
	}

	at := len(body.List) - 1
	if len(body.List) <= 3 || at == counted {
		return
	}
	if result, ok := body.List[at].(*ast.ReturnStmt); ok && span(result) >= 3 {
		results.add(spaced(at))
	}
}

// isGuardClause reports whether a statement is a conditional early exit.
func isGuardClause(stmt ast.Stmt) bool {
	cond, ok := stmt.(*ast.IfStmt)
	if !ok || cond.Else != nil || cond.Body == nil {
		return false
	}
	list := cond.Body.List
	if len(list) == 0 {
		return false
	}
	switch last := list[len(list)-1].(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return true
	case *ast.ExprStmt:
		call, ok := last.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		name, ok := call.Fun.(*ast.Ident)
		return ok && name.Name == "panic"
	default:
		return false
	}
}

// blockDepth returns the deepest block nesting inside a block.
func blockDepth(block *ast.BlockStmt, depth int) int {
	deepest := depth
	for _, stmt := range block.List {
		var inner *ast.BlockStmt
		switch s := stmt.(type) {
		case *ast.IfStmt:
			inner = s.Body
		case *ast.ForStmt:
			inner = s.Body
		case *ast.RangeStmt:
			inner = s.Body
		case *ast.BlockStmt:
			inner = s
		}
		if inner == nil {
			continue
		}
		if d := blockDepth(inner, depth+1); d > deepest {
			deepest = d
		}
	}
	return deepest
}

// isGenerated reports whether a file carries the conventional generated-code
// marker, which by convention appears before the package clause.
func isGenerated(src []byte) bool {
	head := src
	if len(head) > 4096 {
		head = head[:4096]
	}
	for _, line := range strings.Split(string(head), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			return false
		}
		if strings.HasPrefix(line, "// Code generated") && strings.Contains(line, "DO NOT EDIT") {
			return true
		}
	}
	return false
}

// countOperands counts the leaves of a boolean expression tree joined by
// && and ||.
func countOperands(e ast.Expr) int {
	switch e := e.(type) {
	case *ast.BinaryExpr:
		if e.Op == token.LAND || e.Op == token.LOR {
			return countOperands(e.X) + countOperands(e.Y)
		}
		return 1
	case *ast.ParenExpr:
		return countOperands(e.X)
	default:
		return 1
	}
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	case *ast.IndexListExpr:
		return receiverTypeName(t.X)
	default:
		return ""
	}
}
