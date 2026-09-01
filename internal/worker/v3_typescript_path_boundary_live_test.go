package worker

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

const liveCodingFragmentModelEnv = "OMNIDEX_TEST_CODING_FRAGMENT_MODEL"

func TestLiveTypeScriptFragmentReturnsAnImplementationBody(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv(liveCodingFragmentModelEnv))
	if modelName == "" {
		t.Skip(liveCodingFragmentModelEnv + " is not set")
	}
	baseURL := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_URL"))
	if baseURL == "" {
		t.Fatal("OMNIDEX_TEST_OLLAMA_URL is required")
	}
	contextTokens, err := strconv.Atoi(strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_CONTEXT")))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	ctx := t.Context()
	client := ollama.New(baseURL, modelName, "", llm.MaximumModelRequestDuration)

	fixtures := []struct {
		input    assemblyline.FragmentGenerationInput
		values   map[string]any
		expected string
	}{
		{
			input: assemblyline.FragmentGenerationInput{
				Language:  "typescript",
				Dialect:   "TypeScript",
				Signature: "function FormatCalendarDate(month: number, day: number): string",
				Behavior:  "Return the decimal month, then one inert solidus punctuation character, then the decimal day.",
			},
			values:   map[string]any{"month": float64(3), "day": float64(9)},
			expected: "3" + string(rune(47)) + "9",
		},
		{
			input: assemblyline.FragmentGenerationInput{
				Language:  "typescript",
				Dialect:   "TypeScript",
				Signature: "function PrefixCommand(name: string): string",
				Behavior:  "Return one inert solidus punctuation character immediately followed by the supplied command name.",
			},
			values:   map[string]any{"name": "deploy"},
			expected: string(rune(47)) + "deploy",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.input.Signature, func(t *testing.T) {
			var acceptedResponse string
			execute := func(
				job assemblyline.PortableJob,
				requestedModel string,
				prompt string,
			) (assemblyline.PortableResult, error) {
				if requestedModel != modelName {
					return assemblyline.PortableResult{}, fmt.Errorf(
						"live source-body model %q differs from %q", requestedModel, modelName,
					)
				}
				result, err := executeLivePortablePrompt(
					ctx, client, contextTokens, requestedModel, job, prompt, t,
				)
				return result, err
			}
			runtime := typedWorkerRuntime{
				Context:     ctx,
				MaxAttempts: assemblyline.MaxSourceBodyAttempts,
				Execute: func(
					job assemblyline.PortableJob,
					requestedModel string,
				) (assemblyline.PortableResult, error) {
					prompt, err := assemblyline.RenderPortableJob(job)
					if err != nil {
						return assemblyline.PortableResult{}, err
					}
					return execute(job, requestedModel, prompt)
				},
				Correct: func(
					job assemblyline.PortableJob,
					requestedModel string,
					correction assemblyline.SourceBodyCorrection,
				) (assemblyline.PortableResult, error) {
					prompt, err := correction.ModelInput()
					if err != nil {
						return assemblyline.PortableResult{}, err
					}
					return execute(job, requestedModel, prompt)
				},
				Release: func(assemblyline.PortableJob) error { return nil },
				Finalize: func(
					_ assemblyline.PortableJob,
					result assemblyline.PortableResult,
					validationErr error,
				) error {
					if validationErr == nil {
						acceptedResponse = result.Candidate
					}
					return nil
				},
			}
			source, err := runDirectCodingLanguageFragmentWorker(
				runtime,
				modelName,
				directCodingLanguageGenerationJob{
					Subject: fixture.input.Signature,
					Input:   fixture.input,
					Validate: func(
						_ assemblyline.FragmentGenerationInput,
						body string,
					) (string, error) {
						fragment, err := assemblyline.ParseTypeScriptFunctionBody(
							assemblyline.TypeScriptFunctionContract{
								Signature: fixture.input.Signature,
							},
							body,
						)
						return fragment.Source, err
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := evaluateRestrictedLiveTypeScriptReturn(source, fixture.values)
			if err != nil {
				t.Fatal(err)
			}
			if actual != fixture.expected {
				t.Fatalf("result=%q, want %q", actual, fixture.expected)
			}
			t.Logf("model=%s accepted_ordinary_response=%q", modelName, acceptedResponse)
		})
	}
}

func evaluateRestrictedLiveTypeScriptReturn(source string, values map[string]any) (string, error) {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(typescript.LanguageTypescript())); err != nil {
		return "", err
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		return "", fmt.Errorf("restricted evaluator received no syntax tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() || root.NamedChildCount() != 1 {
		return "", fmt.Errorf("restricted evaluator requires one valid declaration")
	}
	declaration := root.NamedChild(0)
	body := declaration.ChildByFieldName("body")
	if body == nil || body.NamedChildCount() != 1 ||
		body.NamedChild(0).Kind() != "return_statement" ||
		body.NamedChild(0).NamedChildCount() != 1 {
		return "", fmt.Errorf("restricted evaluator requires one direct return expression")
	}
	value, err := evaluateLiveTypeScriptExpression(
		body.NamedChild(0).NamedChild(0), []byte(source), values,
	)
	if err != nil {
		return "", err
	}
	result, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("restricted evaluator result is %T, want string", value)
	}
	return result, nil
}

func evaluateLiveTypeScriptExpression(
	node *treesitter.Node,
	source []byte,
	values map[string]any,
) (any, error) {
	if node == nil {
		return nil, fmt.Errorf("restricted evaluator received an empty expression")
	}
	switch node.Kind() {
	case "identifier":
		value, exists := values[node.Utf8Text(source)]
		if !exists {
			return nil, fmt.Errorf("restricted evaluator received unknown identifier %q", node.Utf8Text(source))
		}
		return value, nil
	case "number":
		return strconv.ParseFloat(node.Utf8Text(source), 64)
	case "parenthesized_expression":
		if node.NamedChildCount() != 1 {
			return nil, fmt.Errorf("restricted evaluator received invalid parentheses")
		}
		return evaluateLiveTypeScriptExpression(node.NamedChild(0), source, values)
	case "binary_expression":
		left := node.ChildByFieldName("left")
		right := node.ChildByFieldName("right")
		if left == nil || right == nil || strings.TrimSpace(
			string(source[left.EndByte():right.StartByte()]),
		) != "+" {
			return nil, fmt.Errorf("restricted evaluator permits only binary addition")
		}
		leftValue, err := evaluateLiveTypeScriptExpression(left, source, values)
		if err != nil {
			return nil, err
		}
		rightValue, err := evaluateLiveTypeScriptExpression(right, source, values)
		if err != nil {
			return nil, err
		}
		if _, stringLeft := leftValue.(string); stringLeft {
			return liveTypeScriptString(leftValue) + liveTypeScriptString(rightValue), nil
		}
		if _, stringRight := rightValue.(string); stringRight {
			return liveTypeScriptString(leftValue) + liveTypeScriptString(rightValue), nil
		}
		leftNumber, leftOK := leftValue.(float64)
		rightNumber, rightOK := rightValue.(float64)
		if !leftOK || !rightOK {
			return nil, fmt.Errorf("restricted evaluator addition received unsupported operands")
		}
		return leftNumber + rightNumber, nil
	case "call_expression":
		callee := node.ChildByFieldName("function")
		arguments := node.ChildByFieldName("arguments")
		if callee == nil || arguments == nil || arguments.NamedChildCount() != 1 {
			return nil, fmt.Errorf("restricted evaluator received unsupported call")
		}
		argument, err := evaluateLiveTypeScriptExpression(arguments.NamedChild(0), source, values)
		if err != nil {
			return nil, err
		}
		switch callee.Utf8Text(source) {
		case "String":
			return liveTypeScriptString(argument), nil
		case "String.fromCharCode":
			code, ok := argument.(float64)
			if !ok || code != float64(rune(code)) || code < 0 || code > 127 {
				return nil, fmt.Errorf("restricted evaluator received invalid character code")
			}
			return string(rune(code)), nil
		default:
			return nil, fmt.Errorf("restricted evaluator received unsupported call %q", callee.Utf8Text(source))
		}
	default:
		return nil, fmt.Errorf("restricted evaluator received unsupported expression %s", node.Kind())
	}
}

func liveTypeScriptString(value any) string {
	if number, ok := value.(float64); ok {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return fmt.Sprint(value)
}

func TestRestrictedLiveTypeScriptReturnEvaluatorRejectsControlFlow(t *testing.T) {
	source := `function PrefixCommand(name: string): string {
  if (false) {
    return String.fromCharCode(47) + name;
  }
  return name;
}`
	if _, err := evaluateRestrictedLiveTypeScriptReturn(
		source, map[string]any{"name": "deploy"},
	); err == nil {
		t.Fatal("control-flow source unexpectedly received an unconditional result")
	}
}
