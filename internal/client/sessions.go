package client

import (
	"context"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/sessionlist"
	"net/http"
	"net/url"
)

func (client *Client) CLISessions(ctx context.Context, root, identity string) (sessionlist.Listing, error) {
	var result sessionlist.Listing
	if err := model.ValidateChannelWorkspaceRoot(root); err != nil {
		return result, err
	}
	owner, _, err := projectroot.ParseClientWorkspaceIdentity(identity)
	if err != nil {
		return result, err
	}
	query := url.Values{"workspace_root": {root}, "workspace_identity": {identity}}
	result.Sessions = []sessionlist.Entry{}
	seen := make(map[model.ChannelID]bool)
	var after model.ChannelID
	for {
		var page sessionlist.Listing
		if err := client.doJSONBounded(ctx, http.MethodGet, "/v1/channels/cli-sessions?"+query.Encode(), nil, &page, http.StatusOK, 8*1024*1024); err != nil {
			return result, err
		}
		if page.Sessions == nil {
			return result, fmt.Errorf("sessions must be an array")
		}
		if len(page.Sessions) > sessionlist.PageSize {
			return result, fmt.Errorf("sessions page exceeds limit")
		}
		for _, entry := range page.Sessions {
			if err := entry.Validate(owner); err != nil {
				return result, err
			}
			if seen[entry.ID] || (after != "" && entry.ID <= after) {
				return result, fmt.Errorf("duplicate or nonadvancing CLI session")
			}
			seen[entry.ID] = true
		}
		result.Sessions = append(result.Sessions, page.Sessions...)
		if page.NextCursor == "" {
			return result, nil
		}
		if len(page.Sessions) == 0 || page.NextCursor != page.Sessions[len(page.Sessions)-1].ID || page.NextCursor <= after {
			return result, fmt.Errorf("invalid sessions continuation")
		}
		after = page.NextCursor
		query.Set("after", string(after))
	}
}
