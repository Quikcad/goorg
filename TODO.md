# TODO

Implementation plan, derived from the three specifications:

- [`docs/directory-organization.md`](docs/directory-organization.md) — `dir/`, 6 rules
- [`docs/file-organization.md`](docs/file-organization.md) — `org/`, 12 rules
- [`docs/logic-organization.md`](docs/logic-organization.md) — `logic/`, 9 specified + 50 proposed

**27 specified rules: 22 syntax-tier, 5 type-tier.**

Phases are ordered by dependency, cheapest first. Each ends with goorg passing
its own new rules — see [Ground rules](#ground-rules).

---

## Ground rules

These hold for every phase. They are not tasks; they are the conditions any task
is done under.

**gofmt is authoritative on whitespace.** goorg complements `gofmt`, never
competes with it. No rule may report on anything `gofmt` would rewrite, and no
rule's fix may be undone by running `gofmt`. `logic/expand-struct-definition` is
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

## Phase 0 — Decisions

**Goal:** settle what blocks code. Everything below is a decision, not an
implementation.

**Blocked by:** nothing. Blocks everything.

- [ ] **Do we take on `go/types`?** Determines whether 5 of 27 specified rules
      and roughly half the proposals are buildable at all. Recommended: two
      explicit tiers, syntax always, types opt-in.
      → [The type-information problem](docs/logic-organization.md#the-type-information-problem)
- [ ] **Settle family names before any rule ships.** Rule IDs appear in every
      consumer's `.goorg.yaml` and are contractual. Open specifically:
      is `logic/` right alongside `dir/`/`org/`/`pat/`, and do
      `logic/factory-naming` (a naming convention) and
      `logic/expand-struct-definition` (declaration layout) belong in `pat/`?
      Moving them is free now and breaking later.
- [ ] **Decide goorg's own repository layout, under goorg's own `dir/` rules.**
      This is not cosmetic — see [the dogfooding
      conflict](#the-dogfooding-conflict) below.
- [ ] **Decide the budget numbers.** Every limit in all three documents is a
      guess (12 functions/file, 5 exported, 3 unexported, 20 directory entries,
      12 struct fields). Measure against real Quikcad repositories first: a
      limit below the existing median makes a rule unadoptable.
- [ ] **Approve or deny the 50 logic proposals.** Only affects Phase 6.
      → [Proposed rules](docs/logic-organization.md#proposed-rules)
- [ ] **Decide the suppression syntax.** No document specifies how to silence a
      finding on one line, and every linter needs it. Proposal:
      `//goorg:ignore <rule-id> — <reason>`, reason mandatory.
- [ ] **Decide whether `_test.go` files are in scope.** Drafted as exempt
      throughout `org/`, which leaves the largest files in many packages
      unchecked.
      → [file-organization open question 7](docs/file-organization.md#open-questions)

### The dogfooding conflict

goorg cannot dogfood `dir/domain-layout: domains` unless goorg itself adopts
domains. Under that mode `cmd/goorg/main.go` is a violation — a package sitting
directly under `cmd/` with no domain layer. The options:

- **Adopt domains in this repo** — `cmd/tooling/goorg/main.go`. goorg genuinely
  dogfoods its own strictest layout rule.
- **Configure `cmd: packages` for this repo** — the current tree stays, and the
  `domains` path is never exercised by dogfooding. It then has to be covered by
  fixtures alone.

Same question for `internal/` versus `pkg/`: putting the rule engine in `pkg/`
makes goorg embeddable as a library and exercises the domain rules; keeping it
in `internal/` keeps the API surface at zero. Decide both before Phase 1, since
the answer determines where every file goes.

---

## Phase 1 — Engine, syntax tier

**Goal:** rebuild the engine deleted earlier, shaped for what the three
documents now require rather than for the original nine-rule sketch.

**Blocked by:** Phase 0 layout and family-name decisions.

- [ ] Apply the Phase 0 layout decision; module skeleton in place
- [ ] `diag` — `Diagnostic`, `Position`, `Severity`, deterministic sort, counts
- [ ] `project` — tree walk, parse, `Project`/`Package`/`File`/`Dir` model,
      parse errors surfaced as findings rather than aborting the run
- [ ] `rules` — registry, `Rule` struct **carrying a `Tier` field from day one**
      (`syntax` | `types`), even though only `syntax` exists yet. Retrofitting a
      tier onto a populated registry is far more expensive than declaring it
      unused for three phases.
- [ ] `runner` — severity resolution, `RuleID`/`Severity` stamping, rules stay
      ignorant of configuration
- [ ] `config` — `.goorg.yaml`, glob severity with specificity precedence,
      per-rule settings as opaque closures, exclude globs. Unknown keys, unknown
      rule IDs and unknown severities are hard errors.
- [ ] `report` — `text`, `github`, `json`; `auto` resolves to `github` under
      `GITHUB_ACTIONS`
- [ ] CLI — `check`, `rules`, `explain`, `init`, `version`; flags accepted in any
      position; exit codes **0 clean / 1 findings / 2 could-not-run**
- [ ] Suppression comments, per the Phase 0 decision
- [ ] Rule test harness — in-memory file map → temp tree → findings, plus the
      registry well-formedness and determinism meta-tests
- [ ] **gofmt conformance harness** — for every rule fixture, run `gofmt` over it
      and assert the finding set is unchanged. This is what makes "complements
      gofmt" enforceable instead of aspirational, and it must exist before the
      first rule lands, not after.
- [ ] CI: restore `task dogfood`; wire the gofmt conformance harness into `task`

**Exit criteria:** `task` passes; `goorg check ./...` runs and reports nothing,
because no rules are registered yet.

---

## Phase 2 — `dir/` family

**Goal:** all 6 directory rules. Cheapest family, entirely syntactic, and the
one that constrains this repository's own shape.

**Blocked by:** Phase 1.

- [ ] `dir/max-entries`
- [ ] `dir/top-level-layout`
- [ ] `dir/domain-layout` — the `domains` / `packages` / `any` mode, per root
- [ ] `dir/max-package-depth` — **counts package directories only**; asset
      directories are exempt or the rule contradicts `dir/embedded-assets`
- [ ] `dir/domain-has-no-go-files`
- [ ] `dir/embedded-assets`
- [ ] Resolve the asset-directory definition once, shared by the depth and
      asset rules: a directory is a package **iff** it contains `.go` files
- [ ] Enable all 6 on this repository and fix what they find

**Exit criteria:** goorg's own tree passes all 6.

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

- [ ] `logic/expand-struct-definition` — do this one first. Detection is exact
      (compare the line of `Fields.Opening` and `Fields.Closing`), it has no
      heuristic and no false positives, and it is the sharpest test of the gofmt
      conformance harness.
- [ ] `logic/max-condition-operands`
- [ ] `logic/max-object-members`
- [ ] `logic/iota-candidate` — shares the enum classifier with
      `org/member-order`; build it once
- [ ] `logic/prefer-guard-clause`
- [ ] `logic/factory-naming` — ship `scope: prefixed` only. `all-factories`
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

**Blocked by:** Phase 0 decision on `go/types`. If that decision is no, this
phase and 5 rules are cut.

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

**Blocked by:** Phase 0 approve/deny; individually, Phase 4 or 5 by tier.

- [ ] Implement approved syntax-tier proposals
- [ ] Implement approved type-tier proposals
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

- **Autofix (`--fix`).** `logic/expand-struct-definition` is purely positional
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
