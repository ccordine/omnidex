package specialists

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/gryph/omnidex/internal/modelcontext"
)

const (
	maxSkillPurposeBytes      = 1536
	maxSkillInstructionsBytes = 1536
)

var forbiddenSkillFrameworkControlTerms = []string{
	"downstream agent",
	"downstream agents",
	"model agent",
	"model agents",
	"semantic agent",
	"semantic agents",
	"downstream worker",
	"downstream workers",
	"model worker",
	"model workers",
	"semantic worker",
	"semantic workers",
	"act as orchestrator",
	"act as an orchestrator",
	"acts as orchestrator",
	"acts as an orchestrator",
	"model orchestration",
	"semantic orchestration",
	"tool call",
	"tool calls",
	"tool choice",
	"tool selection",
	"control plane",
	"workflow decision",
	"workflow control",
	"state owner",
	"state ownership",
	"permission to continue",
	"approval to continue",
	"completion status",
	"completion claim",
	"completeness review",
	"quality review",
	"retry decision",
	"task queue",
	"downstream code",
	"code owns",
	"code alone",
	"code owned",
	"code proven",
	"code bound",
	"code established",
	"code selected",
	"code reserved",
	"exact output limit evidence",
	"source owning",
	"later station",
	"separate station",
	"this call sees",
	"independently sieve",
	"authorize or discard",
	"authorizes and resolves",
	"not canon generation",
	"decide completeness",
	"review accepted",
	"review retained",
	"reopen accepted",
	"revoke accepted",
}

type Spec struct {
	ID           string `json:"id"`
	Purpose      string `json:"purpose"`
	Instructions string `json:"instructions,omitempty"`
}

func (s Spec) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("specialist id is required")
	}
	if err := validateSkillModelText("purpose", s.ID, s.Purpose, maxSkillPurposeBytes); err != nil {
		return err
	}
	if err := validateSkillModelText(
		"instructions", s.ID, s.Instructions, maxSkillInstructionsBytes,
	); err != nil {
		return err
	}
	return nil
}

func validateSkillModelText(label, id, value string, maxBytes int) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("specialist %s %s must be non-empty exact-trimmed text", id, label)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("specialist %s %s exceeds %d bytes", id, label, maxBytes)
	}
	if modelcontext.ContainsPathIdentity(value) {
		return fmt.Errorf("specialist %s %s contains a filesystem identity", id, label)
	}
	if term := skillFrameworkControlTerm(value); term != "" {
		return fmt.Errorf(
			"specialist %s %s contains framework-control language %q", id, label, term,
		)
	}
	return nil
}

func skillFrameworkControlTerm(value string) string {
	normalized := strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToLower(character)
		}
		return ' '
	}, value)
	framed := " " + strings.Join(strings.Fields(normalized), " ") + " "
	for _, term := range forbiddenSkillFrameworkControlTerms {
		if strings.Contains(framed, " "+term+" ") {
			return term
		}
	}
	return ""
}
