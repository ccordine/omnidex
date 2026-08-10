package cognitiongauntlet

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
)

const OfflineResumeSchedulePolicyV1 = "omnidex.offline-resume-schedule-policy.v1"

type OfflineResumeScheduleKind string

const (
	ResumeUninterrupted       OfflineResumeScheduleKind = "uninterrupted"
	ResumeOneSeededKill       OfflineResumeScheduleKind = "one_seeded_kill"
	ResumeFiveSeededKills     OfflineResumeScheduleKind = "five_seeded_kills"
	ResumeEveryDecision       OfflineResumeScheduleKind = "every_decision"
	ResumeLiveInferenceExpiry OfflineResumeScheduleKind = "live_inference_expiry"
)

type OfflineResumeSchedule struct {
	ID                 string                    `json:"id"`
	Kind               OfflineResumeScheduleKind `json:"kind"`
	DecisionBoundaries []uint32                  `json:"decision_boundaries"`
	Dynamic            bool                      `json:"dynamic"`
	RequiredKills      int                       `json:"required_kills"`
}

func BuildOfflineResumeSchedules(
	seed uint64,
	minimumDecisionDepth int,
	modelCallBudget int,
) ([]OfflineResumeSchedule, error) {
	if seed == 0 || minimumDecisionDepth < 7 || modelCallBudget <= minimumDecisionDepth {
		return nil, fmt.Errorf("offline Resume workload lacks its required decision authority")
	}
	interior := minimumDecisionDepth - 1
	one := seededResumeBoundaries(seed, interior, 1)
	five := seededResumeBoundaries(seed^0x9e3779b97f4a7c15, interior, 5)
	schedules := []OfflineResumeSchedule{
		{ID: "resume-uninterrupted-v1", Kind: ResumeUninterrupted, DecisionBoundaries: []uint32{}, RequiredKills: 0},
		{ID: "resume-one-seeded-v1", Kind: ResumeOneSeededKill, DecisionBoundaries: one, RequiredKills: 1},
		{ID: "resume-five-seeded-v1", Kind: ResumeFiveSeededKills, DecisionBoundaries: five, RequiredKills: 5},
		{ID: "resume-every-decision-v1", Kind: ResumeEveryDecision, DecisionBoundaries: []uint32{}, Dynamic: true, RequiredKills: -1},
		{ID: "resume-live-inference-expiry-v1", Kind: ResumeLiveInferenceExpiry, DecisionBoundaries: []uint32{}, RequiredKills: 1},
	}
	for _, schedule := range schedules {
		if err := schedule.Validate(minimumDecisionDepth, modelCallBudget); err != nil {
			return nil, err
		}
	}
	return schedules, nil
}

func (schedule OfflineResumeSchedule) Validate(
	minimumDecisionDepth int,
	modelCallBudget int,
) error {
	if err := requireExact(schedule.ID, "offline Resume schedule ID", 128); err != nil {
		return err
	}
	if schedule.DecisionBoundaries == nil || minimumDecisionDepth < 1 || modelCallBudget < 1 {
		return fmt.Errorf("offline Resume schedule authority is incomplete")
	}
	for index, boundary := range schedule.DecisionBoundaries {
		if boundary == 0 || int(boundary) >= minimumDecisionDepth || int(boundary) >= modelCallBudget ||
			(index > 0 && schedule.DecisionBoundaries[index-1] >= boundary) {
			return fmt.Errorf("offline Resume schedule boundaries are not exact interior decisions")
		}
	}
	switch schedule.Kind {
	case ResumeUninterrupted:
		if schedule.ID != "resume-uninterrupted-v1" || schedule.Dynamic ||
			schedule.RequiredKills != 0 || len(schedule.DecisionBoundaries) != 0 {
			return fmt.Errorf("uninterrupted Resume schedule changed")
		}
	case ResumeOneSeededKill:
		if schedule.ID != "resume-one-seeded-v1" || schedule.Dynamic ||
			schedule.RequiredKills != 1 || len(schedule.DecisionBoundaries) != 1 {
			return fmt.Errorf("one-kill Resume schedule changed")
		}
	case ResumeFiveSeededKills:
		if schedule.ID != "resume-five-seeded-v1" || schedule.Dynamic ||
			schedule.RequiredKills != 5 || len(schedule.DecisionBoundaries) != 5 {
			return fmt.Errorf("five-kill Resume schedule changed")
		}
	case ResumeEveryDecision:
		if schedule.ID != "resume-every-decision-v1" || !schedule.Dynamic ||
			schedule.RequiredKills != -1 || len(schedule.DecisionBoundaries) != 0 {
			return fmt.Errorf("every-decision Resume schedule changed")
		}
	case ResumeLiveInferenceExpiry:
		if schedule.ID != "resume-live-inference-expiry-v1" || schedule.Dynamic ||
			schedule.RequiredKills != 1 || len(schedule.DecisionBoundaries) != 0 {
			return fmt.Errorf("live-inference Resume schedule changed")
		}
	default:
		return fmt.Errorf("offline Resume schedule kind %q is not registered", schedule.Kind)
	}
	return nil
}

func seededResumeBoundaries(seed uint64, maximum int, count int) []uint32 {
	values := make([]uint32, maximum)
	for index := range values {
		values[index] = uint32(index + 1)
	}
	for index := len(values) - 1; index > 0; index-- {
		var input [24]byte
		binary.BigEndian.PutUint64(input[0:8], seed)
		binary.BigEndian.PutUint64(input[8:16], uint64(index))
		binary.BigEndian.PutUint64(input[16:24], uint64(maximum))
		digest := sha256.Sum256(input[:])
		other := int(binary.BigEndian.Uint64(digest[:8]) % uint64(index+1))
		values[index], values[other] = values[other], values[index]
	}
	result := append([]uint32{}, values[:count]...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
