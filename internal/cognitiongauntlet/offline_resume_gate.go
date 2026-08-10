package cognitiongauntlet

import "fmt"

func deriveOfflineResumeGate(
	runs []OfflineResumeRunReceipt,
	baseline ResumeBaselineArtifact,
) OfflineResumeGate {
	gate := OfflineResumeGate{
		RequiredSchedules: len(runs), Reasons: []string{},
	}
	for index, run := range runs {
		gate.RequiredInterruptions += resumeRequiredInterruptions(run)
		gate.ProvenInterruptions += len(run.Interruptions)
		gate.RestorationMismatches += run.Recovery.RestorationMismatches
		gate.ProjectionMismatches += run.Recovery.ProjectionMismatches
		semanticsMatch := run.Semantics == baseline.Semantics
		if run.Schedule.Kind == ResumeLiveInferenceExpiry {
			semanticsMatch = liveResumeSemanticsMatch(run.Semantics, baseline.Semantics)
		}
		if !semanticsMatch {
			gate.SemanticMismatches++
		}
		if run.LiveStaleProbe != nil {
			gate.StaleWriteClasses += liveStaleWriteClassCount(*run.LiveStaleProbe)
		}
		if err := run.validate(run.Schedule, baseline); err != nil {
			gate.Reasons = append(gate.Reasons, fmt.Sprintf("run_%d:%v", index+1, err))
			continue
		}
		gate.QualifiedSchedules++
	}
	gate.Passed = gate.RequiredSchedules == 5 && gate.QualifiedSchedules == gate.RequiredSchedules &&
		gate.ProvenInterruptions == gate.RequiredInterruptions && gate.StaleWriteClasses == 5 &&
		gate.SemanticMismatches == 0 && gate.RestorationMismatches == 0 &&
		gate.ProjectionMismatches == 0
	if gate.Passed && len(gate.Reasons) != 0 {
		gate.Passed = false
	}
	if !gate.Passed && len(gate.Reasons) == 0 {
		gate.Reasons = append(gate.Reasons, "Resume promotion requirements are incomplete")
	}
	return gate
}

func resumeRequiredInterruptions(run OfflineResumeRunReceipt) int {
	switch run.Schedule.Kind {
	case ResumeUninterrupted:
		return 0
	case ResumeEveryDecision:
		if run.Semantics.ModelDecisions > 0 {
			return run.Semantics.ModelDecisions - 1
		}
		return 0
	default:
		return run.Schedule.RequiredKills
	}
}

func liveStaleWriteClassCount(receipt LiveStaleProbeReceipt) int {
	count := 0
	for _, proof := range receipt.Probes {
		if proof.Validate() == nil {
			count += proof.Port.writeClasses()
		}
	}
	return count
}
