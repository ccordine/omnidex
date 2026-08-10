package llm

import "testing"

func TestInferenceInputByteBudgetOwnsOutputAndOverheadReserve(t *testing.T) {
	available, reserved, err := InferenceInputByteBudget(8192, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if reserved != 1024 || available != 8192-1024-inferenceContextOverheadTokens {
		t.Fatalf("available=%d reserved=%d", available, reserved)
	}
	if err := ValidateInferenceBudget(8192, 1024, string(make([]byte, available+1))); err == nil {
		t.Fatal("ValidateInferenceBudget accepted one byte beyond the exported exact budget")
	}
}
