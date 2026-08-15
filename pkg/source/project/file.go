package project

import (
	"go/ast"
	"strings"
)

// File is a single parsed Go source file.
type File struct {
	// Rel is the slash-separated path relative to the project root.
	Rel string
	// Name is the base file name, e.g. "server_test.go".
	Name string
	// Dir is the slash-separated directory relative to the project root.
	Dir string
	// Syntax is the parsed AST, with comments attached.
	Syntax *ast.File
	// Src is the file's bytes. Suppression scanning needs them to tell a
	// trailing comment from a standalone one, which the AST alone cannot say.
	Src []byte
	// IsTest reports whether the file name ends in _test.go.
	IsTest bool
	// Lines is the total line count of the file on disk.
	Lines int
}

// LineText returns the source of a 1-based line, without its terminator. It
// returns "" when the line is out of range.
func (f *File) LineText(line int) string {
	if line < 1 {
		return ""
	}
	start := 0
	for n := 1; n < line; n++ {
		i := indexByteFrom(f.Src, start, '\n')
		if i < 0 {
			return ""
		}
		start = i + 1
	}
	end := indexByteFrom(f.Src, start, '\n')
	if end < 0 {
		end = len(f.Src)
	}
	if start > end {
		return ""
	}
	return strings.TrimSuffix(string(f.Src[start:end]), "\r")
}

func indexByteFrom(b []byte, from int, c byte) int {
	if from >= len(b) {
		return -1
	}
	i := strings.IndexByte(string(b[from:]), c)
	if i < 0 {
		return -1
	}
	return from + i
}
