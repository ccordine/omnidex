//go:build linux

package hostbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const screenMJPEGBoundary = "omniscreen"

func listScreenMonitors() ([]ScreenMonitor, string, error) {
	if monitors, err := listHyprlandMonitors(); err == nil && len(monitors) > 0 {
		return monitors, "hyprland-grim", nil
	}
	if monitors, err := listXRandRMonitors(); err == nil && len(monitors) > 0 {
		return monitors, "x11", nil
	}
	return nil, "", fmt.Errorf("no monitors found; on Hyprland install grim and hyprctl, on X11 install xrandr, and ensure ffmpeg is installed")
}

func streamScreenMJPEG(ctx context.Context, w http.ResponseWriter, monitorID string, fps, quality, scalePct int) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg is required for screen streaming")
	}

	monitors, backend, err := listScreenMonitors()
	if err != nil {
		return err
	}
	monitor, err := pickScreenMonitor(monitors, monitorID)
	if err != nil {
		return err
	}

	switch backend {
	case "x11":
		return streamX11MJPEG(ctx, w, monitor, fps, quality, scalePct)
	default:
		return streamGrimMJPEG(ctx, w, monitor, fps, quality, scalePct)
	}
}

func listHyprlandMonitors() ([]ScreenMonitor, error) {
	if _, err := exec.LookPath("hyprctl"); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("grim"); err != nil {
		return nil, err
	}
	out, err := exec.Command("hyprctl", "monitors", "-j").Output()
	if err != nil {
		return nil, err
	}
	var payload []struct {
		ID      int     `json:"id"`
		Name    string  `json:"name"`
		Width   int     `json:"width"`
		Height  int     `json:"height"`
		X       int     `json:"x"`
		Y       int     `json:"y"`
		Scale   float64 `json:"scale"`
		Focused bool    `json:"focused"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, err
	}
	monitors := make([]ScreenMonitor, 0, len(payload))
	for _, item := range payload {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		monitors = append(monitors, ScreenMonitor{
			ID:      name,
			Name:    name,
			Width:   item.Width,
			Height:  item.Height,
			X:       item.X,
			Y:       item.Y,
			Primary: item.Focused,
		})
	}
	if len(monitors) == 0 {
		return nil, fmt.Errorf("hyprctl returned no monitors")
	}
	return monitors, nil
}

var xrandrMonitorLine = regexp.MustCompile(`^\s*\d+:\s+([+\*]*)([\w-]+)\s+(\d+)/\d+x(\d+)/\d+\+(\d+)\+(\d+)`)

func listXRandRMonitors() ([]ScreenMonitor, error) {
	if _, err := exec.LookPath("xrandr"); err != nil {
		return nil, err
	}
	out, err := exec.Command("xrandr", "--listmonitors").Output()
	if err != nil {
		return nil, err
	}
	monitors := make([]ScreenMonitor, 0, 4)
	for _, line := range strings.Split(string(out), "\n") {
		match := xrandrMonitorLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		width, _ := strconv.Atoi(match[3])
		height, _ := strconv.Atoi(match[4])
		x, _ := strconv.Atoi(match[5])
		y, _ := strconv.Atoi(match[6])
		name := strings.TrimSpace(match[2])
		monitors = append(monitors, ScreenMonitor{
			ID:      name,
			Name:    name,
			Width:   width,
			Height:  height,
			X:       x,
			Y:       y,
			Primary: strings.Contains(match[1], "*"),
		})
	}
	if len(monitors) == 0 {
		return nil, fmt.Errorf("xrandr returned no monitors")
	}
	return monitors, nil
}

func streamX11MJPEG(ctx context.Context, w http.ResponseWriter, monitor ScreenMonitor, fps, quality, scalePct int) error {
	display := strings.TrimSpace(os.Getenv("DISPLAY"))
	if display == "" {
		display = ":0"
	}

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "x11grab", "-draw_mouse", "1",
		"-framerate", strconv.Itoa(fps),
		"-video_size", fmt.Sprintf("%dx%d", monitor.Width, monitor.Height),
		"-i", fmt.Sprintf("%s+%d,%d", display, monitor.X, monitor.Y),
		"-an",
	}
	args = append(args, screenScaleFilter(scalePct)...)
	args = append(args, "-f", "mpjpeg", "-q:v", strconv.Itoa(quality), "-")

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg x11grab failed: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=ffmpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Omni-Screen-Monitor", monitor.Name)
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	return copyStream(ctx, w, flusher, stdout)
}

func streamGrimMJPEG(ctx context.Context, w http.ResponseWriter, monitor ScreenMonitor, fps, quality, scalePct int) error {
	if _, err := exec.LookPath("grim"); err != nil {
		return fmt.Errorf("grim is required for Wayland/Hyprland screen streaming")
	}

	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+screenMJPEGBoundary)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Omni-Screen-Monitor", monitor.Name)
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported by response writer")
	}

	interval := time.Second / time.Duration(fps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var encodeMu sync.Mutex
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			jpeg, err := captureGrimJPEG(ctx, monitor.Name, quality, scalePct)
			if err != nil {
				continue
			}
			encodeMu.Lock()
			if _, err := fmt.Fprintf(w, "--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", screenMJPEGBoundary, len(jpeg)); err != nil {
				encodeMu.Unlock()
				return err
			}
			if _, err := w.Write(jpeg); err != nil {
				encodeMu.Unlock()
				return err
			}
			if _, err := io.WriteString(w, "\r\n"); err != nil {
				encodeMu.Unlock()
				return err
			}
			flusher.Flush()
			encodeMu.Unlock()
		}
	}
}

func captureGrimJPEG(ctx context.Context, monitorName string, quality, scalePct int) ([]byte, error) {
	grim := exec.CommandContext(ctx, "grim", "-o", monitorName, "-")
	png, err := grim.Output()
	if err != nil {
		return nil, err
	}

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "image2pipe", "-i", "pipe:0",
		"-frames:v", "1",
	}
	args = append(args, screenScaleFilter(scalePct)...)
	args = append(args, "-f", "mjpeg", "-q:v", strconv.Itoa(quality), "pipe:1")

	ffmpeg := exec.CommandContext(ctx, "ffmpeg", args...)
	ffmpeg.Stdin = bytes.NewReader(png)
	return ffmpeg.Output()
}

func screenScaleFilter(scalePct int) []string {
	if scalePct >= 100 {
		return nil
	}
	if scalePct < 25 {
		scalePct = 25
	}
	ratio := fmt.Sprintf("%.2f", float64(scalePct)/100.0)
	return []string{"-vf", fmt.Sprintf("scale=trunc(iw*%s/2)*2:trunc(ih*%s/2)*2", ratio, ratio)}
}

func copyStream(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, src io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
