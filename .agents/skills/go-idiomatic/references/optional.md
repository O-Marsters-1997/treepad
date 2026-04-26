# Option[T] — Explicit Optionality in Go

## Why not `*T`?

Using a pointer to signal "this might not be set" is the most common source of accidental nil
panics in Go. A `*string` struct field is ambiguous:

- Is `nil` "not provided by the user"?
- Or "intentionally cleared"?
- Or "we forgot to set it"?

`Option[T]` makes the contract explicit. The type tells the reader — and the compiler — that
absence is an expected, handled state, not an error to be ignored.

Additional benefits:
- **JSON round-trips correctly**: `null` decodes to `None`, `"alice"` decodes to `Some("alice")`.
  With `*T`, you have to remember to handle nil in MarshalJSON.
- **Clearer test data**: `Some("alice")` in a test struct is obviously intentional; a non-nil
  pointer requires an extra variable or `ptr("alice")` helper to construct.
- **No accidental zero-value confusion**: `option.None[int]()` and `option.Some(0)` are distinct.
  With `*int`, you must remember that `new(int)` gives you `Some(0)`, not `None`.

## When to reach for `Option[T]`

- Struct fields that are genuinely optional (user may or may not supply them).
- Function return values where the zero value of `T` is meaningful data (e.g. `Option[int]` for
  a result that could legitimately be 0).
- Configuration values that distinguish "not set" from "set to zero/empty".

## When NOT to use it

- **Mutation**: if the function's job is to modify a value through a pointer, keep the pointer.
- **Large structs**: if you're passing a struct to avoid copying, keep the pointer.
- **Interfaces**: `nil` interface values have their own semantics; don't wrap interfaces in Option.
- **Everywhere**: don't reach for it reflexively. If `T` has a natural zero value that means
  "absent" (empty string, 0, nil map), and that convention is clear in context, use the zero value.

---

## Implementation

Drop this into your project at `internal/option/option.go` (or `pkg/option/option.go` if the
project is a library). It has no external dependencies beyond the standard library and requires
Go 1.18+.

```go
package option

import (
	"encoding/json"
	"fmt"
)

// Option[T] represents a value that may or may not be present.
// Use it instead of *T when the absence of a value is a meaningful, expected state.
type Option[T any] struct {
	value *T
}

// Some wraps a present value.
func Some[T any](v T) Option[T] {
	return Option[T]{value: &v}
}

// None returns an absent value.
func None[T any]() Option[T] {
	return Option[T]{}
}

// IsSome reports whether a value is present.
func (o Option[T]) IsSome() bool {
	return o.value != nil
}

// IsNone reports whether no value is present.
func (o Option[T]) IsNone() bool {
	return o.value == nil
}

// Unwrap returns the contained value. Panics if absent.
// Prefer UnwrapOr in code paths where absence is a real possibility.
func (o Option[T]) Unwrap() T {
	if o.IsNone() {
		panic("option: Unwrap called on None")
	}
	return *o.value
}

// UnwrapOr returns the contained value, or fallback if absent.
func (o Option[T]) UnwrapOr(fallback T) T {
	if o.IsNone() {
		return fallback
	}
	return *o.value
}

// MarshalJSON encodes Some[T] as the JSON representation of T, and None as null.
func (o Option[T]) MarshalJSON() ([]byte, error) {
	if o.IsNone() {
		return []byte("null"), nil
	}
	return json.Marshal(*o.value)
}

// UnmarshalJSON decodes null as None and any other JSON value as Some[T].
func (o *Option[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*o = None[T]()
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("option: %w", err)
	}
	*o = Some(v)
	return nil
}

// String implements fmt.Stringer for debugging.
func (o Option[T]) String() string {
	if o.IsNone() {
		return "None"
	}
	return fmt.Sprintf("Some(%v)", *o.value)
}
```

---

## Usage examples

```go
import "yourmodule/internal/option"

type UserProfile struct {
    Name     string
    Nickname option.Option[string]  // user may or may not set this
    Age      option.Option[int]     // 0 is a valid age to store, so zero value won't do
}

// Constructing
profile := UserProfile{
    Name:     "Alice",
    Nickname: option.Some("ali"),
    Age:      option.None[int](),
}

// Reading
if profile.Nickname.IsSome() {
    fmt.Println("Display as:", profile.Nickname.Unwrap())
}

displayName := profile.Nickname.UnwrapOr(profile.Name)

// JSON round-trip
// {"name":"Alice","nickname":"ali","age":null}  — None[int] marshals as null
data, _ := json.Marshal(profile)

// Testing (clear intent, no pointer helpers needed)
want := UserProfile{
    Name:     "Alice",
    Nickname: option.None[string](),
    Age:      option.Some(30),
}
```

---

## Inspired by

The pattern follows `moznion/go-optional` and Rust's `Option<T>`. Hosting our own copy means no
external module dependency for a small, stable abstraction.
