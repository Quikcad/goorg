# TODO

Implementation plan, derived from the three specifications:

- [`docs/directory-organization.md`](docs/directory-organization.md) — `dir/`, 6 rules
- [`docs/file-organization.md`](docs/file-organization.md) — `org/`, 12 rules
- [`docs/logic-organization.md`](docs/logic-organization.md) — `logic/` 7 + `pat/` 2, plus 50 proposed

**27 specified rules: 22 syntax-tier, 5 type-tier.** 6 implemented.

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

## Phase 3 — `org/` family, syntax subset

**Goal:** 10 of 12 file-organization rules. `org/consumer-locality` and
`org/global-file-scoped` are type-tier and wait for Phase 5.

**Blocked by:** Phase 2 (shares the package model).

- [ ] `org/member-order` — needs the enum classifier, shared with
      `logic/iota-candidate`
- [ ] `org/private-functions-last`
- [ ] `org/singleton-layout`
- [ ] `org/max-functions-per-file`
- [ ] `org/max-public-functions`
- [ ] `org/max-private-functions`
- [ ] `org/type-cohesion`
- [ ] `org/interface-own-file`
- [ ] `org/globals-singleton-only`
- [ ] `org/singleton-instance-func`
- [ ] **Implement the precedence table** as a real mechanism, not as per-rule
      special cases. Two conflicts are guaranteed, not hypothetical:
      `singleton-layout` inverts `private-functions-last`, and `type-cohesion`
      is unsatisfiable alongside the file budgets unless methods are excluded
      from them (`count_methods: false`).
      → [Precedence](docs/file-organization.md#precedence)
- [ ] Build the globals exemption classifiers — sentinel errors, interface
      assertions, compiled patterns, lookup tables, `embed.FS`. **The exemption
      list is the rule.** Go has no immutable composite constant, so without
      these the rule fires on unavoidable idiomatic code and gets switched off.
- [ ] Enable all 10 here and fix what they find

**Exit criteria:** goorg's own tree passes all 10; no rule pair produces
contradictory findings on any fixture.

---

## Phase 4 — `logic/` family, syntax subset

**Goal:** 6 of 9 specified logic rules.

**Blocked by:** Phase 1. Independent of Phases 2–3; can run in parallel.

- [ ] `pat/expand-struct-definition` — do this one first. Detection is exact
      (compare the line of `Fields.Opening` and `Fields.Closing`), it has no
      heuristic and no false positives, and it is the sharpest test of the gofmt
      conformance harness.
- [ ] `logic/max-condition-operands`
- [ ] `logic/max-object-members`
- [ ] `logic/iota-candidate` — shares the enum classifier with
      `org/member-order`; build it once
- [ ] `logic/prefer-guard-clause`
- [ ] `pat/factory-naming` — lives in `pkg/rules/pattern`; ship `scope: prefixed` only. `all-factories`
      flags `Parse`, `Open`, `Dial`, `MustCompile` and every other established
      idiom; it stays opt-in and undocumented in `goorg init`.
- [ ] Decide the imported-result-type gap in `factory-naming`: skip, guess, or
      promote the rule to the type tier
      → [logic open question 7](docs/logic-organization.md#open-questions)
- [ ] Enable all 6 here and fix what they find

**Exit criteria:** goorg's own tree passes all 6; gofmt conformance harness
green, specifically for `expand-struct-definition`.

---

## Phase 5 — Type tier

**Goal:** the 5 type-tier rules, and the loader they need.

**Blocked by:** Phase 4. Approved by
[D1](docs/decisions.md#d1--two-tiers-type-checking-is-opt-in).

- [ ] `go/packages` loader alongside the syntax loader; `golang.org/x/tools`
      becomes the first heavy dependency
- [ ] Tier gating: syntax rules always run; type rules run only when loading
      succeeds
- [ ] **Load failure reports skipped coverage, never silent success.** A linter
      that quietly checks nothing is worse than one that refuses to run.
- [ ] `--syntax-only` flag for editor and pre-commit use
- [ ] `logic/interface-registry` — near-miss detection first; it is the half
      that catches real bugs
- [ ] `logic/any-should-be-generic`
- [ ] `logic/ideal-numeric-type` — ships as `warning`, never `error`;
      `allow_narrowing: false` is not negotiable
- [ ] `org/global-file-scoped`
- [ ] `org/consumer-locality`, **narrow form only** — report only where the sole
      consuming file is another file and the target is under every budget with
      headroom. No what-if machinery. See Phase 7.
- [ ] Measure the runtime cost against the syntax tier and publish both numbers

**Exit criteria:** both tiers pass on this repository; a deliberately broken
dependency produces a skipped-coverage warning, not a pass.

---

## Phase 6 — Approved proposals

**Goal:** whichever of the 50 survive Phase 0.

**Blocked by:** sign-off on
[D7](docs/decisions.md#d7--recommendations-on-the-50-proposals); individually,
Phase 4 or 5 by tier. Recommended split: 10 deny, 28 approve, 12 defer.

- [ ] Implement the 17 approved syntax-tier proposals
- [ ] Implement the 11 approved type-tier proposals
- [ ] For each denied proposal tagged **exists**, document the `golangci-lint`
      configuration that covers it instead, so the gap is deliberate and
      recorded rather than forgotten

---

## Phase 7 — The what-if pass

**Goal:** `org/consumer-locality`'s full exception mechanism.

**Blocked by:** Phase 5's narrow form shipping and proving the general
mechanism is worth building. Do not start this before that evidence exists.

- [ ] Rules may emit a *proposed relocation* instead of a finding
- [ ] Engine applies a relocation to an in-memory project copy and re-runs the
      `org/` family
- [ ] Finding survives only if the total violation count does not increase
- [ ] **Cycle guard** — A's sole consumer is in B and B's sole consumer is in A;
      without a guard the two findings each propose a move that creates the other
- [ ] Rules become re-runnable against a mutated project model

→ [The what-if problem](docs/file-organization.md#the-what-if-problem)

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
