package cognitiongauntlet

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
)

const (
	visibleFactSchemaV1        = "omnidex.public-observation.visible-record-fact.v1"
	visibleFactPolicyID        = cognitionstate.FactAcceptancePolicyID("public-observation.visible-record")
	maxVisibleFactRecords      = 16
	maxVisibleFactContentBytes = 512
)

type visibleRecordFact struct {
	Schema  string                `json:"schema"`
	Source  cognition.EvidenceRef `json:"source"`
	Records []visibleFactRecord   `json:"records"`
}

type visibleFactRecord struct {
	ID            string `json:"id"`
	Content       string `json:"content"`
	ContentSHA256 string `json:"content_sha256"`
}

type visibleObservationFactPlanner struct{ ref cognitionstate.FactPlannerRef }

func (planner visibleObservationFactPlanner) Reference() cognitionstate.FactPlannerRef {
	return planner.ref
}

func (planner visibleObservationFactPlanner) Plan(
	transition cognition.Transition,
) ([]cognitionstate.FactPlan, error) {
	plans := make([]cognitionstate.FactPlan, 0, len(transition.Observations))
	for index, observation := range transition.Observations {
		records, err := visibleFactRecords(observation.Content)
		if err != nil {
			return nil, fmt.Errorf("visible fact observation %d: %w", index+1, err)
		}
		if len(records) == 0 {
			continue
		}
		plans = append(plans, cognitionstate.FactPlan{
			PolicyID:     visibleFactPolicyID,
			EvidenceRefs: []cognition.EvidenceRef{observation.EvidenceRef()},
		})
	}
	return plans, nil
}

func newVisibleObservationFactAuthority() (cognitionstate.FactAcceptanceAuthority, error) {
	policyRef := cognitionstate.FactAcceptancePolicyRef{
		ID: visibleFactPolicyID, Version: "1.0.0",
		SHA256: visibleFactDigest("visible-record-policy.v1\x00" + visibleFactSchemaV1),
	}
	policy, err := cognitionstate.NewFactAcceptancePolicy(policyRef, deriveVisibleObservationFact)
	if err != nil {
		return cognitionstate.FactAcceptanceAuthority{}, err
	}
	registry, err := cognitionstate.NewFactPolicyRegistry([]cognitionstate.FactAcceptancePolicy{policy})
	if err != nil {
		return cognitionstate.FactAcceptanceAuthority{}, err
	}
	planner := visibleObservationFactPlanner{ref: cognitionstate.FactPlannerRef{
		ID: "public-observation.visible-record-planner", Version: "1.0.0",
		SHA256: visibleFactDigest("visible-record-planner.v1\x00public-observation-only"),
	}}
	return cognitionstate.NewFactAcceptanceAuthority(planner, registry)
}

func deriveVisibleObservationFact(evidence []cognitionstate.FactEvidence) (string, error) {
	if len(evidence) != 1 {
		return "", fmt.Errorf("visible record fact requires one exact public observation")
	}
	if err := evidence[0].Ref.Validate(); err != nil {
		return "", fmt.Errorf("visible record fact source: %w", err)
	}
	records, err := visibleFactRecords(evidence[0].Content)
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", fmt.Errorf("public observation contains no content-bearing records")
	}
	raw, err := json.Marshal(visibleRecordFact{
		Schema: visibleFactSchemaV1, Source: evidence[0].Ref, Records: records,
	})
	if err != nil {
		return "", fmt.Errorf("encode visible record fact: %w", err)
	}
	return string(raw), nil
}

func canonicalVisibleFactRecords(records map[string]visibleFactRecord) ([]visibleFactRecord, error) {
	if len(records) > maxVisibleFactRecords {
		return nil, fmt.Errorf("visible record count exceeds %d", maxVisibleFactRecords)
	}
	result := make([]visibleFactRecord, 0, len(records))
	for id, record := range records {
		if requireExact(id, "visible record ID", 256) != nil || record.ID != id ||
			len(record.Content) == 0 || len(record.Content) > maxVisibleFactContentBytes ||
			!validDigest(record.ContentSHA256) || visibleFactDigest(record.Content) != record.ContentSHA256 {
			return nil, fmt.Errorf("visible record identity, content, or digest is invalid")
		}
		result = append(result, record)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, nil
}

func visibleFactDigest(value string) string { return ablationContentSHA256(value) }
