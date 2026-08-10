package llm

import "testing"

func TestPreparedGenerationCloneOwnsEveryProviderByteSlice(t *testing.T) {
	t.Parallel()
	generation := PreparedGeneration{
		ProviderResponseCapture: []byte("response"),
		ProviderIdentityEvidence: ProviderIdentityEvidence{
			Operations: []ProviderIdentityOperationEvidence{{
				Request: []byte("request"), ResponseCapture: []byte("identity"),
			}},
		},
	}
	cloned := generation.Clone()
	generation.ProviderResponseCapture[0] = 'X'
	generation.ProviderIdentityEvidence.Operations[0].Request[0] = 'X'
	generation.ProviderIdentityEvidence.Operations[0].ResponseCapture[0] = 'X'
	if string(cloned.ProviderResponseCapture) != "response" ||
		string(cloned.ProviderIdentityEvidence.Operations[0].Request) != "request" ||
		string(cloned.ProviderIdentityEvidence.Operations[0].ResponseCapture) != "identity" {
		t.Fatal("prepared generation clone retained provider-owned backing memory")
	}
}
