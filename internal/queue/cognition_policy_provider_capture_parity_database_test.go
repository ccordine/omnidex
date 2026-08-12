package queue

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
)

type captureStartedPolicyJournal struct {
	repository *Repository
	attempt    cognitionpolicy.CallAttempt
	result     cognitionpolicy.CallResult
	evidence   cognitionpolicy.CallEvidence
}

func (journal *captureStartedPolicyJournal) Start(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
) (cognitionpolicy.CallReservation, error) {
	reservation, err := journal.repository.StartCognitionPolicyCall(ctx, attempt)
	if err == nil {
		journal.attempt = attempt
	}
	return reservation, err
}

func (journal *captureStartedPolicyJournal) Finish(
	_ context.Context,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.CallEvidence,
) error {
	journal.attempt = attempt
	journal.result = result
	journal.evidence = evidence.Clone()
	return errors.New("capture terminal policy result before persistence")
}

func TestPostgresProviderCaptureMustDeriveNormalizedCallResult(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	mutations := map[string]func([]byte) []byte{
		"response": mutateCapturedResponse,
		"created_at_missing": func(raw []byte) []byte {
			return removeCapturedJSONField(raw, "created_at")
		},
		"created_at_null": func(raw []byte) []byte {
			return nullCapturedJSONField(raw, "created_at")
		},
		"response_missing": func(raw []byte) []byte {
			return removeCapturedJSONField(raw, "response")
		},
		"response_null": func(raw []byte) []byte {
			return nullCapturedJSONField(raw, "response")
		},
		"created_at_year_zero": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte("2026-08-09T22:00:00Z"), []byte("0000-08-09T22:00:00Z"), 1)
		},
		"created_at_hour_24": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte("2026-08-09T22:00:00Z"), []byte("2026-08-09T24:00:00Z"), 1)
		},
		"created_at_second_60": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte("2026-08-09T22:00:00Z"), []byte("2026-08-09T22:00:60Z"), 1)
		},
		"model": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"model":"`), []byte(`"model":"x`), 1)
		},
		"done": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"done":true`), []byte(`"done":false`), 1)
		},
		"usage": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"eval_count":20`), []byte(`"eval_count":21`), 1)
		},
		"duplicate_key": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`{"model":`), []byte(`{"model":"forged","model":`), 1)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "provider-capture-projection-"+name,
			)
			journal := captureAcceptedPolicyResult(t, fixture)
			forgedResult := journal.result
			forgedEvidence := journal.evidence.Clone()
			capture := mutate(append(
				[]byte(nil), forgedEvidence.ProviderResponseCapture.Content...,
			))
			changedCapture, err := cognitionpolicy.NewProviderResponseCaptureEvidence(
				journal.attempt.ID, capture,
			)
			if err != nil {
				t.Fatal(err)
			}
			forgedEvidence.ProviderResponseCapture = changedCapture
			forgedResult.ProviderResponseCapture = changedCapture.Ref
			forgedResult.ProviderResponseCaptureSHA256 = changedCapture.Ref.SHA256
			forgedResult.ProviderResponseCapturedBytes = changedCapture.Ref.Bytes
			forgedResult.ProviderResponseSHA256 = changedCapture.Ref.SHA256
			forgedResult.ProviderResponseBytes = int64(changedCapture.Ref.Bytes)

			if err := commitDirectPolicyResult(
				fixture, journal.attempt, forgedResult, forgedEvidence,
			); err == nil {
				t.Fatal("direct SQL accepted raw provider bytes that contradict the normalized result")
			}
			var status string
			if err := pool.QueryRow(ctx,
				`SELECT status FROM cognition_policy_calls WHERE call_id=$1`, journal.attempt.ID,
			).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "started" {
				t.Fatalf("forged provider projection changed call status to %q", status)
			}
		})
	}
	legitimate := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "provider-capture-projection-legitimate",
	)
	if err := callCognitionGuardPolicy(
		t, legitimate, legitimate.Start.Budget.RemainingPolicyCalls,
	); err != nil {
		t.Fatalf("exact raw provider projection: %v", err)
	}
}

func captureAcceptedPolicyResult(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
) captureStartedPolicyJournal {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context,
		CognitionRuntimeSnapshotCommand{
			Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	schema := fixture.Start.ActionCatalog.Schemas[0]
	request, err := cognition.NewActionRequest(schema.Kind, []cognition.ActionArgument{})
	if err != nil {
		t.Fatal(err)
	}
	decision := cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID,
		Action:       request, EvidenceRefs: []cognition.EvidenceRef{},
		ExpectedEffect: "Expose bounded public state.",
	}
	response, _, err := cognitionJSON(decision)
	if err != nil {
		t.Fatal(err)
	}
	journal := captureStartedPolicyJournal{repository: fixture.Repository}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: string(response)}, cognitionTestBrain(),
		cognitionGuardActivationAuthority(t, fixture),
		cognitionGuardProjectionLoader{repository: fixture.Repository}, &journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(fixture.Context, prepared.Prepared.Snapshot); err == nil {
		t.Fatal("capture journal unexpectedly persisted the policy result")
	}
	if journal.result.Status != cognitionpolicy.CallResultAccepted ||
		journal.evidence.ProviderResponseCapture.Ref.ID == "" {
		t.Fatalf("captured policy terminal result=%+v", journal.result)
	}
	raw, err := exactjson.Canonical(journal.result)
	if err != nil {
		t.Fatal(err)
	}
	var exact bool
	if err := fixture.Pool.QueryRow(fixture.Context,
		`SELECT cognition_call_result_v3_types_are_exact($1::json)`, string(raw),
	).Scan(&exact); err != nil {
		t.Fatal(err)
	}
	if !exact {
		var shape, attestation, observation, identityRef, captureRef, generationRef bool
		var responseRef, encoding, top, optional, usage, action bool
		if err := fixture.Pool.QueryRow(fixture.Context, `SELECT
			cognition_call_result_v3_shape_is_exact($1),
			cognition_provider_attestation_shape_is_bounded($1::jsonb->'provider_attestation'),
			cognition_provider_observation_shape_is_bounded($1::jsonb->'provider_observation'),
			cognition_policy_evidence_ref_types_are_exact($1::jsonb->'provider_identity_evidence'),
			cognition_policy_evidence_ref_types_are_exact($1::jsonb->'provider_response_capture_evidence'),
			cognition_policy_evidence_ref_types_are_exact($1::jsonb->'provider_generation_evidence'),
			cognition_policy_evidence_ref_types_are_exact($1::jsonb->'response_evidence'),
			cognition_provider_content_encoding_types_are_exact($1::jsonb->'provider_content_encoding'),
			cognition_exact_json_nonnegative_integer($1::jsonb->'provider_http_status',2147483647) AND
			cognition_exact_json_nonnegative_integer($1::jsonb->'provider_response_bytes',9223372036854775807) AND
			cognition_exact_json_nonnegative_integer($1::jsonb->'provider_response_captured_bytes',9223372036854775807) AND
			cognition_exact_json_nonnegative_integer($1::jsonb->'response_bytes',9223372036854775807),
			NOT EXISTS (SELECT 1 FROM jsonb_each($1::jsonb) field WHERE field.key=ANY(ARRAY[
			 'provider_request_sha256','provider_response_disposition','provider_response_sha256',
			 'provider_response_capture_sha256','response_sha256','decision_sha256','failure_code','failure_message'
			]) AND jsonb_typeof(field.value)<>'string'),
			(SELECT bool_and(cognition_exact_json_nonnegative_integer(field.value,9223372036854775807))
			 FROM jsonb_each($1::jsonb->'provider_usage') field),
			jsonb_typeof($1::jsonb->'action_schema')='object' AND
			cognition_json_object_has_exact_keys(($1::jsonb->'action_schema')::json,ARRAY['id','version','sha256'])`,
			string(raw),
		).Scan(&shape, &attestation, &observation, &identityRef, &captureRef,
			&generationRef, &responseRef, &encoding, &top, &optional, &usage, &action); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("legitimate Go call result fails SQL type parity shape/attestation/observation/refs/encoding/top/optional/usage/action=%v/%v/%v/%v,%v,%v,%v/%v/%v/%v/%v/%v: %s",
			shape, attestation, observation, identityRef, captureRef, generationRef,
			responseRef, encoding, top, optional, usage, action, raw)
	}
	return journal
}

func commitDirectPolicyResult(
	fixture taskGenerationRetirementFixture,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.CallEvidence,
) error {
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	authority, err := cognitionPolicyCallAuthority(attempt)
	if err != nil {
		return err
	}
	if err := insertCognitionProviderIdentityEvidenceTx(
		fixture.Context, tx, authority, attempt, result, evidence.ProviderIdentity,
	); err != nil {
		return err
	}
	if err := insertCognitionResponseEvidenceTx(
		fixture.Context, tx, authority, attempt, result, evidence.Response,
	); err != nil {
		return err
	}
	if err := insertCognitionProviderResponseCaptureTx(
		fixture.Context, tx, authority, attempt, result, evidence.ProviderResponseCapture,
	); err != nil {
		return err
	}
	if err := insertCognitionProviderGenerationEvidenceTx(
		fixture.Context, tx, authority, attempt, result, evidence.ProviderGeneration,
	); err != nil {
		return err
	}
	resultJSON, err := exactjson.Canonical(result)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(fixture.Context, `
		UPDATE cognition_policy_calls
		SET status=$2,result_json=$3,result_sha256=$4,finished_at=clock_timestamp()
		WHERE call_id=$1 AND status='started'
	`, attempt.ID, result.Status, string(resultJSON), cognitionPayloadSHA(resultJSON)); err != nil {
		return err
	}
	return tx.Commit(fixture.Context)
}
