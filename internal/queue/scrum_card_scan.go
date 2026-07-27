package queue

import "github.com/jackc/pgx/v5"

func scanDBScrumCard(row pgx.Row) (DBScrumCard, error) {
	var card DBScrumCard
	err := row.Scan(
		&card.ID,
		&card.ProjectID,
		&card.Title,
		&card.Description,
		&card.Column,
		&card.Checklist,
		&card.RefFiles,
		&card.Chat,
		&card.ModelConfig,
		&card.AgentConfig,
		&card.CardTicket,
		&card.CardPrompt,
		&card.RecipeID,
		&card.Recipe,
		&card.Tags,
		&card.PlanningChat,
		&card.CoachConfig,
		&card.TestCriteria,
		&card.FlowMetrics,
		&card.JobID,
		&card.TagsJobID,
		&card.TicketJobID,
		&card.ConsoleLog,
		&card.PlayState,
		&card.QueueOrder,
		&card.BoardOrder,
		&card.CreatedAt,
		&card.UpdatedAt,
	)
	if err != nil {
		return DBScrumCard{}, err
	}
	sanitizeScrumCardFields(&card)
	return card, nil
}
