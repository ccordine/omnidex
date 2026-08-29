package specialists

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const learnedSkillIDPrefix = "learned_"

type SkillStatus string

const (
	SkillStatusActive SkillStatus = "active"
)

type SkillSource string

const (
	SkillSourceLearned SkillSource = "learned"
)

type SkillKind string

const (
	SkillKindCodeProcedure SkillKind = "code_procedure"
)

type SkillVersion struct {
	Spec           Spec
	Version        int
	Status         SkillStatus
	Source         SkillSource
	Kind           SkillKind
	CreatedByJobID *int64
	ContentSHA256  string
}

func (version SkillVersion) Validate() error {
	if err := version.Spec.Validate(); err != nil {
		return err
	}
	if version.Version < 1 {
		return fmt.Errorf("skill %s version must be positive", version.Spec.ID)
	}
	if version.Status != SkillStatusActive {
		return fmt.Errorf("skill %s has invalid status %q", version.Spec.ID, version.Status)
	}
	switch version.Source {
	case SkillSourceLearned:
		if version.Kind != SkillKindCodeProcedure {
			return fmt.Errorf("learned skill %s has unsupported kind %q", version.Spec.ID, version.Kind)
		}
		if version.CreatedByJobID == nil || *version.CreatedByJobID < 1 {
			return fmt.Errorf("learned skill %s requires a creating job", version.Spec.ID)
		}
		if !validLearnedSkillID(version.Spec.ID) {
			return fmt.Errorf("learned skill id %q must be code-owned", version.Spec.ID)
		}
	default:
		return fmt.Errorf("skill %s has invalid source %q", version.Spec.ID, version.Source)
	}
	wantHash, err := SkillContentHash(version.Spec, version.Kind)
	if err != nil {
		return err
	}
	if version.ContentSHA256 != wantHash {
		return fmt.Errorf("skill %s content hash does not match its immutable contract", version.Spec.ID)
	}
	return nil
}

func validLearnedSkillID(id string) bool {
	if !strings.HasPrefix(id, learnedSkillIDPrefix) || len(id) != len(learnedSkillIDPrefix)+32 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(id, learnedSkillIDPrefix))
	return err == nil
}

func SkillContentHash(spec Spec, kind SkillKind) (string, error) {
	if err := spec.Validate(); err != nil {
		return "", err
	}
	payload := struct {
		Kind         SkillKind `json:"kind,omitempty"`
		ID           string    `json:"id"`
		Purpose      string    `json:"purpose"`
		Instructions string    `json:"instructions"`
	}{
		Kind: kind,
		ID:   strings.TrimSpace(spec.ID), Purpose: strings.TrimSpace(spec.Purpose),
		Instructions: strings.TrimSpace(spec.Instructions),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode skill %s immutable contract: %w", spec.ID, err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
