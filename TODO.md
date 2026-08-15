# TODO

Implementation plan, derived from the three specifications:

- [`docs/directory-organization.md`](docs/directory-organization.md) — `dir/`, 6 rules
- [`docs/file-organization.md`](docs/file-organization.md) — `org/`, 12 rules
- [`docs/logic-organization.md`](docs/logic-organization.md) — `logic/` 7 + `pat/` 2, plus 50 proposed

**55 rules implemented** — 39 syntax-tier, 16 type-tier.

Decisions that bind implementation are recorded in
[`docs/decisions.md`](docs/decisions.md).

Phases are ordered by dependency, cheapest first. Each ends with goorg passing
its own new rules — see [Ground rules](#ground-rules).

---

## Ground rules

These hold for every phase. They are not tasks; they are the conditions any task
is done under.

**gofmt is authoritative on whitespace.** goorg complements `gofmt`, never
competes with it. No rule may report on anything `gofmt` would rewrite, and no
rule's fix may be undone by running `gofmt`. `pat/expand-struct-definition` is
the rule closest to this line: `gofmt` preserves whichever struct form the
author wrote, which is exactly why the rule has room to exist — but that has to
be *proved*, not assumed. See the conformance harness in
[Phase 1](#phase-1--engine-syntax-tier).

**goorg checks goorg, on every push.** The dogfood job runs the working binary
against this repository and must pass. A rule this project cannot live under is
a rule that needs rethinking, so the job is never advisory and is never
`continue-on-error`.

**A rule is not done until it is documented and tested.** `Doc` with a
`Rationale:` and a `To fix:` section, a test proving it fires, and a test
proving it stays silent on conforming code.

**Every phase leaves the tree releasable.** Rules land enabled and dogfooded,
not accumulated behind a flag.

---

## Phase 0 — Decisions — **complete**

**Goal:** settle what blocks code.

Recorded in [`docs/decisions.md`](docs/decisions.md). Summary:

- [x] **Take on `go/types`?** Yes — two tiers, types opt-in, `Tier` field on
      every rule from the first commit. → [D1](docs/decisions.md#d1--two-tiers-type-checking-is-opt-in)
- [x] **Family names.** Four: `dir/`, `org/`, `logic/`, `pat/`.
      `factory-naming` and `expand-struct-definition` moved to `pat/`; the
      renames are applied across all three specifications.
      → [D3](docs/decisions.md#d3--four-rule-families)
- [x] **goorg's own layout.** Domains in both `pkg/` and `cmd/`, `internal: any`.
      → [D2](docs/decisions.md#d2--goorg-adopts-domains-in-both-pkg-and-cmd)
- [x] **Budget numbers.** Measured against 3437 hand-written standard-library
      files rather than guessed. Three defaults revised.
      → [D5](docs/decisions.md#d5--budget-defaults-calibrated-against-a-measured-corpus)
- [x] **Suppression syntax.** `//goorg:ignore <rule-id> — <reason>`, reason
      mandatory. → [D4](docs/decisions.md#d4--inline-suppression-requires-a-reason)
- [x] **`_test.go` scope.** Checked by structure rules, exempt from budgets and
      ordering. → [D6](docs/decisions.md#d6--test-files-are-checked-by-structure-rules-exempt-from-budgets)
- [ ] **The 50 proposals.** Recommendations written — 10 deny, 28 approve,
      12 defer — but **awaiting sign-off**. Blocks Phase 6 only.
      → [D7](docs/decisions.md#d7--recommendations-on-the-50-proposals)

### Still to re-measure

[D5](docs/decisions.md#d5--budget-defaults-calibrated-against-a-measured-corpus)
calibrated against the Go standard library, which is old, unusually low-level,
and written under conventions predating most of the ecosystem. It is a proxy for
"typical Go", not for Quikcad's code.

- [ ] Re-run the measurement against a real Quikcad repository before the first
      release, and revise the defaults if the distributions differ materially
- [ ] Keep the measurement tool in-tree so the numbers stay reproducible

---

## Phase 1 — Engine, syntax tier — **complete**

**Goal:** the engine, shaped for what the specifications require.

- [x] `pkg/lint/diag` — `Diagnostic`, `Position`, `Severity`, `Counts`,
      deterministic sort
- [x] `pkg/source/project` — tree walk, parse, `Project`/`Package`/`File`/`Dir`
      model; parse errors surface as `goorg/parse-error` findings rather than
      aborting the run
- [x] `pkg/lint/rule` — `Rule` with a `Tier` field per
      [D1](docs/decisions.md#d1--two-tiers-type-checking-is-opt-in), `Category`,
      `Context`, and `Set`
- [x] `pkg/lint/runner` — severity resolution, `RuleID`/`Severity` stamping,
      tier gating, suppression application
- [x] `pkg/lint/config` — `.goorg.yaml`, glob severity with specificity
      precedence, per-rule settings as opaque closures, exclude globs; unknown
      keys, rule IDs and severities are hard errors
- [x] `pkg/lint/report` — `text`, `github`, `json`; `auto` resolves to `github`
      under `GITHUB_ACTIONS`
- [x] `internal/cli` — `check`, `rules`, `explain`, `init`, `version`; flags
      accepted in any position; exit codes 0 / 1 / 2
- [x] `cmd/lint/goorg` — thin main
- [x] Suppression per [D4](docs/decisions.md#d4--inline-suppression-requires-a-reason),
      with reasonless directives reported as `goorg/invalid-suppression` and
      unused ones as `goorg/stale-suppression`
- [x] `goorg/` reserved for meta-diagnostics and not suppressible — a broken
      directive cannot hide the report of its own brokenness
- [x] `pkg/lint/ruletest` — fixture harness plus `AssertWellFormed`,
      `AssertDeterministic` and `AssertGofmtStable`
- [x] **gofmt conformance harness**, with a negative test proving it rejects a
      rule that reports on whitespace gofmt rewrites
- [x] CI: `task` runs fmt, vet, test, dogfood and passes

**No global registry.** Rule families export `Rules()` and `internal/cli`
composes them explicitly in `buildRuleSet`. goorg therefore has no package-level
mutable state — the same constraint `org/globals-singleton-only` imposes on
everyone else, met before the rule exists to enforce it.

**Exit criteria met:** `task` passes; `goorg check ./...` runs clean and reports
nothing, because no families are registered yet.

### Deferred out of Phase 1

- [ ] Suppression currently keys on the widest AST node starting on the line
      below the directive. That covers declarations and statements correctly,
      but a directive above a `case` clause or inside a composite literal is
      untested. Revisit once real rules produce findings in those positions.

---

## Phase 2 — `dir/` family — **complete**

**Goal:** all 6 directory rules, enabled on this repository.

- [x] `pkg/rules/directory`, wired into `buildRuleSet`
- [x] `dir/max-entries` — with glob overrides, most specific pattern winning
- [x] `dir/top-level-layout`
- [x] `dir/domain-layout` — `domains` / `packages` / `any` per root
- [x] `dir/max-package-depth` — counts package directories only
- [x] `dir/domain-has-no-go-files`
- [x] `dir/embedded-assets`
- [x] A directory is a package **iff** it contains `.go` files — the definition
      the depth and asset rules share, so an asset directory below a package is
      never mistaken for a subpackage
- [x] `.goorg.yaml` enables all 6; `task dogfood` passes
- [x] `pkg/lint/glob` extracted so rules and config share one path matcher
      without rules importing config

**Ownership split.** `dir/domain-layout` and `dir/domain-has-no-go-files` both
originally fired on a depth-1 directory holding Go files, producing two findings
with contradictory advice for one mistake. They now split on whether packages
sit beneath the directory. A regression test asserts no directory is reported by
more than one rule.

**Exit criteria met:** goorg's own tree passes all 6, and a deliberately
violating tree produces exactly one finding per defect.

---

## Phase 3 — `org/` family, syntax subset — **complete**

**Goal:** 10 of 12 file-organization rules, enabled on this repository.
`org/consumer-locality` and `org/global-file-scoped` are type-tier, phase 5.

- [x] `org/member-order` — sections, per-type contiguity, separate var blocks
- [x] `org/private-functions-last`
- [x] `org/singleton-layout`
- [x] `org/max-functions-per-file`
- [x] `org/max-public-functions`
- [x] `org/max-private-functions`
- [x] `org/type-cohesion`
- [x] `org/interface-own-file`
- [x] `org/globals-singleton-only`
- [x] `org/singleton-instance-func`
- [x] Precedence implemented as shared classifiers rather than per-rule
      special cases: `decls.go` sections every declaration, `filekind.go`
      recognises a singleton file as a distinct shape
- [x] Globals exemption classifiers, including `embed.FS` and the addressed
      composite literal `&T{...}`
- [x] `.goorg.yaml` enables all 10; `task dogfood` passes

**Two spec refinements the implementation forced**, both recorded in
`docs/file-organization.md`:

- A var whose initializer references a local type sinks to that type's section.
  Vars-before-types is the letter of `member-order`; introduce-before-use is its
  point, and `var defaultRule = &Rule{...}` cannot precede `type Rule`.
- Factories are exempt from `private-functions-last`. A factory belongs beside
  its type, and the exported-first split would drag it away.

**Exit criteria met:** goorg passes all 16 rules on itself, and a violating
tree produces one finding per defect.

### Deferred out of Phase 3

- [ ] `org/member-order`'s `grouping: by-kind` mode is accepted in config but
      only `per-type` is enforced. Nothing uses `by-kind` yet.

---

## Phase 4 — `logic/` and `pat/`, syntax subset — **complete**

**Goal:** the four syntax-tier `logic/` rules and both `pat/` rules.

- [x] `pat/expand-struct-definition` — done first, as the sharpest test of the
      gofmt conformance harness
- [x] `logic/max-condition-operands` — including mixed `&&`/`||` without parens
- [x] `logic/max-object-members`
- [x] `logic/iota-candidate`
- [x] `logic/prefer-guard-clause`
- [x] `pat/factory-naming` — `scope: prefixed` only
- [x] `pkg/source/decl` extracted so the enum and factory classifiers exist
      once, shared by `org/member-order`, `logic/iota-candidate`,
      `org/type-cohesion` and `pat/factory-naming`
- [x] `.goorg.yaml` enables all 6; `task dogfood` passes

**Imported result types are skipped** by `pat/factory-naming`, resolving
[logic open question 7](docs/logic-organization.md#open-questions). An imported
interface is syntactically indistinguishable from an imported struct, and
guessing would misname every constructor returning an error-like interface. The
gap is real and recorded; closing it needs the type tier.

**Exit criteria met:** goorg passes all 22 of its own rules.

### Deferred out of Phase 4

- [ ] `logic/max-object-members` counts methods across the package but reports
      at the type declaration. A type whose methods are split across
      build-constrained files is counted once, which is right, but the finding
      does not say which file the excess methods are in.

---

## Phase 5 — Type tier — **complete**

**Goal:** the five type-tier rules and the loader they need.

- [x] `pkg/source/typed` — a `go/packages` loader beside the syntax one
- [x] Tier gating in the runner; a `Types` rule is never invoked without a
      type-checked program
- [x] **Load failure reports skipped coverage and exits 2**, never silent
      success — including per-package failures, so one broken package cannot
      look like a package that passed
- [x] The type tier is only loaded when a type-tier rule is actually enabled
- [x] `--syntax-only`, which still works on a tree that does not compile
- [x] `logic/interface-registry` — near-miss first, with registry packages
      loaded on demand
- [x] `logic/any-should-be-generic`
- [x] `logic/ideal-numeric-type` — `warning`, `allow_narrowing: false`
- [x] `org/global-file-scoped`
- [x] `org/consumer-locality`, narrow form
- [x] Runtime measured and published in
      [D1](docs/decisions.md#d1--two-tiers-type-checking-is-opt-in)

**Measured on goorg itself:** 20 ms syntax-only, 650 ms both tiers, against
475 ms for `go vet ./...`. The type tier costs ~30× the syntax tier and sits in
the same range as vet, which is the honest comparison since both type-check.

**Three rule bugs the dogfood run caught**, all now fixed:

- `org/global-file-scoped` fired on every rule-definition table. It exists to
  force *mutable* state behind an accessor, so it now grants the same
  exemptions as `org/globals-singleton-only` — a definition table, a compiled
  pattern and a sentinel error are none of them mutable.
- `org/consumer-locality` proposed moving `newPackage` out of `package.go`,
  which `org/type-cohesion` forbids. It now skips factories, and skips files
  that export nothing, and skips exported declarations entirely — an exported
  declaration's real consumers are in other packages, which a package-scoped
  analysis cannot see.
- `logic/interface-registry` resolved nothing in a module that did not import
  `fmt`, which is exactly where a missing Stringer hides. Registry packages are
  now loaded on demand.

**Exit criteria met:** both tiers pass on this repository; a deliberately broken
package produces a coverage gap rather than a pass.

### Deferred out of Phase 5

- [ ] `logic/ideal-numeric-type` only examines function parameters. Struct
      fields and package-level variables have the same conversion pressure and
      are not yet counted.
- [ ] `org/consumer-locality` approximates "the target file has room" with a
      declaration count rather than consulting the real budgets. That is
      [Phase 7](#phase-7--the-what-if-pass).

---

## Phase 6 — Approved proposals — **complete**

**Goal:** the 28 rules [D7](docs/decisions.md#d7--disposition-of-the-50-proposals)
approved.

- [x] 17 syntax-tier proposals
- [x] 11 type-tier proposals
- [x] `.goorg.yaml` enables all 55 rules; `task dogfood` passes
- [ ] Document the `golangci-lint` configuration covering the 9 denied
      **exists** rules, so the gap stays deliberate rather than forgotten

**Three limits calibrated rather than guessed**, added to
[D5](docs/decisions.md#d5--budget-defaults-calibrated-against-a-measured-corpus):
`max-function-lines` 80 (p96), `max-nesting-depth` 3 (p95),
`max-function-params` 5 (p97). Nesting at p95 is why it found seven real
offenders in goorg's own code.

**Two rules needed narrowing before they were shippable**, both recorded in
their `Doc`:

- `logic/boolean-field-count` fired on every settings struct. Its rationale is
  that most combinations of flags are invalid — but an options struct is a bag
  of *independent* switches where every combination is legal. Types whose names
  end in Settings, Options, Config or Flags are exempt.
- `logic/enum-missing-string` fired on string-backed enums, which already print
  their own value. Exempt.

**`logic/constraint-too-wide` ships narrower than the proposal described.** The
sketch was "`[T any]` where the body requires `comparable`" — but such code does
not compile, so there is nothing to report. The rule instead reports the
detectable and genuinely wrong shape: `[T any]` whose body asserts the value to
an interface at run time, which is the constraint written in the wrong place.
The general question is undecidable and the `Doc` says so.

**Exit criteria met:** goorg passes all 55 of its own rules. 34 ms syntax-only,
726 ms both tiers.

---

## Phase 7 — The what-if pass — **complete**

**Goal:** `org/consumer-locality`'s full exception mechanism.

- [x] Rules emit a *proposed relocation* instead of a finding
      (`rule.Relocation`, `Context.Propose`)
- [x] `pkg/lint/whatif` applies a relocation to an in-memory project copy and
      re-runs the rules that depend on file membership
- [x] A finding survives only if the total violation count does not rise
- [x] **Cycle guard** — proposals moving declarations both ways between the
      same pair of files are dropped, and counted so the silence is visible
- [x] Rules are re-runnable against a mutated project model; the move is
      textual and both files are reparsed, so every position stays honest
- [x] `--no-what-if` restores the unexamined form

**`rule.Placement` is the piece the specification did not anticipate.** Without
it every relocation is rejected, because moving a declaration to the end of
another file always upsets `org/member-order`. Ordering within the new home is a
separate and always-available fix, so those rules declare `PlacementOrder` and
sit out the comparison. The zero value participates, so a new rule takes part
unless it opts out.

**The rule's own fallback had to stand down.** `max_target_declarations` was
pre-filtering exactly the cases the pass exists to judge, so it now applies only
when the pass is off. `Context.WhatIf()` tells a rule which world it is in.

**Cost:** about 20 ms on goorg itself, on top of 665 ms for both tiers.

### Was it worth building?

Honestly: not yet demonstrated on this codebase. Across phases 5 and 6 the
narrow form's approximation never once suppressed a real finding here, so the
pass has not yet changed an answer outside its own tests. It is built, correct
and tested — but the evidence that a project needs it will come from a
repository with more single-consumer helpers than goorg has.

---

## Phase 8 — Distribution

**Goal:** make it consumable. Most scaffolding already exists and needs
verifying against the real binary rather than writing from scratch.

**Blocked by:** Phase 2 at the earliest — there must be rules worth shipping.

- [ ] Verify `action.yml` end to end against a real release; the asset-name
      template is coupled to `.goreleaser.yaml` and has never been exercised
- [ ] First tagged release through the release workflow
- [ ] Adoption guide: `--fail-on=warning` plus `--max-warnings` as a ratchet for
      codebases that predate the standard
- [ ] Rewrite `README.md`'s rules table — it still lists the nine rules of the
      deleted sketch, none of which survive into these specifications
- [ ] Rewrite `CLAUDE.md`'s architecture and "adding a rule" sections against the
      real engine, including the tier split
- [ ] Publish the standard itself, not just the tool — the three `docs/` files
      are the house standard and are the reason anyone adopts the linter

---

## Not scheduled

Deliberately unplanned. Recorded so the decision is visible rather than
forgotten.

- **Autofix (`--fix`).** `pat/expand-struct-definition` is purely positional
  and could not get it wrong; `logic/iota-candidate` and several control-flow
  proposals are mechanical. goorg currently promises never to modify source, and
  reversing that is a product decision, not a task.
  → [logic open question 6](docs/logic-organization.md#open-questions)
- **A third severity below `warning`.** Heuristic rules — `ideal-numeric-type`,
  `any-should-be-generic`, `single-impl-interface` — are wrong often enough that
  `error` gets them disabled and `warning` gets them ignored. An advisory level
  excluded from the exit code may be the answer.
- **One-type-per-file as an explicit rule.** `org/type-cohesion` plus
  `org/interface-own-file` in `own-file` mode already imply it. Stating it
  directly would be clearer than leaving it emergent from two other rules, and
  would replace both.
- **SARIF output**, for GitHub code scanning. `json` covers the programmatic
  case for now.
