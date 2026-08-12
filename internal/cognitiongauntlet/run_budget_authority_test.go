package cognitiongauntlet

import "testing"

func TestRawRunBudgetV2IsDerivedAndStructuralV1CannotExecute(t *testing.T) {
	structural := InitialMicrogauntletsV2()[0].Budget
	if structural.Schema != RunBudgetSchemaStructuralV1 || structural.Validate() != nil {
		t.Fatal("initial v1 benchmark budget is not preserved as structural authority")
	}
	generation := mustRatGeneration(t)
	if err := structural.ValidateFor(generation); err == nil {
		t.Fatal("structural v1 benchmark budget was accepted for serious execution")
	}

	executable, err := NewExecutableRunBudgetV2(
		structural, generation.Fixed.Brain.Sampling,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantInputTokens := executable.Station.MaxInputBytes +
		generation.Fixed.Brain.Sampling.InputSpecialTokenReserve
	if executable.Schema != RunBudgetSchemaRawV2 ||
		executable.Station.MaxInputTokens != wantInputTokens ||
		structural.Station.MaxInputTokens == executable.Station.MaxInputTokens {
		t.Fatalf("derived executable budget=%+v structural=%+v", executable, structural)
	}
	if err := executable.ValidateFor(generation); err != nil {
		t.Fatal(err)
	}
}

func TestRawRunBudgetV2RejectsCallerSubstitutionAndSecondDerivation(t *testing.T) {
	structural := InitialMicrogauntletsV2()[0].Budget
	generation := mustRatGeneration(t)
	executable, err := NewExecutableRunBudgetV2(
		structural, generation.Fixed.Brain.Sampling,
	)
	if err != nil {
		t.Fatal(err)
	}
	changed := executable
	changed.Station.MaxInputTokens--
	if err := changed.ValidateFor(generation); err == nil {
		t.Fatal("caller-substituted raw input token ceiling was accepted")
	}
	if _, err := NewExecutableRunBudgetV2(
		executable, generation.Fixed.Brain.Sampling,
	); err == nil {
		t.Fatal("executable budget was silently re-derived as a compatibility path")
	}
}
