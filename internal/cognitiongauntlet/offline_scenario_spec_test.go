package cognitiongauntlet

import "testing"

func offlineScenarioTestBudget() RunBudget { return InitialMicrogauntletsV2()[0].Budget }

func offlineExecutableScenarioTestBudget(t *testing.T) RunBudget {
	t.Helper()
	generation := mustRatGeneration(t)
	budget, err := NewExecutableRunBudgetV2(
		offlineScenarioTestBudget(), generation.Fixed.Brain.Sampling,
	)
	if err != nil {
		t.Fatal(err)
	}
	return budget
}

func mustOfflineScenarioSpec(t *testing.T, suite Suite, seed uint64) OfflineScenarioSpec {
	t.Helper()
	budget := offlineExecutableScenarioTestBudget(t)
	spec, err := ResolveOfflineScenarioSpecV1(suite, seed, budget)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestResolveOfflineScenarioSpecUsesOneStrictInitialOrExtendedBranch(t *testing.T) {
	budget := offlineScenarioTestBudget()
	for _, suite := range []Suite{
		SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined,
		SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder,
	} {
		spec, err := ResolveOfflineScenarioSpecV1(suite, 91_001+uint64(len(suite)), budget)
		if err != nil {
			t.Fatalf("resolve %s: %v", suite, err)
		}
		if err := spec.Validate(); err != nil {
			t.Fatalf("validate %s: %v", suite, err)
		}
		if spec.Suite() != suite || spec.Budget() != budget {
			t.Fatalf("%s authority changed", suite)
		}
		initial := spec.Initial != nil
		extended := spec.Extended != nil
		if initial == extended {
			t.Fatalf("%s did not select exactly one scenario branch", suite)
		}
	}
}

func TestOfflineScenarioSpecRejectsMixedAndDedicatedExecutionAuthorities(t *testing.T) {
	initial, err := ResolveOfflineScenarioSpecV1(SuiteRetrieve, 92_001, offlineScenarioTestBudget())
	if err != nil {
		t.Fatal(err)
	}
	extended, err := ResolveOfflineScenarioSpecV1(SuiteTraverse, 92_002, offlineScenarioTestBudget())
	if err != nil {
		t.Fatal(err)
	}
	mixed := initial
	mixed.Extended = extended.Extended
	if err := mixed.Validate(); err == nil {
		t.Fatal("mixed initial/extended scenario authority was accepted")
	}
	for _, suite := range []Suite{SuiteResume, SuiteScale, SuiteTransfer, SuiteRogue} {
		if _, err := ResolveOfflineScenarioSpecV1(suite, 92_100, offlineScenarioTestBudget()); err == nil {
			t.Fatalf("dedicated suite %s entered ordinary scenario execution", suite)
		}
	}
}

func TestOfflineScenarioSpecPropagatesInitialValidationWithoutExtendedFallback(t *testing.T) {
	invalid := offlineScenarioTestBudget()
	invalid.EnvironmentActions = 0
	_, err := ResolveOfflineScenarioSpecV1(SuiteRetrieve, 92_200, invalid)
	if err == nil {
		t.Fatal("invalid initial scenario was accepted")
	}
	if got := err.Error(); got == "" || got == "extended cognition suite \"retrieve\" is not registered" {
		t.Fatalf("initial validation was replaced by an extended fallback: %v", err)
	}
}

func TestOfflineScenarioSpecGeneratesBothAuthoritiesWithoutFallback(t *testing.T) {
	for _, suite := range []Suite{SuiteRetrieve, SuiteTraverse} {
		spec, err := ResolveOfflineScenarioSpecV1(suite, 93_000+uint64(len(suite)), offlineScenarioTestBudget())
		if err != nil {
			t.Fatal(err)
		}
		generated, err := generateOfflineScenario(spec)
		if err != nil {
			t.Fatalf("generate %s: %v", suite, err)
		}
		if generated.scenario.Ref() != generated.public.Scenario || generated.suite != suite ||
			generated.oracleSHA256 == "" || generated.taskArchetype == "" {
			t.Fatalf("%s generated authority is incomplete", suite)
		}
		if (generated.initial != nil) == (generated.extended != nil) {
			t.Fatalf("%s generation did not preserve its exact branch", suite)
		}
	}
}
