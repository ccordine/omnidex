package api

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/ollama"
)

type roleplayModelPageProbe struct {
	pages   map[int]ollama.ModelPage
	err     error
	offsets []int
	limits  []int
}

func (*roleplayModelPageProbe) HasModel(context.Context, string) (bool, error) {
	return false, nil
}

func (probe *roleplayModelPageProbe) ListModelPage(
	_ context.Context,
	limit, offset int,
) (ollama.ModelPage, error) {
	probe.limits = append(probe.limits, limit)
	probe.offsets = append(probe.offsets, offset)
	if probe.err != nil {
		return ollama.ModelPage{}, probe.err
	}
	return probe.pages[offset], nil
}

func TestLoadInstalledRoleplayModelNamesConsumesEveryPage(t *testing.T) {
	t.Parallel()
	probe := &roleplayModelPageProbe{pages: map[int]ollama.ModelPage{
		0: {
			Offset: 0, HasMore: true,
			Models: []ollama.ModelInfo{{Name: "alpha:1b"}, {Name: "beta:2b"}},
		},
		2: {
			Offset: 2,
			Models: []ollama.ModelInfo{{Name: "gamma:4b"}},
		},
	}}
	server := &Server{ollamaModelAuthority: probe}

	names, err := server.loadInstalledRoleplayModelNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"alpha:1b", "beta:2b", "gamma:4b"}) {
		t.Fatalf("names=%v", names)
	}
	if !reflect.DeepEqual(probe.offsets, []int{0, 2}) ||
		!reflect.DeepEqual(probe.limits, []int{ollama.MaxModelPageSize, ollama.MaxModelPageSize}) {
		t.Fatalf("offsets=%v limits=%v", probe.offsets, probe.limits)
	}
}

func TestLoadInstalledRoleplayModelNamesRejectsNonAdvancingPagination(t *testing.T) {
	t.Parallel()
	probe := &roleplayModelPageProbe{pages: map[int]ollama.ModelPage{
		0: {Offset: 0, HasMore: true},
	}}
	server := &Server{ollamaModelAuthority: probe}

	_, err := server.loadInstalledRoleplayModelNames(context.Background())
	if !errors.Is(err, errRoleplayModelCatalogUnavailable) ||
		!strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("error=%v", err)
	}
	if !reflect.DeepEqual(probe.offsets, []int{0}) {
		t.Fatalf("non-advancing page was retried: %v", probe.offsets)
	}
}

func TestLoadInstalledRoleplayModelNamesWrapsProviderAndAuthorityFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		probe *roleplayModelPageProbe
		want  string
	}{
		{
			name:  "provider error",
			probe: &roleplayModelPageProbe{err: errors.New("connection refused")},
			want:  "connection refused",
		},
		{
			name: "wrong offset",
			probe: &roleplayModelPageProbe{pages: map[int]ollama.ModelPage{
				0: {Offset: 7, Models: []ollama.ModelInfo{{Name: "alpha:1b"}}},
			}},
			want: "returned offset 7",
		},
		{
			name: "duplicate model",
			probe: &roleplayModelPageProbe{pages: map[int]ollama.ModelPage{
				0: {
					Offset: 0,
					Models: []ollama.ModelInfo{{Name: "alpha:1b"}, {Name: "alpha:1b"}},
				},
			}},
			want: "duplicated",
		},
		{
			name: "inexact model name",
			probe: &roleplayModelPageProbe{pages: map[int]ollama.ModelPage{
				0: {Offset: 0, Models: []ollama.ModelInfo{{Name: " alpha:1b"}}},
			}},
			want: "is invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := &Server{ollamaModelAuthority: test.probe}
			_, err := server.loadInstalledRoleplayModelNames(context.Background())
			if !errors.Is(err, errRoleplayModelCatalogUnavailable) ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want sentinel and %q", err, test.want)
			}
		})
	}
}
