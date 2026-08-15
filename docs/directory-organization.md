# Directory organization

Specification for the `dir/` rule family — the rules governing **where code
lives**: the shape of the tree, which directories may exist, and what may sit
inside each one.

> **Status: specification only.** Nothing here is implemented yet. Rule IDs,
> config keys, and defaults are proposals. See [Open questions](#open-questions)
> for the decisions still outstanding.

---

## The canonical tree

Every rule below is a constraint on this shape:

```
repo/
├── cmd/
│   └── billing/                    domain      — directories only, no .go files
│       └── invoicer/               package     — package main
│           ├── main.go
│           ├── run.go
│           └── templates/          assets      — no .go files
│               └── invoice.tmpl
├── pkg/
│   └── billing/                    domain
│       ├── invoice/                package
│       │   └── invoice.go
│       └── ledger/                 package
│           ├── ledger.go
│           └── ledger_test.go
├── internal/
│   └── ...
├── docs/                           no Go packages — unconstrained
├── go.mod
└── README.md
```

## Terminology

These four terms are used precisely throughout, and several rules depend on
telling them apart.

| Term | Definition |
| --- | --- |
| **Root directory** | A top-level directory that is permitted to contain Go packages: `pkg/`, `cmd/`, `internal/`. |
| **Domain directory** | The layer directly under a root, e.g. `pkg/billing/`. Groups related packages. Contains directories and nothing else. |
| **Package directory** | A directory containing at least one `.go` file. This is a Go package. |
| **Asset directory** | A directory under a package holding embedded or otherwise non-Go files. Contains **no** `.go` files, so it is not a package. |

The load-bearing distinction is **package directory vs. asset directory**: a
directory is a package if and only if it contains `.go` files. Depth and layout
rules constrain package directories; asset directories are exempt from them but
carry their own constraint (no Go files, ever).

---

## Rules

| ID | Summary |
| --- | --- |
| [`dir/max-entries`](#dirmax-entries) | A directory holds at most N entries (files + subdirectories) |
| [`dir/top-level-layout`](#dirtop-level-layout) | Only `pkg`, `cmd`, `internal` may contain Go packages |
| [`dir/domain-layout`](#dirdomain-layout) | Packages sit at `<root>/<domain>/<package>`, configurable per root |
| [`dir/max-package-depth`](#dirmax-package-depth) | No subdomains and no subpackages |
| [`dir/domain-has-no-go-files`](#dirdomain-has-no-go-files) | A domain directory contains zero `.go` files |
| [`dir/embedded-assets`](#dirembedded-assets) | Non-Go files live in a subdirectory, never beside `.go` files |

---

### `dir/max-entries`

**A directory may contain at most N entries, counting files and immediate
subdirectories together.**

The count is of *immediate* children only, not recursive. A directory with 8
files and 5 subdirectories has 13 entries.

#### Configuration

```yaml
settings:
  dir/max-entries:
    limit: 20
    # Optional per-path overrides, matched as globs over root-relative paths.
    # The most specific matching pattern wins.
    overrides:
      "cmd/*/*": 12      # entry points stay small
      "docs/**": 50      # documentation is not code
```

#### Example

```
pkg/billing/invoice/          limit: 20
├── invoice.go                 1
├── invoice_test.go            2
├── line_item.go               3
├── ...
└── templates/                20   ← one entry, however many files inside
```

#### Rationale

A directory is the unit people scan. Past a couple of dozen entries nobody
reads the listing — they `grep` and hope, which means new code lands wherever
the last file did rather than where it belongs. The limit is a forcing
function: when a package outgrows it, the fix is to split along the seam that
already exists, and the limit makes you find that seam while it is still
obvious.

#### Interactions

- [`dir/embedded-assets`](#dirembedded-assets) collapses many asset files into a
  single subdirectory entry, so the two rules pull in the same direction.
- Splitting a package to satisfy this rule must not create a subpackage —
  [`dir/max-package-depth`](#dirmax-package-depth) forbids that. The correct
  split is a **sibling package in the same domain**, not a nested one.

---

### `dir/top-level-layout`

**At the repository root, only `pkg/`, `cmd/` and `internal/` may contain Go
packages.**

Other top-level directories are permitted without restriction *as long as they
contain no `.go` files* at any depth. `docs/`, `deploy/`, `scripts/`, `.github/`
are all fine.

This rule is about Go packages, not about directories in general.

#### Configuration

```yaml
settings:
  dir/top-level-layout:
    roots: [pkg, cmd, internal]
    # May the repository root itself hold .go files?
    allow_go_files_at_root: false
```

#### Examples

```
✓  docs/architecture.md              no Go — unconstrained
✓  scripts/release.sh                no Go — unconstrained
✗  tools/generate/main.go            Go package outside a root
✗  main.go                           Go file at the repository root
```

#### Rationale

Three roots, three meanings, no overlap: `cmd/` is what the repo produces,
`pkg/` is what other repos may import, `internal/` is what only this repo may
use. Every Go file in the tree is therefore classified by its path alone —
"can I import this?" is answered by looking, never by asking. A fourth
top-level Go directory reopens that question for the whole repository.

#### Interactions

- `internal/` is subject to this rule but its interior may follow different
  layout rules from `pkg/` and `cmd/` — see
  [`dir/domain-layout`](#dirdomain-layout) and
  [Open questions](#open-questions).

---

### `dir/domain-layout`

**Go packages under a root sit at `<root>/<domain>/<package>`, and the domain
layer is either mandatory or forbidden — never mixed.**

A *domain* groups the packages belonging to one area of the system. The rule is
configurable per root, in both directions:

| Mode | Meaning | Package path |
| --- | --- | --- |
| `domains` | Every package belongs to a domain. No package may sit directly under the root. | `pkg/billing/invoice/` |
| `packages` | No domain layer. Every package sits directly under the root. | `pkg/invoice/` |
| `any` | Either shape is accepted. The rule does not apply to this root. | — |

The point of the setting is that a repository picks **one** shape per root and
holds to it. A tree with both `pkg/invoice/` and `pkg/billing/invoice/` gives a
reader no way to predict where anything is.

#### Configuration

```yaml
settings:
  dir/domain-layout:
    pkg: domains
    cmd: domains
    internal: any
```

#### Examples

Under `pkg: domains`:

```
✓  pkg/billing/invoice/invoice.go        domain + package
✓  pkg/billing/ledger/ledger.go          second package in the same domain
✗  pkg/invoice/invoice.go                package directly under the root
```

Under `pkg: packages`:

```
✓  pkg/invoice/invoice.go
✗  pkg/billing/invoice/invoice.go        domain layer not permitted here
```

#### Rationale

A domain is the unit of ownership. When packages sit directly under `pkg/`,
the root becomes a flat list that grows monotonically and expresses no
relationships — nothing says that `invoice`, `ledger` and `dunning` are one
system while `imageproxy` is not. Grouping by domain makes the seams in the
system visible in the file tree, and makes "who owns this?" answerable from the
path.

Allowing both shapes at once is worse than either alone, which is why this is a
mode and not a pair of independent toggles.

#### Interactions

- Pairs with [`dir/max-package-depth`](#dirmax-package-depth): this rule sets
  the *minimum* nesting for a package, that one sets the maximum. Together they
  pin packages to exactly one depth.
- [`dir/domain-has-no-go-files`](#dirdomain-has-no-go-files) keeps the domain
  layer a pure container.

---

### `dir/max-package-depth`

**No subdomains and no subpackages: a Go package may not nest below the depth
its root's layout mode implies.**

Depth is measured from the root, whose immediate children are depth 1:

```
pkg/billing/                     depth 1   (domain)
pkg/billing/invoice/             depth 2   (package)
pkg/billing/invoice/pdf/         depth 3   ← too deep
pkg/billing/eu/invoice/          depth 3   ← too deep (subdomain)
```

**Only package directories are counted.** An
[asset directory](#dirembedded-assets) may sit below a package without
violating this rule, because it holds no Go code:

```
✓  pkg/billing/invoice/templates/invoice.tmpl      asset dir at depth 3
✗  pkg/billing/invoice/templates/render.go         now it is a package
```

#### Configuration

```yaml
settings:
  dir/max-package-depth:
    pkg: 2         # domain + package
    cmd: 2
    internal: 2
```

With `dir/domain-layout` set to `packages` for a root, the corresponding depth
is `1`.

#### Rationale

Nesting implies a hierarchy that Go does not have. `pkg/billing/invoice/pdf` reads
as though `pdf` were part of `invoice` and somehow more private than it — but Go
has exactly one visibility boundary below the module, `internal/`, and nesting is
not it. `pdf` is equally importable from anywhere either way, so the hierarchy
communicates a restriction that does not exist.

Subdomains have the same problem one level up, and they compound: once
`pkg/billing/eu/` exists, the next team adds `pkg/billing/eu/vat/`, and the path
to a package stops being predictable.

The flat shape forces the real question — *is this a distinct responsibility?*
If yes it is a sibling package in the same domain; if no it belongs in the file
it came from.

#### Interactions

- The lower bound comes from [`dir/domain-layout`](#dirdomain-layout).
- When [`dir/max-entries`](#dirmax-entries) forces a package to split, this rule
  is what stops the split from becoming a subpackage.

---

### `dir/domain-has-no-go-files`

**A domain directory contains no `.go` files whatsoever.**

Not one. Not `doc.go`, not a shared `types.go`, not a single helper.

```
pkg/billing/
├── invoice/          ✓ package
├── ledger/           ✓ package
└── types.go          ✗ Go file in a domain directory
```

#### Configuration

None. The rule is absolute — a threshold would defeat it.

#### Rationale

The moment a domain directory holds Go code it becomes a package, and a package
that sits above its siblings is where "shared" types accumulate. Every package
in the domain then imports it, it acquires a dependency on each of them in turn,
and the domain has an import cycle waiting to happen and a file nobody can
change safely.

Keeping the layer empty of code means a domain is purely a grouping. It has no
API, so nothing can depend on it, so it cannot rot.

If a type is genuinely shared across a domain's packages, it belongs in its own
package within that domain — named for what it is, not for the fact that
several things use it.

#### Interactions

- This is what makes the domain layer in
  [`dir/domain-layout`](#dirdomain-layout) meaningful rather than decorative.
- Under `packages` mode there are no domain directories, so the rule is inert.
- **Ownership split with `dir/domain-layout`.** A depth-1 directory holding Go
  files is exactly one defect, and which one depends on whether packages sit
  beneath it. With child packages it is a domain polluted with code, and this
  rule reports it. Without them it is a package that never got a domain, and
  `dir/domain-layout` reports it. Both rules originally fired on both cases, so
  one mistake produced two findings carrying contradictory advice — move the
  code out, and move the directory in.

---

### `dir/embedded-assets`

**Files that are not Go source live in a subdirectory of the package, never
alongside the `.go` files.**

```
pkg/billing/invoice/
├── invoice.go                    ✓
├── invoice_test.go               ✓
├── templates/                    ✓ asset directory
│   ├── invoice.tmpl
│   └── receipt.tmpl
└── schema.json                   ✗ non-Go file beside the source
```

`//go:embed` reaches into subdirectories without difficulty
(`//go:embed templates`, `//go:embed templates/*`), so this costs nothing at the
call site. It cannot reach *outside* the package directory, which is why the
assets stay under the package rather than moving to a shared tree.

An asset directory must contain no `.go` files — the moment it does, it is a
package and [`dir/max-package-depth`](#dirmax-package-depth) applies to it.

#### Configuration

```yaml
settings:
  dir/embedded-assets:
    # Files permitted beside .go source despite not being Go.
    allow:
      - go.mod
      - go.sum
      - README.md
      - LICENSE
      - .gitignore
```

`testdata/` is a directory and is exempt by the Go toolchain's own convention.

#### Rationale

Go source and embedded assets are read for different reasons and changed on
different schedules, but interleaved in one listing they compete for the same
attention. Worse, a bare listing gives no signal about which files are compiled
and which are data — a reader has to open `.json` or `.tmpl` files to discover
whether they are inputs to the build, fixtures, or leftovers nobody deleted.

A named subdirectory answers that in the path: `templates/` is data, and
everything at the package level is code.

#### Interactions

- Collapses many files into one entry for
  [`dir/max-entries`](#dirmax-entries).
- Asset directories are exempt from
  [`dir/max-package-depth`](#dirmax-package-depth) precisely because they
  contain no Go code.

---

## Worked example

A tree satisfying every rule above, with `pkg: domains`, `cmd: domains`,
`max-entries: 20`, `max-package-depth: 2`:

```
repo/
├── cmd/
│   └── billing/                        domain: directories only
│       ├── invoicer/                   package main
│       │   ├── main.go
│       │   └── config/
│       │       └── defaults.yaml       assets, not beside the .go files
│       └── reconciler/                 sibling package, same domain
│           └── main.go
├── pkg/
│   ├── billing/                        domain
│   │   ├── invoice/                    package — depth 2
│   │   ├── ledger/                     sibling, not a subpackage
│   │   └── dunning/
│   └── imageproxy/                     separate domain
│       └── proxy/
├── internal/
│   └── platform/
│       └── telemetry/
├── docs/                               no Go — unconstrained
│   └── directory-organization.md
├── go.mod
└── README.md
```

And the violations each rule catches in it:

| Change | Rule violated |
| --- | --- |
| `pkg/billing/types.go` | `dir/domain-has-no-go-files` |
| `pkg/invoice/invoice.go` | `dir/domain-layout` |
| `pkg/billing/invoice/pdf/pdf.go` | `dir/max-package-depth` |
| `pkg/billing/eu/invoice/` | `dir/max-package-depth` (subdomain) |
| `tools/gen/main.go` | `dir/top-level-layout` |
| `pkg/billing/invoice/schema.json` | `dir/embedded-assets` |
| a 30-file package | `dir/max-entries` |

---

## Open questions

Decisions still to make. Each changes what gets implemented, so they should be
settled before the rules are written.

1. **Does `internal/` use domains?** The domain shape was specified as
   `<pkg|cmd>/<domain>/<package>`, which leaves `internal/` unaddressed. Drafted
   above as `internal: any` — unconstrained — but it could equally be held to
   the same domain shape. If it is, the same depth limit presumably applies.

2. **May `internal/` appear nested?** Go permits `pkg/billing/internal/` and
   treats it as private to `pkg/billing/`. That is genuinely useful for keeping
   a domain's helpers unimportable, but it conflicts with both
   `dir/max-package-depth` and the "three roots" model of
   `dir/top-level-layout`. Allow it as a special case, or forbid it?

3. **What does `dir/max-entries` count?** Specifically: do `_test.go` files
   count toward the limit, and does `testdata/` count as an entry? Counting
   tests means a well-tested package hits the limit sooner, which may punish
   the wrong thing.

4. **What is the default limit for `dir/max-entries`?** Drafted as 20. Worth
   measuring against real Quikcad repositories before fixing it, and worth
   deciding whether domains, packages, and asset directories should share one
   number.

5. **Should `dir/embedded-assets` apply to all non-Go files, or only to files
   actually named in a `//go:embed` directive?** Drafted as all non-Go files,
   since the narrow version would permit a stray `notes.txt` while rejecting an
   embedded one — but the narrow version is closer to the literal requirement
   and produces fewer findings on adoption.

6. **May asset directories nest?** `templates/email/welcome.tmpl` is natural for
   a large asset set, and harmless since none of it is Go. Assumed permitted
   above; worth confirming, along with whether `dir/max-entries` applies inside
   them.

7. **Is a single-package domain acceptable?** `pkg/billing/` containing only
   `invoice/` satisfies every rule as written, but a domain with one package is
   arguably just a package wearing an extra directory. Leave it alone, or warn?

8. **May the repository root hold `.go` files?** Drafted as no
   (`allow_go_files_at_root: false`), which forbids a root-level `doc.go` or
   `tools.go`. The `tools.go` dependency-pinning pattern is the main casualty;
   modern Go has `go.mod` tool directives instead, so this is likely fine.
