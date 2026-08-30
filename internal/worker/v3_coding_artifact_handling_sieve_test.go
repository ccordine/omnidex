package worker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationArtifactSieveDiscardsNonAuthoritativeCandidatesOnly(t *testing.T) {
	t.Parallel()
	directives := []assemblyline.ArtifactDirective{
		{Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactReference},
		{Token: "ARTIFACT_2", Disposition: assemblyline.ArtifactAbsenceCandidate},
		{Token: "ARTIFACT_3", Disposition: assemblyline.ArtifactProtect},
		{Token: "ARTIFACT_4", Disposition: assemblyline.ArtifactRequire},
		{Token: "ARTIFACT_5", Disposition: assemblyline.ArtifactForbid},
	}

	got, err := sieveDirectCodingApplicationArtifactDirectives(directives)
	if err != nil {
		t.Fatal(err)
	}
	want := []assemblyline.ArtifactDirective{
		{Token: "ARTIFACT_3", Disposition: assemblyline.ArtifactProtect},
		{Token: "ARTIFACT_4", Disposition: assemblyline.ArtifactRequire},
		{Token: "ARTIFACT_5", Disposition: assemblyline.ArtifactForbid},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("retained directives=%+v want %+v", got, want)
	}
	if !reflect.DeepEqual(directives, []assemblyline.ArtifactDirective{
		{Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactReference},
		{Token: "ARTIFACT_2", Disposition: assemblyline.ArtifactAbsenceCandidate},
		{Token: "ARTIFACT_3", Disposition: assemblyline.ArtifactProtect},
		{Token: "ARTIFACT_4", Disposition: assemblyline.ArtifactRequire},
		{Token: "ARTIFACT_5", Disposition: assemblyline.ArtifactForbid},
	}) {
		t.Fatal("application artifact sieve mutated candidate intake")
	}
}

func TestApplicationArtifactSieveRejectsStructurallyUnknownDisposition(t *testing.T) {
	t.Parallel()
	_, err := sieveDirectCodingApplicationArtifactDirectives([]assemblyline.ArtifactDirective{{
		Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactDisposition("invented"),
	}})
	if err == nil || !strings.Contains(err.Error(), "unsupported disposition") {
		t.Fatalf("unknown disposition error=%v", err)
	}
}

func TestApplicationInterpreterDoesNotPromoteArtifactIntakeCandidates(t *testing.T) {
	t.Parallel()
	for _, handling := range []assemblyline.ArtifactHandling{
		assemblyline.ArtifactMentionedOnly,
		assemblyline.ArtifactPossibleAbsenceCandidate,
	} {
		handling := handling
		t.Run(string(handling), func(t *testing.T) {
			const modelRequest = "Build a browser display associated with ARTIFACT_1."
			const authoritativeRequest = "Build a browser display associated with private/reference.png."
			applicationContext, err := assemblyline.BootstrapApplicationContext(
				modelRequest, assemblyline.ApplicationWorkspaceEmpty,
			)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := newDirectCodingApplicationRequestAuthority(
				authoritativeRequest, modelRequest,
			)
			if err != nil {
				t.Fatal(err)
			}
			artifactCalls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					candidate := ""
					switch job.Kind {
					case assemblyline.WorkApplicationClassify:
						candidate = string(assemblyline.ApplicationSurfaceBrowser)
					case assemblyline.WorkApplicationProductContext:
						candidate = "A browser display."
					case assemblyline.WorkApplicationRequirementInventory:
						candidate = "Show the display."
					case assemblyline.WorkApplicationRequirementCandidateAuthorization:
						candidate = assemblyline.ApplicationRequirementCandidateEntailed
					case assemblyline.WorkApplicationRequirementCandidateKind:
						var presenceErr error
						candidate, presenceErr = applicationRequirementCandidateContentPresenceForKindForTest(
							job,
							assemblyline.ApplicationRequirementCandidateTaskLocal,
						)
						if presenceErr != nil {
							return assemblyline.PortableResult{}, presenceErr
						}
					case assemblyline.WorkApplicationRequirementCandidateCardinality:
						candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
					case assemblyline.WorkApplicationRequirementCandidateResultRelation:
						candidate = assemblyline.ApplicationRequirementNoDerivedResult
					case assemblyline.WorkArtifactHandling:
						artifactCalls++
						candidate = string(handling)
					default:
						return assemblyline.PortableResult{}, fmt.Errorf(
							"unexpected work kind %q", job.Kind,
						)
					}
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			}
			interpretation, err := runDirectCodingApplicationInterpreter(
				runtime, "intent-model", "surface-model", "artifact-model",
				authority, applicationContext,
				[]assemblyline.ArtifactIdentity{{
					Token: "ARTIFACT_1", Value: "private/reference.png",
				}},
			)
			if err != nil {
				t.Fatal(err)
			}
			if artifactCalls != 1 || len(interpretation.Specification.Artifacts) != 0 {
				t.Fatalf(
					"handling=%s calls=%d promoted artifacts=%+v",
					handling, artifactCalls, interpretation.Specification.Artifacts,
				)
			}
			if len(interpretation.AcceptedRequirements) != 1 {
				t.Fatalf("independent accepted requirement was lost: %+v", interpretation)
			}
		})
	}
}
