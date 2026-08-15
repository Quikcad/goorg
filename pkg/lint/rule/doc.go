// Package rule defines goorg's rule model and the set that collects rules.
//
// A rule is a plain struct rather than an interface: rules are data plus one
// function, and keeping them as values makes the set trivial to list, document
// and test.
//
// There is deliberately no global registry. Rule families export a Rules()
// function and the CLI composes them explicitly, so goorg has no package-level
// mutable state — the same constraint org/globals-singleton-only imposes on
// everyone else.
//
// Rules never see configuration. A rule returns findings with only Position,
// Message and Help filled in; the runner stamps on RuleID and Severity, and
// per-rule options arrive through Context.Settings as an opaque closure.
package rule
