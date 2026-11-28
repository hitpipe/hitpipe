package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"math"
	"os"
	"sort"
)

func tally(values map[string]int) int {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	total := 0
	for _, key := range keys {
		fmt.Printf("  %-31q %d\n", key, values[key])
		total += values[key]
	}
	return total
}

func main() {
	var path string
	flag.StringVar(&path, "source", "", "path to projection.go")
	flag.Parse()
	if path == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		panic(err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || (function.Name.Name != "Decode" && function.Name.Name != "decodeFields") {
			continue
		}
		start, end := fset.Position(function.Body.Pos()), fset.Position(function.Body.End())
		var lexer scanner.Scanner
		block := src[start.Offset:end.Offset]
		lexer.Init(token.NewFileSet().AddFile(path, -1, len(block)), block, nil, 0)
		operators, operands := map[string]int{}, map[string]int{}
		for {
			_, tok, literal := lexer.Scan()
			if tok == token.EOF {
				break
			}
			switch tok {
			case token.LPAREN, token.RPAREN, token.LBRACK, token.RBRACK, token.LBRACE,
				token.RBRACE, token.COMMA, token.SEMICOLON, token.PERIOD:
				continue
			}
			if tok.IsOperator() || tok.IsKeyword() {
				operators[tok.String()]++
			} else if tok.IsLiteral() {
				operands[literal]++
			}
		}
		branches := map[string]int{}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch element := node.(type) {
			case *ast.IfStmt:
				branches["if"]++
			case *ast.ForStmt, *ast.RangeStmt:
				branches["for"]++
			case *ast.CaseClause:
				if element.List != nil {
					branches["case"]++
				}
			case *ast.CommClause:
				if element.Comm != nil {
					branches["case"]++
				}
			case *ast.BinaryExpr:
				if element.Op == token.LAND || element.Op == token.LOR {
					branches[element.Op.String()]++
				}
			}
			return true
		})
		fmt.Printf("%s · строки %d–%d\nОператоры\n", function.Name.Name, start.Line, end.Line)
		N1 := tally(operators)
		fmt.Println("Операнды")
		N2 := tally(operands)
		n1, n2 := len(operators), len(operands)
		n, N := n1+n2, N1+N2
		V := float64(N) * math.Log2(float64(n))
		D := (float64(n1) / 2) * float64(N2) / float64(n2)
		fmt.Println("Точки решения")
		cc := tally(branches) + 1
		fmt.Printf("n1=%d N1=%d n2=%d N2=%d n=%d N=%d V=%.2f D=%.2f E=%.2f B=%.4f CC=%d\n", n1, N1, n2, N2, n, N, V, D, D*V, V/3000, cc)
	}
}
