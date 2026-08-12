package cognitionreplay

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestBaseV2BindsOneSealedEpisodeTerminalAuthority(t *testing.T) {
	artifact, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyBase(artifact.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	manifest := verified.Manifest()
	if manifest.Schema != BaseSchemaV2 || manifest.TerminalAuthority.Kind != TerminalSealedEpisode ||
		!validDigest(manifest.TerminalAuthoritySHA256) {
		t.Fatalf("v2 terminal manifest=%+v", manifest)
	}
	terminal, ok := manifest.TerminalAuthority.SealedEpisode()
	if !ok || terminal.EpisodeID != "episode-817" {
		t.Fatalf("sealed terminal=%+v/%t", terminal, ok)
	}
	if _, preEpisode := manifest.TerminalAuthority.PreEpisodeBrainBootstrapFailure(); preEpisode {
		t.Fatal("sealed terminal also exposed a pre-episode variant")
	}
}

func TestBaseVerifierRejectsLegacySchemaWithoutFallback(t *testing.T) {
	artifact, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	entries := readTestContainer(t, artifact.Bytes)
	var manifest BaseManifest
	if err := decodeCanonical(entries[0].Body, &manifest, "test base manifest"); err != nil {
		t.Fatal(err)
	}
	manifest.Schema = "omnidex-replay/v1"
	entries[0].Body, err = marshalCanonical(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBase(writeTestContainer(t, entries)); err == nil {
		t.Fatal("legacy replay schema was accepted as a fallback")
	}
}

func TestTerminalAuthorityRejectsUnknownOrCrossVariantFields(t *testing.T) {
	terminal := validPreEpisodeTerminalForTest(t)
	raw, err := json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string][]byte{
		"unknown envelope field":  bytes.Replace(raw, []byte(`"value":`), []byte(`"other":true,"value":`), 1),
		"sealed field in failure": bytes.Replace(raw, []byte(`"record_id":`), []byte(`"episode_seal_sha256":"`+testDigest("seal")+`","record_id":`), 1),
		"case alias":              bytes.Replace(raw, []byte(`"record_id":`), []byte(`"Record_ID":`), 1),
	} {
		name, candidate := name, candidate
		t.Run(name, func(t *testing.T) {
			var decoded TerminalAuthority
			if err := json.Unmarshal(candidate, &decoded); err == nil {
				t.Fatal("inexact terminal union was accepted")
			}
		})
	}
}

func TestPreEpisodeSourceValidationRejectsShortInputWithoutPanic(t *testing.T) {
	terminal, ok := validPreEpisodeTerminalForTest(t).PreEpisodeBrainBootstrapFailure()
	if !ok {
		t.Fatal("pre-episode fixture lost its variant")
	}
	for size := 0; size < 4; size++ {
		values := make([]SourceRecord, size)
		if err := validatePreEpisodeSources(values, terminal, map[uint64]struct{}{}); err == nil {
			t.Fatalf("short source set of %d records was accepted", size)
		}
	}
}

func TestChunkPublicBytesUsesOneExactBoundedManifest(t *testing.T) {
	raw := bytes.Repeat([]byte{'x'}, 2*maxBlobBytes+17)
	binding, blobs, err := ChunkPublicBytes("provider-body", "application/octet-stream", raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyChunkedBlob(binding, blobs, ChunkedBlobPublicAgentKnowledge)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("in-memory chunking changed provider bytes")
	}
	if _, _, err := ChunkPublicBytes(
		"provider-body", "application/octet-stream", raw[:maxBlobBytes],
	); err == nil {
		t.Fatal("direct-sized body was accepted by the chunk-only constructor")
	}
}

func validPreEpisodeTerminalForTest(t *testing.T) TerminalAuthority {
	t.Helper()
	requested := "episode-" + testDigest("public-authority")
	value := PreEpisodeBrainBootstrapFailureTerminal{
		RecordID: requestedRecordIDForTest(), RequestedEpisodeID: requested,
		Actor: cognition.AttemptRef{
			JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-1",
		},
		FailureID: "brain_bootstrap_failure_" + testDigest("failure"),
		PublicRunAuthority: SourceRef{
			Ordinal: 1, Kind: SourcePublicRunAuthority, ID: requested,
			PayloadSHA256: testDigest("public-authority"),
		},
		FailureAuthority: SourceRef{
			Ordinal: 2, Kind: SourceProviderFailureAuthority, ID: requestedRecordIDForTest(),
			PayloadSHA256: testDigest("failure-authority"),
		},
		FailureReceipt: SourceRef{
			Ordinal: 3, Kind: SourceBrainBootstrapFailureReceipt,
			ID:            "brain_bootstrap_failure_" + testDigest("failure"),
			PayloadSHA256: testDigest("failure-receipt"),
		},
		IdentityEvidence: SourceRef{
			Ordinal: 4, Kind: SourceProviderIdentityEvidence,
			ID:            "provider_identity_" + testDigest("evidence"),
			PayloadSHA256: testDigest("evidence-manifest"),
		},
	}
	terminal, err := NewPreEpisodeBrainBootstrapFailureTerminal(value)
	if err != nil {
		t.Fatal(err)
	}
	return terminal
}

func requestedRecordIDForTest() string {
	return "cognition_provider_failure_" + testDigest("failure-authority")
}
