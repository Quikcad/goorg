// Package organization implements the org/ rule family: how code is
// distributed across the files of a package.
//
// Every rule here is syntax-tier. Two of the twelve specified rules —
// org/consumer-locality and org/global-file-scoped — need go/types to resolve
// which identifiers refer to which declaration, and land in phase 5.
//
// Several rules in this family can want opposite things about the same
// declaration. Rather than each rule special-casing the others, they share two
// classifiers:
//
//   - decls.go assigns every top-level declaration a section in the canonical
//     file order, so the ordering rules agree on what a declaration *is*.
//   - filekind.go recognises a singleton file as a distinct shape, which is
//     what lets org/singleton-layout invert the usual exported-first ordering
//     without org/private-functions-last having to know about singletons.
//
// See docs/file-organization.md, including its precedence table.
package organization
