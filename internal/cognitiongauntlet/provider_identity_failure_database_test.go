package cognitiongauntlet

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresFullCognitionPersistsProviderIdentityFailuresBeforeEpisodeStart(t *testing.T) {
	for _, test := range []struct {
		name, kind string
		failAt     int
	}{
		{name: "Brain bootstrap", kind: "brain_bootstrap", failAt: 1},
		{name: "provider process", kind: "provider_process", failAt: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
			_, bundle, request := publicFailureFixture(t, ctx, pool, repository, hostStore)
			frozenBrain, err := bundle.Authority.RatGeneration.Fixed.Brain.attestedBrain()
			if err != nil {
				t.Fatal(err)
			}
			episode, err := PublicVariantEpisodeRef(bundle.Authority)
			if err != nil {
				t.Fatal(err)
			}
			first, firstBootstrap := runPublicProviderIdentityFailure(
				t, ctx, bundle, request, test.failAt,
			)
			assertProviderActivationFailureRows(t, pool, episode, 1)
			firstRecord := assertExactProviderFailureEvidence(
				t, repository, request.Attempt, episode, test.kind, first,
				firstBootstrap, frozenBrain,
			)

			second, secondBootstrap := runPublicProviderIdentityFailure(
				t, ctx, bundle, request, test.failAt,
			)
			if second.Ref != first.Ref {
				t.Fatal("identical provider failure replay changed raw evidence identity")
			}
			assertProviderActivationFailureRows(t, pool, episode, 1)
			secondRecord := assertExactProviderFailureEvidence(
				t, repository, request.Attempt, episode, test.kind, second,
				secondBootstrap, frozenBrain,
			)
			if !reflect.DeepEqual(secondRecord, firstRecord) {
				t.Fatalf("identical provider failure replay changed record\nfirst=%+v\nsecond=%+v",
					firstRecord, secondRecord)
			}
		})
	}
}

func runPublicProviderIdentityFailure(
	t *testing.T,
	ctx context.Context,
	bundle PublicInferenceBundle,
	request PublicFullCognitionRunRequest,
	failAt int,
) (llm.ProviderIdentityEvidence, llm.ObservedProviderIdentity) {
	t.Helper()
	witness := &witnessPolicyClient{model: bundle.Authority.RatGeneration.Fixed.Brain.Model}
	client := &providerIdentityFailureClient{witnessPolicyClient: witness, failAt: failAt}
	request.Client = client
	if _, err := RunPublicFullCognition(ctx, bundle, request); err == nil {
		t.Fatal("provider identity failure was not returned loudly")
	}
	if witness.calls() != 0 {
		t.Fatalf("provider identity failure consumed %d policy calls", witness.calls())
	}
	evidence := client.evidence()
	if err := evidence.Validate(); err != nil {
		t.Fatalf("provider failure raw evidence: %v", err)
	}
	return evidence, client.successfulObservation()
}

func assertProviderActivationFailureRows(
	t *testing.T,
	pool *pgxpool.Pool,
	episode cognition.EpisodeRef,
	wantFailures int,
) {
	t.Helper()
	var episodes, policyCalls, failures int
	if err := pool.QueryRow(t.Context(), `
		SELECT
		  (SELECT COUNT(*) FROM cognition_episodes WHERE episode_id=$1),
		  (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		  (SELECT COUNT(*) FROM cognition_provider_activation_failures WHERE episode_id=$1)
	`, episode.ID).Scan(&episodes, &policyCalls, &failures); err != nil {
		t.Fatal(err)
	}
	if episodes != 0 || policyCalls != 0 || failures != wantFailures {
		t.Fatalf("episode/policy/failure=%d/%d/%d", episodes, policyCalls, failures)
	}
}

func assertExactProviderFailureEvidence(
	t *testing.T,
	repository *queue.Repository,
	authority model.StepAttemptAuthority,
	episode cognition.EpisodeRef,
	wantKind string,
	want llm.ProviderIdentityEvidence,
	wantBootstrap llm.ObservedProviderIdentity,
	frozenBrain cognitionpolicy.AttestedBrain,
) queue.CognitionProviderActivationFailureRecord {
	t.Helper()
	page, err := repository.ReadCognitionProviderActivationFailurePage(
		t.Context(), queue.CognitionProviderActivationFailurePageRequest{
			Authority: authority, EpisodeID: episode.ID,
			Limit: queue.MaxCognitionProviderActivationFailurePageSize,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalRecords != 1 || len(page.Records) != 1 ||
		page.NextRecordNumber != page.Records[0].RecordNumber {
		t.Fatalf("provider failure page=%+v", page)
	}
	record := page.Records[0]
	wantActor := cognition.AttemptRef{
		JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		Attempt: uint64(authority.Attempt), WorkerID: authority.WorkerID,
	}
	if record.Kind != wantKind || record.EpisodeID != episode.ID || record.Actor != wantActor ||
		record.RecordID == "" || record.ReceiptSHA256 == "" || record.AuthoritySHA256 == "" ||
		record.Evidence.Ref != want.Ref || len(record.Evidence.Operations) != len(want.Operations) {
		t.Fatalf("provider failure record=%+v", record)
	}
	if wantKind == "brain_bootstrap" && (record.Bootstrap == nil || record.Process != nil) {
		t.Fatalf("Brain bootstrap failure union=%+v/%+v", record.Bootstrap, record.Process)
	}
	if wantKind == "provider_process" && (record.Bootstrap != nil || record.Process == nil) {
		t.Fatalf("provider process failure union=%+v/%+v", record.Bootstrap, record.Process)
	}
	if wantKind == "brain_bootstrap" && (record.SuccessfulBootstrap != nil ||
		record.BootstrapEvidence != nil || wantBootstrap.Evidence.Ref.ID != "") {
		t.Fatal("Brain bootstrap failure invented a prior successful bootstrap")
	}
	if wantKind == "provider_process" {
		if record.SuccessfulBootstrap == nil || record.BootstrapEvidence == nil ||
			wantBootstrap.Evidence.Validate() != nil ||
			record.SuccessfulBootstrap.Ref != frozenBrain.Ref ||
			record.SuccessfulBootstrap.Attestation != wantBootstrap.Attestation ||
			record.SuccessfulBootstrap.BootstrapObservation != wantBootstrap.Observation ||
			record.SuccessfulBootstrap.Host != frozenBrain.Host {
			t.Fatalf("provider process failure lost its exact successful bootstrap: %+v", record)
		}
		assertProviderFailureEvidenceManifest(
			t, repository, authority, episode, record.RecordID,
			*record.BootstrapEvidence, wantBootstrap.Evidence,
		)
	}
	assertProviderFailureEvidenceManifest(
		t, repository, authority, episode, record.RecordID, record.Evidence, want,
	)
	return record
}

func assertProviderFailureEvidenceManifest(
	t *testing.T,
	repository *queue.Repository,
	authority model.StepAttemptAuthority,
	episode cognition.EpisodeRef,
	recordID string,
	manifest queue.CognitionProviderIdentityEvidenceManifest,
	want llm.ProviderIdentityEvidence,
) {
	t.Helper()
	if manifest.Ref != want.Ref || len(manifest.Operations) != len(want.Operations) {
		t.Fatalf("provider failure evidence manifest=%+v", manifest)
	}
	for index, operation := range want.Operations {
		metadata := manifest.Operations[index]
		if metadata.Index != index || metadata.Operation != operation.Operation ||
			metadata.Method != operation.Method || metadata.Endpoint != operation.Endpoint ||
			metadata.RequestDisposition != operation.RequestDisposition ||
			metadata.RequestSHA256 != operation.RequestSHA256 ||
			metadata.RequestBytes != operation.RequestBytes ||
			metadata.HTTPStatus != operation.HTTPStatus ||
			metadata.Disposition != operation.Disposition ||
			metadata.ResponseComplete != operation.ResponseComplete ||
			metadata.ContentEncoding != operation.ContentEncoding ||
			metadata.ResponseSHA256 != operation.ResponseSHA256 ||
			metadata.ResponseBytes != operation.ResponseBytes {
			t.Fatalf("failure evidence operation %d metadata changed: %+v", index, metadata)
		}
		gotRequest := readProviderFailureBody(
			t, repository, authority, episode, recordID, want.Ref.ID, index,
			queue.CognitionProviderIdentityRequestBody,
			operation.RequestSHA256, operation.RequestBytes,
		)
		gotResponse := readProviderFailureBody(
			t, repository, authority, episode, recordID, want.Ref.ID, index,
			queue.CognitionProviderIdentityResponseBody,
			operation.ResponseSHA256, operation.ResponseBytes,
		)
		if !bytes.Equal(gotRequest, operation.Request) ||
			!bytes.Equal(gotResponse, operation.ResponseCapture) {
			t.Fatalf("failure evidence operation %d raw bodies changed", index)
		}
	}
}

func readProviderFailureBody(
	t *testing.T,
	repository *queue.Repository,
	authority model.StepAttemptAuthority,
	episode cognition.EpisodeRef,
	recordID string,
	evidenceID string,
	operation int,
	kind queue.CognitionProviderIdentityBodyKind,
	wantSHA string,
	total int,
) []byte {
	t.Helper()
	result := make([]byte, 0, total)
	for offset := 0; ; {
		page, err := repository.ReadCognitionProviderActivationFailureBody(
			t.Context(), queue.CognitionProviderActivationFailureBodyRequest{
				Authority: authority, EpisodeID: episode.ID, RecordID: recordID,
				EvidenceID: evidenceID, OperationIndex: operation, Kind: kind,
				Offset: offset, Limit: queue.MaxCognitionPolicyEvidencePageBytes,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if page.Ref.ID != evidenceID || page.OperationIndex != operation || page.Kind != kind ||
			page.SHA256 != wantSHA || page.TotalBytes != total || page.Offset != offset {
			t.Fatalf("provider failure evidence page metadata changed: %+v", page)
		}
		result = append(result, page.Content...)
		if page.NextOffset == page.TotalBytes {
			return result
		}
		if page.NextOffset <= offset {
			t.Fatal("provider failure evidence page did not advance")
		}
		offset = page.NextOffset
	}
}
