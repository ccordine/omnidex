package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

const fragmentGenerationOutputLimitReplacementMigration = "184_fragment_generation_output_limit_replacement.sql"
const fragmentGenerationOutputLimitReplacementAuthorityMigration = "184_fragment_generation_output_limit_replacement_authority.sql"

func TestFragmentGenerationOutputLimitReplacementMigrationPreservesHistoryAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + fragmentGenerationOutputLimitReplacementMigration)
	if err != nil {
		t.Fatal(err)
	}
	authorityRaw, err := os.ReadFile(
		"../../migrations/" + fragmentGenerationOutputLimitReplacementAuthorityMigration,
	)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw) + string(authorityRaw)
	for _, required := range []string{
		"'fragment_generation_replacement'",
		"scope='portable_fragment_worker'",
		"station='coding_fragment'",
		"gap_payload->'original'->>'language'",
		"0295101bc9f22439463b3054efb15a715fcd1ee02fcfc3df8a69b903f595814a",
		"6e03d3f28a47eae720644b268139ffc85a832197ad77608a31e5a3f2f6c66fed",
		"9ffb069bb0a14804df717b9cd918e167c3bfc88eede8f3cd744b39c1715ff303",
		"fragment_generation_replacement_authority_is_exact",
		"43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae",
		"replacement_json_nonnegative_integer_is_exact",
		"f57834d6b2254d72f43e31c2e2538561e67a3c4c96007f300395670c04a741e8",
		"origin_gap_opening_id", "origin_call_receipt_id",
		"llm_call_evidence_truncate_immutable",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("output-limit replacement migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"DELETE FROM", "TRUNCATE station_", "TRUNCATE llm_call_",
		"fresh reset", "render-portable-job.v6",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("output-limit replacement migration contains forbidden history cutover %q", forbidden)
		}
	}
}

func TestPostgresFragmentGenerationReplacementProjectionAuthority(t *testing.T) {
	for _, fixture := range []struct {
		name       string
		language   string
		dialect    string
		signature  string
		behavior   string
		source     string
		validKind  StationGapProjectionKind
		forgedKind StationGapProjectionKind
	}{
		{
			name: "Go source declaration", language: "go", dialect: "Go 1.24",
			signature: "func Ready() bool", behavior: "Return whether the operation is ready.",
			source:     "func Ready() bool { return true }",
			validKind:  StationGapProjectionSourceDeclaration,
			forgedKind: StationGapProjectionTypeScriptFunction,
		},
		{
			name: "TypeScript function", language: "typescript", dialect: "TypeScript 5.9.3",
			signature: "function ready(): boolean", behavior: "Return whether the operation is ready.",
			source:     "function ready(): boolean { return true; }",
			validKind:  StationGapProjectionTypeScriptFunction,
			forgedKind: StationGapProjectionSourceDeclaration,
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(
				t.Context(), loadCheckedMigrationBundle(t),
			); err != nil {
				t.Fatal(err)
			}
			claim := claimStationTestJob(t, repository, "replacement-projection-"+fixture.language)
			original := assemblyline.FragmentGenerationInput{
				Language: fixture.language, Dialect: fixture.dialect,
				Signature: fixture.signature, Behavior: fixture.behavior,
			}
			origin, err := assemblyline.NewFragmentGenerationJob(original)
			if err != nil {
				t.Fatal(err)
			}
			originGap, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
				Authority: claim.Authority, Job: origin, Station: station.CodingFragment,
				ContextTokens: 32768, MaxOutputTokens: 32768,
				OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
			})
			if err != nil {
				t.Fatal(err)
			}
			originPrepared := stationOutputProjectionTestPrepared(t, originGap)
			originDiscovery := persistStationDiscoverySuccess(
				t, repository, claim.Authority, originGap, originPrepared,
			)
			originCall, err := repository.OpenStationCall(
				t.Context(), StationCallOpenRecord{
					Authority: claim.Authority, Gap: originGap,
					Discovery: originDiscovery, Prepared: originPrepared,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			const rejectedPrefix = "partial declaration rejected at exact output limit"
			originResult := stationCallOutputLimitWithContent(
				t, originPrepared, originCall, rejectedPrefix,
			)
			originReceipt, err := repository.RecordStationCallReceiptAndEvidence(
				t.Context(), StationCallReceiptEvidenceRecord{
					Receipt: StationCallReceiptRecord{
						Authority: claim.Authority, OpeningID: originCall.ID,
						GapID: originGap.GapID, Result: originResult,
						Error: "exact station provider call ended with done_reason=length",
					},
					RequestedModel:  originPrepared.ContextModel,
					EvidenceAttempt: 1, LatencyMS: 1,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repository.CloseStationGap(
				t.Context(), StationGapTerminalRecord{
					Authority: claim.Authority, OpeningID: originGap.ID,
					GapID: originGap.GapID, Status: StationGapFailed,
					Error: "exact station provider call ended with done_reason=length",
				},
			); err != nil {
				t.Fatal(err)
			}
			job, err := assemblyline.NewFragmentGenerationReplacementJob(
				assemblyline.FragmentGenerationReplacementInput{
					Original: original,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repository.OpenStationGapDiscovery(
				t.Context(), StationGapDiscoveryOpenRecord{
					Gap: StationGapOpenRecord{
						Authority: claim.Authority, Job: job, Station: station.CodingFragment,
						ContextTokens: 32768, MaxOutputTokens: 32768,
						OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
						ReplacementOrigin: &StationGapReplacementOrigin{
							GapOpeningID:  originGap.ID,
							CallReceiptID: originReceipt.Receipt.ID + 1,
						},
					},
					Selection: llm.ProviderIdentitySelection{
						Model: originPrepared.ContextModel, NativeContextLimit: 32768,
					},
				},
			); err == nil || !strings.Contains(
				err.Error(), "exact persisted failed output-limit origin",
			) {
				t.Fatalf("forged replacement origin error=%v", err)
			}
			if _, err := repository.OpenStationGapDiscovery(
				t.Context(), StationGapDiscoveryOpenRecord{
					Gap: StationGapOpenRecord{
						Authority: claim.Authority, Job: job, Station: station.CodingFragment,
						ContextTokens: 16384, MaxOutputTokens: 16384,
						OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
						ReplacementOrigin: &StationGapReplacementOrigin{
							GapOpeningID:  originGap.ID,
							CallReceiptID: originReceipt.Receipt.ID,
						},
					},
					Selection: llm.ProviderIdentitySelection{
						Model: originPrepared.ContextModel, NativeContextLimit: 16384,
					},
				},
			); err == nil || !strings.Contains(
				err.Error(), "exact persisted failed output-limit origin",
			) {
				t.Fatalf("replacement budget drift error=%v", err)
			}
			replacementRecord := StationGapOpenRecord{
				Authority: claim.Authority, Job: job, Station: station.CodingFragment,
				ContextTokens: 32768, MaxOutputTokens: 32768,
				OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
				ReplacementOrigin: &StationGapReplacementOrigin{
					GapOpeningID:  originGap.ID,
					CallReceiptID: originReceipt.Receipt.ID,
				},
			}
			if _, err := repository.OpenStationGapDiscovery(
				t.Context(), StationGapDiscoveryOpenRecord{
					Gap: replacementRecord,
					Selection: llm.ProviderIdentitySelection{
						Model: "different-model:9b", NativeContextLimit: 32768,
					},
				},
			); err == nil || !strings.Contains(
				err.Error(), "exact origin provider model",
			) {
				t.Fatalf("replacement model drift error=%v", err)
			}
			opened, err := repository.OpenStationGapDiscovery(
				t.Context(), StationGapDiscoveryOpenRecord{
					Gap: replacementRecord,
					Selection: llm.ProviderIdentitySelection{
						Model: originPrepared.ContextModel, NativeContextLimit: 32768,
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			gap := opened.Gap
			prepared := stationOutputProjectionTestPrepared(t, gap)
			discovery := persistOpenedStationDiscoverySuccess(
				t, repository, claim.Authority, gap, opened.Discovery, prepared,
			)
			call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
				Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
			})
			if err != nil {
				t.Fatal(err)
			}
			result := stationCallSuccessWithContent(t, prepared, call, fixture.source)
			receiptEvidence, err := repository.RecordStationCallReceiptAndEvidence(
				t.Context(), StationCallReceiptEvidenceRecord{
					Receipt: StationCallReceiptRecord{
						Authority: claim.Authority, OpeningID: call.ID,
						GapID: gap.GapID, Result: result,
					},
					RequestedModel: prepared.ContextModel, EvidenceAttempt: 1, LatencyMS: 1,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			projection := &StationGapSourceProjection{
				Kind:                 fixture.validKind,
				CallReceiptSHA256:    receiptEvidence.Receipt.GenerationSHA256,
				SourceResponseSHA256: receiptEvidence.Evidence.ResponseSHA256,
				StartByte:            0, EndByte: len(fixture.source),
			}
			forged := *projection
			forged.Kind = fixture.forgedKind
			if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
				Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
				Status: StationGapResolved, Response: fixture.source, Projection: &forged,
			}); err == nil || !strings.Contains(err.Error(), "projection differs") {
				t.Fatalf("mismatched replacement projection error=%v", err)
			}
			outcome, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
				Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
				Status: StationGapResolved, Response: fixture.source, Projection: projection,
			})
			if err != nil {
				t.Fatal(err)
			}
			if outcome.ProjectionKind != fixture.validKind ||
				outcome.Response != fixture.source {
				t.Fatalf("replacement outcome=%+v", outcome)
			}
			attemptEvidence, err := repository.StationAttemptCallEvidence(
				t.Context(), claim.Authority,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(attemptEvidence) != 2 || attemptEvidence[0].Response != "" ||
				attemptEvidence[1].Response != fixture.source ||
				strings.Contains(attemptEvidence[0].Response, rejectedPrefix) {
				t.Fatalf(
					"replacement attempt evidence exposed rejected source or lost call accounting: %+v",
					attemptEvidence,
				)
			}
		})
	}
}
