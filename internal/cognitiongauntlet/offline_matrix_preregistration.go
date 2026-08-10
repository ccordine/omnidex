package cognitiongauntlet

import (
	"fmt"
	"reflect"
)

const (
	OfflineMatrixPreregistrationSchemaV1 = "omnidex.offline-cognition-matrix-preregistration.v1"
	matrixOrderingV1                     = "case-major-variant-minor.v1"
	matrixStatisticalTestV1              = "exact-paired-binomial-mcnemar.v1"
	matrixAlphaPPM                       = 50_000
	matrixMinimumEffectBasisPoints       = 1
	matrixContextReductionBasisPoints    = 4_500
	matrixMaximumSuccessLossBasisPoints  = 200
)

type OfflineMatrixPlan struct {
	Policy      CompetencePolicy `json:"policy"`
	Suites      []Suite          `json:"suites"`
	Seeds       []uint64         `json:"seeds"`
	Repetitions int              `json:"repetitions"`
	Surface     Surface          `json:"surface"`
}

type OfflineMatrixCase struct {
	ID         string `json:"id"`
	Suite      Suite  `json:"suite"`
	Seed       uint64 `json:"seed"`
	Repetition int    `json:"repetition"`
}

type OfflineMatrixPreregistration struct {
	Schema                        string              `json:"schema"`
	Hypothesis                    string              `json:"hypothesis"`
	Policy                        CompetencePolicy    `json:"policy"`
	Suites                        []Suite             `json:"suites"`
	Seeds                         []uint64            `json:"seeds"`
	Repetitions                   int                 `json:"repetitions"`
	Surface                       Surface             `json:"surface"`
	Variants                      []Variant           `json:"variants"`
	ContaminatedVariants          []Variant           `json:"contaminated_variants"`
	BenchmarkOnlyVariants         []Variant           `json:"benchmark_only_variants"`
	Cases                         []OfflineMatrixCase `json:"cases"`
	SampleCount                   int                 `json:"sample_count"`
	RunCount                      int                 `json:"run_count"`
	Ordering                      string              `json:"ordering"`
	StatisticalTest               string              `json:"statistical_test"`
	AlphaPPM                      int                 `json:"alpha_ppm"`
	MinimumEffectBasisPoints      int                 `json:"minimum_effect_basis_points"`
	ContextReductionBasisPoints   int                 `json:"context_reduction_basis_points"`
	MaximumSuccessLossBasisPoints int                 `json:"maximum_success_loss_basis_points"`
}

func NewOfflineMatrixPreregistration(
	plan OfflineMatrixPlan,
) (OfflineMatrixPreregistration, error) {
	if err := plan.Validate(); err != nil {
		return OfflineMatrixPreregistration{}, err
	}
	registration := buildOfflineMatrixPreregistration(plan)
	return registration, registration.Validate()
}

func (plan OfflineMatrixPlan) Validate() error {
	if plan.Policy != CompetenceSuccessSuperiority &&
		plan.Policy != CompetenceEfficiencySuperiority {
		return fmt.Errorf("offline matrix competence policy is unregistered")
	}
	if plan.Surface != SurfaceFilesystem {
		return fmt.Errorf("complete offline matrix requires the shared filesystem surface")
	}
	if plan.Suites == nil || len(plan.Suites) == 0 || len(plan.Suites) > 5 ||
		plan.Seeds == nil || len(plan.Seeds) == 0 || len(plan.Seeds) > 256 ||
		plan.Repetitions <= 0 || plan.Repetitions > 10 {
		return fmt.Errorf("offline matrix coordinates are outside registered bounds")
	}
	for index, suite := range plan.Suites {
		if _, err := initialMicrogauntletSpec(suite); err != nil {
			return err
		}
		if index > 0 && initialMatrixSuiteRank(plan.Suites[index-1]) >= initialMatrixSuiteRank(suite) {
			return fmt.Errorf("offline matrix suites must be unique and sorted")
		}
	}
	for index, seed := range plan.Seeds {
		if seed == 0 || (index > 0 && plan.Seeds[index-1] >= seed) {
			return fmt.Errorf("offline matrix seeds must be positive, unique, and sorted")
		}
	}
	if len(plan.Suites)*len(plan.Seeds)*plan.Repetitions > 256 {
		return fmt.Errorf("offline matrix sample count exceeds 256")
	}
	return nil
}

func initialMatrixSuiteRank(suite Suite) int {
	switch suite {
	case SuiteRetrieve:
		return 1
	case SuiteRecall:
		return 2
	case SuiteUnlock:
		return 3
	case SuiteMutate:
		return 4
	case SuiteCombined:
		return 5
	default:
		return 0
	}
}

func (registration OfflineMatrixPreregistration) Validate() error {
	plan := OfflineMatrixPlan{
		Policy: registration.Policy, Suites: cloneMatrixSlice(registration.Suites),
		Seeds: cloneMatrixSlice(registration.Seeds), Repetitions: registration.Repetitions,
		Surface: registration.Surface,
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	expected := buildOfflineMatrixPreregistration(plan)
	if !reflect.DeepEqual(registration, expected) {
		return fmt.Errorf("offline matrix preregistration differs from code-owned policy")
	}
	return nil
}

func (registration OfflineMatrixPreregistration) SHA256() (string, error) {
	if err := registration.Validate(); err != nil {
		return "", err
	}
	return digestJSON(registration)
}

func buildOfflineMatrixPreregistration(plan OfflineMatrixPlan) OfflineMatrixPreregistration {
	variants := offlineMatrixVariantOrder()
	cases := make([]OfflineMatrixCase, 0, len(plan.Suites)*len(plan.Seeds)*plan.Repetitions)
	for _, suite := range plan.Suites {
		for _, seed := range plan.Seeds {
			for repetition := 1; repetition <= plan.Repetitions; repetition++ {
				cases = append(cases, OfflineMatrixCase{
					ID:    fmt.Sprintf("%s-%d-r%d", suite, seed, repetition),
					Suite: suite, Seed: seed, Repetition: repetition,
				})
			}
		}
	}
	hypothesis := "Full cognition has positive paired success lift over raw observation, with rescues exceeding regressions and no validity loss."
	if plan.Policy == CompetenceEfficiencySuperiority {
		hypothesis = "Full cognition preserves success within two percentage points while reducing median context by at least forty-five percent, duplicate acquisitions, and tool operations."
	}
	return OfflineMatrixPreregistration{
		Schema: OfflineMatrixPreregistrationSchemaV1, Hypothesis: hypothesis,
		Policy: plan.Policy, Suites: cloneMatrixSlice(plan.Suites), Seeds: cloneMatrixSlice(plan.Seeds),
		Repetitions: plan.Repetitions, Surface: plan.Surface, Variants: variants,
		ContaminatedVariants:  []Variant{VariantOracleEvidence},
		BenchmarkOnlyVariants: []Variant{VariantRawShell}, Cases: cases,
		SampleCount: len(cases), RunCount: len(cases) * len(variants), Ordering: matrixOrderingV1,
		StatisticalTest: matrixStatisticalTestV1, AlphaPPM: matrixAlphaPPM,
		MinimumEffectBasisPoints:      matrixMinimumEffectBasisPoints,
		ContextReductionBasisPoints:   matrixContextReductionBasisPoints,
		MaximumSuccessLossBasisPoints: matrixMaximumSuccessLossBasisPoints,
	}
}

func cloneMatrixSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append([]T{}, values...)
}

func offlineMatrixVariantOrder() []Variant {
	return []Variant{
		VariantRawObservation, VariantFullTranscript, VariantTranscriptCompacted,
		VariantTaskLedger, VariantLedgerWorkingSet, VariantLedgerProjection,
		VariantFullCognition, VariantRawShell, VariantOracleEvidence,
	}
}
