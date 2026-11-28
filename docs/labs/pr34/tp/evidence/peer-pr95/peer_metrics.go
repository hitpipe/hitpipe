// peer_metrics reports cyclomatic complexity and exact five-line clones for PR #95.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type position struct {
	File     string `json:"file"`
	Function string `json:"function"`
	From     int    `json:"from"`
	To       int    `json:"to"`
}
type function struct {
	position
	Complexity int `json:"complexity"`
}
type codeLine struct {
	number int
	tokens []string
}
type occurrence struct {
	position
	lines []int
}
type clones struct {
	SignificantLines int          `json:"significant_lines"`
	DuplicatedLines  int          `json:"duplicated_lines"`
	Groups           [][]position `json:"groups"`
}

type result struct {
	Functions []function `json:"functions"`
	Missing   []string   `json:"missing_files"`
	Product   clones     `json:"product_clones"`
	Tests     clones     `json:"test_clones"`
}

func complexity(body *ast.BlockStmt) int {
	value := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			value++
		case *ast.CaseClause:
			if node.List != nil {
				value++
			}
		case *ast.CommClause:
			if node.Comm != nil {
				value++
			}
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				value++
			}
		}
		return true
	})
	return value
}

func scanLines(file string, src []byte, firstLine int) []codeLine {
	var lexer scanner.Scanner
	fset := token.NewFileSet()
	f := fset.AddFile(file, -1, len(src))
	lexer.Init(f, src, nil, 0)
	var lines []codeLine
	for {
		pos, tok, lit := lexer.Scan()
		if tok == token.EOF {
			return lines
		}
		switch tok {
		case token.LBRACE, token.RBRACE, token.SEMICOLON, token.COMMENT:
			continue
		}
		if lit == "" {
			lit = tok.String()
		}
		number := firstLine - 1 + f.Line(pos)
		if len(lines) == 0 || lines[len(lines)-1].number != number {
			lines = append(lines, codeLine{number: number})
		}
		line := &lines[len(lines)-1]
		line.tokens = append(line.tokens, lit)
	}
}

func findClones(total int, sightings map[string][]occurrence) clones {
	out := clones{SignificantLines: total, Groups: make([][]position, 0)}
	covered := map[string]bool{}
	keys := make([]string, 0, len(sightings))
	for key, copies := range sightings {
		if len(copies) >= 2 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		copies := sightings[key]
		group := make([]position, 0, len(copies))
		for _, copy := range copies {
			group = append(group, copy.position)
			for _, line := range copy.lines {
				covered[fmt.Sprintf("%s:%d", copy.File, line)] = true
			}
		}
		out.Groups = append(out.Groups, group)
	}
	out.DuplicatedLines = len(covered)
	return out
}

func main() {
	root := flag.String("root", ".", "delivery module directory")
	flag.Parse()
	paths := []string{
		"internal/intake/intent.go", "internal/intake/postgres_linux_test.go",
		"internal/clickup/client.go", "internal/clickup/client_test.go",
	}
	files := make([]function, 0)
	missing := make([]string, 0)
	product, tests := map[string][]occurrence{}, map[string][]occurrence{}
	productLines, testLines := 0, 0
	for _, path := range paths {
		src, err := os.ReadFile(filepath.Join(*root, path))
		if os.IsNotExist(err) {
			missing = append(missing, path)
			continue
		}
		if err != nil {
			panic(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			panic(err)
		}
		isTest := strings.HasSuffix(path, "_test.go")
		sightings := product
		total := &productLines
		if isTest {
			sightings, total = tests, &testLines
		}
		for _, declaration := range f.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			begin, end := fset.Position(fn.Body.Pos()), fset.Position(fn.Body.End())
			pos := position{path, fn.Name.Name, fset.Position(fn.Pos()).Line, end.Line}
			files = append(files, function{pos, complexity(fn.Body)})
			lines := scanLines(path, src[begin.Offset:end.Offset], begin.Line)
			*total += len(lines)
			const window = 5
			for i := 0; i+window <= len(lines); i++ {
				var signature strings.Builder
				numbers := make([]int, 0, window)
				for _, line := range lines[i : i+window] {
					signature.WriteString(strings.Join(line.tokens, "\x1f"))
					signature.WriteByte('\x1e')
					numbers = append(numbers, line.number)
				}
				key := signature.String()
				sightings[key] = append(sightings[key], occurrence{position{path, fn.Name.Name, numbers[0], numbers[len(numbers)-1]}, numbers})
			}
		}
	}
	report := result{files, missing, findClones(productLines, product), findClones(testLines, tests)}
	writer := json.NewEncoder(os.Stdout)
	writer.SetIndent("", "  ")
	if err := writer.Encode(report); err != nil {
		panic(err)
	}
}
