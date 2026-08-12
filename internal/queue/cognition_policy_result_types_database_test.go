package queue

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
)

func TestPostgresCallResultRejectsGoIncompatibleJSONScalars(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	mutations := map[string]func(cognitionpolicy.CallResult, []byte) []byte{
		"identity_string": replaceResultBytes(
			`"provider_identity_checked":true`, `"provider_identity_checked":"true"`,
		),
		"status_string": replaceResultBytes(
			`"provider_http_status":200`, `"provider_http_status":"200"`,
		),
		"done_string": replaceResultBytes(
			`"provider_done":true`, `"provider_done":"true"`,
		),
		"done_null": replaceResultBytes(
			`"provider_done":true`, `"provider_done":null`,
		),
		"usage_string": replaceResultBytes(
			`"eval_count":20`, `"eval_count":"20"`,
		),
		"response_bytes_string": func(result cognitionpolicy.CallResult, raw []byte) []byte {
			value := fmt.Sprintf(`"response_bytes":%d`, result.ResponseBytes)
			return bytes.Replace(raw, []byte(value), []byte(
				fmt.Sprintf(`"response_bytes":"%d"`, result.ResponseBytes),
			), 1)
		},
		"action_id_129_bytes": func(result cognitionpolicy.CallResult, raw []byte) []byte {
			return bytes.Replace(raw,
				[]byte(fmt.Sprintf(`"id":%q`, result.ActionSchema.ID)),
				[]byte(fmt.Sprintf(`"id":%q`, strings.Repeat("a", 129))), 1,
			)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "result-json-types-"+name,
			)
			journal := captureAcceptedPolicyResult(t, fixture)
			raw, err := exactjson.Canonical(journal.result)
			if err != nil {
				t.Fatal(err)
			}
			forged := mutate(journal.result, append([]byte(nil), raw...))
			if bytes.Equal(forged, raw) {
				t.Fatal("test mutation did not change the result JSON")
			}
			if err := commitDirectPolicyResultJSON(
				fixture, journal.attempt, journal.result, journal.evidence, forged,
			); err == nil {
				t.Fatal("direct SQL accepted a result Go cannot decode or validate")
			}
		})
	}
}

func TestPostgresTerminalResultRejectsMissingOrExtraneousRawReferences(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	for _, test := range []struct {
		name  string
		forge func(captureStartedPolicyJournal) cognitionpolicy.CallResult
	}{
		{"missing_response_capture", func(journal captureStartedPolicyJournal) cognitionpolicy.CallResult {
			result := journal.result
			result.ProviderResponseCapture = cognitionpolicy.ProviderResponseCaptureEvidenceRef{}
			return result
		}},
		{"unchecked_policy_authority_identity", func(journal captureStartedPolicyJournal) cognitionpolicy.CallResult {
			result := zeroFailedResult(journal.attempt, cognitionpolicy.CallFailurePolicyAuthority)
			result.ProviderIdentityEvidence = journal.result.ProviderIdentityEvidence
			return result
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "result-raw-ref-"+test.name,
			)
			journal := captureAcceptedPolicyResult(t, fixture)
			forged := test.forge(journal)
			if err := forged.Validate(journal.attempt); err == nil {
				t.Fatal("Go accepted forged raw evidence reference")
			}
			raw, err := exactjson.Canonical(forged)
			if err != nil {
				t.Fatal(err)
			}
			if err := commitDirectPolicyResultJSON(
				fixture, journal.attempt, journal.result, journal.evidence, raw,
			); err == nil {
				t.Fatal("direct SQL accepted forged raw evidence reference")
			}
		})
	}
}

func replaceResultBytes(oldValue, newValue string) func(
	cognitionpolicy.CallResult, []byte,
) []byte {
	return func(_ cognitionpolicy.CallResult, raw []byte) []byte {
		return bytes.Replace(raw, []byte(oldValue), []byte(newValue), 1)
	}
}

func commitDirectPolicyResultJSON(
	fixture taskGenerationRetirementFixture,
	attempt cognitionpolicy.CallAttempt,
	associationResult cognitionpolicy.CallResult,
	evidence cognitionpolicy.CallEvidence,
	resultJSON []byte,
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
	for _, insert := range []func() error{
		func() error {
			return insertCognitionProviderIdentityEvidenceTx(
				fixture.Context, tx, authority, attempt, associationResult, evidence.ProviderIdentity,
			)
		},
		func() error {
			return insertCognitionResponseEvidenceTx(
				fixture.Context, tx, authority, attempt, associationResult, evidence.Response,
			)
		},
		func() error {
			return insertCognitionProviderResponseCaptureTx(
				fixture.Context, tx, authority, attempt, associationResult,
				evidence.ProviderResponseCapture,
			)
		},
		func() error {
			return insertCognitionProviderGenerationEvidenceTx(
				fixture.Context, tx, authority, attempt, associationResult, evidence.ProviderGeneration,
			)
		},
	} {
		if err := insert(); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(fixture.Context, `UPDATE cognition_policy_calls SET
		status=$2,result_json=$3,result_sha256=$4,finished_at=clock_timestamp()
		WHERE call_id=$1 AND status='started'`,
		attempt.ID, associationResult.Status, string(resultJSON), cognitionPayloadSHA(resultJSON),
	); err != nil {
		return err
	}
	return tx.Commit(fixture.Context)
}
