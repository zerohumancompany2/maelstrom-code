package tools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type ReadSymbolTool struct {
	RootDir string
}

func (ReadSymbolTool) Definition() Definition {
	return Definition{
		Name:        "read_symbol",
		Description: "Read a named symbol definition from a source file",
		Parameters: map[string]string{
			"path":   "string",
			"symbol": "string",
		},
		Required: []string{"path", "symbol"},
	}
}

func (t ReadSymbolTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	relPath := request.Call.Call.Arguments["path"]
	symbol := request.Call.Call.Arguments["symbol"]
	if strings.TrimSpace(relPath) == "" {
		return ExecutionResult{ToolName: "read_symbol", DisplayContent: "symbol error: path is required", IsError: true}, nil
	}
	if strings.TrimSpace(symbol) == "" {
		return ExecutionResult{ToolName: "read_symbol", DisplayContent: "symbol error: symbol is required", IsError: true}, nil
	}
	absPath := relPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(t.RootDir, relPath)
	}
	if filepath.Ext(absPath) != ".go" {
		return ExecutionResult{ToolName: "read_symbol", DisplayContent: fmt.Sprintf("symbol error: unsupported file type %q", filepath.Ext(absPath)), IsError: true}, nil
	}
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		return ExecutionResult{ToolName: "read_symbol", DisplayContent: fmt.Sprintf("symbol error: %v", err), IsError: true}, nil
	}
	content := string(contentBytes)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, content, parser.ParseComments)
	if err != nil {
		return ExecutionResult{ToolName: "read_symbol", DisplayContent: fmt.Sprintf("symbol error: %v", err), IsError: true}, nil
	}
	result, ok := extractGoSymbol(fset, file, content, relPath, symbol)
	if !ok {
		return ExecutionResult{ToolName: "read_symbol", DisplayContent: fmt.Sprintf("symbol error: %q not found in %s", symbol, relPath), IsError: true}, nil
	}
	return ExecutionResult{ToolName: "read_symbol", DisplayContent: result}, nil
}

func extractGoSymbol(fset *token.FileSet, file *ast.File, content, relPath, symbol string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			fullName := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := summarizeExpr(d.Recv.List[0].Type)
				fullName = recv + "." + d.Name.Name
			}
			if !matchesSymbol(symbol, fullName, d.Name.Name) {
				continue
			}
			start := fset.Position(d.Pos()).Line
			end := fset.Position(d.End()).Line
			return formatSymbolResult(relPath, fullName, start, end, lines), true
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if !matchesSymbol(symbol, s.Name.Name, s.Name.Name) {
						continue
					}
					start := fset.Position(s.Pos()).Line
					end := fset.Position(s.End()).Line
					return formatSymbolResult(relPath, s.Name.Name, start, end, lines), true
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if !matchesSymbol(symbol, name.Name, name.Name) {
							continue
						}
						start := fset.Position(s.Pos()).Line
						end := fset.Position(s.End()).Line
						return formatSymbolResult(relPath, name.Name, start, end, lines), true
					}
				}
			}
		}
	}
	return "", false
}

func matchesSymbol(requested, fullName, shortName string) bool {
	normalizedRequested := strings.TrimSpace(strings.TrimPrefix(requested, "*"))
	normalizedFull := strings.TrimSpace(strings.TrimPrefix(fullName, "*"))
	if normalizedRequested == normalizedFull || normalizedRequested == shortName {
		return true
	}
	if strings.HasSuffix(normalizedFull, "."+normalizedRequested) {
		return true
	}
	return false
}

func formatSymbolResult(relPath, name string, startLine, endLine int, lines []string) string {
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	body := formatLinesWithNumbers(lines[startLine-1:endLine], startLine)
	return fmt.Sprintf("%s::%s\nLines: %d-%d\n\n%s", relPath, name, startLine, endLine, body)
}
