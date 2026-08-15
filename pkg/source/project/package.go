package project

import "strings"

// Package is every Go file in a single directory.
//
// Go permits a directory to hold both `foo` and the external test package
// `foo_test`; both land here, and Name always refers to the non-test package.
type Package struct {
	// Dir is the slash-separated directory relative to the project root.
	Dir string
	// Name is the package clause of the non-test files, e.g. "server".
	Name string
	// Files holds every Go file in the directory, sorted by name.
	Files []*File
}

// NonTestFiles returns the files that are not _test.go.
func (p *Package) NonTestFiles() []*File {
	out := make([]*File, 0, len(p.Files))
	for _, f := range p.Files {
		if !f.IsTest {
			out = append(out, f)
		}
	}
	return out
}

// resolveName picks the package clause that identifies the directory,
// preferring a non-test file so an external `foo_test` package never wins.
func (p *Package) resolveName() string {
	for _, f := range p.Files {
		if !f.IsTest {
			return f.Syntax.Name.Name
		}
	}
	for _, f := range p.Files {
		return strings.TrimSuffix(f.Syntax.Name.Name, "_test")
	}
	return ""
}
