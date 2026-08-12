package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresProductionSemanticReplayExportsAndReopensExactly(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	bundle, request, preregistered := semanticReplayMatrixFixture(
		t, ctx, pool, repository, hostStore,
	)
	run, err := RunPublicFullCognition(ctx, bundle, request)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := LoadSealedEpisode(request.EpisodeSealPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cold, run.Episode) {
		t.Fatal("cold-read Full episode differs from the run seal")
	}
	trace, err := readProductionTrace(ctx, repository, run.Episode.Manifest.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	materialization := exactOneProposalMaterialization(t, trace)
	assertProposalMaterializationRejectsRehashedUnknownField(t, materialization)
	sidecars := semanticReplaySidecarsFromEpisode(t, request.EpisodeSealPath, run.Episode)
	first, err := ExportProductionSemanticReplay(ctx, repository, bundle, run.Episode, sidecars)
	if err != nil {
		t.Fatalf("export semantic replay for outcome %+v: %v", run.Episode.Manifest.Outcome, err)
	}
	second, err := ExportProductionSemanticReplay(ctx, repository, bundle, run.Episode, sidecars)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 || !bytes.Equal(first.Bytes, second.Bytes) {
		t.Fatal("production semantic replay export is not byte deterministic")
	}

	frozen, err := bundle.Authority.RatGeneration.Fixed.Brain.attestedBrain()
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := collectSemanticReplayEvidence(trace, frozen)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapAuthority, activationAuthority, err :=
		semanticReplayRuntimeEvidenceAuthorities(cold)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := loadRuntimeBrainBootstrapEvidence(
		request.EpisodeSealPath, bootstrapAuthority,
		bundle.Authority.RatGeneration.Fixed.Brain,
	)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := loadRuntimeProviderActivationEvidence(
		request.EpisodeSealPath, activationAuthority,
		bundle.Authority.RatGeneration.Fixed.Brain,
	)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.BootstrapEvidence.Ref != inventory.initialBootstrap.Evidence ||
		activation.Receipt != inventory.initialObservation ||
		activation.IdentityEvidence.Ref != inventory.initialObservation.Observation.Evidence {
		t.Fatal("cold-read Full runtime sidecars differ from exact queue provider authorities")
	}
	verified, err := cognitionreplay.VerifyBase(first.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := verifyProductionSemanticProjection(verified)
	if err != nil || projection.verified.SHA256() != first.SHA256 {
		t.Fatal(err)
	}
	serious, err := VerifyProductionSemanticReplayFor(first.Bytes, preregistered)
	if err != nil || serious.SHA256() != first.SHA256 {
		t.Fatalf("verify replay for preregistered Matrix coordinate: %v", err)
	}
	if err := serious.RequireSeriousExecution(); err != nil {
		t.Fatalf("require serious replay execution: %v", err)
	}
	manifest := verified.Manifest()
	if manifest.SourceCount != len(trace.Records) || len(verified.Sources()) != len(trace.Records) ||
		manifest.EventCount != len(verified.Events()) || manifest.EventCount == 0 ||
		manifest.CheckpointCount != len(verified.Checkpoints()) || manifest.CheckpointCount == 0 ||
		manifest.BlobCount == 0 || len(manifest.SourceMappings) == 0 {
		t.Fatalf("semantic replay closure is incomplete: %+v", manifest)
	}

	mutations := map[string]func(*SealedEpisode){
		"metric": func(value *SealedEpisode) {
			value.Manifest.Resources.WallMilliseconds++
		},
		"trace omission": func(value *SealedEpisode) {
			for index, entry := range value.Manifest.Trace {
				if strings.HasPrefix(entry.ID, "production-") {
					value.Manifest.Trace = append(
						value.Manifest.Trace[:index], value.Manifest.Trace[index+1:]...,
					)
					return
				}
			}
			t.Fatal("production trace wrapper is absent")
		},
		"trace extra": func(value *SealedEpisode) {
			payload, payloadErr := traceJSONObject(struct {
				Schema string `json:"schema"`
			}{Schema: "omnidex.semantic-replay-extra.v1"})
			if payloadErr != nil {
				t.Fatal(payloadErr)
			}
			index := len(value.Manifest.Trace) - 1
			extra := TraceEntry{Kind: TraceLedger, ID: "semantic-extra", Payload: payload}
			value.Manifest.Trace = append(value.Manifest.Trace, TraceEntry{})
			copy(value.Manifest.Trace[index+1:], value.Manifest.Trace[index:])
			value.Manifest.Trace[index] = extra
		},
		"trace mutation": func(value *SealedEpisode) {
			for index := range value.Manifest.Trace {
				if value.Manifest.Trace[index].Kind == TraceProjection &&
					!strings.HasPrefix(value.Manifest.Trace[index].ID, "production-") {
					var projection ProjectionTrace
					if err := decodeTracePayload(
						value.Manifest.Trace[index].Payload, &projection,
						"semantic replay mutation projection",
					); err != nil || len(projection.Selected) == 0 {
						t.Fatalf("decode projection mutation witness: %v", err)
					}
					projection.Selected[0].RenderedBytes++
					payload, err := traceJSONObject(projection)
					if err != nil || projection.Validate() != nil {
						t.Fatalf("encode valid projection mutation witness: %v", err)
					}
					value.Manifest.Trace[index].Payload = payload
					return
				}
			}
			t.Fatal("derived projection trace is absent")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := cloneAndResealSemanticEpisode(t, run.Episode, mutate)
			if _, err := ExportProductionSemanticReplay(
				ctx, repository, bundle, changed, sidecars,
			); err == nil {
				t.Fatal("semantic replay accepted changed episode derivation")
			}
		})
	}
}

func exactOneProposalMaterialization(
	t *testing.T,
	trace productionTrace,
) queue.CognitionSealedTraceRecord {
	t.Helper()
	var found []queue.CognitionSealedTraceRecord
	for _, record := range trace.Records {
		if record.Kind == queue.CognitionTraceKindProposalMaterialization {
			found = append(found, record)
		}
	}
	if len(found) != 1 {
		t.Fatalf("production Full replay proposal materializations=%d want 1", len(found))
	}
	return found[0]
}

func assertProposalMaterializationRejectsRehashedUnknownField(
	t *testing.T,
	record queue.CognitionSealedTraceRecord,
) {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(record.Payload, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown_semantic_authority"] = true
	raw, err := exactjson.Canonical(object)
	if err != nil {
		t.Fatal(err)
	}
	record.Payload, record.SHA256 = raw, digestExactBytes(raw)
	state := &semanticReplayState{}
	if _, err := state.mapProposalMaterialization(
		record, semanticReplaySourceForRecord(t, 1, record),
	); err == nil {
		t.Fatal("semantic proposal mapper accepted rehashed unknown payload authority")
	}
}

func cloneAndResealSemanticEpisode(
	t *testing.T,
	episode SealedEpisode,
	mutate func(*SealedEpisode),
) SealedEpisode {
	t.Helper()
	value := episode
	value.Manifest.Trace = append([]TraceEntry(nil), episode.Manifest.Trace...)
	mutate(&value)
	for index := range value.Manifest.Trace {
		value.Manifest.Trace[index].Sequence = uint64(index + 1)
	}
	prepared, err := prepareEpisodeManifest(value.Manifest)
	if err != nil {
		t.Fatalf("prepare internally valid changed episode: %v", err)
	}
	value.Manifest = prepared
	value.SealSHA256, err = digestJSON(value.Manifest)
	if err != nil || value.Validate() != nil {
		t.Fatalf("reseal internally valid changed episode: %v", err)
	}
	return value
}

func semanticReplaySidecarsFromEpisode(
	t *testing.T,
	episodePath string,
	episode SealedEpisode,
) ProductionSemanticReplaySidecars {
	t.Helper()
	var bootstrap RuntimeBrainBootstrapEvidenceAuthority
	var activation RuntimeProviderActivationEvidenceAuthority
	for _, entry := range episode.Manifest.Trace {
		switch entry.Kind {
		case TraceProviderBootstrap:
			value, err := decodeRuntimeBrainBootstrapTrace(entry)
			if err != nil || bootstrap.Schema != "" {
				t.Fatalf("runtime Brain bootstrap trace: %v", err)
			}
			bootstrap = value
		case TraceProviderActivation:
			value, err := decodeRuntimeProviderActivationTrace(entry)
			if err != nil || activation.Schema != "" {
				t.Fatalf("runtime provider activation trace: %v", err)
			}
			activation = value
		}
	}
	if bootstrap.Validate() != nil || activation.Validate() != nil {
		t.Fatal("sealed episode lacks exact runtime provider sidecar authorities")
	}
	bootstrapRaw, err := os.ReadFile(runtimeBrainBootstrapEvidencePath(episodePath, bootstrap))
	if err != nil {
		t.Fatal(err)
	}
	activationRaw, err := os.ReadFile(runtimeProviderActivationEvidencePath(episodePath, activation))
	if err != nil {
		t.Fatal(err)
	}
	return ProductionSemanticReplaySidecars{
		RuntimeBrainBootstrapEvidence:     bootstrapRaw,
		RuntimeProviderActivationEvidence: activationRaw,
	}
}
