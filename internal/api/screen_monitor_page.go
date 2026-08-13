package api

import (
	"net/http"

	"github.com/gryph/omnidex/internal/hostbridge"
)

const screenMonitorUIPageSize = 10

func screenMonitorPageRequest(r *http.Request, fallbackLimit int) (hostbridge.ScreenMonitorPageRequest, error) {
	limit, err := exactChannelQueryInteger(r, "limit", fallbackLimit, 1, hostbridge.MaxScreenMonitorPageSize)
	if err != nil {
		return hostbridge.ScreenMonitorPageRequest{}, err
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		return hostbridge.ScreenMonitorPageRequest{}, err
	}
	return hostbridge.ScreenMonitorPageRequest{Limit: limit, Offset: offset}, nil
}
