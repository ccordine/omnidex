package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestLanguageFragmentWorkerRejectsPathBearingEnvelopeBeforeInference(t *testing.T) {
	called := false
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			called = true
			return "", nil
		}),
	}
	_, err := runDirectCodingLanguageFragmentWorker(
		runtime, "fragment-model",
		directCodingLanguageGenerationJob{
			Subject: "opaque-block",
			Input: assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022", Signature: "function feature001(input, dependencies)",
				Behavior: "Persist the model-invented value at src/generated.",
			},
			Validate: validateDirectCodingJavaScriptFragment,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "filesystem identity") || called {
		t.Fatalf("path-bearing fragment err=%v inference_called=%t", err, called)
	}
}

func TestLanguageFragmentWorkerRejectsKnownBareArtifactInCandidate(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{"internal/transport.go"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			return `function runtimeLabel() { return "transport.go"; }`, nil
		}),
	}
	_, err = runDirectCodingLanguageFragmentWorker(
		runtime, "fragment-model", directCodingLanguageGenerationJob{
			Subject: "opaque-block",
			Input: assemblyline.FragmentGenerationInput{
				Language: "javascript", Dialect: "ECMAScript 2022", Signature: "function runtimeLabel()",
				Behavior: "Return one runtime label.",
			},
			Validate: func(_ assemblyline.FragmentGenerationInput, candidate string) (string, error) {
				return candidate, nil
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("known artifact candidate error=%v", err)
	}
}

func TestLanguageFragmentWorkerAcceptsInterpretedControlEscapes(t *testing.T) {
	t.Parallel()
	candidate := `func AppendLineBreak(input string) string {
	return input + "\n"
}`
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			return candidate, nil
		}),
	}
	got, err := runDirectCodingLanguageFragmentWorker(
		runtime, "fragment-model", directCodingLanguageGenerationJob{
			Subject: "opaque-block",
			Input: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24", Signature: "func AppendLineBreak(input string) string",
				Behavior: "Append one line break to a label.",
			},
			Validate: validateDirectCodingGoFragment,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `input + "\n"`) {
		t.Fatalf("validated fragment lost its newline escape: %q", got)
	}
}

func TestJavaScriptFragmentScopeRejectsUndeclaredAndDynamicAuthority(t *testing.T) {
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022", Signature: "function feature001(input, dependencies)",
		Behavior: "Return a result.",
		PermittedSymbols: []string{
			"export function normalizeTaskResult(value)", "const directCapability = 'capability_001';",
		},
	}
	valid := `function feature001(input, dependencies) {
	const values = input.arguments.map((argument) => String(argument));
	const value = values[0] ?? '';
  return normalizeTaskResult({ output: value, state: { dependency: dependencies[directCapability] } });
}`
	if _, err := validateDirectCodingJavaScriptFragment(input, valid); err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]string{
		"undeclared": `function feature001(input, dependencies) { return hiddenRepositoryAPI(input); }`,
		"process":    `function feature001(input, dependencies) { return process.env; }`,
		"constructor": `function feature001(input, dependencies) {
  return input.constructor.constructor('return globalThis')();
}`,
		"computed constructor": `function feature001(input, dependencies) {
  return input["constructor"]["constructor"]('return globalThis')();
}`,
		"assembled constructor": `function feature001(input, dependencies) {
  return input["con" + "structor"];
}`,
		"destructured constructor alias": `function feature001(input, dependencies) {
  const { constructor: HostFunction } = (() => {});
  const host = HostFunction("return process")();
  return { output: host.env.SECRET ?? "", error: "", exitCode: 0, state: {} };
}`,
		"destructured constructor shorthand": `function feature001(input, dependencies) {
  const { constructor } = (() => {});
  return constructor("return process")();
}`,
		"computed destructured constructor": `function feature001(input, dependencies) {
  const { ["con" + "structor"]: HostFunction } = (() => {});
  return HostFunction("return process")();
}`,
		"dynamic import": `function feature001(input, dependencies) {
  return import(input.arguments[0]);
}`,
		"runtime computed key": `function feature001(input, dependencies) {
  const key = input.arguments[0];
  return ({})[key][key]('return process')();
}`,
		"reflection primitive": `function feature001(input, dependencies) {
  return Object.getOwnPropertyDescriptor(Object.getPrototypeOf(() => {}), 'constructor').value('return process')();
}`,
		"nondeterministic promise": `function feature001(input, dependencies) {
  return Promise.resolve(input);
}`,
		"nondeterministic date": `function feature001(input, dependencies) {
  return Date.now();
}`,
		"nondeterministic random": `function feature001(input, dependencies) {
  return Math.random();
}`,
		"host metadata": `function feature001(input, dependencies) {
  return import.meta.url;
}`,
		"nested binding escape": `function feature001(input, dependencies) {
  if (input.arguments.length > 0) {
    const hiddenRepositoryAPI = (value) => value;
  }
  return hiddenRepositoryAPI(input);
}`,
		"permitted shadow": `function feature001(input, dependencies) {
  const normalizeTaskResult = (value) => value;
  return normalizeTaskResult(input);
}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateDirectCodingJavaScriptFragment(input, candidate); err == nil {
				t.Fatal("accepted JavaScript authority outside the exact fragment scope")
			}
		})
	}
	sensitiveConstantInput := input
	sensitiveConstantInput.PermittedSymbols = append(
		append([]string(nil), input.PermittedSymbols...),
		"const hostKey = 'constructor';",
	)
	if _, err := validateDirectCodingJavaScriptFragment(
		sensitiveConstantInput,
		`function feature001(input, dependencies) { return input[hostKey]; }`,
	); err == nil {
		t.Fatal("accepted a code-owned computed key whose exact value is a sensitive host property")
	}
}

func TestJavaScriptFragmentProjectionExposesDeclarationWithoutRuntimeImplementation(t *testing.T) {
	runtime, err := javaScriptCommandLineRuntimeDocument(
		requireDirectCodingVersionProfile(t, javaScriptCommandLineVersionProfileV1),
	)
	if err != nil {
		t.Fatal(err)
	}
	feature := assemblyline.SourceDocument{
		ID: "feature", Path: "feature.mjs", AdapterID: "javascript",
		Blocks: []assemblyline.SourceBlock{{
			ID: "feature.001", Signature: "function feature001(input, dependencies)",
			Contract:  "Return one normalized result.",
			API:       "function feature001(input, dependencies)",
			DependsOn: []string{"runtime.api"}, Capabilities: []string{"runtime.api"},
			TaskID: "task_001", Role: assemblyline.SourceBlockTaskImplementation,
		}},
	}
	stage := directCodingProgram{
		StackID: genericJavaScriptCommandLineAdapter, VersionProfileID: javaScriptCommandLineVersionProfileV1,
		Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
			runtime, feature,
		}}}
	input, err := directCodingLanguageFragmentInput(&stage, assemblyline.SourceBlockRef{
		Document: feature, Block: feature.Blocks[0],
	}, "javascript")
	if err != nil {
		t.Fatal(err)
	}
	visible := strings.Join(input.PermittedSymbols, "\n")
	if visible != "export function normalizeTaskResult(value)" {
		t.Fatalf("JavaScript model-visible runtime authority=%q", visible)
	}
	for _, leakedImplementation := range []string{"throw new", "Number.isInteger", "return {"} {
		if strings.Contains(visible, leakedImplementation) {
			t.Fatalf("JavaScript runtime implementation leaked through API projection: %q", visible)
		}
	}
}

func TestCodeOwnedNodeAndJavaVerificationCommandsAreNarrow(t *testing.T) {
	valid := []struct {
		name string
		args []string
	}{
		{"node", javaScriptNodeTestArgs()},
		{"node", javaScriptNodeCheckArgs("main.mjs")},
		{"cargo", []string{"test", "--locked", "--offline"}},
		{"cargo", []string{"test", "--locked", "--offline", "--test", "feature_test"}},
		{"cargo", []string{"check", "--locked", "--offline", "--all-targets"}},
		{"cargo", []string{"build", "--locked", "--offline"}},
		{"javac", []string{"--release", "21", "-Xlint:all", "-Werror", "-d", "build/classes", "Feature.java", "Main.java"}},
		{"java", []string{"-ea", "-cp", "build/classes", "TestRunner", "FeatureTest001", "verifyFeature001"}},
		{"jar", []string{"--create", "--file", "build/application.jar", "--main-class", "Main", "-C", "build/classes", "."}},
	}
	for _, command := range valid {
		if err := validateV3Command(command.name, command.args); err != nil {
			t.Fatalf("valid %s command rejected: %v", command.name, err)
		}
	}
	for _, command := range []struct {
		name string
		args []string
	}{
		{"node", []string{"--test"}},
		{"node", append(javaScriptNodeTestArgs(), "untrusted.test.mjs")},
		{"cargo", []string{"test", "--release"}},
		{"cargo", []string{"check", "--manifest-path", "other/Cargo.toml"}},
		{"cargo", []string{"test", "--locked", "--offline", "--test", "bad/target"}},
		{"cargo", []string{"fmt", "--check", "--all"}},
		{"javac", []string{"Feature.java"}},
		{"java", []string{"-ea", "-cp", ".", "Arbitrary"}},
		{"jar", []string{"--extract", "--file", "archive.jar"}},
	} {
		if err := validateV3Command(command.name, command.args); err == nil {
			t.Fatalf("accepted broadened command %s %s", command.name, strings.Join(command.args, " "))
		}
	}
}
