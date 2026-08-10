package cognitionpolicy

import (
	"errors"
	"testing"
)

func TestBrainReferenceRequiresFullFrozenIdentity(t *testing.T) {
	t.Parallel()
	valid := policyTestBrain()
	if err := valid.Validate(); err != nil {
		t.Fatalf("validate brain: %v", err)
	}
	mutations := map[string]func(*BrainRef){
		"model":           func(ref *BrainRef) { ref.Model = "" },
		"model digest":    func(ref *BrainRef) { ref.Digest = "bad" },
		"quantization":    func(ref *BrainRef) { ref.Quantization = "" },
		"sampling":        func(ref *BrainRef) { ref.SamplingSHA256 = "bad" },
		"sampling limit":  func(ref *BrainRef) { ref.Sampling.MaxOutputTokens++ },
		"native context":  func(ref *BrainRef) { ref.NativeContextLimit = 0 },
		"byte ceiling":    func(ref *BrainRef) { ref.ContextCeilingBytes = 0 },
		"backend":         func(ref *BrainRef) { ref.Backend = "" },
		"backend version": func(ref *BrainRef) { ref.BackendVersion = "" },
		"hardware":        func(ref *BrainRef) { ref.Hardware = "" },
		"invalid UTF-8":   func(ref *BrainRef) { ref.Hardware = string([]byte{0xff}) },
		"NUL":             func(ref *BrainRef) { ref.Backend = "bad\x00backend" },
		"native over limit": func(ref *BrainRef) {
			ref.NativeContextLimit = MaxNativeContextLimit + 1
		},
		"ceiling over limit": func(ref *BrainRef) {
			ref.ContextCeilingBytes = MaxContextCeilingBytes + 1
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			invalid := valid
			mutate(&invalid)
			if err := invalid.Validate(); !errors.Is(err, ErrInvalidBrain) {
				t.Fatalf("error = %v, want ErrInvalidBrain", err)
			}
		})
	}
}
