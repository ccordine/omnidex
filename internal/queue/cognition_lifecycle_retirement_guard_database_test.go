package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresCognitionLifecycleRetirementRejectsIdentitySubstitution(t *testing.T) {
	fixture := startLifecycleRetirementFixture(t, "retirement-identity-substitution")
	descriptor := lifecycleCancelDescriptorForTest(t, fixture, "retirement-identity-substitution")
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.Context)
	if _, found, err := loadLifecycleOperationTx(
		fixture.Context, tx, descriptor, fixture.Job.ID,
	); err != nil || found {
		t.Fatalf("reserve lifecycle identity found=%t error=%v", found, err)
	}
	episode, found, err := loadCognitionEpisodeTx(fixture.Context, tx, fixture.EpisodeID, false)
	if err != nil || !found {
		t.Fatalf("load cognition episode found=%t error=%v", found, err)
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(
		fixture.Context, tx, fixture.EpisodeID, false,
	)
	if err != nil || !found {
		t.Fatalf("load cognition graph found=%t error=%v", found, err)
	}
	retirement, err := newCognitionLifecycleRetirement(
		descriptor, episode, graph, cancellationCodeForLifecycleTest(t, descriptor.Kind),
	)
	if err != nil {
		t.Fatal(err)
	}
	forgedIdentity := retirement
	forgedIdentity.ID, forgedIdentity.SHA256 = "", ""
	forgedIdentity.OperationSHA256 = cognitionTestDigest("f")
	identityJSON, identitySHA, err := cognitionJSON(forgedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	forgedDescriptor := retirement
	forgedDescriptor.ID = "cognition_retirement_" + identitySHA
	forgedDescriptor.SHA256 = identitySHA
	descriptorJSON, descriptorJSONSHA, err := cognitionJSON(forgedDescriptor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.Context, `
		INSERT INTO cognition_lifecycle_retirements (
		 retirement_id,retirement_sha256,identity_json,descriptor_json,descriptor_json_sha256,
		 operation_id,operation_kind,operation_sha256,episode_id,job_id,job_generation,
		 step_id,cancellation_code,expected_revision,expected_revision_sha256,
		 graph_version,graph_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`, forgedDescriptor.ID, forgedDescriptor.SHA256, string(identityJSON), string(descriptorJSON),
		descriptorJSONSHA, retirement.OperationID, retirement.OperationKind,
		retirement.OperationSHA256, retirement.EpisodeID, retirement.JobID,
		retirement.JobGeneration, retirement.StepID, retirement.Code,
		int64(retirement.ExpectedRevision.Number), retirement.ExpectedRevision.SHA256,
		int64(retirement.GraphVersion), retirement.GraphSHA256); err == nil {
		t.Fatal("retirement identity substitution was accepted")
	}
}

func TestPostgresCognitionLifecycleSealSetRejectsIdentitySubstitution(t *testing.T) {
	fixture := startLifecycleRetirementFixture(t, "seal-set-identity-substitution")
	descriptor := lifecycleCancelDescriptorForTest(t, fixture, "seal-set-identity-substitution")
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.Context)
	if _, found, err := loadLifecycleOperationTx(
		fixture.Context, tx, descriptor, fixture.Job.ID,
	); err != nil || found {
		t.Fatalf("reserve lifecycle identity found=%t error=%v", found, err)
	}
	set, err := newCognitionLifecycleSealSet(descriptor, fixture.Job.ID, 1, []cognitionLifecycleSealEntry{})
	if err != nil {
		t.Fatal(err)
	}
	forgedIdentity := set
	forgedIdentity.SHA256 = ""
	forgedIdentity.OperationSHA256 = cognitionTestDigest("e")
	identityJSON, identitySHA, err := cognitionJSON(forgedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	forgedSet := set
	forgedSet.SHA256 = identitySHA
	setJSON, setJSONSHA, err := cognitionJSON(forgedSet)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fixture.Context, `
		INSERT INTO cognition_lifecycle_operation_seals (
		 operation_id,operation_kind,operation_sha256,job_id,generation,episode_count,
		 seal_set_json,seal_set_sha256,identity_json,seal_set_json_sha256
		) VALUES ($1,$2,$3,$4,$5,0,$6,$7,$8,$9)
	`, set.OperationID, set.OperationKind, set.OperationSHA256, set.JobID, set.Generation,
		string(setJSON), forgedSet.SHA256, string(identityJSON), setJSONSHA); err == nil {
		t.Fatal("lifecycle seal-set identity substitution was accepted")
	}
}

func TestPostgresCognitionLifecycleRetirementRejectsDuplicateJSONKeys(t *testing.T) {
	fixture := startLifecycleRetirementFixture(t, "retirement-duplicate-json")
	descriptor := lifecycleCancelDescriptorForTest(t, fixture, "retirement-duplicate-json")
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.Context)
	if _, found, err := loadLifecycleOperationTx(
		fixture.Context, tx, descriptor, fixture.Job.ID,
	); err != nil || found {
		t.Fatalf("reserve lifecycle identity found=%t error=%v", found, err)
	}
	episode, found, err := loadCognitionEpisodeTx(fixture.Context, tx, fixture.EpisodeID, false)
	if err != nil || !found {
		t.Fatalf("load cognition episode found=%t error=%v", found, err)
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(
		fixture.Context, tx, fixture.EpisodeID, false,
	)
	if err != nil || !found {
		t.Fatalf("load cognition graph found=%t error=%v", found, err)
	}
	retirement, err := newCognitionLifecycleRetirement(
		descriptor, episode, graph, cancellationCodeForLifecycleTest(t, descriptor.Kind),
	)
	if err != nil {
		t.Fatal(err)
	}
	identity := retirement
	identity.ID, identity.SHA256 = "", ""
	identityJSON, _, err := cognitionJSON(identity)
	if err != nil {
		t.Fatal(err)
	}
	duplicateIdentity := strings.Replace(
		string(identityJSON), "{", `{"schema":"duplicate",`, 1,
	)
	duplicateIdentitySHA := rawCognitionLifecycleTestSHA(duplicateIdentity)
	forgedDescriptor := retirement
	forgedDescriptor.ID = "cognition_retirement_" + duplicateIdentitySHA
	forgedDescriptor.SHA256 = duplicateIdentitySHA
	descriptorJSON, _, err := cognitionJSON(forgedDescriptor)
	if err != nil {
		t.Fatal(err)
	}
	duplicateDescriptor := strings.Replace(
		string(descriptorJSON), "{", `{"schema":"duplicate",`, 1,
	)
	if _, err := tx.Exec(fixture.Context, `
		INSERT INTO cognition_lifecycle_retirements (
		 retirement_id,retirement_sha256,identity_json,descriptor_json,descriptor_json_sha256,
		 operation_id,operation_kind,operation_sha256,episode_id,job_id,job_generation,
		 step_id,cancellation_code,expected_revision,expected_revision_sha256,
		 graph_version,graph_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`, forgedDescriptor.ID, duplicateIdentitySHA, duplicateIdentity, duplicateDescriptor,
		rawCognitionLifecycleTestSHA(duplicateDescriptor), retirement.OperationID,
		retirement.OperationKind, retirement.OperationSHA256, retirement.EpisodeID,
		retirement.JobID, retirement.JobGeneration, retirement.StepID, retirement.Code,
		int64(retirement.ExpectedRevision.Number), retirement.ExpectedRevision.SHA256,
		int64(retirement.GraphVersion), retirement.GraphSHA256); err == nil {
		t.Fatal("duplicate lifecycle descriptor JSON key was accepted")
	}
}

func lifecycleCancelDescriptorForTest(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	label string,
) lifecycleOperationDescriptor {
	t.Helper()
	command, err := normalizeCancelJobCommand(testCancelCommand(
		t, fixture.Job.ID, label, "Exercise direct SQL lifecycle authority.",
	))
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleCancelJob, command)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func cancellationCodeForLifecycleTest(
	t *testing.T,
	kind LifecycleOperationKind,
) cognitionruntime.CancellationCode {
	t.Helper()
	code, _, err := lifecycleCancellationCode(kind)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func rawCognitionLifecycleTestSHA(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
