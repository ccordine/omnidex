package cognitiongauntlet

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This is contaminated world-validation machinery. It proves that two
// unrelated public surfaces drive the same code-owned fact path; the witness
// policy is never competence evidence.
func TestPostgresFullCognitionProjectsPublicFactsAcrossUnrelatedSurfaces(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	generation := mustRatGeneration(t)
	tests := []struct {
		name    string
		spec    int
		surface Surface
	}{
		{name: "retrieve-symbolic", spec: 0, surface: SurfaceSymbolic},
		{name: "recall-record", spec: 1, surface: SurfaceRecord},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[test.spec])
			if err != nil {
				t.Fatal(err)
			}
			claim := claimScaleStep(t, repository, fixture.spec.Budget.WorkingSetBytes, index)
			result, err := RunFullCognition(ctx, fixture, FullCognitionRunRequest{
				Surface: test.surface, RatGeneration: generation,
				RuntimeFingerprint: transferTestFingerprint(), Repetition: 1,
				Attempt: claim, Pool: pool, HostStore: hostStore,
				Client: &witnessPolicyClient{
					model:        generation.Fixed.Brain.Model,
					witness:      fixture.generated.PrivateOracle().Witness,
					evidenceUses: fixture.generated.PrivateOracle().EvidenceUses,
				},
				EpisodeSealPath:     filepath.Join(t.TempDir(), "episode.json"),
				EvaluationPath:      filepath.Join(t.TempDir(), "evaluation.json"),
				LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
				ProjectionPolicyVersion: "context-projection.v1",
			})
			if err != nil {
				t.Fatal(err)
			}
			assertProjectedPublicFact(t, pool, repository, result.Episode.Manifest.EpisodeID)
		})
	}
}

type persistedVisibleFact struct {
	Content       string
	ContentSHA256 string
	Source        cognition.EvidenceRef
}

func assertProjectedPublicFact(
	t *testing.T,
	pool *pgxpool.Pool,
	reader sealedTraceReader,
	episodeID cognition.EpisodeID,
) {
	t.Helper()
	facts := readPersistedVisibleFacts(t, pool, episodeID)
	trace, err := readProductionTrace(t.Context(), reader, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	target := facts[0]
	snapshots := traceSnapshotByCall(t, trace)
	factCalls := make(map[int64]struct{})
	sourceRaw, laterFact, envelopeFact := false, false, false
	for _, record := range trace.Records {
		switch record.Kind {
		case "context_projection":
			projection := contextbuilder.Projection{}
			if err := decodeProductionPayload(record.Payload, &projection, "fact projection"); err != nil {
				t.Fatal(err)
			}
			snapshot, exists := snapshots[record.CallOrdinal]
			if !exists {
				t.Fatalf("projection call %d has no sealed runtime snapshot", record.CallOrdinal)
			}
			rawSelected := projectionContainsFactSource(projection, target)
			factSelected := projectionContainsVisibleFact(t, projection, target)
			switch {
			case snapshot.CurrentRevision.Number == target.Source.Revision.Number:
				sourceRaw = sourceRaw || rawSelected
				if factSelected {
					t.Fatal("same-revision accepted fact duplicated its raw source in model context")
				}
			case snapshot.CurrentRevision.Number > target.Source.Revision.Number && factSelected:
				if rawSelected {
					t.Fatal("historical raw evidence remained beside its complete accepted fact")
				}
				if !containsEvidenceRef(snapshot.EvidenceRefs, target.Source) {
					t.Fatal("projected fact lineage was absent from the model decision evidence packet")
				}
				laterFact = true
				factCalls[record.CallOrdinal] = struct{}{}
			}
		case "policy_attempt":
			attempt := cognitionpolicy.CallAttempt{}
			if err := decodeProductionPayload(record.Payload, &attempt, "fact policy attempt"); err != nil {
				t.Fatal(err)
			}
			assertNoPrivateModelVocabulary(t, attempt.Envelope)
			if _, factCall := factCalls[record.CallOrdinal]; factCall {
				envelopeFact = envelopeFact || strings.Contains(attempt.Envelope, visibleFactSchemaV1)
			}
		}
	}
	if !sourceRaw || !laterFact || !envelopeFact {
		t.Fatalf("accepted public fact source_raw=%t later_fact=%t model_envelope=%t facts=%d",
			sourceRaw, laterFact, envelopeFact, len(facts))
	}
}

func readPersistedVisibleFacts(
	t *testing.T,
	pool *pgxpool.Pool,
	episodeID cognition.EpisodeID,
) []persistedVisibleFact {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT entries.content,entries.content_sha256,evidence.observation_id,
		       evidence.revision,evidence.revision_sha256,evidence.content_sha256
		FROM cognition_accepted_facts facts
		JOIN task_entries entries ON entries.ledger_id=facts.ledger_id AND entries.id=facts.entry_id
		JOIN cognition_accepted_fact_evidence evidence ON evidence.fact_id=facts.fact_id
		WHERE facts.episode_id=$1 ORDER BY evidence.revision,facts.fact_id,evidence.position
	`, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := make([]persistedVisibleFact, 0)
	for rows.Next() {
		var fact persistedVisibleFact
		var observation string
		var revision int64
		if err := rows.Scan(&fact.Content, &fact.ContentSHA256, &observation, &revision,
			&fact.Source.Revision.SHA256, &fact.Source.SHA256); err != nil {
			t.Fatal(err)
		}
		fact.Source.ObservationID = cognition.ObservationID(observation)
		fact.Source.Revision.EpisodeID = episodeID
		fact.Source.Revision.Number = uint64(revision)
		assertVisibleFactContent(t, fact)
		result = append(result, fact)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(result) == 0 {
		t.Fatal("production episode persisted no accepted public fact")
	}
	return result
}

func assertVisibleFactContent(t *testing.T, persisted persistedVisibleFact) {
	t.Helper()
	fact := visibleRecordFact{}
	if err := decodeStrictJSON([]byte(persisted.Content), &fact, "persisted visible fact"); err != nil ||
		fact.Schema != visibleFactSchemaV1 || fact.Source != persisted.Source || len(fact.Records) == 0 {
		t.Fatalf("persisted visible fact=%+v error=%v", fact, err)
	}
	for _, record := range fact.Records {
		if record.Content == "" || visibleFactDigest(record.Content) != record.ContentSHA256 {
			t.Fatalf("persisted visible fact lost bounded content: %+v", record)
		}
	}
	assertNoPrivateModelVocabulary(t, persisted.Content)
}

func projectionContainsVisibleFact(
	t *testing.T,
	projection contextbuilder.Projection,
	fact persistedVisibleFact,
) bool {
	t.Helper()
	expected := visibleFactSourceTaskRef(fact)
	for _, selected := range projection.Selected {
		if selected.Role != workingset.RoleFact || selected.ContentSHA256 != fact.ContentSHA256 {
			continue
		}
		if len(selected.SourceRefs) != 1 || selected.SourceRefs[0] != expected {
			t.Fatalf("projected fact source refs=%+v want %+v", selected.SourceRefs, expected)
		}
		return true
	}
	return false
}

func projectionContainsFactSource(projection contextbuilder.Projection, fact persistedVisibleFact) bool {
	expected := visibleFactSourceTaskRef(fact)
	for _, selected := range projection.Selected {
		if selected.Role == workingset.RoleEvidence && selected.Ref == expected {
			return true
		}
	}
	return false
}

func visibleFactSourceTaskRef(fact persistedVisibleFact) taskstate.Ref {
	return taskstate.Ref{
		URI: "cognition:episode/" + string(fact.Source.Revision.EpisodeID) + "/observation/" +
			string(fact.Source.ObservationID),
		Version: fmt.Sprint(fact.Source.Revision.Number), Hash: fact.Source.SHA256,
		Relation: taskstate.RefEvidence,
	}
}

func containsEvidenceRef(refs []cognition.EvidenceRef, expected cognition.EvidenceRef) bool {
	for _, ref := range refs {
		if ref == expected {
			return true
		}
	}
	return false
}
