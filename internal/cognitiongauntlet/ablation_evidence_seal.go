package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
)

const maxAblationEvidenceBytes = 256 * 1024 * 1024

func prepareAblationEvidence(
	evidencePath string,
	publicAuthority PublicRunAuthority,
	state *ablationState,
	journal *ablationCallJournal,
	terminal ablationPendingTerminal,
	terminalCause ablationTerminalCause,
	contextBudgetFailure *ablationContextBudgetFailure,
	bootstrap RuntimeBrainBootstrapEvidenceAuthority,
	activation RuntimeProviderActivationEvidenceAuthority,
) (ablationEvidenceArtifact, AblationEvidenceAuthority, error) {
	variant := publicAuthority.Variant
	class, private, err := ablationReplayClass(variant)
	publicAuthoritySHA, publicAuthorityErr := publicAuthority.SHA256()
	if err != nil || publicAuthorityErr != nil || terminal.Validate() != nil ||
		terminalCause.Validate() != nil || bootstrap.Validate() != nil || activation.Validate() != nil {
		return ablationEvidenceArtifact{}, AblationEvidenceAuthority{},
			fmt.Errorf("ablation evidence input authority is invalid: %v %v", err, publicAuthorityErr)
	}
	calls, err := journal.freeze()
	if err != nil {
		return ablationEvidenceArtifact{}, AblationEvidenceAuthority{}, err
	}
	builder := newAblationContentBuilder(private)
	callEvidence, err := buildAblationCalls(calls, builder)
	if err != nil {
		return ablationEvidenceArtifact{}, AblationEvidenceAuthority{}, err
	}
	contextBudget, err := buildAblationContextBudgetEvidence(contextBudgetFailure, builder)
	if err != nil {
		return ablationEvidenceArtifact{}, AblationEvidenceAuthority{}, err
	}
	ledger, err := freezeAblationLedger(state)
	if err != nil {
		return ablationEvidenceArtifact{}, AblationEvidenceAuthority{}, err
	}
	workingSet, err := freezeAblationWorkingSet(state)
	if err != nil {
		return ablationEvidenceArtifact{}, AblationEvidenceAuthority{}, err
	}
	root := ablationEvidenceRoot{
		Schema: AblationEvidenceSchemaV1, EpisodeID: state.episode.ID, Variant: variant,
		Class: class, PublicRunAuthority: publicAuthority,
		PublicRunAuthoritySHA256: publicAuthoritySHA,
		Actor:                    state.actor, Goal: state.goal.Clone(), Completion: state.completion.Clone(),
		Obligation:     state.obligation.Clone(),
		BrainBootstrap: bootstrap, ProviderActivation: activation,
		WorldCatalog: state.catalog.Clone(),
		Transitions:  cloneAblationTransitions(state.transitions), Calls: callEvidence,
		Actions:   cloneAblationActionEvidence(state.actionEvidence),
		NoActions: cloneAblationNoActionEvidence(state.noActions),
		Ledger:    ledger, WorkingSet: workingSet, ContextBudget: contextBudget,
		TerminalCause: terminalCause, Terminal: terminal,
		ChunkedBlobs: sortedAblationBindings(builder.bindings),
	}
	artifact := ablationEvidenceArtifact{
		Schema: AblationEvidenceSchemaV1, Root: root, Blobs: sortedAblationBlobs(builder.blobs),
	}
	if err := verifyAblationEvidenceArtifact(artifact); err != nil {
		return ablationEvidenceArtifact{}, AblationEvidenceAuthority{}, err
	}
	raw, err := encodeAblationEvidenceArtifact(artifact)
	if err != nil || len(raw) < 1 || len(raw) > maxAblationEvidenceBytes {
		return ablationEvidenceArtifact{}, AblationEvidenceAuthority{},
			fmt.Errorf("ablation evidence exceeds its exact byte bound: %v", err)
	}
	digest := digestExactBytes(raw)
	authority := AblationEvidenceAuthority{
		Schema: AblationEvidenceAuthoritySchemaV1, ID: "ablation_evidence_" + digest,
		SHA256: digest, Bytes: len(raw), File: "ablation-evidence.json",
	}
	if err := authority.Validate(); err != nil || evidencePath == "" {
		return ablationEvidenceArtifact{}, AblationEvidenceAuthority{},
			fmt.Errorf("ablation evidence authority is invalid: %v", err)
	}
	return artifact, authority, nil
}

func cloneAblationNoActionEvidence(
	values []ablationNoActionEvidence,
) []ablationNoActionEvidence {
	result := make([]ablationNoActionEvidence, len(values))
	copy(result, values)
	return result
}

func (authority AblationEvidenceAuthority) Validate() error {
	if authority.Schema != AblationEvidenceAuthoritySchemaV1 ||
		authority.ID != "ablation_evidence_"+authority.SHA256 || !validDigest(authority.SHA256) ||
		authority.Bytes < 1 || authority.Bytes > maxAblationEvidenceBytes ||
		authority.File != "ablation-evidence.json" ||
		filepath.Base(authority.File) != authority.File {
		return fmt.Errorf("ablation evidence authority is invalid")
	}
	return nil
}

func sealAblationEvidence(
	evidencePath string,
	artifact ablationEvidenceArtifact,
	authority AblationEvidenceAuthority,
) error {
	if err := verifyAblationEvidenceArtifact(artifact); err != nil {
		return err
	}
	raw, err := encodeAblationEvidenceArtifact(artifact)
	if err != nil || authority.Validate() != nil || len(raw) != authority.Bytes ||
		digestExactBytes(raw) != authority.SHA256 {
		return fmt.Errorf("ablation evidence differs from its authority")
	}
	return writeExclusiveAtomic(evidencePath, raw)
}

func sortedAblationBindings(
	values map[string]cognitionreplay.ChunkedBlobBinding,
) []cognitionreplay.ChunkedBlobBinding {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]cognitionreplay.ChunkedBlobBinding, len(keys))
	for index, key := range keys {
		result[index] = values[key]
	}
	return result
}

func sortedAblationBlobs(values map[string]cognitionreplay.Blob) []ablationEvidenceBlob {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]ablationEvidenceBlob, len(keys))
	for index, key := range keys {
		blob := values[key]
		result[index] = ablationEvidenceBlob{
			SHA256: blob.SHA256, MediaType: blob.MediaType,
			Data: append([]byte(nil), blob.Data...),
		}
	}
	return result
}

func cloneAblationTransitions(values []cognition.Transition) []cognition.Transition {
	result := make([]cognition.Transition, len(values))
	for index, value := range values {
		result[index] = value.Clone()
	}
	return result
}

func loadAblationEvidence(path string, authority AblationEvidenceAuthority) (ablationEvidenceArtifact, error) {
	if err := authority.Validate(); err != nil {
		return ablationEvidenceArtifact{}, err
	}
	if filepath.Base(path) != authority.File {
		return ablationEvidenceArtifact{}, fmt.Errorf("ablation evidence path differs from its authority")
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) != authority.Bytes || digestExactBytes(raw) != authority.SHA256 {
		return ablationEvidenceArtifact{}, fmt.Errorf("read exact ablation evidence: %v", err)
	}
	var artifact ablationEvidenceArtifact
	if err := decodeStrictJSON(raw, &artifact, "ablation evidence"); err != nil {
		return ablationEvidenceArtifact{}, err
	}
	canonical, err := encodeAblationEvidenceArtifact(artifact)
	if err != nil || string(canonical) != string(raw) || verifyAblationEvidenceArtifact(artifact) != nil {
		return ablationEvidenceArtifact{}, fmt.Errorf("ablation evidence encoding or authority changed")
	}
	return artifact, nil
}
