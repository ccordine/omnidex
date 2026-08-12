package cognitiongauntlet

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPublicFullRuntimeEvidenceSealsAndColdReadsExactly(t *testing.T) {
	fixture, episodePath, sealed, evidence := sealPublicFullRuntimeEvidenceFixture(t)
	loaded, err := LoadSealedEpisode(episodePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, sealed) {
		t.Fatal("cold-read Full episode differs from its in-memory seal")
	}
	bootstrap, activation, err := semanticReplayRuntimeEvidenceAuthorities(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap != evidence.bootstrapAuthority || activation != evidence.activationAuthority {
		t.Fatal("cold-read Full episode changed a runtime evidence authority")
	}
	bootstrapRaw, err := os.ReadFile(runtimeBrainBootstrapEvidencePath(episodePath, bootstrap))
	if err != nil {
		t.Fatal(err)
	}
	activationRaw, err := os.ReadFile(runtimeProviderActivationEvidencePath(episodePath, activation))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bootstrapRaw, fixture.sidecars.RuntimeBrainBootstrapEvidence) ||
		!bytes.Equal(activationRaw, fixture.sidecars.RuntimeProviderActivationEvidence) {
		t.Fatal("sealed Full runtime evidence bytes differ from the exact live evidence")
	}
	loadedBootstrap, err := loadRuntimeBrainBootstrapEvidence(
		episodePath, bootstrap, fixture.frozen,
	)
	if err != nil || !reflect.DeepEqual(loadedBootstrap, fixture.bootstrap) {
		t.Fatalf("cold-read Brain bootstrap differs from live evidence: %v", err)
	}
	loadedActivation, err := loadRuntimeProviderActivationEvidence(
		episodePath, activation, fixture.frozen,
	)
	if err != nil || !reflect.DeepEqual(loadedActivation, fixture.activation) {
		t.Fatalf("cold-read provider activation differs from live evidence: %v", err)
	}
}

func TestPublicFullRuntimeEvidenceRejectsMissingOrTamperedSidecars(t *testing.T) {
	for _, test := range []struct {
		name   string
		target func(string, publicFullRuntimeEvidence) string
		mutate func(*testing.T, string)
	}{
		{
			name: "missing bootstrap",
			target: func(path string, evidence publicFullRuntimeEvidence) string {
				return runtimeBrainBootstrapEvidencePath(path, evidence.bootstrapAuthority)
			},
			mutate: removeRuntimeEvidenceFile,
		},
		{
			name: "tampered bootstrap",
			target: func(path string, evidence publicFullRuntimeEvidence) string {
				return runtimeBrainBootstrapEvidencePath(path, evidence.bootstrapAuthority)
			},
			mutate: tamperRuntimeEvidenceFile,
		},
		{
			name: "missing activation",
			target: func(path string, evidence publicFullRuntimeEvidence) string {
				return runtimeProviderActivationEvidencePath(path, evidence.activationAuthority)
			},
			mutate: removeRuntimeEvidenceFile,
		},
		{
			name: "tampered activation",
			target: func(path string, evidence publicFullRuntimeEvidence) string {
				return runtimeProviderActivationEvidencePath(path, evidence.activationAuthority)
			},
			mutate: tamperRuntimeEvidenceFile,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, episodePath, _, evidence := sealPublicFullRuntimeEvidenceFixture(t)
			test.mutate(t, test.target(episodePath, evidence))
			if _, err := LoadSealedEpisode(episodePath); err == nil {
				t.Fatal("cold reader accepted missing or tampered Full runtime evidence")
			}
		})
	}
}

func TestSemanticReplayRuntimeEvidenceAuthoritiesRequireExactPrefix(t *testing.T) {
	_, _, sealed, evidence := sealPublicFullRuntimeEvidenceFixture(t)
	if gotBootstrap, gotActivation, err := semanticReplayRuntimeEvidenceAuthorities(sealed); err != nil || gotBootstrap != evidence.bootstrapAuthority ||
		gotActivation != evidence.activationAuthority {
		t.Fatalf("exact runtime evidence prefix was rejected: %v", err)
	}
	for name, mutate := range map[string]func([]TraceEntry) []TraceEntry{
		"missing bootstrap": func(trace []TraceEntry) []TraceEntry {
			return append([]TraceEntry(nil), trace[1:]...)
		},
		"missing activation": func(trace []TraceEntry) []TraceEntry {
			return append(append([]TraceEntry(nil), trace[:1]...), trace[2:]...)
		},
		"swapped prefix": func(trace []TraceEntry) []TraceEntry {
			trace[0], trace[1] = trace[1], trace[0]
			return trace
		},
		"duplicate bootstrap": func(trace []TraceEntry) []TraceEntry {
			return append(trace, trace[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := sealed
			changed.Manifest.Trace = mutate(append([]TraceEntry(nil), sealed.Manifest.Trace...))
			if _, _, err := semanticReplayRuntimeEvidenceAuthorities(changed); err == nil {
				t.Fatal("semantic replay accepted an inexact runtime evidence prefix")
			}
		})
	}
}

func sealPublicFullRuntimeEvidenceFixture(
	t *testing.T,
) (semanticRuntimeSidecarTestFixture, string, SealedEpisode, publicFullRuntimeEvidence) {
	t.Helper()
	fixture := semanticRuntimeSidecarFixture(t)
	episodePath := filepath.Join(t.TempDir(), "episode.json")
	evidence, err := preparePublicFullRuntimeEvidence(episodePath, fullRuntimeComponents{
		frozenFingerprint: fixture.frozen,
		brainBootstrap:    fixture.bootstrap, providerActivation: fixture.activation,
	})
	if err != nil {
		t.Fatal(err)
	}
	template := validRecorderTemplate(t)
	recorder, err := NewEpisodeRecorder(template)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.appendTo(recorder); err != nil {
		t.Fatal(err)
	}
	revision := cognition.WorldRevision{
		EpisodeID: template.EpisodeID, Number: 1, SHA256: strings.Repeat("e", 64),
	}
	entries := []struct {
		kind    TraceKind
		id      string
		payload any
	}{
		{TraceProjection, "projection-1", testProjectionTrace()},
		{TraceModelCall, "model-call-1", testModelCallTrace()},
	}
	for _, entry := range entries {
		payload, payloadErr := traceJSONObject(entry.payload)
		if payloadErr != nil {
			t.Fatal(payloadErr)
		}
		if err := recorder.Append(entry.kind, entry.id, &revision, payload); err != nil {
			t.Fatalf("append Full fixture trace: %v", err)
		}
	}
	if err := recorder.Append(
		TraceTerminal, "terminal-1", &revision, terminalTestPayload(t),
	); err != nil {
		t.Fatal(err)
	}
	if err := evidence.seal(episodePath, fixture.frozen); err != nil {
		t.Fatal(err)
	}
	sealed, err := recorder.Seal(
		episodePath, time.Unix(1_700_000_000, 0).UTC(),
		time.Unix(1_700_000_001, 0).UTC(), revision,
		Outcome{Terminal: true, GoalSatisfied: true, PublicOutcome: "complete"},
		Resources{
			PolicyCallsConsumed: 1, ModelCalls: 1, ModelDecisions: 1,
			InputTokens: 32, OutputTokens: 16, ContextBytes: 128,
			OutputBytes: 64, PeakContextBytes: 128,
			ProviderTotalNanoseconds: 4, ProviderLoadNanoseconds: 1,
			ProviderPromptEvalNanoseconds: 1, ProviderEvalNanoseconds: 1,
		},
		MemoryMetrics{}, PlanningMetrics{}, RecoveryMetrics{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, episodePath, sealed, evidence
}

func removeRuntimeEvidenceFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func tamperRuntimeEvidenceFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 1
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
