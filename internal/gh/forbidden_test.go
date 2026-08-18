package gh

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// forbiddenVerbs are gh stack subcommands ADR 0003 and issue #137 forbid:
// the first four assume a single checkout, the last two need tracking state
// `gh stack link` does not create.
var forbiddenVerbs = []string{"init", "add", "submit", "modify", "rebase", "sync"}

// TestNoForbiddenGHSurface asserts this package's non-test source names no
// forbidden verb as an identifier and embeds none as a string literal — the
// latter is what would appear as a `gh stack <verb>` argument.
func TestNoForbiddenGHSurface(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, e.Name(), nil, parser.AllErrors)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.Ident:
				if isForbidden(v.Name) {
					t.Errorf("%s: identifier %q names a forbidden gh stack verb", e.Name(), v.Name)
				}
			case *ast.BasicLit:
				if v.Kind == token.STRING {
					val := strings.Trim(v.Value, `"`+"`")
					if isForbidden(val) {
						t.Errorf("%s: string literal %q names a forbidden gh stack verb", e.Name(), val)
					}
				}
			}
			return true
		})
	}
}

func isForbidden(s string) bool {
	for _, v := range forbiddenVerbs {
		if s == v {
			return true
		}
	}
	return false
}
