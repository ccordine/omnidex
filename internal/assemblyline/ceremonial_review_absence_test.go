package assemblyline

import "testing"

func TestCeremonialFrontDoorReviewKindsAreNotRegistered(t *testing.T) {
	t.Parallel()
	for _, retired := range []WorkKind{
		"application_intent_review",
		"application_job_specification_review",
	} {
		if validWorkKind(retired) {
			t.Fatalf("ceremonial work kind %q remains registered", retired)
		}
		if _, err := newPortableJob(retired, map[string]string{"value": "x"}); err == nil {
			t.Fatalf("ceremonial work kind %q was accepted", retired)
		}
	}
}
