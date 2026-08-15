package llm

import "testing"

func TestExactPreparedTemperatureProgressionUsesProfileBaseAndCeiling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		profile    string
		wantValues []float64
	}{
		{
			name:       "deterministic coder",
			profile:    ExactPreparedTokenizerProfileQwen25Coder,
			wantValues: []float64{0, 0.2, 0.4, 0.6, 0.8, 1},
		},
		{
			name:       "reasoning profile",
			profile:    ExactPreparedTokenizerProfileQwen3Qwen2,
			wantValues: []float64{0.6, 0.8, 1},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			expected := exactTemperatureTestExpectation(test.profile)
			transport, err := ResolveExactPreparedTransport(expected)
			if err != nil {
				t.Fatal(err)
			}
			current := transport.Temperature
			for index, want := range test.wantValues {
				if current == nil || float64(*current) != want {
					t.Fatalf("temperature[%d]=%v want %v", index, current, want)
				}
				next, ok, err := NextExactPreparedTemperature(expected, current)
				if err != nil {
					t.Fatal(err)
				}
				if index == len(test.wantValues)-1 {
					if ok || next != nil {
						t.Fatalf("ceiling advanced to %v", next)
					}
					continue
				}
				if !ok {
					t.Fatalf("temperature[%d] stopped before ceiling", index)
				}
				current = next
			}
		})
	}
}

func TestExactPreparedTemperatureProgressionPreservesNativeDefaultBeforeExploration(t *testing.T) {
	t.Parallel()
	expected := exactTemperatureTestExpectation(ExactPreparedTokenizerProfileMistral3)
	transport, err := ResolveExactPreparedTransport(expected)
	if err != nil {
		t.Fatal(err)
	}
	if transport.Temperature != nil {
		t.Fatalf("native-default profile unexpectedly forced temperature %v", transport.Temperature)
	}
	next, ok, err := NextExactPreparedTemperature(expected, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || next == nil || float64(*next) != 0.2 {
		t.Fatalf("first exploratory temperature=%v ok=%t", next, ok)
	}
}

func exactTemperatureTestExpectation(profile string) ProviderIdentityExpectation {
	return ProviderIdentityExpectation{
		Backend: ExactPreparedProviderBackend, BackendVersion: ExactPreparedProviderVersion,
		Model: "opaque:model", Digest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Quantization: "Q4_K_M", NativeContextLimit: 8192, TokenizerProfile: profile,
	}
}
