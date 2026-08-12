package cognitionreplay

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
	"time"
)

func validBaseInput(t *testing.T) BaseInput {
	t.Helper()
	terminal, err := NewSealedEpisodeTerminal(SealedEpisodeTerminal{
		EpisodeID: "episode-817", EpisodeSealSHA256: testDigest("episode-seal"),
		TraceSHA256:      testDigest("sealed-trace"),
		EpisodeStartedAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
		SealedAt:         time.Date(2026, 8, 11, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	start := testBlob(t, `{"episode":"started"}`)
	observe := testBlob(t, `{"observation":"hall"}`)
	goal := testBlob(t, `{"goal":"reach target"}`)
	knowledge := testBlob(t, `{"location":"hall"}`)
	sources := []SourceRecord{
		{Ordinal: 1, CallOrdinal: 0, Phase: 1, Sequence: 1, Kind: "transition", ID: "start", Payload: start.Ref()},
		{Ordinal: 2, CallOrdinal: 0, Phase: 2, Sequence: 1, Kind: "episode_progress", ID: "goal", Payload: goal.Ref()},
		{Ordinal: 3, CallOrdinal: 1, Phase: 1, Sequence: 1, Kind: "transition", ID: "observe", Payload: observe.Ref()},
	}
	events := []Event{
		{Sequence: 1, Kind: EventWorldStarted, MappingSchema: StructuralMappingSchemaV1, Sources: []SourceRef{sources[0].Ref()}, Payload: start.Ref()},
		{Sequence: 2, Kind: EventGoalActivated, MappingSchema: StructuralMappingSchemaV1, Sources: []SourceRef{sources[1].Ref()}, Payload: goal.Ref()},
		{Sequence: 3, Kind: EventObservationAcquired, MappingSchema: StructuralMappingSchemaV1, Sources: []SourceRef{sources[2].Ref()}, Payload: observe.Ref()},
	}
	entry := KnowledgeEntry{
		Kind: KnowledgeObservation, Ref: "observation://hall", Status: KnowledgeActive,
		Authority: AuthorityEnvironment, Content: knowledge.Ref(), SourceEvents: []uint64{3},
	}
	checkpoints := []KnowledgeCheckpoint{
		{Sequence: 1, AfterEvent: 0, State: KnowledgeState{Schema: KnowledgeStateSchemaV1, Entries: []KnowledgeEntry{}}},
		{Sequence: 2, AfterEvent: 3, State: KnowledgeState{Schema: KnowledgeStateSchemaV1, Revision: &PublicRevision{Number: 1, SHA256: testDigest("revision-1")}, Entries: []KnowledgeEntry{entry}}, Delta: &KnowledgeDelta{
			Schema: KnowledgeDeltaSchemaV1, FromEvent: 1, ThroughEvent: 3,
			SetRevision: &PublicRevision{Number: 1, SHA256: testDigest("revision-1")}, Upserts: []KnowledgeEntry{entry}, Releases: []KnowledgeRelease{},
		}},
	}
	return BaseInput{
		TerminalAuthority: terminal, PublicWorldSHA256: testDigest("public-world"),
		PublicWorldSchema: "omnidex.public-world.v1", PublicAuthoritySHA256: testDigest("public-authority"),
		Sources: sources, Events: events, Checkpoints: checkpoints,
		Blobs: []Blob{start, observe, goal, knowledge},
	}
}

func validPrivateOverlayInput(t *testing.T, base Artifact) PrivateOverlayInput {
	t.Helper()
	verified, err := VerifyBase(base.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	oracle := testBlob(t, `{"hidden":"world"}`)
	evaluation := testBlob(t, `{"goal_success":true}`)
	snapshot := testBlob(t, `{"rooms":["hall","tower"]}`)
	return PrivateOverlayInput{
		BaseReplaySHA256:        base.SHA256,
		TerminalAuthoritySHA256: verified.Manifest().TerminalAuthoritySHA256,
		OracleSHA256:            oracle.SHA256, EvaluationSHA256: evaluation.SHA256,
		Sources: []PrivateSource{
			{Ordinal: 1, Kind: PrivateSourceOracle, ID: "oracle", Payload: oracle.Ref()},
			{Ordinal: 2, Kind: PrivateSourceEvaluation, ID: "evaluation", Payload: evaluation.Ref()},
		},
		Events: []PrivateEvent{
			{Sequence: 1, Kind: PrivateEventWorldTruth, Sources: []PrivateSourceRef{
				{Ordinal: 1, Kind: PrivateSourceOracle, ID: "oracle", PayloadSHA256: oracle.SHA256},
			}, Payload: snapshot.Ref()},
			{Sequence: 2, Kind: PrivateEventEvaluation, Sources: []PrivateSourceRef{
				{Ordinal: 2, Kind: PrivateSourceEvaluation, ID: "evaluation", PayloadSHA256: evaluation.SHA256},
			}, Payload: evaluation.Ref()},
		},
		Frames: []PrivateFrame{{Sequence: 1, AfterEvent: 2, Snapshot: snapshot.Ref()}},
		Blobs:  []Blob{oracle, evaluation, snapshot},
	}
}

func testBlob(t *testing.T, value string) Blob {
	t.Helper()
	blob, err := NewBlob("application/json", []byte(value))
	if err != nil {
		t.Fatal(err)
	}
	return blob
}

func testDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

type testContainerEntry struct {
	Name string
	Body []byte
}

func readTestContainer(t *testing.T, raw []byte) []testContainerEntry {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	result := make([]testContainerEntry, len(reader.File))
	for index, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(handle)
		closeErr := handle.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("read container entry: %v / %v", err, closeErr)
		}
		result[index] = testContainerEntry{Name: file.Name, Body: body}
	}
	return result
}

func writeTestContainer(t *testing.T, entries []testContainerEntry) []byte {
	t.Helper()
	values := make([]containerFile, len(entries))
	for index, entry := range entries {
		values[index] = containerFile{name: entry.Name, body: entry.Body}
	}
	raw, err := encodeContainer(values)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneTestEntries(values []testContainerEntry) []testContainerEntry {
	result := make([]testContainerEntry, len(values))
	for index, value := range values {
		result[index] = testContainerEntry{Name: value.Name, Body: append([]byte(nil), value.Body...)}
	}
	return result
}
