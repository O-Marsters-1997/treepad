# Value vs Pointer in Go

The right choice keeps APIs clear and avoids a class of subtle bugs around mutation, copying,
and nil semantics.

## Contents

- [The decision rule](#the-decision-rule)
- [Receiver consistency](#receiver-consistency)
- [When "large" is actually large](#when-large-is-actually-large)
- [Never use pointers for optionality](#never-use-pointers-for-optionality)

---

## The decision rule

**Use a pointer when:**

- The function or method **mutates** the value — callers must see the change.
- The struct is measurably expensive to copy (benchmark first; most structs aren't).
- Consistency requires it — if any method on the type needs a pointer receiver, use pointer
  receivers throughout.

**Use a value when:**

- The type is small and immutable (coordinates, IDs, scalars, config values).
- The zero value is a useful state (e.g. `sync.Mutex`, `bytes.Buffer` — these are always passed
  by pointer, but their *zero value* is what makes them usable without initialisation).
- You want to signal to callers that the function does not mutate the argument.

---

## Receiver consistency

A value receiver works on both values and pointers. A pointer receiver only works on pointers (or
addressable values). If any method on the type needs to mutate state, all methods should use pointer
receivers — otherwise the type's method set is split and interface satisfaction becomes surprising:

```go
// Bad: mixed receivers on the same type
type Counter struct{ n int }

func (c Counter) Value() int { return c.n }  // value receiver
func (c *Counter) Inc()      { c.n++ }       // pointer receiver
// Value() is NOT in *Counter's method set — satisfying interfaces becomes confusing
```

```go
// Good: pointer receivers throughout (since Inc needs mutation)
func (c *Counter) Value() int { return c.n }
func (c *Counter) Inc()       { c.n++ }
```

The standard library is the guide: `bytes.Buffer`, `sync.Mutex`, `http.Request` use pointer
receivers throughout. Small, immutable types like `time.Time` use value receivers throughout.
Pick one style per type and hold it.

---

## When "large" is actually large

"Large" means measurably expensive in a benchmark — not intuitively big. A struct with 10 `int`
fields is still cheaper to copy than a single heap allocation. Don't switch from value to pointer
for performance without profiling first.

`sync.Mutex`, `bytes.Buffer`, and `os.File` are effectively "large" because they embed
synchronisation primitives or OS handles, not just because they have many fields. Copying them
would break their semantics.

A common mistake: returning `*T` from a constructor when the returned `T` is small, because it
"looks like a reference type". If the type doesn't need to be mutated through the pointer and isn't
genuinely large, return the value.

```go
// Unnecessary — Point is small and immutable
func NewPoint(x, y float64) *Point {
    return &Point{x, y}
}

// Better — caller gets a value they can copy freely
func NewPoint(x, y float64) Point {
    return Point{x, y}
}
```

---

## Never use pointers for optionality

`*string` as a struct field is ambiguous:

- Is `nil` "not provided by the user"?
- Or "intentionally cleared"?
- Or "we forgot to set it"?

Use `Option[T]` instead — see `references/optional.md`. The type signature makes the contract
explicit, and `None` and `Some(zero)` are distinguishable in a way that `nil` and `new(T)` are not.

```go
// Bad: nil is ambiguous
type User struct {
    Nickname *string
}

// Good: absence is explicit
type User struct {
    Nickname option.Option[string]
}
```
