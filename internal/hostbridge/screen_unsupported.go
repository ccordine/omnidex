//go:build !linux

package hostbridge

import (
	"context"
	"fmt"
	"net/http"
)

func listScreenMonitorPage(ScreenMonitorPageRequest) (ScreenMonitorPage, error) {
	return ScreenMonitorPage{}, fmt.Errorf("screen streaming is only supported on Linux hosts")
}

func streamScreenMJPEG(ctx context.Context, w http.ResponseWriter, monitorID string, fps, quality, scalePct int) error {
	return fmt.Errorf("screen streaming is only supported on Linux hosts")
}
