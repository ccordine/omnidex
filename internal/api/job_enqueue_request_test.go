package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestDecodeGenericCodingEnqueueAcceptsOneClientWorkspaceRoot(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(`{
		"instruction":"make the requested change",
		"pipeline":"coding",
		"metadata":{"client_cwd":"/host/projects/example","session_id":"session-1"}
	}`))

	decoded, err := decodeGenericCodingEnqueue(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatalf("decode coding enqueue: %v", err)
	}
	if decoded.Pipeline != model.PipelineCoding || decoded.Metadata == nil ||
		decoded.Metadata.ClientCWD != "/host/projects/example" {
		t.Fatalf("decoded request has wrong workspace authority: %+v", decoded)
	}
}

func TestDecodeGenericCodingEnqueueRejectsRemovedHostWorkspaceCopy(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(`{
		"instruction":"make the requested change",
		"pipeline":"coding",
		"metadata":{
			"client_cwd":"/host/projects/example",
			"host_env_cwd":"/host/projects/example"
		}
	}`))

	_, err := decodeGenericCodingEnqueue(httptest.NewRecorder(), request)
	if err == nil || !strings.Contains(err.Error(), "host_env_cwd") {
		t.Fatalf("removed host_env_cwd was not rejected explicitly: %v", err)
	}
}

func TestValidateGenericCodingMetadataRejectsNonCanonicalClientWorkspace(t *testing.T) {
	for _, workspace := range []string{"", "relative/project", "/host/projects/../project", " /host/project"} {
		err := validateGenericCodingMetadata(genericCodingMetadata{ClientCWD: workspace})
		if err == nil {
			t.Fatalf("non-canonical client_cwd %q was accepted", workspace)
		}
	}
}
