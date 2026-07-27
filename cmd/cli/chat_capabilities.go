package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/specialist"
)

func enabledAutomationCapabilities(
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
) []string {
	lines := make([]string, 0, 5)
	if localShell {
		role := specialist.ForLocalCapability("local_shell")
		lines = append(lines, "- local_shell: run local shell commands, inspect files, build code, and run tests in the current directory. (specialist: "+role.ID+")")
	}
	if localMedia {
		role := specialist.ForLocalCapability("local_media")
		lines = append(lines, "- local_media: inspect player status/metadata and control playback when explicitly requested. (specialist: "+role.ID+")")
	}
	if localBrowser {
		role := specialist.ForLocalCapability("local_browser")
		lines = append(lines, "- local_browser: inspect local browser processes, tabs, and console activity when available. (specialist: "+role.ID+")")
	}
	if localScreen {
		role := specialist.ForLocalCapability("local_screen")
		lines = append(lines, "- local_screen: capture local screenshots and extract OCR/vision summaries. (specialist: "+role.ID+")")
	}
	if localAudio {
		role := specialist.ForLocalCapability("local_audio")
		lines = append(lines, "- local_audio: manage local audio notes capture, status, and search. (specialist: "+role.ID+")")
	}
	return lines
}

func describePlaybackControlIntent(intent playbackControlIntent) string {
	switch strings.ToLower(strings.TrimSpace(intent.Action)) {
	case "play":
		return "resume/play the active local media player (VLC via MPRIS/playerctl)"
	case "pause":
		return "pause the active local media player (VLC via MPRIS/playerctl)"
	case "play-pause":
		return "toggle play/pause on the active local media player"
	case "status":
		return "check what is currently playing on the active local media player (status/title/path)"
	default:
		return "control local media playback"
	}
}

func describeNextEpisodeIntent(intent nextEpisodeIntent) string {
	if strings.TrimSpace(intent.ShowHint) != "" {
		return fmt.Sprintf("play the next episode near the current media file, preferring show `%s`", intent.ShowHint)
	}
	return "play the next episode near the current media file"
}

func describePlaybackContextIntent(intent playbackContextIntent) string {
	if strings.TrimSpace(intent.Query) != "" {
		return fmt.Sprintf("inspect current playback subtitles around now and focus on `%s`", intent.Query)
	}
	return "inspect current playback subtitles around the current timestamp"
}

func describeBrowserScanIntent(intent browserScanIntent) string {
	if intent.EmailWatch {
		return "inspect local email tabs and report what is newly visible in inbox views"
	}
	if intent.WithConsole {
		return fmt.Sprintf("scan local browser tabs and read JavaScript console activity for %ds", intent.Seconds)
	}
	return "scan local browser processes and open tabs"
}

func describeScreenReadIntent(intent screenReadIntent) string {
	mode := "OCR text"
	if intent.WithOCR && intent.WithVision {
		mode = "OCR text + vision summary"
	} else if intent.WithVision {
		mode = "vision summary"
	}
	if strings.TrimSpace(intent.Prompt) != "" {
		return fmt.Sprintf("capture your screen and run %s focused on `%s`", mode, intent.Prompt)
	}
	return fmt.Sprintf("capture your screen and run %s", mode)
}

func describeLocalShellIntent(intent localShellIntent) string {
	switch intent.Action {
	case "create_file":
		target := strings.TrimSpace(intent.Target)
		if target == "" {
			target = "test"
		}
		return fmt.Sprintf("create file `%s` in the current directory", target)
	case "rename_file":
		return fmt.Sprintf("rename `%s` to `%s` in the current directory", strings.TrimSpace(intent.Source), strings.TrimSpace(intent.Target))
	case "run_command":
		return fmt.Sprintf("run local command `%s` in the current directory", strings.TrimSpace(intent.Command))
	case "show_system_summary":
		return "inspect local system summary (user, OS, shell, cwd, time)"
	case "show_running_processes":
		return "inspect currently running processes using multiple local strategies (top/ps/services)"
	case "show_ip":
		return "inspect local/public IP information"
	case "show_open_ports":
		return "inspect open local ports"
	case "show_open_ports_detailed":
		return "inspect open local ports with process details (may require sudo)"
	case "show_network_profile":
		return "inspect local network profile (IP/location/VPN/tools)"
	case "show_network_location":
		return "estimate location from current public IP"
	case "show_vpn_status":
		return "inspect VPN interface/process/connection status"
	case "show_network_tools_catalog":
		return "list available local/web network tools"
	case "show_repo_walkthrough":
		return "walk through git repository changes in chronological order to resume project context"
	case "install_network_tools":
		return "run the local network-tools install helper script"
	default:
		return "execute a local shell action"
	}
}

func describeLocalAudioNotesIntent(intent localAudioNotesIntent) string {
	switch intent.Action {
	case "start":
		return "start audio notes capture for this session"
	case "stop":
		return "stop audio notes capture"
	case "status":
		return "check current audio notes status"
	case "search":
		return fmt.Sprintf("search audio notes for `%s`", strings.TrimSpace(intent.Query))
	default:
		return "run an audio notes action"
	}
}
