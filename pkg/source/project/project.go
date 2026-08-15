package project

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
)

// ParseErrorRule is the meta-diagnostic ID for a file goorg could not parse.
// The goorg/ namespace is reserved for the tool's own diagnostics and is not a
// configurable rule family.
const ParseErrorRule = "goorg/parse-error"

// alwaysSkip are directories that are never part of a project's own source, so
// they are pruned before any user-configured exclusion is consulted.
var alwaysSkip = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

// Options controls loading.
type Options struct {
	// Exclude reports whether a root-relative slash path should be skipped.
	// Returning true for a directory prunes the whole subtree. It may be nil.
	Exclude func(rel string) bool
}

// Project is a loaded source tree.
type Project struct {
	// Root is the absolute path to the project root.
	Root string
	// Module is the module path from go.mod, or "" if there is none.
	Module string
	// Fset positions every parsed file.
	Fset *token.FileSet
	// Dirs holds every scanned directory, sorted by path.
	Dirs []*Dir
	// Packages holds every directory containing Go source, sorted by path.
	Packages []*Package
	// ParseErrors holds syntax errors found while loading. They are surfaced
	// as diagnostics rather than aborting the run, so one broken file does not
	// hide findings in the rest of the tree.
	ParseErrors []diag.Diagnostic
}

// Load walks root and parses every Go file that survives filtering.
func Load(root string, opts Options) (*Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("read root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %s is not a directory", root)
	}

	p := &Project{Root: abs, Fset: token.NewFileSet(), Module: readModulePath(abs)}
	byDir := map[string]*Package{}

	err = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(abs, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if entry.IsDir() {
			return p.visitDir(rel, path, entry.Name(), opts)
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		if opts.Exclude != nil && opts.Exclude(rel) {
			return nil
		}
		return p.parseFile(rel, path, entry.Name(), byDir)
	})
	if err != nil {
		return nil, err
	}

	p.index(byDir)
	return p, nil
}

// Position converts an AST position into a root-relative diagnostic position.
func (p *Project) Position(pos token.Pos) diag.Position {
	if !pos.IsValid() {
		return diag.Position{}
	}
	tp := p.Fset.Position(pos)
	rel, err := filepath.Rel(p.Root, tp.Filename)
	if err != nil {
		rel = tp.Filename
	}
	return diag.Position{Path: filepath.ToSlash(rel), Line: tp.Line, Col: tp.Column}
}

// Files returns every parsed file across every package, sorted by path.
func (p *Project) Files() []*File {
	var out []*File
	for _, pkg := range p.Packages {
		out = append(out, pkg.Files...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out
}

// FileAt returns the file at a root-relative path, or nil.
func (p *Project) FileAt(rel string) *File {
	for _, pkg := range p.Packages {
		for _, f := range pkg.Files {
			if f.Rel == rel {
				return f
			}
		}
	}
	return nil
}

// visitDir records a directory and decides whether to descend into it.
func (p *Project) visitDir(rel, path, name string, opts Options) error {
	if rel == "." {
		entries, nonGo := scanDir(path, rel, opts)
		p.Dirs = append(p.Dirs, &Dir{
			Rel: ".", Name: filepath.Base(p.Root), Entries: entries, NonGoFiles: nonGo,
		})
		return nil
	}
	// The Go toolchain itself ignores directories beginning with "." or "_",
	// so a layout linter must too.
	if alwaysSkip[name] || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return filepath.SkipDir
	}
	if opts.Exclude != nil && opts.Exclude(rel) {
		return filepath.SkipDir
	}
	entries, nonGo := scanDir(path, rel, opts)
	p.Dirs = append(p.Dirs, &Dir{
		Rel:        rel,
		Name:       name,
		Depth:      strings.Count(rel, "/") + 1,
		Entries:    entries,
		NonGoFiles: nonGo,
	})
	return nil
}

// parseFile parses one Go file into the package for its directory.
func (p *Project) parseFile(rel, path, name string, byDir map[string]*Package) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", rel, err)
	}
	syntax, parseErr := parser.ParseFile(p.Fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if parseErr != nil {
		p.ParseErrors = append(p.ParseErrors, parseDiagnostic(rel, parseErr))
		// A file that will not parse has no usable AST, so it cannot
		// contribute to any rule.
		return nil
	}

	dir := filepath.ToSlash(filepath.Dir(rel))
	pkg, ok := byDir[dir]
	if !ok {
		pkg = &Package{Dir: dir}
		byDir[dir] = pkg
	}
	pkg.Files = append(pkg.Files, &File{
		Rel:    rel,
		Name:   name,
		Dir:    dir,
		Syntax: syntax,
		Src:    src,
		IsTest: strings.HasSuffix(name, "_test.go"),
		Lines:  countLines(src),
	})
	return nil
}

// index finalizes the loaded tree: names packages, sorts everything, and marks
// which directories hold Go source.
func (p *Project) index(byDir map[string]*Package) {
	goDirs := map[string]bool{}
	for dir, pkg := range byDir {
		pkg.Name = pkg.resolveName()
		sort.Slice(pkg.Files, func(i, j int) bool { return pkg.Files[i].Name < pkg.Files[j].Name })
		p.Packages = append(p.Packages, pkg)
		goDirs[dir] = true
	}
	sort.Slice(p.Packages, func(i, j int) bool { return p.Packages[i].Dir < p.Packages[j].Dir })
	sort.Slice(p.Dirs, func(i, j int) bool { return p.Dirs[i].Rel < p.Dirs[j].Rel })
	for _, d := range p.Dirs {
		d.HasGo = goDirs[d.Rel]
	}
	diag.Sort(p.ParseErrors)
}

// scanDir reads a directory's immediate children once, returning both the
// entry count and the non-Go file names.
//
// Entries skip what the walker would never descend into, so the count matches
// what a reader sees in a listing rather than what is on disk.
func scanDir(path, rel string, opts Options) (entries int, nonGo []string) {
	children, err := os.ReadDir(path)
	if err != nil {
		return 0, nil
	}
	for _, c := range children {
		name := c.Name()
		if alwaysSkip[name] || strings.HasPrefix(name, ".") {
			continue
		}
		childRel := name
		if rel != "." {
			childRel = rel + "/" + name
		}
		if opts.Exclude != nil && opts.Exclude(childRel) {
			continue
		}
		entries++
		if !c.IsDir() && !strings.HasSuffix(name, ".go") {
			nonGo = append(nonGo, name)
		}
	}
	sort.Strings(nonGo)
	return entries, nonGo
}

func countLines(src []byte) int {
	if len(src) == 0 {
		return 0
	}
	n := strings.Count(string(src), "\n")
	if src[len(src)-1] != '\n' {
		n++
	}
	return n
}

// parseDiagnostic converts a go/parser error into a diagnostic. The parser
// reports absolute paths, so the message is rebuilt root-relative.
func parseDiagnostic(rel string, err error) diag.Diagnostic {
	d := diag.Diagnostic{
		Position: diag.Position{Path: rel, Line: 1},
		RuleID:   ParseErrorRule,
		Severity: diag.Error,
		Message:  "file could not be parsed",
		Help:     "goorg cannot check a file it cannot parse; fix the syntax error or exclude the file",
	}
	if msg, line, col := firstParseError(err.Error()); msg != "" {
		d.Message, d.Line, d.Col = msg, line, col
	}
	return d
}

// firstParseError extracts the message and position of the first error in a
// go/parser error list, which is formatted as "path:line:col: message".
func firstParseError(s string) (msg string, line, col int) {
	first, _, _ := strings.Cut(s, "\n")
	parts := strings.SplitN(first, ":", 4)
	if len(parts) < 4 {
		return strings.TrimSpace(first), 1, 0
	}
	fmt.Sscanf(parts[1], "%d", &line)
	fmt.Sscanf(parts[2], "%d", &col)
	if line == 0 {
		line = 1
	}
	return strings.TrimSpace(parts[3]), line, col
}

// readModulePath returns the module path declared in root/go.mod, or "".
func readModulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path)
		}
	}
	return ""
}
