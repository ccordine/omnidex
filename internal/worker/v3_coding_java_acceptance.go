package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javagrammar "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func javaAcceptanceRequiredAssertions(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (int, error) {
	if stage == nil || ref.Block.TaskID == "" {
		return 0, fmt.Errorf("frozen acceptance task authority is required")
	}
	found := false
	criteria := 0
	for _, task := range stage.Workload.Tasks {
		if task.ID != ref.Block.TaskID {
			continue
		}
		if found {
			return 0, fmt.Errorf("frozen workload repeats task %s", ref.Block.TaskID)
		}
		found = true
		criteria = len(task.AcceptanceCriteria)
	}
	if !found || criteria == 0 {
		return 0, fmt.Errorf("frozen task %s has no acceptance criteria", ref.Block.TaskID)
	}
	return criteria, nil
}

func inspectJavaAcceptance(
	source []byte,
	featureClass string,
	featureMethod string,
	requiredAssertions int,
) error {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(javagrammar.Language())); err != nil {
		return err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return fmt.Errorf("Java acceptance parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return fmt.Errorf("Java acceptance source is not parseable")
	}
	featureCalls := 0
	declarations := make(map[string]int)
	resultNames := make(map[string]struct{})
	assignments := make(map[string]int)
	shapeCalls := make([]*treesitter.Node, 0, 1)
	assertionCalls := make([]*treesitter.Node, 0, requiredAssertions)
	forbiddenCatch := false
	nestedAssertion := false
	javaWalkTree(root, func(node *treesitter.Node) {
		if node.Kind() == "catch_clause" {
			forbiddenCatch = true
		}
		if javaExactMethodInvocation(source, node, featureClass, featureMethod) {
			featureCalls++
		}
		if node.Kind() == "variable_declarator" {
			name, value := node.ChildByFieldName("name"), node.ChildByFieldName("value")
			if name == nil {
				return
			}
			identifier := javaNodeText(name, source)
			declarations[identifier]++
			if javaExactMethodInvocation(source, value, featureClass, featureMethod) {
				resultNames[identifier] = struct{}{}
			}
		}
		if node.Kind() == "assignment_expression" {
			left := node.ChildByFieldName("left")
			if left != nil && left.Kind() == "identifier" {
				assignments[javaNodeText(left, source)]++
			}
		}
		if javaExactMethodInvocation(source, node, "Runtime", "requireResult") {
			shapeCalls = append(shapeCalls, node)
			if !javaDirectMethodBodyInvocation(node) {
				nestedAssertion = true
			}
		}
		if javaExactMethodInvocation(source, node, "Runtime", "require") {
			assertionCalls = append(assertionCalls, node)
			if !javaDirectMethodBodyInvocation(node) {
				nestedAssertion = true
			}
		}
	})
	if forbiddenCatch {
		return fmt.Errorf("Java acceptance cannot catch verification failures")
	}
	if nestedAssertion {
		return fmt.Errorf("Java acceptance assertions must be direct method-body statements")
	}
	if featureCalls != 1 || len(resultNames) != 1 {
		return fmt.Errorf("Java acceptance requires one exact stored %s.%s result", featureClass, featureMethod)
	}
	resultName := ""
	for name := range resultNames {
		resultName = name
	}
	if declarations[resultName] != 1 || assignments[resultName] != 0 {
		return fmt.Errorf("Java acceptance result binding %s must be immutable and unique", resultName)
	}
	if len(shapeCalls) != 1 || !javaCallHasExactIdentifier(shapeCalls[0], source, resultName) {
		return fmt.Errorf("Java acceptance must pass only %s to Runtime.requireResult", resultName)
	}
	if len(assertionCalls) < requiredAssertions {
		return fmt.Errorf(
			"Java acceptance has %d result assertions for %d criteria",
			len(assertionCalls), requiredAssertions,
		)
	}
	seen := make(map[string]struct{}, len(assertionCalls))
	for _, call := range assertionCalls {
		if call.StartByte() < shapeCalls[0].StartByte() {
			return fmt.Errorf("Java acceptance must validate %s before its result conditions", resultName)
		}
		arguments := javaCallArguments(call)
		if len(arguments) != 2 {
			return fmt.Errorf("Runtime.require requires one condition and one failure message")
		}
		condition := arguments[0]
		if err := javaValidateAcceptanceCondition(source, condition, resultName); err != nil {
			return err
		}
		canonical := javaCanonicalExpression(source, condition)
		if _, duplicate := seen[canonical]; duplicate {
			return fmt.Errorf("Java acceptance repeats result condition %s", canonical)
		}
		seen[canonical] = struct{}{}
	}
	return nil
}

func javaDirectMethodBodyInvocation(node *treesitter.Node) bool {
	if node == nil {
		return false
	}
	statement := node.Parent()
	if statement == nil || statement.Kind() != "expression_statement" {
		return false
	}
	body := statement.Parent()
	if body == nil || body.Kind() != "block" {
		return false
	}
	method := body.Parent()
	if method == nil || method.Kind() != "method_declaration" {
		return false
	}
	declaredBody := method.ChildByFieldName("body")
	return declaredBody != nil && declaredBody.Id() == body.Id()
}
