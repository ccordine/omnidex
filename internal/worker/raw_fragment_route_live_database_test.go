package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

type liveRawFragmentProviderCall struct {
	Prepared   llm.PreparedModel
	Generation llm.PreparedGeneration
	Err        error
}

type liveRawFragmentStationClient struct {
	llm.ExactStationClient
	mu    sync.Mutex
	calls []liveRawFragmentProviderCall
}

func newLiveRawFragmentStationClient(
	client llm.ExactStationClient,
) *liveRawFragmentStationClient {
	return &liveRawFragmentStationClient{ExactStationClient: client}
}

func (client *liveRawFragmentStationClient) GeneratePreparedExact(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	generation, err := client.ExactStationClient.GeneratePreparedExact(ctx, prepared)
	client.mu.Lock()
	client.calls = append(client.calls, liveRawFragmentProviderCall{
		Prepared: prepared, Generation: generation, Err: err,
	})
	client.mu.Unlock()
	return generation, err
}

func (client *liveRawFragmentStationClient) callCount() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return len(client.calls)
}

func (client *liveRawFragmentStationClient) callsFrom(
	start int,
) []liveRawFragmentProviderCall {
	client.mu.Lock()
	defer client.mu.Unlock()
	return append([]liveRawFragmentProviderCall(nil), client.calls[start:]...)
}

type liveRawFragmentPersistence struct {
	status, response, responseSHA256, projectionKind   string
	callReceiptSHA256, sourceResponseSHA256            string
	wireRequestSHA256, receiptStatus, generationJSON   string
	generationSHA256, evidenceStatus, evidenceResponse string
	evidenceResponseSHA256                             string
	providerResponseCapture                            []byte
	startByte, endByte                                 int
}

func assertLiveProductionRawFragment(
	t *testing.T,
	pool *pgxpool.Pool,
	jobID int64,
	job assemblyline.PortableJob,
	candidate string,
	wantProjection queue.StationGapProjectionKind,
	calls []liveRawFragmentProviderCall,
) {
	t.Helper()
	if candidate == "" || candidate != strings.TrimSpace(candidate) {
		t.Fatal("qualified raw fragment must be nonempty exact trimmed source")
	}
	if len(calls) != 1 || calls[0].Err != nil {
		t.Fatalf("raw fragment provider calls=%d error=%v, want one successful call", len(calls), callError(calls))
	}
	if calls[0].Generation.Content != candidate {
		t.Fatal("production worker candidate differs from its one exact provider response")
	}
	if calls[0].Prepared.Protocol != llm.ExactPreparedProtocolRawTextV2 ||
		calls[0].Prepared.ContextModel != liveQwenRawFragmentModel ||
		calls[0].Prepared.RawTextStopSequence != llm.ExactPreparedRawChatEndV1 ||
		calls[0].Prepared.ProviderIdentityExpectation == nil ||
		calls[0].Prepared.ProviderIdentityExpectation.TokenizerProfile != llm.ExactPreparedTokenizerProfile {
		t.Fatalf("provider call did not use the registered Qwen 3.5 raw ChatML route: %+v", calls[0].Prepared)
	}
	owned, err := llm.OwnBoundedPreparedGeneration(calls[0].Generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := llm.ValidateExactPreparedGenerationForRequest(calls[0].Prepared, owned); err != nil {
		t.Fatalf("validate production prepared generation: %v", err)
	}
	wireRequestSHA256, err := llm.ExactPreparedRequestSHA256(calls[0].Prepared)
	if err != nil {
		t.Fatal(err)
	}
	var gaps, discoveries, discoveryReceipts, discoveryCaptures int
	var openings, receipts, responseCaptures, identityCaptures, evidence, outcomes int
	if err := pool.QueryRow(t.Context(), `
		SELECT
		 (SELECT count(*) FROM station_gap_openings WHERE job_id=$1 AND work_id=$2),
		 (SELECT count(*) FROM station_provider_discoveries discoveries JOIN station_gap_openings gaps ON gaps.id=discoveries.gap_opening_id WHERE gaps.job_id=$1 AND gaps.work_id=$2),
		 (SELECT count(*) FROM station_provider_discovery_receipts discovery_receipts JOIN station_provider_discoveries discoveries ON discoveries.id=discovery_receipts.opening_id JOIN station_gap_openings gaps ON gaps.id=discoveries.gap_opening_id WHERE gaps.job_id=$1 AND gaps.work_id=$2),
		 (SELECT count(*) FROM station_provider_discovery_captures discovery_captures JOIN station_provider_discoveries discoveries ON discoveries.id=discovery_captures.opening_id JOIN station_gap_openings gaps ON gaps.id=discoveries.gap_opening_id WHERE gaps.job_id=$1 AND gaps.work_id=$2),
		 (SELECT count(*) FROM station_call_openings calls JOIN station_gap_openings gaps ON gaps.id=calls.gap_opening_id WHERE gaps.job_id=$1 AND gaps.work_id=$2),
		 (SELECT count(*) FROM station_call_receipts receipts JOIN station_call_openings calls ON calls.id=receipts.opening_id JOIN station_gap_openings gaps ON gaps.id=calls.gap_opening_id WHERE gaps.job_id=$1 AND gaps.work_id=$2),
		 (SELECT count(*) FROM station_call_response_captures response_captures JOIN station_call_openings calls ON calls.id=response_captures.opening_id JOIN station_gap_openings gaps ON gaps.id=calls.gap_opening_id WHERE gaps.job_id=$1 AND gaps.work_id=$2),
		 (SELECT count(*) FROM station_call_identity_captures identity_captures JOIN station_call_openings calls ON calls.id=identity_captures.opening_id JOIN station_gap_openings gaps ON gaps.id=calls.gap_opening_id WHERE gaps.job_id=$1 AND gaps.work_id=$2),
		 (SELECT count(*) FROM llm_call_evidence evidence JOIN station_call_openings calls ON calls.id=evidence.station_call_opening_id JOIN station_gap_openings gaps ON gaps.id=calls.gap_opening_id WHERE gaps.job_id=$1 AND gaps.work_id=$2),
		 (SELECT count(*) FROM station_gap_outcomes outcomes JOIN station_gap_openings gaps ON gaps.id=outcomes.opening_id WHERE gaps.job_id=$1 AND gaps.work_id=$2)
	`, jobID, job.ID).Scan(
		&gaps, &discoveries, &discoveryReceipts, &discoveryCaptures,
		&openings, &receipts, &responseCaptures, &identityCaptures, &evidence, &outcomes,
	); err != nil {
		t.Fatal(err)
	}
	if gaps != 1 || discoveries != 1 || discoveryReceipts != 1 || discoveryCaptures != 5 ||
		openings != 1 || receipts != 1 || responseCaptures != 1 || identityCaptures != 5 ||
		evidence != 1 || outcomes != 1 {
		t.Fatalf(
			"persisted gap/discovery/discovery-receipt/discovery-capture/call/call-receipt/response-capture/identity-capture/evidence/outcome=%d/%d/%d/%d/%d/%d/%d/%d/%d/%d",
			gaps, discoveries, discoveryReceipts, discoveryCaptures, openings, receipts,
			responseCaptures, identityCaptures, evidence, outcomes,
		)
	}
	var persisted liveRawFragmentPersistence
	if err := pool.QueryRow(t.Context(), `
		SELECT outcomes.status,outcomes.response,outcomes.response_sha256,
		       outcomes.projection_kind,outcomes.call_receipt_sha256,
		       outcomes.source_response_sha256,outcomes.source_start_byte,
		       outcomes.source_end_byte,calls.wire_request_sha256,
		       receipts.status,receipts.generation_json,receipts.generation_sha256,
		       evidence.status,evidence.response,evidence.response_sha256,
		       response_captures.capture
		FROM station_gap_openings gaps
		JOIN station_gap_outcomes outcomes ON outcomes.opening_id=gaps.id
		JOIN station_call_openings calls ON calls.gap_opening_id=gaps.id
		JOIN station_call_receipts receipts ON receipts.opening_id=calls.id
		JOIN station_call_response_captures response_captures ON response_captures.opening_id=calls.id
		JOIN llm_call_evidence evidence ON evidence.station_call_opening_id=calls.id
		WHERE gaps.job_id=$1 AND gaps.work_id=$2
	`, jobID, job.ID).Scan(
		&persisted.status, &persisted.response, &persisted.responseSHA256,
		&persisted.projectionKind, &persisted.callReceiptSHA256,
		&persisted.sourceResponseSHA256, &persisted.startByte, &persisted.endByte,
		&persisted.wireRequestSHA256, &persisted.receiptStatus,
		&persisted.generationJSON, &persisted.generationSHA256,
		&persisted.evidenceStatus, &persisted.evidenceResponse,
		&persisted.evidenceResponseSHA256, &persisted.providerResponseCapture,
	); err != nil {
		t.Fatal(err)
	}
	wantCandidateSHA256 := qualificationSHA256([]byte(candidate))
	discardedBytes := len(candidate) - (persisted.endByte - persisted.startByte)
	if persisted.status != string(queue.StationGapResolved) ||
		persisted.receiptStatus != "succeeded" || persisted.evidenceStatus != "succeeded" ||
		persisted.response != candidate || persisted.evidenceResponse != candidate ||
		persisted.responseSHA256 != wantCandidateSHA256 ||
		persisted.evidenceResponseSHA256 != wantCandidateSHA256 ||
		persisted.sourceResponseSHA256 != persisted.evidenceResponseSHA256 ||
		persisted.sourceResponseSHA256 != wantCandidateSHA256 ||
		persisted.projectionKind != string(wantProjection) || persisted.startByte != 0 ||
		persisted.endByte != len(candidate) || persisted.callReceiptSHA256 != persisted.generationSHA256 ||
		persisted.wireRequestSHA256 != wireRequestSHA256 || discardedBytes != 0 {
		t.Fatalf("persisted raw fragment authority differs from one exact response: %+v", persisted)
	}
	type durableGeneration struct {
		llm.PreparedGeneration
		ProviderIdentityEvidence llm.ProviderIdentityEvidence `json:"provider_identity_evidence"`
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(persisted.generationJSON)))
	decoder.DisallowUnknownFields()
	var decoded durableGeneration
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	durable := decoded.PreparedGeneration
	durable.ProviderIdentityEvidence = decoded.ProviderIdentityEvidence
	durable.ProviderResponseCapture = append([]byte(nil), persisted.providerResponseCapture...)
	identityRows, err := pool.Query(t.Context(), `
		SELECT identity_captures.operation_index,
		       identity_captures.request_capture,
		       identity_captures.response_capture
		FROM station_call_identity_captures identity_captures
		JOIN station_call_openings calls ON calls.id=identity_captures.opening_id
		JOIN station_gap_openings gaps ON gaps.id=calls.gap_opening_id
		WHERE gaps.job_id=$1 AND gaps.work_id=$2
		ORDER BY identity_captures.operation_index
	`, jobID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer identityRows.Close()
	identityIndex := 0
	for identityRows.Next() {
		var operationIndex int
		var requestCapture, responseCapture []byte
		if err := identityRows.Scan(&operationIndex, &requestCapture, &responseCapture); err != nil {
			t.Fatal(err)
		}
		if operationIndex != identityIndex || operationIndex >= len(durable.ProviderIdentityEvidence.Operations) {
			t.Fatalf("persisted identity capture index=%d want %d", operationIndex, identityIndex)
		}
		operation := &durable.ProviderIdentityEvidence.Operations[operationIndex]
		operation.Request = append([]byte(nil), requestCapture...)
		operation.ResponseCapture = append([]byte(nil), responseCapture...)
		identityIndex++
	}
	if err := identityRows.Err(); err != nil {
		t.Fatal(err)
	}
	if identityIndex != 5 {
		t.Fatalf("reconstructed provider identity captures=%d want 5", identityIndex)
	}
	if err := llm.ValidateExactPreparedGenerationForRequest(calls[0].Prepared, durable); err != nil {
		t.Fatalf("validate persisted prepared generation: %v", err)
	}
	if durable.Content != candidate ||
		qualificationSHA256([]byte(persisted.generationJSON)) != persisted.generationSHA256 {
		t.Fatal("persisted generation content or receipt digest differs from accepted candidate")
	}
	t.Logf(
		"qwen_raw_fragment_production_qualification kind=%s request_sha256=%s provider_response_sha256=%s candidate_sha256=%s discarded_bytes=%d",
		job.Kind, wireRequestSHA256, durable.ProviderResponseSHA256, wantCandidateSHA256, discardedBytes,
	)
}

func callError(calls []liveRawFragmentProviderCall) error {
	if len(calls) != 1 {
		return fmt.Errorf("call cardinality differs from one")
	}
	return calls[0].Err
}
