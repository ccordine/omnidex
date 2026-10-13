package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/chatcommands"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/projectroot"
)

func objectiveConversationCLICommands(authority turnAuthority) ([]assemblyline.ConversationCLICommandReference, error) {
	if authority.ClientWorkspaceIdentity == "" || authority.ChannelMode != model.ChannelModeAssistant {
		return nil, nil
	}
	if err := projectroot.ValidateClientWorkspaceIdentity(authority.ClientWorkspaceIdentity); err != nil {
		return nil, fmt.Errorf("conversation CLI workspace origin: %w", err)
	}
	references := make([]assemblyline.ConversationCLICommandReference, 0)
	provenance, err := objectiveModelProvenance(authority)
	if err != nil {
		return nil, err
	}
	mentioned := make(map[string]struct{})
	for _, identity := range modelcontext.PathIdentities(authority.Instruction, provenance) {
		mentioned[identity.Value] = struct{}{}
	}
	for _, identity := range authority.ModelArtifactIdentities {
		if _, referenced := mentioned[identity.Value]; !referenced {
			continue
		}
		definition, registered := chatcommands.LookupReference(identity.Value)
		if !registered {
			continue
		}
		references = append(references, assemblyline.ConversationCLICommandReference{
			Token: identity.Token, Command: definition.Name,
		})
	}
	return references, nil
}
