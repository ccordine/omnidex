package cognitiongauntlet

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func TestEpisodeSealRejectsUnstructuredOrMismatchedStationTrace(t *testing.T) {
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	manifest.Trace[0].Payload = terminalTestPayload(t)
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted an unstructured Context Projection trace")
	}

	manifest = validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	call := testModelCallTrace()
	call.ProjectionSHA256 = strings.Repeat("f", 64)
	manifest.Trace[1].Payload = mustTraceJSONObject(t, call)
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted a model call bound to another Context Projection")
	}

	manifest = validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	call = testModelCallTrace()
	call.Budget.MaxInputBytes++
	manifest.Trace[1].Payload = mustTraceJSONObject(t, call)
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted a per-call station budget outside sealed authority")
	}
}

func TestEpisodeSealReconcilesStationUsageWithResources(t *testing.T) {
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	manifest.Resources.ContextBytes++
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted aggregate context bytes that differ from sealed calls")
	}
	manifest = validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	manifest.Resources.OutputTokens++
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted aggregate output tokens that differ from sealed calls")
	}
}

func TestProjectionTraceRejectsSelfSourceLineage(t *testing.T) {
	projection := testProjectionTrace()
	projection.Selected = projection.Selected[:1]
	projection.Selected[0].SourceRefs = []ProjectionReferenceIdentity{projection.Selected[0].Ref}
	if err := projection.Validate(); err == nil {
		t.Fatal("raw projected evidence accepted itself as derived source lineage")
	}
}

func sealedModelEpisode(t *testing.T, selected []ProjectedReference) SealedEpisode {
	t.Helper()
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	projection := testProjectionTrace()
	projection.Selected = append([]ProjectedReference(nil), selected...)
	projection.RenderedBytes = 80
	manifest.Trace[0].Payload = mustTraceJSONObject(t, projection)
	path := filepath.Join(t.TempDir(), "episode.json")
	sealed, err := SealEpisode(path, manifest)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func testStationBudget() StationBudget {
	return StationBudget{
		MaxInputBytes: 4096, MaxInputTokens: 1024,
		MaxOutputBytes: 1024, MaxOutputTokens: 256,
	}
}

func testProjectionTrace() ProjectionTrace {
	return ProjectionTrace{
		Schema: ProjectionTraceSchemaV1, ProjectionID: "projection-1",
		ProjectionSHA256: strings.Repeat("d", 64), RenderedBytes: 80,
		EstimatedTokens: 20, TokenEstimator: "utf8-bytes-div-four.v1",
		Selected: []ProjectedReference{
			projectedReference("evidence://critical", 40, "a"),
			projectedReference("evidence://distractor", 20, "b"),
		},
	}
}

func testModelCallTrace() ModelCallTrace {
	return ModelCallTrace{
		Schema: ModelCallTraceSchemaV1, ProjectionID: "projection-1",
		ProjectionSHA256: strings.Repeat("d", 64), Budget: testStationBudget(),
		InputBytes: 128, InputTokens: 32, OutputBytes: 64, OutputTokens: 16,
	}
}

func testProjectionRelevance(
	episode SealedEpisode,
	oracle OracleManifest,
) *ProjectionRelevanceEvidence {
	return &ProjectionRelevanceEvidence{
		Schema: ProjectionRelevanceSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256,
		RelevantRefs: []ProjectionReferenceIdentity{
			projectionReferenceIdentity("evidence://critical", "a"),
		},
		CriticalUses: []CriticalProjectionUse{{
			ProjectionID:  "projection-1",
			Ref:           projectionReferenceIdentity("evidence://critical", "a"),
			RequiredBytes: 40,
		}},
	}
}

func terminalTestPayload(t *testing.T) taskstate.JSONObject {
	t.Helper()
	return mustTraceJSONObject(t, struct {
		Kind string `json:"kind"`
	}{Kind: "terminal"})
}

func mustTraceJSONObject(t *testing.T, value any) taskstate.JSONObject {
	t.Helper()
	payload, err := traceJSONObject(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
