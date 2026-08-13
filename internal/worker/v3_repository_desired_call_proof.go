package worker

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

const maxDesiredRepositoryTargetPaths = 8

var desiredRepositoryShellMutationPattern = regexp.MustCompile(
	`(?i)(?:^|[;&|[:space:]])(?:rm|mv|sed)[[:space:]]+(?:-|[^[:space:]])`,
)

type desiredRepositoryCallProof struct {
	TotalModelCalls                 int
	SemanticGapCalls                int
	DeclarationGenerationCalls      int
	DeclarationCorrectionCalls      int
	ModelSelectedMutationOperations int
	ModelVisibleTargetPaths         int
}

func (session *directCodingSession) proveDesiredRepositoryCalls(
	graphTargetPaths []string,
) (desiredRepositoryCallProof, error) {
	if session == nil || session.runtime == nil || session.runtime.svc == nil ||
		session.runtime.svc.repo == nil || session.runtime.claim == nil {
		return desiredRepositoryCallProof{}, fmt.Errorf(
			"desired repository call proof requires one active durable attempt",
		)
	}
	evidence, err := session.runtime.svc.repo.StationAttemptCallEvidence(
		session.runtime.ctx, session.runtime.claim.Authority,
	)
	if err != nil {
		return desiredRepositoryCallProof{}, fmt.Errorf(
			"prove desired repository calls from immutable station evidence: %w", err,
		)
	}
	return compileDesiredRepositoryCallProof(evidence, graphTargetPaths)
}

func compileDesiredRepositoryCallProof(
	evidence []queue.StationAttemptCallEvidence,
	targetPaths []string,
) (desiredRepositoryCallProof, error) {
	var proof desiredRepositoryCallProof
	if len(targetPaths) > maxDesiredRepositoryTargetPaths {
		return proof, fmt.Errorf(
			"desired repository call proof exceeds %d target paths",
			maxDesiredRepositoryTargetPaths,
		)
	}
	for _, target := range targetPaths {
		if target == "" || target != strings.TrimSpace(target) || len(target) > 4096 ||
			!utf8.ValidString(target) || strings.ContainsRune(target, '\x00') {
			return proof, fmt.Errorf("desired repository call proof has invalid target path authority")
		}
	}
	for _, call := range evidence {
		if call.OpeningID < 1 {
			return proof, fmt.Errorf("desired repository call proof has invalid opening identity")
		}
		kind, err := desiredRepositoryOriginalWorkKind(call.WorkKind, call.Payload)
		if err != nil {
			return proof, fmt.Errorf("classify station call %d: %w", call.OpeningID, err)
		}
		proof.TotalModelCalls++
		switch kind {
		case assemblyline.WorkFragmentGeneration:
			if call.WorkKind != assemblyline.WorkFragmentGeneration {
				proof.DeclarationCorrectionCalls++
			} else {
				proof.DeclarationGenerationCalls++
			}
		case assemblyline.WorkFragmentCorrection:
			proof.DeclarationCorrectionCalls++
		default:
			proof.SemanticGapCalls++
		}
		for _, target := range targetPaths {
			if desiredRepositoryCallContainsTarget(call.Prompt, target) ||
				desiredRepositoryCallContainsTarget(call.Response, target) {
				proof.ModelVisibleTargetPaths++
			}
		}
		if desiredRepositoryContainsPhysicalOperation(call.Response) {
			proof.ModelSelectedMutationOperations++
		}
	}
	if proof.ModelVisibleTargetPaths != 0 {
		return proof, fmt.Errorf(
			"desired repository model-visible target path count is %d; expected zero before mutation",
			proof.ModelVisibleTargetPaths,
		)
	}
	if proof.ModelSelectedMutationOperations != 0 {
		return proof, fmt.Errorf(
			"desired repository model-selected mutation operation count is %d; expected zero",
			proof.ModelSelectedMutationOperations,
		)
	}
	return proof, nil
}

func desiredRepositoryCallContainsTarget(value, target string) bool {
	if strings.Contains(value, target) {
		return true
	}
	base := path.Base(target)
	return base != "." && base != "/" && base != target && strings.Contains(value, base)
}

func desiredRepositoryOriginalWorkKind(
	kind assemblyline.WorkKind,
	payload string,
) (assemblyline.WorkKind, error) {
	if kind != assemblyline.WorkResponseCorrection {
		return kind, nil
	}
	var correction assemblyline.ResponseCorrectionInput
	if err := json.Unmarshal([]byte(payload), &correction); err != nil {
		return "", fmt.Errorf("decode response correction authority: %w", err)
	}
	if err := correction.Original.Validate(); err != nil {
		return "", fmt.Errorf("validate response correction original: %w", err)
	}
	if correction.Original.Kind == assemblyline.WorkResponseCorrection {
		return "", fmt.Errorf("nested response correction is unsupported")
	}
	return correction.Original.Kind, nil
}

func desiredRepositoryContainsPhysicalOperation(value string) bool {
	lower := strings.ToLower(value)
	for _, operation := range []string{
		"create_file", "delete_file", "rename_file", "move_file", "write_file",
	} {
		if strings.Contains(lower, operation) {
			return true
		}
	}
	return desiredRepositoryShellMutationPattern.MatchString(value)
}

func desiredRepositoryMutationOperations(
	compiled desiredRepositoryCompileResult,
) (int, error) {
	if len(compiled.States) < 1 || len(compiled.States) > maxDesiredRepositoryTargetPaths {
		return 0, fmt.Errorf("desired repository mutation operation evidence is incomplete")
	}
	operations := 0
	for _, state := range compiled.States {
		switch {
		case state.Present && state.Source.FileID == "":
			operations++
		case !state.Present && state.Source.FileID != "":
			operations++
		case state.Present && state.Source.FileID != "":
			operations++
		default:
			return 0, fmt.Errorf("desired repository state has no exact physical transition")
		}
	}
	if operations != len(compiled.States) {
		return 0, fmt.Errorf("desired repository mutation operation evidence is incomplete")
	}
	return operations, nil
}
