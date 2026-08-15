# File and package organization

Specification for the `org/` rule family — the rules governing **how code is
distributed across the files of a package**: what order declarations appear in,
how much may live in one file, which declarations belong together, and how
package-level state is contained.

Where [`dir/`](directory-organization.md) constrains where a package lives and
[`logic/`](logic-organization.md) constrains what a declaration looks like,
`org/` constrains the file it lands in.

> **Status: specification only.** Nothing here is implemented. Rule IDs, config
> keys, and defaults are proposals. See [Open questions](#open-questions).

---

## The canonical file

Every ordering rule below is a constraint on this shape.

```go
// Package registry resolves handlers by name.
package registry

import (
	"fmt"
	"sync"
)

// 1. enums — the named type and its constant block together
type Status int

const (
	StatusUnknown Status = iota
	StatusActive
	StatusRetired
)

// 2. package-level variables — one block each, never grouped
var ErrNotFound = fmt.Errorf("registry: not found")

var _ fmt.Stringer = Status(0)

// 3. interfaces (when not in a file of their own)
type Handler interface {
	Handle(name string) error
}

// 4. structs, each followed by its factory and then its methods
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Handler
}

func NewRegistry() *Registry { ... }        // factory

func (r *Registry) Register(...) error { ... }   // exported methods
func (r *Registry) Lookup(...) (Handler, bool) { ... }

func (r *Registry) evict(...) { ... }            // unexported methods

// 5. exported functions
func Normalize(name string) string { ... }

// 6. unexported functions — always last
func validate(name string) error { ... }
```

Singleton files follow a different order — see
[`org/singleton-layout`](#orgsingleton-layout).

---

## The what-if problem

**Read this before approving [`org/consumer-locality`](#orgconsumer-locality).**
It is that rule's central difficulty, and it is an engine capability, not a
detail of one check.

You asked for consumer-locality with an escape hatch: move a declaration to the
file that uses it, *unless* moving it would cause another error. That escape
hatch is unlike anything else specified so far. Every rule to date answers a
question about the tree as it exists. This one answers a question about a tree
that does not exist yet:

> If `validate` moved from `parser.go` to `lexer.go`, would `lexer.go` then
> exceed its function budget, break `org/type-cohesion`, or violate
> `org/member-order`?

Two ways to build it:

**(a) The rule hard-codes the budgets.** `org/consumer-locality` reads the
configured limits of `org/max-functions-per-file`, `org/max-public-functions`,
`org/max-private-functions` and `org/max-file-lines`, and simulates the move
itself. Simple to write. Couples one rule to five others, and silently goes
stale the moment a sixth budget rule is added — the failure mode is a
recommendation that creates a new violation, which is the worst possible output
for a linter that is telling you to move code.

**(b) The engine gains a what-if pass.** A rule may emit a *proposed relocation*
rather than a finding. The engine applies it to an in-memory copy of the
project, re-runs the `org/` family, and keeps the finding only if the total
violation count does not increase. General, correct, and extends to any future
rule for free. It is also a substantial piece of machinery: rules must become
re-runnable against a mutated project model, and the engine needs a cycle guard
for the case where A wants to move to B's file and B wants to move to A's.

**Recommendation: (b), but not first.** Ship `org/consumer-locality` reporting
only the unambiguous case — a declaration whose sole consuming file is another
file, where the target file is under every budget with room to spare. That
subset needs no what-if machinery and covers most real findings. Add the general
mechanism when the narrow version proves it is worth it.

This is [open question 1](#open-questions).

---

## Ordering within a file

### `org/member-order`

**Declarations appear in the canonical order: enums, package variables,
interfaces, then each struct followed by its factory and its methods, then
functions.**

The rule governs the order of declaration *kinds*. The exported/unexported split
within the function section is
[`org/private-functions-last`](#orgprivate-functions-last).

#### Configuration

```yaml
settings:
  org/member-order:
    order: [enums, vars, interfaces, structs, functions]
    # Within the struct section: keep each type with its own factory and
    # methods (`per-type`), or group all structs, then all factories, then all
    # methods (`by-kind`).
    grouping: per-type
    # Each package-level var gets its own `var` block rather than sharing a
    # grouped `var (...)` declaration.
    separate_var_blocks: true
    # Unexported methods after exported ones, within each type's method run.
    unexported_methods_last: true
```

#### Rationale

A file read top to bottom should introduce things before it uses them. Enums are
the vocabulary the rest of the file is written in; types are the nouns; methods
are what those nouns do; free functions are what is left. Reading in that order
means never encountering a name whose meaning is defined two hundred lines
further down.

The ordering also gives every future addition an obvious home. Without one, new
code goes at the bottom of the file regardless of what it is, and the file
becomes a chronological log of what was added when — which is exactly as useful
for navigation as it sounds.

`separate_var_blocks` is the one clause that costs something. A grouped
`var (...)` block reads as a single unit, which is precisely the problem: it
invites unrelated globals to accumulate under one header where each new entry is
a one-line diff nobody questions. Separate blocks make each global a visible,
individual declaration — and given
[`org/globals-singleton-only`](#orgglobals-singleton-only) there should be very
few of them anyway.

#### Detection

Syntactic. Classify each top-level `Decl` by kind, then check the sequence of
kinds is non-decreasing in the configured order. Enum detection reuses
`logic/iota-candidate`'s classifier: a `TypeSpec` for a named integer or string
type together with a `const` block of that type.

#### Interactions

- [`org/singleton-layout`](#orgsingleton-layout) **overrides** this rule in
  files that declare a singleton. See [Precedence](#precedence).
- Depends on the same enum classifier as `logic/iota-candidate`.

---

### `org/private-functions-last`

**Unexported functions come after every exported function in the file.**

```go
func Parse(s string) (*Doc, error) { ... }      // ✓ exported first
func Render(d *Doc) string        { ... }

func normalize(s string) string   { ... }       // ✓ unexported last
func validate(d *Doc) error       { ... }
```

#### Configuration

```yaml
settings:
  org/private-functions-last:
    # Apply the same split to methods, within each type's method run.
    include_methods: true
```

#### Rationale

A file's exported functions are its purpose; its unexported functions are how it
achieves that. A reader arriving from another package is looking for the former
and will read past the latter, so putting helpers first taxes every reader to
save the author nothing.

The split also makes the file's public surface countable at a glance, which is
what [`org/max-public-functions`](#orgmax-public-functions) measures — you can
see whether a file is growing in surface area or merely in implementation.

#### Detection

Syntactic. Within the function section, find any exported `FuncDecl` appearing
after an unexported one.

#### Interactions

- Overridden by [`org/singleton-layout`](#orgsingleton-layout), where the
  unexported `instance` function is required to appear *before* the exported
  accessors.

---

### `org/singleton-layout`

**A file declaring a singleton is laid out as: the singleton's var block, then
the unexported `instance` function, then the exported accessors.**

```go
package registry

var (
	registryOnce sync.Once
	registry     *Registry
)

func instance() *Registry {
	registryOnce.Do(func() {
		registry = &Registry{entries: map[string]Handler{}}
	})
	return registry
}

func Register(name string, h Handler) error { return instance().register(name, h) }
func Lookup(name string) (Handler, bool)    { return instance().lookup(name) }
```

This is the one place where an unexported function is required to precede
exported ones, because `instance` is not a helper — it is the accessor the rest
of the file is built on, and every exported function below it is a one-line
delegation.

#### Configuration

```yaml
settings:
  org/singleton-layout:
    instance_func: instance
    # The singleton's var block, the instance function, and the exported
    # accessors must be the only top-level declarations in the file.
    exclusive_file: true
```

#### Rationale

A singleton is the one construct where package-level mutable state is
sanctioned, so it earns a fixed shape that makes the sanction visible.
Everything about the pattern that can go wrong — initialisation racing,
initialisation happening twice, some other file reaching past the accessor to
touch the variable — is prevented by the shape rather than by discipline.

Putting `instance` directly under the var block also means the entire
initialisation story is on one screen: the state, the guard, and the single
function permitted to construct it.

#### Detection

Syntactic. A file is a singleton file when it declares a package-level `var` of
a named type together with a `sync.Once`. From there, check the position of the
`instance` function relative to the var block and to exported functions.

#### Interactions

- Overrides [`org/member-order`](#orgmember-order) and
  [`org/private-functions-last`](#orgprivate-functions-last) for this file.
- Enforced together with
  [`org/singleton-instance-func`](#orgsingleton-instance-func) and
  [`org/global-file-scoped`](#orgglobal-file-scoped); the three describe one
  pattern from three angles.

---

## File budgets

### `org/max-functions-per-file`

**A file declares at most N functions.**

#### Configuration

```yaml
settings:
  org/max-functions-per-file:
    limit: 12
    # Methods are counted against the owning type's budget
    # (logic/max-object-members), not against the file's.
    count_methods: false
    count_tests: false
```

#### Rationale

Function count is the closest available proxy for how many distinct things a
file does. It is a better measure than line count for this purpose: one long
function is a `logic/` problem, whereas twenty short ones is an `org/` problem,
and only the second means the file has stopped being about one thing.

#### Detection

Syntactic.

#### Interactions

- **`count_methods` must default to `false`,** or this rule contradicts
  [`org/type-cohesion`](#orgtype-cohesion) outright: a type with fifteen methods
  cannot simultaneously keep them in one file and stay under a twelve-function
  budget. Method count is capped by `logic/max-object-members` instead, which is
  the right place for it — that is a property of the type, not of the file.
- Participates in the exception mechanism of
  [`org/consumer-locality`](#orgconsumer-locality).

---

### `org/max-public-functions`

**A file declares at most N exported functions.**

#### Configuration

```yaml
settings:
  org/max-public-functions:
    limit: 5
    count_methods: false
```

#### Rationale

Exported functions are the package's API surface, and surface concentrated in
one file is surface nobody owns. A file with a dozen exported functions is
either a package pretending to be a file, or a grab-bag — and it is the file
every subsequent addition gets appended to, because it is already the one that
"has the API in it".

A tight cap forces the question *which package does this belong to* while the
answer is still cheap.

#### Detection

Syntactic.

---

### `org/max-private-functions`

**A file that declares at least one exported function may declare at most N
unexported functions.**

A file of *only* unexported functions is unconstrained by this rule — that is a
helper file, and helper files are allowed to be helper files.

#### Configuration

```yaml
settings:
  org/max-private-functions:
    limit: 3
    # Only applies to files that also export something.
    when_file_has_exports: true
    count_methods: false
```

#### Rationale

Unexported functions piling up beneath an exported one are the visible residue
of a file doing too much. Each is a step the exported function needs but does not
name, and past a small number the file has an implementation of its own that is
no longer readable as "the API plus a little glue".

Keeping the cap severe forces a choice between two good outcomes: the helpers
were really one cohesive thing, in which case they become a type with methods; or
they were unrelated, in which case they belong in different files near the code
that uses them — which is what
[`org/consumer-locality`](#orgconsumer-locality) will then say.

#### Detection

Syntactic.

#### Interactions

- Applying this rule commonly produces a relocation that
  [`org/consumer-locality`](#orgconsumer-locality) would independently propose.

---

## What belongs together

### `org/type-cohesion`

**A type, its factory, and all of its methods live in one file.**

```
✗  registry.go        type Registry struct { ... }
   registry_ops.go    func (r *Registry) Register(...)
   registry_new.go    func NewRegistry() *Registry

✓  registry.go        the type, NewRegistry, and every method
```

#### Configuration

```yaml
settings:
  org/type-cohesion:
    # Factories are identified by logic/factory-naming's classifier: a
    # function whose first non-error result is the type.
    include_factories: true
    # Build-constrained files legitimately split a type's methods by platform.
    ignore_build_constrained: true
    ignore_tests: true
```

#### Rationale

A type and its methods are one unit of meaning. Split across files, there is no
single place that answers "what can this do?", and the answer has to be
assembled by grep — which reliably misses the method in the file nobody thought
to look in.

The split also breaks review: a change to an invariant touches the struct in one
file and the methods that maintain it in three others, and no reviewer sees the
whole change at once.

#### Detection

Syntactic. Group `FuncDecl`s by receiver type name and compare against the file
holding the `TypeSpec`. Factory identification reuses `logic/factory-naming`.

#### Interactions

- Requires `count_methods: false` on the file budget rules, per
  [`org/max-functions-per-file`](#orgmax-functions-per-file).
- Takes precedence over [`org/consumer-locality`](#orgconsumer-locality): a
  method never relocates to its caller's file.

---

### `org/interface-own-file`

**An interface declaration gets its own file, as a struct does.**

#### Configuration

```yaml
settings:
  org/interface-own-file:
    # `own-file`   — one interface per file, nothing else in it
    # `separate`   — interfaces may share a file with each other, but not with
    #                struct declarations
    mode: own-file
    # Small interfaces are often best declared beside their consumer.
    min_methods: 1
```

#### Rationale

An interface is a contract, and a contract with a file to itself is one that can
be read, reviewed, and diffed without the noise of an implementation beside it.
Keeping it separate from any struct also removes the strongest visual cue that
the interface exists *for* that struct — which is the habit that produces
producer-side interfaces with exactly one implementation.

#### Detection

Syntactic.

#### Interactions

- Together with [`org/type-cohesion`](#orgtype-cohesion) this amounts to a
  **one-type-per-file** standard that is nowhere stated as such. Worth making
  explicit — [open question 4](#open-questions).
- In tension with `logic/interface-at-consumer` (proposed): declaring an
  interface next to the consumer that needs it argues for the interface sharing
  a file with that consumer's code, not for isolation. `min_methods` exists to
  let single-method consumer-side interfaces stay put.

---

### `org/consumer-locality`

**A declaration whose only consumers within the package live in one other file
belongs in that file.**

For each package-level declaration, collect the set of files that reference it.
If that set is exactly one file, and it is not the declaring file, the
declaration is in the wrong place.

```
lexer.go     calls validate() three times      ← the only consumer
parser.go    func validate(...) error          ← declared here

✗  validate belongs in lexer.go
```

The rule applies to functions, enums, and their constants. It does **not** apply
to methods, which belong to their type by
[`org/type-cohesion`](#orgtype-cohesion).

**Exception:** the finding is suppressed when moving the declaration would cause
a different `org/` violation — the target file exceeding a function budget, or
the move breaking type cohesion or member order. See
[the what-if problem](#the-what-if-problem) for how that is decided, and
[Precedence](#precedence) for the resolution order.

Cases that deliberately produce nothing:

| Situation | Result |
| --- | --- |
| No consumers in the package | silent — that is dead code, a different rule |
| Consumers in two or more files | silent — it is shared, it stays put |
| Sole consumer is the declaring file | silent — already correct |
| Consumed only from outside the package | silent — no within-package signal |

#### Configuration

```yaml
settings:
  org/consumer-locality:
    check_functions: true
    check_enums: true
    check_vars: false        # see org/global-file-scoped, which is stricter
    # Report only when the target file is under every budget with this much
    # headroom. Setting 0 requires the full what-if pass.
    required_headroom: 2
    ignore_tests: true
```

#### Rationale

Code should live next to the code that needs it. A helper in a distant file is a
helper nobody knows exists, so the next person who needs it writes a second one —
and now there are two subtly different implementations, which is how a package
accumulates three functions that all normalise a name.

Locality also makes deletion safe. When a function sits beside its only caller,
removing the caller makes the function's redundancy obvious. When it sits three
files away, it survives forever because nobody can prove it is unused.

The single-consumer condition is what keeps the rule honest: it fires only when
there is exactly one right answer. Anything shared across files is genuinely
package-level and the rule stays quiet.

#### Detection

**Type-tier for correctness.** Resolving which identifiers refer to a given
package-level declaration is exact with `go/types` (`Info.Uses`). A syntactic
approximation — matching identifiers by name — is possible but wrong in the
presence of shadowing: a local variable named `validate` would count as a
consumer, and the rule would propose moving a function to a file that never
calls it. For a rule whose entire output is "move this code", that failure mode
is unacceptable, so it should not ship on the syntactic approximation.

The exception mechanism is the harder half — see
[the what-if problem](#the-what-if-problem).

#### Interactions

- Yields to every other `org/` rule. See [Precedence](#precedence).
- Frequently proposes the same move that
  [`org/max-private-functions`](#orgmax-private-functions) forces.
- Cycle risk: A's sole consumer is in B, and B's sole consumer is in A. The
  what-if pass needs a guard, or the two findings will each propose a move that
  creates the other.

---

## Globals and singletons

### `org/globals-singleton-only`

**Package-level `var` is permitted only as the backing state of a singleton.**

Everything else that wants to be a global is either a constant, a field on a
type, or a parameter.

#### Configuration

```yaml
settings:
  org/globals-singleton-only:
    # Declarations exempt from the ban. Go offers no immutable composite
    # literal, so some of these have no alternative spelling.
    allow:
      - sentinel-errors        # var ErrFoo = errors.New(...)
      - interface-assertions   # var _ Iface = (*T)(nil)
      - compiled-patterns      # var re = regexp.MustCompile(...)
      - lookup-tables          # var validStatuses = map[Status]bool{...}
    # Require the exempt forms to be unexported, so they cannot be mutated by
    # another package.
    require_unexported_tables: true
```

#### Rationale

A package-level variable is state with no owner and no lifetime. Anything in the
package can read it, anything can write it, nothing coordinates the two, and
tests that touch it cannot run in parallel or in isolation. The cost does not
show up until the package has grown enough that no single person knows every
writer.

The singleton exception exists because process-wide state is occasionally real —
a connection pool, a metrics registry, a cache. Confining globals to that one
pattern means every global in the codebase has an accessor, a `sync.Once`, and a
known initialisation point, instead of being a variable someone assigned to in
an `init` function.

#### Detection

Syntactic, given the exemption classifiers.

**The exemption list is the whole design.** Go has no immutable composite
constant: a lookup table, a compiled regexp, and a sentinel error can only be
package-level `var`s. A rule without these exemptions would fire on idiomatic,
correct, unavoidable code and would be switched off within a day. The four
listed defaults are the ones with no alternative spelling —
[open question 5](#open-questions) asks whether the list is complete.

#### Interactions

- Defines what [`org/singleton-instance-func`](#orgsingleton-instance-func) and
  [`org/global-file-scoped`](#orgglobal-file-scoped) then constrain.

---

### `org/singleton-instance-func`

**A singleton is constructed by an unexported function named `instance`, and
that function guards construction with `sync.Once`.**

```go
// ✓
func instance() *Registry {
	registryOnce.Do(func() { registry = &Registry{...} })
	return registry
}

// ✗ constructed in init — no accessor, no guard, ordering is implicit
func init() { registry = &Registry{...} }

// ✗ lazily constructed without a guard — races under concurrent first use
func instance() *Registry {
	if registry == nil {
		registry = &Registry{...}
	}
	return registry
}
```

#### Configuration

```yaml
settings:
  org/singleton-instance-func:
    name: instance
    require_sync_once: true
    # Forbid assigning the singleton var anywhere other than inside the
    # once-guarded closure.
    forbid_external_assignment: true
    # Forbid constructing the singleton in an init function.
    forbid_init: true
```

#### Rationale

The unguarded lazy form is a data race that will not reproduce in testing and
will not be caught by review, because it looks exactly like correct code. The
`init` form is not racy but is worse in another way: it fixes construction order
across the whole package graph, runs whether or not the singleton is ever used,
and gives no place to return an error.

`sync.Once` is the one spelling with none of those problems, and mandating a
single name for the accessor means the pattern is greppable — you can enumerate
every singleton in a codebase by searching for one identifier.

#### Detection

Syntactic. Find package-level vars paired with a `sync.Once`, then check the
accessor's name, that construction happens inside `once.Do`, and that no other
function assigns the variable.

#### Interactions

- The layout of the resulting file is
  [`org/singleton-layout`](#orgsingleton-layout).
- `forbid_external_assignment` is a strictly weaker version of
  [`org/global-file-scoped`](#orgglobal-file-scoped); with that rule enabled it
  is redundant.

---

### `org/global-file-scoped`

**A package-level variable may be referenced only within the file that declares
it.**

Go has no file-level visibility. This rule creates it by convention: `internal/`
restricts a package's reach, unexported restricts a package member's reach, and
this restricts a variable's reach to a single file.

```
registry.go    var registry *Registry     declared here
registry.go    func instance() ...        ✓ same file
lookup.go      registry.entries[name]     ✗ reaches past the accessor
```

#### Configuration

```yaml
settings:
  org/global-file-scoped:
    # Sentinel errors are meant to be referenced package-wide, and by importers.
    exempt:
      - sentinel-errors
      - interface-assertions
    ignore_tests: true
```

#### Rationale

The accessor is the point. A singleton's `instance` function exists to guarantee
that initialisation happened exactly once before anyone reads the value — and
every reference that bypasses it and touches the variable directly is a
guarantee lost. Those references are also invisible: nothing in the declaration
says who reads it, so the set of writers can only be found by searching the
whole package.

Confining a variable to one file makes its complete usage auditable by reading
that file, which is the same property `internal/` gives a package. It is the
strongest containment available for state that has to exist.

#### Detection

Type-tier, for the same reason as
[`org/consumer-locality`](#orgconsumer-locality): a local variable shadowing the
global's name would otherwise register as an access, and here a false positive
accuses correct code of a race.

#### Interactions

- Subsumes `forbid_external_assignment` from
  [`org/singleton-instance-func`](#orgsingleton-instance-func).
- Combined with [`org/singleton-layout`](#orgsingleton-layout)'s
  `exclusive_file`, this yields **one singleton per file, and nothing else in
  it** — the strongest form of the pattern.

---

## Precedence

Several of these rules can want opposite things about the same declaration.
Resolution order, highest first:

| Rank | Rule | Wins because |
| --- | --- | --- |
| 1 | `org/type-cohesion` | A method's home is its type. Never relocated. |
| 2 | `org/singleton-layout` | Overrides `member-order` and `private-functions-last` in singleton files. |
| 3 | `org/interface-own-file` | Placement of a type declaration. |
| 4 | file budgets | A move that breaks a budget is not an improvement. |
| 5 | `org/member-order`, `org/private-functions-last` | Ordering within whatever file the above settled on. |
| 6 | `org/consumer-locality` | Yields to everything. This is the exception you asked for, stated as a rank. |

Two conflicts are worth calling out because they are guaranteed to occur, not
hypothetical:

- **`singleton-layout` vs `private-functions-last`.** The singleton pattern
  requires the unexported `instance` to appear *before* the exported accessors.
  This is the only sanctioned inversion, and it is why singleton files are
  detected as a distinct shape rather than handled by the general ordering rule.
- **`type-cohesion` vs the file budgets.** A type with more methods than the
  file's function budget cannot satisfy both. Resolved by excluding methods from
  the file budgets entirely (`count_methods: false`) and capping them on the
  type via `logic/max-object-members`.

---

## Open questions

1. **How is `org/consumer-locality`'s exception implemented?** Hard-coded budget
   knowledge, or a general what-if pass in the engine. Recommended above: ship
   the unambiguous subset first, add the machinery later — see
   [the what-if problem](#the-what-if-problem). This is the largest decision in
   this document.

2. **Does `org/member-order` group by type or by kind?** `per-type` — struct A,
   its factory, its methods, then struct B — is drafted as the default and reads
   as what you described. `by-kind` — all structs, then all factories, then all
   methods — is the other reading. Under
   [`org/interface-own-file`](#orgorg-interface-own-file)'s `own-file` mode plus
   one-type-per-file the distinction mostly disappears, since a file holds one
   type anyway.

3. **Where do non-struct, non-interface types go?** Type aliases, named function
   types, and named basic types that are not enums have no slot in the canonical
   order. Fold them into the struct section, or give them their own?

4. **Should one-type-per-file be stated outright?** `org/type-cohesion` plus
   `org/interface-own-file` in `own-file` mode already imply it. Stating it
   directly would be clearer than leaving it as an emergent consequence of two
   other rules, and would replace both.

5. **Is the globals exemption list complete?** Sentinel errors, interface
   assertions, compiled patterns, and lookup tables are drafted. Others with a
   claim: `embed.FS` variables (`//go:embed` requires a package-level var and
   there is no alternative spelling), `sync.Pool` instances, and build-time
   variables set by `-ldflags`. The `embed.FS` case in particular is
   unavoidable — the directive cannot target anything else.

6. **What counts as "the exported accessors" of a singleton?** The layout was
   described as "the public methods afterwards", which could mean package-level
   functions delegating to `instance()` (as drafted), or methods on the
   singleton's own type. The two produce different files; the drafted form keeps
   the type reusable as a non-singleton.

7. **Do these rules apply to `_test.go` files?** Drafted as `ignore_tests: true`
   throughout. Test files legitimately carry many small unexported helpers and
   table-driven fixtures, and would violate
   [`org/max-private-functions`](#orgmax-private-functions) constantly. But
   exempting them entirely means the largest files in many packages are
   unchecked.

8. **What are the actual numbers?** Every budget default here — 12 functions, 5
   exported, 3 unexported — is a guess. They should be measured against real
   Quikcad repositories before being fixed, since a limit set below the existing
   median makes the rule unadoptable.
