package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

func TestPostgresTerminalResultSemanticsMatchEveryGoFailureCode(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "terminal-result-parity",
	)
	journal := captureAcceptedPolicyResult(t, fixture)
	for _, code := range []cognitionpolicy.CallFailureCode{
		cognitionpolicy.CallFailureResponseLimit,
		cognitionpolicy.CallFailureInvalidDecision,
		cognitionpolicy.CallFailureAuthorityDenied,
		cognitionpolicy.CallFailureProviderUsage,
		cognitionpolicy.CallFailureProviderUsageLimit,
		cognitionpolicy.CallFailureGeneration,
		cognitionpolicy.CallFailureProviderIdentity,
		cognitionpolicy.CallFailureProviderRequest,
		cognitionpolicy.CallFailurePolicyAuthority,
		cognitionpolicy.CallFailureProviderEvidence,
	} {
		code := code
		t.Run(string(code), func(t *testing.T) {
			valid := terminalResultForFailureCode(t, journal, code)
			if err := valid.Validate(journal.attempt); err != nil {
				t.Fatalf("Go rejected registered terminal result: %v", err)
			}
			if !postgresTerminalResultExact(t, fixture, journal.attempt, valid) {
				t.Fatal("PostgreSQL rejected a Go-valid terminal result")
			}
			forged := valid
			forgeTerminalResultForCode(&forged, journal.attempt, code)
			if err := forged.Validate(journal.attempt); err == nil {
				t.Fatal("Go accepted forged terminal result")
			}
			if postgresTerminalResultExact(t, fixture, journal.attempt, forged) {
				t.Fatal("PostgreSQL accepted forged terminal result")
			}
		})
	}
}

func terminalResultForFailureCode(
	t *testing.T,
	journal captureStartedPolicyJournal,
	code cognitionpolicy.CallFailureCode,
) cognitionpolicy.CallResult {
	t.Helper()
	result := journal.result
	result.Status = cognitionpolicy.CallResultRejected
	result.ActionSchema = cognition.ActionSchemaRef{}
	result.DecisionSHA256 = ""
	result.FailureCode = code
	result.FailureMessage = "registered terminal failure"
	switch code {
	case cognitionpolicy.CallFailureResponseLimit:
		result.ProviderDoneReason = "length"
		result.ProviderUsage.EvalCount = journal.attempt.RuntimeBudget.MaxOutputTokens
	case cognitionpolicy.CallFailureInvalidDecision,
		cognitionpolicy.CallFailureAuthorityDenied:
	case cognitionpolicy.CallFailureProviderUsage:
		result.ProviderDonePresent = false
		result.ProviderDone = false
	case cognitionpolicy.CallFailureProviderUsageLimit:
		result.ProviderUsage.PromptEvalCount = journal.attempt.RuntimeBudget.MaxInputTokens + 1
	case cognitionpolicy.CallFailureGeneration:
		result.Status = cognitionpolicy.CallResultFailed
		result.ProviderResponseDisposition = llm.ProviderResponseEmptyContent
		clearTerminalResponse(&result)
	case cognitionpolicy.CallFailureProviderIdentity:
		result = zeroFailedResult(journal.attempt, code)
		result.ProviderIdentityEvidence = journal.result.ProviderIdentityEvidence
	case cognitionpolicy.CallFailureProviderRequest:
		result.Status = cognitionpolicy.CallResultFailed
		result.ProviderRequestSHA256 = strings.Repeat("f", 64)
	case cognitionpolicy.CallFailurePolicyAuthority:
		result.Status = cognitionpolicy.CallResultFailed
	case cognitionpolicy.CallFailureProviderEvidence:
		generation := llm.PreparedGeneration{
			Schema:                     llm.PreparedGenerationSchemaV1,
			ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
		}
		evidence, err := cognitionpolicy.NewProviderGenerationEvidence(
			journal.attempt.ID, generation,
		)
		if err != nil {
			t.Fatal(err)
		}
		result = zeroFailedResult(journal.attempt, code)
		result.ProviderGenerationEvidence = evidence.Ref
	default:
		t.Fatalf("unregistered test failure code %q", code)
	}
	return result
}

func zeroFailedResult(
	attempt cognitionpolicy.CallAttempt,
	code cognitionpolicy.CallFailureCode,
) cognitionpolicy.CallResult {
	return cognitionpolicy.CallResult{
		Schema: cognitionpolicy.CallResultSchemaV3, CallID: attempt.ID,
		Status:                     cognitionpolicy.CallResultFailed,
		ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
		FailureCode:                code, FailureMessage: "registered terminal failure",
	}
}

func clearTerminalResponse(result *cognitionpolicy.CallResult) {
	result.ResponseSHA256 = ""
	result.ResponseBytes = 0
	result.ResponseEvidence = cognitionpolicy.ModelResponseEvidenceRef{}
}

func forgeTerminalResultForCode(
	result *cognitionpolicy.CallResult,
	attempt cognitionpolicy.CallAttempt,
	code cognitionpolicy.CallFailureCode,
) {
	switch code {
	case cognitionpolicy.CallFailureResponseLimit:
		result.ProviderUsage.EvalCount--
	case cognitionpolicy.CallFailureInvalidDecision,
		cognitionpolicy.CallFailureAuthorityDenied:
		result.ProviderDoneReason = "length"
	case cognitionpolicy.CallFailureProviderUsage:
		result.ProviderDonePresent = true
		result.ProviderDone = true
	case cognitionpolicy.CallFailureProviderUsageLimit:
		result.ProviderUsage.PromptEvalCount = attempt.RuntimeBudget.MaxInputTokens
	case cognitionpolicy.CallFailureGeneration:
		result.ProviderResponseDisposition = llm.ProviderResponseSucceeded
	case cognitionpolicy.CallFailureProviderIdentity:
		result.ProviderIdentityEvidence = llm.ProviderIdentityEvidenceRef{}
	case cognitionpolicy.CallFailureProviderRequest:
		result.ProviderRequestSHA256 = attempt.ExpectedProviderRequestSHA256
	case cognitionpolicy.CallFailurePolicyAuthority:
		result.ProviderIdentityChecked = false
	case cognitionpolicy.CallFailureProviderEvidence:
		result.ProviderGenerationEvidence = cognitionpolicy.ProviderGenerationEvidenceRef{}
	}
}

func postgresTerminalResultExact(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
) bool {
	t.Helper()
	resultJSON, err := exactjson.Canonical(result)
	if err != nil {
		t.Fatal(err)
	}
	budgetJSON, err := exactjson.Canonical(attempt.RuntimeBudget)
	if err != nil {
		t.Fatal(err)
	}
	brainJSON, err := exactjson.Canonical(attempt.Brain)
	if err != nil {
		t.Fatal(err)
	}
	var exact bool
	if err := fixture.Pool.QueryRow(fixture.Context, `SELECT
		cognition_call_result_v3_types_are_exact($1::json) AND
		cognition_policy_terminal_result_is_exact($1::jsonb,$2::jsonb,$3::jsonb,$4)`,
		string(resultJSON), string(budgetJSON), string(brainJSON),
		attempt.ExpectedProviderRequestSHA256,
	).Scan(&exact); err != nil {
		t.Fatal(err)
	}
	return exact
}
