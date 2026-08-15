package diag

import "fmt"

// Position locates a diagnostic.
//
// Path is always slash-separated and relative to the project root, so output is
// byte-identical on a laptop and on a CI runner. Line and Col are 1-based; a
// Line of 0 means the diagnostic is about a whole file or directory rather than
// any particular line, which is how the directory-layout rules report.
type Position struct {
	Path string
	Line int
	Col  int
}

// String renders the position in the conventional file:line:col form, omitting
// the parts that are not known.
func (p Position) String() string {
	switch {
	case p.Line == 0:
		return p.Path
	case p.Col == 0:
		return fmt.Sprintf("%s:%d", p.Path, p.Line)
	default:
		return fmt.Sprintf("%s:%d:%d", p.Path, p.Line, p.Col)
	}
}
