package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleLineProviderFramingRemainsCodeOwnedAndByteExact(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	checks := map[string][]string{
		"internal/assemblyline/portable_response_framing.go": {
			"func PortableResponseFramingForWorkKind(",
			"func PortableResponseFramingForJob(",
		},
		"internal/worker/llm_response_contract.go": {
			"assemblyline.PortableResponseFramingForJob(job)",
			"case assemblyline.PortableResponseFramingSingleLine:",
			"contract.ResponseFraming = framing",
		},
		"internal/llm/exact_prepared_request.go": {
			"ExactPreparedLineStopV1 = \"\\n\"",
			"ExactPreparedRawChatUserPrefixV1",
			"ExactPreparedRawChatAssistantBoundaryV1",
			"func ExactPreparedRequestModelInput(prepared PreparedModel)",
			"profile.transport != exactPreparedTransportRaw",
			"return ExactPreparedRawChatUserPrefixV1 + modelInput +",
		},
		"internal/llm/exact_profile_request.go": {
			"prepared.RawTextStopSequence == ExactPreparedLineStopV1",
			"request.Options.Stop = []string{prepared.RawTextStopSequence}",
			"ExactPreparedRequestModelInput(prepared)",
		},
		"internal/queue/station_call_response_framing.go": {
			"func ExpectedStationCallStopSequence(",
			"return llm.ExactPreparedLineStopV1, nil",
			"return llm.ExactPreparedRawChatEndV1, nil",
		},
		"internal/queue/station_call_validation.go": {
			"llm.ExactPreparedRequestModelInput(prepared)",
			"ModelInputSHA256: stationGapSHA256(modelInput)",
		},
		"internal/worker/exact_station_replay.go": {
			"exactStationReplayStoredModelInput(boundary, gap, call)",
			"llm.ExactPreparedRequestModelInput(llm.PreparedModel{",
		},
		"internal/assemblyline/raw_semantic_leaf.go": {
			"leaf := raw",
			"leaf != strings.TrimSpace(leaf)",
			"!allowMultiline && strings.ContainsAny(leaf, \"\\r\\n\")",
		},
	}
	for relative, required := range checks {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		source := string(raw)
		for _, token := range required {
			if !strings.Contains(source, token) {
				t.Errorf("%s omitted framing invariant %q", relative, token)
			}
		}
	}

	framing, err := os.ReadFile(filepath.Join(
		root, "internal", "assemblyline", "portable_response_framing.go",
	))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"internal/llm", "strings.Trim", ".Candidate",
		"portableResponseFramingOriginal", "ResponseCorrection",
	} {
		if strings.Contains(string(framing), forbidden) {
			t.Errorf("assemblyline response framing owns forbidden operation %q", forbidden)
		}
	}
}
