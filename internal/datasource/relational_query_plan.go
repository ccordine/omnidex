package datasource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

const RelationalQueryPlanV1 = "omnidex.relational-query-plan.v1"

type JoinPathSelection struct {
	TargetRelationID string `json:"target_relation_id"`
	PathID           string `json:"path_id"`
}

type PlannedJoinPath struct {
	TargetRelationID string   `json:"target_relation_id"`
	Path             JoinPath `json:"path"`
}

// RelationalQueryPlan is the complete data contract for one code-owned read.
// It deliberately contains no SQL, connection information, or credentials.
type RelationalQueryPlan struct {
	Schema             string              `json:"schema"`
	SourceID           string              `json:"source_id"`
	SchemaFingerprint  string              `json:"schema_fingerprint"`
	Intent             RelationalIntent    `json:"intent"`
	JoinPathSelections []JoinPathSelection `json:"join_path_selections"`
	JoinPaths          []PlannedJoinPath   `json:"join_paths"`
	Outputs            []CompiledOutput    `json:"outputs"`
	IntentHash         string              `json:"intent_hash"`
	PlanHash           string              `json:"plan_hash"`
}

func BuildRelationalQueryPlan(
	snapshot SchemaSnapshot,
	intent RelationalIntent,
	selected map[string]string,
) (RelationalQueryPlan, error) {
	if err := snapshot.ValidateIntegrity(); err != nil {
		return RelationalQueryPlan{}, err
	}
	normalizeIntentSlices(&intent)
	compiled, err := CompilePostgresWithJoinPaths(snapshot, intent, selected)
	if err != nil {
		return RelationalQueryPlan{}, err
	}
	selections := make([]JoinPathSelection, 0, len(selected))
	for target, pathID := range selected {
		selections = append(selections, JoinPathSelection{TargetRelationID: target, PathID: pathID})
	}
	sort.Slice(selections, func(i, j int) bool {
		return selections[i].TargetRelationID < selections[j].TargetRelationID
	})
	targets, err := relationalPlanJoinTargets(snapshot, intent)
	if err != nil {
		return RelationalQueryPlan{}, err
	}
	paths := make([]PlannedJoinPath, len(targets))
	for index, target := range targets {
		path, err := ResolveJoinPath(snapshot, intent.FromRelationID, target, selected[target])
		if err != nil {
			return RelationalQueryPlan{}, err
		}
		paths[index] = PlannedJoinPath{TargetRelationID: target, Path: path}
	}
	plan := RelationalQueryPlan{
		Schema: RelationalQueryPlanV1, SourceID: snapshot.SourceID,
		SchemaFingerprint: snapshot.Fingerprint, Intent: intent,
		JoinPathSelections: selections, JoinPaths: paths,
		Outputs: append([]CompiledOutput(nil), compiled.Outputs...), IntentHash: compiled.IntentHash,
	}
	planHash, err := relationalQueryPlanHash(plan)
	if err != nil {
		return RelationalQueryPlan{}, err
	}
	plan.PlanHash = planHash
	return plan, nil
}

func (plan RelationalQueryPlan) Validate(snapshot SchemaSnapshot) error {
	if plan.Schema != RelationalQueryPlanV1 || plan.SourceID != snapshot.SourceID ||
		plan.SchemaFingerprint != snapshot.Fingerprint {
		return fmt.Errorf("relational query plan authority does not match schema snapshot")
	}
	selected := make(map[string]string, len(plan.JoinPathSelections))
	for _, selection := range plan.JoinPathSelections {
		if selection.TargetRelationID == "" || selection.PathID == "" {
			return fmt.Errorf("relational query plan contains an incomplete join-path selection")
		}
		if _, duplicate := selected[selection.TargetRelationID]; duplicate {
			return fmt.Errorf("relational query plan repeats a join-path selection target")
		}
		selected[selection.TargetRelationID] = selection.PathID
	}
	rebuilt, err := BuildRelationalQueryPlan(snapshot, plan.Intent, selected)
	if err != nil {
		return fmt.Errorf("rebuild relational query plan: %w", err)
	}
	if !reflect.DeepEqual(plan, rebuilt) {
		return fmt.Errorf("relational query plan fields or hashes are not canonical")
	}
	return nil
}

func CompilePostgresPlan(snapshot SchemaSnapshot, plan RelationalQueryPlan) (CompiledQuery, error) {
	if err := plan.Validate(snapshot); err != nil {
		return CompiledQuery{}, err
	}
	selected := make(map[string]string, len(plan.JoinPathSelections))
	for _, selection := range plan.JoinPathSelections {
		selected[selection.TargetRelationID] = selection.PathID
	}
	compiled, err := CompilePostgresWithJoinPaths(snapshot, plan.Intent, selected)
	if err != nil {
		return CompiledQuery{}, err
	}
	if compiled.IntentHash != plan.IntentHash || !reflect.DeepEqual(compiled.Outputs, plan.Outputs) {
		return CompiledQuery{}, fmt.Errorf("compiled PostgreSQL query diverged from relational plan")
	}
	return compiled, nil
}

func relationalPlanJoinTargets(snapshot SchemaSnapshot, intent RelationalIntent) ([]string, error) {
	targets, err := requiredJoinRelationIDs(snapshot, intent)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(targets)+len(intent.Exists))
	for _, target := range targets {
		set[target] = struct{}{}
	}
	for _, existence := range intent.Exists {
		if existence.RelationID != intent.FromRelationID {
			set[existence.RelationID] = struct{}{}
		}
	}
	targets = targets[:0]
	for target := range set {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets, nil
}

func relationalQueryPlanHash(plan RelationalQueryPlan) (string, error) {
	plan.PlanHash = ""
	encoded, err := json.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("encode relational query plan: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
