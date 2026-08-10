package cognitiongauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	digestPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	commitPattern   = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	scenarioPattern = regexp.MustCompile(`^scenario-[0-9a-f]{64}$`)
	episodePattern  = regexp.MustCompile(`^episode-[0-9a-f]{64}$`)
)

func (manifest PublicManifest) Validate() error {
	if manifest.Schema != PublicManifestSchemaV1 {
		return fmt.Errorf("public cognition scenario schema is invalid")
	}
	if !validSuite(manifest.Suite) || !scenarioPattern.MatchString(string(manifest.Scenario.ID)) ||
		!validDigest(manifest.Scenario.SHA256) || !validDigest(manifest.ActionCatalogSHA256) {
		return fmt.Errorf("public cognition scenario identity, suite, or catalog is invalid")
	}
	for label, value := range map[string]string{
		"format version": manifest.FormatVersion, "surface version": manifest.SurfaceVersion,
		"action catalog version": manifest.ActionCatalogVersion, "goal": manifest.Goal,
	} {
		if err := requireExact(value, label, 4096); err != nil {
			return err
		}
	}
	return manifest.Difficulty.Validate()
}

func (difficulty Difficulty) Validate() error {
	if difficulty.WorldSize <= 0 || difficulty.WorldSize > 1_000_000 ||
		difficulty.RelevantArtifacts <= 0 || difficulty.RelevantArtifacts > difficulty.WorldSize ||
		difficulty.SolutionDepth <= 0 || difficulty.SolutionDepth > 10_000 ||
		difficulty.BranchingFactor < 0 || difficulty.BranchingFactor > 10_000 ||
		!finite(difficulty.DistractorRatio) || difficulty.DistractorRatio < 0 || difficulty.DistractorRatio > 1 ||
		difficulty.SemanticAmbiguity < 0 || difficulty.SemanticAmbiguity > 10_000 ||
		difficulty.DependencyCount < 0 || difficulty.DependencyCount > 10_000 ||
		difficulty.DelayedFactCount < 0 || difficulty.DelayedFactCount > 10_000 ||
		difficulty.SimultaneousGoals < 0 || difficulty.SimultaneousGoals > 10_000 ||
		difficulty.IrreversibleActions < 0 || difficulty.IrreversibleActions > 10_000 ||
		difficulty.WorkingSetBudgetBytes <= 0 || difficulty.WorkingSetBudgetBytes > 64*1024*1024 ||
		difficulty.ContextBudgetBytes <= 0 || difficulty.ContextBudgetBytes > 64*1024*1024 ||
		difficulty.ToolBudget <= 0 || difficulty.ToolBudget > 1_000_000 ||
		difficulty.RestartCount < 0 || difficulty.RestartCount > 10_000 {
		return fmt.Errorf("cognition scenario difficulty is outside registered bounds")
	}
	return nil
}

func (manifest OracleManifest) Validate() error {
	if manifest.Schema != OracleManifestSchemaV1 || !scenarioPattern.MatchString(string(manifest.ScenarioID)) ||
		!validDigest(manifest.PublicSHA256) || !validDigest(manifest.OracleSHA256) {
		return fmt.Errorf("private cognition oracle identity is invalid")
	}
	if err := requireExact(manifest.GeneratorVersion, "generator version", 256); err != nil {
		return err
	}
	if err := requireExact(manifest.TaskArchetype, "task archetype", 256); err != nil {
		return err
	}
	if manifest.WitnessCost <= 0 || manifest.LowerBound <= 0 || manifest.LowerBound > manifest.WitnessCost {
		return fmt.Errorf("private cognition oracle cost bounds are invalid")
	}
	switch manifest.Quality {
	case OracleOptimal:
		if manifest.OptimalCost == nil || *manifest.OptimalCost <= 0 ||
			*manifest.OptimalCost < manifest.LowerBound || *manifest.OptimalCost > manifest.WitnessCost {
			return fmt.Errorf("optimal cognition oracle requires a proven bounded cost")
		}
	case OracleWitnessOnly:
		if manifest.OptimalCost != nil {
			return fmt.Errorf("witness-only cognition oracle cannot claim an optimal cost")
		}
	default:
		return fmt.Errorf("private cognition oracle quality is invalid")
	}
	return nil
}

func DecisionEfficiency(quality OracleQuality, actual, reference int64) (EfficiencyMetric, error) {
	if actual < 0 || reference <= 0 {
		return EfficiencyMetric{}, fmt.Errorf("decision efficiency costs are invalid")
	}
	metric := EfficiencyMetric{Ratio: float64(actual) / float64(reference)}
	switch quality {
	case OracleOptimal:
		metric.Name = MetricDecisionRegret
	case OracleWitnessOnly:
		metric.Name = MetricWitnessOverhead
	default:
		return EfficiencyMetric{}, fmt.Errorf("decision efficiency oracle quality is invalid")
	}
	return metric, nil
}

func (evaluation Evaluation) Validate() error {
	if evaluation.Schema != EvaluationSchemaV1 || !validDigest(evaluation.EpisodeSealSHA256) ||
		!validDigest(evaluation.OracleSHA256) {
		return fmt.Errorf("cognition evaluation identity is invalid")
	}
	if err := requireExact(evaluation.EvaluatorVersion, "evaluator version", 256); err != nil {
		return err
	}
	if err := requireExact(evaluation.TaskArchetype, "evaluated task archetype", 256); err != nil {
		return err
	}
	if evaluation.GoalSuccess && !evaluation.ValidTerminalState {
		return fmt.Errorf("successful cognition evaluation requires a valid terminal state")
	}
	if evaluation.GoalSuccess {
		if evaluation.Attribution != nil {
			return fmt.Errorf("successful cognition evaluation cannot have failure attribution")
		}
	} else {
		if evaluation.Attribution == nil {
			return fmt.Errorf("failed cognition evaluation requires deterministic attribution")
		}
		if err := evaluation.Attribution.Validate(); err != nil {
			return err
		}
	}
	if evaluation.CleanDesk != nil {
		if err := evaluation.CleanDesk.Validate(); err != nil {
			return err
		}
	}
	if evaluation.CausalAcquisition != nil {
		if err := evaluation.CausalAcquisition.Validate(); err != nil {
			return err
		}
	}
	_, err := DecisionEfficiency(evaluation.Quality, evaluation.ActualDecisionCost, evaluation.ReferenceDecisionCost)
	return err
}

func (evaluation Evaluation) EfficiencyMetric() (EfficiencyMetric, error) {
	if err := evaluation.Validate(); err != nil {
		return EfficiencyMetric{}, err
	}
	return DecisionEfficiency(evaluation.Quality, evaluation.ActualDecisionCost, evaluation.ReferenceDecisionCost)
}

func validSuite(suite Suite) bool {
	switch suite {
	case SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined, SuiteTraverse,
		SuiteBind, SuiteRevise, SuiteOrder, SuiteResume, SuiteScale, SuiteTransfer, SuiteRogue:
		return true
	default:
		return false
	}
}

func validDigest(value string) bool { return digestPattern.MatchString(value) }

func validCommitIdentity(value string) bool { return commitPattern.MatchString(value) }

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func requireExact(value, label string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len([]byte(value)) > maximum {
		return fmt.Errorf("%s must be one exact UTF-8 value within %d bytes", label, maximum)
	}
	return nil
}

func digestJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
