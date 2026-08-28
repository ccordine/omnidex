package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	PlainTextArtifactTaskSchemaV1 = "omnidex.plain-text-artifact-task.v1"
	PlainTextArtifactStackID      = "plain_text_artifact_v1"
	PlainTextArtifactProfileID    = "plain_text_utf8_lf_v1"
	plainTextArtifactTaskID       = "task_001"
)

// FrozenPlainTextArtifactTask is the path-blind semantic authority for one
// standalone document. Artifact placement is deliberately absent and is bound
// later by the code-owned target tree and coverage plan.
type FrozenPlainTextArtifactTask struct {
	Schema                  string `json:"schema"`
	ID                      string `json:"id"`
	OriginalAuthoritySHA256 string `json:"original_authority_sha256"`
	Requirement             string `json:"requirement"`
	RequirementSHA256       string `json:"requirement_sha256"`
	SHA256                  string `json:"sha256"`
}

type plainTextArtifactTaskIdentity struct {
	Schema                  string `json:"schema"`
	ID                      string `json:"id"`
	OriginalAuthoritySHA256 string `json:"original_authority_sha256"`
	Requirement             string `json:"requirement"`
	RequirementSHA256       string `json:"requirement_sha256"`
}

// FreezePlainTextArtifactTask binds the immutable request to the exact
// path-redacted behavior used by the one source station. Neither value may be
// reconstructed from a semantic paraphrase.
func FreezePlainTextArtifactTask(
	originalAuthority string,
	pathRedactedRequirement string,
) (FrozenPlainTextArtifactTask, error) {
	var zero FrozenPlainTextArtifactTask
	if originalAuthority == "" || originalAuthority != strings.TrimSpace(originalAuthority) {
		return zero, fmt.Errorf("plain-text artifact task requires one exact trimmed authority")
	}
	if pathRedactedRequirement == "" ||
		pathRedactedRequirement != strings.TrimSpace(pathRedactedRequirement) {
		return zero, fmt.Errorf("plain-text artifact task requires one exact trimmed requirement")
	}
	if len(pathRedactedRequirement) > maxLocalBehaviorBytes {
		return zero, fmt.Errorf("plain-text artifact task requirement exceeds %d bytes", maxLocalBehaviorBytes)
	}
	if err := ValidatePathFreeModelContext(
		"plain-text artifact task requirement", pathRedactedRequirement,
	); err != nil {
		return zero, err
	}
	task := FrozenPlainTextArtifactTask{
		Schema:                  PlainTextArtifactTaskSchemaV1,
		ID:                      plainTextArtifactTaskID,
		OriginalAuthoritySHA256: ExactObjectiveContextSHA(originalAuthority),
		Requirement:             pathRedactedRequirement,
		RequirementSHA256:       ExactObjectiveContextSHA(pathRedactedRequirement),
	}
	digest, err := plainTextArtifactTaskDigest(task)
	if err != nil {
		return zero, err
	}
	task.SHA256 = digest
	if err := task.Validate(); err != nil {
		return zero, err
	}
	return task, nil
}

func (task FrozenPlainTextArtifactTask) Validate() error {
	if task.Schema != PlainTextArtifactTaskSchemaV1 || task.ID != plainTextArtifactTaskID {
		return fmt.Errorf("plain-text artifact task has invalid schema or code-owned identity")
	}
	if task.Requirement == "" || task.Requirement != strings.TrimSpace(task.Requirement) ||
		len(task.Requirement) > maxLocalBehaviorBytes {
		return fmt.Errorf("plain-text artifact task has invalid requirement")
	}
	if err := ValidatePathFreeModelContext(
		"plain-text artifact task requirement", task.Requirement,
	); err != nil {
		return err
	}
	if task.RequirementSHA256 != ExactObjectiveContextSHA(task.Requirement) ||
		!validPortableSHA256(task.OriginalAuthoritySHA256) {
		return fmt.Errorf("plain-text artifact task authority digest is invalid")
	}
	expected, err := plainTextArtifactTaskDigest(task)
	if err != nil {
		return err
	}
	if task.SHA256 != expected {
		return fmt.Errorf("plain-text artifact task differs from its frozen authority")
	}
	return nil
}

func plainTextArtifactTaskDigest(task FrozenPlainTextArtifactTask) (string, error) {
	encoded, err := json.Marshal(plainTextArtifactTaskIdentity{
		Schema: task.Schema, ID: task.ID,
		OriginalAuthoritySHA256: task.OriginalAuthoritySHA256,
		Requirement:             task.Requirement, RequirementSHA256: task.RequirementSHA256,
	})
	if err != nil {
		return "", fmt.Errorf("encode plain-text artifact task identity: %w", err)
	}
	return ExactObjectiveContextSHA(string(encoded)), nil
}

func validPortableSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

// PlainTextArtifactCoverage is code-owned provenance joining the frozen task
// to exactly one adapter-recognized target path.
type PlainTextArtifactCoverage struct {
	TaskSHA256 string
	Path       string
	Kind       TargetArtifactKind
}

func NewPlainTextArtifactCoverage(
	task FrozenPlainTextArtifactTask,
	target TargetTree,
) (PlainTextArtifactCoverage, error) {
	var zero PlainTextArtifactCoverage
	if err := task.Validate(); err != nil {
		return zero, err
	}
	if target.StackID != PlainTextArtifactStackID ||
		target.VersionProfileID != PlainTextArtifactProfileID || len(target.Paths) != 1 {
		return zero, fmt.Errorf("plain-text artifact target lacks exact stack, profile, or cardinality authority")
	}
	if !plainTextSourcePath(target.Paths[0]) {
		return zero, fmt.Errorf("plain-text artifact target %q is outside its selected adapter", target.Paths[0])
	}
	coverage := PlainTextArtifactCoverage{
		TaskSHA256: task.SHA256, Path: target.Paths[0], Kind: TargetArtifactImplementation,
	}
	return coverage, nil
}

// CompilePlainTextArtifactBlueprint is the focused stack compiler. It consumes
// frozen task, target, and coverage authorities and returns one neutral source
// document with one path-blind generated node.
func CompilePlainTextArtifactBlueprint(
	task FrozenPlainTextArtifactTask,
	target TargetTree,
	coverage PlainTextArtifactCoverage,
) (SourceBlueprint, error) {
	var zero SourceBlueprint
	expected, err := NewPlainTextArtifactCoverage(task, target)
	if err != nil {
		return zero, err
	}
	if coverage != expected {
		return zero, fmt.Errorf("plain-text artifact coverage differs from frozen task and target authority")
	}
	blueprint := SourceBlueprint{Documents: []SourceDocument{{
		ID: "plain_text_document", Path: coverage.Path, AdapterID: PlainTextAdapterID,
		Blocks: []SourceBlock{{
			ID: "plain_text_node", Signature: TextFragmentSignature,
			Contract: task.Requirement, API: TextFragmentSignature,
			TaskID: task.ID, Role: SourceBlockTaskImplementation,
		}},
	}}}
	if err := ValidatePlainTextSourceBlueprint(blueprint); err != nil {
		return zero, err
	}
	return blueprint, nil
}
