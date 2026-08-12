package cognitionreplay

import (
	"reflect"
	"testing"
)

func TestAblationSemanticProjectionUsesDistinctFrozenRegistry(t *testing.T) {
	if actual := ablationSourceRegistryDigest(); actual != ablationSourceRegistrySHA256V1 {
		t.Fatalf("ablation source registry digest=%s", actual)
	}
	input := validAblationSemanticProjectionInput(t, AblationProjectionSerious)
	artifact, err := ExportAblationSemanticProjection(input)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyBase(artifact.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	manifest := verified.Manifest()
	if manifest.SemanticStatus != SemanticAblationProjection ||
		manifest.ProjectionAuthority != nil || manifest.AblationProjectionAuthority == nil ||
		manifest.AblationProjectionAuthority.RegistryID != AblationSourceRegistryIDV1 ||
		manifest.AblationProjectionAuthority.RegistrySHA256 != ablationSourceRegistrySHA256V1 ||
		manifest.PrivateData {
		t.Fatalf("ablation projection authority=%+v", manifest)
	}
	if !reflect.DeepEqual(AblationSemanticSourceKinds(), []string{
		"ablation.action_outcome", "ablation.call_disposition", "ablation.call_input",
		"ablation.call_outcome", "ablation.context_budget_failure", "ablation.ledger_event",
		"ablation.model_response", "ablation.provider_activation", "ablation.provider_bootstrap",
		"ablation.provider_generation", "ablation.provider_identity",
		"ablation.provider_response_capture", "ablation.root", "ablation.terminal",
		"ablation.transition", "ablation.working_set_event", "ablation.working_set_initial",
	}) {
		t.Fatal("ablation semantic source registry changed")
	}
}

func TestAblationAndProductionSemanticRegistriesAreIsolated(t *testing.T) {
	ablation := validAblationSemanticProjectionInput(t, AblationProjectionSerious)
	ablation.Sources[0].Kind = "transition"
	ablation.Events[0].Sources[0] = ablation.Sources[0].Ref()
	if _, err := ExportAblationSemanticProjection(ablation); err == nil {
		t.Fatal("ablation projection accepted a queue sealed-trace source kind")
	}
	production := semanticProjectionContentFixture(t)
	production.Sources[0].Kind = "ablation.root"
	production.Events[0].Sources[0] = production.Sources[0].Ref()
	if _, err := ExportSemanticProjection(production); err == nil {
		t.Fatal("production projection accepted an ablation source kind")
	}
	ablation = validAblationSemanticProjectionInput(t, AblationProjectionSerious)
	ablation.Events[0].MappingSchema = SemanticMappingSchemaV1
	if _, err := ExportAblationSemanticProjection(ablation); err == nil {
		t.Fatal("ablation projection accepted the queue semantic mapping schema")
	}
	ablation = validAblationSemanticProjectionInput(t, AblationProjectionSerious)
	ablation.Sources[0].Kind = "ablation.unknown"
	ablation.Events[0].Sources[0] = ablation.Sources[0].Ref()
	if _, err := ExportAblationSemanticProjection(ablation); err == nil {
		t.Fatal("ablation projection accepted an unfrozen source kind")
	}
}

func TestAblationSemanticProjectionSeparatesPrivateContaminatedOverlay(t *testing.T) {
	public := validAblationSemanticProjectionInput(t, AblationProjectionSerious)
	public.AblationEvidenceAuthority = privateProjectionContent(t, "private-evidence")
	if _, err := ExportAblationSemanticProjection(public); err == nil {
		t.Fatal("public ablation projection accepted private evidence content")
	}
	private := validAblationSemanticProjectionInput(t, AblationProjectionContaminated)
	private.PrivateOverlayRequired = true
	artifact, err := ExportAblationSemanticProjection(private)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyBase(artifact.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Manifest().AblationProjectionAuthority.PrivateOverlayRequired ||
		verified.Manifest().PrivateData {
		t.Fatal("contaminated ablation public base did not require a separate private overlay")
	}
	entries := readTestContainer(t, artifact.Bytes)
	var changed BaseManifest
	if err := decodeCanonical(entries[0].Body, &changed, "ablation manifest"); err != nil {
		t.Fatal(err)
	}
	changed.AblationProjectionAuthority.PrivateOverlayRequired = false
	entries[0].Body, err = marshalCanonical(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBase(writeTestContainer(t, entries)); err == nil {
		t.Fatal("contaminated ablation replay accepted a removed private-overlay requirement")
	}
	private.PrivateOverlayRequired = false
	if _, err := ExportAblationSemanticProjection(private); err == nil {
		t.Fatal("contaminated ablation projection omitted its private-overlay requirement")
	}
	artifact, err = ExportAblationSemanticProjection(
		validAblationSemanticProjectionInput(t, AblationProjectionSerious),
	)
	if err != nil {
		t.Fatal(err)
	}
	verified, err = VerifyBase(artifact.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Manifest().PrivateData {
		t.Fatal("public ablation projection claimed private content")
	}
	entries = readTestContainer(t, artifact.Bytes)
	var manifest BaseManifest
	if err := decodeCanonical(entries[0].Body, &manifest, "ablation manifest"); err != nil {
		t.Fatal(err)
	}
	manifest.ProjectionAuthority = &ProjectionAuthority{}
	entries[0].Body, err = marshalCanonical(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBase(writeTestContainer(t, entries)); err == nil {
		t.Fatal("ablation projection accepted simultaneous production authority")
	}
}

func TestAblationSemanticProjectionAllowsContentDedupButRequiresJSONRoots(t *testing.T) {
	input := validAblationSemanticProjectionInput(t, AblationProjectionSerious)
	shared, sharedBindings, sharedBlobs := projectionContentForTest(
		t, false, "shared-sidecar", []byte(`{"shared":true}`),
	)
	input.Sidecars = []ProjectionSidecarAuthority{
		{Kind: "model_response", ID: "call-1", Content: shared},
		{Kind: "model_response", ID: "call-2", Content: shared},
	}
	input.ChunkedBlobs = append(input.ChunkedBlobs, sharedBindings...)
	input.Blobs = append(input.Blobs, sharedBlobs...)
	if _, err := ExportAblationSemanticProjection(input); err != nil {
		t.Fatalf("content-addressed sidecar deduplication was rejected: %v", err)
	}
	input.Sidecars[1].ID = input.Sidecars[0].ID
	if _, err := ExportAblationSemanticProjection(input); err == nil {
		t.Fatal("duplicated sidecar identity was accepted")
	}
	input = validAblationSemanticProjectionInput(t, AblationProjectionSerious)
	octets, _, octetBlobs, err := NewPublicProjectionContent(
		"non-json-root", "application/octet-stream", []byte("not-json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	input.PublicBundleAuthority = octets
	input.Blobs = append(input.Blobs, octetBlobs...)
	if _, err := ExportAblationSemanticProjection(input); err == nil {
		t.Fatal("ablation projection accepted a non-JSON root authority")
	}
}

func validAblationSemanticProjectionInput(
	t *testing.T,
	class AblationProjectionClass,
) AblationSemanticProjectionInput {
	t.Helper()
	base := validBaseInput(t)
	base.Sources[0].Kind = "ablation.root"
	base.Sources[1].Kind = "ablation.root"
	base.Sources[2].Kind = "ablation.transition"
	for index := range base.Events {
		base.Events[index].MappingSchema = AblationSemanticMappingSchemaV1
		base.Events[index].Sources = []SourceRef{base.Sources[index].Ref()}
	}
	private := false
	contents := make([]ProjectionContentAuthority, 5)
	bindings := []ChunkedBlobBinding{}
	blobs := append([]Blob(nil), base.Blobs...)
	for index, label := range []string{"public", "episode", "evidence", "bootstrap", "activation"} {
		content, moreBindings, moreBlobs := projectionContentForTest(
			t, private, label, []byte(`{"authority":"`+label+`"}`),
		)
		contents[index] = content
		bindings = append(bindings, moreBindings...)
		blobs = append(blobs, moreBlobs...)
	}
	return AblationSemanticProjectionInput{
		TerminalAuthority: base.TerminalAuthority,
		PublicWorldSHA256: base.PublicWorldSHA256, PublicWorldSchema: base.PublicWorldSchema,
		PublicAuthoritySHA256: base.PublicAuthoritySHA256, ClaimedClass: class,
		PublicBundleAuthority: contents[0], SealedEpisodeAuthority: contents[1],
		AblationEvidenceAuthority: contents[2], BrainBootstrapAuthority: contents[3],
		ProviderActivationAuthority: contents[4], Sidecars: []ProjectionSidecarAuthority{},
		Sources: base.Sources, Events: base.Events, Checkpoints: base.Checkpoints,
		ChunkedBlobs: bindings, Blobs: blobs,
	}
}

func projectionContentForTest(
	t *testing.T,
	private bool,
	id string,
	raw []byte,
) (ProjectionContentAuthority, []ChunkedBlobBinding, []Blob) {
	t.Helper()
	var content ProjectionContentAuthority
	var bindings []ChunkedBlobBinding
	var blobs []Blob
	var err error
	if private {
		content, bindings, blobs, err = NewPrivateProjectionContent(id, "application/json", raw)
	} else {
		content, bindings, blobs, err = NewPublicProjectionContent(id, "application/json", raw)
	}
	if err != nil {
		t.Fatal(err)
	}
	return content, bindings, blobs
}

func privateProjectionContent(t *testing.T, id string) ProjectionContentAuthority {
	t.Helper()
	content, _, _ := projectionContentForTest(t, true, id, []byte(`{"private":true}`))
	return content
}
