# Decisions

Decisions that bind implementation, newest last. Each is numbered and permanent:
superseded decisions are marked, not deleted, so a rule can always be traced
back to the reasoning that shaped it.

This is the output of [Phase 0](../TODO.md#phase-0--decisions).

| # | Decision | Status |
| --- | --- | --- |
| [D1](#d1--two-tiers-type-checking-is-opt-in) | Two tiers; type-checking is opt-in | accepted |
| [D2](#d2--goorg-adopts-domains-in-both-pkg-and-cmd) | goorg adopts domains in both `pkg/` and `cmd/` | accepted |
| [D3](#d3--four-rule-families) | Four rule families: `dir/`, `org/`, `logic/`, `pat/` | accepted |
| [D4](#d4--inline-suppression-requires-a-reason) | Inline suppression requires a reason | accepted |
| [D5](#d5--budget-defaults-calibrated-against-a-measured-corpus) | Budget defaults calibrated against a measured corpus | accepted |
| [D6](#d6--test-files-are-checked-by-structure-rules-exempt-from-budgets) | Test files are checked by structure rules, exempt from budgets | accepted, revisit |
| [D7](#d7--recommendations-on-the-50-proposals) | Disposition of the 50 proposals | accepted |

---

## D1 — Two tiers; type-checking is opt-in

**Decision.** goorg has two rule tiers. Syntax rules use `go/parser` only, run
always, and work on a tree that does not compile. Type rules use `go/packages`
and `go/types`, and run only when the module loads successfully. Every `Rule`
carries a `Tier` field from the first commit.

**Context.** 5 of the 27 specified rules cannot be answered from syntax:
`logic/interface-registry`, `logic/any-should-be-generic`,
`logic/ideal-numeric-type`, `org/consumer-locality`, `org/global-file-scoped`.
Roughly half the 50 proposals are the same.

**Consequences.**

- `golang.org/x/tools` becomes the first heavy dependency, against a current
  footprint of one YAML library.
- `--syntax-only` gives the fast path for editors and pre-commit hooks.
- **A type-tier load failure reports skipped coverage and exits 2.** It never
  silently passes. A linter that quietly checks nothing is worse than one that
  refuses to run, and this is the failure mode that would otherwise let a
  broken CI config look green for months.
- The `Tier` field is declared in Phase 1 and unused until Phase 5.
  Retrofitting it onto a populated 22-rule registry costs far more than
  carrying it unused for three phases.

**Measured, on goorg itself** (~6000 lines, 14 packages, warm cache):

| Run | Time |
| --- | --- |
| `--syntax-only` (22 rules) | **20 ms** |
| both tiers (27 rules) | **650 ms** |
| `go vet ./...`, for scale | 475 ms |

The type tier costs about 30× the syntax tier and lands in the same range as
`go vet` — which is the honest comparison, since both type-check the module.
That is the price of the five rules, and it is why the tier is only loaded when
a type-tier rule is actually enabled: a project that switches them off pays
nothing.

→ [The type-information problem](logic-organization.md#the-type-information-problem)

---

## D2 — goorg adopts domains in both `pkg/` and `cmd/`

**Decision.** This repository is laid out under `dir/domain-layout: domains` for
both roots, with `internal: any`. A domain is a subject area, not the project
name: `pkg/lint/`, not `pkg/goorg/`.

```
cmd/lint/goorg/main.go            thin main, wires os streams into the CLI

pkg/lint/diag/                    Diagnostic, Position, Severity
pkg/lint/rule/                    Rule, Tier, registry, Context
pkg/lint/runner/                  executes enabled rules
pkg/lint/config/                  .goorg.yaml
pkg/lint/report/                  text / github / json renderers

pkg/source/project/               syntax-tier tree model
pkg/source/typed/                 type-tier loader                    (Phase 5)

pkg/rules/directory/              dir/ family
pkg/rules/organization/           org/ family
pkg/rules/logic/                  logic/ family
pkg/rules/pattern/                pat/ family

internal/cli/                     flag parsing, commands, exit codes
internal/buildinfo/               version stamp
```

**Context.** goorg cannot dogfood its own strictest layout rule unless it obeys
it. Under `domains`, the previous `cmd/goorg/main.go` was a violation — a
package directly under a root with no domain layer.

**Consequences.**

- Every `dir/` rule is exercised against real code, not only fixtures.
- The engine is importable, so goorg is embeddable as a library. That is a
  commitment: `pkg/` is public API and breaking it is a major release.
- The CLI stays in `internal/`, so flag parsing and exit-code behaviour are not
  API. `internal: any` keeps it flat, which also exercises the `any` mode.
- Domain directories (`pkg/lint/`, `pkg/rules/`, `cmd/lint/`) contain no Go
  files, per `dir/domain-has-no-go-files`.

**Open.** `cmd/lint/` and `pkg/lint/` share a domain name across two roots.
Legal, and arguably correct — they are the same subject area — but if a second
binary ever appears the `cmd/` domain may want renaming.

---

## D3 — Four rule families

**Decision.** `dir/`, `org/`, `logic/`, `pat/`. Two rules move out of `logic/`:

| Was | Is |
| --- | --- |
| `logic/factory-naming` | `pat/factory-naming` |
| `logic/expand-struct-definition` | `pat/expand-struct-definition` |

**Context.** Rule IDs appear in every consumer's `.goorg.yaml` and are
contractual. Renaming one after release breaks configs silently — the old key
matches no rule, and the rule falls back to its default severity. Settling this
before a single rule ships is the only cheap moment.

**Consequences.**

- `logic/` is now purely about structure and complexity: type size, interface
  satisfaction, numeric modelling, control flow.
- `pat/` absorbs naming conventions and declaration layout.
- Both moved rules stay documented in
  [`logic-organization.md`](logic-organization.md) until a `pat/` document
  exists. Their IDs are already correct there.
- The `logic/` specified count drops from 9 to 7; `pat/` starts at 2. Total is
  still 27.

---

## D4 — Inline suppression requires a reason

**Decision.**

```go
//goorg:ignore org/max-private-functions — parser state machine, splitting hurts
func lex(s string) []token { ... }
```

The directive applies to the declaration or statement it immediately precedes.
A reason is mandatory; a directive without one is itself a finding, reported
under `goorg/invalid-suppression` at `error`.

**Context.** No specification covered suppression, and every linter needs it. The
alternative spellings were `//nolint:` for golangci-lint familiarity, and
config-only exclusion with no inline form at all.

**Consequences.**

- Suppressions are greppable, reviewable, and carry their justification to
  whoever reads the code next. The common failure — a suppression added under
  deadline that nobody can later evaluate — is prevented by construction.
- Not `//nolint:`-compatible. When both tools run, two directive styles coexist.
  Accepted: reusing `//nolint:` would make goorg rule IDs appear in a namespace
  golangci-lint does not recognise, which is worse.
- `goorg/` is reserved as a namespace for the tool's own meta-diagnostics
  (`goorg/parse-error`, `goorg/invalid-suppression`). It is not a rule family
  and cannot be configured.
- Needs an unused-suppression check: a directive that suppresses nothing is
  stale and should be reported. Scheduled with the mechanism in Phase 1.

---

## D5 — Budget defaults calibrated against a measured corpus

**Decision.** Every budget default is set from a measured distribution rather
than guessed. Corpus: the Go standard library, 3437 hand-written files after
excluding 428 generated ones.

| Rule | p50 | p75 | p90 | p95 | Guessed | **Default** | ~% flagged |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `dir/max-entries` | 3 | 9 | 19 | 32 | 20 | **20** | 10% |
| `org/max-functions-per-file` | 2 | 5 | 13 | 21 | 12 | **15** | 8% |
| `org/max-public-functions` | 0 | 1 | 4 | 7 | 5 | **6** | 7% |
| `org/max-private-functions` | 1 | 4 | 9 | 17 | 3 | **5** | 20% |
| `logic/max-object-members` (fields) | 3 | 5 | 9 | 12 | 12 | **12** | 5% |
| `logic/max-object-members` (methods) | 3 | 6 | 12 | 20 | 15 | **15** | 7% |
| `logic/max-condition-operands` | 1 | 1 | 2 | 2 | 4 | **4** | 1% |
| `logic/max-function-lines` | 6 | 16 | 40 | 68 | — | **80** | 4% |
| `logic/max-nesting-depth` | 0 | 1 | 2 | 3 | — | **3** | 5% |
| `logic/max-function-params` | 1 | 2 | 3 | 4 | — | **5** | 3% |

**What the measurement does and does not tell us.** It measures *adoption
cost* — how much typical Go a limit would flag — not *correctness*. For rules
that deliberately depart from ordinary Go practice the corpus is the wrong
authority, and two cases matter:

- **`org/max-private-functions`.** The brief was "severely limited". The corpus
  says p75 is 4, so the guessed limit of 3 would flag over a third of files.
  The standard library freely mixes public API and private implementation in
  one file, which is exactly the habit this rule exists to break — so a high
  flag rate is the rule working, not the rule misconfigured. **5** is proposed
  as the shipping default because a rule that fires on a third of files gets
  switched off in week one; **3** remains defensible if severity is the point.
  This is the one number worth arguing about.
- **`logic/max-condition-operands`.** p99 is 4 and p95 is 2, so a limit of 4
  flags about 1% of conditions. The guess was well calibrated by accident. It
  could tighten to 3 at roughly 2% and still be comfortable.

`org/max-file-lines` appeared in this table until phase 8. It was never
implemented: `logic/max-function-lines` measures the same thing at a finer
grain, and a file budget on top of a function budget would report one problem
twice.

The last three were added in phase 6, measured the same way. `max-nesting-depth`
at 3 sits exactly at p95, which is why it found seven real offenders in goorg's
own code rather than none.

**Caveat.** The standard library is old, unusually low-level, and written under
conventions that predate most of Go's ecosystem. It is a proxy for "typical
Go", not for Quikcad's code. **These defaults should be re-measured against a
real Quikcad repository before the first release** — the measurement tool is
about 200 lines and is worth keeping.

---

## D6 — Test files are checked by structure rules, exempt from budgets

**Decision.** `_test.go` files are subject to `dir/` rules and to naming and
layout rules. They are exempt from every budget rule and from `org/member-order`.

| Rule group | `_test.go` |
| --- | --- |
| `dir/` — all | checked |
| `org/max-*`, `logic/max-object-members` | exempt |
| `org/member-order`, `org/private-functions-last` | exempt |
| `org/type-cohesion`, `org/interface-own-file` | exempt |
| `org/globals-*`, `org/singleton-*` | checked |
| `pat/expand-struct-definition` | checked |
| everything else | checked |

**Context.** The draft exempted test files throughout `org/`, which leaves the
largest files in many packages entirely unchecked. Blanket exemption and blanket
enforcement are both wrong: table-driven tests legitimately carry many small
unexported helpers and would violate `org/max-private-functions` constantly,
but nothing excuses a test file from `pat/expand-struct-definition` or from the
globals rules — a global mutated by tests is precisely why tests cannot run in
parallel.

**Consequences.** Test files can be arbitrarily large and helper-dense, but
cannot introduce package state or ignore the conventions that govern how
declarations are written.

**Revisit** once real findings exist. If test files turn out to be where the
worst structural problems hide, the budget exemptions should become generous
limits rather than absent ones.

---

## D7 — Disposition of the 50 proposals

**Accepted.** 10 denied, 28 approved, 12 deferred. Affects Phase 6 only.

The specified rule count rises from 27 to **55**: 39 syntax-tier, 16 type-tier.

### Denied — already covered by an existing linter (9)

Reimplementing these buys one tool and one config; not reimplementing keeps
goorg focused on what nothing else checks. Recommend denying all nine and
documenting the `golangci-lint` configuration that covers them, so the gap is
deliberate and recorded.

7 `struct-field-alignment` · 23 `enum-switch-exhaustive` ·
32 `cyclomatic-complexity` · 33 `cognitive-complexity` · 34 `else-after-return` ·
43 `naked-return` · 46 `unwrapped-error` · 47 `error-equality` ·
49 `error-string-style`

### Denied — redundant with an already-specified rule (1)

10 `mutable-global` is subsumed by
[`org/globals-singleton-only`](file-organization.md#orgglobals-singleton-only),
which is stricter and already specified. Two rules reporting the same defect
means two findings per violation and two places to configure it.

### Approved — syntax tier, low false-positive rate (17)

Cheap, mechanical, and decidable from the AST. These are the ones worth building.

1 `boolean-field-count` · 2 `stringly-typed-enum` · 9 `pointer-to-slice-or-map` ·
11 `interface-size` · 17 `empty-interface-field` · 24 `enum-zero-value-unnamed` ·
26 `duplicate-const-value` · 31 `max-nesting-depth` · 35 `if-chain-to-switch` ·
36 `negated-condition` · 37 `single-case-switch` · 38 `identical-branches` ·
39 `empty-branch` · 40 `max-function-lines` · 41 `max-function-params` ·
42 `max-return-values` · 45 `boolean-parameter`

### Approved — type tier (11)

Real defects, but gated on [D1](#d1--two-tiers-type-checking-is-opt-in) and
Phase 5. Four of them — 27, 28, 29, 30 — are correctness bugs rather than style,
and are the strongest argument for taking on the type tier at all.

6 `exported-embedded-mutex` · 12 `interface-at-consumer` ·
18 `unused-type-parameter` · 19 `constraint-too-wide` · 22 `enum-missing-string` ·
27 `float-equality` · 28 `lossy-conversion` · 29 `unsigned-underflow` ·
30 `integer-division-to-float` · 48 `panic-outside-main` · 50 `context-in-struct`

### Deferred — too noisy or too judgement-heavy to ship (12)

Not rejected on merit; rejected on false-positive cost. Each would need a
detection strategy sharper than currently sketched before it earns a place.

3 `primitive-obsession` · 4 `duration-as-number` · 5 `map-as-struct` ·
8 `zero-value-unusable` · 13 `single-impl-interface` ·
14 `accept-interfaces-return-structs` · 15 `unused-interface-method` ·
16 `interface-name-suffix` · 20 `single-instantiation-generic` ·
21 `reflection-over-generics` · 25 `magic-number` · 44 `loop-invariant-computation`

**Totals:** 10 denied, 28 approved (17 syntax + 11 types), 12 deferred.

### Unreconciled: D7 versus the ticked list

The checkbox list in [`logic-organization.md`](logic-organization.md#proposed-rules)
was ticked independently and **approves 35 rules, not 28**. The two disagree on
19 of the 50. D7 is accepted as the recommendation of record, but the ticked
list is a real signal and has not been overwritten.

| Delta | Rules | Note |
| --- | --- | --- |
| Ticked, D7 denied | 23, 33, 34, 43, 46, 47, 49 | All seven are the `golangci-lint` overlap. Ticking them means goorg reimplements `exhaustive`, `gocognit`, `revive`, `nakedret`, `errorlint` ×2 and `staticcheck ST1005`. |
| Ticked, D7 deferred | 3, 4, 8, 14, 20, 44 | Accepted on merit, deferred on false-positive cost. Ticking them means building them anyway, and each needs a sharper detection strategy first. |
| Unticked, D7 approved | 11, 22, 24, 28, 29, 45 | Includes 28 `lossy-conversion` and 29 `unsigned-underflow`, which are correctness bugs rather than style. |

**Blocks Phase 6 only.** Reconcile before that phase starts; nothing earlier
depends on it.

Deferred rules are not rejected on merit — each needs a sharper detection
strategy than currently sketched before it earns a place. They stay in
[`logic-organization.md`](logic-organization.md) as proposals.
