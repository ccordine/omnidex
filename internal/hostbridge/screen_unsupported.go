//go:build !linux

package hostbridge

import (
	"context"
	"fmt"
	"net/http"
)

func listScreenMonitors() ([]ScreenMonitor, string, error) {
	return nil, "", fmt.Errorf("screen streaming is only supported on Linux hosts")
}

func streamScreenMJPEG(ctx context.Context, w http.ResponseWriter, monitorID string, fps, quality, scalePct int) error {
	return fmt.Errorf("screen streaming is only supported on Linux hosts")
}
