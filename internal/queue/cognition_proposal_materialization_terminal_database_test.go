package queue

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresCognitionProposalMaterializationIsReverseCompleteInTerminalTrace(t *testing.T) {
	fixture, raw := cognitionProposalMaterializationFixture(t)
	materialization, err := DecodeCognitionProposalMaterialization(raw, cognitionPayloadSHA(raw))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := cognitionruntime.NewCancellationEvidence(
		cognitionruntime.CancellationPolicyFailure,
		"The bounded cognition policy response was rejected.",
		errors.New("terminal proposal materialization closure"),
	)
	if err != nil {
		t.Fatal(err)
	}
	command := cognitionruntime.CancellationCommand{
		Binding: cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
			Attempt: cognitionAttempt(fixture.Authority),
		},
		ExpectedRevision: fixture.Start.Transition.Current,
		Code:             cognitionruntime.CancellationPolicyFailure,
		SourceEvidence:   evidence,
	}
	if _, err := fixture.Repository.CancelCognitionEpisode(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	page, err := fixture.Repository.ReadCognitionSealedTrace(
		t.Context(), fixture.EpisodeID,
		CognitionTracePageRequest{Limit: MaxCognitionTracePageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, record := range page.Records {
		if record.Kind == CognitionTraceKindProposalMaterialization {
			seen = record.ID == materialization.ID &&
				record.SHA256 == cognitionPayloadSHA(raw) &&
				record.CallOrdinal == int64(materialization.CallOrdinal) &&
				record.Sequence == int64(materialization.ProposalIndex)
		}
	}
	if !seen {
		t.Fatalf("sealed trace omitted exact proposal materialization %q", materialization.ID)
	}
	if _, err := fixture.Repository.pool.Exec(t.Context(), `
		INSERT INTO cognition_proposal_materializations
		SELECT * FROM cognition_proposal_materializations WHERE materialization_id=$1
	`, materialization.ID); err == nil || !strings.Contains(
		err.Error(), "proposal materialization requires an active cognition episode",
	) {
		t.Fatalf("post-seal proposal materialization insert error=%v", err)
	}
	err = commitMutatedBootstrapTrace(
		t, fixture.Repository.pool, t.Context(), fixture.EpisodeID,
		proposalMaterializationTraceOmissionMutation(),
	)
	if err == nil || !strings.Contains(
		err.Error(), "terminal trace omitted or forged proposal materialization authority",
	) {
		t.Fatalf("proposal materialization omission commit error=%v", err)
	}
	for name, mutation := range map[string]string{
		"extra": proposalMaterializationTraceExtraMutation(materialization),
		"reordered": proposalMaterializationTraceFieldMutation(
			materialization.ID, "sequence", "to_jsonb(31)",
		),
	} {
		t.Run(name, func(t *testing.T) {
			err := commitMutatedBootstrapTrace(
				t, fixture.Repository.pool, t.Context(), fixture.EpisodeID, mutation,
			)
			if err == nil || !strings.Contains(
				err.Error(), "terminal trace omitted or forged proposal materialization authority",
			) {
				t.Fatalf("proposal materialization %s commit error=%v", name, err)
			}
		})
	}
}

func proposalMaterializationTraceOmissionMutation() string {
	return `jsonb_set(saved.trace_json::jsonb,'{records}',(
		SELECT COALESCE(jsonb_agg(record ORDER BY ordinal),'[]'::jsonb)
		FROM jsonb_array_elements(saved.trace_json::jsonb->'records')
		WITH ORDINALITY values_(record,ordinal)
		WHERE record->>'kind'<>'proposal_materialization'))`
}

func proposalMaterializationTraceFieldMutation(id, field, valueSQL string) string {
	return `jsonb_set(saved.trace_json::jsonb,'{records}',(
		SELECT jsonb_agg(CASE WHEN record->>'kind'='proposal_materialization' AND
			record->>'id'='` + id + `' THEN jsonb_set(record,'{` + field + `}',` + valueSQL + `,TRUE)
			ELSE record END ORDER BY ordinal)
		FROM jsonb_array_elements(saved.trace_json::jsonb->'records')
		WITH ORDINALITY values_(record,ordinal)))`
}

func proposalMaterializationTraceExtraMutation(value CognitionProposalMaterialization) string {
	return `jsonb_set(saved.trace_json::jsonb,'{records}',
		saved.trace_json::jsonb->'records'||jsonb_build_array(jsonb_build_object(
			'kind','proposal_materialization','call_ordinal',` +
		fmt.Sprint(value.CallOrdinal) + `,'phase',42,'sequence',31,
			'id','` + value.ID + `-extra','sha256','` + value.SHA256 + `')))`
}
