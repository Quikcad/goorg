package rule

// Category groups rules by the kind of consistency they enforce. It is also the
// first segment of every rule ID, so configuration can address a whole family
// at once with a glob such as `dir/*`.
type Category string

const (
	// Directory covers where code lives: tree shape, permitted directories,
	// and how package names relate to their paths.
	Directory Category = "dir"
	// Organization covers how code is distributed across the files of a
	// package: ordering, file budgets, and what belongs together.
	Organization Category = "org"
	// Logic covers how code is shaped: type size, interface satisfaction,
	// numeric modelling, and control-flow complexity.
	Logic Category = "logic"
	// Pattern covers naming conventions and declaration layout.
	Pattern Category = "pat"
)

// Describe returns a one-line description of a category.
func (c Category) Describe() string {
	switch c {
	case Directory:
		return "directory organization — where code lives"
	case Organization:
		return "file organization — how code is split across files"
	case Logic:
		return "logic organization — how code is shaped"
	case Pattern:
		return "pattern correctness — naming and declaration layout"
	default:
		return string(c)
	}
}

// Categories returns every category in documentation order.
func Categories() []Category {
	return []Category{Directory, Organization, Logic, Pattern}
}
