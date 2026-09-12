package model

import (
	"reflect"
	"testing"
)

func TestCodingPlanCarriesNoModelScopeJudgments(t *testing.T) {
	if _, exists := reflect.TypeOf(CodingPlan{}).FieldByName("ScopeMode"); exists {
		t.Fatal("coding plan retains an optional-scope policy")
	}
	if _, exists := reflect.TypeOf(CodingPlanLeaf{}).FieldByName("Annotation"); exists {
		t.Fatal("coding plan leaf retains a model scope annotation")
	}
}
