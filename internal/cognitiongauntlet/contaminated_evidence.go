package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

// ContaminatedEvidencePacket is available only to the explicitly labeled
// oracle-evidence ceiling. It is never valid for a normative variant.
type ContaminatedEvidencePacket struct {
	Witness      []labyrinth.WitnessAction `json:"witness"`
	EvidenceUses []labyrinth.EvidenceUse   `json:"evidence_uses"`
}

func (packet ContaminatedEvidencePacket) Validate() error {
	if packet.Witness == nil || packet.EvidenceUses == nil || len(packet.Witness) == 0 {
		return fmt.Errorf("contaminated evidence packet authority is incomplete")
	}
	actions := make(map[cognition.ActionID]int, len(packet.Witness))
	for index, action := range packet.Witness {
		if action.ID == "" || action.Schema.Validate() != nil || action.Request.Validate() != nil ||
			action.Cost <= 0 {
			return fmt.Errorf("contaminated evidence witness action %d is invalid", index+1)
		}
		if _, duplicate := actions[action.ID]; duplicate {
			return fmt.Errorf("contaminated evidence witness action identity repeats")
		}
		actions[action.ID] = index
	}
	for index, use := range packet.EvidenceUses {
		acquired, acquisitionExists := actions[use.AcquisitionActionID]
		consumed, consumerExists := actions[use.RequiredByActionID]
		if use.Evidence.ID == "" || !validDigest(use.Evidence.SHA256) ||
			!acquisitionExists || !consumerExists || acquired >= consumed {
			return fmt.Errorf("contaminated evidence use %d is invalid", index+1)
		}
	}
	return nil
}

func contaminatedEvidenceFor(generated generatedOfflineScenario) (ContaminatedEvidencePacket, error) {
	authority := generated.evidenceAuthority()
	packet := ContaminatedEvidencePacket{
		Witness:      append([]labyrinth.WitnessAction{}, authority.Witness...),
		EvidenceUses: append([]labyrinth.EvidenceUse{}, authority.EvidenceUses...),
	}
	return packet, packet.Validate()
}
