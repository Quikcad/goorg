# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`goorg` is a CI linter for Go **project structure**, not Go syntax. It enforces
an in-house style standard covering three things `gofmt` and `golangci-lint`
have no opinion about:

- **directory correctness** (`dir/`) — where code lives
- **organizational correctness** (`org/`) — how code is split across files
- **pattern correctness** (`pat/`) — recurring code shapes and naming

Single static binary, one dependency (`gopkg.in/yaml.v3`), meaningful exit
codes. It reports; it never rewrites source.

## Commands

The build system is [Taskfile](https://taskfile.dev), not make.

```sh
task            # fmt, vet, test, dogfood — everything CI runs
task test       # go test ./...
task build      # -> bin/goorg
task dogfood    # run goorg against its own source tree
task rules      # list rules with their configured severity
task --list
```

Always run `task` before declaring work done. The `dogfood` step matters: goorg
must pass its own rules, and CI fails if it does not.

## Architecture

Data flows one way. No package below imports one above it.

```
cmd/goorg              thin main; wires os streams into cli.Main
  └─ internal/cli      flag parsing, subcommand routing, exit codes
       ├─ internal/config   loads .goorg.yaml, resolves severity, builds excluders
       ├─ internal/project  walks the tree, parses files -> Project/Package/File/Dir
       ├─ internal/runner   runs enabled rules, stamps RuleID + Severity onto findings
       ├─ internal/rules    rule registry and every rule
       ├─ internal/report   text / github / json renderers
       └─ internal/diag     Diagnostic, Position, Severity — the shared vocabulary
```

Two invariants hold this together:

1. **Rules never see configuration.** A rule returns findings with only
   `Position`, `Message` and `Help` filled in. `runner` stamps on `RuleID` and
   `Severity`. Per-rule options arrive through `Context.Settings(&dst)`, which
   `config` supplies as an opaque closure — `rules` does not import `config`,
   and adding a rule option never touches the config package.

2. **`project` does not type-check.** It uses `go/parser`, not `go/packages`.
   Rules are syntactic on purpose: a layout linter has to work on a tree that
   does not compile, because that is when someone is mid-refactor and most
   wants it. Type-based analysis would be a separate, opt-in path — do not
   quietly add `go/types` to an existing rule.

## Adding a rule

1. Write it in `internal/rules/{dir,org,pattern}.go` and `Register` it from
   that file's `init`.
2. Rule IDs are `<category>/<kebab-case>`, matched by a regexp in `Register`,
   which panics at init on a malformed or duplicate ID.
3. Fill in `Doc` properly. `TestRegistryIsWellFormed` enforces that every rule
   is at least 200 characters of documentation and contains both a
   `Rationale:` and a `To fix:` section — because `goorg explain` is only
   useful if rules argue for themselves. Do not weaken that test to land a
   rule; write the docs.
4. Add a test in `internal/rules/rules_test.go` using the `run` helper, which
   materializes an in-memory file map into a temp tree. Fixtures live in the
   test, not in `testdata/` — `**/testdata/**` is excluded by default, and
   keeping source next to its expected finding makes cases readable.
5. Findings must be **deterministic**. `TestRulesAreDeterministic` runs every
   rule repeatedly against the same tree. If you iterate a map anywhere, sort
   before returning, or CI results will flap between identical runs.
6. Pick a `Default` severity conservatively. A new rule shipping as `error`
   breaks every consumer's build on upgrade; `warning` does not.
7. Update the rules table in `README.md`. `goorg init` generates its rule list
   from the live registry, so that needs no edit.

## Conventions

- Messages are lowercase, no trailing period — Go error string convention.
  `Summary` too; a test enforces it.
- `Help` says how to fix it, not what is wrong.
- Positions are always slash-separated and relative to the project root, so
  output is identical on a laptop and on a CI runner. A `Line` of 0 means the
  finding is about a whole file or directory.
- Exit codes `0`/`1`/`2` are contractual. `2` specifically means "goorg could
  not run", so a bad config can never look like a clean pass. Do not collapse
  them.
- Comments explain *why*. The code already says what.

## Testing

- `internal/rules/rules_test.go` — per-rule behavior, plus the registry
  well-formedness and determinism meta-tests.
- `internal/cli/cli_test.go` — end-to-end through `cli.Main` with injected
  streams and env. Exit codes, output formats, flag permutation.
- `internal/config/config_test.go` — glob matching and severity precedence.
  `TestSeveritySpecificity` loops 20 times per case on purpose: map iteration
  order is randomized, so a precedence bug would otherwise be flaky.

The CLI is testable in-process because `cli.Main` takes an `Env` and returns an
exit code rather than calling `os.Exit`. Keep it that way — never call
`os.Exit` outside `cmd/goorg/main.go`.

## Gotchas

- **Flag permutation.** `flag.FlagSet.Parse` stops at the first positional
  argument, so `goorg check ./... -format=json` would silently ignore the flag.
  `parseFlags` in `internal/cli/cli.go` permutes flags ahead of operands first.
  Use `parseFlags`, never `fs.Parse`, in a command.
- **`path.Match` does not cross `/`.** Rule ID globs use `MatchRuleID`, where
  `*` spans the whole ID so a key of `*` means every rule. Path globs use
  `MatchPath`, which implements `**` over segments. They are different
  matchers on purpose; do not merge them.
- The `go` directive in `go.mod` is `1.25.0`, below the local toolchain, so
  that consumers on an older Go can still install. Do not bump it to match
  whatever is installed.

## Git

Commits and pull requests are written as the author's own work. Do not add
`Co-Authored-By` trailers, "Generated with…" footers, or any other AI
attribution to commits, PR bodies, issues, or comments.

Commit messages follow Conventional Commits (`feat:`, `fix:`, `docs:`,
`chore:`, and `feat(rules):` for a new rule) — `.goreleaser.yaml` groups the
changelog from those prefixes.
