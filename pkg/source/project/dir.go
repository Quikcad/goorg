package project

// Dir is a directory in the tree, whether or not it holds Go source.
//
// Layout rules need to see Go-free directories too: a stray directory is a
// finding even when it is empty, and an asset directory is defined precisely by
// containing no Go files.
type Dir struct {
	// Rel is the slash-separated path relative to the project root. The root
	// itself is ".".
	Rel string
	// Name is the base name of the directory.
	Name string
	// Depth is the number of path segments below the root; the root is 0.
	Depth int
	// HasGo reports whether the directory directly contains Go source. This is
	// the definition of "is a package" that the depth and asset rules share.
	HasGo bool
	// Entries is the number of immediate children, files and subdirectories
	// together, after exclusions are applied.
	Entries int
	// NonGoFiles are the base names of the directory's immediate files that
	// are not Go source, sorted. dir/embedded-assets reads these: a non-Go
	// file beside .go source is an asset that belongs in a subdirectory.
	NonGoFiles []string
}
