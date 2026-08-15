// Package logic implements the logic/ rule family: how code is shaped.
//
// Where dir/ constrains where code lives and org/ constrains which file a
// declaration lands in, logic/ constrains the declaration itself — the size of
// a type, the complexity of a condition, the shape of a control-flow branch.
//
// Four of the seven specified rules are here. The other three —
// logic/interface-registry, logic/any-should-be-generic and
// logic/ideal-numeric-type — cannot be answered from syntax and land in
// phase 5. See docs/decisions.md D1.
package logic
