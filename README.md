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
| **File organization** | `org/` | How code is split across files — ordering, budgets, globals | specified |
| **Logic organization** | `logic/` | How code is shaped — type size, interfaces, complexity | specified |
| **Pattern correctness** | `pat/` | Naming conventions and declaration layout | specified |

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

A codebase that predates the standard will not pass on day one. Two knobs make
adoption a ratchet rather than a cliff:

```sh
# Report everything, fail on nothing yet.
goorg check ./... --fail-on=warning --max-warnings=99999

# Then tighten the ceiling as the count falls.
goorg check ./... --max-warnings=40
```

Or set the noisy rules to `warning` in `.goorg.yaml` and promote them to
`error` one at a time.

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

### `org/`, `logic/`, `pat/` — specified, not yet implemented

21 further rules are fully specified in [`docs/`](docs/) and scheduled in
[TODO.md](TODO.md). `goorg rules` always lists what the binary you have actually
runs.

### Tiers

Every rule declares a tier. **Syntax** rules use `go/parser` only, so they work
on a tree that does not compile — which is exactly when someone is mid-refactor
and most wants a layout linter. **Type** rules need `go/types` and therefore a
module that loads; `--syntax-only` skips them. When type loading fails goorg
reports the gap in coverage and exits `2` rather than passing silently.

Every `dir/` rule is syntax-tier.

---

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
