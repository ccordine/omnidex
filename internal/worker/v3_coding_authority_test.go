package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestDirectCodingRequestPreservesExactInstruction(t *testing.T) {
	t.Parallel()

	exact := "  Implement the bounded change.\nPreserve this line.\t "
	runtime := &nativeRuntimeV3{claim: &model.ClaimedStep{Job: model.Job{
		Instruction: exact,
	}}}
	request, err := runtime.directCodingRequest()
	if err != nil {
		t.Fatal(err)
	}
	if request.Instruction != exact {
		t.Fatalf("instruction changed: got %q want %q", request.Instruction, exact)
	}
}

func TestDirectCodingRequestRejectsBlankInstruction(t *testing.T) {
	t.Parallel()

	for _, blank := range []string{"", " ", "\n\t "} {
		runtime := &nativeRuntimeV3{claim: &model.ClaimedStep{Job: model.Job{
			Instruction: blank,
		}}}
		if _, err := runtime.directCodingRequest(); err == nil {
			t.Fatalf("blank instruction accepted: %q", blank)
		}
	}
}
