package cognitiongauntlet

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func TestAblationSemanticReplayExportsAndReopensExactly(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	oracle := fixture.generated.PrivateOracle()
	for _, variant := range []Variant{
		VariantRawObservation,
		VariantFullTranscript,
		VariantTranscriptCompacted,
		VariantTaskLedger,
		VariantLedgerWorkingSet,
		VariantLedgerProjection,
		VariantRawShell,
	} {
		variant := variant
		t.Run(string(variant), func(t *testing.T) {
			surface := SurfaceSymbolic
			if variant == VariantRawShell {
				surface = SurfaceFilesystem
			}
			request := ablationTestRequest(
				t, variant, surface, 1,
				&witnessPolicyClient{
					model:   mustRatGeneration(t).Fixed.Brain.Model,
					witness: oracle.Witness, evidenceUses: oracle.EvidenceUses,
				},
			)
			result, err := RunAblation(context.Background(), fixture, request)
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := NewVariantPublicInferenceBundle(
				fixture, result.Authority, variant,
			)
			if err != nil {
				t.Fatal(err)
			}
			first, err := ExportAblationSemanticReplay(
				bundle, result.Episode, request.EpisodeSealPath, request.EvidenceSealPath,
			)
			if err != nil {
				t.Fatal(err)
			}
			second, err := ExportAblationSemanticReplay(
				bundle, result.Episode, request.EpisodeSealPath, request.EvidenceSealPath,
			)
			if err != nil || first.SHA256 != second.SHA256 || string(first.Bytes) != string(second.Bytes) {
				t.Fatalf("ablation semantic replay is not deterministic: %v", err)
			}
			verified, err := cognitionreplay.VerifyBase(first.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := verifyAblationSemanticProjection(verified)
			if err != nil || projection.verified.SHA256() != first.SHA256 {
				t.Fatalf("cold product verification changed replay: %v", err)
			}
			want := AblationReplaySerious
			if variant == VariantRawShell {
				want = AblationReplayBenchmarkOnly
			}
			if projection.class != want {
				t.Fatalf("replay class=%q, want %q", projection.class, want)
			}
			if variant == VariantRawObservation {
				assertAblationReplayBindsEffectsAndRootObligation(t, projection)
				assertAblationReplayRejectsTraceChronologyMutation(t, projection)
				assertAblationReplayRejectsEpisodeAuthorityMutations(t, verified, projection)
			}
		})
	}
}

func assertAblationReplayBindsEffectsAndRootObligation(
	t *testing.T,
	projection ablationSemanticProjectionVerification,
) {
	t.Helper()
	effectEvents, obligationEvents := 0, 0
	for _, event := range projection.verified.Events() {
		if event.Kind == cognitionreplay.EventEvidenceAcquired {
			for _, source := range event.Sources {
				if source.Kind == "ablation.transition" && len(source.ID) >= len("effect-") &&
					source.ID[:len("effect-")] == "effect-" {
					effectEvents++
				}
			}
		}
		if event.Kind == cognitionreplay.EventObligationCreated {
			obligationEvents++
		}
	}
	wantEffects := 0
	for _, transition := range projection.evidence.Root.Transitions {
		wantEffects += len(transition.Effects)
	}
	if effectEvents != wantEffects || obligationEvents != 1 {
		t.Fatalf("effect/root-obligation events=%d/%d, want %d/1",
			effectEvents, obligationEvents, wantEffects)
	}
	checkpoints := projection.verified.Checkpoints()
	checkpoint := checkpoints[len(checkpoints)-1]
	wantObligation := "obligation://" + string(projection.evidence.Root.Obligation.ID)
	seenObligation := false
	seenEffects := make(map[string]bool, wantEffects)
	for _, entry := range checkpoint.State.Entries {
		if entry.Kind == cognitionreplay.KnowledgeObligation && entry.Ref == wantObligation &&
			entry.Status == cognitionreplay.KnowledgeActive && entry.Authority == cognitionreplay.AuthorityCode {
			seenObligation = true
		}
		if entry.Kind == cognitionreplay.KnowledgeEvidence &&
			entry.Authority == cognitionreplay.AuthorityEnvironment {
			seenEffects[entry.Ref] = true
		}
	}
	if !seenObligation {
		t.Fatal("final checkpoint omitted the code-owned root obligation")
	}
	for _, transition := range projection.evidence.Root.Transitions {
		for _, effect := range transition.Effects {
			ref := "effect://" + string(effect.Kind) + "/" + effect.ContentSHA256
			if !seenEffects[ref] {
				t.Fatalf("final checkpoint omitted exact environment effect %q", ref)
			}
		}
	}
}

func assertAblationReplayRejectsEpisodeAuthorityMutations(
	t *testing.T,
	verified cognitionreplay.VerifiedBase,
	projection ablationSemanticProjectionVerification,
) {
	t.Helper()
	authority := verified.Manifest().AblationProjectionAuthority
	if authority == nil {
		t.Fatal("verified ablation replay lost its projection authority")
	}
	bootstrapRaw, err := verified.ProjectionContent(authority.BrainBootstrap)
	if err != nil {
		t.Fatal(err)
	}
	activationRaw, err := verified.ProjectionContent(authority.ProviderActivation)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*EpisodeManifest){
		"nonterminal outcome": func(manifest *EpisodeManifest) {
			manifest.Outcome.Terminal = false
		},
		"model decision aggregate": func(manifest *EpisodeManifest) {
			manifest.Resources.ModelDecisions++
		},
		"planning aggregate": func(manifest *EpisodeManifest) {
			manifest.Planning.PlanGenerations++
		},
	} {
		t.Run(name, func(t *testing.T) {
			forged := projection.episode
			mutate(&forged.Manifest)
			seal, err := digestJSON(forged.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			forged.SealSHA256 = seal
			if err := verifyAblationSemanticAuthorities(
				projection.bundle, forged, projection.evidence,
				bootstrapRaw, activationRaw, projection.class,
			); err == nil {
				t.Fatalf("ablation semantic replay accepted forged %s", name)
			}
		})
	}
}

func assertAblationReplayRejectsTraceChronologyMutation(
	t *testing.T,
	projection ablationSemanticProjectionVerification,
) {
	t.Helper()
	forged := projection.episode
	forged.Manifest.Trace = append([]TraceEntry(nil), forged.Manifest.Trace...)
	actionIndex, laterProjection := -1, -1
	for index, entry := range forged.Manifest.Trace {
		if actionIndex < 0 && entry.Kind == TraceAction {
			actionIndex = index
			continue
		}
		if actionIndex >= 0 && entry.Kind == TraceProjection {
			laterProjection = index
			break
		}
	}
	if actionIndex < 0 || laterProjection < 0 {
		t.Fatal("ablation fixture lacks two-call action chronology")
	}
	forged.Manifest.Trace[actionIndex], forged.Manifest.Trace[laterProjection] =
		forged.Manifest.Trace[laterProjection], forged.Manifest.Trace[actionIndex]
	for index := range forged.Manifest.Trace {
		forged.Manifest.Trace[index].Sequence = uint64(index + 1)
	}
	if err := verifyAblationSemanticEpisodeTrace(forged, projection.evidence); err == nil {
		t.Fatal("ablation semantic replay accepted call context before its causal action")
	}
}

func TestAblationSemanticReplayActionFailureNeedsNoDuplicateFailureEntry(t *testing.T) {
	evidence := ablationEvidenceArtifact{Root: ablationEvidenceRoot{
		TerminalCause: ablationTerminalCause{Kind: ablationTerminalActionFailure},
	}}
	if err := consumeAblationTraceFailure(&ablationEpisodeTraceCursor{}, evidence); err != nil {
		t.Fatalf("dispatched action failure incorrectly required a duplicate failure entry: %v", err)
	}
}

func TestAblationSemanticReplayRejectsPrivateOracleEvidence(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	oracle := fixture.generated.PrivateOracle()
	request := ablationTestRequest(
		t, VariantOracleEvidence, SurfaceSymbolic, 1,
		&witnessPolicyClient{
			model:   mustRatGeneration(t).Fixed.Brain.Model,
			witness: oracle.Witness, evidenceUses: oracle.EvidenceUses,
		},
	)
	result, err := RunAblation(context.Background(), fixture, request)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := NewVariantPublicInferenceBundle(
		fixture, result.Authority, VariantOracleEvidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExportAblationSemanticReplay(
		bundle, result.Episode, request.EpisodeSealPath, request.EvidenceSealPath,
	); err == nil {
		t.Fatal("oracle-contaminated evidence entered a public ablation replay")
	}
}

func TestAblationSemanticReplayPreservesRepeatedWorkingSetKnowledgeChanges(t *testing.T) {
	builder := newAblationSemanticBuild()
	unit := ablationUnit(
		1, 70, 1, "ablation.working_set_event", "multi-eviction", struct{}{},
		cognitionreplay.EventWorkingSetReleased,
		cognitionreplay.EventWorkingSetReleased,
		cognitionreplay.EventWorkingSetAttached,
	)
	unit.knowledge = []*semanticKnowledgeChange{
		knowledgeChange(cognitionreplay.KnowledgeWorkingSet, "working-set-item://first",
			cognitionreplay.KnowledgeReleased, cognitionreplay.AuthorityCode),
		knowledgeChange(cognitionreplay.KnowledgeWorkingSet, "working-set-item://second",
			cognitionreplay.KnowledgeReleased, cognitionreplay.AuthorityCode),
		knowledgeChange(cognitionreplay.KnowledgeWorkingSet, "working-set-item://new",
			cognitionreplay.KnowledgeActive, cognitionreplay.AuthorityCode),
	}
	if err := builder.appendUnit(unit, digestExactBytes([]byte("evidence"))); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{
		"working-set-item://first", "working-set-item://second", "working-set-item://new",
	} {
		if _, exists := builder.entries[string(cognitionreplay.KnowledgeWorkingSet)+"\x00"+ref]; !exists {
			t.Fatalf("repeated Working Set event lost exact knowledge identity %q", ref)
		}
	}
}
