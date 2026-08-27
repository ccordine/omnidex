package worker

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

const directCodingGoStageTimeout = 2 * time.Minute

func newDirectCodingGoProjectStageExecutor(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectStageExecutor, error) {
	return newDirectCodingLanguageProjectStageExecutor(session, directCodingLanguageStageConfig{
		Language: "go", AdapterID: "go", Timeout: directCodingGoStageTimeout,
		ProjectFragment:    projectDirectCodingGoFragment,
		ValidateFragment:   validateDirectCodingGoFragment,
		ValidateAcceptance: validateDirectCodingGoAcceptance,
		TaskCommands: func(
			context assemblyline.ApplicationTaskContext,
			_ directCodingProgram,
		) ([]testCommand, error) {
			name := "TestFeature" + strings.TrimPrefix(context.Task.TaskID, "task_")
			return []testCommand{{Family: "go", Name: "go", Args: []string{
				"test", "-count=1", "-run", "^" + name + "$", "./...",
			}, Purpose: verificationTest}}, nil
		},
		FinalCommands: func(directCodingProgram) ([]testCommand, error) {
			return goCommandLineVerificationCommandSet(), nil
		},
	})
}

func validateDirectCodingGoFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	return gofragment.ParseNewFunction(input.Signature, input.PermittedSymbols, candidate)
}

func validateDirectCodingGoAcceptance(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	source string,
) error {
	implementationName := ""
	for _, dependencyID := range ref.Block.DependsOn {
		dependency, exists := directCodingSourceBlueprintBlock(stage.Source, dependencyID)
		if !exists || dependency.Role != assemblyline.SourceBlockTaskImplementation {
			continue
		}
		compiled, err := gofragment.CompileNewFunctionSignature(dependency.Signature)
		if err != nil {
			return err
		}
		if implementationName != "" {
			return fmt.Errorf("Go acceptance block %s has multiple implementation owners", ref.Block.ID)
		}
		implementationName = compiled.Name
	}
	if implementationName == "" {
		return fmt.Errorf("Go acceptance block %s has no implementation owner", ref.Block.ID)
	}
	requiredAssertions, err := directCodingAcceptanceCriterionCount(stage, ref)
	if err != nil {
		return err
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "", "package main\n"+source, parser.AllErrors)
	if err != nil {
		return err
	}
	resultNames := make(map[string]struct{})
	featureCalls := 0
	assertions := make(map[string]struct{})
	var function *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		if candidate, ok := declaration.(*ast.FuncDecl); ok {
			function = candidate
			break
		}
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok {
			if identifier, ok := call.Fun.(*ast.Ident); ok && identifier.Name == implementationName {
				featureCalls++
			}
		}
		return true
	})
	invalidStructure := function == nil || function.Body == nil
	if !invalidStructure {
		for _, rawStatement := range function.Body.List {
			switch statement := rawStatement.(type) {
			case *ast.AssignStmt:
				if len(statement.Lhs) == 1 && len(statement.Rhs) == 1 {
					if identifier, ok := statement.Lhs[0].(*ast.Ident); ok &&
						goAcceptanceExactCall(statement.Rhs[0], implementationName) {
						resultNames[identifier.Name] = struct{}{}
					}
				}
			case *ast.IfStmt:
				resultName, condition, meaningful := goAcceptanceMeaningfulCondition(
					statement.Cond, resultNames,
				)
				if meaningful && resultName != "" && goAcceptanceBodyFails(statement.Body) {
					assertions[condition] = struct{}{}
				}
			case *ast.DeclStmt, *ast.EmptyStmt:
			default:
				invalidStructure = true
			}
		}
	}
	if featureCalls != 1 || len(resultNames) != 1 || invalidStructure ||
		len(assertions) < requiredAssertions {
		return fmt.Errorf(
			"Go acceptance block %s must bind one %s result and prove all %d frozen criteria with distinct result-field failure conditions",
			ref.Block.ID, implementationName, requiredAssertions,
		)
	}
	return nil
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
) (string, string, bool) {
	if expression == nil || goAcceptanceContainsBooleanShortcut(expression) ||
		goAcceptanceSelfComparison(expression) {
		return "", "", false
	}
	resultName := ""
	ast.Inspect(expression, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, exists := resultNames[identifier.Name]; exists {
			resultName = identifier.Name
		}
		return true
	})
	if resultName == "" {
		return "", "", false
	}
	var rendered bytes.Buffer
	if err := format.Node(&rendered, token.NewFileSet(), expression); err != nil {
		return "", "", false
	}
	return resultName, rendered.String(), true
}

func goAcceptanceContainsBooleanShortcut(expression ast.Expr) bool {
	invalid := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch candidate := node.(type) {
		case *ast.BinaryExpr:
			if candidate.Op == token.LAND || candidate.Op == token.LOR {
				invalid = true
			}
		case *ast.Ident:
			if candidate.Name == "true" || candidate.Name == "false" {
				invalid = true
			}
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

func goAcceptanceBodyFails(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	fails := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || receiver.Name != "t" {
			return true
		}
		switch selector.Sel.Name {
		case "Fatal", "Fatalf", "Error", "Errorf":
			fails = true
		}
		return true
	})
	return fails
}
