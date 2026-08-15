// Package diag defines the diagnostic vocabulary shared by every layer of
// goorg: rules produce diagnostics, reporters render them, and the CLI derives
// its exit code from them.
//
// Nothing in this package imports anything else in goorg. It is the bottom of
// the dependency graph, which is what lets rules stay ignorant of how they were
// configured and how their findings will be displayed.
package diag
