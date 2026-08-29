package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5"
)

type preparedStationEvidenceFixture struct {
	Authority model.StepAttemptAuthority
	Gap       StationGapOpening
	Call      StationCallOpening
	Receipt   StationCallReceipt
	Result    llm.PreparedGeneration
	Record    LLMCallEvidenceRecord
}

func newStationEvidenceJobForTest(
	t *testing.T,
	exactInstruction string,
) assemblyline.PortableJob {
	t.Helper()
	job, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: exactInstruction,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func prepareSuccessfulStationEvidenceFixture(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	content string,
) preparedStationEvidenceFixture {
	t.Helper()
	gap, prepared, call := openStationEvidenceCallForTest(t, repository, authority, job)
	result := stationCallSuccessWithContent(t, prepared, call, content)
	return prepareStationEvidenceReceiptForTest(
		t, repository, authority, gap, call, result, "",
	)
}

func prepareFailedStationEvidenceFixture(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	partialResponse string,
	errorText string,
) preparedStationEvidenceFixture {
	t.Helper()
	gap, prepared, call := openStationEvidenceCallForTest(t, repository, authority, job)
	result := stationCallTransportFailure(t, prepared, call)
	result.Content = partialResponse
	return prepareStationEvidenceReceiptForTest(
		t, repository, authority, gap, call, result, errorText,
	)
}

func openStationEvidenceCallForTest(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
) (StationGapOpening, llm.PreparedModel, StationCallOpening) {
	t.Helper()
	const contextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.ConversationResponse,
		ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(
			t, job, contextTokens,
		),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	return gap, prepared, call
}

func prepareStationEvidenceReceiptForTest(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	call StationCallOpening,
	result llm.PreparedGeneration,
	errorText string,
) preparedStationEvidenceFixture {
	t.Helper()
	tx, err := repository.pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	receipt, err := insertStationCallReceiptTx(t.Context(), tx, StationCallReceiptRecord{
		Authority: authority, OpeningID: call.ID, GapID: gap.GapID,
		Result: result, Error: errorText,
	}, call)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	record, err := receipt.LLMCallEvidenceRecord(
		authority, gap, call, call.Model, 1, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	return preparedStationEvidenceFixture{
		Authority: authority, Gap: gap, Call: call, Receipt: receipt,
		Result: result, Record: record,
	}
}

func persistPreparedStationEvidenceFixture(
	t *testing.T,
	repository *Repository,
	fixture preparedStationEvidenceFixture,
	contextProjectionID string,
) LLMCallEvidence {
	t.Helper()
	record := fixture.Record
	record.ContextProjectionID = contextProjectionID
	evidence, err := insertLLMCallEvidenceForTest(t.Context(), repository, record)
	if err != nil {
		t.Fatal(err)
	}
	terminal := StationGapTerminalRecord{
		Authority: fixture.Authority, OpeningID: fixture.Gap.ID, GapID: fixture.Gap.GapID,
	}
	if fixture.Receipt.Status == "succeeded" {
		terminal.Status = StationGapResolved
		terminal.Response = fixture.Result.Content
		terminal.Projection = stationGapExactResponseProjection(
			fixture.Receipt.GenerationSHA256, fixture.Result.Content,
		)
	} else {
		terminal.Status = StationGapFailed
		terminal.Error = fixture.Receipt.Error
	}
	if _, err := repository.CloseStationGap(t.Context(), terminal); err != nil {
		t.Fatal(err)
	}
	return evidence
}

func insertLLMCallEvidenceForTest(
	ctx context.Context,
	repository *Repository,
	record LLMCallEvidenceRecord,
) (LLMCallEvidence, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LLMCallEvidence{}, err
	}
	defer tx.Rollback(context.Background())
	evidence, err := insertLLMCallEvidenceTx(ctx, tx, record)
	if err != nil {
		return LLMCallEvidence{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LLMCallEvidence{}, err
	}
	return evidence, nil
}
