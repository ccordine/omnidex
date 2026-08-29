package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPFragmentScopeAcceptsOnlyTypedLocalAuthority(t *testing.T) {
	input := assemblyline.FragmentGenerationInput{
		Language:         "php",
		Dialect:          "PHP >=8.2,<9",
		Signature:        "function feature101(TaskInput $input, array $dependencies): TaskResult",
		Behavior:         "Return a typed result.",
		PermittedSymbols: []string{phpServiceRuntimeAPI()},
	}
	valid := `function feature101(TaskInput $input, array $dependencies): TaskResult {
    $count = count($dependencies);
    $output = trim($input->route);
    return TaskResult::success($output, ['count' => $count]);
}`
	if _, err := validateDirectCodingPHPFragment(input, valid); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"server global": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success($_SERVER['REQUEST_URI']);
}`,
		"filesystem": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success(file_get_contents('/tmp/value'));
}`,
		"network": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    $socket = fsockopen('127.0.0.1', 80); return TaskResult::success('connected');
}`,
		"process": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success(shell_exec('id'));
}`,
		"dynamic evaluation": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success(eval('return "value";'));
}`,
		"source inclusion": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    include '/tmp/value.php'; return TaskResult::success('included');
}`,
		"environment function": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success((string) getenv('PATH'));
}`,
		"dynamic call": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    $handler = $dependencies['handler']; return TaskResult::success($handler());
}`,
		"process class": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    $process = new PDO('sqlite::memory:'); return TaskResult::success('ready');
}`,
		"code-owned class construction": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return new TaskResult('ready', '', []);
}`,
		"environment constant": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success(PHP_OS);
}`,
		"magic file constant": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success(__FILE__);
}`,
		"reference alias": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    $route = $input->route; $alias =& $route; return TaskResult::success($alias);
}`,
		"undeclared variable": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success($missing);
}`,
		"hidden catch": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    try { return TaskResult::success($input->route); } catch (Throwable $failure) { return TaskResult::success(''); }
}`,
		"heredoc path": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    $value = <<<TXT
/etc/passwd
TXT;
    return TaskResult::success($value);
}`,
		"nowdoc path": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    $value = <<<'TXT'
/etc/passwd
TXT;
    return TaskResult::success($value);
}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateDirectCodingPHPFragment(input, source); err == nil {
				t.Fatalf("accepted forbidden PHP authority:\n%s", source)
			}
		})
	}
}

func TestPHPAcceptanceRequiresExactFeatureResultAndDerivedAssertion(t *testing.T) {
	stage, ref := phpAcceptanceFixture(t)
	valid := `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === '/', 'route output mismatch');
    RuntimeAssertions::require($result, $result->error === '', 'unexpected feature failure');
}`
	if err := validateDirectCodingPHPAcceptance(&stage, ref, valid); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"constant assertion": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, true, 'does not inspect result');
}`,
		"self comparison": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result === $result, 'tautology');
}`,
		"field self comparison": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === $result->output, 'tautology');
}`,
		"output field alias": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    $expectedOutput = $result->output;
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === $expectedOutput, 'aliased output');
    RuntimeAssertions::require($result, $result->error === '', 'second criterion');
}`,
		"error field alias": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    $expectedError = $result->error;
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === '/', 'first criterion');
    RuntimeAssertions::require($result, $result->error === $expectedError, 'aliased error');
}`,
		"wrapped field alias": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    $expectedOutput = $result->output;
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === trim($expectedOutput), 'wrapped alias');
    RuntimeAssertions::require($result, $result->error === '', 'second criterion');
}`,
		"boolean escape": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === 'x' || true, 'boolean escape');
}`,
		"bitwise cast truth forcing": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, (bool) (($result->output === 'impossible') | (strlen('x') === 1)), 'bitwise cast escape');
    RuntimeAssertions::require($result, $result->error === '', 'second criterion');
}`,
		"bare bitwise truth forcing": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, ($result->output === 'impossible') | (strlen('x') === 1), 'bitwise escape');
    RuntimeAssertions::require($result, $result->error === '', 'second criterion');
}`,
		"logical conjunction": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output !== '' && $result->error === '', 'combined criteria');
}`,
		"conditional escape": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output !== '' ? $result->error === '' : $result->error !== '', 'conditional escape');
}`,
		"duplicate conditions": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->error === '', 'first duplicate');
    RuntimeAssertions::require($result, ($result->error === ''), 'second duplicate');
}`,
		"detached assertion": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    $other = TaskResult::success('detached');
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($other, $other->output === 'detached', 'detached result');
    RuntimeAssertions::require($result, $result->error === '', 'attached result');
}`,
		"wrong fixture": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture999(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output !== '', 'wrong fixture');
    RuntimeAssertions::require($result, $result->error === '', 'wrong fixture');
}`,
		"dead branch assertions": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    if (false) {
        RuntimeAssertions::requireResult($result);
        RuntimeAssertions::require($result, $result->output !== '', 'dead output assertion');
        RuntimeAssertions::require($result, $result->error === '', 'dead error assertion');
    }
}`,
		"loop assertions": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    for ($index = 0; $index < 1; $index++) {
        RuntimeAssertions::requireResult($result);
        RuntimeAssertions::require($result, $result->output !== '', 'loop output assertion');
        RuntimeAssertions::require($result, $result->error === '', 'loop error assertion');
    }
}`,
		"closure assertions": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    $verify = function () use ($result): void {
        RuntimeAssertions::requireResult($result);
        RuntimeAssertions::require($result, $result->output !== '', 'closure output assertion');
        RuntimeAssertions::require($result, $result->error === '', 'closure error assertion');
    };
    $verify();
}`,
		"overwritten result": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    $result = TaskResult::success('replacement');
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === 'replacement', 'not feature output');
}`,
		"unbound call": `function verifyFeature101(): void {
    RuntimeAssertions::requireResult(feature101(taskInputFixture101(), []));
    $result = TaskResult::success('ready');
    RuntimeAssertions::require($result, $result->output === 'ready', 'unrelated result');
}`,
		"wrong feature": `function verifyFeature101(): void {
    $result = feature102(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output !== '', 'wrong feature');
}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDirectCodingPHPAcceptance(&stage, ref, source); err == nil {
				t.Fatalf("accepted ungrounded PHP verification:\n%s", source)
			}
		})
	}
}

func phpAcceptanceFixture(t *testing.T) (directCodingProgram, assemblyline.SourceBlockRef) {
	t.Helper()
	specification, workload, target, coverage, endpoints := phpServiceStackFixture(t)
	blueprint, staticFiles, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
		testRequestLocalServiceStatePlan(workload), endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	program := directCodingProgram{
		StackID: genericPHPServiceAdapter, VersionProfileID: phpServiceVersionProfileV1,
		Workload: workload, Coverage: coverage,
		ServiceState:     testRequestLocalServiceStatePlan(workload),
		ServiceEndpoints: endpoints, Source: blueprint, StaticFiles: staticFiles,
		Generated: map[string]string{
			"feature.001": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success($input->route);
}`,
			"representation.html.001": phpServiceHTMLRendererFixture(),
		},
	}
	for documentIndex, document := range blueprint.Documents {
		for blockIndex, block := range document.Blocks {
			if block.ID == "acceptance.001" {
				return program, assemblyline.SourceBlockRef{
					DocumentIndex: documentIndex, BlockIndex: blockIndex,
					Document: document, Block: block,
				}
			}
		}
	}
	t.Fatal("PHP acceptance fixture lacks its generated verifier")
	return directCodingProgram{}, assemblyline.SourceBlockRef{}
}
