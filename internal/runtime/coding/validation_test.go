package coding

import (
	"reflect"
	"testing"
)

func TestValidatorCannotCreateWork(t *testing.T) {
	typ := reflect.TypeOf(ValidationResult{})
	got := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		got = append(got, typ.Field(i).Name)
	}
	want := []string{"Status", "Evidence", "Violations", "Reason"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidationResult fields=%v want %v", got, want)
	}

	forbidden := []string{
		"Tasks",
		"TaskQueue",
		"NextAction",
		"Actions",
		"Replan",
		"ReplanningInstruction",
		"Patch",
		"Feedback",
		"FeedbackCommand",
		"Writes",
		"Deletes",
	}
	for _, field := range forbidden {
		if _, ok := typ.FieldByName(field); ok {
			t.Fatalf("ValidationResult must not include work-creation field %q", field)
		}
	}
}
