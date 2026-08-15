// Package directory implements the dir/ rule family: where code lives.
//
// Every rule here is syntax-tier — they read paths and package clauses, never
// types — so they work on a tree that does not compile.
//
// The load-bearing definition shared across the family is that **a directory is
// a package if and only if it contains .go files**. Everything else is an asset
// directory, which is exempt from the depth rules precisely because it holds no
// Go code. Getting this wrong makes dir/max-package-depth and
// dir/embedded-assets contradict each other: the second says to move assets
// into a subdirectory, and the first would then call that subdirectory too deep.
//
// See docs/directory-organization.md.
package directory
