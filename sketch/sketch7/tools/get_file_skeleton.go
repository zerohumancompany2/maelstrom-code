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

type GetFileSkeletonTool struct {
	RootDir string
}

func (GetFileSkeletonTool) Definition() Definition {
	return Definition{
		Name:        "get_file_skeleton",
		Description: "Read compact structural information from a source file",
		Parameters: map[string]string{
			"path": "string",
		},
	}
}

func (t GetFileSkeletonTool) Execute(request ExecutionRequest) (ExecutionResult, error) {
	relPath := request.Call.Call.Arguments["path"]
	if strings.TrimSpace(relPath) == "" {
		return ExecutionResult{ToolName: "get_file_skeleton", DisplayContent: "skeleton error: path is required", IsError: true}, nil
	}
	absPath := relPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(t.RootDir, relPath)
	}
	if filepath.Ext(absPath) != ".go" {
		return ExecutionResult{ToolName: "get_file_skeleton", DisplayContent: fmt.Sprintf("skeleton error: unsupported file type %q", filepath.Ext(absPath)), IsError: true}, nil
	}
	if _, err := os.Stat(absPath); err != nil {
		return ExecutionResult{ToolName: "get_file_skeleton", DisplayContent: fmt.Sprintf("skeleton error: %v", err), IsError: true}, nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return ExecutionResult{ToolName: "get_file_skeleton", DisplayContent: fmt.Sprintf("skeleton error: %v", err), IsError: true}, nil
	}
	output := buildGoSkeleton(file)
	return ExecutionResult{ToolName: "get_file_skeleton", DisplayContent: output}, nil
}

func buildGoSkeleton(file *ast.File) string {
	lines := []string{fmt.Sprintf("package %s", file.Name.Name), ""}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					lines = append(lines, fmt.Sprintf("type %s %s", s.Name.Name, summarizeExpr(s.Type)))
				case *ast.ValueSpec:
					if len(s.Names) == 0 {
						continue
					}
					kind := strings.ToLower(d.Tok.String())
					for _, name := range s.Names {
						lines = append(lines, fmt.Sprintf("%s %s", kind, name.Name))
					}
				}
			}
		case *ast.FuncDecl:
			lines = append(lines, summarizeFuncDecl(d))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func summarizeFuncDecl(fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recvType := summarizeExpr(fn.Recv.List[0].Type)
		return fmt.Sprintf("func (%s) %s(...)", recvType, fn.Name.Name)
	}
	return fmt.Sprintf("func %s(...)", fn.Name.Name)
}

func summarizeExpr(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + summarizeExpr(v.X)
	case *ast.SelectorExpr:
		return summarizeExpr(v.X) + "." + v.Sel.Name
	default:
		return fmt.Sprintf("%T", expr)
	}
}
