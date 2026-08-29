package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

func TestStationGapOpeningPreservesRawPortableJobAndCanonicalProjection(t *testing.T) {
	t.Parallel()

	job, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "\n exact question \n",
	})
	if err != nil {
		t.Fatal(err)
	}
	record := StationGapOpenRecord{
		Authority: model.StepAttemptAuthority{JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a"},
		Job:       job, Station: station.ConversationResponse,
		ContextTokens: 8192, MaxOutputTokens: 8192,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
	validated, err := validateStationGapOpening(record)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Prompt != prompt || validated.PortablePayload != string(job.Payload) {
		t.Fatalf("opening changed exact prompt or payload: %+v", validated)
	}
	if !strings.Contains(validated.ProjectionEnvelope, `"renderer":"omnidex.render-portable-job.v5"`) ||
		strings.Contains(validated.ProjectionEnvelope, `"response_schema"`) ||
		validated.PortableSchema != assemblyline.PortableJobSchemaV2 ||
		validated.Scope != assemblyline.PortableSemanticWorkerScope ||
		len(validated.ProjectionSHA256) != 64 || len(validated.PortableEnvelopeSHA256) != 64 {
		t.Fatalf("projection envelope=%q sha=%q", validated.ProjectionEnvelope, validated.ProjectionSHA256)
	}
	if err := ValidateStationGapSemanticUncertainty(validated); err != nil {
		t.Fatalf("opening omitted exact semantic uncertainty authority: %v", err)
	}
}

func TestStationGapOpeningRejectsGenericOrUnboundedAuthority(t *testing.T) {
	t.Parallel()

	job, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Exact request.",
	})
	if err != nil {
		t.Fatal(err)
	}
	base := StationGapOpenRecord{
		Authority: model.StepAttemptAuthority{JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker"},
		Job:       job, Station: station.ConversationResponse,
		ContextTokens: 8192, MaxOutputTokens: 8192,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
	for name, mutate := range map[string]func(*StationGapOpenRecord){
		"forged portable identity": func(record *StationGapOpenRecord) { record.Job.ID = strings.Repeat("c", 64) },
		"different station":        func(record *StationGapOpenRecord) { record.Station = station.GroundedAnswer },
		"missing station":          func(record *StationGapOpenRecord) { record.Station = "" },
		"missing budget":           func(record *StationGapOpenRecord) { record.ContextTokens = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			record := base
			mutate(&record)
			if _, err := validateStationGapOpening(record); err == nil {
				t.Fatalf("accepted invalid record %#v", record)
			}
		})
	}
}

func TestStationMappingIsExplicitForEveryPortableWorkKind(t *testing.T) {
	t.Parallel()
	for _, kind := range assemblyline.AllWorkKinds() {
		_, err := stationForPortableWorkKind(kind)
		if err != nil {
			t.Fatalf("production work kind %q has no station mapping: %v", kind, err)
		}
	}
}

func TestStationMappingRejectsRemovedSkillProcedureWork(t *testing.T) {
	t.Parallel()

	if _, err := stationForPortableWorkKind(assemblyline.WorkKind("skill_procedure")); err == nil ||
		!strings.Contains(err.Error(), "not a production semantic station") {
		t.Fatalf("removed skill procedure mapping error=%v", err)
	}
}

func TestStationMappingRejectsRetiredWork(t *testing.T) {
	t.Parallel()

	for _, retired := range []assemblyline.WorkKind{
		"conversation_context_selection",
		"memory_context_selection",
		"roleplay_narrative_continuity",
		"application_acceptance_grounding_review",
		"application_service_endpoint_contract",
	} {
		if _, err := stationForPortableWorkKind(retired); err == nil ||
			!strings.Contains(err.Error(), "not a production semantic station") {
			t.Fatalf("retired context mapping %q error=%v", retired, err)
		}
	}
}

func TestStationGapTerminalRequiresOneExactOutcome(t *testing.T) {
	t.Parallel()

	base := StationGapTerminalRecord{
		Authority: model.StepAttemptAuthority{JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker"},
		OpeningID: 7, GapID: strings.Repeat("d", 64), Status: StationGapResolved,
		Response: " exact response ", Projection: stationGapExactResponseProjection(
			strings.Repeat("a", 64), " exact response ",
		),
	}
	if err := validateStationGapTerminal(base); err != nil {
		t.Fatal(err)
	}
	const prefix = "wrapped "
	for _, kind := range []StationGapProjectionKind{
		StationGapProjectionSourceDeclaration,
		StationGapProjectionTypeScriptFunction,
	} {
		wrapped := base
		raw := prefix + base.Response
		wrapped.Projection = &StationGapSourceProjection{
			Kind: kind, CallReceiptSHA256: strings.Repeat("a", 64),
			SourceResponseSHA256: stationGapSHA256(raw),
			StartByte:            len(prefix), EndByte: len(raw),
		}
		if err := validateStationGapTerminal(wrapped); err == nil ||
			!strings.Contains(err.Error(), "exact full response") {
			t.Fatalf("resolved gap accepted %s projected inner span: %v", kind, err)
		}
	}
	forgedIdentity := base
	forgedIdentity.Projection = stationGapExactResponseProjection(
		strings.Repeat("a", 64), base.Response,
	)
	forgedIdentity.Projection.SourceResponseSHA256 = strings.Repeat("b", 64)
	if err := validateStationGapTerminal(forgedIdentity); err == nil ||
		!strings.Contains(err.Error(), "identity") {
		t.Fatalf("resolved gap accepted a different source response identity: %v", err)
	}
	base.Error = "invented failure"
	if err := validateStationGapTerminal(base); err == nil {
		t.Fatal("resolved gap accepted an error")
	}
	base.Status, base.Response, base.Projection, base.Error = StationGapFailed, "", nil, "provider failed"
	if err := validateStationGapTerminal(base); err != nil {
		t.Fatal(err)
	}
	base.Response = "partial"
	if err := validateStationGapTerminal(base); err == nil {
		t.Fatal("failed gap accepted a response")
	}
}

func stationGapExactResponseProjection(
	receiptSHA256 string,
	response string,
) *StationGapSourceProjection {
	return &StationGapSourceProjection{
		Kind: StationGapProjectionExactResponse, CallReceiptSHA256: receiptSHA256,
		SourceResponseSHA256: stationGapSHA256(response), StartByte: 0, EndByte: len(response),
	}
}
