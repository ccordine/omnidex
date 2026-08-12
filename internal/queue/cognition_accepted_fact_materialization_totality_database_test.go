package queue

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresCognitionAcceptedFactMaterializationRejectsPrivilegedBatchOmission(t *testing.T) {
	fixture := newCognitionAcceptedFactMaterializationFixture(t, false)
	tx, err := fixture.Database.Repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE cognition_accepted_fact_materializations
		DISABLE TRIGGER cognition_accepted_fact_materializations_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		DELETE FROM cognition_accepted_fact_materializations WHERE materialization_id=$1
	`, fixture.Value.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`); err == nil ||
		!strings.Contains(err.Error(), "lost its accepted-fact materialization batch") {
		t.Fatalf("privileged zero-batch omission error=%v", err)
	}
}

func TestPostgresCognitionAcceptedFactMaterializationRejectsPrivilegedMemberOmission(t *testing.T) {
	fixture := newCognitionAcceptedFactMaterializationFixture(t, true)
	tx, err := fixture.Database.Repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE cognition_accepted_fact_materialization_members
		DISABLE TRIGGER cognition_accepted_fact_materialization_members_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		DELETE FROM cognition_accepted_fact_materialization_members WHERE materialization_id=$1
	`, fixture.Value.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`); err == nil ||
		!strings.Contains(err.Error(), "lost its materialization membership") {
		t.Fatalf("privileged member omission error=%v", err)
	}
}

func TestPostgresCognitionAcceptedFactMaterializationIsReverseCompleteInTerminalTrace(t *testing.T) {
	fixture := newCognitionAcceptedFactMaterializationFixture(t, true)
	evidence, err := cognitionruntime.NewCancellationEvidence(
		cognitionruntime.CancellationPolicyFailure,
		"The bounded cognition policy response was rejected.",
		errors.New("terminal accepted-fact materialization closure"),
	)
	if err != nil {
		t.Fatal(err)
	}
	command := cognitionruntime.CancellationCommand{
		Binding: cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.Database.EpisodeID},
			Attempt: cognitionAttempt(fixture.Database.Authority),
		},
		ExpectedRevision: fixture.Database.Start.Transition.Current,
		Code:             cognitionruntime.CancellationPolicyFailure, SourceEvidence: evidence,
	}
	if _, err := fixture.Database.Repository.CancelCognitionEpisode(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	seen := false
	for offset := 0; ; {
		page, err := fixture.Database.Repository.ReadCognitionSealedTrace(
			t.Context(), fixture.Database.EpisodeID,
			CognitionTracePageRequest{Offset: offset, Limit: MaxCognitionTracePageSize},
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range page.Records {
			if record.Kind == CognitionTraceKindAcceptedFactMaterialization {
				seen = record.ID == fixture.Value.ID &&
					record.SHA256 == cognitionPayloadSHA(fixture.Raw) &&
					record.CallOrdinal == 0 &&
					record.Phase == CognitionAcceptedFactMaterializationInitialTracePhase &&
					record.Sequence == int64(fixture.Value.TransitionRevision)
			}
		}
		if page.NextOffset < 0 {
			break
		}
		offset = page.NextOffset
	}
	if !seen {
		t.Fatal("sealed trace omitted exact initial accepted-fact materialization")
	}
	if _, err := fixture.Database.Repository.pool.Exec(t.Context(), `
		INSERT INTO cognition_accepted_fact_materializations
		SELECT * FROM cognition_accepted_fact_materializations WHERE materialization_id=$1
	`, fixture.Value.ID); err == nil || !strings.Contains(
		err.Error(), "accepted-fact materialization requires an active cognition episode",
	) {
		t.Fatalf("post-seal accepted-fact materialization insert error=%v", err)
	}
	for name, mutation := range map[string]string{
		"omitted":       acceptedFactMaterializationTraceOmissionMutation(),
		"changed-phase": acceptedFactMaterializationTracePhaseMutation(fixture.Value.ID),
	} {
		t.Run(name, func(t *testing.T) {
			err := commitMutatedBootstrapTrace(
				t, fixture.Database.Repository.pool, t.Context(), fixture.Database.EpisodeID, mutation,
			)
			if err == nil || !strings.Contains(
				err.Error(), "terminal trace omitted or forged accepted-fact materialization authority",
			) {
				t.Fatalf("terminal accepted-fact %s error=%v", name, err)
			}
		})
	}
}

func acceptedFactMaterializationTraceOmissionMutation() string {
	return `jsonb_set(saved.trace_json::jsonb,'{records}',(
		SELECT COALESCE(jsonb_agg(record ORDER BY ordinal),'[]'::jsonb)
		FROM jsonb_array_elements(saved.trace_json::jsonb->'records')
		WITH ORDINALITY values_(record,ordinal)
		WHERE record->>'kind'<>'accepted_fact_materialization'))`
}

func acceptedFactMaterializationTracePhaseMutation(id string) string {
	return `jsonb_set(saved.trace_json::jsonb,'{records}',(
		SELECT jsonb_agg(CASE WHEN record->>'kind'='accepted_fact_materialization' AND
			record->>'id'='` + id + `' THEN jsonb_set(record,'{phase}',to_jsonb(12),TRUE)
			ELSE record END ORDER BY ordinal)
		FROM jsonb_array_elements(saved.trace_json::jsonb->'records')
		WITH ORDINALITY values_(record,ordinal)))`
}
