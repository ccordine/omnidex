package main

import (
	"fmt"
	"strings"
)

func buildChatActionCandidate(
	line string,
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
	shellState *localShellState,
) *chatActionCandidate {
	clean := strings.TrimSpace(line)
	if clean == "" {
		return nil
	}

	kind := matchChatCapabilityKind(clean, localMedia, localBrowser, localScreen, localShell, localAudio)
	switch kind {
	case "local_media":
		if intent, ok := parsePlaybackControlIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_media",
				Input:   clean,
				Summary: describePlaybackControlIntent(intent),
			})
		}
		if intent, ok := parseNextEpisodeIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_media",
				Input:   clean,
				Summary: describeNextEpisodeIntent(intent),
			})
		}
		if intent, ok := parsePlaybackContextIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_media",
				Input:   clean,
				Summary: describePlaybackContextIntent(intent),
			})
		}
		return withCandidateSpecialist(&chatActionCandidate{
			Kind:    "local_media",
			Input:   clean,
			Summary: fmt.Sprintf("use local media capabilities to handle `%s`", truncateForWatch(clean, 200)),
		})
	case "local_browser":
		if intent, ok := parseBrowserScanIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_browser",
				Input:   clean,
				Summary: describeBrowserScanIntent(intent),
			})
		}
		return withCandidateSpecialist(&chatActionCandidate{
			Kind:    "local_browser",
			Input:   clean,
			Summary: fmt.Sprintf("use local browser inspection capabilities to handle `%s`", truncateForWatch(clean, 200)),
		})
	case "local_screen":
		if intent, ok := parseScreenReadIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_screen",
				Input:   clean,
				Summary: describeScreenReadIntent(intent),
			})
		}
		return withCandidateSpecialist(&chatActionCandidate{
			Kind:    "local_screen",
			Input:   clean,
			Summary: fmt.Sprintf("use local screen read capabilities to handle `%s`", truncateForWatch(clean, 200)),
		})
	case "local_shell":
		if intent, ok := parseLocalShellIntent(clean, shellState); ok {
			if shouldRouteLocalShellIntentToCore(clean, intent) {
				break
			}
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_shell",
				Input:   clean,
				Summary: describeLocalShellIntent(intent),
			})
		}
		break
	case "local_audio":
		if intent, ok := parseLocalAudioNotesIntent(clean); ok {
			return withCandidateSpecialist(&chatActionCandidate{
				Kind:    "local_audio",
				Input:   clean,
				Summary: describeLocalAudioNotesIntent(intent),
			})
		}
		return withCandidateSpecialist(&chatActionCandidate{
			Kind:    "local_audio",
			Input:   clean,
			Summary: fmt.Sprintf("use local audio notes capabilities to handle `%s`", truncateForWatch(clean, 200)),
		})
	}

	return withCandidateSpecialist(&chatActionCandidate{
		Kind:    "core_job",
		Input:   clean,
		Summary: fmt.Sprintf("submit this request to the core pipeline (`%s`) and run planning -> execution -> review", truncateForWatch(clean, 220)),
	})
}

func requiresActionConfirmation(confirmActions bool, candidate *chatActionCandidate) bool {
	if !confirmActions || candidate == nil {
		return false
	}
	return strings.TrimSpace(candidate.Kind) != "core_job"
}

func shouldAutoApproveCandidate(candidate *chatActionCandidate) bool {
	if candidate == nil {
		return false
	}
	switch strings.TrimSpace(candidate.Kind) {
	case "local_shell":
		intent, ok := parseLocalShellIntent(candidate.Input, nil)
		if !ok {
			return false
		}
		switch strings.TrimSpace(intent.Action) {
		case "create_file",
			"rename_file",
			"run_command",
			"show_system_summary",
			"show_running_processes",
			"show_ip",
			"show_open_ports",
			"show_network_profile",
			"show_network_location",
			"show_vpn_status",
			"show_network_tools_catalog",
			"show_repo_walkthrough":
			return true
		default:
			return false
		}
	case "local_media":
		if intent, ok := parsePlaybackControlIntent(candidate.Input); ok {
			return strings.EqualFold(strings.TrimSpace(intent.Action), "status")
		}
		if _, ok := parsePlaybackContextIntent(candidate.Input); ok {
			return true
		}
		return false
	default:
		return false
	}
}

func shouldRouteLocalShellIntentToCore(input string, intent localShellIntent) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return false
	}
	switch strings.TrimSpace(intent.Action) {
	case "create_file":
		authoringRequest := containsAnyPhrase(lower, []string{
			"write ",
			"populate ",
			"fill ",
			"add ",
			"build ",
			"implement ",
			"design ",
			"style ",
			"template",
			"boilerplate",
			"content",
			"with html",
			"with css",
			"with javascript",
			"including html",
			"containing html",
		})
		frontendContext := containsAnyPhrase(lower, []string{
			"landing page",
			"tailwind",
			"tailwind css",
			"css",
			"javascript",
			"react",
			"vue",
			"svelte",
			"component",
			"feature",
			"codebase",
			"existing codebase",
		})
		if authoringRequest && frontendContext {
			return true
		}
	}
	return false
}
