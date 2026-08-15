# goorg

Enforce Go project structure and house style in CI.

`gofmt` settles how a line looks. `go vet` and `golangci-lint` catch what a
statement does wrong. Neither has an opinion about **where a package lives,
which file a type belongs in, or what a constructor is called** — and those are
exactly the decisions that drift as a codebase and a team grow.

goorg checks the layer above syntax:

| Category | Rule prefix | What it enforces |
| --- | --- | --- |
| **Directory correctness** | `dir/` | Where code lives — tree shape, permitted directories, package name vs. path |
| **Organizational correctness** | `org/` | How code is split across files — naming, size, package docs |
| **Pattern correctness** | `pat/` | Recurring code shapes — signature and naming conventions |

It is a single static binary with one dependency, a meaningful exit code, and
native GitHub Actions annotations. It never rewrites your source.

---

## Install

```sh
go install github.com/Quikcad/goorg/cmd/goorg@latest
```

Or grab a binary from [Releases](https://github.com/Quikcad/goorg/releases).

## Use

```sh
goorg init          # write a starter .goorg.yaml
goorg check ./...   # check the project
goorg rules         # list every rule and the severity it runs at here
goorg explain dir/cmd-layout
```

Running `goorg` with no arguments checks the current tree. Flags may appear
anywhere:

```sh
goorg check ./internal/... --format=json --fail-on=warning
```

### Output

```
internal/httpclient/client.go:1:9: error: package client is in directory internal/httpclient; expected package httpclient [dir/package-matches-directory]
  help: rename the package to httpclient, or move it to a directory named client
internal/utils: error: directory internal/utils uses the banned name "utils" [dir/forbidden-directories]
  help: name packages after what they provide; split the contents into purpose-named packages

2 errors, 0 warnings
rules: dir/forbidden-directories (1), dir/package-matches-directory (1)
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
go install github.com/Quikcad/goorg/cmd/goorg@latest
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
  org/max-file-lines: off

settings:
  dir/forbidden-directories:
    names: [src, util, utils, common, misc]
  org/max-file-lines:
    limit: 800
    include_tests: false

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

### `dir/` — directory correctness

| Rule | Default | Enforces |
| --- | --- | --- |
| `dir/package-matches-directory` | error | Package name matches its directory, ignoring `-`/`_`; `v2/` resolves to the parent |
| `dir/forbidden-directories` | error | No grab-bag names: `src`, `util`, `common`, `misc`, … |
| `dir/cmd-layout` | error | `package main` lives in `cmd/<binary>/` and declares a `func main` |

### `org/` — organizational correctness

| Rule | Default | Enforces |
| --- | --- | --- |
| `org/file-naming` | error | Lowercase file names with `_` separators |
| `org/max-file-lines` | warning | Files stay under a line limit (600) |
| `org/package-doc` | warning | Exactly one package comment, opening with `Package <name>` |

### `pat/` — pattern correctness

| Rule | Default | Enforces |
| --- | --- | --- |
| `pat/error-var-naming` | error | Sentinel errors are named `Err…` / `err…` |
| `pat/context-first-param` | error | `context.Context` is the first parameter, named `ctx` |
| `pat/receiver-naming` | error | One receiver name per type; never `this` or `self` |

Rules are syntactic — goorg parses but does not type-check, so it still reports
on a tree that does not compile. That is a deliberate trade: a layout linter is
most useful mid-refactor.

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
