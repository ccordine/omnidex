package worker

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
)

func (runtime *nativeRuntimeV3) agentPlanningContext(request directCodingRequest, root string) ([]codingagent.ArtifactBinding, []codingagent.ConstructionFact, *codingagent.PlanningEnvironment, error) {
	session := &directCodingSession{runtime: runtime, request: request, root: root, protectedPaths: map[string]struct{}{}}
	intake, err := runtime.svc.repo.CodingAgentIntakeContext(runtime.ctx, runtime.claim.Authority)
	if err != nil {
		return nil, nil, nil, err
	}
	if intake == nil {
		return nil, nil, nil, fmt.Errorf("native task planning requires the original intake context snapshot")
	}
	inputs := directCodingApplicationInputs{RequestAuthority: directCodingApplicationRequestAuthority{modelRequest: intake.RedactedRequest}, Runtime: directCodingWorkerRuntime(session)}
	for _, binding := range intake.ArtifactBindings {
		inputs.Identities = append(inputs.Identities, assemblyline.ArtifactIdentity{Token: binding.Token, Value: binding.Value})
	}
	evidence := runtime.svc.repo.CodingAgentEvidence(runtime.claim.Authority)
	entries, err := evidence.Read(runtime.ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	var facts []codingagent.ConstructionFact
	retained := false
	for _, entry := range entries {
		if entry.Phase == "construction_context" {
			facts = entry.ConstructionFacts
			retained = true
		}
	}
	if !retained {
		facts, err = session.resolveAgentConstructionContext(inputs)
		if err != nil {
			return nil, nil, nil, err
		}
		if _, err = evidence.Append(runtime.ctx, codingagent.Entry{Generation: runtime.claim.Authority.Generation, Phase: "construction_context", ConstructionFacts: facts}); err != nil {
			return nil, nil, nil, err
		}
	}
	environment := &codingagent.PlanningEnvironment{NewFilesAllowed: true, Executables: []string{}}
	for _, name := range []string{"go", "python3", "sh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil && resolved != "" {
			environment.Executables = append(environment.Executables, name)
		}
	}
	return intake.ArtifactBindings, facts, environment, nil
}
