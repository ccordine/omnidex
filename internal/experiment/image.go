package experiment

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var imageReferencePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*:[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ResolveImage acquires one code-selected technical image and returns its actual
// immutable daemon identity. Missing local images are acquired explicitly.
func (docker *Docker) ResolveImage(ctx context.Context, reference string) (string, error) {
	if docker == nil || docker.cli == nil || ctx == nil || len(reference) > 256 || !imageReferencePattern.MatchString(reference) {
		return "", fmt.Errorf("Docker image acquisition requires one exact tagged reference and client context")
	}
	stdout, stderr, err := docker.capture(ctx, []string{"image", "ls", "--all", "--quiet", "--no-trunc", "--filter", "reference=" + reference}, nil)
	if err != nil {
		return "", dockerOperationError("inspect local image candidates", stderr, err)
	}
	ids := strings.Fields(string(stdout))
	if len(ids) > 1 {
		return "", fmt.Errorf("Docker image reference resolved to multiple local identities")
	}
	if len(ids) == 0 {
		_, stderr, err := docker.capture(ctx, []string{"image", "pull", "--quiet", reference}, nil)
		if err != nil {
			return "", dockerOperationError("acquire technical image", stderr, err)
		}
	}
	stdout, stderr, err = docker.capture(ctx, []string{"image", "inspect", "--format", "{{.Id}}", reference}, nil)
	if err != nil {
		return "", dockerOperationError("observe acquired image identity", stderr, err)
	}
	id := strings.TrimSpace(string(stdout))
	if !imageIDPattern.MatchString(id) || (len(ids) == 1 && ids[0] != id) {
		return "", fmt.Errorf("Docker image acquisition did not retain one exact immutable identity")
	}
	return id, nil
}
