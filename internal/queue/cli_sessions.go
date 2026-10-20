package queue

import (
	"context"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/sessionlist"
)

func (r *Repository) CLISessions(ctx context.Context, clientID string, after model.ChannelID) (sessionlist.Listing, error) {
	result := sessionlist.Listing{Sessions: []sessionlist.Entry{}}
	if after != "" && !projectroot.IsCLIChatChannelID(after) {
		return result, fmt.Errorf("invalid sessions cursor")
	}
	if err := projectroot.ValidateClientIdentity(clientID); err != nil {
		return result, err
	}
	if ctx == nil || r == nil || r.pool == nil {
		return result, fmt.Errorf("CLI sessions require PostgreSQL and context")
	}
	rows, err := r.pool.Query(ctx, `
 SELECT c.id, c.workspace_root, c.cli_workspace_identity,
 COALESCE(j.status, 'idle'), statement_timestamp()
 FROM ai_channels c
 LEFT JOIN LATERAL (
 SELECT status FROM jobs
 WHERE pipeline='chat' AND metadata->>'channel_id'=c.id
 AND status IN ('pending','running','waiting_input')
 ORDER BY id DESC LIMIT 1
 ) j ON TRUE
 WHERE starts_with(c.cli_workspace_identity, $1)
 AND c.id > $2
 ORDER BY c.id LIMIT $3`, "client_"+clientID+"_", after, sessionlist.PageSize+1)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry sessionlist.Entry
		if err := rows.Scan(&entry.ID, &entry.Directory, &entry.WorkspaceIdentity, &entry.Activity, &entry.ObservedAt); err != nil {
			return result, err
		}
		if err := entry.Validate(clientID); err != nil {
			return result, err
		}
		result.Sessions = append(result.Sessions, entry)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Sessions) > sessionlist.PageSize {
		result.Sessions = result.Sessions[:sessionlist.PageSize]
		result.NextCursor = result.Sessions[len(result.Sessions)-1].ID
	}
	return result, nil
}
