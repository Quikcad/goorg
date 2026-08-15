# Adopting goorg

A codebase that predates the standard will not pass on day one, and that is not
a reason to weaken the standard. This is how to turn 400 findings into a number
that only goes down.

---

## The shape of the problem

goorg reports two kinds of thing, and they need different treatment:

- **Mechanical violations** — a file over its budget, a struct on one line, a
  hand-numbered enum. Large in number, each cheap to fix, and safe to fix in
  bulk.
- **Structural violations** — a package in the wrong root, a type whose methods
  are scattered, a global that should be a singleton. Small in number,
  individually expensive, and each one a decision somebody has to make.

Running everything at `error` on day one buries the second kind under the first.

---

## Step 1: find out where you stand

```sh
goorg check ./... --fail-on=warning --max-warnings=99999 --format=json > baseline.json
```

Nothing fails. The JSON is a record you can diff against later.

For the shape of it:

```sh
goorg check ./... --brief | tail -5
```

The last line names every rule that fired and how many times. That is the list
to triage, and it is almost always much shorter than the finding count suggests.

---

## Step 2: turn off what you are not ready for

Set the noisy rules to `warning`, or `off`, in `.goorg.yaml`. Be honest about
which: a rule at `warning` that nobody ever fixes is a rule that is `off` while
pretending otherwise.

```yaml
version: 1

rules:
  # Adopted. These fail the build.
  "*": error

  # Not yet. Each of these has an owner and a date.
  org/consumer-locality: "off"      # 140 findings, needs a package-by-package pass
  logic/max-function-lines: warning # 31 findings, fixing opportunistically
```

Turning a rule off is better than lowering the whole set to `warning`. The rules
you have adopted keep their teeth.

---

## Step 3: ratchet

Two knobs, and they compose:

```sh
# Fail on errors only; let warnings accumulate but cap them.
goorg check ./... --max-warnings=140

# Then lower the ceiling as the count falls. It can never rise again.
goorg check ./... --max-warnings=120
```

`--max-warnings` in CI is the ratchet: new code cannot add warnings, because the
count is already at the limit. Lower it whenever someone clears a batch.

`--fail-on=warning` is the end state — when a rule's count reaches zero, promote
it to `error` in the config and drop it out of the warning budget.

---

## Step 4: fix in the right order

Fix the mechanical rules first, in bulk, in their own commits. They are large in
number and near-zero in risk, and clearing them makes the structural findings
visible.

Roughly cheapest first:

| Rule | Why it is cheap |
| --- | --- |
| `pat/expand-struct-definition` | Purely positional; no behaviour changes |
| `logic/iota-candidate` | Mechanical, and the compiler checks you |
| `logic/duplicate-const-value` | Usually a real bug, and always a small fix |
| `org/member-order` | Moving declarations; no behaviour changes |
| `org/private-functions-last` | Same |
| `logic/max-condition-operands` | Extract a named predicate |
| `logic/prefer-guard-clause` | Invert and unindent |

Leave these for last — each is a design decision, not an edit:

`dir/domain-layout`, `dir/top-level-layout`, `org/type-cohesion`,
`org/globals-singleton-only`, `logic/interface-at-consumer`.

---

## Step 5: use the fast tier while you work

The type tier costs about thirty times the syntax tier, because it type-checks
the module. While iterating:

```sh
goorg check ./... --syntax-only    # 39 of 55 rules, milliseconds
```

CI should run the full set. `--syntax-only` announces what it skipped rather
than passing quietly, so it cannot be mistaken for a clean run.

---

## Suppressing, honestly

Some findings are correct about the shape and wrong about the instance. Say so
in the source, with a reason:

```go
//goorg:ignore logic/iota-candidate — the literal values are the published contract, not an ordering
const (
	ExitOK       = 0
	ExitFindings = 1
	ExitError    = 2
)
```

The reason is mandatory; a directive without one is itself an error. A
suppression that stops matching anything is reported as stale, so they do not
accumulate silently.

If you are writing the same reason for the fifth time, the rule's default is
wrong for your codebase — configure it rather than repeating yourself.

---

## Where goorg stops

goorg deliberately does not check what `golangci-lint` already checks well. Nine
rules were proposed and denied for exactly that reason
([D7](decisions.md#d7--disposition-of-the-50-proposals)); run both tools, with
this covering the gap:

```yaml
# .golangci.yml — the checks goorg deliberately does not duplicate
linters:
  enable:
    - errorlint      # %w vs %v, and err == ErrFoo instead of errors.Is
    - exhaustive     # switch over an enum missing cases
    - gocognit       # cognitive complexity
    - gocyclo        # cyclomatic complexity
    - nakedret       # bare return in a long function
    - revive         # else after return, and much else
    - govet          # fieldalignment lives here
    - staticcheck    # ST1005 error string style

linters-settings:
  gocognit:
    min-complexity: 20
  gocyclo:
    min-complexity: 15
  govet:
    enable:
      - fieldalignment
```

goorg checks the layer above: where code lives, how it is split, and what shape
it has. `golangci-lint` checks what a statement does wrong. Neither replaces the
other, and `gofmt` remains authoritative on whitespace — no goorg rule reports
on anything gofmt would rewrite, and a test harness proves it.
