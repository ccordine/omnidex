package api

import "github.com/gryph/omnidex/internal/model"

func scrumSyncTerminalPlayOutput(card ScrumCard, job model.JobDetails) (ScrumCard, error) {
	updated := card
	if synced, ok, err := syncRunningJobChannelChat(updated, job); err != nil {
		return card, err
	} else if ok {
		updated = synced
	}
	if synced, ok, err := syncRunningJobConsoleLog(updated, job); err != nil {
		return card, err
	} else if ok {
		updated = synced
	}
	return updated, nil
}
