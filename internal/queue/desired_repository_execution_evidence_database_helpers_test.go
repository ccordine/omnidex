package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

type desiredExecutionFixture struct {
	repositoryMutationDatabaseFixture
	graphID string
	after   repositoryfacts.Snapshot
}

func newDesiredExecutionFixture(t *testing.T, label, transition string) desiredExecutionFixture {
	t.Helper()
	fixture := newRepositoryMutationDatabaseFixture(t, label)
	graphID := "desired_graph_" + repositoryMutationDigest("desired-execution-"+label)
	command := fixture.command
	command.ContractID = graphID
	command.Patch = "desired repository state " + transition + " " + label
	command.PatchSHA256 = repositoryMutationDigest(command.Patch)
	command.StageID = "repository_change_stage_" + command.PatchSHA256
	file := fixture.snapshot.Files[0]
	target := file.Path
	content := []byte("desired-" + transition + "-post")
	command.ChangedFiles[0] = RepositoryMutationFile{
		FileID: file.ID, Path: file.Path,
		SourcePresent: true, SourceSHA256: file.SHA256,
		SourceSize: file.Size, SourceMode: file.Mode,
	}
	switch transition {
	case "create":
		target = "created.go"
		fileID, err := repositoryfacts.FileIDForAbsentPath(fixture.snapshot, target)
		if err != nil {
			t.Fatal(err)
		}
		command.ChangedFiles[0] = RepositoryMutationFile{FileID: fileID, Path: target}
		command.ChangedFiles[0].ExpectedPresent = true
	case "modify":
		command.ChangedFiles[0].ExpectedPresent = true
	case "delete":
	default:
		t.Fatalf("unregistered desired execution transition %q", transition)
	}
	if command.ChangedFiles[0].ExpectedPresent {
		command.ChangedFiles[0].ExpectedSHA256 = repositoryMutationDigest(string(content))
		command.ChangedFiles[0].ExpectedSize = int64(len(content))
		command.ChangedFiles[0].ExpectedMode = 0o644
	}
	var state atomic.Value
	state.Store(RepositoryMutationSource)
	err := fixture.repository.ApplyRepositoryMutation(
		fixture.ctx, fixture.authority, command, stateClassifier(&state),
		func(context.Context) error {
			var mutateErr error
			if transition == "delete" {
				mutateErr = os.Remove(filepath.Join(fixture.snapshot.Root, target))
			} else {
				mutateErr = os.WriteFile(filepath.Join(fixture.snapshot.Root, target), content, 0o644)
			}
			if mutateErr == nil {
				state.Store(RepositoryMutationPost)
			}
			return mutateErr
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	after, err := repositoryfacts.BuildGitSnapshot(
		fixture.ctx, fixture.snapshot.Root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	projectID, err := fixture.repository.JobProjectID(fixture.ctx, fixture.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.StoreRepositorySnapshot(fixture.ctx, projectID, after); err != nil {
		t.Fatal(err)
	}
	fixture.command = command
	return desiredExecutionFixture{
		repositoryMutationDatabaseFixture: fixture, graphID: graphID, after: after,
	}
}

func (fixture desiredExecutionFixture) recordVerification(t *testing.T) {
	t.Helper()
	planID := repositoryMutationDigest("one exact verification plan")
	expectedPostID := repositoryMutationDigest("one exact expected post state")
	for _, scope := range []string{"baseline", "staged", "authoritative"} {
		metadata := map[string]any{
			"repository_verification_scope":        scope,
			"repository_mutation_owner_id":         fixture.graphID,
			"repository_desired_artifact_graph_id": fixture.graphID,
			"repository_source_snapshot_id":        fixture.snapshot.ID,
			"repository_verification_plan_id":      planID,
		}
		if scope == "baseline" {
			metadata["repository_verification_baseline_id"] =
				"repository_baseline_" + repositoryMutationDigest("baseline")
		} else {
			metadata["repository_change_stage_id"] = fixture.command.StageID
			metadata["repository_change_patch_sha256"] = fixture.command.PatchSHA256
			metadata["repository_expected_post_id"] = expectedPostID
			if scope == "authoritative" {
				metadata["repository_verification_snapshot_id"] = fixture.after.ID
			}
		}
		commandMetadata := cloneDesiredExecutionMetadata(metadata)
		commandMetadata["repository_structured_proof_valid"] = true
		commandMetadata["succeeded"] = true
		fixture.writeEvidence(t, evidence.Record{
			Kind: evidence.KindTestResult, SourceType: "command", SourceRef: "go",
			Command: "go test -json -count=1 ./...", Confidence: 1,
			Metadata: commandMetadata,
		})
		acceptanceMetadata := cloneDesiredExecutionMetadata(metadata)
		acceptanceMetadata["repository_verification_command_count"] = 1
		sourceType := "command-plan"
		if scope == "baseline" {
			sourceType = "command-baseline"
			acceptanceMetadata["repository_verification_baseline_accepted"] = true
		} else {
			acceptanceMetadata["repository_verification_plan_accepted"] = true
		}
		fixture.writeEvidence(t, evidence.Record{
			Kind: evidence.KindTestResult, SourceType: sourceType, SourceRef: "go",
			Confidence: 1, Metadata: acceptanceMetadata,
		})
	}
}

func (fixture desiredExecutionFixture) recordPostIndex(t *testing.T) {
	t.Helper()
	fixture.writeEvidence(t, evidence.Record{
		Kind: evidence.KindRepositoryIndex, SourceType: "repository",
		SourceRef: fixture.after.ID, Hash: fixture.after.GitStateSHA256, Confidence: 1,
		Metadata: map[string]any{
			"snapshot_id": fixture.after.ID, "file_count": len(fixture.after.Files),
			"analysis_ids": []string{},
		},
	})
}

func (fixture desiredExecutionFixture) writeEvidence(t *testing.T, record evidence.Record) {
	t.Helper()
	record.JobID, record.StepID = fixture.job.ID, fixture.stepID
	if err := fixture.repository.WriteEvidence(fixture.ctx, fixture.authority, record); err != nil {
		t.Fatal(fmt.Errorf("write desired execution evidence: %w", err))
	}
}

func cloneDesiredExecutionMetadata(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source)+2)
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
