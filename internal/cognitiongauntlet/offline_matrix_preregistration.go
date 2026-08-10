package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"time"
)

const (
	OfflineMatrixPreregistrationSchemaV2 = "omnidex.offline-cognition-matrix-preregistration.v2"
	matrixOrderingV2                     = "blind-variant-major-case-minor.evaluate-after-all-inference.v2"
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

type OfflineMatrixFixedAuthority struct {
	Budget                  RunBudget          `json:"budget"`
	RatGeneration           RatGeneration      `json:"rat_generation"`
	RuntimeFingerprint      RuntimeFingerprint `json:"runtime_fingerprint"`
	InferenceTimeoutSeconds int                `json:"inference_timeout_seconds"`
	OmnidexCommit           string             `json:"omnidex_commit"`
	LedgerSchemaVersion     string             `json:"ledger_schema_version"`
	WorkingSetPolicyVersion string             `json:"working_set_policy_version"`
	ProjectionPolicyVersion string             `json:"projection_policy_version"`
}

type OfflineMatrixPreregistration struct {
	Schema                        string                      `json:"schema"`
	Hypothesis                    string                      `json:"hypothesis"`
	Policy                        CompetencePolicy            `json:"policy"`
	Suites                        []Suite                     `json:"suites"`
	Seeds                         []uint64                    `json:"seeds"`
	Repetitions                   int                         `json:"repetitions"`
	Surface                       Surface                     `json:"surface"`
	Variants                      []Variant                   `json:"variants"`
	TournamentSeed                Variant                     `json:"tournament_seed"`
	TournamentRounds              []OfflineMatrixRound        `json:"tournament_rounds"`
	TournamentSelectionPolicy     string                      `json:"tournament_selection_policy"`
	DiagnosticVariants            []Variant                   `json:"diagnostic_variants"`
	ContaminatedVariants          []Variant                   `json:"contaminated_variants"`
	BenchmarkOnlyVariants         []Variant                   `json:"benchmark_only_variants"`
	Cases                         []OfflineMatrixCase         `json:"cases"`
	SampleCount                   int                         `json:"sample_count"`
	RunCount                      int                         `json:"run_count"`
	Ordering                      string                      `json:"ordering"`
	StatisticalTest               string                      `json:"statistical_test"`
	AlphaPPM                      int                         `json:"alpha_ppm"`
	MinimumEffectBasisPoints      int                         `json:"minimum_effect_basis_points"`
	ContextReductionBasisPoints   int                         `json:"context_reduction_basis_points"`
	MaximumSuccessLossBasisPoints int                         `json:"maximum_success_loss_basis_points"`
	Fixed                         OfflineMatrixFixedAuthority `json:"fixed"`
	RegisteredAt                  time.Time                   `json:"registered_at"`
}

func NewOfflineMatrixPreregistration(
	plan OfflineMatrixPlan,
	fixed OfflineMatrixFixedAuthority,
) (OfflineMatrixPreregistration, error) {
	if err := plan.Validate(); err != nil {
		return OfflineMatrixPreregistration{}, err
	}
	if err := fixed.Validate(); err != nil {
		return OfflineMatrixPreregistration{}, err
	}
	registration := buildOfflineMatrixPreregistration(plan, fixed, time.Now().UTC())
	return registration, registration.Validate()
}

func (fixed OfflineMatrixFixedAuthority) Validate() error {
	if err := fixed.Budget.ValidateFor(fixed.RatGeneration); err != nil {
		return err
	}
	if err := fixed.RuntimeFingerprint.Validate(); err != nil {
		return err
	}
	if fixed.RuntimeFingerprint.ProductionSourceSHA256 != fixed.RatGeneration.Runtime.SourceSHA256 ||
		fixed.InferenceTimeoutSeconds <= 0 || fixed.InferenceTimeoutSeconds > 24*60*60 ||
		!validCommitIdentity(fixed.OmnidexCommit) {
		return fmt.Errorf("offline matrix fixed execution authority is invalid")
	}
	for label, value := range map[string]string{
		"Task Ledger schema version":        fixed.LedgerSchemaVersion,
		"Working Set policy version":        fixed.WorkingSetPolicyVersion,
		"Context Projection policy version": fixed.ProjectionPolicyVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	return nil
}

func (plan OfflineMatrixPlan) Validate() error {
	if plan.Policy != CompetenceSuccessSuperiority &&
		plan.Policy != CompetenceEfficiencySuperiority {
		return fmt.Errorf("offline matrix competence policy is unregistered")
	}
	if plan.Surface != SurfaceFilesystem {
		return fmt.Errorf("complete offline matrix requires the shared filesystem surface")
	}
	if plan.Suites == nil || len(plan.Suites) == 0 || len(plan.Suites) > 9 ||
		plan.Seeds == nil || len(plan.Seeds) == 0 || len(plan.Seeds) > 256 ||
		plan.Repetitions <= 0 || plan.Repetitions > 10 {
		return fmt.Errorf("offline matrix coordinates are outside registered bounds")
	}
	for index, suite := range plan.Suites {
		if offlineScenarioSuiteRank(suite) == 0 {
			return fmt.Errorf("offline matrix suite %q requires another execution rail", suite)
		}
		if index > 0 && offlineScenarioSuiteRank(plan.Suites[index-1]) >= offlineScenarioSuiteRank(suite) {
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

func offlineScenarioSuiteRank(suite Suite) int {
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
	case SuiteTraverse:
		return 6
	case SuiteBind:
		return 7
	case SuiteRevise:
		return 8
	case SuiteOrder:
		return 9
	default:
		return 0
	}
}

func (registration OfflineMatrixPreregistration) Validate() error {
	plan := registration.Plan()
	if err := plan.Validate(); err != nil {
		return err
	}
	if err := registration.Fixed.Validate(); err != nil {
		return err
	}
	if registration.RegisteredAt.IsZero() ||
		registration.RegisteredAt.After(time.Now().UTC().Add(time.Minute)) {
		return fmt.Errorf("offline matrix preregistration time is invalid")
	}
	expected := buildOfflineMatrixPreregistration(plan, registration.Fixed, registration.RegisteredAt)
	if !reflect.DeepEqual(registration, expected) {
		return fmt.Errorf("offline matrix preregistration differs from code-owned policy")
	}
	return nil
}

func (registration OfflineMatrixPreregistration) Plan() OfflineMatrixPlan {
	return OfflineMatrixPlan{
		Policy: registration.Policy, Suites: cloneMatrixSlice(registration.Suites),
		Seeds: cloneMatrixSlice(registration.Seeds), Repetitions: registration.Repetitions,
		Surface: registration.Surface,
	}
}

func (registration OfflineMatrixPreregistration) Matches(
	plan OfflineMatrixPlan,
	fixed OfflineMatrixFixedAuthority,
) bool {
	return reflect.DeepEqual(registration.Plan(), plan) &&
		reflect.DeepEqual(registration.Fixed, fixed)
}

func (registration OfflineMatrixPreregistration) SHA256() (string, error) {
	if err := registration.Validate(); err != nil {
		return "", err
	}
	return digestJSON(registration)
}

func buildOfflineMatrixPreregistration(
	plan OfflineMatrixPlan,
	fixed OfflineMatrixFixedAuthority,
	registeredAt time.Time,
) OfflineMatrixPreregistration {
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
	hypothesis := "Full cognition has positive paired success lift over the strongest baseline selected by the preregistered tournament, with rescues exceeding regressions and no validity loss."
	if plan.Policy == CompetenceEfficiencySuperiority {
		hypothesis = "Full cognition preserves success within two percentage points of the strongest preregistered baseline while reducing median context by at least forty-five percent, duplicate acquisitions, and tool operations."
	}
	return OfflineMatrixPreregistration{
		Schema: OfflineMatrixPreregistrationSchemaV2, Hypothesis: hypothesis,
		Policy: plan.Policy, Suites: cloneMatrixSlice(plan.Suites), Seeds: cloneMatrixSlice(plan.Seeds),
		Repetitions: plan.Repetitions, Surface: plan.Surface, Variants: variants,
		TournamentSeed: VariantRawObservation, TournamentRounds: offlineMatrixTournamentRounds(),
		TournamentSelectionPolicy: matrixTournamentSelectionPolicyV1,
		DiagnosticVariants:        []Variant{VariantTranscriptCompacted, VariantOracleEvidence},
		ContaminatedVariants:      []Variant{VariantOracleEvidence},
		BenchmarkOnlyVariants:     []Variant{VariantRawShell}, Cases: cases,
		SampleCount: len(cases), RunCount: len(cases) * len(variants), Ordering: matrixOrderingV2,
		StatisticalTest: matrixStatisticalTestV1, AlphaPPM: matrixAlphaPPM,
		MinimumEffectBasisPoints:      matrixMinimumEffectBasisPoints,
		ContextReductionBasisPoints:   matrixContextReductionBasisPoints,
		MaximumSuccessLossBasisPoints: matrixMaximumSuccessLossBasisPoints,
		Fixed:                         fixed,
		RegisteredAt:                  registeredAt,
	}
}

func cloneMatrixSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append([]T{}, values...)
}
