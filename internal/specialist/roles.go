package specialist

import (
	"fmt"
	"strings"
)

type Role struct {
	ID               string
	Name             string
	Scope            string
	Responsibilities []string
}

const (
	RolePlannerSpecialist            = "planner_specialist"
	RoleToolingSpecialist            = "tooling_specialist"
	RoleFilesystemResearchSpecialist = "filesystem_research_specialist"
	RoleIntentTaggingSpecialist      = "intent_tagging_specialist"
	RoleMemoryRetrievalSpecialist    = "memory_retrieval_specialist"
	RoleWebResearchSpecialist        = "web_research_specialist"
	RoleAnalysisSpecialist           = "analysis_specialist"
	RoleResponseSpecialist           = "response_specialist"
	RoleReviewVerificationSpecialist = "review_verification_specialist"
	RoleMediaControlSpecialist       = "media_control_specialist"
	RoleBrowserInspectionSpecialist  = "browser_inspection_specialist"
	RoleScreenVisionSpecialist       = "screen_vision_specialist"
	RoleShellExecutionSpecialist     = "shell_execution_specialist"
	RoleAudioNotesSpecialist         = "audio_notes_specialist"
	RoleOrchestrationSpecialist      = "orchestration_specialist"
	RoleMemorySpecialist             = "memory_specialist"
	RoleCorrectionSpecialist         = "correction_specialist"
	RoleManagerSpecialist            = "manager_specialist"
	RoleExpectationSpecialist        = "expectation_specialist"
	RoleResearchSpecialist           = "research_specialist"
	RoleCodeSpecialist               = "code_specialist"
	RoleWorkerSpecialist             = "worker_specialist"
	RoleSummarySpecialist            = "summary_specialist"
)

func ForPipelineAction(action string) Role {
	action = strings.TrimPrefix(normalize(action), "v3_")
	switch action {
	case "intent_parse", "plan", "planning":
		return Role{
			ID:    RolePlannerSpecialist,
			Name:  "Planning Specialist",
			Scope: "break requests into executable, verifiable micro-steps",
			Responsibilities: []string{
				"produce actionable plans with completion criteria",
				"sequence specialist actions in the safest order",
				"flag missing information only when truly blocking",
			},
		}
	case "capability_audit", "tooling":
		return Role{
			ID:    RoleToolingSpecialist,
			Name:  "Tooling Specialist",
			Scope: "validate environment/tool availability and risk controls",
			Responsibilities: []string{
				"check required tools and host capabilities",
				"surface missing-tool and approval constraints",
				"prepare execution-safe assumptions",
			},
		}
	case "workspace_scan", "workspace_research":
		return Role{
			ID:    RoleFilesystemResearchSpecialist,
			Name:  "Filesystem Research Specialist",
			Scope: "collect repository/workspace evidence for planning and execution",
			Responsibilities: []string{
				"inspect workspace snapshot and relevant files",
				"provide grounded repository context",
				"avoid speculative assumptions about file state",
			},
		}
	case "tag":
		return Role{
			ID:    RoleIntentTaggingSpecialist,
			Name:  "Intent Tagging Specialist",
			Scope: "classify request intent for retrieval/routing",
			Responsibilities: []string{
				"derive concise high-signal tags",
				"improve retrieval and specialist routing precision",
			},
		}
	case "retrieve", "memory_retrieval":
		return Role{
			ID:    RoleMemoryRetrievalSpecialist,
			Name:  "Memory Retrieval Specialist",
			Scope: "collect relevant historical memory context",
			Responsibilities: []string{
				"retrieve and rank memory candidates",
				"prefer relevant/project/session-scoped context",
				"avoid treating stale memory as run evidence",
			},
		}
	case "web_search", "external_research", "subtask":
		return Role{
			ID:    RoleWebResearchSpecialist,
			Name:  "Web Research Specialist",
			Scope: "gather fresh external evidence when needed",
			Responsibilities: []string{
				"run focused web queries",
				"return source-grounded context for downstream steps",
				"skip when freshness/external data is unnecessary",
			},
		}
	case "analyze", "analysis":
		return Role{
			ID:    RoleAnalysisSpecialist,
			Name:  "Analysis Specialist",
			Scope: "synthesize available context into execution guidance",
			Responsibilities: []string{
				"summarize high-impact constraints and facts",
				"highlight assumptions and blockers",
				"preserve grounding boundaries for response step",
			},
		}
	case "assist", "roleplay", "narrate", "response_draft", "finalize":
		return Role{
			ID:    RoleResponseSpecialist,
			Name:  "Response Specialist",
			Scope: "compose user-facing response from grounded context",
			Responsibilities: []string{
				"write concise, directly useful output",
				"align claims with available evidence",
				"carry forward assumptions explicitly",
			},
		}
	case "verify", "verification", "memory_review":
		return Role{
			ID:    RoleReviewVerificationSpecialist,
			Name:  "Review & Verification Specialist",
			Scope: "audit grounding and run verification loops",
			Responsibilities: []string{
				"validate response claims and completeness",
				"run tests when applicable",
				"enforce retry/replan when evidence is missing",
			},
		}
	default:
		return Role{
			ID:    RoleOrchestrationSpecialist,
			Name:  "Orchestration Specialist",
			Scope: "coordinate unknown or fallback actions",
			Responsibilities: []string{
				"route work to available specialists",
				"maintain safe, deterministic execution flow",
			},
		}
	}
}

func ForLocalCapability(kind string) Role {
	switch normalize(kind) {
	case "local_media":
		return Role{
			ID:    RoleMediaControlSpecialist,
			Name:  "Media Control Specialist",
			Scope: "control/inspect local media playback (VLC/MPRIS/playerctl)",
			Responsibilities: []string{
				"handle playback state and next-episode navigation",
				"inspect subtitle/context queries around current playback",
			},
		}
	case "local_browser":
		return Role{
			ID:    RoleBrowserInspectionSpecialist,
			Name:  "Browser Inspection Specialist",
			Scope: "inspect local browser tabs/processes/console activity",
			Responsibilities: []string{
				"scan open tabs and browser processes",
				"collect devtools console/email-watch context when requested",
			},
		}
	case "local_screen":
		return Role{
			ID:    RoleScreenVisionSpecialist,
			Name:  "Screen Vision Specialist",
			Scope: "capture local screen and extract OCR/vision context",
			Responsibilities: []string{
				"capture screenshots safely",
				"produce OCR and targeted visual summaries",
			},
		}
	case "local_shell":
		return Role{
			ID:    RoleShellExecutionSpecialist,
			Name:  "Shell Execution Specialist",
			Scope: "execute local shell/file actions under safety policy",
			Responsibilities: []string{
				"run allowlisted commands and file operations",
				"surface command output plus repo change evidence",
			},
		}
	case "local_audio":
		return Role{
			ID:    RoleAudioNotesSpecialist,
			Name:  "Audio Notes Specialist",
			Scope: "manage local audio notes capture/search lifecycle",
			Responsibilities: []string{
				"start/stop/status audio notes sessions",
				"search captured note transcripts",
			},
		}
	case "core_job":
		return ForPipelineAction("plan")
	default:
		return Role{
			ID:    RoleOrchestrationSpecialist,
			Name:  "Orchestration Specialist",
			Scope: "route requests to the correct local or core specialist",
			Responsibilities: []string{
				"choose safest capable automation path",
				"fallback to core pipeline when needed",
			},
		}
	}
}

func Summary(role Role) string {
	role = normalizeRole(role)
	return fmt.Sprintf("%s (%s): %s", role.Name, role.ID, role.Scope)
}

func DetailLines(role Role) []string {
	role = normalizeRole(role)
	lines := []string{
		fmt.Sprintf("Specialist: %s", role.Name),
		"Specialist ID: " + role.ID,
		"Specialist Scope: " + role.Scope,
	}
	if len(role.Responsibilities) > 0 {
		lines = append(lines, "Specialist Responsibilities:")
		for _, responsibility := range role.Responsibilities {
			clean := strings.TrimSpace(responsibility)
			if clean == "" {
				continue
			}
			lines = append(lines, "- "+clean)
		}
	}
	return lines
}

func normalizeRole(role Role) Role {
	clean := Role{
		ID:    strings.TrimSpace(role.ID),
		Name:  strings.TrimSpace(role.Name),
		Scope: strings.TrimSpace(role.Scope),
	}
	if clean.ID == "" {
		clean.ID = RoleOrchestrationSpecialist
	}
	if clean.Name == "" {
		clean.Name = "Orchestration Specialist"
	}
	if clean.Scope == "" {
		clean.Scope = "route and coordinate execution"
	}
	clean.Responsibilities = make([]string, 0, len(role.Responsibilities))
	for _, responsibility := range role.Responsibilities {
		text := strings.TrimSpace(responsibility)
		if text == "" {
			continue
		}
		clean.Responsibilities = append(clean.Responsibilities, text)
	}
	return clean
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func EnvVarForRoleID(roleID string) string {
	switch normalize(roleID) {
	case RolePlannerSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_PLANNER"
	case RoleToolingSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_TOOLING"
	case RoleFilesystemResearchSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_FILESYSTEM_RESEARCH"
	case RoleIntentTaggingSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_INTENT_TAGGING"
	case RoleMemoryRetrievalSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_MEMORY_RETRIEVAL"
	case RoleWebResearchSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_WEB_RESEARCH"
	case RoleAnalysisSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_ANALYSIS"
	case RoleResponseSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_RESPONSE"
	case RoleReviewVerificationSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_REVIEW_VERIFICATION"
	case RoleMediaControlSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_MEDIA_CONTROL"
	case RoleBrowserInspectionSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_BROWSER_INSPECTION"
	case RoleScreenVisionSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_SCREEN_VISION"
	case RoleShellExecutionSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_SHELL_EXECUTION"
	case RoleAudioNotesSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_AUDIO_NOTES"
	case RoleMemorySpecialist:
		return "OLLAMA_MODEL_SPECIALIST_MEMORY"
	case RoleCorrectionSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_CORRECTION"
	case RoleManagerSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_MANAGER"
	case RoleExpectationSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_EXPECTATION"
	case RoleResearchSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_RESEARCH"
	case RoleCodeSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_CODE"
	case RoleWorkerSpecialist:
		return "OLLAMA_MODEL_SPECIALIST_WORKER"
	case RoleSummarySpecialist:
		return "OLLAMA_MODEL_SPECIALIST_SUMMARY"
	default:
		return ""
	}
}
