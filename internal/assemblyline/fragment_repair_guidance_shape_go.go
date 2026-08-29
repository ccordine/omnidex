package assemblyline

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"strings"
)

// fragmentRepairGuidanceContainsGoDeclaration uses the Go lexer to locate
// declaration starts and balanced candidate endings. Strings and comments are
// atomic lexer tokens, so source-looking user-visible text inside them is not
// mistaken for declaration authority.
func fragmentRepairGuidanceContainsGoDeclaration(
	instruction string,
) bool {
	fileSet := token.NewFileSet()
	file := fileSet.AddFile("", fileSet.Base(), len(instruction))
	var lexical scanner.Scanner
	lexical.Init(file, []byte(instruction), nil, scanner.ScanComments)
	starts := make([]int, 0, 2)
	for {
		position, current, _ := lexical.Scan()
		if current == token.EOF {
			return false
		}
		offset := file.Offset(position)
		switch current {
		case token.FUNC:
			starts = append(starts, offset)
		case token.RBRACE:
			for _, start := range starts {
				if start < offset &&
					fragmentRepairGuidanceIsCompleteGoDeclaration(instruction[start:offset+1]) {
					return true
				}
			}
		}
	}
}

func fragmentRepairGuidanceIsCompleteGoDeclaration(source string) bool {
	file, err := parser.ParseFile(
		token.NewFileSet(), "", "package guidance\n\n"+source, parser.AllErrors,
	)
	if err != nil || file == nil || len(file.Decls) != 1 {
		return false
	}
	declaration, ok := file.Decls[0].(*ast.FuncDecl)
	return ok && declaration.Body != nil
}

func fragmentRepairGuidanceIsWholeGoSourceBody(source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	const prefix = "package guidance\n\nfunc omnidexRepairInstruction(){\n"
	const suffix = "\n}"
	wrapped := prefix + source + suffix
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "", wrapped, parser.AllErrors)
	if err != nil || file == nil || len(file.Decls) != 1 {
		return false
	}
	declaration, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || declaration.Name == nil ||
		declaration.Name.Name != "omnidexRepairInstruction" ||
		declaration.Body == nil || len(declaration.Body.List) == 0 {
		return false
	}
	opening := fileSet.Position(declaration.Body.Lbrace).Offset
	closing := fileSet.Position(declaration.Body.Rbrace).Offset
	return opening == len(prefix)-2 && closing == len(prefix)+len(source)+1
}
