package assemblyline

import "testing"

func TestPortableJobRegistryRejectsRemovedFileContentMapper(t *testing.T) {
	const removedKind = WorkKind("application_file_content")
	for _, kind := range AllWorkKinds() {
		if kind == removedKind {
			t.Fatal("removed requirement-to-path mapper remains in portable job registry")
		}
	}
	job := PortableJob{
		Schema: "omnidex.portable-job.v1",
		ID:     "removed-file-content-mapper",
		Kind:   removedKind,
		Payload: []byte(`{
}`),
	}
	if err := job.Validate(); err == nil {
		t.Fatal("removed requirement-to-path mapper remained a valid portable job")
	}
}
