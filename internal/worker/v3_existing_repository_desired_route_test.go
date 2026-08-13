package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func TestArtifactAbsenceClassificationReturnsTruthNotFilesystemOperation(t *testing.T) {
	t.Parallel()
	identities := []assemblyline.ArtifactIdentity{{Token: "ARTIFACT_1", Value: "obsolete.go"}}
	var prompt string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			var err error
			prompt, _, err = assemblyline.RenderPortableJob(job)
			decision := assemblyline.ArtifactHandlingDecision{
				Schema: assemblyline.ArtifactHandlingSchemaV1,
				Token:  "ARTIFACT_1", Handling: assemblyline.ArtifactMustBeAbsent,
			}
			raw, marshalErr := json.Marshal(decision)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, marshalErr
		},
	}
	directives, err := classifyArtifactHandling(
		runtime, "semantic", "Remove ARTIFACT_1.", identities,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(directives) != 1 || directives[0].Disposition != assemblyline.ArtifactForbid {
		t.Fatalf("directives=%+v", directives)
	}
	if strings.Contains(prompt, "obsolete.go") {
		t.Fatalf("artifact classification leaked physical identity: %s", prompt)
	}
	for _, forbidden := range []string{"create_file", "delete_file", "write_file", " rm ", " mv "} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("artifact classification exposed operation %q: %s", forbidden, prompt)
		}
	}
}

func TestArtifactRoutingRejectsRenameOrMoveIdentityTransferBeforeInference(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	session := &directCodingSession{
		repositoryIndex: &repositoryindex.Result{Snapshot: snapshot, Analyses: []repositoryfacts.Analysis{analysis}},
	}
	_, err := classifyExistingRepositoryArtifactDirectives(
		session, "Move ARTIFACT_1 to ARTIFACT_2.",
		[]assemblyline.ArtifactIdentity{
			{Token: "ARTIFACT_1", Value: "first.go"},
			{Token: "ARTIFACT_2", Value: "renamed.go"},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "identity transfer") {
		t.Fatalf("rename/move route error=%v", err)
	}
}
