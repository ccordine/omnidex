package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

type RoleplayResearchPreparation struct {
	Command    roleplay.ResearchCommand
	Simulation roleplay.SimulationTurnAuthority
	Research   roleplay.ResearchTurnAuthority
}

// PrepareRoleplayResearchTurnTx is the sole production boundary that may
// designate a roleplay turn as an external command. Unmatched text is returned
// untouched; malformed text in the reserved namespace fails loudly.
func PrepareRoleplayResearchTurnTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	userMessageID int64,
	exact string,
) (RoleplayResearchPreparation, bool, error) {
	if ctx == nil || tx == nil {
		return RoleplayResearchPreparation{}, false, fmt.Errorf(
			"roleplay research preparation requires transaction authority",
		)
	}
	command, matched, err := roleplay.ParseResearchCommand(exact)
	if err != nil || !matched {
		return RoleplayResearchPreparation{}, matched, err
	}
	operationID, err := roleplay.NewSimulationTransitionIdentity()
	if err != nil {
		return RoleplayResearchPreparation{}, true, err
	}
	simulation, err := roleplay.PrepareSimulationTurnTx(
		ctx, tx, roleplay.SimulationTurnPreparationRequest{
			OperationID: operationID, ChannelID: channelID,
			UserMessageID: userMessageID, InputKind: roleplay.SimulationTurnExternalCommand,
		},
	)
	if err != nil {
		return RoleplayResearchPreparation{}, true, err
	}
	research, err := roleplay.AuthorizeResearchPreparationTx(ctx, tx, simulation, command)
	if err != nil {
		return RoleplayResearchPreparation{}, true, err
	}
	return RoleplayResearchPreparation{
		Command: command, Simulation: simulation, Research: research,
	}, true, nil
}

func BindRoleplayResearchTurnJobTx(
	ctx context.Context,
	tx pgx.Tx,
	preparation RoleplayResearchPreparation,
	jobID int64,
) error {
	if ctx == nil || tx == nil {
		return fmt.Errorf("roleplay research job binding requires transaction authority")
	}
	if err := preparation.Research.Validate(); err != nil {
		return err
	}
	if preparation.Simulation.PreparationID != preparation.Research.PreparationID {
		return fmt.Errorf("roleplay research and simulation preparation identities differ")
	}
	if err := roleplay.BindSimulationPreparationJobTx(
		ctx, tx, preparation.Simulation.PreparationID, jobID,
	); err != nil {
		return err
	}
	return roleplay.BindResearchPreparationJobTx(
		ctx, tx, preparation.Research.PreparationID, jobID,
	)
}
