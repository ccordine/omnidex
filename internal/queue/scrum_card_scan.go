package queue

import "github.com/jackc/pgx/v5"

func scanDBScrumCard(row pgx.Row) (DBScrumCard, error) {
	var card DBScrumCard
	err := row.Scan(
		&card.ID, &card.ProjectID, &card.Title, &card.Description, &card.Column,
		&card.Checklist, &card.RefFiles, &card.CardTicket, &card.CardPrompt,
		&card.Tags, &card.TestCriteria, &card.FlowMetrics,
		&card.JobID, &card.PlayState,
		&card.QueueOrder, &card.BoardOrder,
		&card.ChannelMessageCount, &card.ChannelContentBytes,
		&card.CreatedAt, &card.UpdatedAt,
	)
	if err != nil {
		return DBScrumCard{}, err
	}
	if err := validateStoredScrumCard(card); err != nil {
		return DBScrumCard{}, err
	}
	return card, nil
}
