# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`goorg` is a CI linter for Go **project structure**, not Go syntax. It enforces
an in-house standard covering what `gofmt` and `golangci-lint` have no opinion
about: where a package lives, which file a type belongs in, how big a type is
allowed to get, and what a constructor is called.

55 rules across four families:

| Prefix | Family | Rules |
| --- | --- | --- |
| `dir/` | directory organization — where code lives | 6 |
| `org/` | file organization — how code splits across files | 12 |
| `logic/` | logic organization — how code is shaped | 35 |
| `pat/` | pattern correctness — naming and declaration layout | 2 |

The standard itself lives in [`docs/`](docs/). Decisions that bind the
implementation are numbered in [`docs/decisions.md`](docs/decisions.md) — read
D1–D7 before changing anything structural.

## Commands

The build system is [Taskfile](https://taskfile.dev), not make.

```sh
task              # fmt, vet, test, dogfood — everything CI runs
task test
task build        # -> bin/goorg
task dogfood      # run goorg against its own source
task calibrate    # measure budget distributions over a corpus
task --list
```

Always run `task` before declaring work done. The `dogfood` step is not
advisory: goorg must pass all 55 of its own rules, and CI fails if it does not.

## Architecture

Data flows one way. No package below imports one above it.

```
cmd/lint/goorg              thin main; wires os streams into cli.Main
  └─ internal/cli           flags, subcommands, exit codes, tier planning
       ├─ pkg/lint/config     .goorg.yaml — severity, settings, excludes
       ├─ pkg/lint/runner     runs enabled rules, stamps RuleID + Severity
       ├─ pkg/lint/rule       Rule, Tier, Placement, Category, Context, Set
       ├─ pkg/lint/whatif     evaluates proposed relocations
       ├─ pkg/lint/suppress   //goorg:ignore directives
       ├─ pkg/lint/report     text / github / json
       ├─ pkg/lint/glob       the two pattern matchers
       ├─ pkg/lint/diag       Diagnostic, Position, Severity — the vocabulary
       ├─ pkg/lint/ruletest   the fixture harness every family tests against
       ├─ pkg/source/project  parse-only tree model (syntax tier)
       ├─ pkg/source/typed    go/packages loader (type tier)
       ├─ pkg/source/decl     classifiers more than one family needs
       └─ pkg/rules/*         directory, organization, logic, pattern
```

### The invariants

1. **Rules never see configuration.** A rule returns findings with only
   `Position`, `Message` and `Help` filled in; `runner` stamps on `RuleID` and
   `Severity`. Options arrive through `Context.Settings(&dst)` as an opaque
   closure, so `rules` does not import `config` and adding an option never
   touches the config package.

2. **There is no global registry.** Families export `Rules()` and
   `internal/cli.buildRuleSet` composes them explicitly. goorg therefore has no
   package-level mutable state — the same constraint
   `org/globals-singleton-only` imposes on everyone else.

3. **Two tiers, and a Types rule never runs without a program.** Syntax rules
   use `go/parser` and work on a tree that does not compile. Type rules need
   `go/packages`. A load failure reports skipped coverage and exits 2; it never
   passes silently. See [D1](docs/decisions.md).

4. **A rule proposes a move; the engine decides.** A rule whose advice is "put
   this elsewhere" emits a `rule.Relocation` rather than a finding.
   `pkg/lint/whatif` makes the move on an in-memory copy and keeps the finding
   only if nothing got worse. Rules do not read each other's settings.

## Adding a rule

1. Write it in the family package, add it to that package's `Rules()`.
2. IDs are `<dir|org|logic|pat>/<kebab-case>`, checked by a regexp in
   `rule.NewSet`. **IDs are contractual** — they appear in every consumer's
   `.goorg.yaml`, and renaming one silently disables it.
3. Set `Tier`. `Syntax` unless the rule genuinely needs types; a type-tier rule
   costs every user the price of a build.
4. Set `Placement` only if the rule is about position *within* a file rather
   than which file. Getting this wrong makes the what-if pass reject every
   relocation.
5. Fill in `Doc`. `ruletest.AssertWellFormed` enforces 200+ characters with a
   `Rationale:` and a `To fix:` section. Do not weaken it to land a rule —
   `goorg explain` is how anyone decides whether to adopt one, and a rule that
   cannot argue for itself gets switched off.
6. Add fixtures to the family's test: one proving it fires, one proving it stays
   **silent on conforming code**. The second is what stops a rule being written
   broadly enough to fire on correct Go.
7. Pick a conservative `Default`. A new rule shipping as `error` breaks every
   consumer's build on upgrade.
8. Run `task`. Then fix what it finds in goorg's own source, or reconsider the
   rule.

### Calibrate limits, do not guess them

Every numeric default came from measuring the Go standard library with
`task calibrate`, and the results are tabulated in
[D5](docs/decisions.md). `logic/max-nesting-depth` at 3 sits exactly at the
stdlib p95, which is why it found seven real offenders here; a guessed limit
would have been useless or unusable. When adding a budget, measure first.

The measurement tells you *adoption cost*, not correctness. For a rule that
deliberately departs from ordinary Go practice, a high flag rate may be the rule
working.

## Conventions

- Messages are lowercase, no trailing period. `Summary` too; a test enforces it.
- `Help` says how to fix it, not what is wrong.
- Positions are slash-separated and root-relative, so output is identical on a
  laptop and on CI. `Line` 0 means the whole file or directory.
- Exit codes `0`/`1`/`2` are contractual. `2` means "goorg could not run", so a
  bad config can never look like a clean pass. Do not collapse them.
- Findings must be deterministic. Sort anything derived from a map;
  `ruletest.AssertDeterministic` catches iteration order reaching the output.
- Comments explain *why*. The code already says what.

## Testing

- `pkg/rules/*/…_test.go` — per-rule fixtures plus family-wide well-formedness,
  determinism and gofmt-conformance assertions.
- `internal/cli/cli_test.go`, `tier_test.go` — end to end through `cli.Main`
  with injected streams. Exit codes, formats, flag permutation, tier gating,
  the what-if pass.
- `pkg/lint/whatif/whatif_test.go` — relocations accepted, rejected, cycles.
- `pkg/lint/config/config_test.go` — severity precedence, looped 20 times
  because map order is randomized and a precedence bug would otherwise be flaky.

`ruletest.Run` rejects a type-tier rule rather than nil-dereferencing; use
`ruletest.SyntaxTier` for family-wide loops and exercise type rules through
`internal/cli`.

## Gotchas

- **gofmt is authoritative on whitespace.** `ruletest.AssertGofmtStable`
  reformats every fixture and asserts the finding set is unchanged. A rule that
  fails it competes with the formatter. `pat/expand-struct-definition` is the
  closest to that line and only has room to exist because gofmt preserves
  whichever struct form you wrote.
- **Flag permutation.** `flag.FlagSet.Parse` stops at the first positional, so
  `goorg check ./... -format=json` would ignore the flag. Use `parseFlags`,
  never `fs.Parse`.
- **Two glob matchers, deliberately different.** `glob.MatchRuleID` lets `*`
  cross the `/` so a key of `*` means every rule; `glob.MatchPath` implements
  `**` over segments. Do not merge them.
- **Two rules must never report the same defect.** `dir/domain-layout` and
  `dir/domain-has-no-go-files` both fired on one directory until they split on
  whether packages sit beneath it. Both families have a regression test
  asserting no position is reported twice.
- **The `go` directive in `go.mod` is `1.25.0`**, below the local toolchain, so
  consumers on an older Go can still install. Do not bump it to match whatever
  is installed.
- **The release path is exercised once a year.** `.goreleaser.yaml`'s `main:`
  pointed at a package that had not existed for four phases. CI now has a
  `release-config` job that builds the target and checks the archive-name
  templates in `.goreleaser.yaml` and `action.yml` still agree.

## Git

Commits and pull requests are written as the author's own work. Do not add
`Co-Authored-By` trailers, "Generated with…" footers, or any other AI
attribution to commits, PR bodies, issues, or comments.

Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, `feat(rules):` for a
new rule) — `.goreleaser.yaml` groups the changelog from those prefixes.
