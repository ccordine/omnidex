package queue

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/taskstate"
)

type cognitionAcceptedFactMaterializationFixture struct {
	Database  cognitionDatabaseFixture
	Authority cognitionstate.FactAcceptanceAuthority
	Value     CognitionAcceptedFactMaterialization
	Raw       []byte
}

func TestPostgresCognitionAcceptedFactMaterializationPortableVerifierOwnsZeroAndNonzero(t *testing.T) {
	for _, withFact := range []bool{false, true} {
		t.Run(fmt.Sprintf("fact=%t", withFact), func(t *testing.T) {
			fixture := newCognitionAcceptedFactMaterializationFixture(t, withFact)
			if got, want := len(fixture.Value.Members), boolInt(withFact); got != want {
				t.Fatalf("accepted-fact members=%d want=%d", got, want)
			}
			payloadSHA := cognitionPayloadSHA(fixture.Raw)
			if payloadSHA == fixture.Value.SHA256 {
				t.Fatal("full payload SHA unexpectedly equals internal identity SHA")
			}
			trace := CognitionAcceptedFactMaterializationTraceAuthority{
				TransitionID:     fixture.Value.TransitionID,
				TransitionSHA256: fixture.Value.TransitionSHA256,
				CallOrdinal:      fixture.Value.CallOrdinal,
				Phase:            CognitionAcceptedFactMaterializationInitialTracePhase,
				Sequence:         int64(fixture.Value.TransitionRevision),
				ID:               fixture.Value.ID, SHA256: payloadSHA,
			}
			if err := VerifyCognitionAcceptedFactMaterializationTrace(
				fixture.Value, trace, fixture.Database.Start.Transition, fixture.Authority,
			); err != nil {
				t.Fatalf("verify exact accepted-fact materialization: %v", err)
			}
			trace.SHA256 = fixture.Value.SHA256
			if err := VerifyCognitionAcceptedFactMaterializationTrace(
				fixture.Value, trace, fixture.Database.Start.Transition, fixture.Authority,
			); err == nil {
				t.Fatal("trace verifier accepted internal identity SHA as payload SHA")
			}
		})
	}
}

func TestPostgresCognitionAcceptedFactMaterializationRejectsPortableForgeries(t *testing.T) {
	fixture := newCognitionAcceptedFactMaterializationFixture(t, true)
	tests := map[string]func(*CognitionAcceptedFactMaterialization){
		"transition": func(value *CognitionAcceptedFactMaterialization) {
			value.TransitionSHA256 = cognitionTestDigest("c")
		},
		"authority": func(value *CognitionAcceptedFactMaterialization) {
			value.FactAuthority.Planner.SHA256 = cognitionTestDigest("d")
		},
		"pre-ledger": func(value *CognitionAcceptedFactMaterialization) {
			value.PreFactLedger.Version++
		},
		"member command": func(value *CognitionAcceptedFactMaterialization) {
			value.Members[0].Command.Content = "forged accepted fact"
		},
		"member order": func(value *CognitionAcceptedFactMaterialization) {
			value.Members[0].Index++
		},
		"entry URI": func(value *CognitionAcceptedFactMaterialization) {
			value.Members[0].EntryURI += "-forged"
		},
		"output version": func(value *CognitionAcceptedFactMaterialization) {
			value.OutputLedgerVersion++
		},
		"call bigint": func(value *CognitionAcceptedFactMaterialization) {
			value.CallOrdinal = math.MaxInt64 + 1
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			forged := fixture.Value
			forged.Members = append([]CognitionAcceptedFactMaterializationMember(nil), forged.Members...)
			mutate(&forged)
			rehashAcceptedFactMaterialization(t, &forged)
			raw, err := exactjson.Canonical(forged)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeCognitionAcceptedFactMaterialization(
				raw, cognitionPayloadSHA(raw),
			); err == nil {
				t.Fatal("self-consistently rehashed accepted-fact forgery was accepted")
			}
		})
	}
	changedAuthority := cognitionFactAuthorityForTest(
		t, planFirstCognitionObservation, cognitionTestDigest("e"),
	)
	if err := VerifyCognitionAcceptedFactMaterialization(
		fixture.Value, fixture.Database.Start.Transition, changedAuthority,
	); err == nil {
		t.Fatal("portable verifier accepted changed executable fact authority")
	}
}

func TestPostgresCognitionAcceptedFactProjectionBindsExactEntryAndSources(t *testing.T) {
	fixture := newCognitionAcceptedFactMaterializationFixture(t, true)
	member := fixture.Value.Members[0]
	selected := taskstate.Ref{
		URI: member.EntryURI, Version: fmt.Sprint(member.OutputLedgerVersion),
		Hash: taskLedgerTestSHA256(member.Command.Content), Relation: taskstate.RefSource,
	}
	evidence, err := ResolveCognitionAcceptedFactProjection(selected, member.Command.Refs, member)
	if err != nil || !reflect.DeepEqual(evidence, member.Fact.EvidenceRefs) {
		t.Fatalf("resolve exact accepted fact evidence=%+v error=%v", evidence, err)
	}
	selected.Hash = cognitionTestDigest("f")
	if _, err := ResolveCognitionAcceptedFactProjection(selected, member.Command.Refs, member); err == nil {
		t.Fatal("projection resolver accepted changed fact content identity")
	}
	selected.Hash = taskLedgerTestSHA256(member.Command.Content)
	if _, err := ResolveCognitionAcceptedFactProjection(selected, nil, member); err == nil {
		t.Fatal("projection resolver accepted omitted fact source lineage")
	}
	forged := member
	forged.Command.Content = "coherently changed fact content"
	selected.Hash = taskLedgerTestSHA256(forged.Command.Content)
	if _, err := ResolveCognitionAcceptedFactProjection(
		selected, forged.Command.Refs, forged,
	); err == nil {
		t.Fatal("projection resolver accepted command content outside mapping identity")
	}
	forged = member
	forged.EntryURI += "-forged"
	selected.URI = forged.EntryURI
	selected.Hash = taskLedgerTestSHA256(forged.Command.Content)
	if _, err := ResolveCognitionAcceptedFactProjection(
		selected, forged.Command.Refs, forged,
	); err == nil {
		t.Fatal("projection resolver accepted URI outside fact ledger identity")
	}
}

func newCognitionAcceptedFactMaterializationFixture(
	t *testing.T, withFact bool,
) cognitionAcceptedFactMaterializationFixture {
	t.Helper()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "064")); err != nil {
		t.Fatal(err)
	}
	database := newCognitionDatabaseFixture(t, repository)
	authority := cognitionTestFactAuthority()
	if withFact {
		authority = cognitionFactAuthorityForTest(t, planFirstCognitionObservation, cognitionTestDigest("b"))
	}
	if _, err := repository.StartCognitionEpisode(t.Context(), database.Start, authority); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var payloadSHA string
	if err := pool.QueryRow(t.Context(), `
		SELECT payload_json,payload_json_sha256
		FROM cognition_accepted_fact_materializations WHERE episode_id=$1
	`, database.EpisodeID).Scan(&raw, &payloadSHA); err != nil {
		t.Fatal(err)
	}
	value, err := DecodeCognitionAcceptedFactMaterialization(raw, payloadSHA)
	if err != nil {
		t.Fatal(err)
	}
	return cognitionAcceptedFactMaterializationFixture{
		Database: database, Authority: authority, Value: value, Raw: raw,
	}
}

func rehashAcceptedFactMaterialization(
	t *testing.T, value *CognitionAcceptedFactMaterialization,
) {
	t.Helper()
	raw, err := exactjson.Canonical(value.identity())
	if err != nil {
		t.Fatal(err)
	}
	value.SHA256 = cognitionPayloadSHA(raw)
	value.ID = cognitionAcceptedFactMaterializationPrefix + value.SHA256
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
