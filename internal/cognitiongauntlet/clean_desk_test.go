package cognitiongauntlet

import (
	"strings"
	"testing"
)

func TestCleanDeskEvaluationJoinsSealedProjectionToPrivateOracle(t *testing.T) {
	episode := sealedModelEpisode(t, []ProjectedReference{
		projectedReference("evidence://critical", 40, "a"),
		projectedReference("evidence://distractor", 20, "b"),
	})
	oracle := cleanDeskOracle(episode)
	evidence := ProjectionRelevanceEvidence{
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
	metrics, err := EvaluateCleanDesk(episode, oracle, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.TotalModelVisibleBytes != 128 || metrics.RelevantProjectedBytes != 40 ||
		metrics.IrrelevantSelectedBytes != 20 || metrics.MissingCriticalBytes != 0 ||
		!metrics.ConcentrationQualified || metrics.ContextConcentration != 0.3125 {
		t.Fatalf("clean-desk metrics=%+v", metrics)
	}
	if len(metrics.Calls) != 1 || metrics.Calls[0].Budget != testStationBudget() ||
		metrics.Calls[0].InputTokens != 32 || metrics.Calls[0].OutputBytes != 64 {
		t.Fatalf("per-call metrics=%+v", metrics.Calls)
	}
}

func TestCleanDeskNeverRewardsProjectionThatOmittedCriticalEvidence(t *testing.T) {
	episode := sealedModelEpisode(t, []ProjectedReference{
		projectedReference("evidence://distractor", 20, "b"),
	})
	oracle := cleanDeskOracle(episode)
	metrics, err := EvaluateCleanDesk(episode, oracle, ProjectionRelevanceEvidence{
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
	})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.ContextConcentration != 0 || metrics.MissingCriticalBytes != 40 ||
		metrics.MissingCriticalRefs != 1 || metrics.ConcentrationQualified {
		t.Fatalf("omitted critical evidence received clean-desk credit: %+v", metrics)
	}
}

func TestCleanDeskCreditsCompactFactOnlyThroughSealedEvidenceLineage(t *testing.T) {
	critical := projectionReferenceIdentity("evidence://critical", "a")
	fact := projectedReference("fact://accepted", 12, "c")
	fact.SourceRefs = []ProjectionReferenceIdentity{critical}
	episode := sealedModelEpisode(t, []ProjectedReference{fact})
	oracle := cleanDeskOracle(episode)
	metrics, err := EvaluateCleanDesk(episode, oracle, ProjectionRelevanceEvidence{
		Schema: ProjectionRelevanceSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256,
		RelevantRefs: []ProjectionReferenceIdentity{critical},
		CriticalUses: []CriticalProjectionUse{{
			ProjectionID: "projection-1", Ref: critical, RequiredBytes: 40,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !metrics.ConcentrationQualified || metrics.RelevantProjectedBytes != 12 ||
		metrics.MissingCriticalBytes != 0 {
		t.Fatalf("distilled evidence lineage metrics=%+v", metrics)
	}
	fact.SourceRefs = []ProjectionReferenceIdentity{}
	episode = sealedModelEpisode(t, []ProjectedReference{fact})
	oracle = cleanDeskOracle(episode)
	metrics, err = EvaluateCleanDesk(episode, oracle, ProjectionRelevanceEvidence{
		Schema: ProjectionRelevanceSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256,
		RelevantRefs: []ProjectionReferenceIdentity{critical},
		CriticalUses: []CriticalProjectionUse{{
			ProjectionID: "projection-1", Ref: critical, RequiredBytes: 40,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.ConcentrationQualified || metrics.IrrelevantSelectedBytes != 12 ||
		metrics.MissingCriticalBytes != 40 {
		t.Fatalf("unproven compact fact received relevance credit: %+v", metrics)
	}
}

func TestCleanDeskRejectsModelOrProductionRelevanceClaims(t *testing.T) {
	episode := sealedModelEpisode(t, []ProjectedReference{
		projectedReference("evidence://critical", 40, "a"),
	})
	oracle := cleanDeskOracle(episode)
	input := ProjectionRelevanceEvidence{
		Schema: ProjectionRelevanceSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256,
		RelevantRefs: []ProjectionReferenceIdentity{
			projectionReferenceIdentity("evidence://critical", "a"),
		},
		CriticalUses: []CriticalProjectionUse{{
			ProjectionID:  "unknown-projection",
			Ref:           projectionReferenceIdentity("evidence://critical", "a"),
			RequiredBytes: 40,
		}},
	}
	if _, err := EvaluateCleanDesk(episode, oracle, input); err == nil {
		t.Fatal("clean-desk evaluator accepted an unsealed projection authority")
	}
	input.CriticalUses[0].ProjectionID = "projection-1"
	input.RelevantRefs[0].ContentSHA256 = strings.Repeat("c", 64)
	if _, err := EvaluateCleanDesk(episode, oracle, input); err == nil {
		t.Fatal("clean-desk evaluator accepted a critical ref outside private relevance authority")
	}
}

func TestSymbolicScoringRequiresPostSealCleanDeskAuthorityForModelEpisodes(t *testing.T) {
	episode := sealedModelEpisode(t, []ProjectedReference{
		projectedReference("evidence://critical", 40, "a"),
	})
	oracle := cleanDeskOracle(episode)
	base := SymbolicEvaluationEvidence{
		GoalPredicateSatisfied: true, ValidTerminalState: true,
		ActualDecisionCost: oracle.WitnessCost,
	}
	if _, err := ScoreSealedEpisode(episode, oracle, base); err == nil {
		t.Fatal("model episode was scored without private post-seal relevance authority")
	}
	base.ProjectionRelevance = testProjectionRelevance(episode, oracle)
	evaluation, err := ScoreSealedEpisode(episode, oracle, base)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.CleanDesk == nil || !evaluation.CleanDesk.ConcentrationQualified {
		t.Fatalf("evaluation clean desk=%+v", evaluation.CleanDesk)
	}
}

func cleanDeskOracle(episode SealedEpisode) OracleManifest {
	_, oracle := validManifestPair()
	oracle.ScenarioID = episode.Manifest.Scenario.ID
	oracle.PublicSHA256 = episode.Manifest.Scenario.SHA256
	return oracle
}

func projectedReference(uri string, bytes int64, digestByte string) ProjectedReference {
	return ProjectedReference{
		Ref: ProjectionReferenceIdentity{
			URI: uri, Version: "revision-1", ContentSHA256: strings.Repeat(digestByte, 64),
		},
		SourceRefs: []ProjectionReferenceIdentity{}, RenderedBytes: bytes,
	}
}

func projectionReferenceIdentity(uri, digestByte string) ProjectionReferenceIdentity {
	return ProjectionReferenceIdentity{
		URI: uri, Version: "revision-1", ContentSHA256: strings.Repeat(digestByte, 64),
	}
}
