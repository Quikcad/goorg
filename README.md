# goorg

Enforce Go project structure and house style in CI.

`gofmt` settles how a line looks. `go vet` and `golangci-lint` catch what a
statement does wrong. Neither has an opinion about **where a package lives,
which file a type belongs in, or what a constructor is called** — and those are
exactly the decisions that drift as a codebase and a team grow.

goorg checks the layer above syntax:

| Category | Rule prefix | What it enforces | Status |
| --- | --- | --- | --- |
| **Directory organization** | `dir/` | Where code lives — tree shape, domains, package depth | **6 rules** |
| **File organization** | `org/` | How code is split across files — ordering, budgets, globals | **13 rules** |
| **Logic organization** | `logic/` | How code is shaped — type size, conditions, control flow | **36 rules** |
| **Pattern correctness** | `pat/` | Naming conventions and declaration layout | **2 rules** |

The standard itself lives in [`docs/`](docs/); decisions that bind the
implementation are recorded in [`docs/decisions.md`](docs/decisions.md).

It is a single static binary with one dependency, a meaningful exit code, and
native GitHub Actions annotations. It never rewrites your source.

---

## Install

```sh
go install github.com/Quikcad/goorg/cmd/lint/goorg@latest
```

Or grab a binary from [Releases](https://github.com/Quikcad/goorg/releases).

## Use

```sh
goorg init          # write a starter .goorg.yaml
goorg check ./...   # check the project
goorg rules         # list every rule and the severity it runs at here
goorg explain dir/domain-layout
```

Running `goorg` with no arguments checks the current tree. Flags may appear
anywhere:

```sh
goorg check ./internal/... --format=json --fail-on=warning
```

### Output

```
pkg/billing/types.go:1:9: error: domain directory pkg/billing contains 1 Go file [dir/domain-has-no-go-files]
  help: move the code into a package within the domain, named for what it provides
pkg/billing/invoice/schema.json: error: schema.json sits beside Go source [dir/embedded-assets]
  help: move it into a subdirectory and embed that directory instead
pkg/invoice/inv.go:1:9: error: package invoice sits directly under pkg/; it must belong to a domain [dir/domain-layout]
  help: move it to pkg/<domain>/invoice/

3 errors, 0 warnings
rules: dir/domain-has-no-go-files (1), dir/domain-layout (1), dir/embedded-assets (1)
```

Suppress a single finding in source, with a reason — a directive without one is
itself an error:

```go
//goorg:ignore dir/embedded-assets — vendored fixture, cannot be moved
```

### Exit codes

These are contractual — CI can branch on them.

| Code | Meaning |
| --- | --- |
| `0` | No findings at or above the failure threshold |
| `1` | Violations reported |
| `2` | goorg could not run: bad flags, bad config, unreadable tree |

The split between `1` and `2` is deliberate. A misspelled rule name in
`.goorg.yaml` exits `2`, so a broken config can never masquerade as a clean run.

---

## CI

### GitHub Actions

The bundled action installs a released binary and annotates the pull request
diff inline:

```yaml
name: Lint
on: [pull_request]

jobs:
  goorg:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Quikcad/goorg@v1
```

Inputs: `version`, `args`, `working-directory`, `config`, `fail-on`, `format`.

```yaml
      - uses: Quikcad/goorg@v1
        with:
          version: v1.2.3
          args: check ./internal/...
          fail-on: warning
```

### Anywhere else

```sh
go install github.com/Quikcad/goorg/cmd/lint/goorg@latest
goorg check ./...
```

`--format=github` is selected automatically when `GITHUB_ACTIONS=true`.
Elsewhere use `--format=json` to feed another tool, or `--format=text`
(the default) for a log a human will read.

### Adopting on an existing codebase

A codebase that predates the standard will not pass on day one, and that is not
a reason to weaken the standard. `--max-warnings` is a ratchet: set it to
today's count so new code cannot add to it, then lower it as batches get
cleared.

```sh
goorg check ./... --fail-on=warning --max-warnings=99999   # see where you stand
goorg check ./... --max-warnings=140                       # then ratchet down
```

[docs/adoption.md](docs/adoption.md) has the full sequence, including which
rules are cheap to clear in bulk and which are design decisions to leave for
last.

---

## Configuration

goorg looks for `.goorg.yaml` at the project root — the nearest ancestor
directory holding a `go.mod` or a config file. With no config, every rule runs
at its shipped default.

```yaml
version: 1

rules:
  # Keys are exact rule IDs or globs. The most specific key wins regardless of
  # the order it appears in, so `dir/*` overrides `*` and an exact ID overrides
  # both.
  "*": warning
  dir/*: error
  dir/max-entries: off

settings:
  dir/domain-layout:
    pkg: domains
    cmd: domains
    internal: any
  dir/max-entries:
    limit: 20
    overrides:
      "docs/**": 50

exclude:
  # Globs over root-relative paths. `**` matches any number of directories, and
  # a pattern with no `/` applies at any depth. `**/testdata/**` is always
  # excluded and does not need listing.
  - "**/*.pb.go"
  - "**/mock_*.go"
```

Severities are `error` (reported, fails the run), `warning` (reported, does
not fail), and `off` (not run).

Unknown keys, unknown rule IDs, and unknown severities are all **hard errors**.
A linter that silently ignores a typo in its own config is worse than no linter,
because the disabled rule looks like a passing one.

---

## Rules

Run `goorg explain <rule>` for the rationale, examples, and options for any of
these.

### `dir/` — directory organization — 6 rules

Where a package is allowed to live, and what a directory may hold.

| Rule | Default | Tier | Enforces |
| --- | --- | --- | --- |
| `dir/domain-has-no-go-files` | error | syntax | a domain directory contains no Go files whatsoever |
| `dir/domain-layout` | error | syntax | packages sit at the nesting depth their root's mode requires |
| `dir/embedded-assets` | error | syntax | non-Go files live in a subdirectory, never beside Go source |
| `dir/max-entries` | warning | syntax | a directory holds at most a configured number of entries |
| `dir/max-package-depth` | error | syntax | no subdomains and no subpackages |
| `dir/top-level-layout` | error | syntax | only the configured roots may contain Go packages |

### `org/` — file organization — 13 rules

Which file a declaration belongs in, and in what order files present them. 3 of them need the module to compile.

| Rule | Default | Tier | Enforces |
| --- | --- | --- | --- |
| `org/consumer-locality` | warning | types | a declaration used from only one other file belongs in that file |
| `org/global-file-scoped` | error | types | a package-level variable is referenced only in the file that declares it |
| `org/globals-singleton-only` | error | syntax | package-level variables are permitted only as singleton state |
| `org/interface-method-order` | warning | types | methods are declared in the order the interface declares them |
| `org/interface-own-file` | warning | syntax | an interface declaration gets a file of its own |
| `org/max-functions-per-file` | error | syntax | a file declares at most a configured number of functions |
| `org/max-private-functions` | error | syntax | a file with exports declares few unexported functions |
| `org/max-public-functions` | error | syntax | a file declares at most a configured number of exported functions |
| `org/member-order` | error | syntax | declarations appear in the canonical file order |
| `org/private-functions-last` | error | syntax | unexported functions come after every exported one |
| `org/singleton-instance-func` | error | syntax | a singleton is built by an unexported instance function guarded by sync.Once |
| `org/singleton-layout` | error | syntax | a singleton file is laid out as state, accessor, then exported functions |
| `org/type-cohesion` | error | syntax | a type, its factory and its methods live in one file |

### `logic/` — logic organization — 35 rules

The shape of the code itself — type size, conditions, control flow, enums. 14 of them need the module to compile.

| Rule | Default | Tier | Enforces |
| --- | --- | --- | --- |
| `logic/any-should-be-generic` | warning | types | any that only carries a value should be a type parameter |
| `logic/boolean-field-count` | warning | syntax | a struct carries at most a configured number of bool fields |
| `logic/boolean-parameter` | **off** | syntax | a bool parameter on an exported function is unreadable at the call site |
| `logic/constraint-too-wide` | warning | types | a type parameter narrowed at runtime should be narrowed in its constraint |
| `logic/context-in-struct` | error | types | a context.Context stored in a struct outlives its request |
| `logic/duplicate-const-value` | error | syntax | two constants in one block share a value |
| `logic/empty-branch` | warning | syntax | an empty branch needs a comment saying why |
| `logic/empty-interface-field` | warning | syntax | a struct field typed any erases what it holds |
| `logic/enum-missing-string` | warning | types | an enum type has no String method |
| `logic/enum-zero-value-unnamed` | warning | syntax | an iota enum names its zero value |
| `logic/exported-embedded-mutex` | error | types | an exported struct embedding a mutex leaks Lock into its API |
| `logic/float-equality` | error | types | floating-point values are compared with == or != |
| `logic/ideal-numeric-type` | warning | types | a numeric parameter should be the type its uses already speak |
| `logic/identical-branches` | error | syntax | two branches of one conditional have identical bodies |
| `logic/if-chain-to-switch` | warning | syntax | a long else-if chain on one operand should be a switch |
| `logic/integer-division-to-float` | error | types | integer division converted to a float truncates first |
| `logic/interface-at-consumer` | warning | types | an interface declared beside its only implementation belongs at the consumer |
| `logic/interface-registry` | error | types | types are checked against a registry of interfaces |
| `logic/interface-size` | warning | syntax | an interface declares at most a configured number of methods |
| `logic/iota-candidate` | error | syntax | a run of consecutive integer constants should use iota |
| `logic/lossy-conversion` | error | types | a narrowing numeric conversion has no range check |
| `logic/max-condition-operands` | error | syntax | a condition combines at most a configured number of operands |
| `logic/max-function-lines` | warning | syntax | a function body stays within a configured line count |
| `logic/max-function-params` | warning | syntax | a function takes at most a configured number of parameters |
| `logic/max-nesting-depth` | error | syntax | block nesting stays within a configured depth |
| `logic/max-object-members` | error | syntax | a type declares at most a configured number of members |
| `logic/max-return-values` | warning | syntax | a function returns at most a configured number of values |
| `logic/negated-condition` | warning | syntax | an if/else on a negated condition should be flipped |
| `logic/panic-outside-main` | error | types | a library may not panic |
| `logic/pointer-to-slice-or-map` | warning | syntax | a pointer to a slice or map is almost always a mistake |
| `logic/prefer-guard-clause` | error | syntax | a wholly wrapped body should invert into a guard clause |
| `logic/section-spacing` | warning | syntax | a function's guard prologue and its result are set off by a blank line |
| `logic/single-case-switch` | warning | syntax | a switch with one case is an if, or a missing case |
| `logic/stringly-typed-enum` | warning | syntax | a run of string constants used as an enum needs a named type |
| `logic/unsigned-underflow` | warning | types | subtraction on an unsigned type can wrap to a huge value |
| `logic/unused-type-parameter` | warning | types | a type parameter used once is not doing generic work |

### `pat/` — pattern correctness — 2 rules

Naming conventions and declaration layout.

| Rule | Default | Tier | Enforces |
| --- | --- | --- | --- |
| `pat/expand-struct-definition` | error | syntax | a struct with fields is written across multiple lines |
| `pat/factory-naming` | error | syntax | Make returns a value, New returns a pointer |

`logic/boolean-parameter` is the one rule that ships **off**: a bare `bool` on
an exported function is a real readability cost but a common and not-wrong
shape. Enable it with `logic/boolean-parameter: warning` when the team wants
the convention.

### Tiers

On goorg itself, with all 57 rules enabled and a warm build cache, the syntax
tier takes 34 ms and both tiers take 730 ms. `go vet ./...` on the same tree
takes 80 ms — it reuses cached export data per package, which goorg does not,
so the type tier is the part of a run you feel. That is what `--syntax-only`
is for.

Every rule declares a tier. **Syntax** rules use `go/parser` only, so they work
on a tree that does not compile — which is exactly when someone is mid-refactor
and most wants a layout linter. **Type** rules need `go/types` and therefore a
module that loads; `--syntax-only` skips them. When type loading fails goorg
reports the gap in coverage and exits `2` rather than passing silently.

The type tier is only loaded when a type-tier rule is actually enabled, so a
project that switches them off pays nothing for them.

---

## Relationship to other tools

`gofmt` stays authoritative on whitespace: no goorg rule reports on anything
gofmt would rewrite, and a test harness reformats every fixture to prove it.

`golangci-lint` covers what a statement does wrong, and goorg deliberately does
not duplicate it — nine proposed rules were denied for that reason. The
`.golangci.yml` covering that gap is in
[docs/adoption.md](docs/adoption.md#where-goorg-stops).

## Develop

```sh
task            # fmt, vet, test, and dogfood — everything CI runs
task test
task dogfood    # run goorg against its own source
task --list
```

goorg checks itself in CI. A rule this project cannot live under is a rule that
needs rethinking.

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to add a rule.

## License

MIT — see [LICENSE](LICENSE).
