# goorg

Enforce Go project structure and house style in CI.

`gofmt` settles how a line looks. `go vet` and `golangci-lint` catch what a
statement does wrong. Neither has an opinion about **where a package lives,
which file a type belongs in, or what a constructor is called** — and those are
exactly the decisions that drift as a codebase and a team grow.

goorg checks the layer above syntax:

| Category | Rule prefix | What it enforces | Status |
| --- | --- | --- | --- |
| **Directory organization** | `dir/` | Where code lives — tree shape, domains, package depth | **6 rules, shipped** |
| **File organization** | `org/` | How code is split across files — ordering, budgets, globals | **10 rules, shipped** |
| **Logic organization** | `logic/` | How code is shaped — type size, conditions, control flow | **4 rules, shipped** |
| **Pattern correctness** | `pat/` | Naming conventions and declaration layout | **2 rules, shipped** |

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

### `dir/` — directory organization — **implemented**

| Rule | Default | Enforces |
| --- | --- | --- |
| `dir/domain-layout` | error | Packages sit at `<root>/<domain>/<package>`; per-root `domains` / `packages` / `any` |
| `dir/domain-has-no-go-files` | error | A domain directory holds no `.go` files at all |
| `dir/embedded-assets` | error | Non-Go files live in a subdirectory, never beside Go source |
| `dir/max-entries` | warning | A directory holds at most N entries, files and subdirectories together |
| `dir/max-package-depth` | error | No subdomains and no subpackages |
| `dir/top-level-layout` | error | Only `pkg/`, `cmd/`, `internal/` may contain Go packages |

### `org/` — file organization — **implemented**

| Rule | Default | Enforces |
| --- | --- | --- |
| `org/member-order` | error | Enums, vars, interfaces, types with their factories and methods, then functions |
| `org/private-functions-last` | error | Unexported functions after every exported one |
| `org/singleton-layout` | error | Singleton files: state, `instance`, then exported accessors |
| `org/singleton-instance-func` | error | Construction guarded by `sync.Once`, never in `init` |
| `org/globals-singleton-only` | error | Package-level vars only as singleton state |
| `org/type-cohesion` | error | A type, its factory and its methods in one file |
| `org/interface-own-file` | warning | An interface gets a file of its own |
| `org/max-functions-per-file` | error | At most N functions per file |
| `org/max-public-functions` | error | At most N exported functions per file |
| `org/max-private-functions` | error | Few unexported helpers beside an exported API |

### `logic/` — logic organization — **implemented**

| Rule | Default | Enforces |
| --- | --- | --- |
| `logic/iota-candidate` | error | A run of consecutive integer constants uses `iota` |
| `logic/max-condition-operands` | error | At most N operands per condition; `&&`/`||` mixed only with parentheses |
| `logic/max-object-members` | error | At most N fields and N methods per type |
| `logic/prefer-guard-clause` | error | A wholly wrapped body inverts into a guard clause |

### `pat/` — pattern correctness — **implemented**

| Rule | Default | Enforces |
| --- | --- | --- |
| `pat/expand-struct-definition` | error | A struct with fields spans multiple lines, one field each |
| `pat/factory-naming` | error | `Make` returns a value, `New` returns a pointer |

One rule ships **off**: `logic/boolean-parameter` objects to a bare `bool` on an
exported function, which is a real readability cost but a common and not-wrong
shape. Enable it with `logic/boolean-parameter: warning` when the team wants the
convention.

### Type tier — **implemented**

These five need the module to compile. `--syntax-only` skips them.

| Rule | Default | Enforces |
| --- | --- | --- |
| `logic/interface-registry` | error | Types are checked against a registry of interfaces, including near misses |
| `logic/any-should-be-generic` | warning | `any` that only carries a value should be a type parameter |
| `logic/ideal-numeric-type` | warning | A numeric parameter is the type its uses already speak |
| `org/global-file-scoped` | error | A package variable is referenced only in its declaring file |
| `org/consumer-locality` | warning | A declaration used from one other file belongs in it |

### Tiers

On goorg itself, with all 55 rules enabled, the syntax tier takes 34 ms and
both tiers take 730 ms — against 475 ms for `go vet ./...`, the honest
comparison since both type-check.

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
