package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type liveCodingQualificationCoverageSnapshot struct {
	AcceptedRequirements []string
	ExcludedCandidates   []string
	ZeroDeltas           []assemblyline.ApplicationRequirementCandidateZeroDelta
	ExactZeroDeltas      int
	SemanticZeroDeltas   int
}

func captureLiveCodingQualificationCoverageSnapshot(
	job assemblyline.PortableJob,
) (*liveCodingQualificationCoverageSnapshot, error) {
	if job.Kind != assemblyline.WorkApplicationRequirementCoverage {
		return nil, nil
	}
	var input assemblyline.ApplicationRequirementCoverageInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		return nil, fmt.Errorf("decode live coverage authority: %w", err)
	}
	rebuilt, err := assemblyline.NewApplicationRequirementCoverageJob(input)
	if err != nil {
		return nil, fmt.Errorf("validate live coverage authority: %w", err)
	}
	if rebuilt.ID != job.ID {
		return nil, fmt.Errorf("live coverage authority does not reproduce its job identity")
	}
	snapshot := &liveCodingQualificationCoverageSnapshot{
		AcceptedRequirements: append([]string(nil), input.AcceptedRequirements...),
		ExcludedCandidates:   append([]string(nil), input.ExcludedCandidates...),
		ZeroDeltas: append(
			[]assemblyline.ApplicationRequirementCandidateZeroDelta(nil),
			input.ZeroDeltas...,
		),
	}
	for _, evidence := range snapshot.ZeroDeltas {
		if liveCodingQualificationZeroDeltaIsExact(snapshot, evidence) {
			snapshot.ExactZeroDeltas++
			continue
		}
		if evidence.RetainedSet != assemblyline.ApplicationRequirementZeroDeltaAcceptedSet ||
			evidence.OutcomeRelation.Relation != assemblyline.ApplicationRequirementSameRuntimeOutcome {
			return nil, fmt.Errorf("live coverage contains unclassified zero-delta evidence")
		}
		snapshot.SemanticZeroDeltas++
	}
	return snapshot, nil
}

func liveCodingQualificationZeroDeltaIsExact(
	snapshot *liveCodingQualificationCoverageSnapshot,
	evidence assemblyline.ApplicationRequirementCandidateZeroDelta,
) bool {
	switch evidence.RetainedSet {
	case assemblyline.ApplicationRequirementZeroDeltaAcceptedSet:
		return evidence.RetainedIndex >= 0 &&
			evidence.RetainedIndex < len(snapshot.AcceptedRequirements) &&
			evidence.Candidate == snapshot.AcceptedRequirements[evidence.RetainedIndex]
	case assemblyline.ApplicationRequirementZeroDeltaExcludedSet:
		return evidence.RetainedIndex >= 0 &&
			evidence.RetainedIndex < len(snapshot.ExcludedCandidates) &&
			evidence.Candidate == snapshot.ExcludedCandidates[evidence.RetainedIndex]
	default:
		return false
	}
}

func assertLiveCodingQualificationCalls(
	t *testing.T,
	calls []liveCodingQualificationCall,
	frozen assemblyline.FrozenApplicationWorkload,
) {
	t.Helper()
	counts, nonRuntimeCount, taskLocalCount := inspectLiveCodingQualificationCalls(t, calls)
	snapshots := liveCodingQualificationCoverageSnapshots(t, calls)
	finalCoverage := snapshots[len(snapshots)-1]
	featureCount := len(frozen.Tasks)
	zeroDeltaCount := len(finalCoverage.ZeroDeltas)
	generationCount := counts[assemblyline.WorkApplicationRequirement]
	kindCount := counts[assemblyline.WorkApplicationRequirementCandidateKind]
	cardinalityCount := counts[assemblyline.WorkApplicationRequirementCandidateCardinality]
	resultRelationCount := counts[assemblyline.WorkApplicationRequirementCandidateResultRelation]
	groundingCount := counts[assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding]
	resultCorrectionCount := counts[assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection]
	outcomeRelationCount := counts[assemblyline.WorkApplicationRequirementCandidateOutcomeRelation]
	splitCount := counts[assemblyline.WorkApplicationRequirementCandidateSplit]
	splitCorrectionCount := counts[assemblyline.WorkApplicationRequirementCandidateSplitCorrection]
	if counts[assemblyline.WorkApplicationContextNeedCoverage] != 0 ||
		counts[assemblyline.WorkApplicationProductContext] != 1 ||
		len(finalCoverage.AcceptedRequirements) != featureCount ||
		len(finalCoverage.ExcludedCandidates) != nonRuntimeCount ||
		zeroDeltaCount > assemblyline.MaxApplicationRequirementZeroDeltas ||
		generationCount != featureCount+nonRuntimeCount+zeroDeltaCount ||
		len(snapshots) != 1+featureCount+zeroDeltaCount ||
		kindCount != nonRuntimeCount+taskLocalCount ||
		kindCount > generationCount+splitCount+resultCorrectionCount ||
		cardinalityCount < taskLocalCount ||
		cardinalityCount > taskLocalCount+splitCount ||
		resultRelationCount != featureCount+resultCorrectionCount ||
		groundingCount != resultCorrectionCount ||
		outcomeRelationCount < featureCount*(featureCount-1)/2+finalCoverage.SemanticZeroDeltas ||
		outcomeRelationCount > taskLocalCount*assemblyline.MaxApplicationRequirementLeaves ||
		splitCount > taskLocalCount*assemblyline.MaxApplicationRequirementCandidateSplitDepth ||
		splitCorrectionCount > splitCount ||
		resultCorrectionCount > generationCount {
		t.Fatalf("live qualification raw-leaf call shape differs from code-owned fixed points: %v", counts)
	}
}

func inspectLiveCodingQualificationCalls(
	t *testing.T,
	calls []liveCodingQualificationCall,
) (map[assemblyline.WorkKind]int, int, int) {
	t.Helper()
	counts := map[assemblyline.WorkKind]int{}
	nonRuntimeCount, taskLocalCount := 0, 0
	for _, call := range calls {
		counts[call.kind]++
		for _, digest := range []string{
			call.jobSHA256, call.promptSHA256, call.requestSHA256,
			call.responseSHA256, call.candidateSHA256,
		} {
			if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != sha256.Size {
				t.Fatal("live qualification call lacks an exact digest")
			}
		}
		if call.promptBytes < 1 || call.promptTokens < 1 || call.outputTokens < 1 ||
			call.providerDuration <= 0 || call.wallDuration <= 0 {
			t.Fatal("live qualification call lacks bounded native metrics")
		}
		if call.kind != assemblyline.WorkApplicationRequirementCandidateKind {
			continue
		}
		switch call.candidate {
		case assemblyline.ApplicationRequirementCandidateNonRuntime:
			nonRuntimeCount++
		case assemblyline.ApplicationRequirementCandidateTaskLocal:
			taskLocalCount++
		default:
			t.Fatalf("live qualification candidate-kind result=%q", call.candidate)
		}
	}
	return counts, nonRuntimeCount, taskLocalCount
}

func liveCodingQualificationCoverageSnapshots(
	t *testing.T,
	calls []liveCodingQualificationCall,
) []*liveCodingQualificationCoverageSnapshot {
	t.Helper()
	var snapshots []*liveCodingQualificationCoverageSnapshot
	var relations []string
	for _, call := range calls {
		if call.kind == assemblyline.WorkApplicationRequirementCoverage {
			if call.coverageSnapshot == nil {
				t.Fatal("live coverage call lacks its validated authority snapshot")
			}
			snapshots = append(snapshots, call.coverageSnapshot)
			relations = append(relations, call.candidate)
		} else if call.coverageSnapshot != nil {
			t.Fatalf("non-coverage call %q carries a coverage snapshot", call.kind)
		}
	}
	if len(snapshots) == 0 || len(snapshots[0].AcceptedRequirements) != 0 ||
		len(snapshots[0].ExcludedCandidates) != 0 || len(snapshots[0].ZeroDeltas) != 0 {
		t.Fatal("live qualification lacks an empty initial coverage authority")
	}
	for index := 1; index < len(snapshots); index++ {
		previous, current := snapshots[index-1], snapshots[index]
		if !liveCodingQualificationPrefix(previous.AcceptedRequirements, current.AcceptedRequirements) ||
			!liveCodingQualificationPrefix(previous.ExcludedCandidates, current.ExcludedCandidates) ||
			!liveCodingQualificationPrefix(previous.ZeroDeltas, current.ZeroDeltas) ||
			len(current.AcceptedRequirements)-len(previous.AcceptedRequirements)+
				len(current.ZeroDeltas)-len(previous.ZeroDeltas) != 1 {
			t.Fatalf("live coverage snapshot %d did not preserve one code-owned transition", index)
		}
	}
	for index, relation := range relations {
		expected := assemblyline.ApplicationRequirementRemains
		if index == len(relations)-1 {
			expected = assemblyline.ApplicationNoUncoveredRequirement
		}
		if relation != expected {
			t.Fatalf("live coverage relation %d=%q, want %q", index, relation, expected)
		}
	}
	return snapshots
}

func liveCodingQualificationPrefix[T comparable](previous, current []T) bool {
	return len(current) >= len(previous) && slices.Equal(previous, current[:len(previous)])
}
