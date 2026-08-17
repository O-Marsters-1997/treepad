package batch

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// internalAllowlist names internal packages batch may import. An exported
// signature can only name an internal type if the package importing it does
// so, so this is a stricter guarantee than walking signatures directly.
var internalAllowlist = map[string]string{
	// exports only func Slug(string) string — no type it could leak into a
	// public signature.
	"github.com/O-Marsters-1997/treepad/internal/slug": "no exported types",
}

// TestNoInternalImports asserts batch's non-test files import nothing under
// internal/... beyond internalAllowlist. This is the regression guard for two
// acceptance criteria at once: no exported identifier here may name a type
// declared under internal/..., and batch must not import
// internal/treepad/fromspec specifically.
func TestNoInternalImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}

		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquote import %s: %v", e.Name(), imp.Path.Value, err)
			}
			if !strings.Contains(path, "/internal/") {
				continue
			}
			if _, ok := internalAllowlist[path]; ok {
				continue
			}
			t.Errorf("%s imports %s, which is not in internalAllowlist", e.Name(), path)
		}
	}
}
