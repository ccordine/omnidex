package worker

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestDirectCodingSemanticLeafCallBindsRawResultWithStationDecoder(t *testing.T) {
	t.Parallel()
	input := assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a small browser tool.",
	}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(current assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if current.ID != job.ID || model != "semantic-model" {
				t.Fatalf("current=%+v model=%q", current, model)
			}
			return assemblyline.PortableResult{
				JobID: current.ID, Candidate: "browser_application",
			}, nil
		},
	}
	result, err := runDirectCodingSemanticLeafCall(
		runtime, "semantic-model", "surface", job, nil,
		func(raw string) (assemblyline.ApplicationClassification, error) {
			return assemblyline.DecodeApplicationClassification(input, raw)
		},
		func(value assemblyline.ApplicationClassification) error {
			return value.Validate()
		},
	)
	if err != nil || result.Surface != assemblyline.ApplicationSurfaceBrowser || calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
	}
}

func TestDirectCodingSemanticLeafNeverNormalizesModelBytes(t *testing.T) {
	t.Parallel()
	input := assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a small browser tool.",
	}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name      string
		candidate string
	}{
		{name: "surrounding whitespace", candidate: " browser_application "},
		{name: "structured wrapper", candidate: `{"surface":"browser_application"}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 3,
				Execute: func(current assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					return assemblyline.PortableResult{JobID: current.ID, Candidate: testCase.candidate}, nil
				},
			}
			_, err := runDirectCodingSemanticLeafCall(
				runtime, "semantic-model", "surface", job, nil,
				func(raw string) (assemblyline.ApplicationClassification, error) {
					return assemblyline.DecodeApplicationClassification(input, raw)
				},
				func(value assemblyline.ApplicationClassification) error {
					return value.Validate()
				},
			)
			if err == nil || calls != 1 {
				t.Fatalf("model bytes were normalized: calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestDirectCodingSemanticLeafRejectsFilesystemIdentityBeforeDecoding(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		[]string{"internal/private/secret_owner.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.ApplicationProductContextInput{
		UserRequest: "Build a small browser tool.",
		Context: mustApplicationContextForSemanticLeafTest(
			t, "Build a small browser tool.",
		),
	}
	job, err := assemblyline.NewApplicationProductContextJob(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{
		"The owner is /private/file.go.",
		"The owner is secret_owner.go.",
	} {
		decodeCalls := 0
		_, err := runDirectCodingSemanticLeafCall(
			typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				PathProvenance: provenance,
				Execute: func(current assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					return assemblyline.PortableResult{JobID: current.ID, Candidate: candidate}, nil
				},
			},
			"semantic-model", "product", job, nil,
			func(raw string) (string, error) {
				decodeCalls++
				return raw, nil
			},
			func(string) error { return nil },
		)
		if err == nil || decodeCalls != 0 {
			t.Fatalf("candidate=%q decode_calls=%d error=%v", candidate, decodeCalls, err)
		}
	}
}

func TestDirectCodingSemanticLeafKeepsTypedHTTPValuesOutsideFilesystemGrammar(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		kind      assemblyline.WorkKind
		candidate string
	}{
		{assemblyline.WorkApplicationServiceEndpointRouteTemplate, "/records/{record_id}"},
		{assemblyline.WorkApplicationServiceEndpointRequestMedia, "application/json"},
		{assemblyline.WorkApplicationServiceEndpointResponseMedia, "text/html"},
	} {
		if err := validateDirectCodingSemanticCandidatePathBoundary(
			fixture.kind, fixture.candidate, assemblyline.ArtifactIdentityProvenance{},
		); err != nil {
			t.Fatalf("kind=%s candidate=%q error=%v", fixture.kind, fixture.candidate, err)
		}
	}
}

func TestDirectCodingSemanticLeafUsesQuotedSourceGrammarOnlyForRepairGuidance(t *testing.T) {
	t.Parallel()
	for _, candidate := range []string{
		`Set the placard to "Gallery [temporary]\nHours:\n  09:00-17:00".`,
		`Replace the displayed literal with "Ready\nWaiting".`,
	} {
		if err := validateDirectCodingSemanticCandidatePathBoundary(
			assemblyline.WorkTypeScriptRepairGuidance, candidate,
			assemblyline.ArtifactIdentityProvenance{},
		); err != nil {
			t.Fatalf("repair guidance %q error=%v", candidate, err)
		}
		if err := validateDirectCodingSemanticCandidatePathBoundary(
			assemblyline.WorkApplicationProductContext, candidate,
			assemblyline.ArtifactIdentityProvenance{},
		); err == nil {
			t.Fatalf("ordinary semantic result %q received repair-source grammar", candidate)
		}
	}
	for _, candidate := range []string{
		`Set the returned string to "../private/value".`,
		`Set the returned string to "C:\\private\\value".`,
	} {
		if err := validateDirectCodingSemanticCandidatePathBoundary(
			assemblyline.WorkTypeScriptRepairGuidance, candidate,
			assemblyline.ArtifactIdentityProvenance{},
		); err == nil {
			t.Fatalf("repair guidance accepted path %q", candidate)
		}
	}
}

func mustApplicationContextForSemanticLeafTest(
	t *testing.T,
	request string,
) assemblyline.ApplicationContext {
	t.Helper()
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	return context
}
