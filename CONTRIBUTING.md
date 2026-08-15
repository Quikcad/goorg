# Contributing to goorg

## Getting set up

You need Go 1.25+ and [go-task](https://taskfile.dev/installation).

```sh
git clone https://github.com/Quikcad/goorg
cd goorg
task          # fmt, vet, test, and dogfood
```

`task` runs everything CI runs. If it passes locally, CI should agree.

## The bar for a new rule

goorg encodes a house standard, so every rule is a policy decision before it is
a piece of code. Before writing one, be able to answer:

1. **What breaks without it?** Not "it's inconsistent" — what actually goes
   wrong. A reader misled, a refactor made impossible, a bug that only shows up
   on CI. If the honest answer is "nothing, I just prefer it", it is not a rule.
2. **Is it mechanically checkable without types?** goorg parses but does not
   type-check (see [CLAUDE.md](CLAUDE.md#architecture) for why). A rule needing
   full type information is a different tool.
3. **What is the false-positive story?** A rule that fires on legitimate code
   trains people to disable it, which costs more than the rule was worth.
4. **Should it ship as `error` or `warning`?** A new rule defaulting to `error`
   breaks every consumer's build the moment they upgrade. Default to `warning`
   unless the violation is unambiguous.

Open an issue with the rule proposal template before writing code for anything
non-obvious. It is cheaper to argue about the policy than about the diff.

## Writing the rule

Walked through in [CLAUDE.md](CLAUDE.md#adding-a-rule). The short version:

- Register it from `init` in `internal/rules/{dir,org,pattern}.go`.
- ID is `<category>/<kebab-case>`; `Register` panics on a bad or duplicate one.
- `Doc` must include a `Rationale:` and a `To fix:` section, and be substantial.
  `TestRegistryIsWellFormed` fails the build otherwise. This is not bureaucracy:
  `goorg explain` is how anyone decides whether to adopt a rule, and a rule that
  cannot argue for itself will just be switched off.
- Sort anything derived from a map. `TestRulesAreDeterministic` catches
  iteration order leaking into output.
- Add cases to `internal/rules/rules_test.go`, including at least one that
  proves the rule stays **silent** on conforming code.
- Update the rules table in `README.md`.

## Changing an existing rule

Tightening a rule is a breaking change for every consumer, even though nothing
about the API changed — their build starts failing on code that passed
yesterday.

- Loosening a rule, or fixing a false positive: patch release.
- Adding a rule as `warning`: minor release.
- Adding a rule as `error`, or making an existing rule stricter: major release,
  and say so explicitly in the changelog.

## Commits and pull requests

Conventional Commits, because `.goreleaser.yaml` builds the changelog from the
prefixes:

```
feat(rules): add org/interface-at-consumer
fix: resolve v2 directories against their parent package name
docs: explain the exit code contract
chore: bump actions/checkout
```

Keep commits scoped to one change. Before opening a PR:

```sh
task
```

Commits and PRs are written as the author's own work — no AI attribution
trailers, footers, or badges.

## Reporting a bug

Include the goorg version (`goorg version`), the smallest source tree that
reproduces it, your `.goorg.yaml`, and what you expected instead. A false
positive without a reproducible tree usually cannot be fixed.
