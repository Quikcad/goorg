// Package project loads a Go source tree into the shape goorg's rules care
// about: the directory tree, the package in each directory, and the parsed
// syntax of every Go file.
//
// It deliberately does not type-check. Rules in the syntax tier are about
// layout and source-level patterns, so full type information would cost far
// more than it buys — and a layout linter has to work on a tree that does not
// compile, because that is exactly when someone is mid-refactor and most wants
// it. The type tier is a separate loader; see docs/decisions.md D1.
package project
