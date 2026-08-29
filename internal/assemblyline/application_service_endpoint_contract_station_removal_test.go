package assemblyline

import "testing"

func TestAggregateServiceTechnicalContractWorkKindsAreRetired(t *testing.T) {
	t.Parallel()
	retired := map[WorkKind]struct{}{}
	for _, value := range []WorkKind{
		"application_service_endpoint_contract",
	} {
		retired[value] = struct{}{}
	}
	for _, kind := range AllWorkKinds() {
		if _, forbidden := retired[kind]; forbidden {
			t.Fatalf("aggregate service endpoint work kind %q remains registered", kind)
		}
	}
	for kind := range retired {
		job := PortableJob{
			Schema:  "omnidex.portable-job.v1",
			Kind:    kind,
			Payload: []byte(`{}`),
		}
		job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
		if err := job.Validate(); err == nil {
			t.Fatalf("retired service technical kind %q remains valid", kind)
		}
	}
}
