package cognitiongauntlet

import (
	"reflect"
	"testing"
)

func TestOfflineResumeSchedulesAreCodeOwnedAndDeterministic(t *testing.T) {
	first, err := BuildOfflineResumeSchedules(15_001, 7, 32)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildOfflineResumeSchedules(15_001, 7, 32)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first) != 5 {
		t.Fatalf("Resume schedules are not deterministic: %#v %#v", first, second)
	}
	if len(first[1].DecisionBoundaries) != 1 || len(first[2].DecisionBoundaries) != 5 ||
		!first[3].Dynamic || first[4].Kind != ResumeLiveInferenceExpiry {
		t.Fatalf("Resume schedule policy changed: %#v", first)
	}
	for _, schedule := range first {
		if err := schedule.Validate(7, 32); err != nil {
			t.Fatalf("schedule %q: %v", schedule.ID, err)
		}
	}
}

func TestOfflineResumeSchedulesRejectPostRegistrationSubstitution(t *testing.T) {
	schedules, err := BuildOfflineResumeSchedules(15_001, 7, 32)
	if err != nil {
		t.Fatal(err)
	}
	tests := []func(*OfflineResumeSchedule){
		func(value *OfflineResumeSchedule) { value.DecisionBoundaries = nil },
		func(value *OfflineResumeSchedule) { value.DecisionBoundaries = []uint32{0} },
		func(value *OfflineResumeSchedule) { value.RequiredKills++ },
		func(value *OfflineResumeSchedule) { value.Dynamic = !value.Dynamic },
		func(value *OfflineResumeSchedule) { value.Kind = "invented" },
	}
	for index, mutate := range tests {
		candidate := schedules[index%len(schedules)]
		candidate.DecisionBoundaries = append([]uint32{}, candidate.DecisionBoundaries...)
		mutate(&candidate)
		if err := candidate.Validate(7, 32); err == nil {
			t.Fatalf("mutation %d was accepted: %#v", index, candidate)
		}
	}
}

func TestOfflineResumeSchedulesRequireFiveInteriorDecisions(t *testing.T) {
	for _, depth := range []int{0, 1, 6} {
		if _, err := BuildOfflineResumeSchedules(15_001, depth, 32); err == nil {
			t.Fatalf("decision depth %d was accepted", depth)
		}
	}
	if _, err := BuildOfflineResumeSchedules(0, 7, 32); err == nil {
		t.Fatal("zero seed was accepted")
	}
}
