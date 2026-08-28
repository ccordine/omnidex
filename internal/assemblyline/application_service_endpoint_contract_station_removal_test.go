package assemblyline

import "testing"

func TestBundledServiceEndpointContractWorkKindIsRetired(t *testing.T) {
	t.Parallel()
	for _, kind := range AllWorkKinds() {
		if kind == WorkKind("application_service_endpoint_contract") {
			t.Fatal("bundled service endpoint contract remains in the production work registry")
		}
	}
	job := PortableJob{
		Schema:  "omnidex.portable-job.v1",
		Kind:    WorkKind("application_service_endpoint_contract"),
		Payload: []byte(`{}`),
	}
	job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
	if err := job.Validate(); err == nil {
		t.Fatal("bundled service endpoint contract remains a valid production job")
	}
}
