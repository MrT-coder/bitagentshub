package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// textCall is one call of T or Tf in the office's code.
type textCall struct {
	where string // file:line
	text  string // the English text, "" when the argument is not a literal
}

// textCalls finds every T and Tf call in the non-test files of the root
// package and internal/. A call must pass its English text as a literal,
// so the catalog can be checked here; only Tf's own body passes a variable.
func textCalls(t *testing.T) []textCall {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	top, _ := filepath.Glob(filepath.Join(root, "*.go"))
	files = append(files, top...)
	err = filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var calls []textCall
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		own := f.Name.Name == "i18n"
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && own && fn.Recv == nil && (fn.Name.Name == "T" || fn.Name.Name == "Tf") {
				continue // the lookup itself
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 || !isText(call.Fun, own) {
					return true
				}
				c := textCall{where: fset.Position(call.Pos()).String()}
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					c.text, _ = strconv.Unquote(lit.Value)
				}
				calls = append(calls, c)
				return true
			})
		}
	}
	return calls
}

// isText reports a call of i18n.T or i18n.Tf, or of T and Tf inside i18n.
func isText(fun ast.Expr, own bool) bool {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		x, ok := f.X.(*ast.Ident)
		return ok && x.Name == "i18n" && (f.Sel.Name == "T" || f.Sel.Name == "Tf")
	case *ast.Ident:
		return own && (f.Name == "T" || f.Name == "Tf")
	}
	return false
}

var verb = regexp.MustCompile(`%[-+# 0-9.*]*[a-zA-Z%]`)

func TestEveryTextHasItsSpanishVersion(t *testing.T) {
	calls := textCalls(t)
	if len(calls) < 50 {
		t.Fatalf("found only %d texts; is the scan looking in the right place?", len(calls))
	}
	used := map[string]bool{}
	for _, c := range calls {
		switch es, ok := spanish[c.text]; {
		case c.text == "":
			t.Errorf("%s: the text is not a string literal, so the catalog cannot be checked", c.where)
		case !ok:
			t.Errorf("%s: no Spanish for %q", c.where, c.text)
		case !slices.Equal(verb.FindAllString(c.text, -1), verb.FindAllString(es, -1)):
			t.Errorf("%s: %q and %q do not keep the same verbs in the same order", c.where, c.text, es)
		}
		used[c.text] = true
	}
	for en := range spanish {
		if !used[en] {
			t.Errorf("the catalog has %q, which no code uses", en)
		}
	}
}
