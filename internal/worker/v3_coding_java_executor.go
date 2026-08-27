package worker

import (
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javagrammar "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

const directCodingJavaStageTimeout = 2 * time.Minute

func newDirectCodingJavaProjectStageExecutor(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectStageExecutor, error) {
	return newDirectCodingLanguageProjectStageExecutor(session, directCodingLanguageStageConfig{
		Language: "java", AdapterID: "java", Timeout: directCodingJavaStageTimeout,
		ProjectFragment:    assemblyline.ProjectJavaFragment,
		ValidateFragment:   validateDirectCodingJavaFragment,
		ValidateAcceptance: validateDirectCodingJavaAcceptance,
		TaskCommands:       javaCommandLineTaskCommands,
		FinalCommands:      javaCommandLineVerificationCommands,
	})
}

func validateDirectCodingJavaFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	validated, err := assemblyline.ValidateJavaFragment(input.Signature, candidate)
	if err != nil {
		return "", err
	}
	if err := validateDirectCodingJavaScope(input, validated); err != nil {
		return "", err
	}
	return validated, nil
}

func validateDirectCodingJavaAcceptance(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	source string,
) error {
	validated, err := assemblyline.ValidateJavaFragment(ref.Block.Signature, source)
	if err != nil {
		return err
	}
	featureClass, featureMethod, err := javaAcceptanceImplementation(stage, ref)
	if err != nil {
		return fmt.Errorf("Java acceptance block %s: %w", ref.Block.ID, err)
	}
	requiredAssertions, err := javaAcceptanceRequiredAssertions(stage, ref)
	if err != nil {
		return fmt.Errorf("Java acceptance block %s: %w", ref.Block.ID, err)
	}
	if err := inspectJavaAcceptance(
		[]byte(validated), featureClass, featureMethod, requiredAssertions,
	); err != nil {
		return fmt.Errorf("Java acceptance block %s: %w", ref.Block.ID, err)
	}
	return nil
}

func javaAcceptanceImplementation(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, string, error) {
	if stage == nil {
		return "", "", fmt.Errorf("acceptance stage is nil")
	}
	ownerID := ""
	method := ""
	for _, dependencyID := range ref.Block.DependsOn {
		dependency, exists := directCodingSourceBlueprintBlock(stage.Source, dependencyID)
		if !exists || dependency.Role != assemblyline.SourceBlockTaskImplementation {
			continue
		}
		if ownerID != "" {
			return "", "", fmt.Errorf("multiple implementation owners")
		}
		const prefix = "static Map<String, Object> "
		if !strings.HasPrefix(dependency.Signature, prefix) {
			return "", "", fmt.Errorf("implementation signature %q is not a Java result method", dependency.Signature)
		}
		remaining := strings.TrimPrefix(dependency.Signature, prefix)
		separator := strings.IndexByte(remaining, '(')
		if separator <= 0 {
			return "", "", fmt.Errorf("implementation signature %q has no method name", dependency.Signature)
		}
		ownerID, method = dependencyID, strings.TrimSpace(remaining[:separator])
	}
	if ownerID == "" {
		return "", "", fmt.Errorf("no implementation owner")
	}
	for _, document := range stage.Source.Documents {
		for _, block := range document.Blocks {
			if block.ID != ownerID {
				continue
			}
			className, err := javaClassNameFromPreamble(document.Preamble)
			return className, method, err
		}
	}
	return "", "", fmt.Errorf("implementation owner %s has no source document", ownerID)
}

func javaClassNameFromPreamble(preamble string) (string, error) {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(javagrammar.Language())); err != nil {
		return "", err
	}
	source := []byte(strings.TrimSpace(preamble) + "\n}")
	tree := parser.Parse(source, nil)
	if tree == nil {
		return "", fmt.Errorf("Java class preamble parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return "", fmt.Errorf("Java class preamble is not parseable")
	}
	className := ""
	for index := uint(0); index < root.NamedChildCount(); index++ {
		declaration := root.NamedChild(index)
		if declaration == nil || declaration.Kind() != "class_declaration" {
			continue
		}
		name := declaration.ChildByFieldName("name")
		if name == nil || className != "" {
			return "", fmt.Errorf("Java source document requires one named class")
		}
		className = string(source[name.StartByte():name.EndByte()])
	}
	if className == "" {
		return "", fmt.Errorf("Java source document has no class wrapper")
	}
	return className, nil
}
