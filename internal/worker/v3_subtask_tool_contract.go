package worker

import (
	"strconv"
	"strings"
)

const subtaskToolControlCommand = "CONTROL_PLANE_COMMAND: Execute the SPECIALIST_INVOCATION above now. Return exactly one raw JSON object matching one concrete tool-turn envelope. Do not use markdown fences, acknowledge the command, copy an envelope template, or discuss the request."

func subtaskToolResponseContract(roleID string) string {
	role := strconv.Quote(strings.TrimSpace(roleID))
	return strings.Join([]string{
		`TOOL_CALL_ENVELOPE: {"role_id":` + role + `,"status":"continue","tool_calls":[{"name":"tool.name","input":{}}],"final":"","error":""}`,
		`SUCCESS_ENVELOPE: {"role_id":` + role + `,"status":"success","tool_calls":[],"final":"concrete grounded result","error":""}`,
		`BLOCKED_ENVELOPE: {"role_id":` + role + `,"status":"blocked","tool_calls":[],"final":"","error":"explicit blocker"}`,
		`FAILURE_ENVELOPE: {"role_id":` + role + `,"status":"fail","tool_calls":[],"final":"","error":"explicit failure"}`,
		"Choose exactly one envelope and replace every placeholder with the concrete decision. Never emit pipe-delimited status choices.",
	}, "\n")
}
