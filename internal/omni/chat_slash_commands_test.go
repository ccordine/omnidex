package omni

import "testing"

func TestParseChatSlashCommandBuildPlanThink(t *testing.T) {
	cases := []struct {
		input    string
		kind     chatSlashKind
		objective string
		forceExec bool
		thinkOnly bool
		taskMode  TaskMode
	}{
		{"/build a react calculator", chatSlashTurn, "a react calculator", true, false, TaskModeCreateProject},
		{"/plan auth migration", chatSlashTurn, "auth migration", true, false, TaskModeInspectOnly},
		{"/think what is rust?", chatSlashTurn, "what is rust?", false, true, ""},
		{"/search tailwind display box", chatSlashSearch, "tailwind display box", false, false, ""},
		{"/research go generics", chatSlashResearch, "go generics", false, false, ""},
		{"/thoughts turn_000001", chatSlashThoughts, "turn_000001", false, false, ""},
		{"/job fix tests", chatSlashManage, "fix tests", false, false, ""},
		{"/queue scaffold api", chatSlashMicro, "scaffold api", false, false, ""},
	}
	for _, tc := range cases {
		cmd, ok := parseChatSlashCommand(tc.input)
		if !ok {
			t.Fatalf("parseChatSlashCommand(%q) = false", tc.input)
		}
		if cmd.Kind != tc.kind {
			t.Fatalf("parseChatSlashCommand(%q).Kind = %v, want %v", tc.input, cmd.Kind, tc.kind)
		}
		if tc.kind == chatSlashTurn {
			if cmd.Turn.Objective != tc.objective {
				t.Fatalf("objective = %q, want %q", cmd.Turn.Objective, tc.objective)
			}
			if cmd.Turn.ForceExecution != tc.forceExec {
				t.Fatalf("forceExecution = %t, want %t", cmd.Turn.ForceExecution, tc.forceExec)
			}
			if cmd.Turn.ThinkOnly != tc.thinkOnly {
				t.Fatalf("thinkOnly = %t, want %t", cmd.Turn.ThinkOnly, tc.thinkOnly)
			}
			if tc.taskMode != "" && cmd.Turn.TaskMode != tc.taskMode {
				t.Fatalf("taskMode = %q, want %q", cmd.Turn.TaskMode, tc.taskMode)
			}
		}
		if tc.kind == chatSlashSearch || tc.kind == chatSlashResearch || tc.kind == chatSlashManage || tc.kind == chatSlashMicro || tc.kind == chatSlashThoughts {
			if cmd.Args != tc.objective {
				t.Fatalf("args = %q, want %q", cmd.Args, tc.objective)
			}
		}
	}
}

func TestParseChatSlashCommandUsageErrors(t *testing.T) {
	cmd, ok := parseChatSlashCommand("/build")
	if !ok || cmd.Kind != chatSlashUsageError {
		t.Fatalf("expected usage error for bare /build, got %#v", cmd)
	}
}

func TestResearchCommandQueryWrapper(t *testing.T) {
	query, ok := researchCommandQuery("/research Thailand weather now")
	if !ok || query != "Thailand weather now" {
		t.Fatalf("researchCommandQuery = %q, %t", query, ok)
	}
}
