package api

import (
	"strings"
	"testing"
)

func TestChatProgressClientAppliesOnlyBoundedServerComponentAuthority(t *testing.T) {
	t.Parallel()
	execution := readFrontendSource(t, "web/src/lib/chat_execution_coordinator.ts")
	jobs := readFrontendSource(t, "web/src/lib/chat_jobs_coordinator.ts")
	contract := readFrontendSource(t, "web/src/lib/chat_execution_contract.ts")
	view := readFrontendSource(t, "web/src/controllers/chat_view_controller.ts")

	for path, source := range map[string]string{
		"execution": execution,
		"jobs":      jobs,
	} {
		if !strings.Contains(source, "/v1/ui/chat/jobs/") {
			t.Errorf("%s coordinator does not read the bounded chat job state endpoint", path)
		}
		if strings.Contains(source, "fetch(`/v1/jobs/${") {
			t.Errorf("%s coordinator still reads raw job contexts", path)
		}
	}
	for _, required := range []string{
		"current_generation", "latest_context_id", "count > 24",
		"escaped current generation", "bounded progress authority",
	} {
		if !strings.Contains(contract, required) {
			t.Errorf("typed chat execution contract lacks %q", required)
		}
	}
	for path, source := range map[string]string{
		"execution": execution, "jobs": jobs, "view": view,
	} {
		for _, forbidden := range []string{"innerHTML =", "insertAdjacentHTML(", "createElement(\"li\")", "createElement('li')"} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s client renders progress/component markup via %q", path, forbidden)
			}
		}
	}
	if !strings.Contains(execution, "requireServerComponentBundle(payload") ||
		!strings.Contains(execution, "renderJobState(jobStateBundle)") ||
		!strings.Contains(view, "renderJobState(bundle: string)") ||
		!strings.Contains(view, "this.renderComponentBundle(bundle)") ||
		!strings.Contains(view, "this.recyclrController.renderBundle(bundle)") {
		t.Fatal("chat view does not apply required server markup through the Recyclr bridge")
	}
}
