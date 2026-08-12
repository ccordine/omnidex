package cognitionreplay

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestStructuralBaseExportIsDeterministicAndVerifiable(t *testing.T) {
	input := validBaseInput(t)
	first, err := ExportStructuralBase(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExportStructuralBase(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes, second.Bytes) || first.SHA256 != second.SHA256 {
		t.Fatal("identical sealed inputs did not produce a byte-identical replay")
	}
	verified, err := VerifyBase(first.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if verified.SHA256() != first.SHA256 || verified.Manifest().SemanticStatus != SemanticStructural {
		t.Fatalf("verified replay identity changed: %#v", verified.Manifest())
	}
}

func TestHandBuiltOpaqueStructuralReplayCannotBecomeSeriousEvidence(t *testing.T) {
	input := validBaseInput(t)
	opaque := testBlob(t, `{"seed":817,"oracle_sha256":"`+testDigest("oracle")+`"}`)
	replaced := input.Sources[0].Payload.SHA256
	input.Sources[0].Payload = opaque.Ref()
	input.Events[0].Sources = []SourceRef{input.Sources[0].Ref()}
	input.Events[0].Payload = opaque.Ref()
	input.Blobs = append(input.Blobs, opaque)
	filtered := input.Blobs[:0]
	for _, blob := range input.Blobs {
		if blob.SHA256 != replaced {
			filtered = append(filtered, blob)
		}
	}
	input.Blobs = filtered
	artifact, err := ExportStructuralBase(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBase(artifact.Bytes); err != nil {
		t.Fatal(err)
	}
}

func TestBasePagesLargeOrderedStreamsAndBindsEverySourceKindMapping(t *testing.T) {
	input := validBaseInput(t)
	payload := testBlob(t, `{"value":"shared"}`)
	input.Sources = make([]SourceRecord, maxPageItems+1)
	input.Events = make([]Event, maxPageItems+1)
	for index := range input.Sources {
		ordinal := uint64(index + 1)
		input.Sources[index] = SourceRecord{
			Ordinal: ordinal, CallOrdinal: int64(index), Phase: 1, Sequence: 1,
			Kind: "transition", ID: fmt.Sprintf("record-%03d", index), Payload: payload.Ref(),
		}
		input.Events[index] = Event{
			Sequence: ordinal, Kind: EventWorldTransition,
			MappingSchema: StructuralMappingSchemaV1,
			Sources:       []SourceRef{input.Sources[index].Ref()}, Payload: payload.Ref(),
		}
	}
	input.Checkpoints = []KnowledgeCheckpoint{
		{Sequence: 1, AfterEvent: 0, State: KnowledgeState{Schema: KnowledgeStateSchemaV1, Entries: []KnowledgeEntry{}}},
		{Sequence: 2, AfterEvent: uint64(len(input.Events)), State: KnowledgeState{Schema: KnowledgeStateSchemaV1, Entries: []KnowledgeEntry{}}, Delta: &KnowledgeDelta{
			Schema: KnowledgeDeltaSchemaV1, FromEvent: 1, ThroughEvent: uint64(len(input.Events)),
			Upserts: []KnowledgeEntry{}, Releases: []KnowledgeRelease{},
		}},
	}
	input.Blobs = []Blob{payload}
	artifact, err := ExportStructuralBase(input)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyBase(artifact.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	manifest := verified.Manifest()
	if len(manifest.SourceMappings) != 1 || manifest.SourceMappings[0].SourceKind != "transition" ||
		len(manifest.SourceMappings[0].EventKinds) != 1 ||
		manifest.SourceMappings[0].EventKinds[0] != EventWorldTransition {
		t.Fatalf("source mapping coverage=%#v", manifest.SourceMappings)
	}
	var sourcePages, eventPages int
	for _, entry := range manifest.Entries {
		switch entry.Kind {
		case entrySourcePage:
			sourcePages++
		case entryEventPage:
			eventPages++
		}
	}
	if sourcePages != 2 || eventPages != 2 {
		t.Fatalf("page counts source=%d event=%d", sourcePages, eventPages)
	}
}

func TestBaseVerifierRejectsMissingSourceKindMapping(t *testing.T) {
	artifact, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	entries := readTestContainer(t, artifact.Bytes)
	var manifest BaseManifest
	if err := decodeCanonical(entries[0].Body, &manifest, "test replay manifest"); err != nil {
		t.Fatal(err)
	}
	manifest.SourceMappings = manifest.SourceMappings[1:]
	entries[0].Body, err = marshalCanonical(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBase(writeTestContainer(t, entries)); err == nil {
		t.Fatal("replay with missing source-kind mapping was accepted")
	}
}

func TestEventTimingRequiresOneCitedTypedTimestampWithinEpisode(t *testing.T) {
	input := validBaseInput(t)
	terminal, ok := input.TerminalAuthority.SealedEpisode()
	if !ok {
		t.Fatal("fixture lost sealed episode terminal authority")
	}
	source := input.Sources[0].Ref()
	at := terminal.EpisodeStartedAt.Add(1500 * time.Millisecond)
	timing := EventTiming{
		Timestamp: at, ElapsedNanoseconds: at.Sub(terminal.EpisodeStartedAt).Nanoseconds(), Source: source,
	}
	if err := validateEventTiming(
		timing, []SourceRef{source}, terminal.EpisodeStartedAt, terminal.SealedAt,
	); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*EventTiming){
		"uncited":         func(value *EventTiming) { value.Source = input.Sources[1].Ref() },
		"default":         func(value *EventTiming) { value.Timestamp = time.Time{} },
		"guessed elapsed": func(value *EventTiming) { value.ElapsedNanoseconds++ },
		"after seal": func(value *EventTiming) {
			value.Timestamp = terminal.SealedAt.Add(time.Nanosecond)
			value.ElapsedNanoseconds = value.Timestamp.Sub(terminal.EpisodeStartedAt).Nanoseconds()
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			candidate := timing
			mutate(&candidate)
			if err := validateEventTiming(
				candidate, []SourceRef{source}, terminal.EpisodeStartedAt, terminal.SealedAt,
			); err == nil {
				t.Fatal("inexact replay timing was accepted")
			}
		})
	}
}

func TestPublicEventRegistryKeepsLifecycleAndEpistemicTransitionsDistinct(t *testing.T) {
	required := []EventKind{
		EventFactAccepted,
		EventDecisionAccepted,
		EventDecisionRejected,
		EventPlanRevised,
		EventModelCallDisposition,
		EventProviderRequestDisposition,
		EventProviderProcessObserved,
		EventEpisodeCanceled,
		EventEpisodeSealed,
	}
	for _, kind := range required {
		if !validPublicEventKind(kind) {
			t.Fatalf("required public event kind %q is not registered", kind)
		}
	}
	if validPublicEventKind(EventKind("private.world_truth")) {
		t.Fatal("private truth event was registered as a public event")
	}
}

func TestBaseRejectsOrphanAndInexactAuthority(t *testing.T) {
	tests := map[string]func(*BaseInput){
		"reordered source": func(input *BaseInput) {
			input.Sources[0], input.Sources[1] = input.Sources[1], input.Sources[0]
		},
		"missing source citation": func(input *BaseInput) {
			input.Events[2].Sources = []SourceRef{input.Sources[0].Ref()}
		},
		"unknown source citation": func(input *BaseInput) {
			input.Events[0].Sources[0].ID = "unknown"
		},
		"reordered event": func(input *BaseInput) {
			input.Events[0], input.Events[1] = input.Events[1], input.Events[0]
		},
		"orphan blob": func(input *BaseInput) {
			input.Blobs = append(input.Blobs, testBlob(t, `{"unused":true}`))
		},
		"missing blob": func(input *BaseInput) {
			input.Blobs = input.Blobs[1:]
		},
		"checkpoint gap": func(input *BaseInput) {
			input.Checkpoints[1].Delta.FromEvent = 2
		},
		"nil checkpoint entries": func(input *BaseInput) {
			input.Checkpoints[0].State.Entries = nil
		},
		"nil delta upserts": func(input *BaseInput) {
			input.Checkpoints[1].Delta.Upserts = nil
		},
		"nil delta releases": func(input *BaseInput) {
			input.Checkpoints[1].Delta.Releases = nil
		},
		"private event": func(input *BaseInput) {
			input.Events[0].Kind = EventKind("private.world_truth")
		},
		"guessed structural timestamp": func(input *BaseInput) {
			input.Events[0].Timing = &EventTiming{}
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			input := validBaseInput(t)
			mutate(&input)
			if _, err := ExportStructuralBase(input); err == nil {
				t.Fatal("invalid replay input was accepted")
			}
		})
	}
}

func TestBaseVerifierRejectsAlteredMissingReorderedAndPrivateContainerData(t *testing.T) {
	artifact, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	entries := readTestContainer(t, artifact.Bytes)
	for name, mutate := range map[string]func([]testContainerEntry) []testContainerEntry{
		"altered blob": func(values []testContainerEntry) []testContainerEntry {
			values[len(values)-1].Body = append(values[len(values)-1].Body, '!')
			return values
		},
		"missing page": func(values []testContainerEntry) []testContainerEntry {
			return append(values[:1], values[2:]...)
		},
		"reordered entries": func(values []testContainerEntry) []testContainerEntry {
			values[1], values[2] = values[2], values[1]
			return values
		},
		"orphan entry": func(values []testContainerEntry) []testContainerEntry {
			return append(values, testContainerEntry{Name: "blobs/sha256/" + testDigest("orphan"), Body: []byte("orphan")})
		},
		"private manifest field": func(values []testContainerEntry) []testContainerEntry {
			values[0].Body = bytes.Replace(
				values[0].Body, []byte("}\n"),
				[]byte(",\"oracle_sha256\":\""+testDigest("oracle")+"\"}\n"), 1,
			)
			return values
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			candidate := mutate(cloneTestEntries(entries))
			if _, err := VerifyBase(writeTestContainer(t, candidate)); err == nil {
				t.Fatal("corrupt replay container was accepted")
			}
		})
	}
}
