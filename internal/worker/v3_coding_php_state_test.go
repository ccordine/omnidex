package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPDurableStateProjectionIsCodeOwnedAndRequestLocalProjectionIsClean(t *testing.T) {
	t.Parallel()
	durable := phpDurableStateProgramFixture(t)
	hasState, err := phpServiceProgramRequiresPostgreSQL(durable)
	if err != nil || !hasState {
		t.Fatalf("durable PHP state=%t error=%v", hasState, err)
	}
	byBlock := make(map[string]assemblyline.SourceBlock)
	for _, document := range durable.Source.Documents {
		for _, block := range document.Blocks {
			byBlock[block.ID] = block
		}
	}
	stateBlock := byBlock["feature.state.001"]
	featureBlock := byBlock["feature.001"]
	if stateBlock.Static == "" || stateBlock.API == "" ||
		!strings.Contains(stateBlock.Static, phpSingleQuoted(directCodingServiceStateDefaultKey)) ||
		!strings.Contains(stateBlock.API, "public static function load(): array") ||
		!stringSliceContains(featureBlock.Capabilities, stateBlock.ID) ||
		stringSliceContains(featureBlock.Capabilities, "runtime.state") {
		t.Fatalf("durable PHP facade=%+v feature capabilities=%v", stateBlock, featureBlock.Capabilities)
	}
	for _, required := range []string{
		phpServiceStateMigrationPath, phpServiceStateMigrationRunner,
		phpServiceStateVerificationPath, phpServiceStateVerificationEnv,
		phpServiceStateDeploymentEnv,
	} {
		if strings.TrimSpace(phpServiceFileContent(t, durable.StaticFiles, required)) == "" {
			t.Fatalf("durable PHP projection omitted %s", required)
		}
	}
	compose := phpServiceFileContent(t, durable.StaticFiles, "docker-compose.yml")
	for _, required := range []string{
		"image: " + phpServicePostgresImage, "condition: service_healthy",
		"${SERVICE_STATE_DB_PASSWORD}", "service_state_data:/var/lib/postgresql/data",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("durable PHP Compose omits %s:\n%s", required, compose)
		}
	}

	requestLocal, _ := phpAcceptanceFixture(t)
	if hasState, err := phpServiceProgramRequiresPostgreSQL(requestLocal); err != nil || hasState {
		t.Fatalf("request-local PHP state=%t error=%v", hasState, err)
	}
	for _, file := range requestLocal.StaticFiles {
		if file.Path == phpServiceStateMigrationPath || file.Path == phpServiceStateMigrationRunner ||
			file.Path == phpServiceStateVerificationPath || file.Path == phpServiceStateVerificationEnv ||
			file.Path == phpServiceStateDeploymentEnv {
			t.Fatalf("request-local PHP projection retained %s", file.Path)
		}
	}
	if strings.Contains(phpServiceFileContent(t, requestLocal.StaticFiles, "docker-compose.yml"), "postgres:") ||
		strings.Contains(phpServiceFileContent(t, requestLocal.StaticFiles, "Dockerfile"), "pdo_pgsql") {
		t.Fatal("request-local PHP projection retained unused PostgreSQL mechanics")
	}
}

func TestPHPDurableStateVerificationStartsAndMigratesBeforeAnyAppProcess(t *testing.T) {
	t.Parallel()
	program := phpDurableStateProgramFixture(t)
	commands, err := phpServiceVerificationCommands(program)
	if err != nil {
		t.Fatal(err)
	}
	up, migration, write, read := -1, -1, -1, -1
	testRunner, httpVerifier := -1, -1
	resets := make([]int, 0, 3)
	for index, command := range commands {
		joined := strings.Join(command.Args, " ")
		switch joined {
		case strings.Join(phpServiceComposeArgs(true, "up", "--detach", "--wait", "postgres"), " "):
			up = index
		case strings.Join(phpServiceComposeArgs(true, "run", "--rm", "--no-deps", "app", "php", phpServiceStateMigrationRunner), " "):
			migration = index
		case strings.Join(phpServiceComposeArgs(true, "run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, "write"), " "):
			write = index
		case strings.Join(phpServiceComposeArgs(true, "run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, "read"), " "):
			read = index
		case strings.Join(phpServiceComposeArgs(true, "run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, "reset"), " "):
			resets = append(resets, index)
		case strings.Join(phpServiceComposeArgs(true, "run", "--rm", "--no-deps", "app", "php", "tests/TestRunner.php"), " "):
			testRunner = index
		case strings.Join(phpServiceComposeArgs(true, "run", "--rm", "--no-deps", "app", "php", "tests/HttpVerifier.php"), " "):
			httpVerifier = index
		}
		if err := validateV3Command(command.Name, command.Args); err != nil {
			t.Fatalf("stateful command %q escaped the exact allowlist: %v", joined, err)
		}
		if strings.Contains(joined, " run --rm --no-deps app ") && (up < 0 || up >= index) {
			t.Fatalf("app process ran before PostgreSQL was healthy: %v", commands)
		}
	}
	if up < 0 || migration <= up || write <= migration || read <= write {
		t.Fatalf("state verification order up=%d migration=%d write=%d read=%d", up, migration, write, read)
	}
	if len(resets) != 3 || !(read < resets[0] && resets[0] < testRunner &&
		testRunner < resets[1] && resets[1] < httpVerifier && httpVerifier < resets[2]) {
		t.Fatalf(
			"application state isolation resets=%v test_runner=%d http_verifier=%d commands=%v",
			resets, testRunner, httpVerifier, commands,
		)
	}
}

func TestPHPDurableStateRunnerResetsBetweenEveryGeneratedVerifier(t *testing.T) {
	t.Parallel()
	storage := directCodingServiceStoragePlan{
		WorkloadSHA256: "fixture", Namespace: "workload:fixture",
		ByTask: map[string]directCodingServiceStorageKind{
			"task_001": directCodingServiceStoragePostgreSQL,
			"task_002": directCodingServiceStoragePostgreSQL,
		},
	}
	source := phpServiceTestRunnerSource([]phpServiceFeatureBinding{
		{VerificationName: "verifyInventoryFeature", TaskID: "task_001"},
		{VerificationName: "verifyAppointmentFeature", TaskID: "task_002"},
	}, storage)
	call := phpServiceStateResetFunctionName + "();"
	if strings.Count(source, call) != 4 {
		t.Fatalf("multi-feature runner reset count=%d source:\n%s", strings.Count(source, call), source)
	}
	first := strings.Index(source, "verifyInventoryFeature();")
	second := strings.Index(source, "verifyAppointmentFeature();")
	firstBefore := strings.LastIndex(source[:first], call)
	firstAfter := strings.Index(source[first+1:], call) + first + 1
	secondBefore := strings.LastIndex(source[:second], call)
	secondAfter := strings.Index(source[second+1:], call) + second + 1
	if firstBefore < 0 || firstAfter <= first || secondBefore <= firstAfter ||
		secondBefore >= second || secondAfter <= second ||
		!strings.Contains(source, "verifyInventoryFeature state isolation") ||
		!strings.Contains(source, "verifyAppointmentFeature state isolation") {
		t.Fatalf("multi-feature runner did not isolate and aggregate each verifier:\n%s", source)
	}
}

func TestPHPDurableFocusedVerificationResetsAroundAcceptance(t *testing.T) {
	t.Parallel()
	program := phpDurableStateProgramFixture(t)
	specification, _, _, _, _ := phpServiceStackFixture(t)
	context, err := assemblyline.ProjectApplicationTaskContext(
		applicationWorkloadInput(specification), program.Workload, program.Workload.Tasks[0].ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := phpServiceTaskVerificationCommands(context, program)
	if err != nil {
		t.Fatal(err)
	}
	resets := make([]int, 0, 2)
	acceptance := -1
	for index, command := range commands {
		joined := strings.Join(command.Args, " ")
		if strings.HasSuffix(joined, " "+phpServiceStateVerificationPath+" reset") {
			resets = append(resets, index)
		}
		if strings.HasSuffix(joined, " tests/Feature101Test.php") {
			acceptance = index
		}
	}
	if len(resets) != 2 || acceptance < 0 || !(resets[0] < acceptance && acceptance < resets[1]) {
		t.Fatalf("focused state isolation resets=%v acceptance=%d commands=%v", resets, acceptance, commands)
	}
}

func TestPHPDurableStateValidationFailsLoudly(t *testing.T) {
	t.Parallel()
	program := phpDurableStateProgramFixture(t)
	program.StaticFiles = nil
	if _, err := phpServiceProgramRequiresPostgreSQL(program); err == nil ||
		!strings.Contains(err.Error(), "lacks its code-owned PostgreSQL artifacts") {
		t.Fatalf("durable program without storage artifacts error=%v", err)
	}
	valid := phpStateAssemblyValidationFixture()
	if err := validatePHPServiceStateAssembly(valid); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(map[string]string)
		want string
	}{
		{name: "partial artifact set", edit: func(files map[string]string) {
			delete(files, phpServiceStateMigrationRunner)
		}, want: "complete artifact set"},
		{name: "tampered schema", edit: func(files map[string]string) {
			files[phpServiceStateMigrationPath] += "-- changed\n"
		}, want: "code-owned schema"},
		{name: "credential in deployment", edit: func(files map[string]string) {
			files["docker-compose.yml"] += phpServiceStateVerificationSecret
		}, want: "verification credential"},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := make(map[string]string, len(valid))
			for path, source := range valid {
				files[path] = source
			}
			test.edit(files)
			err := validatePHPServiceStateAssembly(files)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("state assembly error=%v want=%q", err, test.want)
			}
		})
	}
}

func TestPostgreSQLMigrationAdapterRejectsUnsafeOrNonTransactionalSource(t *testing.T) {
	t.Parallel()
	if err := validatePostgreSQLMigrationArtifactSource(
		phpServiceStateMigrationPath, []byte(phpServiceStateMigrationSQL()),
	); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"CREATE TABLE IF NOT EXISTS values_table (id BIGINT);",
		"BEGIN; CREATE TABLE IF NOT EXISTS values_table (id BIGINT); DROP TABLE values_table; COMMIT;",
		"BEGIN; CREATE TABLE IF NOT EXISTS values_table (value TEXT DEFAULT 'unterminated); COMMIT;",
	} {
		if err := validatePostgreSQLMigrationArtifactSource(
			phpServiceStateMigrationPath, []byte(source),
		); err == nil {
			t.Fatalf("PostgreSQL migration adapter accepted unsafe source %q", source)
		}
	}
}

func phpDurableStateProgramFixture(t *testing.T) directCodingProgram {
	t.Helper()
	specification, workload, target, coverage, endpoints := phpServiceStackFixture(t)
	state := testRequestLocalServiceStatePlan(workload)
	state.ByTask[workload.Tasks[0].ID] =
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	capabilities := directCodingCapabilityGraph{"requirement_001": nil}
	state = bindTestServiceStateInterfaces(
		t, workload, capabilities, state, testIntegerServiceStateField("count"),
	)
	blueprint, staticFiles, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		capabilities, target, coverage, state, endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	return directCodingProgram{
		StackID: genericPHPServiceAdapter, VersionProfileID: phpServiceVersionProfileV1,
		Workload: workload, Coverage: coverage, ServiceState: state,
		ServiceEndpoints: endpoints, Source: blueprint, StaticFiles: staticFiles,
	}
}

func phpStateAssemblyValidationFixture() map[string]string {
	return map[string]string{
		phpServiceStateMigrationPath:    phpServiceStateMigrationSQL(),
		phpServiceStateMigrationRunner:  phpServiceStateMigrationRunnerSource(),
		phpServiceStateVerificationPath: phpServiceStateVerifierSource("workload:fixture"),
		phpServiceStateVerificationEnv:  phpServiceStateVerificationEnvironment(),
		phpServiceStateDeploymentEnv:    phpServiceStateDeploymentEnvironment(),
		"Dockerfile": "docker-php-ext-install -j1 pdo_pgsql\nCOPY " +
			phpServiceStateMigrationPath + " ./x\nCOPY " + phpServiceStateMigrationRunner +
			" ./x\nCOPY " + phpServiceStateVerificationPath + " ./x\n",
		"docker-compose.yml": "postgres:\ncondition: service_healthy\n" +
			"${SERVICE_STATE_DB_PASSWORD}\nDATABASE_URL:\n" +
			"service_state_data:/var/lib/postgresql/data\n",
		"src/Runtime.php": phpServiceStateRuntimeSource(),
	}
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
