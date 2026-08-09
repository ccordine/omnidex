package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gryph/omnidex/internal/queue"
)

func (c *Client) JobHistory(
	ctx context.Context,
	jobID int64,
	request queue.JobHistoryRequest,
) (queue.JobHistoryPage, error) {
	if jobID <= 0 {
		return queue.JobHistoryPage{}, fmt.Errorf("job history requires a positive job ID")
	}
	if request.Limit < 1 || request.Limit > queue.MaxJobHistoryPageSize {
		return queue.JobHistoryPage{}, fmt.Errorf(
			"job history limit must be between 1 and %d", queue.MaxJobHistoryPageSize,
		)
	}
	values := url.Values{}
	values.Set("stream", string(request.Stream))
	values.Set("limit", strconv.Itoa(request.Limit))
	if request.Cursor != "" {
		values.Set("cursor", request.Cursor)
	}
	path := fmt.Sprintf("/v1/jobs/%d/history?%s", jobID, values.Encode())
	var page queue.JobHistoryPage
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &page); err != nil {
		return queue.JobHistoryPage{}, err
	}
	if page.JobID != jobID || page.Stream != request.Stream {
		return queue.JobHistoryPage{}, fmt.Errorf(
			"job history response authority is %d/%q, expected %d/%q",
			page.JobID, page.Stream, jobID, request.Stream,
		)
	}
	return page, nil
}
