package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const maxJobItemPathBytes = 128

type jobItemAction string

const (
	jobItemRead          jobItemAction = ""
	jobItemHistory       jobItemAction = "history"
	jobItemFeedback      jobItemAction = "feedback"
	jobItemInterrupt     jobItemAction = "interrupt"
	jobItemReplan        jobItemAction = "replan"
	jobItemCancel        jobItemAction = "cancel"
	jobItemPlanRead      jobItemAction = "plan"
	jobItemPlanDecisions jobItemAction = "plan/decisions"
	jobItemPlanFreeze    jobItemAction = "plan/freeze"
)

func decodeJobItemRoute(request *http.Request) (int64, jobItemAction, error) {
	if request == nil || request.URL == nil {
		return 0, "", fmt.Errorf("job item request URL is required")
	}
	path := request.URL.Path
	if len(path) > maxJobItemPathBytes || request.URL.EscapedPath() != path {
		return 0, "", fmt.Errorf("job item path must be one exact canonical path")
	}
	const prefix = "/v1/jobs/"
	if !strings.HasPrefix(path, prefix) {
		return 0, "", fmt.Errorf("job item path is not registered")
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) < 1 || len(parts) > 3 || parts[0] == "" {
		return 0, "", fmt.Errorf("job item path is not registered")
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != parts[0] {
		return 0, "", fmt.Errorf("job ID must be one canonical positive integer")
	}
	action := jobItemRead
	if len(parts) >= 2 {
		action = jobItemAction(strings.Join(parts[1:], "/"))
		switch action {
		case jobItemHistory, jobItemFeedback, jobItemInterrupt, jobItemReplan, jobItemCancel,
			jobItemPlanRead, jobItemPlanDecisions, jobItemPlanFreeze:
		default:
			return 0, "", fmt.Errorf("job item action is not registered")
		}
	}
	return id, action, nil
}
