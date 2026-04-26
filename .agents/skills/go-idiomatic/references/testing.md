# Go Testing Patterns Reference

Comprehensive Go testing patterns for writing reliable, maintainable tests.

For the TDD workflow (red-green-refactor, when to write the test first), use the `tdd` skill.

## Contents

- [Table-Driven Tests](#table-driven-tests)
- [Subtests and Sub-benchmarks](#subtests-and-sub-benchmarks)
- [Test Helpers](#test-helpers)
- [Golden Files](#golden-files)
- [Mocking with Interfaces](#mocking-with-interfaces)
- [Benchmarks](#benchmarks)
- [Fuzzing](#fuzzing-go-118)
- [Test Coverage](#test-coverage)
- [HTTP Handler Testing](#http-handler-testing)
- [Testing Commands](#testing-commands)
- [Best Practices](#best-practices)
- [CI/CD Integration](#integration-with-cicd)

---

## Table-Driven Tests

The standard Go test pattern. Enables comprehensive coverage with minimal code and makes it easy
to add new cases without changing test logic.

```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        expected int
    }{
        {"positive numbers", 2, 3, 5},
        {"negative numbers", -1, -2, -3},
        {"zero values", 0, 0, 0},
        {"mixed signs", -1, 1, 0},
        {"large numbers", 1_000_000, 2_000_000, 3_000_000},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Add(tt.a, tt.b)
            if got != tt.expected {
                t.Errorf("Add(%d, %d) = %d; want %d", tt.a, tt.b, got, tt.expected)
            }
        })
    }
}
```

### Table-Driven Tests with Error Cases

```go
func TestParseConfig(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    *Config
        wantErr bool
    }{
        {
            name:  "valid config",
            input: `{"host": "localhost", "port": 8080}`,
            want:  &Config{Host: "localhost", Port: 8080},
        },
        {
            name:    "invalid JSON",
            input:   `{invalid}`,
            wantErr: true,
        },
        {
            name:    "empty input",
            input:   "",
            wantErr: true,
        },
        {
            name:  "minimal config",
            input: `{}`,
            want:  &Config{},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseConfig(tt.input)

            if tt.wantErr {
                if err == nil {
                    t.Error("expected error, got nil")
                }
                return
            }

            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }

            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("got %+v; want %+v", got, tt.want)
            }
        })
    }
}
```

---

## Subtests and Sub-benchmarks

### Organising Related Tests

Subtests share setup and let you run a subset with `-run`:

```go
func TestUser(t *testing.T) {
    db := setupTestDB(t)

    t.Run("Create", func(t *testing.T) {
        user := &User{Name: "Alice"}
        if err := db.CreateUser(user); err != nil {
            t.Fatalf("CreateUser: %v", err)
        }
        if user.ID == "" {
            t.Error("expected user ID to be set")
        }
    })

    t.Run("Get", func(t *testing.T) {
        user, err := db.GetUser("alice-id")
        if err != nil {
            t.Fatalf("GetUser: %v", err)
        }
        if user.Name != "Alice" {
            t.Errorf("got name %q; want %q", user.Name, "Alice")
        }
    })
}
```

### Parallel Subtests

```go
func TestParallel(t *testing.T) {
    tests := []struct {
        name  string
        input string
    }{
        {"case1", "input1"},
        {"case2", "input2"},
        {"case3", "input3"},
    }

    for _, tt := range tests {
        tt := tt // capture range variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            result := Process(tt.input)
            _ = result
        })
    }
}
```

---

## Test Helpers

### Helper Functions

Mark helpers with `t.Helper()` so failure lines point at the callsite, not the helper:

```go
func setupTestDB(t *testing.T) *sql.DB {
    t.Helper()

    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("open database: %v", err)
    }
    t.Cleanup(func() { db.Close() })

    if _, err := db.Exec(schema); err != nil {
        t.Fatalf("create schema: %v", err)
    }

    return db
}

func assertNoError(t *testing.T, err error) {
    t.Helper()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

func assertEqual[T comparable](t *testing.T, got, want T) {
    t.Helper()
    if got != want {
        t.Errorf("got %v; want %v", got, want)
    }
}
```

### Temporary Files and Directories

```go
func TestFileProcessing(t *testing.T) {
    tmpDir := t.TempDir() // automatically cleaned up after the test

    testFile := filepath.Join(tmpDir, "test.txt")
    if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
        t.Fatalf("create test file: %v", err)
    }

    result, err := ProcessFile(testFile)
    if err != nil {
        t.Fatalf("ProcessFile: %v", err)
    }
    _ = result
}
```

---

## Golden Files

Compare output against reference files stored in `testdata/`. Use `-update` to regenerate them:

```go
var update = flag.Bool("update", false, "update golden files")

func TestRender(t *testing.T) {
    tests := []struct {
        name  string
        input Template
    }{
        {"simple", Template{Name: "test"}},
        {"complex", Template{Name: "test", Items: []string{"a", "b"}}},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Render(tt.input)
            golden := filepath.Join("testdata", tt.name+".golden")

            if *update {
                if err := os.WriteFile(golden, got, 0644); err != nil {
                    t.Fatalf("update golden file: %v", err)
                }
            }

            want, err := os.ReadFile(golden)
            if err != nil {
                t.Fatalf("read golden file: %v", err)
            }

            if !bytes.Equal(got, want) {
                t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
            }
        })
    }
}
```

---

## Mocking with Interfaces

Prefer hand-written mocks over generated ones for small interfaces — they're simpler to understand
and maintain:

```go
type UserRepository interface {
    GetUser(id string) (*User, error)
    SaveUser(user *User) error
}

type MockUserRepository struct {
    GetUserFunc  func(id string) (*User, error)
    SaveUserFunc func(user *User) error
}

func (m *MockUserRepository) GetUser(id string) (*User, error) {
    return m.GetUserFunc(id)
}

func (m *MockUserRepository) SaveUser(user *User) error {
    return m.SaveUserFunc(user)
}

func TestUserService(t *testing.T) {
    mock := &MockUserRepository{
        GetUserFunc: func(id string) (*User, error) {
            if id == "123" {
                return &User{ID: "123", Name: "Alice"}, nil
            }
            return nil, ErrNotFound
        },
    }

    service := NewUserService(mock)
    user, err := service.GetUserProfile("123")
    if err != nil {
        t.Fatalf("GetUserProfile: %v", err)
    }
    if user.Name != "Alice" {
        t.Errorf("got name %q; want %q", user.Name, "Alice")
    }
}
```

---

## Benchmarks

### Basic Benchmark

```go
func BenchmarkProcess(b *testing.B) {
    data := generateTestData(1000)
    b.ResetTimer() // don't count setup time

    for i := 0; i < b.N; i++ {
        Process(data)
    }
}

// go test -bench=BenchmarkProcess -benchmem
// BenchmarkProcess-8   10000   105234 ns/op   4096 B/op   10 allocs/op
```

### Benchmark Across Input Sizes

```go
func BenchmarkSort(b *testing.B) {
    for _, size := range []int{100, 1_000, 10_000, 100_000} {
        b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
            data := generateRandomSlice(size)
            b.ResetTimer()

            for i := 0; i < b.N; i++ {
                tmp := make([]int, len(data))
                copy(tmp, data)
                sort.Ints(tmp)
            }
        })
    }
}
```

---

## Fuzzing (Go 1.18+)

### Basic Fuzz Test

```go
func FuzzParseJSON(f *testing.F) {
    f.Add(`{"name": "test"}`)
    f.Add(`{"count": 123}`)
    f.Add(`[]`)

    f.Fuzz(func(t *testing.T, input string) {
        var result map[string]any
        err := json.Unmarshal([]byte(input), &result)
        if err != nil {
            return // invalid input is expected
        }
        // if parsing succeeded, re-encoding must too
        if _, err = json.Marshal(result); err != nil {
            t.Errorf("Marshal failed after successful Unmarshal: %v", err)
        }
    })
}

// go test -fuzz=FuzzParseJSON -fuzztime=30s
```

### Property-Based Fuzz Test

```go
func FuzzCompare(f *testing.F) {
    f.Add("hello", "world")
    f.Add("", "")

    f.Fuzz(func(t *testing.T, a, b string) {
        result := Compare(a, b)

        if a == b && result != 0 {
            t.Errorf("Compare(%q, %q) = %d; want 0", a, b, result)
        }

        reverse := Compare(b, a)
        if result != 0 && (result > 0) == (reverse > 0) {
            t.Errorf("Compare(%q,%q)=%d Compare(%q,%q)=%d: same sign, should be opposite",
                a, b, result, b, a, reverse)
        }
    })
}
```

---

## Test Coverage

```bash
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out      # view in browser
go tool cover -func=coverage.out      # per-function breakdown
go test -race -coverprofile=coverage.out ./...
```

| Code type | Target |
|-----------|--------|
| Critical business logic | 100% |
| Public APIs | 90%+ |
| General code | 80%+ |
| Generated code | Exclude |

---

## HTTP Handler Testing

```go
func TestHealthHandler(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/health", nil)
    w := httptest.NewRecorder()

    HealthHandler(w, req)

    resp := w.Result()
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Errorf("got status %d; want %d", resp.StatusCode, http.StatusOK)
    }

    body, _ := io.ReadAll(resp.Body)
    if string(body) != "OK" {
        t.Errorf("got body %q; want %q", body, "OK")
    }
}

func TestAPIHandler(t *testing.T) {
    tests := []struct {
        name       string
        method     string
        path       string
        body       string
        wantStatus int
        wantBody   string
    }{
        {"get user", http.MethodGet, "/users/123", "", http.StatusOK, `{"id":"123","name":"Alice"}`},
        {"not found", http.MethodGet, "/users/999", "", http.StatusNotFound, ""},
        {"create user", http.MethodPost, "/users", `{"name":"Bob"}`, http.StatusCreated, ""},
    }

    handler := NewAPIHandler()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var body io.Reader
            if tt.body != "" {
                body = strings.NewReader(tt.body)
            }

            req := httptest.NewRequest(tt.method, tt.path, body)
            req.Header.Set("Content-Type", "application/json")
            w := httptest.NewRecorder()

            handler.ServeHTTP(w, req)

            if w.Code != tt.wantStatus {
                t.Errorf("got status %d; want %d", w.Code, tt.wantStatus)
            }
            if tt.wantBody != "" && w.Body.String() != tt.wantBody {
                t.Errorf("got body %q; want %q", w.Body.String(), tt.wantBody)
            }
        })
    }
}
```

---

## Testing Commands

```bash
go test ./...                                      # run all tests
go test -v ./...                                   # verbose output
go test -run TestAdd ./...                         # run specific test
go test -run "TestUser/Create" ./...               # run specific subtest
go test -race ./...                                # with race detector
go test -cover -coverprofile=coverage.out ./...    # with coverage
go test -short ./...                               # skip long-running tests
go test -timeout 30s ./...                         # with timeout
go test -bench=. -benchmem ./...                   # run benchmarks
go test -fuzz=FuzzParse -fuzztime=30s ./...        # run fuzzer
go test -count=10 ./...                            # run each test N times (flakiness detection)
```

---

## Best Practices

**Do:**
- Use table-driven tests for comprehensive coverage
- Test behaviour, not implementation
- Mark helpers with `t.Helper()`
- Use `t.Parallel()` for independent tests
- Clean up resources with `t.Cleanup()`
- Write test names that describe the scenario and expected outcome

**Don't:**
- Test private functions directly — test through the public API
- Use `time.Sleep()` in tests — use channels or conditions instead
- Ignore flaky tests — fix or delete them
- Mock everything — prefer integration tests where setup cost is manageable
- Skip error path testing

---

## Integration with CI/CD

```yaml
# GitHub Actions
test:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: "1.22"

    - name: Run tests
      run: go test -race -coverprofile=coverage.out ./...

    - name: Check coverage threshold
      run: |
        go tool cover -func=coverage.out | grep total \
          | awk -F'%' '{if ($1+0 < 80) { print "Coverage below 80%"; exit 1 }}'
```

**Remember**: Tests are documentation. They show how your code is meant to be used. Write them
clearly and keep them current.
