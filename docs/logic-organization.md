# Logic organization

Specification for the `logic/` rule family — the rules governing **how code is
shaped**: the size and structure of types, the interfaces they satisfy, the
types chosen to model values, and the complexity of control flow.

Where [`dir/`](directory-organization.md) constrains where code lives, `logic/`
constrains what it looks like once you open the file.

> **Status: specification only.** Nothing here is implemented. Rule IDs, config
> keys, and defaults are proposals.
>
> [Specified rules](#specified-rules) are agreed in principle and written up in
> full. [Proposed rules](#proposed-rules) need an approve/deny decision each —
> tick the boxes.

---

## The type-information problem

**Read this before approving anything.** It determines what is buildable.

The `dir/` family is purely syntactic: it reads paths and parses files, and it
works on a tree that does not compile. Several `logic/` rules cannot work that
way. "Does this type implement `fmt.Stringer`?" and "could this `any` be a type
parameter?" are questions about *types*, not about syntax, and answering them
requires the package and its dependencies to type-check.

That splits the family in two:

| Tier | Needs | Works on broken code | Cost |
| --- | --- | --- | --- |
| **syntax** | `go/parser` (already have it) | yes | milliseconds |
| **types** | `go/packages` + `go/types`, i.e. `golang.org/x/tools` | **no** | seconds to minutes; needs deps downloaded |

Of the nine rules you specified, three are type-tier:

| Rule | Tier |
| --- | --- |
| `logic/max-object-members` | syntax |
| `logic/interface-registry` | **types** |
| `logic/iota-candidate` | syntax |
| `logic/any-should-be-generic` | **types** |
| `logic/ideal-numeric-type` | **types** |
| `logic/prefer-guard-clause` | syntax |
| `logic/max-condition-operands` | syntax |
| `logic/factory-naming` | syntax |
| `logic/expand-struct-definition` | syntax |

Consequences worth deciding on deliberately:

- goorg gains its first heavy dependency (`golang.org/x/tools`), against a
  current footprint of one YAML library.
- A type-tier run needs a working module: dependencies fetched, code compiling.
  In CI that is usually true. Mid-refactor on a laptop it often is not.
- Runtime goes from "instant" to "comparable to `go build`".

**Recommended shape:** keep the two tiers explicitly separate. Syntax rules run
always. Type rules run only when loading succeeds, and a failure to load is
reported as a skipped-coverage warning rather than silently passing — a linter
that quietly checks nothing is worse than one that refuses to run. A
`--syntax-only` flag gives the fast path for editor and pre-commit use.

This is [open question 1](#open-questions).

---

## Specified rules

### `logic/max-object-members`

**A type may declare at most N members.** A member is a struct field or a
method with that type as receiver.

#### Configuration

```yaml
settings:
  logic/max-object-members:
    fields: 12
    methods: 15
    # Count fields and methods against one combined budget instead.
    combined: 0          # 0 disables the combined check
    # Embedded types count as one member each, not as their expanded member set.
    count_embedded: true
    # Unexported fields of a type whose zero value is the API (e.g. a builder)
    # can legitimately be numerous.
    exclude_unexported_fields: false
```

#### Example

```go
// ✗ 19 fields — this is three types wearing one name
type Server struct {
    addr, cert, key                     string
    readTimeout, writeTimeout           time.Duration
    maxConns, maxIdle, maxHeaderBytes   int
    logger                              *slog.Logger
    metrics                             *metrics.Registry
    // ...
}

// ✓ the seams were already there
type Server struct {
    listen  ListenConfig
    limits  Limits
    obs     Observability
}
```

#### Rationale

A type's member count is the most reliable proxy for how many responsibilities
it has. Fields are the state something owns; when a type owns nineteen pieces of
state, no single method touches most of them, and the type is really several
types that happen to share a struct literal.

The count also bounds what a reader must hold in their head to reason about any
one method — every field is potentially mutated by every method, so the
invariant surface grows with the product of the two.

#### Detection

Syntactic. Count `ast.Field` entries in the struct type (expanding grouped
declarations such as `a, b, c string` to three), and count `FuncDecl`s with a
receiver of that type across the whole package, including other files.

#### Interactions

- Overlaps in spirit with `dir/max-entries` — same forcing function, different
  unit.
- A type split to satisfy this must not become a subpackage;
  `dir/max-package-depth` forbids that.

---

### `logic/interface-registry`

**Maintain a registry of interfaces, and check every type against it.**

The registry holds the standard-library interfaces worth caring about plus any
project-local ones. For each registered interface and each named type, the rule
reports two situations.

**a. Near miss — the high-value case.** The type has a method matching a
registry method by *name* but not by *signature*, so it silently fails to
implement the interface:

```go
func (s Status) String() (string, error)   // ✗ not fmt.Stringer
```

Nothing about this is a compile error. `fmt.Printf("%v", status)` prints the
underlying value and the author never learns why. This class of bug is invisible
until someone reads output carefully, and it is exactly what a type-aware linter
should catch.

**b. Implements but does not declare.** The type satisfies a registered
interface structurally, but nothing pins that:

```go
type Status int
func (s Status) String() string { ... }

// ✗ nothing records that Status is meant to be a fmt.Stringer,
//   so a later signature change breaks it silently
```

```go
var _ fmt.Stringer = Status(0)   // ✓ compile-time assertion
```

#### Configuration

```yaml
settings:
  logic/interface-registry:
    # Interfaces every type is checked against.
    interfaces:
      - fmt.Stringer
      - error
      - encoding.TextMarshaler
      - encoding.TextUnmarshaler
      - encoding/json.Marshaler
      - encoding/json.Unmarshaler
      - io.Reader
      - io.Writer
      - io.Closer
      - sort.Interface
      - github.com/Quikcad/example/pkg/billing/ledger.Poster
    # Which checks to run.
    report_near_miss: true
    report_missing_assertion: true
    # Only require assertions for exported types — unexported ones are
    # verified by their own package's compilation.
    assertions_for_exported_only: true
```

#### Rationale

Interfaces in Go are satisfied implicitly, which is the language's best feature
and its worst failure mode. Implicit satisfaction means nothing in the source
says "this type is meant to be a `Stringer`", so nothing breaks when it stops
being one. A renamed method or a changed signature turns an interface
implementation into an ordinary method, and every call site that relied on it
falls back to a default behaviour that usually still compiles and often still
runs.

An explicit assertion converts that silent behavioural regression into a build
failure at the point of the change. The near-miss check catches the same class
of bug before the assertion is ever written.

#### Detection

Type-tier. Requires `go/types` to compute method sets and compare signatures,
and to resolve registry entries to `*types.Interface`. Near-miss detection
compares by method name, then diffs the signature to produce the message.

#### Interactions

- Feeds several proposed rules — `logic/enum-missing-string`,
  `logic/interface-at-consumer`, `logic/single-impl-interface`.

---

### `logic/iota-candidate`

**A run of related constants with consecutive literal values should use
`iota`.**

```go
// ✗
const (
    StatusPending  = 0
    StatusActive   = 1
    StatusArchived = 2
)

// ✓
type Status int

const (
    StatusPending Status = iota
    StatusActive
    StatusArchived
)
```

#### Configuration

```yaml
settings:
  logic/iota-candidate:
    # Minimum run length before the rule fires.
    min_constants: 3
    # Also flag runs that start at 1, or step by a constant amount, which
    # iota expresses as `iota + 1` and `iota * 8`.
    allow_offsets: true
    # Also require the run to have a named type rather than untyped constants.
    require_named_type: true
```

#### Rationale

Hand-numbered constants have to be renumbered by hand. Inserting a value in the
middle means editing every line below it, and the one that gets missed produces
two constants with the same value — which is not a compile error, and which
turns a `switch` into a silently unreachable branch.

`iota` also creates the pressure to declare a named type, and a named type is
what makes the enum checkable at all: `func SetStatus(s Status)` cannot be
passed a stray `int`, and `Status` can carry a `String` method.

#### Detection

Syntactic. Walk `GenDecl` const blocks; find runs of ≥ `min_constants` specs
whose values are integer literals forming an arithmetic sequence.

#### Interactions

- Pairs with proposed `logic/enum-missing-string` and
  `logic/enum-switch-exhaustive`, both of which need a named enum type to work
  at all.

---

### `logic/any-should-be-generic`

**A function using `any` where a type parameter would preserve type information
should use a type parameter.**

```go
// ✗ caller must type-assert the result back
func First(items []any) any

// ✓
func First[T any](items []T) T
```

#### Configuration

```yaml
settings:
  logic/any-should-be-generic:
    # Report `any` in parameters, results, or both.
    check_params: true
    check_results: true
    # Skip functions whose `any` is genuinely heterogeneous — it flows into
    # fmt, reflect, or encoding/json.
    ignore_reflective: true
    # Skip exported functions, where changing the signature is breaking.
    exported_only: false
```

#### Rationale

`any` erases the caller's type and makes them assert it back, moving a
compile-time guarantee to a runtime panic. The cases that read as "this must be
`any`" are usually the ones where a single type parameter threads the type
through untouched — the function never inspects the value, it only moves it.

The genuinely heterogeneous cases exist (`fmt.Println`, JSON decoding into an
unknown shape), which is why the rule is a heuristic and should ship as a
warning.

#### Detection

Type-tier. `any` in a signature is syntactic, but deciding whether it *could* be
a type parameter requires knowing whether the function body ever inspects the
dynamic type — a type switch, type assertion, or reflection use means it cannot.
Approximate as: no type assertion, no type switch, no `reflect` import, and the
`any` appears in more than one position (or in a result), meaning the type
genuinely flows through.

#### Interactions

- Proposed `logic/constraint-too-wide` is the follow-on: once it is
  `[T any]`, is `any` the right *constraint*?

---

### `logic/ideal-numeric-type`

**Choose the numeric type that minimizes conversions at the value's use sites.**

For each numeric variable, field, parameter and result, collect every use.
Under each candidate type, count the explicit conversions its uses would
require. Report when a different candidate would need strictly fewer.

```go
// ✗ declared float64, but every use is float32
func scale(v float64) {
    tex.SetScale(float32(v))        // conversion
    mesh.Resize(float32(v), 1.0)    // conversion
    buf.PutFloat32(float32(v))      // conversion
}

// ✓ zero conversions
func scale(v float32) { ... }
```

#### Configuration

```yaml
settings:
  logic/ideal-numeric-type:
    # Candidate sets. A value is only ever compared within its own family.
    families:
      float: [float32, float64]
      signed: [int, int8, int16, int32, int64]
      unsigned: [uint, uint8, uint16, uint32, uint64]
    # Minimum conversions saved before reporting.
    min_savings: 2
    # Never propose narrowing a float64 to float32, which loses precision even
    # when it removes conversions.
    allow_narrowing: false
    # Exported signatures are API; changing them is breaking.
    exported_only: false
```

#### Rationale

A numeric type declared once and converted at every use is a type that was
chosen by habit rather than by the code that consumes it. Each conversion is
noise at the call site, and each is a place where a narrowing conversion can
silently lose precision or overflow without any diagnostic.

Counting conversions turns "what type should this be?" from a matter of taste
into a measurement: the right type is the one the surrounding code already
speaks.

#### Detection

Type-tier. Requires `go/types` to resolve the type of every use site and to
identify conversion expressions (as distinct from same-named function calls).

**This rule needs the most care of the seven.** Conversion count is a proxy, not
the answer — precision requirements, memory layout in large arrays, and the
demands of an external API are all invisible to it. `allow_narrowing: false`
exists because "fewer conversions" must never be allowed to argue for silently
losing precision. It should ship as a `warning`, never `error`.

#### Interactions

- Proposed `logic/lossy-conversion` covers the correctness half of the same
  territory; this rule covers the ergonomics half.

---

### `logic/prefer-guard-clause`

**When a function body is wholly wrapped in a conditional, invert it into a
guard clause.**

```go
// ✗ the whole body is one level deeper than it needs to be
func process(r *Record) error {
    if r != nil {
        if r.Valid() {
            // 40 lines
        }
    }
    return nil
}

// ✓
func process(r *Record) error {
    if r == nil || !r.Valid() {
        return nil
    }
    // 40 lines at the top level
}
```

#### Configuration

```yaml
settings:
  logic/prefer-guard-clause:
    # Minimum statements in the wrapped block before it is worth inverting.
    min_body_statements: 3
    # Also apply inside loop bodies, where `continue` is the guard.
    check_loops: true
    # Only fire when the conditional is the sole statement in the body.
    sole_statement_only: true
```

#### Rationale

A guard clause states the precondition and leaves; a wrapping conditional states
the precondition and then makes the reader carry it for forty lines. The
difference matters most at the bottom of a long function, where the wrapped form
requires scrolling back to recall which branch you are in.

The wrapped form also pushes every subsequent addition one level deeper, so it
compounds — this is how a function reaches five levels of indentation without
anyone deciding it should.

#### Detection

Syntactic. A function (or loop) body whose statement list is a single `IfStmt`
with no `else`, whose block holds at least `min_body_statements` statements.
Nested wrappers collapse into one finding with a combined condition.

#### Interactions

- Directly reduces the depth measured by proposed `logic/max-nesting-depth`.
- The inverted condition may then exceed
  [`logic/max-condition-operands`](#logicmax-condition-operands) — the fix for
  *that* is to name the predicate, not to un-invert the guard.

---

### `logic/max-condition-operands`

**A condition may combine at most N operands with `&&` and `||`.**

```go
// ✗ six operands, mixed operators, no parentheses to help
if u != nil && u.Active && !u.Banned && (u.Role == Admin || u.Role == Owner) && u.MFA {

// ✓ the predicate has a name and the name says what it means
if u.CanAdminister() {
```

#### Configuration

```yaml
settings:
  logic/max-condition-operands:
    max: 4
    # Count the whole boolean expression tree, not just the top level.
    recursive: true
    # Report mixing && and || at the same nesting level without parentheses,
    # regardless of the count.
    require_parens_on_mixed: true
    # Apply to `for` conditions and `switch` case guards as well as `if`.
    include_loops: true
```

#### Rationale

Boolean expressions are read by evaluating them, and people cannot evaluate six
terms without losing track of one. The failure is not that the condition is hard
to read; it is that it is hard to read *incorrectly-but-plausibly* — the reader
believes they understood it, and the term they dropped is the one that mattered.

Naming a predicate replaces the evaluation with a lookup, and gives the
condition a place to be unit-tested. Mixed `&&`/`||` without parentheses is
called out separately because Go's precedence is correct but not obvious, and
the parentheses cost nothing.

#### Detection

Syntactic. Count the leaves of the `BinaryExpr` tree under `&&`/`||` in each
condition.

#### Interactions

- Inverting a guard per
  [`logic/prefer-guard-clause`](#logicprefer-guard-clause) can push a condition
  over this limit. Extracting a named predicate satisfies both.

---

### `logic/factory-naming`

**A factory returning a value type is named `Make*`. A factory returning a
pointer is named `New*`.**

```go
type Config struct { ... }

func MakeConfig() Config           // ✓ value
func NewConfig() *Config           // ✓ pointer

func NewConfig() Config            // ✗ returns a value, named New
func MakeConfig() *Config          // ✗ returns a pointer, named Make
```

The bare forms `New` and `Make` are equally acceptable, and preferred where the
package name already carries the type — `billing.New()` reads better than
`billing.NewBilling()`.

A trailing `error` is ignored when classifying: `func NewClient() (*Client,
error)` is a pointer factory. The first non-`error` result decides.

#### Configuration

```yaml
settings:
  logic/factory-naming:
    value_prefix: Make
    pointer_prefix: New
    # Interfaces are neither value nor pointer at the syntax level. Callers
    # cannot take their address, so they are classified as pointer-like.
    interface_prefix: New
    # Scope. `prefixed` checks only functions already named New*/Make*.
    # `all-factories` additionally requires every function returning a
    # package-local type to carry one of the prefixes — see Detection.
    scope: prefixed
    # Names exempt under `all-factories`, being established Go idiom.
    allow_names: [Parse, Open, Dial, Must, From, Load, Decode, Unmarshal]
```

#### Rationale

Whether a constructor hands back a value or a pointer determines everything a
caller does next: whether assignment copies or aliases, whether a `nil` check is
required, whether the zero value is meaningful, whether the result is safe to
share across goroutines. Today that answer lives in the signature, which means
reading it — and at a call site like `cfg := pkg.NewConfig()` the signature is
not on screen.

Encoding it in the prefix moves the answer to the call site. `cfg :=
billing.MakeConfig()` says a copy was made and there is nothing to nil-check;
`cli := billing.NewClient()` says the opposite. The convention costs one word
and removes a lookup from every read.

It also makes the inconsistent cases visible. A package with both
`NewLedger() Ledger` and `NewEntry() *Entry` has two different ownership models
under one prefix, and nobody noticed.

#### Detection

Syntactic, with one caveat.

Classifying the result is straightforward: a `*T` result is a pointer factory, a
bare `T` is a value factory. Deciding whether `T` is an interface requires
resolving `T` — cheap for a type declared in the same package (scan the
package's `TypeSpec`s), but not resolvable from syntax alone when `T` is
imported from elsewhere. Those cases either fall back to `interface_prefix` by
assumption or get skipped; **this is worth deciding explicitly**
([open question 7](#open-questions)).

The `scope` setting is the real design fork, and the main false-positive risk:

- `prefixed` (default) only checks functions **already** named `New*` or
  `Make*`, verifying the prefix matches what they return. It cannot produce a
  false positive on idiomatic code, because it only judges names the author
  already chose.
- `all-factories` additionally demands that *every* function returning a
  package-local type carry one of the two prefixes. That flags `Parse`, `Open`,
  `Dial`, `FromString`, `MustCompile` — all established Go idiom, and all
  correct as written. The `allow_names` list exists to make this survivable, but
  it will never be complete.

Ship `prefixed`. `all-factories` is available for a team that wants it and is
prepared to curate the allowlist.

#### Interactions

- Generic factories (`func NewStore[T any]() *Store[T]`) classify on the
  instantiated result type; the type parameters do not affect the prefix.
- Methods are not factories and are out of scope, even when they return a new
  value — `(*Builder).Build()` is not renamed to `MakeBuild`.

---

### `logic/expand-struct-definition`

**A struct type with at least one field is written across multiple lines, one
field per line.**

```go
// ✗
type Point struct{ X, Y int }

// ✓
type Point struct {
	X int
	Y int
}
```

`gofmt` does not do this. It preserves whichever form the author wrote, so a
one-line struct survives formatting indefinitely and this is a genuine gap
rather than a duplicate of the formatter.

**`struct{}` is always exempt.** The empty struct is a unit value, not a record,
and `map[string]struct{}`, `chan struct{}` and `struct{}{}` are load-bearing Go
idiom that must never be flagged.

#### Configuration

```yaml
settings:
  logic/expand-struct-definition:
    # Apply to anonymous struct types too — table-test row types, struct-typed
    # variables, struct-typed parameters.
    include_anonymous: true
    # Require one field per line, not merely a multi-line struct. This also
    # forbids `X, Y int` on a shared line.
    one_field_per_line: true
```

#### Rationale

A one-line struct is a struct that has not been budgeted for. Every field added
later has to either extend the line or convert the declaration, and the
conversion is a diff that touches every existing field — so the line grows
instead, and `type Config struct{ Host string; Port int; TLS bool }` is how it
ends up.

The multi-line form also makes the diff of adding a field exactly one line,
which is what makes struct changes reviewable. In the collapsed form, adding a
field rewrites the whole declaration and code review loses the ability to see
what actually changed.

`one_field_per_line` extends the same reasoning to grouped fields: `X, Y int`
saves a line and costs the ability to document, tag, or change the type of `X`
without touching `Y`.

#### Detection

Syntactic and exact. For each `ast.StructType`, compare the line of
`Fields.Opening` against the line of `Fields.Closing`. Equal lines with
`Fields.NumFields() > 0` is a violation. Under `one_field_per_line`, compare the
line of consecutive `Fields.List` entries, and check `len(field.Names) > 1` for
the grouped case.

No type information, no heuristic, no false positives.

#### Interactions

- Expanding a struct makes its field count visible at a glance, which is what
  [`logic/max-object-members`](#logicmax-object-members) then measures.
- Related but distinct from proposed rule 7,
  `logic/struct-field-alignment`, which concerns field *order* rather than
  layout.

---

## Proposed rules

Fifty candidates for approve/deny. Tick to approve.

Tags: **syntax** = AST only, cheap, works on broken code · **types** = needs
`go/types` (see [the type-information problem](#the-type-information-problem)) ·
**exists** = a well-known linter already does this, so approving means deciding
we want it in one tool rather than that it is unavailable.

### Type and data modeling

- [x] 1. **`logic/boolean-field-count`** · syntax — More than N `bool` fields in one struct. Three booleans are eight states, most of which are invalid; a named state enum makes the legal set explicit.
- [x] 2. **`logic/stringly-typed-enum`** · syntax — A run of `string` constants used as an enumeration without a named string type. `type Status string` costs one line and makes the set checkable.
- [x] 3. **`logic/primitive-obsession`** · types — The same primitive appears N+ times across a package's signatures in the same role (`userID string`, `orgID string`). Named types make transposed arguments a compile error.
- [x] 4. **`logic/duration-as-number`** · types — An `int`/`int64` named `*Timeout`, `*Interval`, `*TTL` that should be `time.Duration`. Unit confusion in timeouts is a recurring production bug.
- [ ] 5. **`logic/map-as-struct`** · syntax — `map[string]string`/`map[string]any` with a fixed set of literal keys used as an ad-hoc record. A struct gets field checking and documentation.
- [x] 6. **`logic/exported-embedded-mutex`** · types — An exported struct embedding `sync.Mutex` promotes `Lock`/`Unlock` into its public API, letting callers break the type's invariants. Name the field.
- [ ] 7. **`logic/struct-field-alignment`** · types — Field ordering wastes padding beyond N bytes. Genuinely useful only for types allocated in bulk; noisy elsewhere. (**exists**: `fieldalignment`)
- [x] 8. **`logic/zero-value-unusable`** · types — A type whose zero value panics or misbehaves, with no constructor documented. Go code assumes `var x T` works.
- [x] 9. **`logic/pointer-to-slice-or-map`** · syntax — `*[]T` or `*map[K]V` in a signature. Almost always a misunderstanding of Go's reference semantics.
- [ ] 10. **`logic/mutable-global`** · syntax — Package-level `var` that is not a sentinel error or a genuine constant. Global mutable state defeats parallel tests and hides coupling.

### Interfaces and abstraction

- [ ] 11. **`logic/interface-size`** · syntax — An interface declaring more than N methods. Large interfaces are hard to implement and impossible to fake in a test.
- [x] 12. **`logic/interface-at-consumer`** · types — An interface declared in the package that implements it rather than the one that consumes it. Producer-side interfaces force every consumer into one abstraction.
- [ ] 13. **`logic/single-impl-interface`** · types — An interface with exactly one implementation and no test double. Often speculative indirection; sometimes a deliberate seam, so this should be a warning.
- [x] 14. **`logic/accept-interfaces-return-structs`** · types — An exported function returning an interface where a concrete type would do. Returning interfaces hides fields the caller may legitimately need.
- [ ] 15. **`logic/unused-interface-method`** · types — A method on a project-local interface that no call site invokes through that interface. Dead surface that every implementer still has to write.
- [ ] 16. **`logic/interface-name-suffix`** · syntax — Enforce or forbid the `-er` convention consistently. Worth having only if the house standard picks a side.
- [x] 17. **`logic/empty-interface-field`** · syntax — A struct field typed `any`. Same erasure problem as `any` parameters, but longer-lived.

### Generics

- [x] 18. **`logic/unused-type-parameter`** · types — A type parameter appearing exactly once in a signature. It is not doing generic work; it is an unconstrained hole.
- [x] 19. **`logic/constraint-too-wide`** · types — `[T any]` where the body requires `comparable` or `cmp.Ordered`. A tight constraint documents the contract and improves the error message at the call site.
- [x] 20. **`logic/single-instantiation-generic`** · types — A generic function or type instantiated with exactly one type argument across the whole module. Generality nobody asked for.
- [ ] 21. **`logic/reflection-over-generics`** · types — `reflect` used in a way a type parameter would replace. Reflection is slower and moves errors to runtime.

### Constants and enums

- [ ] 22. **`logic/enum-missing-string`** · types — An `iota` enum type with no `String()` method. Without it, every log line and error message prints an integer.
- [x] 23. **`logic/enum-switch-exhaustive`** · types — A `switch` over an enum type missing cases and lacking a `default`. Adding an enum value should not silently skip a branch. (**exists**: `exhaustive`)
- [ ] 24. **`logic/enum-zero-value-unnamed`** · syntax — An `iota` enum starting at 0 with no constant for the zero value, so the zero value is a valid-looking invalid state. Add an explicit `Unknown`/`Unspecified`.
- [ ] 25. **`logic/magic-number`** · syntax — An unnamed numeric literal outside a small allowlist (`0`, `1`, `2`, powers of two). High false-positive rate; needs a generous allowlist to be tolerable.
- [x] 26. **`logic/duplicate-const-value`** · syntax — Two constants in one block sharing a value. Usually a botched hand-renumbering — the exact failure `logic/iota-candidate` prevents.

### Numeric correctness

- [x] 27. **`logic/float-equality`** · types — `==` or `!=` between floating-point values. Almost always wrong; wants an epsilon comparison.
- [ ] 28. **`logic/lossy-conversion`** · types — A narrowing numeric conversion (`int64`→`int32`, `int`→`uint`) with no preceding range check. Silent truncation and sign flips.
- [ ] 29. **`logic/unsigned-underflow`** · types — Subtraction on an unsigned type without a guard. `uint(0) - 1` is a very large number, not a negative one.
- [x] 30. **`logic/integer-division-to-float`** · types — Integer division whose result is immediately converted to a float. `float64(a/b)` truncates before converting; the author meant `float64(a)/float64(b)`.

### Control flow and complexity

- [x] 31. **`logic/max-nesting-depth`** · syntax — Block nesting beyond N levels. The single best predictor of a function nobody wants to touch.
- [ ] 32. **`logic/cyclomatic-complexity`** · syntax — Independent paths through a function above N. (**exists**: `gocyclo`)
- [x] 33. **`logic/cognitive-complexity`** · syntax — Weights nesting more heavily than branch count, so it tracks readability better than cyclomatic complexity. (**exists**: `gocognit`)
- [x] 34. **`logic/else-after-return`** · syntax — An `else` block after a branch that returns. The `else` is redundant and adds a level. (**exists**: `golint`/`revive`)
- [x] 35. **`logic/if-chain-to-switch`** · syntax — Three or more `else if` branches testing the same operand. A `switch` states the shape and enables exhaustiveness checking.
- [x] 36. **`logic/negated-condition`** · syntax — `if !cond { A } else { B }`. Flipping removes the negation and reads forward.
- [x] 37. **`logic/single-case-switch`** · syntax — A `switch` with one case. Either an `if`, or a missing case.
- [x] 38. **`logic/identical-branches`** · syntax — Two branches of the same `if`/`switch` with identical bodies. Either a copy-paste bug or a condition that does not matter.
- [x] 39. **`logic/empty-branch`** · syntax — A branch with an empty body and no explanatory comment.
- [x] 40. **`logic/max-function-lines`** · syntax — Function length cap. Blunt but effective; overlaps `org/max-file-lines` at a finer grain.
- [x] 41. **`logic/max-function-params`** · syntax — More than N parameters. Past four, call sites become positional puzzles; wants a config struct.
- [x] 42. **`logic/max-return-values`** · syntax — More than N results, `error` excluded. Callers cannot remember which is which; wants a named struct.
- [x] 43. **`logic/naked-return`** · syntax — A bare `return` in a function longer than N lines, where the reader can no longer see what is being returned. (**exists**: `nakedret`)
- [x] 44. **`logic/loop-invariant-computation`** · types — An expression inside a loop that does not depend on the loop. Hoisting it is clearer and usually faster.
- [ ] 45. **`logic/boolean-parameter`** · syntax — A `bool` parameter on an exported function. `Fetch(id, true)` is unreadable at the call site; wants two functions or an option type.

### Errors

- [x] 46. **`logic/unwrapped-error`** · types — `fmt.Errorf` including an error via `%v` instead of `%w`, breaking `errors.Is`/`errors.As` for every caller above. (**exists**: `errorlint`)
- [x] 47. **`logic/error-equality`** · types — `err == ErrFoo` rather than `errors.Is(err, ErrFoo)`. Fails the moment anyone in the chain wraps. (**exists**: `errorlint`)
- [x] 48. **`logic/panic-outside-main`** · types — `panic` in a package outside `cmd/`. A library that panics takes the caller's process down over a decision that was theirs to make.
- [x] 49. **`logic/error-string-style`** · syntax — Error strings that are capitalized or end in punctuation, which reads badly once wrapped into a longer chain. (**exists**: `staticcheck ST1005`)
- [x] 50. **`logic/context-in-struct`** · types — `context.Context` stored as a struct field rather than passed as a parameter. Ties the context's lifetime to the object's, defeating cancellation.

---

## Open questions

1. **Do we take on `go/types`?** Three of the nine specified rules and roughly
   half the proposals need it. The alternatives are: build both tiers; ship
   syntax-only and drop the type-tier rules; or ship syntax-only first and add
   the type tier as a second, opt-in mode. This is the largest decision in the
   document — see [the type-information problem](#the-type-information-problem).

2. **What happens when type loading fails?** Recommended above: report skipped
   coverage rather than pass silently. That means a repository with a broken
   dependency gets findings it cannot act on, which is annoying but honest.

3. **Do we reimplement what `golangci-lint` already does?** Eight proposals are
   tagged **exists**. Reimplementing gives one tool, one config, one output
   format; not reimplementing keeps goorg small and focused on what nothing else
   checks. A defensible position is to deny all eight and document the
   `golangci-lint` config that covers them.

4. **Is `logic/` the right family name,** alongside the existing `dir/`, `org/`
   and `pat/`? Several proposals here (naming, error style) arguably belong in
   `pat/`, and the boundary between "pattern correctness" and "logic
   organization" is not currently sharp.
   `logic/expand-struct-definition` sharpens the question: it is a rule about
   declaration *layout*, adjacent to `gofmt`, and it sits oddly in a family
   otherwise concerned with structure and complexity. `logic/factory-naming` is
   a naming convention, which is squarely `pat/` territory. Both are specified
   here because that is where they were raised; moving them is cheap now and
   expensive once written.

5. **What default severity for heuristic rules?** `logic/ideal-numeric-type`,
   `logic/any-should-be-generic` and `logic/single-impl-interface` are
   judgement calls that will sometimes be wrong. Shipping them as `error` makes
   people disable them; as `warning` they may be ignored. A third state —
   advisory findings excluded from the exit code — may be worth adding.

6. **Should any rule offer autofix?** `logic/iota-candidate`,
   `logic/else-after-return`, `logic/negated-condition` and especially
   `logic/expand-struct-definition` are mechanical rewrites — the last one is
   purely positional and could not get it wrong. goorg currently promises never
   to modify source; a `--fix` flag would be a deliberate reversal of that.

7. **How does `logic/factory-naming` classify an imported result type?**
   Deciding whether `func NewThing() Thing` returns a value or an interface
   requires knowing what `Thing` is. When it is declared in the same package
   that is a syntactic lookup; when it is imported it is not. Options: skip
   imported result types entirely (safe, leaves a gap), assume non-pointer
   imported types are values (wrong for interfaces such as `error`), or promote
   the rule to the type tier (correct, but drags a syntax-tier rule across the
   line drawn in [the type-information problem](#the-type-information-problem)).
