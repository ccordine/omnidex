package worker

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

func directCodingGoTaskAcceptanceName(
	program directCodingProgram,
	taskID string,
) (string, error) {
	blockID, err := directCodingTaskBlockIDByRole(
		program.Source, taskID, assemblyline.SourceBlockTaskVerification,
	)
	if err != nil {
		return "", err
	}
	block, exists := directCodingSourceBlueprintBlock(program.Source, blockID)
	if !exists {
		return "", fmt.Errorf("Go task %s acceptance block %s is absent", taskID, blockID)
	}
	compiled, err := gofragment.CompileNewFunctionSignature(block.Signature)
	if err != nil {
		return "", fmt.Errorf("compile Go task %s acceptance signature: %w", taskID, err)
	}
	if !strings.HasPrefix(compiled.Name, "TestFeature") {
		return "", fmt.Errorf("Go task %s acceptance function %s is not a test", taskID, compiled.Name)
	}
	return compiled.Name, nil
}

func validateDirectCodingGoAcceptance(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	source string,
) error {
	if stage == nil || ref.Block.Role != assemblyline.SourceBlockTaskVerification {
		return fmt.Errorf("Go acceptance validation requires one task verification block")
	}
	acceptanceName, err := directCodingGoTaskAcceptanceName(*stage, ref.Block.TaskID)
	if err != nil {
		return err
	}
	implementationName, err := directCodingGoAcceptanceImplementationName(stage, ref)
	if err != nil {
		return err
	}
	requiredAssertions, err := directCodingAcceptanceObligationCount(stage, ref)
	if err != nil {
		return err
	}
	parsed, err := parser.ParseFile(
		token.NewFileSet(), "", "package main\n"+source, parser.AllErrors,
	)
	if err != nil {
		return fmt.Errorf("parse Go acceptance block %s: %w", ref.Block.ID, err)
	}
	function, err := goAcceptanceFunction(parsed, acceptanceName)
	if err != nil {
		return fmt.Errorf("Go acceptance block %s: %w", ref.Block.ID, err)
	}
	featureCalls := goAcceptanceCallCount(function.Body, implementationName)
	resultNames, assertions, valid := inspectGoAcceptanceBody(
		function.Body, implementationName,
	)
	if featureCalls != 1 || len(resultNames) != 1 || !valid ||
		len(assertions) < requiredAssertions {
		return fmt.Errorf(
			"Go acceptance block %s must bind one %s result and prove the accepted requirement with %d direct result-field failure condition",
			ref.Block.ID, implementationName, requiredAssertions,
		)
	}
	return nil
}

func directCodingGoAcceptanceImplementationName(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	name := ""
	for _, dependencyID := range ref.Block.DependsOn {
		dependency, exists := directCodingSourceBlueprintBlock(stage.Source, dependencyID)
		if !exists || dependency.Role != assemblyline.SourceBlockTaskImplementation {
			continue
		}
		compiled, err := gofragment.CompileNewFunctionSignature(dependency.Signature)
		if err != nil {
			return "", err
		}
		if name != "" {
			return "", fmt.Errorf("Go acceptance block %s has multiple implementation owners", ref.Block.ID)
		}
		name = compiled.Name
	}
	if name == "" {
		return "", fmt.Errorf("Go acceptance block %s has no implementation owner", ref.Block.ID)
	}
	return name, nil
}

func goAcceptanceFunction(file *ast.File, expectedName string) (*ast.FuncDecl, error) {
	if file == nil || len(file.Decls) != 1 {
		return nil, fmt.Errorf("must contain exactly one code-owned test declaration")
	}
	function, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || function.Body == nil || function.Name.Name != expectedName {
		return nil, fmt.Errorf("must contain exactly the code-owned %s declaration", expectedName)
	}
	return function, nil
}

func goAcceptanceCallCount(body *ast.BlockStmt, implementationName string) int {
	calls := 0
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := call.Fun.(*ast.Ident)
		if ok && identifier.Name == implementationName {
			calls++
		}
		return true
	})
	return calls
}

func inspectGoAcceptanceBody(
	body *ast.BlockStmt,
	implementationName string,
) (map[string]struct{}, map[string]struct{}, bool) {
	results := make(map[string]struct{})
	assertions := make(map[string]struct{})
	valid := body != nil
	if body == nil {
		return results, assertions, false
	}
	for _, raw := range body.List {
		switch statement := raw.(type) {
		case *ast.AssignStmt:
			if len(statement.Lhs) == 1 && len(statement.Rhs) == 1 {
				identifier, named := statement.Lhs[0].(*ast.Ident)
				if named && goAcceptanceExactCall(statement.Rhs[0], implementationName) {
					results[identifier.Name] = struct{}{}
				}
			}
		case *ast.IfStmt:
			condition, meaningful := goAcceptanceMeaningfulCondition(statement.Cond, results)
			if meaningful && goAcceptanceBodyFailsDirectly(statement.Body) {
				assertions[condition] = struct{}{}
			} else {
				valid = false
			}
		case *ast.DeclStmt, *ast.EmptyStmt:
		default:
			valid = false
		}
	}
	return results, assertions, valid
}

func goAcceptanceExactCall(expression ast.Expr, implementationName string) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}
	identifier, ok := call.Fun.(*ast.Ident)
	return ok && identifier.Name == implementationName
}

func goAcceptanceMeaningfulCondition(
	expression ast.Expr,
	resultNames map[string]struct{},
) (string, bool) {
	if expression == nil || goAcceptanceContainsBooleanShortcut(expression) ||
		goAcceptanceSelfComparison(expression) {
		return "", false
	}
	observesResult := false
	ast.Inspect(expression, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok {
			_, observesResult = resultNames[identifier.Name]
		}
		return !observesResult
	})
	if !observesResult {
		return "", false
	}
	var rendered bytes.Buffer
	if err := format.Node(&rendered, token.NewFileSet(), expression); err != nil {
		return "", false
	}
	return rendered.String(), true
}

func goAcceptanceContainsBooleanShortcut(expression ast.Expr) bool {
	invalid := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch candidate := node.(type) {
		case *ast.BinaryExpr:
			invalid = candidate.Op == token.LAND || candidate.Op == token.LOR
		case *ast.Ident:
			invalid = candidate.Name == "true" || candidate.Name == "false"
		}
		return !invalid
	})
	return invalid
}

func goAcceptanceSelfComparison(expression ast.Expr) bool {
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	var left, right bytes.Buffer
	if format.Node(&left, token.NewFileSet(), binary.X) != nil ||
		format.Node(&right, token.NewFileSet(), binary.Y) != nil {
		return true
	}
	return left.String() == right.String()
}

func goAcceptanceBodyFailsDirectly(body *ast.BlockStmt) bool {
	if body == nil || len(body.List) == 0 {
		return false
	}
	fails := false
	for _, raw := range body.List {
		expression, ok := raw.(*ast.ExprStmt)
		if !ok {
			return false
		}
		call, ok := expression.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || receiver.Name != "t" {
			return false
		}
		switch selector.Sel.Name {
		case "Fatal", "Fatalf", "Error", "Errorf":
			fails = true
		default:
			return false
		}
	}
	return fails
}
